package sqlite

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

func openDeviceNetworks(t *testing.T) (*DeviceNetworkRepository, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nilo.db")
	database, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewDeviceNetworkRepository(database), path
}

func lineNetwork(id string) network.DeviceNetwork {
	return network.DeviceNetwork{
		ID: id, Name: "Line 3", Location: "Hall B", DeviceInterface: "aa:00:00:00:00:03",
		ApplianceAddress: netip.MustParsePrefix("192.168.0.1/24"), Scope: network.ScopeHost,
		Target: netip.MustParseAddr("192.168.0.130"), Addressing: network.AddressingTranslated,
		Exposed: netip.MustParsePrefix("10.60.1.130/32"), Access: network.AccessBoth,
		LocalInterface: "aa:00:00:00:00:01", LocalSources: []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")},
		VPNProfileID: "profile-a", VPNSources: []netip.Prefix{netip.MustParsePrefix("10.205.148.0/24")},
		Services:     network.ServicePolicy{Protocols: []string{"tcp"}, TCPPorts: []int{502}},
		StartEnabled: true,
	}
}

// TestDeviceNetworkRoundTripAndRevision covers REQ-FUNC-2f8d63b5 and
// REQ-DATA-7b62c9f3 for device networks: every field survives a restart, the
// revision follows the content, and a subnet-scoped network stores without a
// target.
func TestDeviceNetworkRoundTripAndRevision(t *testing.T) {
	ctx := context.Background()
	repository, path := openDeviceNetworks(t)
	at := time.Date(2026, 9, 4, 13, 0, 0, 0, time.UTC)
	saved, err := repository.Save(ctx, lineNetwork("dn-1"), at)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Revision == "" {
		t.Fatal("no revision assigned")
	}
	again, err := repository.Save(ctx, lineNetwork("dn-1"), at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if again.Revision != saved.Revision {
		t.Fatal("an unchanged save changed the revision")
	}
	edited := lineNetwork("dn-1")
	edited.Location = "Hall C"
	changed, err := repository.Save(ctx, edited, at.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision == saved.Revision {
		t.Fatal("an edit kept the revision")
	}
	subnet := network.DeviceNetwork{
		ID: "dn-2", Name: "Whole cell", DeviceInterface: "aa:00:00:00:00:04",
		ApplianceAddress: netip.MustParsePrefix("10.7.0.1/24"), Scope: network.ScopeSubnet,
		Addressing: network.AddressingOriginal, Access: network.AccessLocal,
		LocalInterface: "aa:00:00:00:00:01", LocalSources: []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")},
	}
	if _, err := repository.Save(ctx, subnet, at); err != nil {
		t.Fatalf("save subnet: %v", err)
	}
	if _, err := repository.Save(ctx, network.DeviceNetwork{ID: "dn-3", Name: "bad", DeviceInterface: "x", ApplianceAddress: netip.MustParsePrefix("192.168.0.1/24"), Scope: network.ScopeHost, Addressing: network.AddressingOriginal, Access: network.AccessLocal, LocalInterface: "y", LocalSources: []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}}, at); !errors.Is(err, network.ErrDeviceNetworkRejected) {
		t.Fatalf("a host scope without a target was stored: %v", err)
	}

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	records, err := NewDeviceNetworkRepository(database).List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("%d records after reopening, want 2", len(records))
	}
	byID := map[string]network.DeviceNetwork{}
	for _, record := range records {
		byID[record.ID] = record
	}
	got := byID["dn-1"]
	if got.Name != "Line 3" || got.Location != "Hall C" || got.DeviceInterface != "aa:00:00:00:00:03" ||
		got.ApplianceAddress != edited.ApplianceAddress || got.Scope != network.ScopeHost || got.Target != edited.Target ||
		got.Addressing != network.AddressingTranslated || got.Exposed != edited.Exposed || got.Access != network.AccessBoth ||
		got.LocalInterface != "aa:00:00:00:00:01" || len(got.LocalSources) != 1 || got.LocalSources[0] != edited.LocalSources[0] ||
		got.VPNProfileID != "profile-a" || len(got.VPNSources) != 1 || len(got.Services.Protocols) != 1 || len(got.Services.TCPPorts) != 1 ||
		got.Services.TCPPorts[0] != 502 || !got.StartEnabled || got.Revision != changed.Revision {
		t.Fatalf("round trip = %+v", got)
	}
	if byID["dn-2"].Target.IsValid() || byID["dn-2"].Scope != network.ScopeSubnet || byID["dn-2"].StartEnabled {
		t.Fatalf("subnet round trip = %+v", byID["dn-2"])
	}
	if err := NewDeviceNetworkRepository(database).Delete(ctx, "dn-2"); err != nil {
		t.Fatal(err)
	}
	if err := NewDeviceNetworkRepository(database).Delete(ctx, "dn-2"); !errors.Is(err, network.ErrDeviceNetworkNotFound) {
		t.Fatalf("deleting twice: %v", err)
	}
}
