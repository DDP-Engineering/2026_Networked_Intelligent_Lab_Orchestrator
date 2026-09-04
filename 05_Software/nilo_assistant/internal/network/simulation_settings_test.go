package network

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// memorySettingsStore is the saved adapter configuration kept in memory.
type memorySettingsStore struct {
	records map[string]InterfaceSettings
}

func newMemorySettingsStore() *memorySettingsStore {
	return &memorySettingsStore{records: map[string]InterfaceSettings{}}
}

func (store *memorySettingsStore) Save(_ context.Context, settings InterfaceSettings, at time.Time) (InterfaceSettings, error) {
	if err := settings.Validate(); err != nil {
		return InterfaceSettings{}, err
	}
	settings.Revision = settings.ContentRevision()
	settings.UpdatedAt = at
	store.records[strings.ToLower(settings.AdapterID)] = settings
	return settings, nil
}

func (store *memorySettingsStore) List(context.Context) ([]InterfaceSettings, error) {
	records := make([]InterfaceSettings, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, record)
	}
	return records, nil
}

func (store *memorySettingsStore) Delete(_ context.Context, adapterID string) error {
	if _, ok := store.records[strings.ToLower(adapterID)]; !ok {
		return ErrInterfaceSettingsNotFound
	}
	delete(store.records, strings.ToLower(adapterID))
	return nil
}

func simulatedInterface(t *testing.T, s *Simulation, id string) Interface {
	t.Helper()
	interfaces, err := s.Interfaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range interfaces {
		if candidate.ID == id {
			return candidate
		}
	}
	t.Fatalf("no simulated interface %s", id)
	return Interface{}
}

// TestSimulationAppliesAndPersistsInterfaceSettings covers REQ-FUNC-5e2b91d7
// and REQ-FUNC-3a71c204 in the simulation: a static address is applied,
// survives a restart bound to the adapter, and an automatic method replaces it.
func TestSimulationAppliesAndPersistsInterfaceSettings(t *testing.T) {
	ctx := context.Background()
	store := newMemorySettingsStore()
	s := NewSimulation(time.Now)
	if err := s.UseInterfaceSettingsStore(ctx, store); err != nil {
		t.Fatal(err)
	}
	wired := simulatedInterface(t, s, "sim-enp1s0")
	if wired.AdapterID == "" {
		t.Fatal("the simulated adapter has no stable identity")
	}
	static := Change{Action: ActionInterfaceSettings, Target: "sim-enp1s0", Settings: &InterfaceSettings{
		Method: MethodStatic, Address: netip.MustParsePrefix("192.168.0.1/24"), DNS: []netip.Addr{netip.MustParseAddr("192.168.0.53")}, NeverDefault: true,
	}}
	if _, err := s.Inspect(ctx, static); err != nil {
		t.Fatalf("inspect static: %v", err)
	}
	if err := s.ApplyChange(ctx, static); err != nil {
		t.Fatalf("apply static: %v", err)
	}
	wired = simulatedInterface(t, s, "sim-enp1s0")
	if len(wired.IPv4) != 1 || wired.IPv4[0] != "192.168.0.1/24" || wired.Gateway4 != "" {
		t.Fatalf("after the static change = %+v", wired)
	}
	if wired.Settings == nil || !wired.Settings.Saved || !wired.Settings.Applied || wired.Settings.Method != MethodStatic || !wired.Settings.NeverDefault {
		t.Fatalf("settings state after the static change = %+v", wired.Settings)
	}
	saved, _ := store.List(ctx)
	if len(saved) != 1 || saved[0].AdapterID != wired.AdapterID || saved[0].Address.String() != "192.168.0.1/24" {
		t.Fatalf("saved = %+v, want the static configuration bound to %s", saved, wired.AdapterID)
	}

	// A restart with the same store comes back with the intent applied.
	restarted := NewSimulation(time.Now)
	if err := restarted.UseInterfaceSettingsStore(ctx, store); err != nil {
		t.Fatal(err)
	}
	wired = simulatedInterface(t, restarted, "sim-enp1s0")
	if len(wired.IPv4) != 1 || wired.IPv4[0] != "192.168.0.1/24" || wired.Settings == nil || !wired.Settings.Applied {
		t.Fatalf("after the restart = %+v", wired)
	}

	automatic := Change{Action: ActionInterfaceSettings, Target: "sim-enp1s0", Settings: &InterfaceSettings{Method: MethodAutomatic, DNSAutomatic: true}}
	if err := restarted.ApplyChange(ctx, automatic); err != nil {
		t.Fatalf("apply automatic: %v", err)
	}
	wired = simulatedInterface(t, restarted, "sim-enp1s0")
	if len(wired.IPv4) != 1 || wired.IPv4[0] == "192.168.0.1/24" || wired.Gateway4 == "" {
		t.Fatalf("after the automatic change = %+v, want a leased address and gateway", wired)
	}
	invalid := Change{Action: ActionInterfaceSettings, Target: "sim-enp1s0", Settings: &InterfaceSettings{Method: MethodStatic, Address: netip.MustParsePrefix("192.168.0.0/24")}}
	if _, err := restarted.Inspect(ctx, invalid); !errors.Is(err, ErrInterfaceSettingsRejected) {
		t.Fatalf("inspect an invalid address: %v", err)
	}
	if _, err := restarted.Inspect(ctx, Change{Action: ActionInterfaceSettings, Target: "sim-lo", Settings: &InterfaceSettings{Method: MethodAutomatic, DNSAutomatic: true}}); !errors.Is(err, ErrNotControllable) {
		t.Fatalf("settings on loopback: %v", err)
	}
}

// TestSimulationScansAndJoinsWiFi covers REQ-FUNC-7c40aa18 in the simulation:
// the scan lists supported and unsupported networks, a join connects the
// wireless adapter and persists the intent with a reference and no password.
func TestSimulationScansAndJoinsWiFi(t *testing.T) {
	ctx := context.Background()
	store := newMemorySettingsStore()
	s := NewSimulation(time.Now)
	if err := s.UseInterfaceSettingsStore(ctx, store); err != nil {
		t.Fatal(err)
	}
	scan, err := s.WiFiScan(ctx, "sim-wlan0")
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !scan.Supported || len(scan.Networks) < 3 {
		t.Fatalf("scan = %+v", scan)
	}
	var supported, unsupported int
	for _, found := range scan.Networks {
		if found.Supported {
			supported++
		} else {
			unsupported++
		}
	}
	if supported == 0 || unsupported == 0 {
		t.Fatalf("scan lists supported=%d unsupported=%d", supported, unsupported)
	}
	if _, err := s.WiFiScan(ctx, "sim-enp1s0"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("scanning on a wired adapter: %v", err)
	}
	join := Change{Action: ActionWiFiJoin, Target: "sim-wlan0", WiFi: &WiFiJoin{SSID: "lab-hotspot", Security: WiFiWPA2Personal, ConnectAtStartup: true}, SecretHandle: strings.Repeat("ab", 32)}
	if _, err := s.Inspect(ctx, join); err != nil {
		t.Fatalf("inspect join: %v", err)
	}
	if err := s.ApplyChange(ctx, join); err != nil {
		t.Fatalf("apply join: %v", err)
	}
	wireless := simulatedInterface(t, s, "sim-wlan0")
	if wireless.State != StateConnected || wireless.SSID != "lab-hotspot" || len(wireless.IPv4) == 0 {
		t.Fatalf("after the join = %+v", wireless)
	}
	if wireless.Settings == nil || wireless.Settings.SSID != "lab-hotspot" || !wireless.Settings.Applied {
		t.Fatalf("settings after the join = %+v", wireless.Settings)
	}
	saved, _ := store.List(ctx)
	if len(saved) != 1 || saved[0].WiFi == nil || saved[0].WiFi.SSID != "lab-hotspot" || !saved[0].WiFi.ConnectAtStartup || saved[0].WiFi.SecretRef == "" {
		t.Fatalf("saved wireless intent = %+v", saved)
	}
	for _, refused := range []Change{
		{Action: ActionWiFiJoin, Target: "sim-wlan0", WiFi: &WiFiJoin{SSID: "corp-enterprise", Security: WiFiWPA2Personal}},
		{Action: ActionWiFiJoin, Target: "sim-wlan0", WiFi: &WiFiJoin{SSID: "nowhere", Security: WiFiOpen}},
		{Action: ActionWiFiJoin, Target: "sim-enp1s0", WiFi: &WiFiJoin{SSID: "lab-hotspot", Security: WiFiWPA2Personal}},
	} {
		if _, err := s.Inspect(ctx, refused); err == nil {
			t.Fatalf("join %s on %s was accepted", refused.WiFi.SSID, refused.Target)
		}
	}
}

// TestSimulationRefusesRemovingTheProtectedManagementPath covers
// REQ-REL-4c07b1ea in the simulation, which is what the browser exercises: a
// loopback caller traces to nothing, so only the adapter the operator protected
// is a management path, and the last one cannot be disabled.
func TestSimulationRefusesRemovingTheProtectedManagementPath(t *testing.T) {
	ctx := context.Background()
	s := NewSimulation(time.Now)
	if err := s.UseInterfaceSettingsStore(ctx, newMemorySettingsStore()); err != nil {
		t.Fatal(err)
	}
	disable := Change{Action: ActionInterfaceDisable, Target: "sim-enp1s0", Origin: "127.0.0.1"}
	if _, err := s.Inspect(ctx, disable); err != nil {
		t.Fatalf("disabling an unprotected adapter: %v", err)
	}
	protect := Change{Action: ActionInterfaceSettings, Target: "sim-enp1s0", Origin: "127.0.0.1",
		Settings: &InterfaceSettings{Method: MethodAutomatic, DNSAutomatic: true, ManagementProtected: true}}
	if err := s.ApplyChange(ctx, protect); err != nil {
		t.Fatalf("protect: %v", err)
	}
	_, err := s.Inspect(ctx, disable)
	if !errors.Is(err, ErrManagementPathRequired) || !strings.Contains(err.Error(), "enp1s0") {
		t.Fatalf("disabling the only protected adapter: %v", err)
	}
	if err := s.ApplyChange(ctx, disable); !errors.Is(err, ErrManagementPathRequired) {
		t.Fatalf("applying the disable directly: %v", err)
	}
	// A client on the wired link is also refused; a routed client with another
	// gateway path is allowed once the wireless adapter is up.
	onLink := disable
	onLink.Origin = "192.168.10.13"
	if _, err := s.Inspect(ctx, onLink); !errors.Is(err, ErrManagementPathRequired) {
		t.Fatalf("on-link client: %v", err)
	}
	join := Change{Action: ActionWiFiJoin, Target: "sim-wlan0", WiFi: &WiFiJoin{SSID: "open-cafe", Security: WiFiOpen}}
	if err := s.ApplyChange(ctx, join); err != nil {
		t.Fatal(err)
	}
	routed := disable
	routed.Origin = "203.0.113.9"
	if _, err := s.Inspect(ctx, routed); err != nil {
		t.Fatalf("routed client with a second gateway path: %v", err)
	}
}
