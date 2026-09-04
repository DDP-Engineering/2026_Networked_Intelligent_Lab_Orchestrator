package network

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
)

// operationStoreSpy is the durable half of the lifecycle, kept in memory so the
// order of writes is observable.
type operationStoreSpy struct {
	saved   []OperationRecord
	saveErr error
}

func (store *operationStoreSpy) Save(_ context.Context, record OperationRecord) error {
	if store.saveErr != nil {
		return store.saveErr
	}
	store.saved = append(store.saved, record)
	return nil
}

func (store *operationStoreSpy) Unfinished(context.Context) ([]OperationRecord, error) {
	unfinished := make([]OperationRecord, 0)
	latest := store.latest()
	for _, record := range latest {
		if Unfinished(record.State) {
			unfinished = append(unfinished, record)
		}
	}
	return unfinished, nil
}

func (store *operationStoreSpy) Recent(_ context.Context, _ int) ([]OperationRecord, error) {
	return store.latest(), nil
}

// latest collapses the write history to one entry per operation, which is what
// the SQLite repository stores.
func (store *operationStoreSpy) latest() []OperationRecord {
	byID := map[string]OperationRecord{}
	order := make([]string, 0, len(store.saved))
	for _, record := range store.saved {
		if _, seen := byID[record.ID]; !seen {
			order = append(order, record.ID)
		}
		byID[record.ID] = record
	}
	collapsed := make([]OperationRecord, 0, len(order))
	for _, id := range order {
		collapsed = append(collapsed, byID[id])
	}
	return collapsed
}

// recordingJournal is the audit half.
type recordingJournal struct {
	records []audit.Record
	err     error
	// failOutcome fails only that one stage, so a test can put the failure
	// exactly where it matters.
	failOutcome audit.Outcome
}

func (journal *recordingJournal) Record(_ context.Context, record audit.Record) error {
	if journal.err != nil && (journal.failOutcome == "" || journal.failOutcome == record.Outcome) {
		return journal.err
	}
	journal.records = append(journal.records, record)
	return nil
}

func (journal *recordingJournal) outcomes() []audit.Outcome {
	outcomes := make([]audit.Outcome, 0, len(journal.records))
	for _, record := range journal.records {
		outcomes = append(outcomes, record.Outcome)
	}
	return outcomes
}

// disposableKey builds a well-formed 32-byte base64 key that carries a readable
// label, so a test can search for the value it planted. Padding with 'A' keeps
// the final character canonical, which the parser insists on.
func disposableKey(t *testing.T, label string) string {
	t.Helper()
	const encodedLength = 43
	if len(label) > encodedLength {
		t.Fatalf("label %q is too long for a 32-byte key", label)
	}
	return label + strings.Repeat("A", encodedLength-len(label)) + "="
}

func newDurableHarness(t *testing.T) (*operationHarness, *operationStoreSpy, *recordingJournal) {
	t.Helper()
	store := &operationStoreSpy{}
	journal := &recordingJournal{}
	harness := &operationHarness{
		changer:  newChangerSpy(),
		observer: &observerStub{observed: map[string]string{"state": "connected"}},
		now:      time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC),
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer:  harness.changer,
		Observer: harness.observer,
		Now:      func() time.Time { return harness.now },
		NewID:    sequentialIDs(),
		Store:    store,
		Audit:    journal,
	})
	if err != nil {
		t.Fatalf("NewOperationManager: %v", err)
	}
	harness.manager = manager
	return harness, store, journal
}

// TestAnAcceptedChangeIsRecordedBeforeAnySideEffect covers REQ-DATA-91e7ab46
// and the write-first half of REQ-REL-2e91cb74: the appliance writes down what
// it agreed to do before it does any of it.
func TestAnAcceptedChangeIsRecordedBeforeAnySideEffect(t *testing.T) {
	t.Parallel()
	harness, store, journal := newDurableHarness(t)
	ctx := context.Background()

	previewed, err := harness.manager.Preview(ctx, firstActor, ActionInterfaceDisable, "eth1")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("a preview wrote %d operation records, want none", len(store.saved))
	}
	if len(journal.records) != 0 {
		t.Fatalf("a preview recorded %d audit events, want none", len(journal.records))
	}

	if _, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	if len(store.saved) == 0 {
		t.Fatal("an acknowledged change wrote no operation record")
	}
	first := store.saved[0]
	if first.ID != previewed.ID || first.ActorID != firstActor {
		t.Fatalf("first record = %+v, want the acknowledged operation and its actor", first)
	}
	if first.Action != string(ActionInterfaceDisable) || first.Target != "eth1" {
		t.Fatalf("first record action/target = %q/%q", first.Action, first.Target)
	}

	outcomes := journal.outcomes()
	if len(outcomes) < 2 || outcomes[0] != audit.OutcomeRequested {
		t.Fatalf("audit outcomes = %v, want requested first", outcomes)
	}
	if outcomes[len(outcomes)-1] != audit.OutcomeApplied {
		t.Fatalf("audit outcomes = %v, want applied last", outcomes)
	}
	requested := journal.records[0]
	if requested.Category != audit.CategoryAudit || requested.Component != "network.interfaces" {
		t.Fatalf("requested record category/component = %q/%q", requested.Category, requested.Component)
	}
	if requested.OperationID != previewed.ID || requested.ActorKind != audit.ActorKindUser {
		t.Fatalf("requested record correlation/actor = %q/%q", requested.OperationID, requested.ActorKind)
	}
	if requested.SubjectType != "interface" || requested.SubjectID != "eth1" {
		t.Fatalf("requested subject = %q/%q", requested.SubjectType, requested.SubjectID)
	}
}

// TestAChangeIsRefusedWhenItCannotBeRecorded covers REQ-REL-59e4f80c: an
// unrecordable mutation must not happen at all, rather than happen unrecorded.
func TestAChangeIsRefusedWhenItCannotBeRecorded(t *testing.T) {
	t.Parallel()
	harness, store, journal := newDurableHarness(t)
	ctx := context.Background()
	journal.err = errors.New("audit storage is full")
	journal.failOutcome = audit.OutcomeRequested

	previewed, err := harness.manager.Preview(ctx, firstActor, ActionInterfaceDisable, "eth1")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision); !errors.Is(err, ErrAuditUnavailable) {
		t.Fatalf("acknowledge error = %v, want ErrAuditUnavailable", err)
	}
	if len(harness.changer.armed) != 0 || len(harness.changer.applied) != 0 {
		t.Fatalf("armed %v and applied %v, want neither", harness.changer.armed, harness.changer.applied)
	}
	if len(store.saved) != 0 {
		t.Fatalf("stored %d records for a refused change, want none", len(store.saved))
	}
	current, err := harness.manager.Get(previewed.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if current.State != StatePreviewed {
		t.Fatalf("state = %q, want the operation left previewed", current.State)
	}
}

// TestRollbackIsStillPossibleWhenAuditIsUnavailable covers the second half of
// REQ-REL-59e4f80c: a full disk must not be able to keep a cut-off appliance
// cut off.
func TestRollbackIsStillPossibleWhenAuditIsUnavailable(t *testing.T) {
	t.Parallel()
	harness, _, journal := newDurableHarness(t)
	ctx := context.Background()

	previewed, err := harness.manager.Preview(ctx, firstActor, ActionInterfaceDisable, "eth1")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	// Storage fails only now, once the appliance is already changed.
	journal.err = errors.New("audit storage is full")
	rolled, err := harness.manager.Rollback(ctx, firstActor, previewed.ID)
	if err != nil {
		t.Fatalf("rollback with unavailable audit: %v", err)
	}
	if rolled.State != StateRolledBack {
		t.Fatalf("state = %q, want rolled_back", rolled.State)
	}
	if len(harness.changer.restored) != 1 {
		t.Fatalf("restored %v, want exactly one restoration", harness.changer.restored)
	}
}

// TestEachStageOfOneChangeIsRecordedSeparately covers REQ-DATA-91e7ab46's
// requirement that requested, applied and confirmed are distinct facts.
func TestEachStageOfOneChangeIsRecordedSeparately(t *testing.T) {
	t.Parallel()
	harness, store, journal := newDurableHarness(t)
	ctx := context.Background()

	previewed, _ := harness.manager.Preview(ctx, firstActor, ActionInterfaceDisable, "eth1")
	if _, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if _, err := harness.manager.Confirm(ctx, firstActor, previewed.ID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	want := []audit.Outcome{audit.OutcomeRequested, audit.OutcomeApplied, audit.OutcomeConfirmed}
	got := journal.outcomes()
	if len(got) != len(want) {
		t.Fatalf("audit outcomes = %v, want %v", got, want)
	}
	for index, outcome := range want {
		if got[index] != outcome {
			t.Fatalf("audit outcomes = %v, want %v", got, want)
		}
	}
	final := store.latest()
	if len(final) != 1 || final[0].State != string(StateConfirmed) {
		t.Fatalf("stored state = %+v, want one confirmed operation", final)
	}
	if !final[0].ConfirmDeadline.IsZero() {
		t.Fatal("a confirmed operation still carries a countdown")
	}
}

// TestAnExpiryIsRecordedAsTheApplianceActing covers the system-actor half of
// REQ-DATA-91e7ab46: nobody asked for the timeout, so it is not attributed to
// the administrator who started the change.
func TestAnExpiryIsRecordedAsTheApplianceActing(t *testing.T) {
	t.Parallel()
	harness, _, journal := newDurableHarness(t)
	ctx := context.Background()

	previewed, _ := harness.manager.Preview(ctx, firstActor, ActionInterfaceDisable, "eth1")
	if _, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	harness.now = harness.now.Add(ConfirmationWindow + time.Second)
	harness.manager.ExpireDue(ctx)

	last := journal.records[len(journal.records)-1]
	if last.Outcome != audit.OutcomeRecovered {
		t.Fatalf("last outcome = %q, want recovered", last.Outcome)
	}
	if last.ActorKind != audit.ActorKindSystem {
		t.Fatalf("last actor kind = %q, want system", last.ActorKind)
	}
	if last.OperationID != previewed.ID {
		t.Fatalf("last correlation = %q, want %q", last.OperationID, previewed.ID)
	}
}

// TestAPendingChangeIsFoundAgainAfterARestart covers REQ-FUNC-97c1e5d8 and the
// reattachment half of REQ-REL-2e91cb74.
func TestAPendingChangeIsFoundAgainAfterARestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	first, store, _ := newDurableHarness(t)

	previewed, _ := first.manager.Preview(ctx, firstActor, ActionInterfaceDisable, "eth1")
	if _, err := first.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	// A second manager over the same store is what a restarted appliance has.
	restarted, err := NewOperationManager(OperationManagerOptions{
		Changer:  newChangerSpy(),
		Observer: &observerStub{observed: map[string]string{"state": "connected"}},
		Now:      func() time.Time { return first.now },
		NewID:    sequentialIDs(),
		Store:    store,
		Audit:    &recordingJournal{},
	})
	if err != nil {
		t.Fatalf("NewOperationManager: %v", err)
	}
	if _, err := restarted.Get(previewed.ID); !errors.Is(err, ErrOperationNotFound) {
		t.Fatalf("a fresh manager already knows the operation: %v", err)
	}
	if err := restarted.Adopt(ctx); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	adopted, err := restarted.Get(previewed.ID)
	if err != nil {
		t.Fatalf("get after adopt: %v", err)
	}
	if adopted.State != StateAwaitingConfirmation {
		t.Fatalf("adopted state = %q, want awaiting_confirmation", adopted.State)
	}
	if adopted.ActorID != firstActor {
		t.Fatalf("adopted actor = %q, want %q", adopted.ActorID, firstActor)
	}
	if adopted.ConfirmDeadline.IsZero() {
		t.Fatal("adopted operation lost its deadline, so nothing would ever restore it")
	}

	listed, err := restarted.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != previewed.ID {
		t.Fatalf("recent = %+v, want the pending operation", listed)
	}

	// The adopted window still expires, which is the whole point of finding it
	// again: an appliance that restarted mid-change still puts itself back. The
	// deadline came from the record, so the clock has to pass the original
	// deadline rather than a fresh window opened at adoption.
	first.now = adopted.ConfirmDeadline.Add(time.Second)
	restarted.ExpireDue(ctx)
	resolved, err := restarted.Get(previewed.ID)
	if err != nil {
		t.Fatalf("get after expiry: %v", err)
	}
	if resolved.State != StateRolledBack {
		t.Fatalf("state after expiry = %q, want rolled_back", resolved.State)
	}
}

// TestAnOperationRecordCarriesNoSecret covers the storage half of
// REQ-SEC-c62481fe at the lifecycle boundary rather than only at the repository.
func TestAnOperationRecordCarriesNoSecret(t *testing.T) {
	t.Parallel()
	harness, store, _ := newDurableHarness(t)
	ctx := context.Background()
	// The parser checks the key encoding, so a placeholder that is not valid
	// base64 would be refused before this test could observe where the value
	// ends up. disposableKey builds one that is.
	secret := disposableKey(t, "SENTINELrecorded")

	previewed, err := harness.manager.PreviewChange(ctx, firstActor, Change{
		Action: ActionProfileImport, Target: "office",
		ProfileText: "[Interface]\nPrivateKey = " + secret + "\nAddress = 10.60.0.2/32\n" +
			"[Peer]\nPublicKey = " + disposableKey(t, "SENTINELrecordpeer") + "\n" +
			"AllowedIPs = 10.60.0.0/16\n",
	})
	if err != nil {
		t.Fatalf("preview import: %v", err)
	}

	// The preview consumed the upload. Nothing downstream may still hold it.
	held, err := harness.manager.Get(previewed.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = held
	harness.manager.mutex.Lock()
	retained := harness.manager.operations[previewed.ID].change
	harness.manager.mutex.Unlock()
	if retained.ProfileText != "" {
		t.Fatal("the previewed operation still holds the uploaded profile text")
	}
	if retained.Profile == nil || len(retained.Profile.Peers) != 1 {
		t.Fatalf("the preview did not keep the public configuration: %+v", retained.Profile)
	}
	if _, err := harness.manager.Acknowledge(ctx, firstActor, previewed.ID, previewed.Revision); err != nil {
		t.Fatalf("acknowledge import: %v", err)
	}
	for _, record := range store.saved {
		if strings.Contains(string(record.Parameters), secret) {
			t.Fatalf("stored parameters carry the private key: %s", record.Parameters)
		}
		if strings.Contains(record.Target+record.Revision+record.Failure, secret) {
			t.Fatal("a stored operation field carries the private key")
		}
	}
}
