package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/policy"
)

var ErrDeniedSessionCreation = errors.New("policy denied session creation")

// Service maps an immutable policy decision to an initial session state and
// ensures terminal transitions release the session's active leases.
type Service interface {
	Create(ctx context.Context, request domain.OperationRequest, decision policy.DecisionKind) (domain.Session, error)
	Transition(ctx context.Context, sessionID domain.SessionID, from, to domain.SessionState) (domain.Session, error)
	ReconcileStartup(ctx context.Context) error
}

type service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository, now func() time.Time) (Service, error) {
	if repository == nil || now == nil {
		return nil, errors.New("session repository and clock are required")
	}
	return &service{repository: repository, now: now}, nil
}

func (service *service) Create(ctx context.Context, request domain.OperationRequest, decision policy.DecisionKind) (domain.Session, error) {
	if err := decision.Validate(); err != nil {
		return domain.Session{}, err
	}
	state := domain.PendingApproval
	switch decision {
	case policy.Allow:
		state = domain.Queued
	case policy.Prompt:
		state = domain.PendingApproval
	case policy.Deny:
		return domain.Session{}, ErrDeniedSessionCreation
	}
	now := service.now()
	value, err := domain.NewSession(request.ID(), state, now)
	if err != nil {
		return domain.Session{}, fmt.Errorf("create session value: %w", err)
	}
	if err := service.repository.Create(ctx, value); err != nil {
		return domain.Session{}, err
	}
	return value, nil
}

func (service *service) Transition(ctx context.Context, id domain.SessionID, from, to domain.SessionState) (domain.Session, error) {
	at := service.now()
	value, err := service.repository.Transition(ctx, id, from, to, at)
	if err != nil {
		return domain.Session{}, err
	}
	if isTerminalState(to) {
		if err := service.repository.ReleaseLeases(ctx, id, at); err != nil {
			return domain.Session{}, err
		}
	}
	return value, nil
}

func (service *service) ReconcileStartup(ctx context.Context) error {
	reconciler, ok := service.repository.(StartupReconciler)
	if !ok {
		return ErrStartupReconciliationAPI
	}
	return reconciler.ReconcileStartup(ctx, service.now())
}

func isTerminalState(state domain.SessionState) bool {
	return state == domain.Completed || state == domain.Failed || state == domain.Cancelled || state == domain.Incomplete
}

var _ Service = (*service)(nil)
