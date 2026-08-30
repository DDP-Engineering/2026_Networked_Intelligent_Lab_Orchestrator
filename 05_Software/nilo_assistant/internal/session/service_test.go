package session_test

import (
	"context"
	"testing"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/policy"
	"nilo.local/nilo/internal/session"
)

func TestServiceCreatesPendingOrQueuedSessionFromPolicyDecision(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{}
	service, err := session.NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	for _, test := range []struct {
		name     string
		decision policy.DecisionKind
		want     domain.SessionState
	}{
		{name: "prompt", decision: policy.Prompt, want: domain.PendingApproval},
		{name: "allow", decision: policy.Allow, want: domain.Queued},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestID, err := domain.NewRequestID()
			if err != nil {
				t.Fatalf("NewRequestID() error = %v", err)
			}
			request := rehydratedRequest(t, requestID, now)

			created, err := service.Create(context.Background(), request, test.decision)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if created.State != test.want {
				t.Fatalf("Create() state = %q, want %q", created.State, test.want)
			}
			if created.RequestID != requestID {
				t.Fatalf("Create() request ID = %s, want %s", created.RequestID, requestID)
			}
		})
	}
}

func TestServiceReleasesLeasesAfterTerminalTransition(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	requestID, err := domain.NewRequestID()
	if err != nil {
		t.Fatalf("NewRequestID() error = %v", err)
	}
	sessionID, err := domain.NewSessionID()
	if err != nil {
		t.Fatalf("NewSessionID() error = %v", err)
	}
	value, err := domain.RehydrateSession(sessionID, requestID, domain.Running, now, now, nil)
	if err != nil {
		t.Fatalf("RehydrateSession() error = %v", err)
	}
	repository := &memoryRepository{sessions: map[domain.SessionID]domain.Session{value.ID: value}}
	service, err := session.NewService(repository, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err := service.Transition(context.Background(), value.ID, domain.Running, domain.Completed); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	if repository.released != value.ID {
		t.Fatalf("released session = %s, want %s", repository.released, value.ID)
	}
}

type memoryRepository struct {
	sessions map[domain.SessionID]domain.Session
	released domain.SessionID
}

func (repository *memoryRepository) Create(_ context.Context, value domain.Session) error {
	if repository.sessions == nil {
		repository.sessions = make(map[domain.SessionID]domain.Session)
	}
	repository.sessions[value.ID] = value
	return nil
}

func (repository *memoryRepository) Get(_ context.Context, id domain.SessionID) (domain.Session, error) {
	value, ok := repository.sessions[id]
	if !ok {
		return domain.Session{}, session.ErrSessionNotFound
	}
	return value, nil
}

func (repository *memoryRepository) Transition(_ context.Context, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error) {
	value, ok := repository.sessions[id]
	if !ok {
		return domain.Session{}, session.ErrSessionNotFound
	}
	if value.State != from {
		return domain.Session{}, session.ErrSessionStateConflict
	}
	if err := domain.ValidateTransition(from, to); err != nil {
		return domain.Session{}, err
	}
	value.State, value.UpdatedAt = to, at
	if to == domain.Completed || to == domain.Failed || to == domain.Cancelled || to == domain.Incomplete {
		terminal := at
		value.TerminalAt = &terminal
	}
	repository.sessions[id] = value
	return value, nil
}

func (repository *memoryRepository) AcquireLeases(context.Context, domain.SessionID, []string, time.Time) error {
	return nil
}

func (repository *memoryRepository) ReleaseLeases(_ context.Context, id domain.SessionID, _ time.Time) error {
	repository.released = id
	return nil
}

func (repository *memoryRepository) ListInterrupted(context.Context) ([]domain.Session, error) {
	return nil, nil
}

func rehydratedRequest(t *testing.T, id domain.RequestID, now time.Time) domain.OperationRequest {
	t.Helper()
	actorID, err := domain.NewActorID()
	if err != nil {
		t.Fatalf("NewActorID() error = %v", err)
	}
	integrationID, err := domain.NewIntegrationID()
	if err != nil {
		t.Fatalf("NewIntegrationID() error = %v", err)
	}
	deviceID, err := domain.NewDeviceID()
	if err != nil {
		t.Fatalf("NewDeviceID() error = %v", err)
	}
	configurationSnapshot, err := domain.NewSnapshotID()
	if err != nil {
		t.Fatalf("NewSnapshotID() error = %v", err)
	}
	policySnapshot, err := domain.NewSnapshotID()
	if err != nil {
		t.Fatalf("NewSnapshotID() error = %v", err)
	}
	request, err := domain.RehydrateOperationRequest(id, domain.OperationRequestInput{
		Actor: domain.Actor{ID: actorID}, IntegrationID: integrationID, OperationName: "capture", DeviceID: deviceID,
		CanonicalParameters: []byte(`{}`), ConfigurationSnapshot: configurationSnapshot, PolicySnapshot: policySnapshot,
		Deadline: now.Add(time.Minute),
	}, now)
	if err != nil {
		t.Fatalf("RehydrateOperationRequest() error = %v", err)
	}
	return request
}

var _ session.Repository = (*memoryRepository)(nil)
