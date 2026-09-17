//go:build linux

package nethelper

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
	"nilo.local/nilo/internal/network"
)

// Adapter configuration and Wi-Fi through NetworkManager. NILO owns one
// connection profile per adapter, named after the adapter's permanent hardware
// address and pinned to it, so the profile follows the adapter and never
// silently attaches to a replacement. The operating system's own profiles are
// never edited: a NILO configuration is a separate profile activated on the
// device, and the checkpoint puts the previous one back.

const (
	niloInterfacePrefix = "nilo-if-"
	// scanSettle is how long a requested scan is given before the access
	// points are read. NetworkManager rate-limits scans, so a refused request
	// still reads whatever the last scan found.
	scanSettle = 3 * time.Second
	// Access point security flags, from NM80211ApSecurityFlags.
	apKeyManagementPSK    = 0x100
	apKeyManagement8021X  = 0x200
	apKeyManagementSAE    = 0x400
	apKeyManagementOWE    = 0x800
	apKeyManagementSuiteB = 0x2000
	apFlagPrivacy         = 0x1
)

// ownedConnectionID names the profile NILO keeps for one adapter.
func ownedConnectionID(adapterID string) string {
	return niloInterfacePrefix + strings.ReplaceAll(strings.ToLower(adapterID), ":", "")
}

func hardwareAddressBytes(adapterID string) ([]byte, error) {
	address, err := net.ParseMAC(adapterID)
	if err != nil {
		return nil, fmt.Errorf("adapter identity %q is not a hardware address", adapterID)
	}
	return []byte(address), nil
}

// ownedConnection finds NILO's profile for an adapter, if one exists.
func (n *NetworkManager) ownedConnection(ctx context.Context, adapterID string) (dbus.ObjectPath, map[string]map[string]dbus.Variant, error) {
	var paths []dbus.ObjectPath
	if err := n.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.ListConnections", 0).Store(&paths); err != nil {
		return "", nil, sanitizedBusError("list connections", err)
	}
	wanted := ownedConnectionID(adapterID)
	for _, path := range paths {
		settings, err := n.settingsOf(ctx, path)
		if err != nil {
			continue
		}
		if value[string](settings["connection"], "id") == wanted {
			return path, settings, nil
		}
	}
	return "", nil, nil
}

// ipv4Settings renders the IPv4 half of a configuration.
func ipv4Settings(settings network.InterfaceSettings) map[string]dbus.Variant {
	ipv4 := map[string]dbus.Variant{"never-default": variant(settings.NeverDefault)}
	switch settings.Method {
	case network.MethodLinkLocal:
		ipv4["method"] = variant("link-local")
	case network.MethodStatic:
		ipv4["method"] = variant("manual")
		ipv4["address-data"] = variant([]map[string]dbus.Variant{{
			"address": variant(settings.Address.Addr().String()), "prefix": variant(uint32(settings.Address.Bits())),
		}})
		if settings.Gateway.IsValid() {
			ipv4["gateway"] = variant(settings.Gateway.String())
		}
	default:
		ipv4["method"] = variant("auto")
	}
	if len(settings.DNS) > 0 {
		var servers []uint32
		for _, server := range settings.DNS {
			if server.Is4() {
				raw := server.As4()
				servers = append(servers, binary.NativeEndian.Uint32(raw[:]))
			}
		}
		if len(servers) > 0 {
			ipv4["dns"] = variant(servers)
		}
		ipv4["ignore-auto-dns"] = variant(!settings.DNSAutomatic)
	}
	return ipv4
}

func applyDHCPIdentity(settings map[string]map[string]dbus.Variant, serialName string) error {
	if serialName == "" {
		return nil
	}
	properties, err := dhcpIdentityProperties(
		serialName, value[string](settings["ipv4"], "method"), value[string](settings["ipv6"], "method"),
	)
	if err != nil {
		return err
	}
	if properties.IPv4Hostname != "" {
		settings["ipv4"]["dhcp-send-hostname"] = variant(true)
		settings["ipv4"]["dhcp-hostname"] = variant(properties.IPv4Hostname)
		settings["ipv4"]["dhcp-vendor-class-identifier"] = variant(properties.IPv4Vendor)
	}
	if properties.IPv6Hostname != "" {
		settings["ipv6"]["dhcp-send-hostname"] = variant(true)
		settings["ipv6"]["dhcp-hostname"] = variant(properties.IPv6Hostname)
	}
	return nil
}

// writeConnection updates NILO's profile for the adapter or creates it, and
// returns its path and identity.
func (n *NetworkManager) writeConnection(ctx context.Context, adapterID string, build func(existing map[string]map[string]dbus.Variant, connectionUUID string) (map[string]map[string]dbus.Variant, error)) (dbus.ObjectPath, string, error) {
	path, existing, err := n.ownedConnection(ctx, adapterID)
	if err != nil {
		return "", "", err
	}
	connectionUUID := uuid.NewString()
	if existing != nil {
		connectionUUID = value[string](existing["connection"], "uuid")
	}
	settings, err := build(existing, connectionUUID)
	if err != nil {
		return "", "", err
	}
	if existing != nil {
		var result map[string]dbus.Variant
		if err := n.bus.Object(nm, path).CallWithContext(ctx, connectionAPI+".Update2", 0, settings, uint32(addConnectionToDisk), map[string]dbus.Variant{}).Store(&result); err != nil {
			return "", "", sanitizedBusError("NetworkManager refused the updated profile", err)
		}
		return path, connectionUUID, nil
	}
	var result map[string]dbus.Variant
	if err := n.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.AddConnection2", 0, settings, uint32(addConnectionToDisk), map[string]dbus.Variant{}).Store(&path, &result); err != nil {
		return "", "", sanitizedBusError("NetworkManager refused the new profile", err)
	}
	return path, connectionUUID, nil
}

// activateOn brings a profile up on one device and waits until the device
// carries it with both IP families settled.
func (n *NetworkManager) activateOn(ctx context.Context, path dbus.ObjectPath, device Device, connectionUUID string) error {
	if err := n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".ActivateConnection", 0, path, dbus.ObjectPath(device.Path), dbus.ObjectPath("/")).Err; err != nil {
		return sanitizedBusError("activate the profile", err)
	}
	err := n.wait(ctx, device.Interface.ID, func(now Device) bool {
		return now.Connection == connectionUUID && now.Interface.State == network.StateConnected && !now.Transitioning
	})
	if err != nil {
		// Say what the device was doing instead, which is the one thing an
		// operator needs to know when an activation does not settle.
		if last, findErr := n.find(context.Background(), device.Interface.ID); findErr == nil {
			return fmt.Errorf("the profile did not settle on %s (connection %q, state %s, transitioning %t): %w",
				device.Interface.Name, last.Connection, last.Interface.State, last.Transitioning, err)
		}
	}
	return err
}

// ApplySettings writes the configuration into NILO's profile for the adapter
// and activates it. A wireless adapter keeps the network it is on; its
// addressing changes, its join does not.
func (n *NetworkManager) ApplySettings(ctx context.Context, device Device, settings network.InterfaceSettings) error {
	adapterID := device.Interface.AdapterID
	if adapterID == "" {
		return errors.New("the adapter reports no hardware identity to bind the configuration to")
	}
	mac, err := hardwareAddressBytes(adapterID)
	if err != nil {
		return err
	}
	path, connectionUUID, err := n.writeConnection(ctx, adapterID, func(existing map[string]map[string]dbus.Variant, connectionUUID string) (map[string]map[string]dbus.Variant, error) {
		connectionType := "802-3-ethernet"
		if device.Interface.Kind == network.KindWiFi {
			if existing == nil {
				return nil, fmt.Errorf("%w: join a wireless network before configuring its addressing", network.ErrInterfaceSettingsRejected)
			}
			connectionType = "802-11-wireless"
		}
		result := map[string]map[string]dbus.Variant{
			"connection": {
				"id": variant(ownedConnectionID(adapterID)), "uuid": variant(connectionUUID), "type": variant(connectionType),
				"interface-name": variant(device.Interface.Name), "autoconnect": variant(true),
			},
			"ipv4": ipv4Settings(settings),
			"ipv6": {"method": variant("link-local")},
		}
		if connectionType == "802-3-ethernet" {
			result["802-3-ethernet"] = map[string]dbus.Variant{"mac-address": variant(mac)}
		} else {
			// Keep the wireless network and its security exactly as joined.
			result["802-11-wireless"] = existing["802-11-wireless"]
			if security, ok := existing["802-11-wireless-security"]; ok {
				result["802-11-wireless-security"] = security
			}
			result["connection"]["autoconnect"] = variant(value[bool](existing["connection"], "autoconnect"))
		}
		if err := applyDHCPIdentity(result, n.applianceSerial); err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		return err
	}
	return n.activateOn(ctx, path, device, connectionUUID)
}

// ScanWiFi requests a scan on a wireless device and lists what it found.
func (n *NetworkManager) ScanWiFi(ctx context.Context, device Device) ([]network.WiFiNetwork, error) {
	if device.Interface.Kind != network.KindWiFi {
		return nil, network.ErrUnsupported
	}
	wireless := n.bus.Object(nm, dbus.ObjectPath(device.Path))
	// A refused request means a scan ran moments ago; its result is still
	// what the device reports.
	_ = wireless.CallWithContext(ctx, deviceAPI+".Wireless.RequestScan", 0, map[string]dbus.Variant{}).Err
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(scanSettle):
	}
	var points []dbus.ObjectPath
	if err := wireless.CallWithContext(ctx, deviceAPI+".Wireless.GetAllAccessPoints", 0).Store(&points); err != nil {
		return nil, sanitizedBusError("read access points", err)
	}
	strongest := map[string]network.WiFiNetwork{}
	for _, point := range points {
		props, err := n.properties(ctx, point, nm+".AccessPoint")
		if err != nil {
			continue
		}
		ssid := string(value[[]byte](props, "Ssid"))
		if ssid == "" || len(ssid) > 32 || strings.ContainsAny(ssid, "\x00\r\n") {
			continue
		}
		found := network.WiFiNetwork{SSID: ssid, Strength: int(value[uint8](props, "Strength"))}
		found.Security, found.Supported = classifyAccessPoint(value[uint32](props, "Flags"), value[uint32](props, "WpaFlags"), value[uint32](props, "RsnFlags"))
		if current, ok := strongest[ssid]; !ok || found.Strength > current.Strength {
			strongest[ssid] = found
		}
	}
	networks := make([]network.WiFiNetwork, 0, len(strongest))
	for _, found := range strongest {
		networks = append(networks, found)
	}
	sort.Slice(networks, func(i, j int) bool {
		if networks[i].Strength != networks[j].Strength {
			return networks[i].Strength > networks[j].Strength
		}
		return networks[i].SSID < networks[j].SSID
	})
	return networks, nil
}

// classifyAccessPoint maps NetworkManager's access-point flags to the security
// this milestone joins. Enterprise, Suite-B and legacy WEP are listed as
// unsupported rather than guessed at.
func classifyAccessPoint(flags, wpa, rsn uint32) (network.WiFiSecurity, bool) {
	switch {
	case rsn&apKeyManagementSAE != 0:
		return network.WiFiWPA3Personal, true
	case (rsn|wpa)&apKeyManagementPSK != 0:
		return network.WiFiWPA2Personal, true
	case (rsn|wpa)&(apKeyManagement8021X|apKeyManagementSuiteB) != 0:
		return network.WiFiSecurityUnsupported, false
	case rsn&apKeyManagementOWE != 0:
		return network.WiFiSecurityUnsupported, false
	case flags&apFlagPrivacy != 0:
		return network.WiFiSecurityUnsupported, false
	default:
		return network.WiFiOpen, true
	}
}

// JoinWiFi writes NILO's profile for the wireless adapter with the network
// and its password, and activates it. The password is handed to
// NetworkManager as a system-owned secret and kept in its root-only keyfile.
func (n *NetworkManager) JoinWiFi(ctx context.Context, device Device, join network.WiFiJoin, password string) error {
	adapterID := device.Interface.AdapterID
	if device.Interface.Kind != network.KindWiFi || adapterID == "" {
		return fmt.Errorf("%w: %s is not a wireless adapter with a hardware identity", network.ErrNotControllable, device.Interface.Name)
	}
	mac, err := hardwareAddressBytes(adapterID)
	if err != nil {
		return err
	}
	if join.Security != network.WiFiOpen && password == "" {
		return fmt.Errorf("%w: %s needs a password", network.ErrInterfaceSettingsRejected, join.Security)
	}
	path, connectionUUID, err := n.writeConnection(ctx, adapterID, func(_ map[string]map[string]dbus.Variant, connectionUUID string) (map[string]map[string]dbus.Variant, error) {
		result := map[string]map[string]dbus.Variant{
			"connection": {
				"id": variant(ownedConnectionID(adapterID)), "uuid": variant(connectionUUID), "type": variant("802-11-wireless"),
				"interface-name": variant(device.Interface.Name), "autoconnect": variant(join.ConnectAtStartup),
			},
			"802-11-wireless": {"ssid": variant([]byte(join.SSID)), "mode": variant("infrastructure"), "mac-address": variant(mac)},
			"ipv4":            {"method": variant("auto")},
			"ipv6":            {"method": variant("auto")},
		}
		switch join.Security {
		case network.WiFiWPA2Personal:
			result["802-11-wireless-security"] = map[string]dbus.Variant{"key-mgmt": variant("wpa-psk"), "psk": variant(password), "psk-flags": variant(uint32(0))}
		case network.WiFiWPA3Personal:
			result["802-11-wireless-security"] = map[string]dbus.Variant{"key-mgmt": variant("sae"), "psk": variant(password), "psk-flags": variant(uint32(0))}
		}
		if err := applyDHCPIdentity(result, n.applianceSerial); err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		return err
	}
	return n.activateOn(ctx, path, device, connectionUUID)
}

var _ SettingsDriver = (*NetworkManager)(nil)
