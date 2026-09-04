//go:build linux

package nethelper

import (
	"context"
	"encoding/json"
	"github.com/godbus/dbus/v5"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

func TestInterfaceCountersTolerateDevicesWithoutSysfsStatistics(t *testing.T) {
	counters, err := readInterfaceCounters(filepath.Join(t.TempDir(), "missing-device"), "nm-p2p-dev-wlan0")
	if err != nil {
		t.Fatalf("missing sysfs statistics should not fail the device inventory: %v", err)
	}
	if counters.InterfaceID != "nm-p2p-dev-wlan0" || counters.RXBytes != 0 || counters.TXBytes != 0 {
		t.Fatalf("counters = %+v, want named zero reading", counters)
	}
}

// This test is opt-in and must run inside the disposable network test container.
// The harness owns its entire namespace; no Windows/WSL device is targeted.
func TestNetworkManagerSystemCheckpointAndRestart(t *testing.T) {
	if os.Getenv("NILO_NETWORK_SYSTEM") != "1" {
		t.Skip("requires isolated NetworkManager test container")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal("system test requires disposable container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	items, err := driver.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var target Device
	for _, item := range items {
		t.Logf("device=%s kind=%s controllable=%v", item.Interface.Name, item.Interface.Kind, item.Interface.Controllable)
		if item.Interface.Name == "eth0" {
			target = item
		}
	}
	if target.Interface.ID == "" || !target.Interface.Controllable || target.Connection == "" {
		t.Fatalf("test connection not ready: %+v", target)
	}
	if len(target.Interface.IPv4) == 0 || len(target.Interface.IPv6) == 0 {
		t.Fatal("missing live addresses")
	}
	if target.Interface.Gateway4 != "192.0.2.1" || target.Interface.Gateway6 != "fd00:10::1" {
		t.Fatalf("unexpected live gateways: %+v", target.Interface)
	}
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	record, err := engine.Prepare(ctx, "system-disable", network.Change{Action: network.ActionInterfaceDisable, Target: target.Interface.ID}, network.ConfirmationWindow)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		recovery, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = engine.Restore(recovery, record.ID)
	}()
	if err = engine.Apply(ctx, record.ID); err != nil {
		t.Fatal(err)
	}
	down, err := driver.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range down {
		if d.Interface.ID == target.Interface.ID && (d.Interface.State == network.StateConnected) {
			t.Fatal("device remained connected")
		}
	}
	// A fresh engine must reconcile the saved record before taking a new change.
	restarted, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	restored, _ := restarted.Recovery()
	if restored.Phase != "restored" {
		t.Fatal(restored.Phase)
	}
	// Lose the NM checkpoint as on a daemon restart/reboot; durable UUID and
	// observations must suffice, and success still requires live verification.
	record, err = restarted.Prepare(ctx, "lost-checkpoint", network.Change{Action: network.ActionInterfaceDisable, Target: target.Interface.ID}, network.ConfirmationWindow)
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.Apply(ctx, record.ID); err != nil {
		t.Fatal(err)
	}
	if err = driver.Destroy(ctx, record.Checkpoint); err != nil {
		t.Fatal(err)
	}
	if err = restarted.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	// No helper timer or Bench Core is running here. NM alone must restore
	// this change after the real two-minute timeout.
	record, err = restarted.Prepare(ctx, "nm-timeout", network.Change{Action: network.ActionInterfaceDisable, Target: target.Interface.ID}, network.ConfirmationWindow)
	if err != nil {
		t.Fatal(err)
	}
	if err = restarted.Apply(ctx, record.ID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		recovery, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = restarted.Restore(recovery, record.ID)
	}()
	if err = driver.wait(ctx, target.Interface.ID, func(d Device) bool { return sameState(d, target) }); err != nil {
		t.Fatal(err)
	}
	if time.Until(record.Deadline) > 2*time.Second {
		t.Fatal("restored before the independent timeout")
	}
	if err = restarted.Expire(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkManagerCheckpointRestoresAutoconnect(t *testing.T) {
	if os.Getenv("NILO_NETWORK_SYSTEM") != "1" {
		t.Skip("container only")
	}
	if _, e := os.Stat("/.dockerenv"); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	n, e := NewNetworkManager()
	if e != nil {
		t.Fatal(e)
	}
	defer n.Close()
	items, e := n.Devices(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var before Device
	for _, d := range items {
		if d.Interface.Name == "eth0" {
			before = d
		}
	}
	cp, e := n.Checkpoint(ctx, before, 5*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if e = n.Apply(ctx, network.Change{Action: network.ActionInterfaceDisable, Target: before.Interface.ID}, before); e != nil {
		t.Fatal(e)
	}
	var after Device
	e = n.wait(ctx, before.Interface.ID, func(d Device) bool { after = d; return sameState(d, before) })
	if e != nil {
		t.Fatalf("%v checkpoint=%s before=%+v after=%+v", e, cp, before, after)
	}
}

func TestNetworkSnapshotIncludesNestedDBusValues(t *testing.T) {
	raw := map[string]any{"AddressData": []map[string]dbus.Variant{{"address": dbus.MakeVariant("192.0.2.2"), "prefix": dbus.MakeVariant(uint32(24))}}, "RouteData": []map[string]dbus.Variant{{"dest": dbus.MakeVariant("198.51.100.0"), "prefix": dbus.MakeVariant(uint32(24))}}, "NameserverData": []map[string]dbus.Variant{{"address": dbus.MakeVariant("192.0.2.53")}}}
	b, e := json.Marshal(normalizeDBus(raw))
	if e != nil {
		t.Fatal(e)
	}
	for _, want := range []string{"192.0.2.2", "198.51.100.0", "192.0.2.53"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("snapshot lost %s: %s", want, b)
		}
	}
}
