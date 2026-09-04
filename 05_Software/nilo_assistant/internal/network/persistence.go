package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// OperationRecord is one managed change as it is written down, so that a page
// reload, a core restart or a reboot can find a change that is still in flight.
//
// It carries public metadata only. Imported profile text, private and preshared
// keys and Wi-Fi passwords are held by the privileged helper and referenced;
// nothing that could contain one is stored here.
type OperationRecord struct {
	ID       string
	Action   string
	Target   string
	ActorID  string
	State    string
	Revision string
	// Parameters are the safe normalized parameters of the change, as JSON.
	// Empty means the change had none beyond its target.
	Parameters json.RawMessage
	// CheckpointID references the recovery state the privileged helper armed,
	// which is what a restart reconciles against.
	CheckpointID string
	Failure      string

	CreatedAt time.Time
	UpdatedAt time.Time
	// ConfirmDeadline is zero for an action that opens no confirmation window.
	ConfirmDeadline time.Time
}

// unfinishedStates are the states a restart has to resolve. Everything else is
// terminal and is only ever read for history.
var unfinishedStates = map[OperationState]bool{
	StatePreviewed:            true,
	StateApplying:             true,
	StateAwaitingConfirmation: true,
	StateRollingBack:          true,
}

// Unfinished reports whether a stored state still needs reconciling.
func Unfinished(state string) bool { return unfinishedStates[OperationState(state)] }

// ErrSecretInRecord refuses a record whose parameters name something that
// cannot be public metadata.
var ErrSecretInRecord = errors.New("network operation parameters name a secret value")

// secretParameterNames are the keys whose presence means the caller is about to
// write a secret into public metadata. The check is on the key rather than the
// value, because a value cannot be recognized and a key can.
var secretParameterNames = regexp.MustCompile(
	`(?i)"(private_?key|preshared_?key|psk|password|passphrase|secret|token|profile_?text|config_?text)"\s*:`)

// ValidateRecord refuses a record that is unusable or that carries a secret.
func ValidateRecord(record OperationRecord) error {
	switch {
	case strings.TrimSpace(record.ID) == "":
		return errors.New("a stored network operation requires its identity")
	case strings.TrimSpace(record.Action) == "":
		return errors.New("a stored network operation requires its action")
	case strings.TrimSpace(record.ActorID) == "":
		return errors.New("a stored network operation requires the actor who asked for it")
	case strings.TrimSpace(record.State) == "":
		return errors.New("a stored network operation requires its state")
	case record.CreatedAt.IsZero() || record.UpdatedAt.IsZero():
		return errors.New("a stored network operation requires its timestamps")
	}
	if len(record.Parameters) > 0 {
		if !json.Valid(record.Parameters) {
			return errors.New("stored network operation parameters must be valid JSON")
		}
		if secretParameterNames.Match(record.Parameters) {
			return fmt.Errorf("%w: %s", ErrSecretInRecord, record.Action)
		}
	}
	return nil
}

// OperationStore is where the operation lifecycle writes itself down.
//
// It is optional on the manager: the simulation profile and every unit test
// construct a manager without a database, and making durability a construction
// requirement would turn a durability feature into a startup failure.
type OperationStore interface {
	Save(ctx context.Context, record OperationRecord) error
	// Unfinished returns every operation a restart still has to resolve.
	Unfinished(ctx context.Context) ([]OperationRecord, error)
	// Recent returns the newest operations first, for reattachment after a
	// page reload.
	Recent(ctx context.Context, limit int) ([]OperationRecord, error)
}
