//go:build linux

package nethelper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"nilo.local/nilo/internal/network"
)

const nm = "org.freedesktop.NetworkManager"
const nmPath = dbus.ObjectPath("/org/freedesktop/NetworkManager")
const deviceAPI = nm + ".Device"

// NetworkManager uses the system bus exclusively. No shell or nmcli participates
// in production operations, and saved connection settings are never rewritten.
type NetworkManager struct {
	bus                   *dbus.Conn
	mdnsGroups            map[string]dbus.ObjectPath
	commissioningStore    CommissioningProfileStore
	applianceSerial       string
	commissioningServices commissioningServiceDriver
}

func (n *NetworkManager) ConfigureCommissioningServices(services commissioningServiceDriver) error {
	if services == nil {
		return errors.New("commissioning services are required")
	}
	n.commissioningServices = services
	return nil
}

func (n *NetworkManager) ConfigureApplianceSerial(serialName string) error {
	if !validPermanentSerialName(serialName) {
		return errors.New("valid permanent appliance name is required")
	}
	n.applianceSerial = serialName
	return nil
}

func NewNetworkManager() (*NetworkManager, error) {
	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("system D-Bus unavailable: %w", err)
	}
	n := &NetworkManager{bus: bus, mdnsGroups: map[string]dbus.ObjectPath{}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = n.properties(ctx, nmPath, nm); err != nil {
		bus.Close()
		return nil, fmt.Errorf("NetworkManager unavailable: %w", err)
	}
	return n, nil
}
func (n *NetworkManager) Close() error { return n.bus.Close() }
func (n *NetworkManager) properties(ctx context.Context, path dbus.ObjectPath, iface string) (map[string]dbus.Variant, error) {
	var values map[string]dbus.Variant
	err := n.bus.Object(nm, path).CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", 0, iface).Store(&values)
	return values, err
}
func value[T any](m map[string]dbus.Variant, key string) T {
	var v T
	if raw, ok := m[key]; ok {
		_ = dbus.Store([]any{raw.Value()}, &v)
	}
	return v
}
func (n *NetworkManager) connection(ctx context.Context, path dbus.ObjectPath) (string, error) {
	if path == "" || path == "/" {
		return "", nil
	}
	var settings map[string]map[string]dbus.Variant
	err := n.bus.Object(nm, path).CallWithContext(ctx, nm+".Settings.Connection.GetSettings", 0).Store(&settings)
	if err != nil {
		return "", err
	}
	return value[string](settings["connection"], "uuid"), nil
}
func (n *NetworkManager) Devices(ctx context.Context) ([]Device, error) {
	var paths []dbus.ObjectPath
	if err := n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".GetDevices", 0).Store(&paths); err != nil {
		return nil, err
	}
	if len(paths) > 128 {
		return nil, errors.New("too many network devices")
	}
	out := make([]Device, 0, len(paths))
	for _, path := range paths {
		d, err := n.device(ctx, path)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Interface.ID < out[j].Interface.ID })
	return out, nil
}
func (n *NetworkManager) device(ctx context.Context, path dbus.ObjectPath) (Device, error) {
	props, err := n.properties(ctx, path, deviceAPI)
	if err != nil {
		return Device{}, err
	}
	name := value[string](props, "Interface")
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return Device{}, errors.New("invalid interface name from NetworkManager")
	}
	d := Device{Path: string(path), AutoConnect: value[bool](props, "Autoconnect"), Available: []string{}}
	d.Interface = network.Interface{ID: "nm-" + name, Name: name, Kind: network.KindOther, State: network.StateUnavailable, IPv4: []string{}, IPv6: []string{}, NotControllableReason: "unsupported infrastructure interface"}
	kind := value[uint32](props, "DeviceType")
	switch kind {
	case 1, 20: // Physical and virtual Ethernet use the same wired API.
		d.Interface.Kind = network.KindEthernet
	case 2:
		d.Interface.Kind = network.KindWiFi
	case 29:
		d.Tunnel = true
		d.Interface.Kind = network.KindWireGuard
		d.Interface.NotControllableReason = "a WireGuard tunnel device is managed through its profile"
	case 32:
		d.Interface.Kind = network.KindLoopback
	}
	if d.Interface.Kind == network.KindEthernet || d.Interface.Kind == network.KindWiFi {
		d.Interface.Controllable = value[bool](props, "Managed")
		d.Interface.NotControllableReason = "not managed by NetworkManager"
		if d.Interface.Controllable {
			d.Interface.NotControllableReason = ""
		}
	}
	state := value[uint32](props, "State")
	d.Transitioning = state != 20 && state != 30 && state != 100
	if state == 100 {
		d.Interface.State = network.StateConnected
	} else if state == 30 || state == 20 {
		d.Interface.State = network.StateDisconnected
	}
	specific := deviceAPI + ".Wired"
	speedKey := "Speed"
	if kind == 2 {
		specific = deviceAPI + ".Wireless"
		speedKey = "Bitrate"
	}
	if more, e := n.properties(ctx, path, specific); e == nil {
		permanent := value[string](more, "PermHwAddress")
		if permanent != "" {
			d.Interface.ID += "-" + strings.ToLower(permanent)
			d.Interface.AdapterID = strings.ToLower(permanent)
		} else if address := value[string](more, "HwAddress"); address != "" {
			// A virtual adapter has no permanent address; its current one is
			// the only identity it has, and is stable for its lifetime.
			d.Interface.AdapterID = strings.ToLower(address)
		}
		// A container veth answers the same wired API as a built-in adapter, so
		// the permanent address is paired with the kernel's bus device before
		// this reading may be treated as a physical adapter.
		d.Interface.Physical = physicalAdapter(permanent, busDevicePresent(name))
		d.Interface.LinkCapacityMbps = uint64(value[uint32](more, speedKey))
		if kind == 1 || kind == 20 {
			d.Interface.LinkUp = value[bool](more, "Carrier")
		}
		if kind == 2 {
			d.Interface.LinkCapacityMbps /= 1000
			if point := value[dbus.ObjectPath](more, "ActiveAccessPoint"); point != "" && point != "/" {
				if ap, e := n.properties(ctx, point, nm+".AccessPoint"); e == nil {
					d.Interface.SSID = string(value[[]byte](ap, "Ssid"))
				}
			}
		}
	}
	active := value[dbus.ObjectPath](props, "ActiveConnection")
	if active != "" && active != "/" {
		values, e := n.properties(ctx, active, nm+".Connection.Active")
		if e != nil {
			return Device{}, e
		}
		d.Connection = value[string](values, "Uuid")
		// ACTIVATED can precede completion of optional IP families. A recovery
		// snapshot must not capture their temporary empty address/route state.
		flags := value[uint32](values, "StateFlags")
		d.Transitioning = state != 100 || flags&0x18 != 0x18
	}
	for _, candidate := range value[[]dbus.ObjectPath](props, "AvailableConnections") {
		id, e := n.connection(ctx, candidate)
		if e != nil {
			return Device{}, e
		}
		if id != "" {
			d.Available = append(d.Available, id)
		}
	}
	sort.Strings(d.Available)
	d.Interface.SavedConnections = append([]string(nil), d.Available...)
	snapshots := map[string]any{}
	for _, family := range []string{"4", "6"} {
		ipPath := value[dbus.ObjectPath](props, "Ip"+family+"Config")
		if ipPath == "" || ipPath == "/" {
			continue
		}
		ip, e := n.properties(ctx, ipPath, nm+".IP"+family+"Config")
		if e != nil {
			return Device{}, e
		}
		safe := map[string]any{}
		for _, field := range []string{"AddressData", "RouteData", "Gateway", "NameserverData", "Nameservers", "Domains", "Searches", "DnsOptions", "DnsPriority"} {
			if v, ok := ip[field]; ok {
				safe[field] = v.Value()
			}
		}
		snapshots[family] = safe
		if family == "4" {
			d.Interface.Gateway4 = value[string](ip, "Gateway")
		} else {
			d.Interface.Gateway6 = value[string](ip, "Gateway")
		}
		for _, address := range value[[]map[string]dbus.Variant](ip, "AddressData") {
			text := fmt.Sprintf("%s/%d", value[string](address, "address"), value[uint32](address, "prefix"))
			if family == "4" {
				d.Interface.IPv4 = append(d.Interface.IPv4, text)
			} else {
				d.Interface.IPv6 = append(d.Interface.IPv6, text)
			}
		}
	}
	raw, err := json.Marshal(normalizeDBus(snapshots))
	if err != nil {
		return Device{}, err
	}
	digest := sha256.Sum256(raw)
	d.NetworkState = hex.EncodeToString(digest[:])
	d.Counters, err = readInterfaceCounters(filepath.Join("/sys/class/net", name), d.Interface.ID)
	if err != nil {
		return Device{}, err
	}
	return d, nil
}

func readInterfaceCounters(interfacePath, interfaceID string) (network.CounterReading, error) {
	counters := network.CounterReading{InterfaceID: interfaceID}
	fields := []struct {
		filename    string
		destination *uint64
	}{
		{"rx_bytes", &counters.RXBytes},
		{"tx_bytes", &counters.TXBytes},
		{"rx_errors", &counters.RXErrors},
		{"tx_errors", &counters.TXErrors},
		{"rx_dropped", &counters.RXDropped},
		{"tx_dropped", &counters.TXDropped},
	}
	for _, field := range fields {
		raw, err := os.ReadFile(filepath.Join(interfacePath, "statistics", field.filename))
		if errors.Is(err, os.ErrNotExist) {
			// NetworkManager can report pseudo-devices such as Wi-Fi Direct and
			// an adapter can disappear while its inventory is being sampled.
			// Neither case has kernel counters and neither should take down the
			// appliance service.
			return network.CounterReading{InterfaceID: interfaceID}, nil
		}
		if err != nil {
			return network.CounterReading{}, fmt.Errorf("read interface counters: %w", err)
		}
		*field.destination, err = strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			return network.CounterReading{}, fmt.Errorf("parse interface counter %s: %w", field.filename, err)
		}
	}
	return counters, nil
}

func (n *NetworkManager) find(ctx context.Context, id string) (Device, error) {
	items, err := n.Devices(ctx)
	if err != nil {
		return Device{}, err
	}
	for _, d := range items {
		if d.Interface.ID == id && d.Interface.Controllable {
			return d, nil
		}
	}
	return Device{}, network.ErrNotControllable
}
func (n *NetworkManager) Checkpoint(ctx context.Context, d Device, window time.Duration) (string, error) {
	var checkpoint dbus.ObjectPath
	if d.Interface.ID == "" {
		// A tunnel change is checkpointed against every device, because a full
		// tunnel changes routes and DNS everywhere. Devices the change creates
		// are disconnected on rollback rather than left up.
		err := n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".CheckpointCreate", 0, []dbus.ObjectPath{}, uint32(window/time.Second), uint32(checkpointDisconnectNewDevices)).Store(&checkpoint)
		return string(checkpoint), err
	}
	current, err := n.find(ctx, d.Interface.ID)
	if err != nil {
		return "", err
	}
	if current.Transitioning || !sameState(current, d) {
		return "", network.ErrPreviewStale
	}
	err = n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".CheckpointCreate", 0, []dbus.ObjectPath{dbus.ObjectPath(current.Path)}, uint32(window/time.Second), uint32(0)).Store(&checkpoint)
	return string(checkpoint), err
}
func (n *NetworkManager) setAuto(ctx context.Context, d Device, enabled bool) error {
	return n.bus.Object(nm, dbus.ObjectPath(d.Path)).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Set", 0, deviceAPI, "Autoconnect", dbus.MakeVariant(enabled)).Err
}
func (n *NetworkManager) activate(ctx context.Context, d Device, uuid string) error {
	var path dbus.ObjectPath
	if err := n.bus.Object(nm, dbus.ObjectPath("/org/freedesktop/NetworkManager/Settings")).CallWithContext(ctx, nm+".Settings.GetConnectionByUuid", 0, uuid).Store(&path); err != nil {
		return err
	}
	return n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".ActivateConnection", 0, path, dbus.ObjectPath(d.Path), dbus.ObjectPath("/")).Err
}
func (n *NetworkManager) Apply(ctx context.Context, c network.Change, before Device) error {
	d, err := n.find(ctx, c.Target)
	if err != nil {
		return err
	}
	if !sameState(d, before) {
		return network.ErrPreviewStale
	}
	switch c.Action {
	case network.ActionInterfaceDisable:
		// Disconnect itself inhibits automatic reconnection until explicit
		// activation. A separate Autoconnect=false setting survives NM's
		// checkpoint rollback and would leave the recovered device inhibited.
		if d.Connection != "" {
			if err = n.bus.Object(nm, dbus.ObjectPath(d.Path)).CallWithContext(ctx, deviceAPI+".Disconnect", 0).Err; err != nil {
				return err
			}
		}
		return n.wait(ctx, d.Interface.ID, func(now Device) bool { return now.Connection == "" && now.Interface.State == network.StateDisconnected })
	case network.ActionInterfaceEnable:
		if err = n.activate(ctx, d, c.ConnectionID); err != nil {
			return err
		}
		return n.wait(ctx, d.Interface.ID, func(now Device) bool {
			return now.Connection == c.ConnectionID && now.Interface.State == network.StateConnected && !now.Transitioning
		})
	default:
		return network.ErrUnsupported
	}
}
func (n *NetworkManager) Destroy(ctx context.Context, checkpoint string) error {
	if !strings.HasPrefix(checkpoint, string(nmPath)+"/Checkpoint/") {
		return network.ErrRecoveryUnavailable
	}
	return n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".CheckpointDestroy", 0, dbus.ObjectPath(checkpoint)).Err
}
func sameState(a, b Device) bool {
	return !a.Transitioning && !b.Transitioning && a.Connection == b.Connection && a.AutoConnect == b.AutoConnect && a.Interface.State == b.Interface.State && a.NetworkState == b.NetworkState
}
func (n *NetworkManager) wait(ctx context.Context, id string, check func(Device) bool) error {
	timer := time.NewTicker(200 * time.Millisecond)
	defer timer.Stop()
	// A read can fail while NetworkManager is mid-transition - an active
	// connection object vanishes between the listing and its properties. That
	// is a moment, not an outcome, so the wait keeps observing until the
	// deadline and reports the last failure only then.
	var lastErr error
	for {
		d, err := n.find(ctx, id)
		if err == nil {
			if check(d) {
				return nil
			}
		} else if errors.Is(err, network.ErrNotControllable) {
			return err
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("%w (last observation failed: %v)", ctx.Err(), lastErr)
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
func (n *NetworkManager) Restore(ctx context.Context, r Record) error {
	if r.System != nil {
		return n.restoreTunnel(ctx, r)
	}
	props, err := n.properties(ctx, nmPath, nm)
	if err != nil {
		return err
	}
	exists := false
	for _, cp := range value[[]dbus.ObjectPath](props, "Checkpoints") {
		if string(cp) == r.Checkpoint {
			exists = true
		}
	}
	if exists {
		var results map[string]uint32
		if err = n.bus.Object(nm, nmPath).CallWithContext(ctx, nm+".CheckpointRollback", 0, dbus.ObjectPath(r.Checkpoint)).Store(&results); err != nil {
			return err
		}
		for _, code := range results {
			if code != 0 {
				return errors.New("NetworkManager refused checkpoint restoration")
			}
		}
	}
	d, err := n.find(ctx, r.Before.Interface.ID)
	if err != nil {
		return err
	}
	// After reboot the D-Bus checkpoint no longer exists. Restore the saved UUID
	// using the OS-owned profile, then verify the captured runtime network state.
	if d.Connection != r.Before.Connection {
		if r.Before.Connection == "" {
			if err = n.bus.Object(nm, dbus.ObjectPath(d.Path)).CallWithContext(ctx, deviceAPI+".Disconnect", 0).Err; err != nil {
				return err
			}
		} else if err = n.activate(ctx, d, r.Before.Connection); err != nil {
			return err
		}
	}
	if err = n.setAuto(ctx, d, r.Before.AutoConnect); err != nil {
		return err
	}
	var observed Device
	if err = n.wait(ctx, d.Interface.ID, func(now Device) bool { observed = now; return sameState(now, r.Before) }); err != nil {
		return fmt.Errorf("restore verification: %w; expected connection=%s auto=%t IPv4=%v IPv6=%v digest=%s; observed connection=%s auto=%t IPv4=%v IPv6=%v digest=%s", err, r.Before.Connection, r.Before.AutoConnect, r.Before.Interface.IPv4, r.Before.Interface.IPv6, r.Before.NetworkState, observed.Connection, observed.AutoConnect, observed.Interface.IPv4, observed.Interface.IPv6, observed.NetworkState)
	}
	// A successful explicit rollback may leave the checkpoint until its timer;
	// destroy only this operation's checkpoint, never all checkpoints.
	props, err = n.properties(ctx, nmPath, nm)
	if err != nil {
		return err
	}
	for _, cp := range value[[]dbus.ObjectPath](props, "Checkpoints") {
		if string(cp) == r.Checkpoint {
			return n.Destroy(ctx, r.Checkpoint)
		}
	}
	return nil
}

// D-Bus variants contain unexported fields. Unwrap them before JSON hashing;
// otherwise different routes, addresses or DNS servers could all hash as {}.
func normalizeDBus(raw any) any {
	if v, ok := raw.(dbus.Variant); ok {
		return normalizeDBus(v.Value())
	}
	v := reflect.ValueOf(raw)
	if !v.IsValid() {
		return nil
	}
	switch v.Kind() {
	case reflect.Map:
		out := map[string]any{}
		for _, key := range v.MapKeys() {
			out[key.String()] = normalizeDBus(v.MapIndex(key).Interface())
		}
		return out
	case reflect.Array, reflect.Slice:
		out := make([]any, v.Len())
		for i := range out {
			out[i] = normalizeDBus(v.Index(i).Interface())
		}
		return out
	default:
		return raw
	}
}
