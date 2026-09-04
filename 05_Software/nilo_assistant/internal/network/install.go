package network

import (
	"context"
	"time"
)

// InstallationController starts a service-owned job. Cancelling the requesting
// browser does not cancel installation; PollInstall returns its public state.
// Implementations return only applying, confirmed or failed states.
type InstallationController interface {
	StartInstall(ctx context.Context, id, revision string) (Operation, error)
	PollInstall(ctx context.Context, id string) (Operation, error)
}

func (m *OperationManager) startInstall(ctx context.Context, installer InstallationController, op *Operation) (Operation, error) {
	m.mutex.Lock()
	op.State, op.Progress = StateApplying, "queued"
	m.inFlight = op.ID
	m.mutex.Unlock()
	state, err := installer.StartInstall(ctx, op.ID, op.observed["install_revision"])
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if err != nil {
		// An interrupted reply can hide an accepted job. Poll before allowing
		// another change; the helper independently holds the mutation lock.
		op.Progress = "helper_unavailable"
		return *op, nil
	}
	m.installState(op, state)
	return *op, nil
}

// Called with changeMutex held, keeping polling and new acknowledgements ordered.
func (m *OperationManager) pollInstall(ctx context.Context) {
	installer, ok := m.options.Changer.(InstallationController)
	if !ok {
		return
	}
	m.mutex.Lock()
	op := m.operations[m.inFlight]
	if op == nil || op.Action != ActionWireGuardInstall || op.State != StateApplying {
		m.mutex.Unlock()
		return
	}
	id := op.ID
	m.mutex.Unlock()
	pollCtx, cancel := context.WithTimeout(ctx, time.Second)
	state, err := installer.PollInstall(pollCtx, id)
	cancel()
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if err != nil {
		op.Progress = "helper_unavailable"
		return
	}
	m.installState(op, state)
}

func (m *OperationManager) installState(op *Operation, state Operation) {
	switch state.State {
	case StateApplying, StateConfirmed, StateFailed:
		op.State, op.Progress, op.Failure = state.State, state.Progress, state.Failure
		if state.State != StateApplying {
			m.inFlight = ""
		}
	default:
		op.Progress = "helper_unavailable"
	}
}
