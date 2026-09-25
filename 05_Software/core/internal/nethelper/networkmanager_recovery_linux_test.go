//go:build linux

package nethelper

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"nilo.local/nilo/internal/network"
)

func TestMissingVolatileRecoveryProfileIsAlreadyRestored(t *testing.T) {
	missing := dbus.Error{Name: "org.freedesktop.NetworkManager.Settings.UnknownConnection"}
	if !recoveryProfileMissing(missing) {
		t.Fatal("NetworkManager UnknownConnection was not treated as an already-restored volatile profile")
	}
	if recoveryProfileMissing(errors.New("D-Bus unavailable")) {
		t.Fatal("an unrelated restoration error was ignored")
	}
}

func TestCommissioningReconcileNeverDeletesAForeignSamePrefixProfile(t *testing.T) {
	if os.Getenv("NILO_NETWORK_SYSTEM") != "1" {
		t.Skip("requires isolated NetworkManager test container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	stateDirectory := t.TempDir()
	if err := os.Chmod(stateDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.ConfigureCommissioningProfileStore(store); err != nil {
		t.Fatal(err)
	}
	name := commissioningConnectionPrefix + "foreign-production"
	command := exec.CommandContext(ctx, "nmcli", "connection", "add", "type", "ethernet", "ifname", "eth9",
		"con-name", name, "ipv4.method", "manual", "ipv4.addresses", "198.51.100.10/24", "connection.autoconnect", "no")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create foreign same-prefix profile: %v\n%s", err, output)
	}
	path, before := commissioningProfileByID(t, ctx, driver, name)
	if err := driver.ReconcileCommissioningLinks(ctx, nil); err != nil {
		t.Fatal(err)
	}
	after, err := driver.settingsOf(ctx, path)
	if err != nil {
		t.Fatalf("foreign same-prefix profile was removed: %v", err)
	}
	if !reflect.DeepEqual(normalizeDBus(before), normalizeDBus(after)) {
		t.Fatalf("foreign same-prefix profile changed:\nbefore=%v\nafter=%v", before, after)
	}
}

func TestCommissioningReconcileCreatesAndRemovesAnInventoriedVolatileProfile(t *testing.T) {
	if os.Getenv("NILO_NETWORK_SYSTEM") != "1" {
		t.Skip("requires isolated NetworkManager test container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	stateDirectory := t.TempDir()
	if err := os.Chmod(stateDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.ConfigureCommissioningProfileStore(store); err != nil {
		t.Fatal(err)
	}
	const interfaceName = "nilo-c-test"
	if output, err := exec.CommandContext(ctx, "ip", "link", "add", interfaceName, "type", "veth", "peer", "name", "nilo-c-peer").CombinedOutput(); err != nil {
		t.Fatalf("create disposable commissioning veth: %v\n%s", err, output)
	}
	defer func() {
		if output, err := exec.Command("ip", "link", "delete", interfaceName).CombinedOutput(); err != nil {
			t.Errorf("remove disposable commissioning veth: %v\n%s", err, output)
		}
	}()
	if output, err := exec.CommandContext(ctx, "ip", "link", "set", interfaceName, "up").CombinedOutput(); err != nil {
		t.Fatalf("raise disposable commissioning veth: %v\n%s", err, output)
	}
	var target Device
	for attempt := 0; attempt < 50; attempt++ {
		devices, devicesErr := driver.Devices(ctx)
		if devicesErr != nil {
			t.Fatal(devicesErr)
		}
		for _, device := range devices {
			if device.Interface.Name == interfaceName {
				target = device
			}
		}
		if target.Connection == "" && len(target.Available) == 0 && len(target.Interface.IPv4) == 0 &&
			!hasConfiguredAddress(target.Interface.IPv6) && target.Interface.Gateway4 == "" && target.Interface.Gateway6 == "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if target.Interface.AdapterID == "" || target.Connection != "" || len(target.Available) != 0 || len(target.Interface.IPv4) != 0 ||
		hasConfiguredAddress(target.Interface.IPv6) || target.Interface.Gateway4 != "" || target.Interface.Gateway6 != "" {
		t.Fatalf("fixture Ethernet did not become safe: %+v", target)
	}
	target.Interface.Physical = true // the disposable veth stands in for owned physical hardware in this D-Bus effect test
	target.Interface.Controllable = true
	link := network.CommissioningLink{AdapterID: target.Interface.AdapterID, Interface: interfaceName}
	if err := driver.reconcileCommissioningLinks(ctx, []network.CommissioningLink{link}, []Device{target}); err != nil {
		t.Fatal(err)
	}
	ownership, err := store.LoadCommissioningProfiles()
	if err != nil || len(ownership.Profiles) != 1 {
		t.Fatalf("ownership=%+v err=%v", ownership, err)
	}
	profile := ownership.Profiles[0]
	connection, settings := commissioningProfileByUUID(t, ctx, driver, profile.UUID)
	properties, err := driver.properties(ctx, connection, connectionAPI)
	if err != nil || !value[bool](properties, "Unsaved") || !commissioningProfileMatches(settings, profile) {
		t.Fatalf("owned profile is not exact and volatile: properties=%v settings=%v err=%v", properties, settings, err)
	}
	if err := driver.reconcileCommissioningLinks(ctx, nil, []Device{target}); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.settingsOf(ctx, connection); err == nil {
		t.Fatal("owned commissioning profile remained after withdrawal")
	}
	ownership, err = store.LoadCommissioningProfiles()
	if err != nil || len(ownership.Profiles) != 0 {
		t.Fatalf("stale ownership remained: %+v err=%v", ownership, err)
	}
}

func commissioningProfileByID(t *testing.T, ctx context.Context, driver *NetworkManager, id string) (dbus.ObjectPath, map[string]map[string]dbus.Variant) {
	t.Helper()
	var paths []dbus.ObjectPath
	if err := driver.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.ListConnections", 0).Store(&paths); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		settings, err := driver.settingsOf(ctx, path)
		if err == nil && value[string](settings["connection"], "id") == id {
			return path, settings
		}
	}
	t.Fatalf("connection %q not found", id)
	return "", nil
}

func commissioningProfileByUUID(t *testing.T, ctx context.Context, driver *NetworkManager, wanted string) (dbus.ObjectPath, map[string]map[string]dbus.Variant) {
	t.Helper()
	var paths []dbus.ObjectPath
	if err := driver.bus.Object(nm, settingsPath).CallWithContext(ctx, nm+".Settings.ListConnections", 0).Store(&paths); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		settings, err := driver.settingsOf(ctx, path)
		if err == nil && value[string](settings["connection"], "uuid") == wanted {
			return path, settings
		}
	}
	t.Fatalf("connection UUID %q not found", wanted)
	return "", nil
}

func TestRecoveryConnectionIsVolatileLinkLocalAndNeverRoutesOrResolves(t *testing.T) {
	t.Parallel()

	device := Device{Interface: network.Interface{
		Name: "eth1", AdapterID: "aa:bb:cc:dd:ee:ff", Kind: network.KindEthernet,
		Physical: true, State: network.StateConnected, Controllable: true,
	}}
	settings, err := recoveryConnectionSettings("02f8e84a-4fe8-4915-b386-4aac543f85cf", device)
	if err != nil {
		t.Fatal(err)
	}
	connection := settings["connection"]
	if value[bool](connection, "autoconnect") || value[string](connection, "interface-name") != "eth1" {
		t.Fatalf("connection settings = %v", connection)
	}
	ipv4 := settings["ipv4"]
	if value[string](ipv4, "method") != "link-local" || !value[bool](ipv4, "never-default") ||
		!value[bool](ipv4, "ignore-auto-routes") || !value[bool](ipv4, "ignore-auto-dns") {
		t.Fatalf("IPv4 recovery settings = %v", ipv4)
	}
	if value[string](settings["ipv6"], "method") != "disabled" {
		t.Fatalf("IPv6 recovery settings = %v", settings["ipv6"])
	}
}

func TestRecoveryProfileOwnershipRequiresExactSettingsAndVolatileState(t *testing.T) {
	record := DirectRecoveryRecord{
		Version: 1, ID: "02f8e84a-4fe8-4915-b386-4aac543f85cf", Phase: "active",
		AdapterID: "aa:bb:cc:dd:ee:ff", InterfaceName: "eth1", Deadline: time.Now().Add(time.Minute),
		UpdatedAt: time.Now(),
	}
	device := Device{Interface: network.Interface{Name: record.InterfaceName, AdapterID: record.AdapterID}}
	settings := func() map[string]map[string]dbus.Variant {
		result, err := recoveryConnectionSettings(record.ID, device)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if !recoveryProfileMatches(settings(), record, true) {
		t.Fatal("exact volatile recovery profile was not recognized")
	}
	normalized := settings()
	normalized["802-3-ethernet"]["auto-negotiate"] = variant(false)
	normalized["802-3-ethernet"]["s390-options"] = variant(map[string]string{})
	normalized["connection"]["permissions"] = variant([]string{})
	for _, section := range []string{"ipv4", "ipv6"} {
		normalized[section]["address-data"] = variant([]map[string]any{})
		normalized[section]["addresses"] = variant([][]uint32{})
		normalized[section]["route-data"] = variant([]map[string]any{})
		normalized[section]["routes"] = variant([][]uint32{})
	}
	normalized["proxy"] = map[string]dbus.Variant{}
	if !recoveryProfileMatches(normalized, record, true) {
		t.Fatal("NetworkManager's allowlisted empty runtime defaults were not normalized")
	}
	for name, mutate := range map[string]func(map[string]map[string]dbus.Variant){
		"persistent": func(map[string]map[string]dbus.Variant) {},
		"identity": func(value map[string]map[string]dbus.Variant) {
			value["802-3-ethernet"]["mac-address"] = variant([]byte{0, 1, 2, 3, 4, 5})
		},
		"interface": func(value map[string]map[string]dbus.Variant) {
			value["connection"]["interface-name"] = variant("eth9")
		},
		"autoconnect": func(value map[string]map[string]dbus.Variant) { value["connection"]["autoconnect"] = variant(true) },
		"routing":     func(value map[string]map[string]dbus.Variant) { value["ipv4"]["never-default"] = variant(false) },
		"may fail":    func(value map[string]map[string]dbus.Variant) { value["ipv4"]["may-fail"] = variant(true) },
		"permissions": func(value map[string]map[string]dbus.Variant) {
			value["connection"]["permissions"] = variant([]string{"user:test:"})
		},
		"negotiation": func(value map[string]map[string]dbus.Variant) {
			value["802-3-ethernet"]["auto-negotiate"] = variant(true)
		},
		"unexpected connection property": func(value map[string]map[string]dbus.Variant) {
			value["connection"]["zone"] = variant("trusted")
		},
		"unexpected security section": func(value map[string]map[string]dbus.Variant) {
			value["802-1x"] = map[string]dbus.Variant{"identity": variant("foreign")}
		},
		"gateway": func(value map[string]map[string]dbus.Variant) { value["ipv4"]["gateway"] = variant("192.0.2.1") },
		"address data": func(value map[string]map[string]dbus.Variant) {
			value["ipv4"]["address-data"] = variant([]map[string]any{{"address": "192.0.2.10", "prefix": uint32(24)}})
		},
		"dns": func(value map[string]map[string]dbus.Variant) { value["ipv4"]["dns"] = variant([]uint32{0x01010101}) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := settings()
			mutate(candidate)
			unsaved := name != "persistent"
			if recoveryProfileMatches(candidate, record, unsaved) {
				t.Fatal("modified or persistent profile was accepted as owned")
			}
		})
	}
}

func TestNetworkManagerProducesAnExactlyOwnedRestorableRecoveryProfile(t *testing.T) {
	if os.Getenv("NILO_NETWORK_SYSTEM") != "1" {
		t.Skip("requires isolated NetworkManager test container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	const interfaceName = "nilo-r-test"
	if output, err := exec.CommandContext(ctx, "ip", "link", "add", interfaceName, "type", "veth", "peer", "name", "nilo-r-peer").CombinedOutput(); err != nil {
		t.Fatalf("create disposable recovery veth: %v\n%s", err, output)
	}
	defer func() {
		if output, err := exec.Command("ip", "link", "delete", interfaceName).CombinedOutput(); err != nil {
			t.Errorf("remove disposable recovery veth: %v\n%s", err, output)
		}
	}()
	for _, link := range []string{interfaceName, "nilo-r-peer"} {
		if output, err := exec.CommandContext(ctx, "ip", "link", "set", link, "up").CombinedOutput(); err != nil {
			t.Fatalf("raise disposable recovery link %s: %v\n%s", link, err, output)
		}
	}
	var target Device
	for attempt := 0; attempt < 50; attempt++ {
		devices, devicesErr := driver.Devices(ctx)
		if devicesErr != nil {
			t.Fatal(devicesErr)
		}
		for _, device := range devices {
			if device.Interface.Name == interfaceName {
				target = device
			}
		}
		if target.Interface.LinkUp && target.Connection == "" && len(target.Available) == 0 &&
			len(target.Interface.IPv4) == 0 && !hasConfiguredAddress(target.Interface.IPv6) &&
			target.Interface.Gateway4 == "" && target.Interface.Gateway6 == "" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if target.Interface.AdapterID == "" || !target.Interface.LinkUp || target.Connection != "" ||
		len(target.Available) != 0 || len(target.Interface.IPv4) != 0 || hasConfiguredAddress(target.Interface.IPv6) ||
		target.Interface.Gateway4 != "" || target.Interface.Gateway6 != "" {
		t.Fatalf("fixture Ethernet did not become safe: %+v", target)
	}
	record := DirectRecoveryRecord{
		Version: 1, ID: "02f8e84a-4fe8-4915-b386-4aac543f85cf", Phase: "active",
		AdapterID: target.Interface.AdapterID, InterfaceName: interfaceName,
		Deadline: time.Now().Add(time.Minute), UpdatedAt: time.Now(),
	}
	// The disposable veth is virtual, so inject it only below the production
	// physical-device guard while exercising the real profile creation,
	// activation, normalization, ownership check, and restoration path.
	target.Interface.Physical = true
	target.Interface.Controllable = true
	wantedSettings, err := recoveryConnectionSettings(record.ID, target)
	if err != nil {
		t.Fatal(err)
	}
	var created dbus.ObjectPath
	var result map[string]dbus.Variant
	if err := driver.bus.Object(nm, settingsPath).CallWithContext(
		ctx, nm+".Settings.AddConnection2", 0, wantedSettings, uint32(addConnectionInMemory), map[string]dbus.Variant{},
	).Store(&created, &result); err != nil {
		t.Fatal(err)
	}
	if err := driver.activateOn(ctx, created, target, record.ID); err != nil {
		t.Fatal(err)
	}
	connection, settings := commissioningProfileByUUID(t, ctx, driver, record.ID)
	properties, err := driver.properties(ctx, connection, connectionAPI)
	if err != nil || !value[bool](properties, "Unsaved") || !recoveryProfileMatches(settings, record, true) {
		t.Fatalf("created recovery profile is not exactly owned: properties=%v settings=%v err=%v", properties, settings, err)
	}
	if err := driver.RestoreRecoveryLink(ctx, record); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.settingsOf(ctx, connection); err == nil {
		t.Fatal("owned recovery profile remained after exact restoration")
	}
}

func TestCommissioningConnectionAutoStartsButNeverRoutesOrResolves(t *testing.T) {
	t.Parallel()

	settings, err := commissioningConnectionSettings(
		"02f8e84a-4fe8-4915-b386-4aac543f85cf",
		network.CommissioningLink{AdapterID: "aa:bb:cc:dd:ee:ff", Interface: "eth1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !value[bool](settings["connection"], "autoconnect") {
		t.Fatal("commissioning profile will not arm a disconnected port")
	}
	ipv4 := settings["ipv4"]
	if value[string](ipv4, "method") != "link-local" || !value[bool](ipv4, "never-default") ||
		!value[bool](ipv4, "ignore-auto-routes") || !value[bool](ipv4, "ignore-auto-dns") {
		t.Fatalf("commissioning IPv4 settings = %v", ipv4)
	}
	if value[string](settings["ipv6"], "method") != "disabled" {
		t.Fatalf("commissioning IPv6 settings = %v", settings["ipv6"])
	}
}

func TestCommissioningDeviceKeepsOnlyItsOwnedLinkLocalProfile(t *testing.T) {
	link := network.CommissioningLink{AdapterID: "aa:bb:cc:dd:ee:ff", Interface: "eth1"}
	base := Device{Interface: network.Interface{
		Name: "eth1", AdapterID: link.AdapterID, Kind: network.KindEthernet,
		Physical: true, Controllable: true, State: network.StateConnected,
		IPv4: []string{"169.254.10.20/16"}, IPv6: []string{"fe80::1/64"},
	}}
	base.Connection = "owned"
	base.Available = []string{"owned"}
	if !commissioningDeviceEligible(base, link, "owned") {
		t.Fatal("the owned settled IPv4LL profile would be removed on the next reconcile")
	}
	foreign := base
	foreign.Connection = "production"
	foreign.Available = []string{"owned", "production"}
	if commissioningDeviceEligible(foreign, link, "owned") {
		t.Fatal("a foreign active/available profile was treated as safe for commissioning")
	}
	manualLinkLocal := base
	manualLinkLocal.Connection = ""
	manualLinkLocal.Available = nil
	if commissioningDeviceEligible(manualLinkLocal, link, "") {
		t.Fatal("a pre-existing IPv4LL address without NILO ownership was treated as safe")
	}
}
