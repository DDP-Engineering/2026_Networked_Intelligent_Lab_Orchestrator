package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type SessionState string

const (
	PendingApproval SessionState = "pending_approval"
	Queued          SessionState = "queued"
	Running         SessionState = "running"
	Completed       SessionState = "completed"
	Failed          SessionState = "failed"
	Cancelled       SessionState = "cancelled"
	Incomplete      SessionState = "incomplete"
)

var ErrInvalidTransition = errors.New("invalid session transition")

var ErrInvalidSession = errors.New("invalid session")

// Session is the durable, truthful state of one accepted operation request.
// Its terminal timestamp is present exactly when the state is terminal.
type Session struct {
	ID         SessionID
	RequestID  RequestID
	State      SessionState
	CreatedAt  time.Time
	UpdatedAt  time.Time
	TerminalAt *time.Time
}

// NewSession creates a new session at one of the only two valid entry states.
func NewSession(requestID RequestID, state SessionState, createdAt time.Time) (Session, error) {
	if state != PendingApproval && state != Queued {
		return Session{}, fmt.Errorf("%w: initial state must be pending approval or queued", ErrInvalidSession)
	}
	id, err := NewSessionID()
	if err != nil {
		return Session{}, err
	}
	return RehydrateSession(id, requestID, state, createdAt, createdAt, nil)
}

// RehydrateSession reconstructs one previously persisted session and validates
// the same invariants that apply to a newly created one.
func RehydrateSession(id SessionID, requestID RequestID, state SessionState, createdAt, updatedAt time.Time, terminalAt *time.Time) (Session, error) {
	value := Session{ID: id, RequestID: requestID, State: state, CreatedAt: createdAt, UpdatedAt: updatedAt}
	if terminalAt != nil {
		terminal := *terminalAt
		value.TerminalAt = &terminal
	}
	if err := value.Validate(); err != nil {
		return Session{}, err
	}
	return value, nil
}

func (session Session) Validate() error {
	parsedSessionID := uuid.UUID(session.ID)
	if parsedSessionID == uuid.Nil || parsedSessionID.Version() != uuid.Version(7) || session.RequestID.isZero() {
		return fmt.Errorf("%w: session and request identities are required UUID-v7 values", ErrInvalidSession)
	}
	if !isKnownSessionState(session.State) {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidSession, session.State)
	}
	if !isUTCTimestamp(session.CreatedAt) || !isUTCTimestamp(session.UpdatedAt) || session.UpdatedAt.Before(session.CreatedAt) {
		return fmt.Errorf("%w: creation and update times must be ordered UTC timestamps", ErrInvalidSession)
	}
	terminal := isTerminalSessionState(session.State)
	if terminal != (session.TerminalAt != nil) {
		return fmt.Errorf("%w: terminal timestamp must match terminal state", ErrInvalidSession)
	}
	if session.TerminalAt != nil {
		if !isUTCTimestamp(*session.TerminalAt) || !session.TerminalAt.Equal(session.UpdatedAt) {
			return fmt.Errorf("%w: terminal time must be the UTC terminal update", ErrInvalidSession)
		}
	}
	return nil
}

func isKnownSessionState(state SessionState) bool {
	switch state {
	case PendingApproval, Queued, Running, Completed, Failed, Cancelled, Incomplete:
		return true
	default:
		return false
	}
}

func isTerminalSessionState(state SessionState) bool {
	return state == Completed || state == Failed || state == Cancelled || state == Incomplete
}

func isUTCTimestamp(value time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC
}

func ValidateTransition(from, to SessionState) error {
	allowed := (from == PendingApproval && to == Queued) ||
		(from == Queued && to == Running) ||
		(from == Running && (to == Completed || to == Failed || to == Cancelled || to == Incomplete))
	if !allowed {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	return nil
}
