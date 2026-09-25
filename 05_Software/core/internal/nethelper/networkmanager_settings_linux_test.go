//go:build linux

package nethelper

import (
	"context"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// TestNetworkManagerSettingsSystem covers REQ-FUNC-5e2b91d7 and
// REQ-FUNC-3a71c204 on real NetworkManager: a static address is applied
// through a NILO-owned profile pinned to the adapter's hardware address,
// observed on the device, and put back by an explicit restoration with the
// operating system's own profile active again.
func TestNetworkManagerSettingsSystem(t *testing.T) {
	systemTunnelGate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
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
	devices, err := driver.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var before Device
	for _, device := range devices {
		if device.Interface.Name == "eth0" {
			before = device
		}
	}
	if before.Interface.AdapterID == "" || before.Connection == "" {
		t.Fatalf("eth0 is not ready: %+v", before.Interface)
	}
	settings := &network.InterfaceSettings{
		AdapterID: before.Interface.AdapterID, Method: network.MethodStatic,
		Address: netip.MustParsePrefix("192.0.2.7/24"), Gateway: netip.MustParseAddr("192.0.2.1"),
		DNS: []netip.Addr{netip.MustParseAddr("192.0.2.53")},
	}
	record, err := engine.Prepare(ctx, "static", network.Change{Action: network.ActionInterfaceSettings, Target: before.Interface.ID, Settings: settings}, network.ConfirmationWindow)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer func() {
		recovery, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		_ = engine.Restore(recovery, record.ID)
	}()
	if err := engine.Apply(ctx, "static"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	applied, err := driver.find(ctx, before.Interface.ID)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Connection == before.Connection || len(applied.Interface.IPv4) != 1 || applied.Interface.IPv4[0] != "192.0.2.7/24" || applied.Interface.Gateway4 != "192.0.2.1" {
		t.Fatalf("after the static change = %+v connection=%s", applied.Interface, applied.Connection)
	}
	_, owned, err := driver.ownedConnection(ctx, before.Interface.AdapterID)
	if err != nil || owned == nil {
		t.Fatalf("no NILO-owned profile for the adapter: %v", err)
	}
	if !strings.HasPrefix(value[string](owned["connection"], "id"), niloInterfacePrefix) || value[string](owned["ipv4"], "method") != "manual" {
		t.Fatalf("owned profile = %+v", owned["connection"])
	}
	if err := engine.Restore(ctx, "static"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, err := driver.find(ctx, before.Interface.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameState(restored, before) {
		t.Fatalf("restored=%+v before=%+v", restored.Interface, before.Interface)
	}
	// The operating system's own profile is untouched and NILO's still exists
	// for the next change, inactive.
	if restored.Connection != before.Connection {
		t.Fatalf("restored connection = %s, want %s", restored.Connection, before.Connection)
	}
	_, owned, err = driver.ownedConnection(ctx, before.Interface.AdapterID)
	if err != nil || owned == nil {
		t.Fatal("the NILO-owned profile disappeared with the restoration")
	}
	if err := driver.DeleteTunnel(ctx, value[string](owned["connection"], "uuid")); err == nil {
		t.Fatal("a non-tunnel profile was deleted through the tunnel path")
	}
}

func TestAccessPointClassification(t *testing.T) {
	for _, tc := range []struct {
		flags, wpa, rsn uint32
		want            network.WiFiSecurity
		supported       bool
	}{
		{0, 0, 0, network.WiFiOpen, true},
		{apFlagPrivacy, 0, apKeyManagementPSK, network.WiFiWPA2Personal, true},
		{apFlagPrivacy, apKeyManagementPSK, 0, network.WiFiWPA2Personal, true},
		{apFlagPrivacy, 0, apKeyManagementSAE | apKeyManagementPSK, network.WiFiWPA3Personal, true},
		{apFlagPrivacy, 0, apKeyManagement8021X, network.WiFiSecurityUnsupported, false},
		{apFlagPrivacy, 0, 0, network.WiFiSecurityUnsupported, false},
		{0, 0, apKeyManagementOWE, network.WiFiSecurityUnsupported, false},
	} {
		got, supported := classifyAccessPoint(tc.flags, tc.wpa, tc.rsn)
		if got != tc.want || supported != tc.supported {
			t.Fatalf("flags=%x wpa=%x rsn=%x: %s/%t, want %s/%t", tc.flags, tc.wpa, tc.rsn, got, supported, tc.want, tc.supported)
		}
	}
}
