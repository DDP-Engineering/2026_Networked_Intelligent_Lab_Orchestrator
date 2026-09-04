package network

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type recoveryGate struct {
	entered chan struct{}
	release chan struct{}
	arms    atomic.Int32
	applies atomic.Int32
}

func (g *recoveryGate) ArmRecovery(context.Context, string, time.Duration) (Checkpoint, error) {
	if g.arms.Add(1) == 1 {
		close(g.entered)
		<-g.release
	}
	return Checkpoint{ID: "checkpoint"}, nil
}
func (g *recoveryGate) Apply(context.Context, Action, string) error       { g.applies.Add(1); return nil }
func (g *recoveryGate) Restore(context.Context, Checkpoint) (bool, error) { return true, nil }
func (g *recoveryGate) Discard(context.Context, Checkpoint) error         { return nil }

func TestConcurrentAcknowledgementsReserveBeforeRecovery(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "different operations", true: "duplicate operation"}[duplicate], func(t *testing.T) {
			g := &recoveryGate{entered: make(chan struct{}), release: make(chan struct{})}
			m, _ := NewOperationManager(OperationManagerOptions{Changer: g, Observer: &observerStub{}, Now: time.Now, NewID: sequentialIDs()})
			a, _ := m.Preview(context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
			b := a
			if !duplicate {
				b, _ = m.Preview(context.Background(), firstActor, ActionVPNActivate, "office")
			}
			done := make(chan error, 1)
			go func() { _, err := m.Acknowledge(context.Background(), firstActor, a.ID, a.Revision); done <- err }()
			<-g.entered
			_, err := m.Acknowledge(context.Background(), firstActor, b.ID, b.Revision)
			close(g.release)
			firstErr := <-done
			if err == nil || firstErr != nil || g.arms.Load() != 1 || g.applies.Load() != 1 {
				t.Fatalf("concurrent execution: second=%v first=%v arms=%d applies=%d", err, firstErr, g.arms.Load(), g.applies.Load())
			}
		})
	}
}

type failedDiscard struct{ *changerSpy }

func (failedDiscard) Discard(context.Context, Checkpoint) error {
	return errors.New("helper unavailable")
}

func TestConfirmationDoesNotClaimSuccessWhenCheckpointReleaseFails(t *testing.T) {
	h := newOperationHarness(t)
	h.manager.options.Changer = failedDiscard{h.changer}
	p, _ := h.manager.Preview(context.Background(), firstActor, ActionVPNActivate, "office")
	_, _ = h.manager.Acknowledge(context.Background(), firstActor, p.ID, p.Revision)
	_, err := h.manager.Confirm(context.Background(), firstActor, p.ID)
	op, _ := h.manager.Get(p.ID)
	if err == nil || op.State != StateAwaitingConfirmation {
		t.Fatalf("release failure lost recovery: %v %+v", err, op)
	}
	h.now = h.now.Add(ConfirmationWindow)
	h.manager.ExpireDue(context.Background())
	op, _ = h.manager.Get(p.ID)
	if op.State != StateRolledBack {
		t.Fatalf("recovery did not run: %+v", op)
	}
}

func TestFailedApplyAndFailedRecoveryRemainProminentAndBlockNewChanges(t *testing.T) {
	h := newOperationHarness(t)
	h.changer.applyErr = errors.New("partially applied")
	h.changer.restoreErr = errors.New("recovery unavailable")
	p, _ := h.manager.Preview(context.Background(), firstActor, ActionVPNActivate, "office")
	_, _ = h.manager.Acknowledge(context.Background(), firstActor, p.ID, p.Revision)
	op, _ := h.manager.Get(p.ID)
	if op.State != StateRollbackFailed {
		t.Fatalf("hidden recovery failure: %+v", op)
	}
	q, _ := h.manager.Preview(context.Background(), firstActor, ActionInterfaceDisable, "enp1s0")
	if _, err := h.manager.Acknowledge(context.Background(), firstActor, q.ID, q.Revision); !errors.Is(err, ErrChangeInProgress) {
		t.Fatalf("allowed new change after failed recovery: %v", err)
	}
}
