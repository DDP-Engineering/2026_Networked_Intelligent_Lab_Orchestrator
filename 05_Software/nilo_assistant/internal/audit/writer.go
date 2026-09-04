package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/domain"
)

// Record is what a caller actually knows about something that happened. The
// envelope fields nobody should have to remember — the appliance, the boot, the
// clock and the event identity — are filled in by the Writer.
type Record struct {
	Category    Category
	Component   string
	Action      string
	SubjectType string
	SubjectID   string
	// TargetLabel is optional and is a label, not an identity.
	TargetLabel string

	ActorKind ActorKind
	// ActorID may be left zero for a system or anonymous actor: the writer
	// substitutes the fixed identity for that kind.
	ActorID domain.ActorID

	Outcome     Outcome
	Severity    Severity
	OperationID string

	// Details is marshalled into the payload. It must contain only safe values:
	// no password, key, token, header, cookie, request body or raw subprocess
	// output reaches an event at any category, including debug.
	Details any
}

// WriterOptions are the collaborators every event needs and no caller should
// have to supply per event.
type WriterOptions struct {
	Events      Repository
	ApplianceID string
	BootID      string
	Now         func() time.Time
	// NewID defaults to a UUIDv7, which orders by creation and so keeps a
	// listing readable even before the sequence is looked at.
	NewID func() (string, error)
	// ClockSynchronized reports whether the host clock is currently
	// synchronized. It defaults to reporting that this is unknown, which is
	// recorded as unsynchronized rather than as a synchronization nobody checked.
	ClockSynchronized func() bool
}

// Writer records events with a complete envelope.
type Writer struct {
	options WriterOptions
}

// NewWriter binds a writer to the appliance it records for.
func NewWriter(options WriterOptions) (*Writer, error) {
	switch {
	case options.Events == nil:
		return nil, errors.New("audit writer requires an event repository")
	case options.ApplianceID == "":
		return nil, errors.New("audit writer requires the appliance identity")
	case options.BootID == "":
		return nil, errors.New("audit writer requires the boot identity")
	case options.Now == nil:
		return nil, errors.New("audit writer requires a clock")
	}
	if options.NewID == nil {
		options.NewID = func() (string, error) {
			generated, err := uuid.NewV7()
			return generated.String(), err
		}
	}
	if options.ClockSynchronized == nil {
		options.ClockSynchronized = func() bool { return false }
	}
	return &Writer{options: options}, nil
}

// ApplianceID is the identity every event this writer records carries.
func (writer *Writer) ApplianceID() string {
	if writer == nil {
		return ""
	}
	return writer.options.ApplianceID
}

// Record writes one event. It returns the storage failure unchanged, because a
// caller that is about to change the appliance has to be able to decide not to.
func (writer *Writer) Record(ctx context.Context, record Record) error {
	if writer == nil {
		return errors.New("audit writer is required")
	}
	event, err := writer.Compose(record)
	if err != nil {
		return err
	}
	return writer.options.Events.Append(ctx, event)
}

// Compose builds one complete event without appending it. A caller whose state
// change and audit append share a transaction uses this method, then appends
// through the repository bound to that transaction.
func (writer *Writer) Compose(record Record) (Event, error) {
	if writer == nil {
		return Event{}, errors.New("audit writer is required")
	}
	identity, err := writer.options.NewID()
	if err != nil {
		return Event{}, fmt.Errorf("create audit event identity: %w", err)
	}
	actor, err := writer.actorFor(record)
	if err != nil {
		return Event{}, err
	}
	payload := json.RawMessage(`{}`)
	if record.Details != nil {
		encoded, err := json.Marshal(record.Details)
		if err != nil {
			return Event{}, fmt.Errorf("encode audit details: %w", err)
		}
		payload = encoded
	}
	timeSource := TimeUnsynchronized
	if writer.options.ClockSynchronized() {
		timeSource = TimeSynchronized
	}
	severity := record.Severity
	if severity == "" {
		severity = SeverityInfo
	}
	return Event{
		ID: identity, ActorID: actor, Action: record.Action,
		SubjectType: record.SubjectType, SubjectID: record.SubjectID,
		Payload: payload, CreatedAt: writer.options.Now().UTC(),

		EnvelopeVersion: EnvelopeVersion,
		ApplianceID:     writer.options.ApplianceID,
		BootID:          writer.options.BootID,
		Category:        record.Category,
		Component:       record.Component,
		ActorKind:       record.ActorKind,
		Outcome:         record.Outcome,
		Severity:        severity,
		OperationID:     record.OperationID,
		TargetLabel:     record.TargetLabel,
		TimeSource:      timeSource,
	}, nil
}

// actorFor supplies the fixed identity for the two actors that are not people.
// A system action still needs a durable actor, and a request that failed before
// authentication still needs one, but neither may be given a human's identity.
func (writer *Writer) actorFor(record Record) (domain.ActorID, error) {
	switch record.ActorKind {
	case ActorKindSystem:
		return domain.ParseActorID(SystemActorID)
	case ActorKindAnonymous:
		return domain.ParseActorID(AnonymousActorID)
	default:
		return record.ActorID, nil
	}
}
