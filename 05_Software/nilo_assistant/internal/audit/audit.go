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

// EnvelopeVersion is the shape every event written from now on carries. A
// stored row without one was written before the envelope existed, and reads
// back as version 1 with its envelope fields empty: empty means "the writer
// recorded nothing here", never an asserted value.
const EnvelopeVersion = 2

// Category separates accountable actions from operational events.
type Category string

const (
	CategoryAudit Category = "audit"
	CategoryEvent Category = "event"
)

// ActorKind says what sort of thing acted, so a system action and a failed
// anonymous request are never rendered as a person.
type ActorKind string

const (
	ActorKindUser  ActorKind = "user"
	ActorKindToken ActorKind = "token"
	// ActorKindSystem is the appliance acting on its own: a startup
	// reconciliation, an expiry, a recovery nobody asked for.
	ActorKindSystem ActorKind = "system"
	// ActorKindAnonymous is a request that failed before any identity was
	// established. An address is not an identity and is never promoted to one.
	ActorKindAnonymous ActorKind = "anonymous"
)

// SystemActorID and AnonymousActorID are the two fixed non-human actors. They
// are constants rather than generated values so that every appliance's history
// can be read the same way.
const (
	SystemActorID    = "00000000-0000-4000-8000-00000000005e"
	AnonymousActorID = "00000000-0000-4000-8000-0000000000a0"
)

// Outcome distinguishes the stages of one change, because "requested" and
// "confirmed" are different facts and an event store that conflates them cannot
// answer what actually happened.
type Outcome string

const (
	OutcomeRequested Outcome = "requested"
	OutcomeApplied   Outcome = "applied"
	OutcomeConfirmed Outcome = "confirmed"
	OutcomeRecovered Outcome = "recovered"
	OutcomeFailed    Outcome = "failed"
	OutcomeDenied    Outcome = "denied"
)

// Severity is how much attention the record deserves.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// TimeSource records whether the clock was synchronized when the event was
// written, so a reader can tell an ordering anomaly from a clock problem.
type TimeSource string

const (
	TimeSynchronized   TimeSource = "synchronized"
	TimeUnsynchronized TimeSource = "unsynchronized"
)

var (
	knownCategories = map[Category]bool{CategoryAudit: true, CategoryEvent: true}
	knownActorKinds = map[ActorKind]bool{
		ActorKindUser: true, ActorKindToken: true,
		ActorKindSystem: true, ActorKindAnonymous: true,
	}
	knownOutcomes = map[Outcome]bool{
		OutcomeRequested: true, OutcomeApplied: true, OutcomeConfirmed: true,
		OutcomeRecovered: true, OutcomeFailed: true, OutcomeDenied: true,
	}
	knownSeverities  = map[Severity]bool{SeverityInfo: true, SeverityWarning: true, SeverityError: true}
	knownTimeSources = map[TimeSource]bool{TimeSynchronized: true, TimeUnsynchronized: true}
)

// Event is one immutable, attributable audit record. Sequence is assigned by
// the repository and is therefore zero before Append.
//
// The fields below CreatedAt are the envelope. They are required of an event
// written at EnvelopeVersion 2 or later and absent from one read back at
// version 1.
type Event struct {
	ID          string
	Sequence    int64
	ActorID     domain.ActorID
	Action      string
	SubjectType string
	SubjectID   string
	Payload     json.RawMessage
	CreatedAt   time.Time

	EnvelopeVersion int
	ApplianceID     string
	BootID          string
	Category        Category
	// Component is the product function scope, for example
	// "network.device_networks". It is deliberately not a Go package or symbol.
	Component string
	ActorKind ActorKind
	Outcome   Outcome
	Severity  Severity
	// OperationID correlates the stages of one change. It is empty for an event
	// that belongs to no operation, which is different from an unknown one.
	OperationID string
	// TargetLabel is the target's human label as it read when the event was
	// written, so a later rename does not make the history unreadable. The
	// stable identity stays in SubjectID.
	TargetLabel string
	TimeSource  TimeSource
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
	return event.validateEnvelope()
}

// validateEnvelope enforces the envelope only on an event that claims to carry
// one. An event at version 0 or 1 is the shape written before the envelope
// existed, and is accepted unchanged so existing writers keep working and
// stored rows keep their meaning.
func (event Event) validateEnvelope() error {
	if event.EnvelopeVersion <= 1 {
		return nil
	}
	if event.EnvelopeVersion > EnvelopeVersion {
		return fmt.Errorf("%w: envelope version %d is newer than this appliance understands", ErrInvalidEvent, event.EnvelopeVersion)
	}
	switch {
	case strings.TrimSpace(event.ApplianceID) == "":
		return fmt.Errorf("%w: appliance identity is required", ErrInvalidEvent)
	case strings.TrimSpace(event.BootID) == "":
		return fmt.Errorf("%w: boot identity is required", ErrInvalidEvent)
	case !knownCategories[event.Category]:
		return fmt.Errorf("%w: unknown category %q", ErrInvalidEvent, event.Category)
	case strings.TrimSpace(event.Component) == "":
		return fmt.Errorf("%w: component is required", ErrInvalidEvent)
	case !knownActorKinds[event.ActorKind]:
		return fmt.Errorf("%w: unknown actor kind %q", ErrInvalidEvent, event.ActorKind)
	case !knownOutcomes[event.Outcome]:
		return fmt.Errorf("%w: unknown outcome %q", ErrInvalidEvent, event.Outcome)
	case !knownSeverities[event.Severity]:
		return fmt.Errorf("%w: unknown severity %q", ErrInvalidEvent, event.Severity)
	case !knownTimeSources[event.TimeSource]:
		return fmt.Errorf("%w: unknown time source %q", ErrInvalidEvent, event.TimeSource)
	}
	return nil
}

// SortField names one operator-visible Activity table column.
type SortField string

const (
	SortTime     SortField = "time"
	SortActivity SortField = "activity"
	SortOutcome  SortField = "outcome"
	SortActor    SortField = "actor"
	SortTarget   SortField = "target"
)

// SortOrder controls the direction of an explicitly sorted listing.
type SortOrder string

const (
	SortAscending  SortOrder = "asc"
	SortDescending SortOrder = "desc"
)

// Filter selects an ordered audit-event subset. Zero values select all rows.
type Filter struct {
	ActorID       domain.ActorID
	Action        string
	SubjectType   string
	SubjectID     string
	AfterSequence int64
	// CursorValue is the primary sort value of the last row on the previous
	// page. Sequence is the deterministic tie-breaker.
	CursorValue string
	Limit       int
	Offset      int
	Sort        SortField
	Order       SortOrder

	// Category, Component, Outcome, Severity and OperationID narrow the
	// envelope. A row written before the envelope existed matches none of them,
	// which is correct: it never claimed to be in any of those categories.
	Category    Category
	Component   string
	Outcome     Outcome
	Severity    Severity
	OperationID string
	// From and To bound created_at as a half-open window, so consecutive pages
	// tile without repeating a boundary.
	From time.Time
	To   time.Time
}

func (filter Filter) Validate() error {
	if filter.AfterSequence < 0 || filter.Limit < 0 || filter.Offset < 0 {
		return fmt.Errorf("%w: filter bounds cannot be negative", ErrInvalidEvent)
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && !filter.To.After(filter.From) {
		return fmt.Errorf("%w: time window must end after it starts", ErrInvalidEvent)
	}
	if filter.Sort != "" {
		switch filter.Sort {
		case SortTime, SortActivity, SortOutcome, SortActor, SortTarget:
		default:
			return fmt.Errorf("%w: unknown sort field %q", ErrInvalidEvent, filter.Sort)
		}
		if filter.Order != SortAscending && filter.Order != SortDescending {
			return fmt.Errorf("%w: sorted listings need asc or desc order", ErrInvalidEvent)
		}
	} else if filter.Order != "" || filter.CursorValue != "" {
		return fmt.Errorf("%w: sort order and cursor value need a sort field", ErrInvalidEvent)
	}
	return nil
}

// Repository exposes no operation that can change a stored event. It does
// expose retention, because a bounded appliance cannot keep every record
// forever, and a purge that removes whole expired records is not an edit of a
// retained one.
//
// This is append-only against the application, not against a host
// administrator with the database file. It is not forensic evidence and must
// never be described as tamper-proof.
type Repository interface {
	Append(ctx context.Context, event Event) error
	List(ctx context.Context, filter Filter) ([]Event, error)
	// PurgeBefore deletes at most limit records of one category older than the
	// cutoff and reports how many it deleted, so housekeeping runs in bounded
	// batches off the request path.
	PurgeBefore(ctx context.Context, category Category, cutoff time.Time, limit int) (int, error)
}
