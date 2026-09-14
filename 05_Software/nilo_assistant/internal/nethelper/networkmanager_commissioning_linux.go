//go:build linux

package nethelper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
	"nilo.local/nilo/internal/network"
)

const commissioningConnectionPrefix = "nilo-commissioning-"

func commissioningConnectionID(adapterID string) string {
	return commissioningConnectionPrefix + strings.ReplaceAll(strings.ToLower(adapterID), ":", "")
}

func commissioningConnectionSettings(connectionUUID string, link network.CommissioningLink) (map[string]map[string]dbus.Variant, error) {
	mac, err := hardwareAddressBytes(link.AdapterID)
	if err != nil {
		return nil, err
	}
	return map[string]map[string]dbus.Variant{
		"connection": {
			"id": variant(commissioningConnectionID(link.AdapterID)), "uuid": variant(connectionUUID),
			"type": variant("802-3-ethernet"), "interface-name": variant(link.Interface),
			"autoconnect": variant(true), "autoconnect-priority": variant(int32(-100)),
		},
		"802-3-ethernet": {"mac-address": variant(mac)},
		"ipv4": {
			"method": variant("link-local"), "never-default": variant(true),
			"ignore-auto-routes": variant(true), "ignore-auto-dns": variant(true), "may-fail": variant(false),
		},
		"ipv6": {"method": variant("disabled")},
	}, nil
}

func (n *NetworkManager) ReconcileCommissioningLinks(ctx context.Context, links []network.CommissioningLink) error {
	devices, err := n.Devices(ctx)
	if err != nil {
		return err
	}
	return n.reconcileCommissioningLinks(ctx, links, devices)
}

func (n *NetworkManager) reconcileCommissioningLinks(ctx context.Context, links []network.CommissioningLink, devices []Device) error {
	if n.commissioningStore == nil {
		return ErrCommissioningLinkUnsupported
	}
	ownership, err := n.commissioningStore.LoadCommissioningProfiles()
	if err != nil {
		return fmt.Errorf("load commissioning profile ownership: %w", err)
	}
	byAdapter := make(map[string]Device, len(devices))
	for _, device := range devices {
		if device.Interface.AdapterID != "" {
			byAdapter[strings.ToLower(device.Interface.AdapterID)] = device
		}
	}
	var paths []dbus.ObjectPath
	if err := n.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.ListConnections", 0).Store(&paths); err != nil {
		return sanitizedBusError("list commissioning profiles", err)
	}
	connections := make(map[string]struct {
		path     dbus.ObjectPath
		settings map[string]map[string]dbus.Variant
	}, len(paths))
	for _, path := range paths {
		settings, readErr := n.settingsOf(ctx, path)
		if readErr != nil {
			continue
		}
		connections[value[string](settings["connection"], "uuid")] = struct {
			path     dbus.ObjectPath
			settings map[string]map[string]dbus.Variant
		}{path: path, settings: settings}
	}
	owned := make(map[string]CommissioningProfileRecord, len(ownership.Profiles))
	for _, profile := range ownership.Profiles {
		owned[profile.AdapterID] = profile
		if connection, present := connections[profile.UUID]; present {
			if !commissioningProfileMatches(connection.settings, profile) {
				return ErrCommissioningLinkForeign
			}
			properties, propertyErr := n.properties(ctx, connection.path, connectionAPI)
			if propertyErr != nil || !value[bool](properties, "Unsaved") {
				return ErrCommissioningLinkForeign
			}
		}
	}
	desired := make(map[string]network.CommissioningLink, len(links))
	for _, link := range links {
		key := strings.ToLower(strings.TrimSpace(link.AdapterID))
		device, ok := byAdapter[key]
		profile := owned[key]
		if !ok || !commissioningDeviceEligible(device, link, profile.UUID) {
			continue
		}
		desired[key] = network.CommissioningLink{AdapterID: key, Interface: link.Interface}
	}
	for adapterID, profile := range owned {
		if _, keep := desired[adapterID]; keep {
			continue
		}
		if connection, present := connections[profile.UUID]; present {
			if err := n.bus.Object(nm, connection.path).CallWithContext(ctx, connectionAPI+".Delete", 0).Err; err != nil {
				return sanitizedBusError("remove stale commissioning profile", err)
			}
		}
		delete(owned, adapterID)
		if err := n.saveCommissioningOwnership(owned); err != nil {
			return err
		}
	}
	ids := make([]string, 0, len(desired))
	for adapterID := range desired {
		ids = append(ids, adapterID)
	}
	sort.Strings(ids)
	for _, adapterID := range ids {
		link := desired[adapterID]
		device := byAdapter[link.AdapterID]
		profile, present := owned[adapterID]
		if !present {
			profile = CommissioningProfileRecord{AdapterID: adapterID, Interface: link.Interface, UUID: uuid.NewString()}
			owned[adapterID] = profile
			if err := n.saveCommissioningOwnership(owned); err != nil {
				return err
			}
		}
		connection, connectionPresent := connections[profile.UUID]
		if !connectionPresent {
			settings, err := commissioningConnectionSettings(profile.UUID, link)
			if err != nil {
				return err
			}
			var result map[string]dbus.Variant
			if err := n.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.AddConnection2", 0,
				settings, uint32(addConnectionInMemory), map[string]dbus.Variant{}).Store(&connection.path, &result); err != nil {
				return sanitizedBusError("create volatile commissioning profile", err)
			}
			connection.settings = settings
			connections[profile.UUID] = connection
		}
		if device.Interface.LinkUp && device.Connection != profile.UUID {
			if err := n.activateOn(ctx, connection.path, device, profile.UUID); err != nil {
				return fmt.Errorf("activate commissioning link %s: %w", link.Interface, err)
			}
		}
	}
	return nil
}

func (n *NetworkManager) ConfigureCommissioningProfileStore(store CommissioningProfileStore) error {
	if store == nil {
		return errors.New("commissioning profile ownership store is required")
	}
	if _, err := store.LoadCommissioningProfiles(); err != nil {
		return err
	}
	n.commissioningStore = store
	return nil
}

func (n *NetworkManager) saveCommissioningOwnership(profiles map[string]CommissioningProfileRecord) error {
	record := CommissioningProfileOwnership{Version: 1, Profiles: make([]CommissioningProfileRecord, 0, len(profiles))}
	for _, profile := range profiles {
		record.Profiles = append(record.Profiles, profile)
	}
	sort.Slice(record.Profiles, func(first, second int) bool {
		return record.Profiles[first].AdapterID < record.Profiles[second].AdapterID
	})
	if err := n.commissioningStore.SaveCommissioningProfiles(record); err != nil {
		return fmt.Errorf("save commissioning profile ownership: %w", err)
	}
	return nil
}

func commissioningProfileMatches(settings map[string]map[string]dbus.Variant, profile CommissioningProfileRecord) bool {
	connection := settings["connection"]
	autoconnect := true
	if _, present := connection["autoconnect"]; present {
		autoconnect = value[bool](connection, "autoconnect")
	}
	mac, err := hardwareAddressBytes(profile.AdapterID)
	if err != nil {
		return false
	}
	ipv4 := settings["ipv4"]
	return value[string](connection, "id") == commissioningConnectionID(profile.AdapterID) &&
		value[string](connection, "uuid") == profile.UUID && value[string](connection, "type") == "802-3-ethernet" &&
		value[string](connection, "interface-name") == profile.Interface && autoconnect &&
		value[int32](connection, "autoconnect-priority") == -100 &&
		bytes.Equal(value[[]byte](settings["802-3-ethernet"], "mac-address"), mac) &&
		value[string](ipv4, "method") == "link-local" && value[bool](ipv4, "never-default") &&
		value[bool](ipv4, "ignore-auto-routes") && value[bool](ipv4, "ignore-auto-dns") &&
		value[string](settings["ipv6"], "method") == "disabled"
}

func commissioningDeviceEligible(device Device, link network.CommissioningLink, ownedUUID string) bool {
	if device.Interface.Name != link.Interface || device.Interface.Kind != network.KindEthernet ||
		!device.Interface.Physical || !device.Interface.Controllable || device.Transitioning ||
		hasConfiguredAddress(device.Interface.IPv4) || hasConfiguredAddress(device.Interface.IPv6) ||
		device.Interface.Gateway4 != "" || device.Interface.Gateway6 != "" {
		return false
	}
	if device.Connection != "" && device.Connection != ownedUUID {
		return false
	}
	// IPv4LL that this exact active NILO profile created is stable desired
	// state. A manually assigned or otherwise ownerless IPv4LL address is
	// existing network configuration and must remain untouched.
	if len(device.Interface.IPv4) != 0 && (ownedUUID == "" || device.Connection != ownedUUID) {
		return false
	}
	for _, available := range device.Available {
		if available != ownedUUID {
			return false
		}
	}
	return true
}

func hasConfiguredAddress(addresses []string) bool {
	for _, value := range addresses {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !prefix.Addr().IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

var _ CommissioningLinkDriver = (*NetworkManager)(nil)
