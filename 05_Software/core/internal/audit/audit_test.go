package audit_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/store/sqlite"
)

func TestRepositoryAppendsEventsInSequenceOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := sqlite.NewAuditRepository(database)
	actorID, err := domain.NewActorID()
	if err != nil {
		t.Fatalf("NewActorID() error = %v", err)
	}
	createdAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)

	for index, action := range []string{"operation.started", "operation.completed"} {
		id, idErr := uuid.NewV7()
		if idErr != nil {
			t.Fatalf("uuid.NewV7() error = %v", idErr)
		}
		event := audit.Event{
			ID: id.String(), ActorID: actorID, Action: action,
			SubjectType: "session", SubjectID: "session-42",
			Payload:   []byte(`{"source":"test"}`),
			CreatedAt: createdAt.Add(time.Duration(index) * time.Second),
		}
		if err := repository.Append(ctx, event); err != nil {
			t.Fatalf("Append(%q) error = %v", action, err)
		}
	}

	events, err := repository.List(ctx, audit.Filter{SubjectType: "session", SubjectID: "session-42"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("List() count = %d, want 2", len(events))
	}
	for index, wantAction := range []string{"operation.started", "operation.completed"} {
		wantSequence := int64(index + 1)
		if events[index].Sequence != wantSequence {
			t.Errorf("event[%d].Sequence = %d, want %d", index, events[index].Sequence, wantSequence)
		}
		if events[index].Action != wantAction {
			t.Errorf("event[%d].Action = %q, want %q", index, events[index].Action, wantAction)
		}
		if events[index].ActorID != actorID {
			t.Errorf("event[%d].ActorID = %s, want %s", index, events[index].ActorID.String(), actorID.String())
		}
	}
}

func TestRepositoryRejectsMissingActorOrAction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := sqlite.NewAuditRepository(database)
	actorID, err := domain.NewActorID()
	if err != nil {
		t.Fatalf("NewActorID() error = %v", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7() error = %v", err)
	}
	valid := audit.Event{
		ID: id.String(), ActorID: actorID, Action: "operation.started",
		SubjectType: "session", SubjectID: "session-42", Payload: []byte(`{}`),
		CreatedAt: time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name  string
		event audit.Event
	}{
		{name: "missing actor", event: func() audit.Event { event := valid; event.ActorID = domain.ActorID{}; return event }()},
		{name: "missing action", event: func() audit.Event { event := valid; event.Action = " "; return event }()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := repository.Append(ctx, test.event); !errors.Is(err, audit.ErrInvalidEvent) {
				t.Fatalf("Append() error = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestRepositoryContractIsAppendAndListOnly(t *testing.T) {
	t.Parallel()

	var repository audit.Repository = sqlite.NewAuditRepository(nil)
	_ = repository.Append
	_ = repository.List
}
