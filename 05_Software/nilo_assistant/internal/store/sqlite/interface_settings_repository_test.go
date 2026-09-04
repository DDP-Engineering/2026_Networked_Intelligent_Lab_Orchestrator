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

func openInterfaceSettings(t *testing.T) (*InterfaceSettingsRepository, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nilo.db")
	database, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewInterfaceSettingsRepository(database), path
}

func wiredSettings() network.InterfaceSettings {
	return network.InterfaceSettings{
		AdapterID: "aa:00:00:00:00:01", Method: network.MethodStatic,
		Address: netip.MustParsePrefix("192.168.0.1/24"), Gateway: netip.MustParseAddr("192.168.0.254"),
		DNS:          []netip.Addr{netip.MustParseAddr("192.168.0.53"), netip.MustParseAddr("9.9.9.9")},
		NeverDefault: true, ManagementProtected: true, ConnectionRef: "nm-uuid-1",
	}
}

// TestInterfaceSettingsRoundTripAndRevision covers REQ-FUNC-5e2b91d7's
// persistence and REQ-DATA-7b62c9f3: every field survives a restart, the
// revision changes on an edit and not on an unchanged save, and the adapter
// identity is the key.
func TestInterfaceSettingsRoundTripAndRevision(t *testing.T) {
	ctx := context.Background()
	repository, path := openInterfaceSettings(t)
	at := time.Date(2026, 9, 4, 11, 0, 0, 0, time.UTC)
	saved, err := repository.Save(ctx, wiredSettings(), at)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if saved.Revision == "" || !saved.UpdatedAt.Equal(at) {
		t.Fatalf("saved bookkeeping = %+v", saved)
	}
	again, err := repository.Save(ctx, wiredSettings(), at.Add(time.Minute))
	if err != nil {
		t.Fatalf("save again: %v", err)
	}
	if again.Revision != saved.Revision {
		t.Fatal("an unchanged save changed the revision")
	}
	edited := wiredSettings()
	edited.DNS = edited.DNS[:1]
	changed, err := repository.Save(ctx, edited, at.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("save edit: %v", err)
	}
	if changed.Revision == saved.Revision {
		t.Fatal("an edit kept the revision")
	}
	wireless := network.InterfaceSettings{
		AdapterID: "aa:00:00:00:00:02", Method: network.MethodAutomatic, DNSAutomatic: true,
		WiFi: &network.WiFiSettings{SSID: "lab-hotspot", Security: network.WiFiWPA2Personal, SecretRef: "nm-psk-2", ConnectAtStartup: true},
	}
	if _, err := repository.Save(ctx, wireless, at); err != nil {
		t.Fatalf("save wireless: %v", err)
	}

	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	records, err := NewInterfaceSettingsRepository(database).List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("%d records after reopening, want 2", len(records))
	}
	byAdapter := map[string]network.InterfaceSettings{}
	for _, record := range records {
		byAdapter[record.AdapterID] = record
	}
	wired := byAdapter["aa:00:00:00:00:01"]
	if wired.Method != network.MethodStatic || wired.Address != edited.Address || wired.Gateway != edited.Gateway ||
		len(wired.DNS) != 1 || wired.DNS[0] != edited.DNS[0] || !wired.NeverDefault || !wired.ManagementProtected ||
		wired.ConnectionRef != "nm-uuid-1" || wired.Revision != changed.Revision {
		t.Fatalf("wired round trip = %+v, want %+v", wired, edited)
	}
	wifi := byAdapter["aa:00:00:00:00:02"]
	if wifi.WiFi == nil || wifi.WiFi.SSID != "lab-hotspot" || wifi.WiFi.Security != network.WiFiWPA2Personal ||
		wifi.WiFi.SecretRef != "nm-psk-2" || !wifi.WiFi.ConnectAtStartup || !wifi.DNSAutomatic {
		t.Fatalf("wireless round trip = %+v", wifi)
	}
	if err := NewInterfaceSettingsRepository(database).Delete(ctx, "aa:00:00:00:00:02"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := NewInterfaceSettingsRepository(database).Delete(ctx, "aa:00:00:00:00:02"); !errors.Is(err, network.ErrInterfaceSettingsNotFound) {
		t.Fatalf("deleting twice: %v, want not found", err)
	}
}

// TestInterfaceSettingsNeverStoreAPassword covers REQ-FUNC-7c40aa18's storage
// rule: the table holds a reference to a password, never a password.
func TestInterfaceSettingsNeverStoreAPassword(t *testing.T) {
	ctx := context.Background()
	repository, _ := openInterfaceSettings(t)
	wireless := network.InterfaceSettings{
		AdapterID: "aa:00:00:00:00:02", Method: network.MethodAutomatic, DNSAutomatic: true,
		WiFi: &network.WiFiSettings{SSID: "lab", Security: network.WiFiWPA2Personal, SecretRef: "correct horse battery"},
	}
	if _, err := repository.Save(ctx, wireless, time.Now().UTC()); err == nil {
		t.Fatal("a secret reference that looks like a passphrase was stored")
	}
	if records, _ := repository.List(ctx); len(records) != 0 {
		t.Fatal("the refused record was stored anyway")
	}
}
