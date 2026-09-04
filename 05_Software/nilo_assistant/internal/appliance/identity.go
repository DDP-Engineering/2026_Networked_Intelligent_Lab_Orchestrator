// Package appliance owns the identity a NILO appliance records its history
// under: one stable identifier for the appliance itself, and one identifier for
// each start of the process.
//
// The identity exists before anything can record an event, because an event
// whose appliance is unknown cannot be correlated later, and a hostname, an IP
// address or a MAC address is not an identity: each of them changes for reasons
// that have nothing to do with this being a different appliance, and two clones
// of one image would share all three.
package appliance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// maximumLabelBytes bounds the two operator-supplied labels. They are shown in
// the interface and copied into events as they read at event time, so they are
// kept short enough to stay readable in a table cell.
const maximumLabelBytes = 120

// Identity is what this appliance is. ID is generated once and never changes;
// Name and Location are operator labels and may change at any time.
type Identity struct {
	ID       string
	Name     string
	Location string
	// CommissionedAt is when the identity was first created, which is the
	// closest thing the appliance has to a birthday.
	CommissionedAt time.Time
}

// Validate refuses an identity that could not be used to attribute an event.
func (identity Identity) Validate() error {
	parsed, err := uuid.Parse(identity.ID)
	if err != nil || parsed == uuid.Nil {
		return errors.New("appliance identity must be a UUID")
	}
	if identity.CommissionedAt.IsZero() || identity.CommissionedAt.Location() != time.UTC {
		return errors.New("appliance commissioning time must be UTC")
	}
	if err := validateLabel("name", identity.Name); err != nil {
		return err
	}
	return validateLabel("location", identity.Location)
}

func validateLabel(field, value string) error {
	if len(value) > maximumLabelBytes {
		return fmt.Errorf("appliance %s is longer than %d bytes", field, maximumLabelBytes)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("appliance %s cannot contain control characters", field)
	}
	return nil
}

// Repository stores the single appliance identity. There is deliberately no
// delete and no identity replacement: an appliance that could be given a new
// identity could disown its own history.
type Repository interface {
	// Ensure stores candidate if and only if no identity is stored yet, and
	// returns whatever is stored afterwards. A second call is an ordinary start,
	// not a re-provisioning, so it returns the existing identity unchanged.
	Ensure(ctx context.Context, candidate Identity) (Identity, error)
	// SetLabels changes the two mutable labels.
	SetLabels(ctx context.Context, name, location string) (Identity, error)
}

// Runtime is the identity of this appliance together with the identity of this
// particular boot, which is what tells two runs of one appliance apart.
type Runtime struct {
	Identity Identity
	BootID   string
}

// Provision reads the appliance identity, creating it on first initialization,
// and allocates an identifier for this boot.
//
// Cloning a commissioned appliance copies the stored identity with it. Nothing
// here can detect that, and nothing here pretends to: re-provisioning a clone is
// an operational procedure, not a software check.
func Provision(ctx context.Context, repository Repository, now func() time.Time) (Runtime, error) {
	if repository == nil {
		return Runtime{}, errors.New("appliance provisioning requires a repository")
	}
	if now == nil {
		return Runtime{}, errors.New("appliance provisioning requires a clock")
	}
	candidateID, err := uuid.NewV7()
	if err != nil {
		return Runtime{}, fmt.Errorf("create appliance identity: %w", err)
	}
	bootID, err := uuid.NewV7()
	if err != nil {
		return Runtime{}, fmt.Errorf("create boot identity: %w", err)
	}
	identity, err := repository.Ensure(ctx, Identity{
		ID: candidateID.String(), CommissionedAt: now().UTC(),
	})
	if err != nil {
		return Runtime{}, fmt.Errorf("provision appliance identity: %w", err)
	}
	return Runtime{Identity: identity, BootID: bootID.String()}, nil
}
