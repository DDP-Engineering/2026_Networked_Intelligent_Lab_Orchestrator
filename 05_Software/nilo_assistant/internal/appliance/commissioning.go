package appliance

import (
	"context"
	"errors"
	"fmt"
)

// CommissioningState is where an appliance stands in its lifecycle.
//
// It is deliberately separate from Identity.CommissionedAt, which is the moment
// the identity row was written and says nothing about whether an administrator
// has finished configuring the appliance.
type CommissioningState string

const (
	// CommissioningProvisioning is a clean installation that has not completed
	// commissioning. First-boot behavior is offered only in this state.
	CommissioningProvisioning CommissioningState = "provisioning"
	// CommissioningCommissioned is an appliance whose configuration an
	// administrator has settled, and every installation carried forward from a
	// release that predates this state.
	CommissioningCommissioned CommissioningState = "commissioned"
)

// ErrCommissioningUnknown is any recorded value that is not one of the two
// states, including an absent one.
var ErrCommissioningUnknown = errors.New("unknown appliance commissioning state")

// Validate refuses anything but the two recorded states. There is no default:
// the state decides whether management access is offered on every eligible
// adapter, so guessing it in either direction is unsafe.
func (state CommissioningState) Validate() error {
	switch state {
	case CommissioningProvisioning, CommissioningCommissioned:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrCommissioningUnknown, string(state))
	}
}

// CommissioningStore reads the durable lifecycle state from the appliance
// database.
type CommissioningStore interface {
	LoadCommissioning(context.Context) (CommissioningState, error)
}

// CommissioningWriter records that the appliance has finished commissioning.
// The transition is one way: nothing returns an appliance to provisioning.
type CommissioningWriter interface {
	SaveCommissioning(context.Context, CommissioningState) error
}

// LoadCommissioning reads the recorded state and refuses to report one that is
// absent or unrecognized, so a database that cannot say where the appliance
// stands stops the start instead of choosing for it.
func LoadCommissioning(ctx context.Context, store CommissioningStore) (CommissioningState, error) {
	if store == nil {
		return "", errors.New("a commissioning store is required")
	}
	state, err := store.LoadCommissioning(ctx)
	if err != nil {
		return "", fmt.Errorf("read appliance commissioning state: %w", err)
	}
	if err := state.Validate(); err != nil {
		return "", err
	}
	return state, nil
}
