package network

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

// memoryDeviceNetworkStore keeps saved device networks in memory.
type memoryDeviceNetworkStore struct {
	mu      sync.Mutex
	records map[string]DeviceNetwork
}

func newMemoryDeviceNetworkStore() *memoryDeviceNetworkStore {
	return &memoryDeviceNetworkStore{records: map[string]DeviceNetwork{}}
}

func TestCandidateAdapterIsTreatedAsTheDeviceSideDuringCreate(t *testing.T) {
	t.Parallel()
	proposal := validHost(t)
	state := surroundings{interfaces: []Interface{{
		Name:      "eth0",
		AdapterID: proposal.DeviceInterface,
		Kind:      KindEthernet,
		State:     StateConnected,
		IPv4:      []string{proposal.ApplianceAddress.String()},
	}}}

	if err := proposal.ValidateAgainst(state.connectedSubnets(proposal.DeviceInterface), nil); err != nil {
		t.Fatalf("the candidate device adapter conflicted with its own configured subnet: %v", err)
	}
}

func TestDeviceNetworkManagerRejectsAManagementClaimBeforeStoreOrForwarderMutation(t *testing.T) {
	simulation := NewSimulation(time.Now)
	coordinator := NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{
			AdapterID: proposedNetwork().DeviceInterface, Interface: "wlan0", Role: AdapterRoleManagement,
		}}
	})
	forwarder := newMemoryForwarder()
	store := newMemoryDeviceNetworkStore()
	manager, err := NewDeviceNetworkManager(DeviceNetworkManagerOptions{
		Forwarder: forwarder, Environment: simulationEnvironment{s: simulation},
		Coordinator: coordinator,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.UseStore(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	change := Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: proposedNetwork()}
	if _, err := manager.Inspect(context.Background(), change); err == nil {
		t.Fatal("preview accepted a management adapter as device-facing")
	} else {
		var conflict *AdapterRoleConflictError
		if !errors.As(err, &conflict) || conflict.RequestedRole != AdapterRoleDevice {
			t.Fatalf("preview error=%v, want device role conflict", err)
		}
	}
	if err := manager.Apply(context.Background(), change); err == nil {
		t.Fatal("apply accepted a management adapter as device-facing")
	}
	if len(store.records) != 0 || len(forwarder.snapshot()) != 0 {
		t.Fatalf("refusal mutated store or forwarder: records=%+v plans=%+v", store.records, forwarder.snapshot())
	}
}

func (store *memoryDeviceNetworkStore) Save(_ context.Context, record DeviceNetwork, at time.Time) (DeviceNetwork, error) {
	if err := record.Validate(); err != nil {
		return DeviceNetwork{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record.Revision = record.ContentRevision()
	record.UpdatedAt = at
	if existing, ok := store.records[record.ID]; ok {
		record.CreatedAt = existing.CreatedAt
	} else {
		record.CreatedAt = at
	}
	store.records[record.ID] = record
	return record, nil
}

func (store *memoryDeviceNetworkStore) List(context.Context) ([]DeviceNetwork, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	records := make([]DeviceNetwork, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, record)
	}
	return records, nil
}

func (store *memoryDeviceNetworkStore) Delete(_ context.Context, id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.records[id]; !ok {
		return ErrDeviceNetworkNotFound
	}
	delete(store.records, id)
	return nil
}

type deviceNetworkHarness struct {
	simulation *Simulation
	manager    *OperationManager
	store      *memoryDeviceNetworkStore
	journal    *recordingJournal
	now        time.Time
}

func newDeviceNetworkHarness(t *testing.T) *deviceNetworkHarness {
	t.Helper()
	harness := &deviceNetworkHarness{
		store: newMemoryDeviceNetworkStore(), journal: &recordingJournal{},
		now: time.Date(2026, 9, 4, 14, 0, 0, 0, time.UTC),
	}
	harness.simulation = NewSimulation(func() time.Time { return harness.now })
	ctx := context.Background()
	if err := harness.simulation.UseProfileStore(ctx, newMemoryProfileStore()); err != nil {
		t.Fatal(err)
	}
	if err := harness.simulation.UseInterfaceSettingsStore(ctx, newMemorySettingsStore()); err != nil {
		t.Fatal(err)
	}
	if err := harness.simulation.UseDeviceNetworkStore(ctx, harness.store); err != nil {
		t.Fatal(err)
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: harness.simulation, Observer: harness.simulation,
		Now: func() time.Time { return harness.now }, NewID: sequentialIDs(), Audit: harness.journal,
	})
	if err != nil {
		t.Fatal(err)
	}
	harness.manager = manager
	return harness
}

func (harness *deviceNetworkHarness) run(t *testing.T, change Change) Operation {
	t.Helper()
	ctx := context.Background()
	previewed, err := harness.manager.PreviewChange(ctx, firstActor, change)
	if err != nil {
		t.Fatalf("preview %s: %v", change.Action, err)
	}
	applied, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision)
	if err != nil {
		t.Fatalf("acknowledge %s: %v", change.Action, err)
	}
	if applied.State == StateAwaitingConfirmation {
		if applied, err = harness.manager.Confirm(ctx, firstActor, applied.ID); err != nil {
			t.Fatalf("confirm %s: %v", change.Action, err)
		}
	}
	return applied
}

// proposedNetwork is a device network on the simulated bench: the wireless
// adapter faces the devices, the wired adapter is the local ingress.
func proposedNetwork() *DeviceNetwork {
	return &DeviceNetwork{
		Name: "Line 3", Location: "Hall B", DeviceInterface: "02:00:00:00:00:02",
		ApplianceAddress: netip.MustParsePrefix("192.168.0.1/24"), Scope: ScopeHost,
		Target: netip.MustParseAddr("192.168.0.130"), Addressing: AddressingOriginal,
		Access: AccessLocal, LocalInterface: "02:00:00:00:00:01",
		LocalSources: []netip.Prefix{netip.MustParsePrefix("192.168.10.0/24")},
	}
}

func (harness *deviceNetworkHarness) statuses(t *testing.T) map[string]DeviceNetworkStatus {
	t.Helper()
	statuses, err := harness.simulation.DeviceNetworks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]DeviceNetworkStatus{}
	for _, status := range statuses {
		byName[status.Name] = status
	}
	return byName
}

// TestDeviceNetworkLifecycleThroughTheOperationManager covers REQ-FUNC-9b17ce40
// and REQ-FUNC-2f8d63b5 in the simulation: create, enable, edit, disable and
// delete all run through the acknowledged lifecycle, intent and applied state
// stay separate, and every stage is recorded.
func TestDeviceNetworkLifecycleThroughTheOperationManager(t *testing.T) {
	harness := newDeviceNetworkHarness(t)
	created := harness.run(t, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: proposedNetwork()})
	if created.State != StateConfirmed {
		t.Fatalf("create state = %s, want confirmed without a window", created.State)
	}
	line := harness.statuses(t)["Line 3"]
	if line.ID == "" || line.State != DeviceNetworkDisabled || line.StartEnabled {
		t.Fatalf("after create = %+v, want a saved, disabled network", line)
	}
	enabled := harness.run(t, Change{Action: ActionDeviceNetworkEnable, Target: line.ID})
	if enabled.State != StateConfirmed {
		t.Fatalf("enable = %+v", enabled)
	}
	line = harness.statuses(t)["Line 3"]
	if line.State != DeviceNetworkActive || !line.StartEnabled || !line.Applied {
		t.Fatalf("after enable = %+v, want active and applied", line)
	}
	plans := harness.simulation.AppliedForwarding()
	if len(plans) != 1 || plans[0].DeviceNetworkID != line.ID || plans[0].Waiting {
		t.Fatalf("applied forwarding = %+v", plans)
	}
	edited := proposedNetwork()
	edited.ID = line.ID
	edited.Location = "Hall C"
	edited.Services = ServicePolicy{Protocols: []string{"tcp"}, TCPPorts: []int{502}}
	harness.run(t, Change{Action: ActionDeviceNetworkUpdate, Target: line.ID, DeviceNetwork: edited})
	line = harness.statuses(t)["Line 3"]
	if line.Location != "Hall C" || line.State != DeviceNetworkActive || len(line.Services.TCPPorts) != 1 {
		t.Fatalf("after edit = %+v", line)
	}
	if grants := grantsOf(harness.simulation.AppliedForwarding()[0]); len(grants) != 1 || grants[0].Protocol != "tcp" || grants[0].Ports[0] != 502 {
		t.Fatalf("grants after the edit = %+v, want the restricted policy applied", grants)
	}
	harness.run(t, Change{Action: ActionDeviceNetworkDisable, Target: line.ID})
	line = harness.statuses(t)["Line 3"]
	if line.State != DeviceNetworkDisabled || line.StartEnabled || line.Applied {
		t.Fatalf("after disable = %+v", line)
	}
	if len(harness.simulation.AppliedForwarding()) != 0 {
		t.Fatal("forwarding survived the disable")
	}
	harness.run(t, Change{Action: ActionDeviceNetworkDelete, Target: line.ID})
	if _, present := harness.statuses(t)["Line 3"]; present {
		t.Fatal("the network survived deletion")
	}
	actions := map[string]int{}
	for _, record := range harness.journal.records {
		actions[record.Action]++
	}
	for _, action := range []Action{ActionDeviceNetworkCreate, ActionDeviceNetworkEnable, ActionDeviceNetworkUpdate, ActionDeviceNetworkDisable, ActionDeviceNetworkDelete} {
		if actions[string(action)] == 0 {
			t.Fatalf("no audit record for %s: %v", action, actions)
		}
	}
	for _, record := range harness.journal.records {
		if record.Component != "network.device_networks" || record.SubjectType != "device_network" {
			t.Fatalf("record component/subject = %s/%s", record.Component, record.SubjectType)
		}
	}
}

// TestDeviceNetworkRefusalsNameTheConflict covers REQ-FUNC-46b1d8e3 and the
// management clause: a device subnet that overlaps the uplink, a sibling on
// the same adapter, and a device side on the management path are refused by
// name, and a rollback restores the previous exposure.
func TestDeviceNetworkRefusalsNameTheConflict(t *testing.T) {
	harness := newDeviceNetworkHarness(t)
	ctx := context.Background()
	overlapping := proposedNetwork()
	overlapping.ApplianceAddress = netip.MustParsePrefix("192.168.10.7/24")
	overlapping.Target = netip.MustParseAddr("192.168.10.130")
	_, err := harness.simulation.Inspect(ctx, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: overlapping})
	if !errors.Is(err, ErrDeviceNetworkRejected) || !strings.Contains(err.Error(), "192.168.10.0/24") {
		t.Fatalf("overlap with the uplink: %v", err)
	}
	onManagement := proposedNetwork()
	onManagement.DeviceInterface, onManagement.LocalInterface = "02:00:00:00:00:01", "02:00:00:00:00:02"
	_, err = harness.simulation.Inspect(ctx, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: onManagement, Origin: "192.168.10.13"})
	if !errors.Is(err, ErrManagementPathRequired) {
		t.Fatalf("device side on the client's own adapter: %v", err)
	}
	harness.run(t, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: proposedNetwork()})
	sibling := proposedNetwork()
	sibling.Name = "Line 4"
	sibling.ApplianceAddress = netip.MustParsePrefix("192.168.5.1/24")
	sibling.Target = netip.MustParseAddr("192.168.5.130")
	_, err = harness.simulation.Inspect(ctx, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: sibling})
	if !errors.Is(err, ErrDeviceNetworkRejected) || !strings.Contains(err.Error(), "Line 3") {
		t.Fatalf("sibling on the same adapter: %v", err)
	}

	// An unconfirmed enable is put back: the exposure closes again.
	line := harness.statuses(t)["Line 3"]
	previewed, err := harness.manager.PreviewChange(ctx, firstActor, Change{Action: ActionDeviceNetworkEnable, Target: line.ID})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision)
	if err != nil || applied.State != StateAwaitingConfirmation {
		t.Fatalf("enable = %+v %v", applied, err)
	}
	if harness.statuses(t)["Line 3"].State != DeviceNetworkActive {
		t.Fatal("enable did not open the exposure")
	}
	rolled, err := harness.manager.Rollback(ctx, firstActor, applied.ID)
	if err != nil || rolled.State != StateRolledBack {
		t.Fatalf("rollback = %+v %v", rolled, err)
	}
	line = harness.statuses(t)["Line 3"]
	if line.State != DeviceNetworkDisabled || line.StartEnabled || len(harness.simulation.AppliedForwarding()) != 0 {
		t.Fatalf("after rollback = %+v", line)
	}
}

// TestDeviceNetworkWaitsForItsPrerequisites covers REQ-REL-b6d0e358 and the
// profile dependency: an enabled network over a tunnel that is down waits with
// its guards closed, opens when the tunnel comes up, and keeps its profile from
// being removed.
func TestDeviceNetworkWaitsForItsPrerequisites(t *testing.T) {
	harness := newDeviceNetworkHarness(t)
	ctx := context.Background()
	imported := harness.run(t, Change{Action: ActionProfileImport, Target: "office", ProfileText: importedTunnel("office")})
	if imported.State != StateConfirmed {
		t.Fatal(imported.State)
	}
	overTunnel := proposedNetwork()
	overTunnel.Access = AccessWireGuard
	overTunnel.LocalInterface, overTunnel.LocalSources = "", nil
	overTunnel.VPNProfileID = "sim-office"
	overTunnel.VPNSources = []netip.Prefix{netip.MustParsePrefix("10.60.0.0/16")}
	harness.run(t, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: overTunnel})
	line := harness.statuses(t)["Line 3"]
	harness.run(t, Change{Action: ActionDeviceNetworkEnable, Target: line.ID})
	line = harness.statuses(t)["Line 3"]
	if line.State != DeviceNetworkWaiting || !line.StartEnabled || line.Applied || !strings.Contains(line.Detail, "office") {
		t.Fatalf("enabled over a down tunnel = %+v, want waiting", line)
	}
	plans := harness.simulation.AppliedForwarding()
	if len(plans) != 1 || !plans[0].Waiting || len(grantsOf(plans[0])) != 0 {
		t.Fatalf("plan while waiting = %+v, want guards only", plans)
	}
	if _, err := harness.simulation.Inspect(ctx, Change{Action: ActionProfileRemove, Target: "sim-office"}); !errors.Is(err, ErrProfileReferenced) {
		t.Fatalf("removing the referenced profile: %v", err)
	}
	harness.run(t, Change{Action: ActionVPNActivate, Target: "sim-office"})
	line = harness.statuses(t)["Line 3"]
	if line.State != DeviceNetworkActive || !line.Applied {
		t.Fatalf("after the tunnel came up = %+v, want active", line)
	}
	harness.run(t, Change{Action: ActionVPNDeactivate, Target: "active"})
	if line = harness.statuses(t)["Line 3"]; line.State != DeviceNetworkWaiting {
		t.Fatalf("after the tunnel went down = %+v, want waiting", line)
	}
}

// TestDeviceNetworkCheckAndInstructionsReportSeparateFacts covers
// REQ-FUNC-b207f45a and REQ-FUNC-c93e610b in the simulation.
func TestDeviceNetworkCheckAndInstructionsReportSeparateFacts(t *testing.T) {
	harness := newDeviceNetworkHarness(t)
	ctx := context.Background()
	translated := proposedNetwork()
	translated.Addressing, translated.Exposed = AddressingTranslated, netip.MustParsePrefix("10.60.1.130/32")
	translated.Access = AccessBoth
	translated.VPNProfileID = "sim-office"
	translated.VPNSources = []netip.Prefix{netip.MustParsePrefix("10.60.0.0/16")}
	harness.run(t, Change{Action: ActionProfileImport, Target: "office", ProfileText: importedTunnel("office")})
	harness.run(t, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: translated})
	line := harness.statuses(t)["Line 3"]

	check, err := harness.simulation.CheckDeviceNetwork(ctx, line.ID, DeviceNetworkProbe{})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	facts := map[string]DeviceNetworkFact{}
	for _, fact := range check.Facts {
		facts[fact.Name] = fact
	}
	for _, name := range []string{"adapter", "configuration", "tunnel", "target"} {
		if _, ok := facts[name]; !ok {
			t.Fatalf("check omits the %s fact: %+v", name, check.Facts)
		}
	}
	if facts["tunnel"].Result != FactFailed || facts["target"].Result != FactUnknown {
		t.Fatalf("disabled network over a down tunnel: tunnel=%s target=%s", facts["tunnel"].Result, facts["target"].Result)
	}
	if strings.Contains(strings.ToLower(facts["target"].Detail), "offline") {
		t.Fatal("an unanswered probe was reported as the device being offline")
	}

	instructions, err := harness.simulation.AccessInstructions(ctx, line.ID)
	if err != nil {
		t.Fatalf("instructions: %v", err)
	}
	if instructions.TargetAddress != "10.60.1.130" {
		t.Fatalf("target address = %q, want the exposed address", instructions.TargetAddress)
	}
	machines := map[string]bool{}
	for _, step := range instructions.Steps {
		machines[step.Machine] = true
		if step.Machine == "" || step.Text == "" {
			t.Fatalf("step without a machine or text: %+v", step)
		}
		if strings.Contains(strings.ToLower(step.Text), "privatekey") || strings.Contains(step.Text, "SENTINEL") {
			t.Fatalf("instruction carries key material: %s", step.Text)
		}
	}
	for _, machine := range []string{"client computer", "vpn hub", "engineering software"} {
		if !machines[machine] {
			t.Fatalf("no step for %s: %+v", machine, instructions.Steps)
		}
	}
	unmet := strings.Join(instructions.UnmetPrerequisites, " | ")
	if !strings.Contains(unmet, "office") || !strings.Contains(unmet, "disabled") {
		t.Fatalf("unmet prerequisites = %q, want the inactive profile and the disabled network", unmet)
	}
}

func grantsOf(plan ForwardingPlan) []Resource {
	grants := make([]Resource, 0)
	for _, resource := range plan.Resources {
		if resource.Kind == ResourceForwardGrant {
			grants = append(grants, resource)
		}
	}
	return grants
}

// TestDeviceNetworkManagerSnapshotRestoresRecords covers REQ-REL-b6d0e358 on
// the saved side: a snapshot taken before a change puts the inventory back
// exactly, whether the change added or edited a record.
func TestDeviceNetworkManagerSnapshotRestoresRecords(t *testing.T) {
	harness := newDeviceNetworkHarness(t)
	ctx := context.Background()
	empty, err := harness.simulation.deviceNetworks.Snapshot(ctx)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty snapshot = %+v %v", empty, err)
	}
	harness.run(t, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: proposedNetwork()})
	before, err := harness.simulation.deviceNetworks.Snapshot(ctx)
	if err != nil || len(before) != 1 {
		t.Fatalf("snapshot = %+v %v", before, err)
	}
	line := before[0]
	edited := line
	edited.Name = "Renamed"
	harness.run(t, Change{Action: ActionDeviceNetworkUpdate, Target: line.ID, DeviceNetwork: &edited})
	if err := harness.simulation.deviceNetworks.RestoreSnapshot(ctx, before); err != nil {
		t.Fatalf("restore: %v", err)
	}
	after, err := harness.simulation.deviceNetworks.Snapshot(ctx)
	if err != nil || len(after) != 1 || after[0].ID != line.ID || after[0].Name != line.Name || after[0].Revision != line.Revision || !after[0].UpdatedAt.Equal(line.UpdatedAt) {
		t.Fatalf("restored = %+v %v (want %+v)", after, err, line)
	}
	// Restoring the empty snapshot deletes what was added since.
	if err := harness.simulation.deviceNetworks.RestoreSnapshot(ctx, empty); err != nil {
		t.Fatal(err)
	}
	if after, _ := harness.simulation.deviceNetworks.Snapshot(ctx); len(after) != 0 {
		t.Fatalf("an empty snapshot restores an empty inventory, got %+v", after)
	}
}

// TestDeviceNetworkRejectsSourcesTheProfileDoesNotRouteBack covers
// REQ-SEC-a05f2c1b on the return path: a VPN source the selected profile does
// not carry in any peer's AllowedIPs would never get its replies, so it is
// refused at the preview rather than silently failing later.
func TestDeviceNetworkRejectsSourcesTheProfileDoesNotRouteBack(t *testing.T) {
	harness := newDeviceNetworkHarness(t)
	ctx := context.Background()
	harness.run(t, Change{Action: ActionProfileImport, Target: "office", ProfileText: importedTunnel("office")})
	unrouted := proposedNetwork()
	unrouted.Access = AccessWireGuard
	unrouted.LocalInterface, unrouted.LocalSources = "", nil
	unrouted.VPNProfileID = "sim-office"
	unrouted.VPNSources = []netip.Prefix{netip.MustParsePrefix("10.61.0.0/24")}
	_, err := harness.simulation.Inspect(ctx, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: unrouted})
	if !errors.Is(err, ErrDeviceNetworkRejected) || !strings.Contains(err.Error(), "10.61.0.0/24") || !strings.Contains(err.Error(), "AllowedIPs") {
		t.Fatalf("err = %v", err)
	}
	routed := *unrouted
	routed.VPNSources = []netip.Prefix{netip.MustParsePrefix("10.60.5.0/24")}
	if _, err := harness.simulation.Inspect(ctx, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: &routed}); err != nil {
		t.Fatalf("a routed source was refused: %v", err)
	}
}
