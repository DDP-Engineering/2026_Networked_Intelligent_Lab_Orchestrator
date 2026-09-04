package network

import (
	"context"
	"errors"
	"testing"
	"time"
)

// changerSpy records every connectivity action taken, so a test can assert not
// only what the manager reported but what it actually did to the appliance.
type changerSpy struct {
	armed     []string
	applied   []string
	restored  []string
	discarded []string

	armErr     error
	applyErr   error
	restoreErr error
	// observedRestored is what Restore reports having observed afterwards.
	observedRestored bool
}

func newChangerSpy() *changerSpy { return &changerSpy{observedRestored: true} }

func (spy *changerSpy) ArmRecovery(_ context.Context, operationID string, _ time.Duration) (Checkpoint, error) {
	if spy.armErr != nil {
		return Checkpoint{}, spy.armErr
	}
	spy.armed = append(spy.armed, operationID)
	return Checkpoint{ID: "checkpoint-" + operationID}, nil
}

func (spy *changerSpy) Apply(_ context.Context, action Action, target string) error {
	if spy.applyErr != nil {
		return spy.applyErr
	}
	spy.applied = append(spy.applied, string(action)+":"+target)
	return nil
}

func (spy *changerSpy) Restore(_ context.Context, checkpoint Checkpoint) (bool, error) {
	if spy.restoreErr != nil {
		return false, spy.restoreErr
	}
	spy.restored = append(spy.restored, checkpoint.ID)
	return spy.observedRestored, nil
}

func (spy *changerSpy) Discard(_ context.Context, checkpoint Checkpoint) error {
	spy.discarded = append(spy.discarded, checkpoint.ID)
	return nil
}

// observerStub is the state a preview is computed against.
type observerStub struct{ observed map[string]string }

func (stub *observerStub) Observe(context.Context, Action, string) (map[string]string, error) {
	copied := make(map[string]string, len(stub.observed))
	for key, value := range stub.observed {
		copied[key] = value
	}
	return copied, nil
}

type operationHarness struct {
	manager  *OperationManager
	changer  *changerSpy
	observer *observerStub
	now      time.Time
}

func newOperationHarness(t *testing.T) *operationHarness {
	t.Helper()
	harness := &operationHarness{
		changer:  newChangerSpy(),
		observer: &observerStub{observed: map[string]string{"state": "connected", "connection": "wired-1"}},
		now:      time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC),
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer:  harness.changer,
		Observer: harness.observer,
		Now:      func() time.Time { return harness.now },
		NewID:    sequentialIDs(),
	})
	if err != nil {
		t.Fatalf("NewOperationManager: %v", err)
	}
	harness.manager = manager
	return harness
}

func sequentialIDs() func() string {
	counter := 0
	return func() string {
		counter++
		return "operation-" + string(rune('a'+counter-1))
	}
}

const (
	firstActor  = "actor-1"
	secondActor = "actor-2"
)

// A preview is a proposal, not a change. Nothing may reach the appliance until
// an administrator has seen exactly what will happen and said yes to it.
func TestPreviewChangesNothingAndReturnsAReproducibleRevision(t *testing.T) {
	t.Parallel()

	harness := newOperationHarness(t)
	preview, err := harness.manager.Preview(
		context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if preview.State != StatePreviewed {
		t.Fatalf("state = %q, want %q", preview.State, StatePreviewed)
	}
	if preview.Revision == "" {
		t.Fatal("preview carries no revision")
	}
	if len(harness.changer.armed) != 0 || len(harness.changer.applied) != 0 {
		t.Fatalf("preview touched the appliance: armed=%v applied=%v",
			harness.changer.armed, harness.changer.applied)
	}

	// The same proposal against the same observed state digests identically.
	again, err := harness.manager.Preview(
		context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	if err != nil {
		t.Fatalf("second Preview: %v", err)
	}
	if again.Revision != preview.Revision {
		t.Fatalf("revision is not reproducible: %q then %q", preview.Revision, again.Revision)
	}
	if again.ID == preview.ID {
		t.Fatal("two previews share one operation identity")
	}
}

func TestAcknowledgementRunsTheChangeExactlyOnce(t *testing.T) {
	t.Parallel()

	harness := newOperationHarness(t)
	preview, err := harness.manager.Preview(
		context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	running, err := harness.manager.Acknowledge(
		context.Background(), firstActor, preview.ID, preview.Revision)
	if err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}
	if running.State != StateAwaitingConfirmation {
		t.Fatalf("state = %q, want %q", running.State, StateAwaitingConfirmation)
	}
	if len(harness.changer.applied) != 1 {
		t.Fatalf("applied %v, want exactly one change", harness.changer.applied)
	}
	if len(harness.changer.armed) != 1 {
		t.Fatalf("armed %v, want exactly one recovery checkpoint", harness.changer.armed)
	}

	// Replaying the acknowledgement must not run it a second time.
	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, preview.ID, preview.Revision); !errors.Is(err, ErrAlreadyAcknowledged) {
		t.Fatalf("replayed acknowledgement error = %v, want ErrAlreadyAcknowledged", err)
	}
	if len(harness.changer.applied) != 1 {
		t.Fatalf("a replayed acknowledgement applied the change again: %v", harness.changer.applied)
	}
}

func TestAcknowledgementIsRefusedUnlessItMatchesWhatWasShown(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		corrupt func(*operationHarness, *Operation) (actor, revision string)
		want    error
	}{
		{
			name: "a revision that was never shown",
			corrupt: func(_ *operationHarness, preview *Operation) (string, string) {
				return firstActor, "0000000000000000000000000000000000000000000000000000000000000000"
			},
			want: ErrRevisionMismatch,
		},
		{
			name: "a different administrator",
			corrupt: func(_ *operationHarness, preview *Operation) (string, string) {
				return secondActor, preview.Revision
			},
			want: ErrNotTheActorShown,
		},
		{
			name: "the world moved under the preview",
			corrupt: func(harness *operationHarness, preview *Operation) (string, string) {
				harness.observer.observed["connection"] = "wired-2"
				return firstActor, preview.Revision
			},
			want: ErrPreviewStale,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			harness := newOperationHarness(t)
			preview, err := harness.manager.Preview(
				context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
			if err != nil {
				t.Fatalf("Preview: %v", err)
			}

			actor, revision := test.corrupt(harness, &preview)
			if _, err := harness.manager.Acknowledge(
				context.Background(), actor, preview.ID, revision); !errors.Is(err, test.want) {
				t.Fatalf("Acknowledge error = %v, want %v", err, test.want)
			}
			if len(harness.changer.applied) != 0 {
				t.Fatalf("a refused acknowledgement still changed the appliance: %v", harness.changer.applied)
			}
		})
	}
}

// Only a change that can cut the caller's own path needs confirming. Importing
// a profile or installing a package cannot, so waiting on those would be
// ceremony rather than safety.
func TestOnlyConnectivityChangesEnterAConfirmationWindow(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		action Action
		target string
		want   OperationState
	}{
		{action: ActionInterfaceDisable, target: "enp1s0", want: StateAwaitingConfirmation},
		{action: ActionInterfaceEnable, target: "enp1s0", want: StateAwaitingConfirmation},
		{action: ActionVPNActivate, target: "office", want: StateAwaitingConfirmation},
		{action: ActionVPNDeactivate, target: "office", want: StateAwaitingConfirmation},
		{action: ActionWireGuardInstall, target: "wireguard", want: StateConfirmed},
		{action: ActionProfileImport, target: "office", want: StateConfirmed},
		{action: ActionProfileRemove, target: "office", want: StateConfirmed},
		{action: ActionAutostartSelect, target: "office", want: StateConfirmed},
	} {
		test := test
		t.Run(string(test.action), func(t *testing.T) {
			t.Parallel()
			harness := newOperationHarness(t)
			preview, err := harness.manager.Preview(context.Background(), firstActor, test.action, test.target)
			if err != nil {
				t.Fatalf("Preview: %v", err)
			}
			operation, err := harness.manager.Acknowledge(
				context.Background(), firstActor, preview.ID, preview.Revision)
			if err != nil {
				t.Fatalf("Acknowledge: %v", err)
			}
			if operation.State != test.want {
				t.Fatalf("%s reached %q, want %q", test.action, operation.State, test.want)
			}
			if test.want == StateAwaitingConfirmation {
				if got := operation.ConfirmDeadline.Sub(harness.now); got != ConfirmationWindow {
					t.Fatalf("confirmation window = %s, want %s", got, ConfirmationWindow)
				}
				if ConfirmationWindow != 120*time.Second {
					t.Fatalf("ConfirmationWindow = %s, want 120s", ConfirmationWindow)
				}
			} else if !operation.ConfirmDeadline.IsZero() {
				t.Fatalf("%s carries a confirmation deadline it does not need", test.action)
			}
		})
	}
}

func TestConfirmationKeepsTheChangeAndReleasesTheCheckpoint(t *testing.T) {
	t.Parallel()

	harness := newOperationHarness(t)
	preview, _ := harness.manager.Preview(context.Background(), firstActor, ActionVPNActivate, "office")
	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, preview.ID, preview.Revision); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	harness.now = harness.now.Add(30 * time.Second)
	confirmed, err := harness.manager.Confirm(context.Background(), firstActor, preview.ID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.State != StateConfirmed {
		t.Fatalf("state = %q, want %q", confirmed.State, StateConfirmed)
	}
	if len(harness.changer.discarded) != 1 {
		t.Fatalf("discarded %v, want the one checkpoint released", harness.changer.discarded)
	}
	if len(harness.changer.restored) != 0 {
		t.Fatalf("a confirmed change was restored anyway: %v", harness.changer.restored)
	}
}

// Nobody confirms, because the administrator just cut their own access. The
// window has to assume exactly that and restore without being asked.
func TestAnUnconfirmedChangeIsRestoredWhenTheWindowExpires(t *testing.T) {
	t.Parallel()

	harness := newOperationHarness(t)
	preview, _ := harness.manager.Preview(context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, preview.ID, preview.Revision); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	harness.now = harness.now.Add(ConfirmationWindow + time.Second)
	harness.manager.ExpireDue(context.Background())

	operation, err := harness.manager.Get(preview.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if operation.State != StateRolledBack {
		t.Fatalf("state = %q, want %q", operation.State, StateRolledBack)
	}
	if len(harness.changer.restored) != 1 {
		t.Fatalf("restored %v, want the one checkpoint rolled back", harness.changer.restored)
	}
	if _, err := harness.manager.Confirm(context.Background(), firstActor, preview.ID); err == nil {
		t.Fatal("a confirmation was accepted after the window had already expired")
	}
}

// "Restored" is an observation. When the previous state is not seen afterwards,
// the operation must say so loudly rather than report success.
func TestRestorationThatIsNotObservedIsReportedAsAFailure(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		arrange func(*changerSpy)
	}{
		{
			name:    "the previous state was not observed",
			arrange: func(spy *changerSpy) { spy.observedRestored = false },
		},
		{
			name:    "restoration itself failed",
			arrange: func(spy *changerSpy) { spy.restoreErr = errors.New("checkpoint gone") },
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			harness := newOperationHarness(t)
			test.arrange(harness.changer)

			preview, _ := harness.manager.Preview(
				context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
			if _, err := harness.manager.Acknowledge(
				context.Background(), firstActor, preview.ID, preview.Revision); err != nil {
				t.Fatalf("Acknowledge: %v", err)
			}
			harness.now = harness.now.Add(ConfirmationWindow + time.Second)
			harness.manager.ExpireDue(context.Background())

			operation, err := harness.manager.Get(preview.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if operation.State != StateRollbackFailed {
				t.Fatalf("state = %q, want %q", operation.State, StateRollbackFailed)
			}
			if operation.Failure == "" {
				t.Fatal("a failed rollback carries no explanation")
			}
		})
	}
}

// Two connectivity changes at once means neither can be reasoned about, so the
// second is refused rather than queued.
func TestOnlyOneConnectivityChangeMayBeInFlight(t *testing.T) {
	t.Parallel()

	harness := newOperationHarness(t)
	first, _ := harness.manager.Preview(context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	second, _ := harness.manager.Preview(context.Background(), firstActor, ActionVPNActivate, "office")

	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, first.ID, first.Revision); err != nil {
		t.Fatalf("first Acknowledge: %v", err)
	}
	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, second.ID, second.Revision); !errors.Is(err, ErrChangeInProgress) {
		t.Fatalf("second Acknowledge error = %v, want ErrChangeInProgress", err)
	}
	if len(harness.changer.applied) != 1 {
		t.Fatalf("applied %v, want only the first change", harness.changer.applied)
	}

	// Once the first resolves, the second is allowed.
	if _, err := harness.manager.Confirm(context.Background(), firstActor, first.ID); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, second.ID, second.Revision); err != nil {
		t.Fatalf("Acknowledge after the first resolved: %v", err)
	}
}

// If the appliance cannot be put back, it must not be changed in the first
// place. The refusal has to come before anything is applied.
func TestAChangeIsRefusedWhenRecoveryCannotBeArmed(t *testing.T) {
	t.Parallel()

	harness := newOperationHarness(t)
	harness.changer.armErr = errors.New("NetworkManager refused a checkpoint")

	preview, _ := harness.manager.Preview(context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, preview.ID, preview.Revision); !errors.Is(err, ErrRecoveryUnavailable) {
		t.Fatalf("Acknowledge error = %v, want ErrRecoveryUnavailable", err)
	}
	if len(harness.changer.applied) != 0 {
		t.Fatalf("connectivity was changed with no way back: %v", harness.changer.applied)
	}

	operation, err := harness.manager.Get(preview.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if operation.State != StatePreviewed {
		t.Fatalf("state = %q, want the operation left at %q", operation.State, StatePreviewed)
	}
}

// An explicit rollback is the administrator saying "put it back" before the
// window runs out.
func TestAnAdministratorMayRollBackInsideTheWindow(t *testing.T) {
	t.Parallel()

	harness := newOperationHarness(t)
	preview, _ := harness.manager.Preview(context.Background(), firstActor, ActionVPNActivate, "office")
	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, preview.ID, preview.Revision); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	harness.now = harness.now.Add(10 * time.Second)
	operation, err := harness.manager.Rollback(context.Background(), firstActor, preview.ID)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if operation.State != StateRolledBack {
		t.Fatalf("state = %q, want %q", operation.State, StateRolledBack)
	}
	if len(harness.changer.restored) != 1 {
		t.Fatalf("restored %v, want one", harness.changer.restored)
	}
}

// An action that fails while being applied has to release its checkpoint and
// leave the appliance where it started.
func TestAFailedApplicationRestoresAndReportsTheFailure(t *testing.T) {
	t.Parallel()

	harness := newOperationHarness(t)
	harness.changer.applyErr = errors.New("device busy")

	preview, _ := harness.manager.Preview(context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	if _, err := harness.manager.Acknowledge(
		context.Background(), firstActor, preview.ID, preview.Revision); err == nil {
		t.Fatal("Acknowledge reported success for a change that failed to apply")
	}

	operation, err := harness.manager.Get(preview.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if operation.State != StateFailed {
		t.Fatalf("state = %q, want %q", operation.State, StateFailed)
	}
	if len(harness.changer.restored) != 1 {
		t.Fatalf("a failed application did not restore: %v", harness.changer.restored)
	}
}

func TestNewOperationManagerRequiresItsCollaborators(t *testing.T) {
	t.Parallel()

	complete := OperationManagerOptions{
		Changer: newChangerSpy(), Observer: &observerStub{},
		Now: time.Now, NewID: sequentialIDs(),
	}
	for _, test := range []struct {
		name  string
		spoil func(*OperationManagerOptions)
	}{
		{name: "no changer", spoil: func(o *OperationManagerOptions) { o.Changer = nil }},
		{name: "no observer", spoil: func(o *OperationManagerOptions) { o.Observer = nil }},
		{name: "no clock", spoil: func(o *OperationManagerOptions) { o.Now = nil }},
		{name: "no identity source", spoil: func(o *OperationManagerOptions) { o.NewID = nil }},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options := complete
			test.spoil(&options)
			if _, err := NewOperationManager(options); err == nil {
				t.Fatalf("NewOperationManager accepted options with %s", test.name)
			}
		})
	}
}
