package network

import (
	"context"
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
