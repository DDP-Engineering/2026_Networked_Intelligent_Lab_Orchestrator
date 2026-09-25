package network

import (
	"context"
	"errors"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
)

// recoveryStub is the privileged helper's own record of the last change, as
// the restarted core reads it back.
type recoveryStub struct {
	outcome RecoveryOutcome
	present bool
}

func newUnverifiedCleanupHarness(
	t *testing.T,
	recovery *recoveryStub,
) (*OperationManager, *interceptOperationStore, *changerSpy, Operation) {
	t.Helper()
	changer := newChangerSpy()
	changer.restoreErr = errors.New("checkpoint unavailable")
	store := &interceptOperationStore{
		delegate: &operationStoreSpy{},
		beforeReturn: func(_ context.Context, call int, _ OperationRecord) error {
			if call == 2 {
				return errors.New("second save failed")
			}
			return nil
		},
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: changer,
		Observer: &observerStub{observed: map[string]string{
			"state": "connected", "connection": "wired-1",
		}},
		Now:      func() time.Time { return time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC) },
		NewID:    sequentialIDs(),
		Store:    store,
		Recovery: recovery,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := manager.Preview(
		context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acknowledge(
		context.Background(), firstActor, operation.ID, operation.Revision,
	); !errors.Is(err, ErrRecoveryUnavailable) {
		t.Fatalf("Acknowledge error = %v, want ErrRecoveryUnavailable", err)
	}
	return manager, store, changer, operation
}

// An uncertain cleanup may become verifiably restored after the request has
// returned. Periodic reconciliation must release the same-process interlock
// only after the helper reports that restoration.
func TestUnverifiedCheckpointCleanupClearsAfterVerifiedRecovery(t *testing.T) {
	t.Parallel()
	recovery := &recoveryStub{}
	manager, _, _, first := newUnverifiedCleanupHarness(t, recovery)
	second, err := manager.Preview(
		context.Background(), firstActor, ActionVPNActivate, "office")
	if err != nil {
		t.Fatal(err)
	}
	recovery.outcome = RecoveryOutcome{OperationID: first.ID, Phase: "restored"}
	recovery.present = true
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	settled, err := manager.Get(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.State != StateRolledBack {
		t.Fatalf("state = %q, want %q", settled.State, StateRolledBack)
	}
	if _, err := manager.Acknowledge(
		context.Background(), firstActor, second.ID, second.Revision,
	); err != nil {
		t.Fatalf("Acknowledge after verified recovery: %v", err)
	}
}

// Restarting either before or after helper recovery must rehydrate the
// unresolved operation and keep it blocking until restoration is verified.
func TestUnverifiedCheckpointCleanupSurvivesRestart(t *testing.T) {
	t.Parallel()
	for _, restoredBeforeRestart := range []bool{false, true} {
		name := "before helper recovery"
		if restoredBeforeRestart {
			name = "after helper recovery"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			recovery := &recoveryStub{}
			_, store, changer, first := newUnverifiedCleanupHarness(t, recovery)
			if restoredBeforeRestart {
				recovery.outcome = RecoveryOutcome{OperationID: first.ID, Phase: "restored"}
				recovery.present = true
			} else {
				recovery.outcome = RecoveryOutcome{OperationID: first.ID, Phase: "applied"}
				recovery.present = true
			}

			restarted, err := NewOperationManager(OperationManagerOptions{
				Changer: changer,
				Observer: &observerStub{observed: map[string]string{
					"state": "connected", "connection": "wired-1",
				}},
				Now:      func() time.Time { return time.Date(2026, 9, 12, 9, 1, 0, 0, time.UTC) },
				NewID:    func() string { return "after-restart" },
				Store:    store,
				Recovery: recovery,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := restarted.Adopt(context.Background()); err != nil {
				t.Fatal(err)
			}
			pending, err := restarted.PendingIDs(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 1 || pending[0] != first.ID {
				t.Fatalf("pending after restart = %v, want [%s]", pending, first.ID)
			}
			if err := restarted.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}

			if !restoredBeforeRestart {
				blocked, err := restarted.Preview(
					context.Background(), firstActor, ActionVPNActivate, "office")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := restarted.Acknowledge(
					context.Background(), firstActor, blocked.ID, blocked.Revision,
				); !errors.Is(err, ErrChangeInProgress) {
					t.Fatalf("Acknowledge before recovery = %v, want ErrChangeInProgress", err)
				}
				recovery.outcome = RecoveryOutcome{OperationID: first.ID, Phase: "restored"}
				if err := restarted.Reconcile(context.Background()); err != nil {
					t.Fatal(err)
				}
			}

			settled, err := restarted.Get(first.ID)
			if err != nil {
				t.Fatal(err)
			}
			if settled.State != StateRolledBack {
				t.Fatalf("state = %q, want %q", settled.State, StateRolledBack)
			}
		})
	}
}

// An applying connectivity operation can survive a restart while the helper
// still owns its checkpoint. Adoption must restore the global interlock until
// a matching helper outcome proves that operation terminal.
func TestApplyingConnectivityOperationSurvivesRestartWithInterlock(t *testing.T) {
	t.Parallel()
	for _, checkpointID := range []string{"op-applying", ""} {
		t.Run("checkpoint="+checkpointID, func(t *testing.T) {
			testApplyingConnectivityRestart(t, checkpointID)
		})
	}
}

func testApplyingConnectivityRestart(t *testing.T, checkpointID string) {
	t.Helper()
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	store := &operationStoreSpy{saved: []OperationRecord{{
		ID: "op-applying", Action: string(ActionInterfaceDisable), Target: "eth1", ActorID: firstActor,
		State: string(StateApplying), Revision: "r-applying", CheckpointID: checkpointID,
		CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute),
	}}}
	recovery := &recoveryStub{
		outcome: RecoveryOutcome{OperationID: "op-applying", Phase: "applied"},
		present: true,
	}
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: newChangerSpy(), Observer: &observerStub{observed: map[string]string{}},
		Now: func() time.Time { return now }, NewID: func() string { return "op-next" },
		Store: store, Recovery: recovery,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Adopt(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}

	next, err := manager.Preview(context.Background(), firstActor, ActionVPNActivate, "office")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acknowledge(
		context.Background(), firstActor, next.ID, next.Revision,
	); !errors.Is(err, ErrChangeInProgress) {
		t.Fatalf("Acknowledge while adopted apply remains helper-owned = %v, want ErrChangeInProgress", err)
	}

	recovery.outcome.Phase = "restored"
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	settled, err := manager.Get("op-applying")
	if err != nil {
		t.Fatal(err)
	}
	if settled.State != StateRolledBack {
		t.Fatalf("state after matching restoration = %q, want %q", settled.State, StateRolledBack)
	}
	if _, err := manager.Acknowledge(
		context.Background(), firstActor, next.ID, next.Revision,
	); err != nil {
		t.Fatalf("Acknowledge after matching restoration: %v", err)
	}
}

func (stub *recoveryStub) LastRecovery(context.Context) (RecoveryOutcome, bool, error) {
	return stub.outcome, stub.present, nil
}

// TestAdoptedOperationsAreReconciledAgainstTheHelperRecord covers
// REQ-REL-2e91cb74: a change that was in flight when the core died is resolved
// from what the helper actually did, recorded once as the system, and no
// longer blocks the next change.
func TestAdoptedOperationsAreReconciledAgainstTheHelperRecord(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		phase   string
		present bool
		want    OperationState
		outcome audit.Outcome
	}{
		{"restored by the helper", "restored", true, StateRolledBack, audit.OutcomeRecovered},
		{"confirmed before the crash", "confirmed", true, StateConfirmed, audit.OutcomeConfirmed},
		{"restoration failed", "recovery_failed", true, StateRollbackFailed, audit.OutcomeFailed},
		{"never reached the helper", "", false, StateFailed, audit.OutcomeFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &operationStoreSpy{}
			journal := &recordingJournal{}
			now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
			store.saved = append(store.saved, OperationRecord{
				ID: "op-1", Action: string(ActionInterfaceDisable), Target: "eth1", ActorID: firstActor,
				State: string(StateAwaitingConfirmation), Revision: "r", CheckpointID: "op-1",
				CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute), ConfirmDeadline: now.Add(time.Minute),
			})
			changer := newChangerSpy()
			manager, err := NewOperationManager(OperationManagerOptions{
				Changer: changer, Observer: &observerStub{observed: map[string]string{}},
				Now: func() time.Time { return now }, NewID: sequentialIDs(), Store: store, Audit: journal,
				Recovery: &recoveryStub{outcome: RecoveryOutcome{OperationID: "op-1", Phase: tc.phase}, present: tc.present},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.Adopt(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := manager.Reconcile(context.Background()); err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			adopted, err := manager.Get("op-1")
			if err != nil {
				t.Fatal(err)
			}
			if adopted.State != tc.want {
				t.Fatalf("state after reconciliation = %q, want %q", adopted.State, tc.want)
			}
			if len(journal.records) != 1 || journal.records[0].Outcome != tc.outcome || journal.records[0].ActorKind != audit.ActorKindSystem {
				t.Fatalf("journal = %+v, want one %s record by the system", journal.records, tc.outcome)
			}
			if len(changer.restored) != 0 {
				t.Fatal("reconciliation asked the changer to restore again")
			}
			// The next change must not be blocked by the resolved one, except
			// when restoration failed, which is exactly the state that must block.
			preview, err := manager.Preview(context.Background(), firstActor, ActionInterfaceDisable, "eth2")
			if err != nil {
				t.Fatal(err)
			}
			_, err = manager.Acknowledge(context.Background(), firstActor, preview.ID, preview.Revision)
			if tc.want == StateRollbackFailed && err == nil {
				t.Fatal("a failed restoration did not block the next change")
			}
			if tc.want != StateRollbackFailed && err != nil {
				t.Fatalf("the resolved change still blocks the next one: %v", err)
			}
		})
	}
}

// TestAPendingHelperChangeStaysAwaiting: when the helper is still inside the
// window, the adopted operation keeps waiting with its original deadline.
func TestAPendingHelperChangeStaysAwaiting(t *testing.T) {
	t.Parallel()
	store := &operationStoreSpy{}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store.saved = append(store.saved, OperationRecord{
		ID: "op-1", Action: string(ActionInterfaceDisable), Target: "eth1", ActorID: firstActor,
		State: string(StateAwaitingConfirmation), Revision: "r", CheckpointID: "op-1",
		CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute), ConfirmDeadline: now.Add(time.Minute),
	})
	manager, err := NewOperationManager(OperationManagerOptions{
		Changer: newChangerSpy(), Observer: &observerStub{observed: map[string]string{}},
		Now: func() time.Time { return now }, NewID: sequentialIDs(), Store: store, Audit: &recordingJournal{},
		Recovery: &recoveryStub{outcome: RecoveryOutcome{OperationID: "op-1", Phase: "applied"}, present: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Adopt(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	adopted, _ := manager.Get("op-1")
	if adopted.State != StateAwaitingConfirmation || !adopted.ConfirmDeadline.Equal(now.Add(time.Minute)) {
		t.Fatalf("pending change = %+v, want still awaiting with its deadline", adopted)
	}
}
