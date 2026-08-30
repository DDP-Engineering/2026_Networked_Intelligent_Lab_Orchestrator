package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/domain"
)

var (
	ErrInvalidEvent         = errors.New("invalid audit event")
	ErrEventAlreadyAppended = errors.New("audit event already appended")
)

// Event is one immutable, attributable audit record. Sequence is assigned by
// the repository and is therefore zero before Append.
type Event struct {
	ID          string
	Sequence    int64
	ActorID     domain.ActorID
	Action      string
	SubjectType string
	SubjectID   string
	Payload     json.RawMessage
	CreatedAt   time.Time
}

func (event Event) Validate() error {
	parsedID, err := uuid.Parse(event.ID)
	if err != nil || parsedID == uuid.Nil {
		return fmt.Errorf("%w: event ID must be a UUID", ErrInvalidEvent)
	}
	if event.Sequence != 0 {
		return fmt.Errorf("%w: sequence is repository assigned", ErrInvalidEvent)
	}
	if event.ActorID.String() == uuid.Nil.String() {
		return fmt.Errorf("%w: actor is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(event.Action) == "" {
		return fmt.Errorf("%w: action is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(event.SubjectType) == "" || strings.TrimSpace(event.SubjectID) == "" {
		return fmt.Errorf("%w: subject type and identity are required", ErrInvalidEvent)
	}
	if len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return fmt.Errorf("%w: payload must be valid JSON", ErrInvalidEvent)
	}
	if event.CreatedAt.IsZero() || event.CreatedAt.Location() != time.UTC {
		return fmt.Errorf("%w: creation time must be UTC", ErrInvalidEvent)
	}
	return nil
}

// Filter selects an ordered audit-event subset. Zero values select all rows.
type Filter struct {
	ActorID       domain.ActorID
	Action        string
	SubjectType   string
	SubjectID     string
	AfterSequence int64
	Limit         int
}

func (filter Filter) Validate() error {
	if filter.AfterSequence < 0 || filter.Limit < 0 {
		return fmt.Errorf("%w: filter bounds cannot be negative", ErrInvalidEvent)
	}
	return nil
}

// Repository deliberately exposes no normal mutation or deletion operation.
type Repository interface {
	Append(ctx context.Context, event Event) error
	List(ctx context.Context, filter Filter) ([]Event, error)
}
