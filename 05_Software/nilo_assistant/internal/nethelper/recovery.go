package nethelper

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Independent recovery: what a root-owned timer runs when neither the core
// nor the helper may be alive. It consumes only the saved typed records - the
// recovery record and the forwarding inventory - and never anything a remote
// caller could have supplied.

// RecoveryOutcome is what one independent recovery pass did.
type RecoveryOutcome struct {
	// Restored is the identity of a pending change that was put back.
	Restored string `json:"restored,omitempty"`
	// Phase is the recovery record's phase after the pass, empty without one.
	Phase string `json:"phase,omitempty"`
	// ForwardingVerified reports that the owned table matched the inventory,
	// or was put back to match it.
	ForwardingVerified bool `json:"forwarding_verified"`
	// ForwardingDetail says what the forwarding check found when it could
	// not be verified.
	ForwardingDetail string `json:"forwarding_detail,omitempty"`
}

// RecoverOnce puts an interrupted change back and makes the owned forwarding
// match its inventory, then returns. A pending record is restored whatever
// its deadline: this runs only when the helper that could have confirmed it
// is gone, and a change nobody can confirm is put back.
func RecoverOnce(ctx context.Context, driver Driver, store Store, now func() time.Time) (RecoveryOutcome, error) {
	engine, err := NewEngine(driver, store, now)
	if err != nil {
		return RecoveryOutcome{}, fmt.Errorf("read the recovery state: %w", err)
	}
	var outcome RecoveryOutcome
	if record, ok := engine.Recovery(); ok && pending(&record) {
		outcome.Restored = record.ID
	}
	restoreErr := engine.Reconcile(ctx)
	if record, ok := engine.Recovery(); ok {
		outcome.Phase = record.Phase
	}
	if restoreErr != nil {
		return outcome, fmt.Errorf("restore the interrupted change: %w", restoreErr)
	}
	verifyErr := engine.VerifyForwarding(ctx)
	switch {
	case verifyErr == nil:
		outcome.ForwardingVerified = true
	case errors.Is(verifyErr, ErrForwardingUnsupported):
		outcome.ForwardingVerified = true
		outcome.ForwardingDetail = "this helper manages no forwarding"
	default:
		outcome.ForwardingDetail = verifyErr.Error()
		return outcome, fmt.Errorf("verify the owned forwarding: %w", verifyErr)
	}
	return outcome, nil
}
