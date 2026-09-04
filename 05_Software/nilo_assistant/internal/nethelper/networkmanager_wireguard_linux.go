//go:build linux

package nethelper

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
	"nilo.local/nilo/internal/network"
)

// NetworkManager owns every NILO tunnel as an ordinary WireGuard connection
// profile. The private key and preshared keys are handed to it once, at
// creation, with system-owned secret flags, so NetworkManager keeps them in its
// own root-only keyfile; the helper keystore keeps the import copy the
// connection was created from. Neither copy is ever read back over the socket.
//
// wg-quick is never run. The one WireGuard tool used is `wg show`, read-only,
// to learn the last handshake of an active tunnel.

const (
	// niloTunnelPrefix marks the connections this helper created. Ownership is
	// decided by it, so an operator's own WireGuard profile is never touched.
	niloTunnelPrefix = "nilo-wg-"
	// niloTunnelInterface is the one kernel device every NILO profile uses.
	// One name is enough because at most one NILO tunnel is ever up.
	niloTunnelInterface = "nilo0"
	settingsPath        = dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings")
	connectionAPI       = nm + ".Settings.Connection"
	activeAPI           = nm + ".Connection.Active"
	// checkpointDisconnectNewDevices makes a rollback disconnect devices that
	// appeared after the checkpoint, which is what a tunnel device is.
	checkpointDisconnectNewDevices = 0x4
	// addConnectionToDisk persists the profile like any saved connection.
	addConnectionToDisk = 0x1
	// activeStateActivated and activeStateDeactivated are NetworkManager's
	// active-connection states.
	activeStateActivated   = 2
	activeStateDeactivated = 4
	// tunnelSettle bounds one activation or deactivation wait.
	tunnelSettle = 25 * time.Second
)

func variant(value any) dbus.Variant { return dbus.MakeVariant(value) }

// CreateTunnel writes the profile as a saved, inactive, non-autoconnecting
// NetworkManager connection and returns its identity.
func (n *NetworkManager) CreateTunnel(ctx context.Context, profileID string, profile network.Profile, secrets network.Secrets) (string, error) {
	if err := validateIdentity(profileID); err != nil {
		return "", err
	}
	if len(secrets.PresharedKeys) != len(profile.Peers) {
		return "", errors.New("key material does not match the profile's peers")
	}
	connectionUUID := uuid.NewString()
	settings, err := tunnelSettings(connectionUUID, profileID, profile, secrets)
	if err != nil {
		return "", err
	}
	var (
		path   dbus.ObjectPath
		result map[string]dbus.Variant
	)
	call := n.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.AddConnection2", 0,
		settings, uint32(addConnectionToDisk), map[string]dbus.Variant{})
	if err := call.Store(&path, &result); err != nil {
		return "", sanitizedBusError("NetworkManager refused the tunnel profile", err)
	}
	return connectionUUID, nil
}

// sanitizedBusError keeps a D-Bus error's name but not its message, because a
// settings error can echo the setting it rejected and one of those is the key.
func sanitizedBusError(context string, err error) error {
	var busErr dbus.Error
	if errors.As(err, &busErr) {
		return fmt.Errorf("%s: %s", context, busErr.Name)
	}
	return errors.New(context)
}

// tunnelSettings renders the profile as NetworkManager's connection settings.
// Routing intent is carried as imported: the peers' allowed addresses become
// the tunnel's routes and a default allowed address becomes NetworkManager's
// own policy-routed default, the same shape wg-quick produces.
func tunnelSettings(connectionUUID, profileID string, profile network.Profile, secrets network.Secrets) (map[string]map[string]dbus.Variant, error) {
	peers := make([]map[string]dbus.Variant, 0, len(profile.Peers))
	fullTunnel4, fullTunnel6 := false, false
	for index, peer := range profile.Peers {
		entry := map[string]dbus.Variant{
			"public-key":  variant(peer.PublicKey),
			"allowed-ips": variant(append([]string(nil), peer.AllowedIPs...)),
		}
		if peer.Endpoint != "" {
			entry["endpoint"] = variant(peer.Endpoint)
		}
		if peer.PersistentKeepalive > 0 {
			entry["persistent-keepalive"] = variant(uint32(peer.PersistentKeepalive))
		}
		if peer.HasPresharedKey {
			entry["preshared-key"] = variant(secrets.PresharedKeys[index])
			entry["preshared-key-flags"] = variant(uint32(0))
		}
		for _, allowed := range peer.AllowedIPs {
			switch allowed {
			case "0.0.0.0/0":
				fullTunnel4 = true
			case "::/0":
				fullTunnel6 = true
			}
		}
		peers = append(peers, entry)
	}

	ipv4 := map[string]dbus.Variant{"method": variant("disabled")}
	ipv6 := map[string]dbus.Variant{"method": variant("disabled")}
	var addresses4, addresses6 []map[string]dbus.Variant
	for _, text := range profile.Addresses {
		prefix, err := netip.ParsePrefix(text)
		if err != nil {
			return nil, fmt.Errorf("invalid tunnel address %q", text)
		}
		entry := map[string]dbus.Variant{"address": variant(prefix.Addr().String()), "prefix": variant(uint32(prefix.Bits()))}
		if prefix.Addr().Is4() {
			addresses4 = append(addresses4, entry)
		} else {
			addresses6 = append(addresses6, entry)
		}
	}
	if len(addresses4) > 0 {
		ipv4["method"] = variant("manual")
		ipv4["address-data"] = variant(addresses4)
	}
	if len(addresses6) > 0 {
		ipv6["method"] = variant("manual")
		ipv6["address-data"] = variant(addresses6)
	}

	var dns4 []uint32
	var dns6 [][]byte
	var search []string
	for _, entry := range profile.DNS {
		address, err := netip.ParseAddr(entry)
		switch {
		case err != nil:
			search = append(search, entry)
		case address.Is4():
			raw := address.As4()
			dns4 = append(dns4, binary.NativeEndian.Uint32(raw[:]))
		default:
			raw := address.As16()
			dns6 = append(dns6, raw[:])
		}
	}
	if len(dns4) > 0 {
		ipv4["dns"] = variant(dns4)
		if fullTunnel4 {
			// A full tunnel's resolver replaces the uplink's, as wg-quick does.
			ipv4["dns-priority"] = variant(int32(-1))
		}
	}
	if len(dns6) > 0 {
		ipv6["dns"] = variant(dns6)
		if fullTunnel6 {
			ipv6["dns-priority"] = variant(int32(-1))
		}
	}
	if len(search) > 0 {
		if len(addresses4) > 0 {
			ipv4["dns-search"] = variant(search)
		}
		if len(addresses6) > 0 {
			ipv6["dns-search"] = variant(search)
		}
	}

	wireguard := map[string]dbus.Variant{
		"private-key":       variant(secrets.PrivateKey),
		"private-key-flags": variant(uint32(0)),
		"peers":             variant(peers),
	}
	if profile.MTU > 0 {
		wireguard["mtu"] = variant(uint32(profile.MTU))
	}
	if profile.ListenPort > 0 {
		wireguard["listen-port"] = variant(uint32(profile.ListenPort))
	}
	return map[string]map[string]dbus.Variant{
		"connection": {
			"id":             variant(niloTunnelPrefix + profileID),
			"uuid":           variant(connectionUUID),
			"type":           variant("wireguard"),
			"interface-name": variant(niloTunnelInterface),
			"autoconnect":    variant(false),
		},
		"wireguard": wireguard,
		"ipv4":      ipv4,
		"ipv6":      ipv6,
	}, nil
}

// connectionPath resolves a connection identity to its object path.
func (n *NetworkManager) connectionPath(ctx context.Context, connectionUUID string) (dbus.ObjectPath, error) {
	var path dbus.ObjectPath
	err := n.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.GetConnectionByUuid", 0, connectionUUID).Store(&path)
	if err != nil {
		return "", fmt.Errorf("no connection %s: %w", connectionUUID, sanitizedBusError("lookup", err))
	}
	return path, nil
}

// settingsOf reads a connection's settings. NetworkManager omits secrets from
// this call, so nothing read here can carry a key.
func (n *NetworkManager) settingsOf(ctx context.Context, path dbus.ObjectPath) (map[string]map[string]dbus.Variant, error) {
	var settings map[string]map[string]dbus.Variant
	err := n.bus.Object(nm, path).CallWithContext(ctx, connectionAPI+".GetSettings", 0).Store(&settings)
	return settings, err
}

// DeleteTunnel deletes a NILO-owned connection profile. An external profile is
// refused by name; a profile that is already gone is not an error.
func (n *NetworkManager) DeleteTunnel(ctx context.Context, connectionUUID string) error {
	path, err := n.connectionPath(ctx, connectionUUID)
	if err != nil {
		return nil
	}
	settings, err := n.settingsOf(ctx, path)
	if err != nil {
		return sanitizedBusError("read the tunnel profile", err)
	}
	if !strings.HasPrefix(value[string](settings["connection"], "id"), niloTunnelPrefix) {
		return fmt.Errorf("%w: connection %s was not created by NILO", network.ErrExternalProfile, connectionUUID)
	}
	if err := n.bus.Object(nm, path).CallWithContext(ctx, connectionAPI+".Delete", 0).Err; err != nil {
		return sanitizedBusError("delete the tunnel profile", err)
	}
	return nil
}

// activeTunnels maps connection identities to the active connection path and
// device of every activated WireGuard connection.
type activeTunnel struct {
	path   dbus.ObjectPath
	device string
	state  uint32
}

func (n *NetworkManager) activeTunnels(ctx context.Context) (map[string]activeTunnel, error) {
	props, err := n.properties(ctx, nmPath, nm)
	if err != nil {
		return nil, err
	}
	result := map[string]activeTunnel{}
	for _, path := range value[[]dbus.ObjectPath](props, "ActiveConnections") {
		active, err := n.properties(ctx, path, activeAPI)
		if err != nil {
			continue
		}
		if value[string](active, "Type") != "wireguard" {
			continue
		}
		entry := activeTunnel{path: path, state: value[uint32](active, "State")}
		for _, devicePath := range value[[]dbus.ObjectPath](active, "Devices") {
			if deviceProps, err := n.properties(ctx, devicePath, deviceAPI); err == nil {
				entry.device = value[string](deviceProps, "Interface")
				break
			}
		}
		result[value[string](active, "Uuid")] = entry
	}
	return result, nil
}

// Tunnels lists every WireGuard connection profile with its state and public
// configuration.
func (n *NetworkManager) Tunnels(ctx context.Context) ([]network.TunnelObservation, error) {
	var paths []dbus.ObjectPath
	if err := n.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.ListConnections", 0).Store(&paths); err != nil {
		return nil, sanitizedBusError("list connections", err)
	}
	if len(paths) > 512 {
		return nil, errors.New("too many saved connections")
	}
	active, err := n.activeTunnels(ctx)
	if err != nil {
		return nil, err
	}
	tunnels := make([]network.TunnelObservation, 0)
	for _, path := range paths {
		settings, err := n.settingsOf(ctx, path)
		if err != nil {
			continue
		}
		connection := settings["connection"]
		if value[string](connection, "type") != "wireguard" {
			continue
		}
		id := value[string](connection, "id")
		observation := network.TunnelObservation{
			ConnectionRef: value[string](connection, "uuid"),
			Name:          id,
			NILOOwned:     strings.HasPrefix(id, niloTunnelPrefix),
			Profile:       publicProfileOf(settings),
		}
		if entry, ok := active[observation.ConnectionRef]; ok && entry.state == activeStateActivated {
			observation.Active = true
			observation.Interface = entry.device
			observation.LastHandshake = latestHandshake(ctx, entry.device)
		}
		tunnels = append(tunnels, observation)
	}
	sort.Slice(tunnels, func(i, j int) bool { return tunnels[i].Name < tunnels[j].Name })
	return tunnels, nil
}

// publicProfileOf reads the safe half of a WireGuard connection back out of
// its settings. Secrets are absent from the settings NetworkManager returns.
func publicProfileOf(settings map[string]map[string]dbus.Variant) network.Profile {
	profile := network.Profile{Name: value[string](settings["connection"], "id")}
	for _, family := range []string{"ipv4", "ipv6"} {
		for _, entry := range value[[]map[string]dbus.Variant](settings[family], "address-data") {
			profile.Addresses = append(profile.Addresses, fmt.Sprintf("%s/%d", value[string](entry, "address"), value[uint32](entry, "prefix")))
		}
		profile.DNS = append(profile.DNS, value[[]string](settings[family], "dns-search")...)
	}
	for _, packed := range value[[]uint32](settings["ipv4"], "dns") {
		var raw [4]byte
		binary.NativeEndian.PutUint32(raw[:], packed)
		profile.DNS = append(profile.DNS, netip.AddrFrom4(raw).String())
	}
	for _, raw := range value[[][]byte](settings["ipv6"], "dns") {
		if len(raw) == 16 {
			profile.DNS = append(profile.DNS, netip.AddrFrom16([16]byte(raw)).String())
		}
	}
	wireguard := settings["wireguard"]
	profile.MTU = int(value[uint32](wireguard, "mtu"))
	profile.ListenPort = int(value[uint32](wireguard, "listen-port"))
	for _, entry := range value[[]map[string]dbus.Variant](wireguard, "peers") {
		_, preshared := entry["preshared-key-flags"]
		profile.Peers = append(profile.Peers, network.ProfilePeer{
			PublicKey:           value[string](entry, "public-key"),
			AllowedIPs:          value[[]string](entry, "allowed-ips"),
			Endpoint:            value[string](entry, "endpoint"),
			PersistentKeepalive: int(value[uint32](entry, "persistent-keepalive")),
			HasPresharedKey:     preshared,
		})
	}
	return profile
}

// latestHandshake reads the newest peer handshake of an active tunnel with the
// read-only `wg show` command. A missing tool or an unparsable answer is
// simply no observation.
func latestHandshake(ctx context.Context, device string) *time.Time {
	if device == "" || strings.ContainsAny(device, "/ \t\n") {
		return nil
	}
	tool, err := installedWG(ctx)
	if err != nil || !tool.Installed {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/wg", "show", device, "latest-handshakes")
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = 250 * time.Millisecond
	output := &probeOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil
	}
	var newest int64
	for _, line := range strings.Split(strings.TrimSpace(string(output.Bytes())), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		stamp, err := strconv.ParseInt(fields[1], 10, 64)
		if err == nil && stamp > newest {
			newest = stamp
		}
	}
	if newest == 0 {
		return nil
	}
	at := time.Unix(newest, 0).UTC()
	return &at
}

// ActivateTunnel brings a saved connection up and waits for it to be activated.
func (n *NetworkManager) ActivateTunnel(ctx context.Context, connectionUUID string) error {
	path, err := n.connectionPath(ctx, connectionUUID)
	if err != nil {
		return err
	}
	var activePath dbus.ObjectPath
	if err := n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".ActivateConnection", 0, path, dbus.ObjectPath("/"), dbus.ObjectPath("/")).Store(&activePath); err != nil {
		return sanitizedBusError("activate the tunnel", err)
	}
	return n.waitTunnel(ctx, connectionUUID, true)
}

// DeactivateTunnel brings an active connection down and waits for it to go.
func (n *NetworkManager) DeactivateTunnel(ctx context.Context, connectionUUID string) error {
	active, err := n.activeTunnels(ctx)
	if err != nil {
		return err
	}
	entry, ok := active[connectionUUID]
	if !ok {
		return nil
	}
	if err := n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".DeactivateConnection", 0, entry.path).Err; err != nil {
		var busErr dbus.Error
		if !errors.As(err, &busErr) || !strings.HasSuffix(busErr.Name, ".UnknownConnection") {
			return sanitizedBusError("deactivate the tunnel", err)
		}
	}
	return n.waitTunnel(ctx, connectionUUID, false)
}

// waitTunnel polls until the connection is activated, or gone, as asked.
func (n *NetworkManager) waitTunnel(ctx context.Context, connectionUUID string, up bool) error {
	ctx, cancel := context.WithTimeout(ctx, tunnelSettle)
	defer cancel()
	timer := time.NewTicker(200 * time.Millisecond)
	defer timer.Stop()
	for {
		active, err := n.activeTunnels(ctx)
		if err == nil {
			entry, present := active[connectionUUID]
			switch {
			case up && present && entry.state == activeStateActivated:
				return nil
			case up && present && entry.state == activeStateDeactivated:
				return errors.New("the tunnel failed to activate")
			case !up && (!present || entry.state == activeStateDeactivated):
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("tunnel state wait: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// restoreTunnel puts a tunnel change back: the checkpoint rolls every device
// back where it still exists, the previously active tunnel is explicitly
// re-established either way, and success is only the observed system digest
// matching the snapshot taken before the change.
func (n *NetworkManager) restoreTunnel(ctx context.Context, r Record) error {
	props, err := n.properties(ctx, nmPath, nm)
	if err != nil {
		return err
	}
	if checkpointExists(props, r.Checkpoint) {
		var results map[string]uint32
		if err := n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".CheckpointRollback", 0, dbus.ObjectPath(r.Checkpoint)).Store(&results); err != nil {
			return sanitizedBusError("checkpoint rollback", err)
		}
	}
	// The checkpoint covers what NetworkManager tracked. The tunnel that was up
	// before is re-established explicitly, which also covers a daemon restart
	// or a reboot in which the checkpoint no longer exists.
	tunnels, err := n.Tunnels(ctx)
	if err != nil {
		return err
	}
	current, hadActive := activeOwnedTunnel(tunnels)
	if hadActive && current.ConnectionRef != r.System.ActiveTunnel {
		if err := n.DeactivateTunnel(ctx, current.ConnectionRef); err != nil {
			return err
		}
	}
	if r.System.ActiveTunnel != "" && (!hadActive || current.ConnectionRef != r.System.ActiveTunnel) {
		if err := n.ActivateTunnel(ctx, r.System.ActiveTunnel); err != nil {
			return err
		}
	}
	var observed SystemSnapshot
	err = n.waitSystem(ctx, func(snapshot SystemSnapshot) bool { observed = snapshot; return snapshot == *r.System })
	if err != nil {
		return fmt.Errorf("restore verification: %w; expected tunnel=%q digest=%s; observed tunnel=%q digest=%s",
			err, r.System.ActiveTunnel, r.System.Digest, observed.ActiveTunnel, observed.Digest)
	}
	props, err = n.properties(ctx, nmPath, nm)
	if err != nil {
		return err
	}
	if checkpointExists(props, r.Checkpoint) {
		return n.Destroy(ctx, r.Checkpoint)
	}
	return nil
}

func checkpointExists(props map[string]dbus.Variant, checkpoint string) bool {
	for _, candidate := range value[[]dbus.ObjectPath](props, "Checkpoints") {
		if string(candidate) == checkpoint {
			return true
		}
	}
	return false
}

// waitSystem polls the system snapshot until check accepts it.
func (n *NetworkManager) waitSystem(ctx context.Context, check func(SystemSnapshot) bool) error {
	timer := time.NewTicker(300 * time.Millisecond)
	defer timer.Stop()
	for {
		devices, err := n.Devices(ctx)
		if err == nil {
			tunnels, tunnelErr := n.Tunnels(ctx)
			if tunnelErr == nil && check(systemSnapshot(devices, tunnels)) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

var _ TunnelDriver = (*NetworkManager)(nil)
