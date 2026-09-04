package network

import (
	"context"
	"errors"
	"testing"
)

type installSpy struct {
	*changerSpy
	starts  int
	state   OperationState
	pollErr error
}

func (s *installSpy) StartInstall(context.Context, string, string) (Operation, error) {
	s.starts++
	return Operation{State: StateApplying}, nil
}
func (s *installSpy) PollInstall(context.Context, string) (Operation, error) {
	return Operation{State: s.state, Failure: "package_lock_busy: wait and retry"}, s.pollErr
}

func TestInstallationDoesNotInventFailureWhenHelperIsUnreachable(t *testing.T) {
	h := newOperationHarness(t)
	s := &installSpy{changerSpy: h.changer, state: StateApplying, pollErr: errors.New("disconnected")}
	h.manager.options.Changer = s
	ctx := context.Background()
	p, err := h.manager.Preview(ctx, firstActor, ActionWireGuardInstall, "wireguard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.manager.Acknowledge(ctx, firstActor, p.ID, p.Revision); err != nil {
		t.Fatal(err)
	}
	h.manager.ExpireDue(ctx)
	op, err := h.manager.Get(p.ID)
	if err != nil || op.State != StateApplying || op.Progress != "helper_unavailable" || op.Failure != "" {
		t.Fatalf("invented terminal outcome: %+v %v", op, err)
	}
	s.pollErr, s.state = nil, StateFailed
	h.manager.ExpireDue(ctx)
	op, _ = h.manager.Get(p.ID)
	if op.State != StateFailed || op.Failure != "package_lock_busy: wait and retry" || h.manager.inFlight != "" {
		t.Fatalf("lost actual failure: %+v", op)
	}
}

func TestInstallationRemainsObservableAndSerializesChanges(t *testing.T) {
	h := newOperationHarness(t)
	s := &installSpy{changerSpy: h.changer, state: StateApplying}
	h.manager.options.Changer = s
	ctx := context.Background()
	p, err := h.manager.Preview(ctx, firstActor, ActionWireGuardInstall, "wireguard")
	if err != nil {
		t.Fatal(err)
	}
	op, err := h.manager.Acknowledge(ctx, firstActor, p.ID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if op.State != StateApplying || s.starts != 1 || len(s.applied) != 0 {
		t.Fatalf("installation was not dispatched asynchronously: state=%s starts=%d synchronous=%v", op.State, s.starts, s.applied)
	}
	other, err := h.manager.Preview(ctx, firstActor, ActionInterfaceDisable, "eth0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.manager.Acknowledge(ctx, firstActor, other.ID, other.Revision); !errors.Is(err, ErrChangeInProgress) {
		t.Fatalf("concurrent change: %v", err)
	}
	s.state = StateConfirmed
	h.manager.ExpireDue(ctx)
	op, _ = h.manager.Get(p.ID)
	if op.State != StateConfirmed {
		t.Fatalf("completion not observed: %+v", op)
	}
	if _, err = h.manager.Acknowledge(ctx, firstActor, other.ID, other.Revision); err != nil {
		t.Fatal(err)
	}
}
