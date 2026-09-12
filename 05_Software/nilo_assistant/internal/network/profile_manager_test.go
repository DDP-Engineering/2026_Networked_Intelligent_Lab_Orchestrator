package network

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
)

// memoryProfileStore is the saved inventory kept in memory, with the same
// refusals the SQLite repository makes, so the manager is tested against the
// contract rather than against a database.
type memoryProfileStore struct {
	mu      sync.Mutex
	records map[string]ProfileRecord
}

func newMemoryProfileStore() *memoryProfileStore {
	return &memoryProfileStore{records: map[string]ProfileRecord{}}
}

func (store *memoryProfileStore) Save(_ context.Context, record ProfileRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for id, existing := range store.records {
		if id != record.ID && strings.EqualFold(existing.Name, record.Name) {
			return ErrProfileNameTaken
		}
	}
	store.records[record.ID] = record
	return nil
}

func (store *memoryProfileStore) List(context.Context) ([]ProfileRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	records := make([]ProfileRecord, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Name < records[j].Name })
	return records, nil
}

func (store *memoryProfileStore) Rename(_ context.Context, profileID, name string, at time.Time) (ProfileRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.records[profileID]
	if !ok {
		return ProfileRecord{}, ErrProfileNotFound
	}
	for id, existing := range store.records {
		if id != profileID && strings.EqualFold(existing.Name, name) {
			return ProfileRecord{}, ErrProfileNameTaken
		}
	}
	record.Name, record.UpdatedAt, record.Revision = name, at, "renamed-"+name
	store.records[profileID] = record
	return record, nil
}

func (store *memoryProfileStore) SelectStartup(_ context.Context, profileID string, at time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.records[profileID]; !ok {
		return ErrProfileNotFound
	}
	for id, record := range store.records {
		record.StartSelected = id == profileID
		record.UpdatedAt = at
		store.records[id] = record
	}
	return nil
}

func (store *memoryProfileStore) ClearStartup(_ context.Context, at time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	for id, record := range store.records {
		record.StartSelected = false
		record.UpdatedAt = at
		store.records[id] = record
	}
	return nil
}

func (store *memoryProfileStore) Delete(_ context.Context, profileID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.records[profileID]; !ok {
		return ErrProfileNotFound
	}
	delete(store.records, profileID)
	return nil
}

// tunnelDriverSpy stands in for the privileged helper's tunnel operations. It
// records what it was asked and answers from an inventory the test controls.
type tunnelDriverSpy struct {
	mu        sync.Mutex
	created   []createdTunnel
	deleted   []string
	connected []string
	tunnels   []TunnelObservation
	createErr error
}

type createdTunnel struct {
	record ProfileRecord
	handle string
}

func (spy *tunnelDriverSpy) CreateTunnel(_ context.Context, record ProfileRecord, handle string) (string, error) {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if spy.createErr != nil {
		return "", spy.createErr
	}
	spy.created = append(spy.created, createdTunnel{record: record, handle: handle})
	ref := "uuid-" + record.Name
	spy.tunnels = append(spy.tunnels, TunnelObservation{
		ConnectionRef: ref, Name: "nilo-wg-" + record.ID, NILOOwned: true, Profile: record.Profile,
	})
	return ref, nil
}

func (spy *tunnelDriverSpy) DeleteTunnel(_ context.Context, _ string, connectionRef string) error {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	spy.deleted = append(spy.deleted, connectionRef)
	kept := spy.tunnels[:0]
	for _, tunnel := range spy.tunnels {
		if tunnel.ConnectionRef != connectionRef {
			kept = append(kept, tunnel)
		}
	}
	spy.tunnels = kept
	return nil
}

func (spy *tunnelDriverSpy) Tunnels(context.Context) ([]TunnelObservation, error) {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	return append([]TunnelObservation(nil), spy.tunnels...), nil
}

func (spy *tunnelDriverSpy) ConnectTunnel(_ context.Context, connectionRef string) error {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	spy.connected = append(spy.connected, connectionRef)
	for index := range spy.tunnels {
		spy.tunnels[index].Active = spy.tunnels[index].ConnectionRef == connectionRef
	}
	return nil
}

func (spy *tunnelDriverSpy) setActive(connectionRef string) {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	for index := range spy.tunnels {
		spy.tunnels[index].Active = spy.tunnels[index].ConnectionRef == connectionRef
	}
}

// baseSpy is the interface half of a live appliance: the helper client that
// owns adapters, recovery and key staging. It records the exact change it was
// asked to arm, because that is where the profile identity has to have become
// a connection reference.
type baseSpy struct {
	armed      []Change
	applied    []Change
	restored   []Checkpoint
	discarded  []Checkpoint
	restoreCtx []error
	interfaces []Interface
	recovery   RecoveryOutcome
	recovered  bool
	staged     []string
	bound      []string
	dropped    []string
	counter    int
}

func (spy *baseSpy) Source() Source { return "bench-host" }
func (spy *baseSpy) Interfaces(context.Context) ([]Interface, error) {
	if spy.interfaces != nil {
		return append([]Interface(nil), spy.interfaces...), nil
	}
	return []Interface{{ID: "nm-eth0", Name: "eth0", Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.0.2.2/24"}, Gateway4: "192.0.2.1", Controllable: true}}, nil
}
func (spy *baseSpy) Counters(context.Context) ([]CounterReading, error) { return nil, nil }
func (spy *baseSpy) Inspect(_ context.Context, change Change) (map[string]string, error) {
	return map[string]string{"target": change.Target}, nil
}
func (spy *baseSpy) Observe(ctx context.Context, action Action, target string) (map[string]string, error) {
	return spy.Inspect(ctx, Change{Action: action, Target: target})
}
func (spy *baseSpy) ArmRecovery(context.Context, string, time.Duration) (Checkpoint, error) {
	return Checkpoint{}, errors.New("live recovery requires an exact change")
}
func (spy *baseSpy) ArmChange(_ context.Context, id string, change Change, _ time.Duration) (Checkpoint, error) {
	spy.armed = append(spy.armed, change)
	return Checkpoint{ID: id}, nil
}
func (spy *baseSpy) Apply(ctx context.Context, action Action, target string) error {
	return spy.ApplyChange(ctx, Change{Action: action, Target: target})
}
func (spy *baseSpy) ApplyChange(_ context.Context, change Change) error {
	spy.applied = append(spy.applied, change)
	return nil
}
func (spy *baseSpy) Restore(ctx context.Context, checkpoint Checkpoint) (bool, error) {
	spy.restored = append(spy.restored, checkpoint)
	spy.restoreCtx = append(spy.restoreCtx, ctx.Err())
	return true, nil
}
func (spy *baseSpy) Discard(_ context.Context, checkpoint Checkpoint) error {
	spy.discarded = append(spy.discarded, checkpoint)
	return nil
}
func (spy *baseSpy) StageSecrets(_ context.Context, secrets Secrets) (string, error) {
	spy.counter++
	handle := strings.Repeat("a", 60) + string(rune('0'+spy.counter)) + "000"
	spy.staged = append(spy.staged, handle)
	return handle, nil
}
func (spy *baseSpy) BindSecrets(_ context.Context, handle, profileID string) error {
	spy.bound = append(spy.bound, handle+"->"+profileID)
	return nil
}
func (spy *baseSpy) DropSecrets(_ context.Context, handle string) error {
	spy.dropped = append(spy.dropped, handle)
	return nil
}
func (spy *baseSpy) WireGuard(context.Context) (WireGuardStatus, error) {
	return WireGuardStatus{Reported: true, ReadinessOnly: true, HelperAvailable: true, NetworkManagerAvailable: true, KernelSupported: true, Tools: []ToolStatus{{Name: "wg", Installed: true, Version: "1.0"}}}, nil
}
func (spy *baseSpy) StartInstall(context.Context, string, string) (Operation, error) {
	return Operation{}, ErrUnsupported
}
func (spy *baseSpy) PollInstall(context.Context, string) (Operation, error) {
	return Operation{}, ErrUnsupported
}
func (spy *baseSpy) LastRecovery(context.Context) (RecoveryOutcome, bool, error) {
	return spy.recovery, spy.recovered, nil
}

type applianceHarness struct {
	appliance *Appliance
	base      *baseSpy
	driver    *tunnelDriverSpy
	store     *memoryProfileStore
	manager   *OperationManager
	journal   *recordingJournal
	now       time.Time
}

func newApplianceHarness(t *testing.T) *applianceHarness {
	t.Helper()
	harness := &applianceHarness{
		base: &baseSpy{}, driver: &tunnelDriverSpy{}, store: newMemoryProfileStore(),
		journal: &recordingJournal{}, now: time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC),
	}
	ids := 0
	profiles, err := NewProfileManager(ProfileManagerOptions{
		Driver: harness.driver,
		Now:    func() time.Time { return harness.now },
		NewID: func() string {
			ids++
			return "profile-" + string(rune('a'+ids-1))
		},
	})
	if err != nil {
		t.Fatalf("NewProfileManager: %v", err)
	}
	harness.appliance = NewAppliance(harness.base, profiles)
	if err := harness.appliance.UseProfileStore(context.Background(), harness.store); err != nil {
		t.Fatalf("UseProfileStore: %v", err)
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: harness.appliance, Observer: harness.appliance,
		Now:   func() time.Time { return harness.now },
		NewID: sequentialIDs(),
		Audit: harness.journal,
		// The live appliance stages key material through the helper client, which
		// is the base here.
		Stager: harness.base,
	})
	if err != nil {
		t.Fatalf("NewOperationManager: %v", err)
	}
	harness.manager = manager
	return harness
}

type applianceForwarderSpy struct{}

func (applianceForwarderSpy) ApplyForwarding(context.Context, ForwardingPlan) error { return nil }
func (applianceForwarderSpy) RemoveForwarding(context.Context, string) error        { return nil }
func (applianceForwarderSpy) AppliedForwarding(context.Context) ([]ForwardingPlan, error) {
	return nil, nil
}

type startupForwarderSpy struct{ applied []ForwardingPlan }

func (spy *startupForwarderSpy) ApplyForwarding(_ context.Context, plan ForwardingPlan) error {
	spy.applied = append(spy.applied, plan)
	return nil
}
func (*startupForwarderSpy) RemoveForwarding(context.Context, string) error { return nil }
func (spy *startupForwarderSpy) AppliedForwarding(context.Context) ([]ForwardingPlan, error) {
	return append([]ForwardingPlan(nil), spy.applied...), nil
}

type interceptOperationStore struct {
	delegate     *operationStoreSpy
	mu           sync.Mutex
	saves        int
	beforeReturn func(context.Context, int, OperationRecord) error
}

func (store *interceptOperationStore) Save(ctx context.Context, record OperationRecord) error {
	store.mu.Lock()
	store.saves++
	call := store.saves
	store.mu.Unlock()
	if store.beforeReturn != nil {
		if err := store.beforeReturn(ctx, call, record); err != nil {
			return err
		}
	}
	return store.delegate.Save(ctx, record)
}

func (store *interceptOperationStore) Unfinished(ctx context.Context) ([]OperationRecord, error) {
	return store.delegate.Unfinished(ctx)
}

func (store *interceptOperationStore) Recent(ctx context.Context, limit int) ([]OperationRecord, error) {
	return store.delegate.Recent(ctx, limit)
}

func TestApplianceDefersDeviceForwardingUntilExplicitStartupReconciliation(t *testing.T) {
	harness := newApplianceHarness(t)
	proposal := validHost(t)
	proposal.ID = "device-network-1"
	proposal.StartEnabled = true
	harness.base.interfaces = []Interface{
		{ID: "nm-device", Name: "eth0", AdapterID: proposal.DeviceInterface, Kind: KindEthernet, State: StateConnected, IPv4: []string{proposal.ApplianceAddress.String()}, Controllable: true},
		{ID: "nm-local", Name: "wlan0", AdapterID: proposal.LocalInterface, Kind: KindWiFi, State: StateConnected, IPv4: []string{"192.168.1.103/24"}, Controllable: true},
	}
	store := newMemoryDeviceNetworkStore()
	if _, err := store.Save(context.Background(), proposal, harness.now); err != nil {
		t.Fatal(err)
	}
	forwarder := &startupForwarderSpy{}
	harness.appliance.UseForwarder(forwarder)
	if err := harness.appliance.UseDeviceNetworkStore(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	if len(forwarder.applied) != 0 {
		t.Fatalf("store attachment installed forwarding: %+v", forwarder.applied)
	}
	if err := harness.appliance.ReconcileDeviceNetworks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(forwarder.applied) != 1 || forwarder.applied[0].DeviceNetworkID != proposal.ID {
		t.Fatalf("explicit reconciliation=%+v", forwarder.applied)
	}
}

func TestApplianceCreateSavesDisabledDeviceNetworkWithoutHelperCheckpoint(t *testing.T) {
	t.Parallel()
	harness := newApplianceHarness(t)
	proposal := validHost(t)
	harness.base.interfaces = []Interface{
		{ID: "nm-device", Name: "eth0", AdapterID: proposal.DeviceInterface, Kind: KindEthernet, State: StateConnected, IPv4: []string{proposal.ApplianceAddress.String()}, Controllable: true},
		{ID: "nm-local", Name: "wlan0", AdapterID: proposal.LocalInterface, Kind: KindWiFi, State: StateConnected, IPv4: []string{"192.168.1.103/24"}, Controllable: true},
	}
	harness.appliance.UseForwarder(applianceForwarderSpy{})
	if err := harness.appliance.UseDeviceNetworkStore(context.Background(), newMemoryDeviceNetworkStore()); err != nil {
		t.Fatal(err)
	}

	operation := harness.run(t, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: &proposal})
	if operation.State != StateConfirmed {
		t.Fatalf("create state = %s, want confirmed", operation.State)
	}
	if len(harness.base.armed) != 0 || len(harness.base.applied) != 0 {
		t.Fatalf("disabled create touched helper recovery: armed=%v applied=%v", harness.base.armed, harness.base.applied)
	}
}

func TestApplianceRetainsOldAndNewDeviceRoleClaimsUntilConfirmation(t *testing.T) {
	harness := newApplianceHarness(t)
	proposal := validHost(t)
	newAdapterID := "aa:bb:cc:dd:ee:03"
	harness.base.interfaces = []Interface{
		{ID: "nm-device-old", Name: "eth0", AdapterID: proposal.DeviceInterface, Kind: KindEthernet, State: StateConnected, IPv4: []string{proposal.ApplianceAddress.String()}, Controllable: true},
		{ID: "nm-local", Name: "wlan0", AdapterID: proposal.LocalInterface, Kind: KindWiFi, State: StateConnected, IPv4: []string{"192.168.1.103/24"}, Controllable: true},
		{ID: "nm-device-new", Name: "enx1", AdapterID: newAdapterID, Kind: KindEthernet, State: StateDisconnected, Controllable: true},
	}
	harness.appliance.UseForwarder(applianceForwarderSpy{})
	store := newMemoryDeviceNetworkStore()
	if err := harness.appliance.UseDeviceNetworkStore(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	created := harness.run(t, Change{Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: &proposal})
	if created.State != StateConfirmed {
		t.Fatalf("create=%+v", created)
	}
	records, _ := store.List(context.Background())
	edited := records[0]
	edited.DeviceInterface = newAdapterID
	pending := harness.run(t, Change{Action: ActionDeviceNetworkUpdate, Target: edited.ID, DeviceNetwork: &edited})
	if pending.State != StateAwaitingConfirmation {
		t.Fatalf("update=%+v", pending)
	}
	for _, adapterID := range []string{proposal.DeviceInterface, newAdapterID} {
		err := harness.appliance.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{AdapterID: adapterID, Role: AdapterRoleManagement}})
		var conflict *AdapterRoleConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("pending adapter %s was not reserved: %v", adapterID, err)
		}
	}
	if _, err := harness.manager.Confirm(context.Background(), firstActor, pending.ID); err != nil {
		t.Fatal(err)
	}
	if err := harness.appliance.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{AdapterID: proposal.DeviceInterface, Role: AdapterRoleManagement}}); err != nil {
		t.Fatalf("old role remained reserved after confirmation: %v", err)
	}
	if err := harness.appliance.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{AdapterID: newAdapterID, Role: AdapterRoleManagement}}); err == nil {
		t.Fatal("confirmed device adapter lost its durable role claim")
	}
}

func TestApplianceRehydratesDeviceNetworkRecoveryAcrossRestart(t *testing.T) {
	for _, test := range []struct {
		name      string
		action    Action
		phase     string
		wantCount int
		wantOld   bool
		wantNew   bool
		wantState OperationState
	}{
		{name: "update still applied", action: ActionDeviceNetworkUpdate, phase: "applied", wantCount: 1, wantOld: true, wantNew: true, wantState: StateAwaitingConfirmation},
		{name: "update restored", action: ActionDeviceNetworkUpdate, phase: "restored", wantCount: 1, wantOld: true, wantNew: false, wantState: StateRolledBack},
		{name: "update recovery failed", action: ActionDeviceNetworkUpdate, phase: "recovery_failed", wantCount: 1, wantOld: true, wantNew: true, wantState: StateRollbackFailed},
		{name: "update confirmed", action: ActionDeviceNetworkUpdate, phase: "confirmed", wantCount: 1, wantOld: false, wantNew: true, wantState: StateConfirmed},
		{name: "delete still applied", action: ActionDeviceNetworkDelete, phase: "applied", wantCount: 0, wantOld: true, wantNew: false, wantState: StateAwaitingConfirmation},
		{name: "delete restored", action: ActionDeviceNetworkDelete, phase: "restored", wantCount: 1, wantOld: true, wantNew: false, wantState: StateRolledBack},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			original := validHost(t)
			original.ID = "device-network-1"
			newAdapterID := "aa:bb:cc:dd:ee:03"
			interfaces := []Interface{
				{ID: "nm-device-old", Name: "eth0", AdapterID: original.DeviceInterface, Kind: KindEthernet, State: StateConnected, IPv4: []string{original.ApplianceAddress.String()}, Controllable: true},
				{ID: "nm-local", Name: "wlan0", AdapterID: original.LocalInterface, Kind: KindWiFi, State: StateConnected, IPv4: []string{"192.168.1.103/24"}, Controllable: true},
				{ID: "nm-device-new", Name: "eth1", AdapterID: newAdapterID, Kind: KindEthernet, State: StateDisconnected, Controllable: true},
			}
			deviceStore := newMemoryDeviceNetworkStore()
			if _, err := deviceStore.Save(ctx, original, time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)); err != nil {
				t.Fatal(err)
			}
			operationStore := &operationStoreSpy{}
			first := newApplianceHarness(t)
			first.base.interfaces = interfaces
			first.appliance.UseForwarder(applianceForwarderSpy{})
			if err := first.appliance.UseDeviceNetworkStore(ctx, deviceStore); err != nil {
				t.Fatal(err)
			}
			manager, err := NewOperationManager(OperationManagerOptions{
				Changer: first.appliance, Observer: first.appliance,
				Now: func() time.Time { return first.now }, NewID: sequentialIDs(),
				Store: operationStore, Audit: first.journal, Recovery: first.appliance,
			})
			if err != nil {
				t.Fatal(err)
			}
			first.manager = manager
			change := Change{Action: test.action, Target: original.ID}
			if test.action == ActionDeviceNetworkUpdate {
				edited := original
				edited.DeviceInterface = newAdapterID
				change.DeviceNetwork = &edited
			}
			pending := first.run(t, change)
			if pending.State != StateAwaitingConfirmation {
				t.Fatalf("initial operation=%+v", pending)
			}

			second := newApplianceHarness(t)
			second.base.interfaces = interfaces
			second.base.recovery = RecoveryOutcome{OperationID: pending.ID, Phase: test.phase}
			second.base.recovered = true
			second.appliance.UseForwarder(applianceForwarderSpy{})
			if err := second.appliance.UseDeviceNetworkStore(ctx, deviceStore); err != nil {
				t.Fatal(err)
			}
			restarted, err := NewOperationManager(OperationManagerOptions{
				Changer: second.appliance, Observer: second.appliance,
				Now: func() time.Time { return second.now }, NewID: sequentialIDs(),
				Store: operationStore, Audit: second.journal, Recovery: second.appliance,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := restarted.Adopt(ctx); err != nil {
				t.Fatal(err)
			}
			if err := restarted.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			adopted, err := restarted.Get(pending.ID)
			if err != nil || adopted.State != test.wantState {
				t.Fatalf("adopted state=%+v error=%v, want %s", adopted, err, test.wantState)
			}
			records, err := deviceStore.List(ctx)
			if err != nil || len(records) != test.wantCount {
				t.Fatalf("records=%+v error=%v, want count %d", records, err, test.wantCount)
			}
			if test.wantCount == 1 {
				wantAdapter := original.DeviceInterface
				if test.wantNew {
					wantAdapter = newAdapterID
				}
				if records[0].DeviceInterface != wantAdapter {
					t.Fatalf("stored adapter=%q, want %q", records[0].DeviceInterface, wantAdapter)
				}
			}
			for _, claim := range []struct {
				adapter string
				blocked bool
			}{{original.DeviceInterface, test.wantOld}, {newAdapterID, test.wantNew}} {
				err := second.appliance.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{AdapterID: claim.adapter, Role: AdapterRoleManagement}})
				if (err != nil) != claim.blocked {
					t.Fatalf("management check for %s error=%v, blocked want %t", claim.adapter, err, claim.blocked)
				}
			}
		})
	}
}

func TestApplianceRehydratesConfirmedDeviceNetworkCreateFromDurableInventory(t *testing.T) {
	ctx := context.Background()
	created := validHost(t)
	created.ID = ""
	created.StartEnabled = false
	interfaces := []Interface{
		{ID: "nm-device", Name: "eth1", AdapterID: created.DeviceInterface, Kind: KindEthernet, State: StateConnected, IPv4: []string{created.ApplianceAddress.String()}, Controllable: true},
		{ID: "nm-local", Name: "eth0", AdapterID: created.LocalInterface, Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.1.10/24"}, Controllable: true},
	}
	deviceStore := newMemoryDeviceNetworkStore()
	operationStore := &operationStoreSpy{}
	first := newApplianceHarness(t)
	first.base.interfaces = interfaces
	first.appliance.UseForwarder(applianceForwarderSpy{})
	if err := first.appliance.UseDeviceNetworkStore(ctx, deviceStore); err != nil {
		t.Fatal(err)
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: first.appliance, Observer: first.appliance,
		Now: func() time.Time { return first.now }, NewID: sequentialIDs(),
		Store: operationStore, Audit: first.journal, Recovery: first.appliance,
	})
	if err != nil {
		t.Fatal(err)
	}
	first.manager = manager
	operation := first.run(t, Change{
		Action: ActionDeviceNetworkCreate, Target: "created-after-restart", DeviceNetwork: &created,
	})
	if operation.State != StateConfirmed {
		t.Fatalf("create state=%s, want confirmed without a recovery window", operation.State)
	}
	latest := operationStore.latest()
	if len(latest) != 1 || latest[0].CheckpointID != "" {
		t.Fatalf("create operation persisted a helper checkpoint: %+v", latest)
	}
	metadata, present, err := decodeDeviceNetworkRecovery(Checkpoint{Metadata: latest[0].Parameters})
	if err != nil || !present || metadata.CreatedID == "" {
		t.Fatalf("create recovery metadata present=%t value=%+v error=%v", present, metadata, err)
	}
	records, err := deviceStore.List(ctx)
	if err != nil || len(records) != 1 || records[0].ID != metadata.CreatedID {
		t.Fatalf("created records=%+v error=%v, want durable id %q", records, err, metadata.CreatedID)
	}

	second := newApplianceHarness(t)
	second.base.interfaces = interfaces
	second.appliance.UseForwarder(applianceForwarderSpy{})
	if err := second.appliance.UseDeviceNetworkStore(ctx, deviceStore); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewOperationManager(OperationManagerOptions{
		Changer: second.appliance, Observer: second.appliance,
		Now: func() time.Time { return second.now }, NewID: sequentialIDs(),
		Store: operationStore, Audit: second.journal, Recovery: second.appliance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Adopt(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.appliance.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{
		AdapterID: created.DeviceInterface, Role: AdapterRoleManagement,
	}}); err == nil {
		t.Fatal("durable created device network did not reclaim its adapter role after restart")
	}
}

func TestAppliancePersistsDeviceRecoveryMetadataBeforeHelperPreparation(t *testing.T) {
	ctx := context.Background()
	original := validHost(t)
	original.ID = "device-network-1"
	edited := original
	edited.DeviceInterface = "aa:bb:cc:dd:ee:03"
	deviceStore := newMemoryDeviceNetworkStore()
	if _, err := deviceStore.Save(ctx, original, time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	firstSave := make(chan OperationRecord, 1)
	releaseFirstSave := make(chan struct{})
	operationStore := &interceptOperationStore{
		delegate: &operationStoreSpy{},
		beforeReturn: func(_ context.Context, call int, record OperationRecord) error {
			if call == 1 {
				firstSave <- record
				<-releaseFirstSave
			}
			return nil
		},
	}
	harness := newApplianceHarness(t)
	harness.base.interfaces = []Interface{
		{ID: "nm-device-old", Name: "eth0", AdapterID: original.DeviceInterface, Kind: KindEthernet, State: StateConnected, IPv4: []string{original.ApplianceAddress.String()}, Controllable: true},
		{ID: "nm-device-new", Name: "eth1", AdapterID: edited.DeviceInterface, Kind: KindEthernet, State: StateDisconnected, Controllable: true},
		{ID: "nm-local", Name: "wlan0", AdapterID: original.LocalInterface, Kind: KindWiFi, State: StateConnected, IPv4: []string{"192.168.1.103/24"}, Controllable: true},
	}
	harness.appliance.UseForwarder(applianceForwarderSpy{})
	if err := harness.appliance.UseDeviceNetworkStore(ctx, deviceStore); err != nil {
		t.Fatal(err)
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: harness.appliance, Observer: harness.appliance,
		Now: func() time.Time { return harness.now }, NewID: sequentialIDs(),
		Store: operationStore, Audit: harness.journal, Recovery: harness.appliance,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := manager.PreviewChange(ctx, firstActor, Change{
		Action: ActionDeviceNetworkUpdate, Target: original.ID, DeviceNetwork: &edited,
	})
	if err != nil {
		t.Fatal(err)
	}
	acknowledged := make(chan error, 1)
	go func() {
		_, acknowledgeErr := manager.Acknowledge(ctx, firstActor, preview.ID, preview.Revision)
		acknowledged <- acknowledgeErr
	}()
	record := <-firstSave
	helperWasArmed := len(harness.base.armed) != 0
	close(releaseFirstSave)
	if err := <-acknowledged; err != nil {
		t.Fatal(err)
	}
	if helperWasArmed {
		t.Fatal("helper recovery was prepared before the first durable operation record")
	}
	if record.CheckpointID != "" {
		t.Fatalf("first record checkpoint=%q, want no helper checkpoint yet", record.CheckpointID)
	}
	metadata, present, err := decodeDeviceNetworkRecovery(Checkpoint{Metadata: record.Parameters})
	if err != nil || !present {
		t.Fatalf("first record recovery metadata present=%t error=%v", present, err)
	}
	if len(metadata.Snapshot) != 1 || metadata.Snapshot[0].ID != original.ID {
		t.Fatalf("first record snapshot=%+v, want the pre-change device inventory", metadata.Snapshot)
	}
}

func TestApplianceRestoresPreparedDeviceRecoveryWhenCheckpointPersistenceIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	original := validHost(t)
	original.ID = "device-network-1"
	edited := original
	edited.DeviceInterface = "aa:bb:cc:dd:ee:03"
	deviceStore := newMemoryDeviceNetworkStore()
	if _, err := deviceStore.Save(ctx, original, time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	secondSave := make(chan struct{})
	operationStore := &interceptOperationStore{
		delegate: &operationStoreSpy{},
		beforeReturn: func(saveCtx context.Context, call int, _ OperationRecord) error {
			if call != 2 {
				return nil
			}
			close(secondSave)
			<-saveCtx.Done()
			return saveCtx.Err()
		},
	}
	harness := newApplianceHarness(t)
	harness.base.interfaces = []Interface{
		{ID: "nm-device-old", Name: "eth0", AdapterID: original.DeviceInterface, Kind: KindEthernet, State: StateConnected, IPv4: []string{original.ApplianceAddress.String()}, Controllable: true},
		{ID: "nm-device-new", Name: "eth1", AdapterID: edited.DeviceInterface, Kind: KindEthernet, State: StateDisconnected, Controllable: true},
		{ID: "nm-local", Name: "wlan0", AdapterID: original.LocalInterface, Kind: KindWiFi, State: StateConnected, IPv4: []string{"192.168.1.103/24"}, Controllable: true},
	}
	harness.appliance.UseForwarder(applianceForwarderSpy{})
	if err := harness.appliance.UseDeviceNetworkStore(ctx, deviceStore); err != nil {
		t.Fatal(err)
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: harness.appliance, Observer: harness.appliance,
		Now: func() time.Time { return harness.now }, NewID: sequentialIDs(),
		Store: operationStore, Audit: harness.journal, Recovery: harness.appliance,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := manager.PreviewChange(ctx, firstActor, Change{
		Action: ActionDeviceNetworkUpdate, Target: original.ID, DeviceNetwork: &edited,
	})
	if err != nil {
		t.Fatal(err)
	}
	acknowledged := make(chan error, 1)
	go func() {
		_, acknowledgeErr := manager.Acknowledge(ctx, firstActor, preview.ID, preview.Revision)
		acknowledged <- acknowledgeErr
	}()
	<-secondSave
	cancel()
	if err := <-acknowledged; err == nil {
		t.Fatal("acknowledge succeeded after checkpoint persistence was cancelled")
	}
	if len(harness.base.restored) != 1 || len(harness.base.discarded) != 0 {
		t.Fatalf("prepared helper cleanup restored=%d discarded=%d, want one restore and no discard", len(harness.base.restored), len(harness.base.discarded))
	}
	if harness.base.restoreCtx[0] != nil {
		t.Fatalf("prepared helper restore inherited cancelled context: %v", harness.base.restoreCtx[0])
	}
	latest := operationStore.delegate.latest()
	if len(latest) != 1 || latest[0].State != string(StateFailed) {
		t.Fatalf("latest operation record=%+v, want a terminal failed record", latest)
	}
	for _, adapterID := range []string{original.DeviceInterface, edited.DeviceInterface} {
		if err := harness.appliance.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{
			AdapterID: adapterID, Role: AdapterRoleManagement,
		}}); err != nil && adapterID == edited.DeviceInterface {
			t.Fatalf("proposed adapter remained reserved after successful cleanup: %v", err)
		}
	}
}

func TestApplianceReconcilesInterruptedDeviceNetworkCreateWithoutRecoveryMetadata(t *testing.T) {
	for _, test := range []struct {
		name      string
		persisted bool
		wantState OperationState
	}{
		{name: "before durable create", persisted: false, wantState: StateFailed},
		{name: "after durable create", persisted: true, wantState: StateConfirmed},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			created := validHost(t)
			created.ID = "device-network-created"
			created.StartEnabled = false
			deviceStore := newMemoryDeviceNetworkStore()
			if test.persisted {
				if _, err := deviceStore.Save(ctx, created, time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)); err != nil {
					t.Fatal(err)
				}
			}
			operationStore := &operationStoreSpy{}
			now := time.Date(2026, 9, 12, 1, 0, 0, 0, time.UTC)
			if err := operationStore.Save(ctx, OperationRecord{
				ID: "interrupted-create", Action: string(ActionDeviceNetworkCreate),
				Target: created.ID, ActorID: firstActor, State: string(StateApplying),
				Revision: "accepted", CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			harness := newApplianceHarness(t)
			harness.appliance.UseForwarder(applianceForwarderSpy{})
			if err := harness.appliance.UseDeviceNetworkStore(ctx, deviceStore); err != nil {
				t.Fatal(err)
			}
			manager, err := NewOperationManager(OperationManagerOptions{
				Changer: harness.appliance, Observer: harness.appliance,
				Now: func() time.Time { return harness.now }, NewID: sequentialIDs(),
				Store: operationStore, Audit: harness.journal, Recovery: harness.appliance,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.Adopt(ctx); err != nil {
				t.Fatal(err)
			}
			if err := manager.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			operation, err := manager.Get("interrupted-create")
			if err != nil || operation.State != test.wantState {
				t.Fatalf("reconciled operation=%+v error=%v, want state %s", operation, err, test.wantState)
			}
			claimErr := harness.appliance.AdapterRoleCoordinator().Check([]AdapterRoleClaim{{
				AdapterID: created.DeviceInterface, Role: AdapterRoleManagement,
			}})
			if (claimErr != nil) != test.persisted {
				t.Fatalf("management claim check error=%v, blocked want %t", claimErr, test.persisted)
			}
		})
	}
}

func TestApplianceRefusesRestartWithoutDeviceNetworkRecoveryMetadata(t *testing.T) {
	ctx := context.Background()
	deviceStore := newMemoryDeviceNetworkStore()
	operationStore := &operationStoreSpy{}
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	if err := operationStore.Save(ctx, OperationRecord{
		ID: "unfinished-device-change", Action: string(ActionDeviceNetworkDelete),
		Target: "device-network-1", ActorID: firstActor, State: string(StateAwaitingConfirmation),
		Revision: "accepted", Parameters: json.RawMessage(`{}`), CheckpointID: "checkpoint-legacy",
		CreatedAt: now, UpdatedAt: now, ConfirmDeadline: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	harness := newApplianceHarness(t)
	harness.appliance.UseForwarder(applianceForwarderSpy{})
	if err := harness.appliance.UseDeviceNetworkStore(ctx, deviceStore); err != nil {
		t.Fatal(err)
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: harness.appliance, Observer: harness.appliance,
		Now: func() time.Time { return harness.now }, NewID: sequentialIDs(),
		Store: operationStore, Audit: harness.journal, Recovery: harness.appliance,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Adopt(ctx); err == nil || !strings.Contains(err.Error(), "recovery metadata is missing") {
		t.Fatalf("Adopt() error=%v, want fail-closed missing-metadata refusal", err)
	}
}

func (harness *applianceHarness) run(t *testing.T, change Change) Operation {
	t.Helper()
	ctx := context.Background()
	previewed, err := harness.manager.PreviewChange(ctx, firstActor, change)
	if err != nil {
		t.Fatalf("preview %s on %s: %v", change.Action, change.Target, err)
	}
	applied, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision)
	if err != nil {
		t.Fatalf("acknowledge %s on %s: %v", change.Action, change.Target, err)
	}
	return applied
}

func (harness *applianceHarness) importProfile(t *testing.T, name string) ProfileRecord {
	t.Helper()
	harness.run(t, Change{Action: ActionProfileImport, Target: name, ProfileText: importedTunnel(name)})
	records, _ := harness.store.List(context.Background())
	for _, record := range records {
		if record.Name == name {
			return record
		}
	}
	t.Fatalf("profile %s was not saved", name)
	return ProfileRecord{}
}

func importedTunnel(name string) string {
	return "[Interface]\nPrivateKey = " + disposableLabelKey("SENTINELprivate"+name) + "\nAddress = 10.60.0.2/32\nDNS = 10.60.0.1\n" +
		"[Peer]\nPublicKey = " + disposableLabelKey("SENTINELpeer"+name) + "\nAllowedIPs = 10.60.0.0/16\nEndpoint = hub.example:51820\n"
}

// disposableLabelKey builds a well-formed 32-byte base64 key carrying a
// readable label. Only base64 letters survive from the label, because the
// parser checks the encoding and a hyphen in a name would make it refuse.
func disposableLabelKey(label string) string {
	const encodedLength = 43
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, label)
	if len(clean) > encodedLength {
		clean = clean[:encodedLength]
	}
	return clean + strings.Repeat("A", encodedLength-len(clean)) + "="
}

// TestImportBindsTheStagedKeyThroughTheHelperAndSavesAnInactiveProfile covers
// REQ-FUNC-faf2f697 and REQ-FUNC-0f6ac82d on the live path: the preview stages
// the key, the import hands the helper the handle together with the public
// configuration, and what is saved afterwards is public metadata that references
// the created connection and is neither active nor selected for startup.
func TestImportBindsTheStagedKeyThroughTheHelperAndSavesAnInactiveProfile(t *testing.T) {
	t.Parallel()
	harness := newApplianceHarness(t)
	record := harness.importProfile(t, "office")

	if len(harness.driver.created) != 1 {
		t.Fatalf("the helper was asked to create %d tunnels, want 1", len(harness.driver.created))
	}
	created := harness.driver.created[0]
	if created.handle == "" || created.handle != harness.base.staged[0] {
		t.Fatalf("the helper received handle %q, want the staged handle %q", created.handle, harness.base.staged[0])
	}
	if created.record.ID != record.ID || created.record.Name != "office" {
		t.Fatalf("the helper created %+v, want the saved record", created.record)
	}
	if len(created.record.Profile.Peers) != 1 || created.record.Profile.Peers[0].Endpoint != "hub.example:51820" {
		t.Fatalf("the helper did not receive the public configuration: %+v", created.record.Profile)
	}
	// Binding belongs to the create step, which the helper performs atomically
	// with the connection; the manager must not bind a second time.
	if len(harness.base.bound) != 0 || len(harness.base.dropped) != 0 {
		t.Fatalf("the manager bound %v and dropped %v, want the helper to own both", harness.base.bound, harness.base.dropped)
	}
	if record.ConnectionRef != "uuid-office" || record.SecretRef != record.ID {
		t.Fatalf("saved record references = %q/%q, want the created connection and the profile identity", record.ConnectionRef, record.SecretRef)
	}
	if record.StartSelected {
		t.Fatal("an import selected itself for startup")
	}
	status, err := harness.appliance.WireGuard(context.Background())
	if err != nil {
		t.Fatalf("WireGuard: %v", err)
	}
	if status.ReadinessOnly {
		t.Fatal("a live appliance with an inventory still reported readiness only")
	}
	if len(status.Profiles) != 1 || status.Profiles[0].Active || status.Profiles[0].AutoConnect || !status.Profiles[0].NILOOwned {
		t.Fatalf("inventory after import = %+v, want one inactive NILO-owned profile", status.Profiles)
	}
	if status.Profiles[0].ID != record.ID || status.Profiles[0].Name != "office" {
		t.Fatalf("inventory identity = %s/%s, want %s/office", status.Profiles[0].ID, status.Profiles[0].Name, record.ID)
	}
}

// TestAFailedTunnelCreationDiscardsTheStagedKeyAndSavesNothing is the failure
// half: nothing is saved and no key is left waiting.
func TestAFailedTunnelCreationDiscardsTheStagedKeyAndSavesNothing(t *testing.T) {
	t.Parallel()
	harness := newApplianceHarness(t)
	harness.driver.createErr = errors.New("NetworkManager refused the connection")
	ctx := context.Background()
	previewed, err := harness.manager.PreviewChange(ctx, firstActor, Change{
		Action: ActionProfileImport, Target: "office", ProfileText: importedTunnel("office"),
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision); err == nil {
		t.Fatal("a failed creation was reported as an import")
	}
	if records, _ := harness.store.List(ctx); len(records) != 0 {
		t.Fatalf("%d profiles saved after a failed creation, want none", len(records))
	}
	if len(harness.base.dropped) != 1 || harness.base.dropped[0] != harness.base.staged[0] {
		t.Fatalf("dropped %v, want the staged handle %v", harness.base.dropped, harness.base.staged)
	}
}

// TestActivationResolvesTheConnectionAndPreviewsTheSwitch covers
// REQ-FUNC-a1584be7 and REQ-FUNC-9e05d184: selecting B while A is active is
// one previewed switch whose preview names what it affects, and the change the
// helper is asked to arm carries the connection reference, not the label.
func TestActivationResolvesTheConnectionAndPreviewsTheSwitch(t *testing.T) {
	t.Parallel()
	harness := newApplianceHarness(t)
	ctx := context.Background()
	office := harness.importProfile(t, "office")
	siteB := harness.importProfile(t, "site-b")
	harness.driver.setActive(office.ConnectionRef)

	observed, err := harness.appliance.Inspect(ctx, Change{Action: ActionVPNActivate, Target: siteB.ID})
	if err != nil {
		t.Fatalf("inspect switch: %v", err)
	}
	if observed["active_profile"] != office.ID {
		t.Fatalf("preview active_profile = %q, want %q", observed["active_profile"], office.ID)
	}
	if observed["routes"] != "10.60.0.0/16" || observed["dns"] != "10.60.0.1" {
		t.Fatalf("preview routes/dns = %q/%q, want the imported intent", observed["routes"], observed["dns"])
	}
	if observed["management_path"] == "" {
		t.Fatal("the preview does not say which management path stays in place")
	}

	applied := harness.run(t, Change{Action: ActionVPNActivate, Target: siteB.ID})
	if applied.State != StateAwaitingConfirmation {
		t.Fatalf("switch state = %q, want awaiting_confirmation", applied.State)
	}
	if len(harness.base.armed) != 1 || harness.base.armed[0].ConnectionID != siteB.ConnectionRef {
		t.Fatalf("armed %+v, want one change carrying %s", harness.base.armed, siteB.ConnectionRef)
	}
	if harness.base.armed[0].Target != siteB.ID || harness.base.armed[0].Action != ActionVPNActivate {
		t.Fatalf("armed change = %+v, want the activation of %s", harness.base.armed[0], siteB.ID)
	}
	if len(harness.base.applied) != 1 || harness.base.applied[0].ConnectionID != siteB.ConnectionRef {
		t.Fatalf("applied %+v, want the resolved switch", harness.base.applied)
	}

	// Deactivating "active" resolves to whichever NILO-owned tunnel is up.
	harness.driver.setActive(siteB.ConnectionRef)
	if _, err := harness.manager.Confirm(ctx, firstActor, applied.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	harness.run(t, Change{Action: ActionVPNDeactivate, Target: "active"})
	if last := harness.base.armed[len(harness.base.armed)-1]; last.ConnectionID != siteB.ConnectionRef || last.Action != ActionVPNDeactivate {
		t.Fatalf("deactivation armed %+v, want %s", last, siteB.ConnectionRef)
	}
}

// TestRemovalRefusesAnActiveOrReferencedProfileAndDeletesOnlyAnInactiveOne
// covers REQ-FUNC-090be3ae and the dependency half of REQ-FUNC-0f6ac82d.
func TestRemovalRefusesAnActiveOrReferencedProfileAndDeletesOnlyAnInactiveOne(t *testing.T) {
	t.Parallel()
	harness := newApplianceHarness(t)
	ctx := context.Background()
	office := harness.importProfile(t, "office")
	siteB := harness.importProfile(t, "site-b")
	harness.driver.setActive(office.ConnectionRef)

	if _, err := harness.appliance.Inspect(ctx, Change{Action: ActionProfileRemove, Target: office.ID}); !errors.Is(err, ErrProfileActive) {
		t.Fatalf("removing the active profile: %v, want ErrProfileActive", err)
	}
	harness.appliance.profiles.options.Dependencies = func(_ context.Context, profileID string) ([]ProfileDependency, error) {
		if profileID == siteB.ID {
			return []ProfileDependency{{Kind: "device network", ID: "dn-1", Name: "Line 3"}}, nil
		}
		return nil, nil
	}
	_, err := harness.appliance.Inspect(ctx, Change{Action: ActionProfileRemove, Target: siteB.ID})
	if !errors.Is(err, ErrProfileReferenced) || !strings.Contains(err.Error(), "Line 3") {
		t.Fatalf("removing a referenced profile: %v, want ErrProfileReferenced naming Line 3", err)
	}
	harness.appliance.profiles.options.Dependencies = nil
	harness.run(t, Change{Action: ActionProfileRemove, Target: siteB.ID})
	if len(harness.driver.deleted) != 1 || harness.driver.deleted[0] != siteB.ConnectionRef {
		t.Fatalf("deleted %v, want %s", harness.driver.deleted, siteB.ConnectionRef)
	}
	records, _ := harness.store.List(ctx)
	if len(records) != 1 || records[0].ID != office.ID {
		t.Fatalf("records after removal = %+v, want only office", records)
	}
}

// TestRenameKeepsTheIdentityTheKeyAndTheConnection covers REQ-FUNC-72e0dc95.
func TestRenameKeepsTheIdentityTheKeyAndTheConnection(t *testing.T) {
	t.Parallel()
	harness := newApplianceHarness(t)
	ctx := context.Background()
	office := harness.importProfile(t, "office")
	harness.importProfile(t, "site-b")

	if _, err := harness.appliance.Inspect(ctx, Change{Action: ActionProfileRename, Target: office.ID, Name: "site-b"}); !errors.Is(err, ErrProfileNameTaken) {
		t.Fatalf("renaming onto a taken name: %v, want ErrProfileNameTaken", err)
	}
	harness.run(t, Change{Action: ActionProfileRename, Target: office.ID, Name: "Head office"})
	records, _ := harness.store.List(ctx)
	var renamed ProfileRecord
	for _, record := range records {
		if record.ID == office.ID {
			renamed = record
		}
	}
	if renamed.Name != "Head office" || renamed.ConnectionRef != office.ConnectionRef || renamed.SecretRef != office.SecretRef {
		t.Fatalf("renamed record = %+v, want the same identity, key reference and connection", renamed)
	}
	if renamed.Revision == office.Revision {
		t.Fatal("an accepted edit did not change the revision")
	}
	if len(harness.driver.created) != 2 || len(harness.driver.deleted) != 0 {
		t.Fatal("a rename touched the helper's tunnels")
	}
}

// TestExternalTunnelsAreListedAndRefused covers REQ-FUNC-1a417267 on the live
// path: a tunnel NILO did not create is visible, marked external, and never
// acted on, and the single active tunnel rule only counts NILO's own.
func TestExternalTunnelsAreListedAndRefused(t *testing.T) {
	t.Parallel()
	harness := newApplianceHarness(t)
	ctx := context.Background()
	harness.driver.tunnels = append(harness.driver.tunnels, TunnelObservation{
		ConnectionRef: "external-uuid", Name: "wg0", Interface: "wg0", Active: true, NILOOwned: false,
		Profile: Profile{Addresses: []string{"10.205.148.4/32"}, Peers: []ProfilePeer{{Endpoint: "hub.example.org:51820", AllowedIPs: []string{"0.0.0.0/0"}}}},
	})
	status, err := harness.appliance.WireGuard(ctx)
	if err != nil {
		t.Fatalf("WireGuard: %v", err)
	}
	if len(status.Profiles) != 1 || status.Profiles[0].NILOOwned || !status.Profiles[0].Active || status.Profiles[0].ID != "external-uuid" {
		t.Fatalf("external inventory = %+v, want one active externally managed entry", status.Profiles)
	}
	for _, action := range []Action{ActionVPNActivate, ActionVPNDeactivate, ActionProfileRemove, ActionAutostartSelect, ActionProfileRename} {
		if _, err := harness.appliance.Inspect(ctx, Change{Action: action, Target: "external-uuid", Name: "x"}); !errors.Is(err, ErrExternalProfile) {
			t.Fatalf("%s on an external tunnel: %v, want ErrExternalProfile", action, err)
		}
	}
	if _, err := harness.appliance.Inspect(ctx, Change{Action: ActionVPNDeactivate, Target: "active"}); !errors.Is(err, ErrExternalProfile) {
		t.Fatalf("deactivating \"active\" with only an external tunnel up: %v, want ErrExternalProfile", err)
	}
	if len(harness.base.armed) != 0 || len(harness.driver.deleted) != 0 {
		t.Fatal("an external tunnel was touched")
	}
}

// TestStartupConnectsTheSelectedProfileOnceAndRecordsItAsTheSystem covers
// REQ-FUNC-16e918cb on the live path and its audit: the selected profile is
// connected when the runtime starts, attributed to the system actor, and not
// connected a second time once it is up.
func TestStartupConnectsTheSelectedProfileOnceAndRecordsItAsTheSystem(t *testing.T) {
	t.Parallel()
	harness := newApplianceHarness(t)
	ctx := context.Background()
	office := harness.importProfile(t, "office")
	harness.importProfile(t, "site-b")

	startup := NewStartup(StartupOptions{Connector: harness.appliance, Audit: harness.journal})
	if done := startup.Attempt(ctx); !done {
		t.Fatal("with nothing selected, startup was not finished")
	}
	if len(harness.driver.connected) != 0 {
		t.Fatalf("connected %v with no selection", harness.driver.connected)
	}

	harness.run(t, Change{Action: ActionAutostartSelect, Target: office.ID, Enabled: true})
	startup = NewStartup(StartupOptions{Connector: harness.appliance, Audit: harness.journal})
	if done := startup.Attempt(ctx); !done {
		t.Fatal("startup did not finish after connecting the selection")
	}
	if len(harness.driver.connected) != 1 || harness.driver.connected[0] != office.ConnectionRef {
		t.Fatalf("connected %v, want %s once", harness.driver.connected, office.ConnectionRef)
	}
	if done := startup.Attempt(ctx); !done || len(harness.driver.connected) != 1 {
		t.Fatalf("a second attempt connected again: %v", harness.driver.connected)
	}
	var startupRecords []audit.Record
	for _, record := range harness.journal.records {
		if record.Action == "vpn.startup" {
			startupRecords = append(startupRecords, record)
		}
	}
	if len(startupRecords) != 1 {
		t.Fatalf("recorded %d startup events, want 1", len(startupRecords))
	}
	if startupRecords[0].ActorKind != audit.ActorKindSystem || startupRecords[0].Outcome != audit.OutcomeApplied {
		t.Fatalf("startup record = %+v, want the system actor and an applied outcome", startupRecords[0])
	}
	if startupRecords[0].SubjectID != office.ID || startupRecords[0].TargetLabel != "office" {
		t.Fatalf("startup subject = %s/%s, want %s/office", startupRecords[0].SubjectID, startupRecords[0].TargetLabel, office.ID)
	}
	status, _ := harness.appliance.WireGuard(ctx)
	active := 0
	for _, profile := range status.Profiles {
		if profile.Active {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("%d active profiles after startup, want 1", active)
	}
}
