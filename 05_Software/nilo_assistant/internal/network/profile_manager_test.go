package network

import (
	"context"
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
	interfaces []Interface
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
func (spy *baseSpy) Restore(context.Context, Checkpoint) (bool, error) { return true, nil }
func (spy *baseSpy) Discard(context.Context, Checkpoint) error         { return nil }
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
