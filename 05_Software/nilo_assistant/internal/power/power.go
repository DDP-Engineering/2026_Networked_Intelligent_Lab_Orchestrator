// Package power defines the narrow whole-appliance power boundary.
package power

import (
	"context"
	"errors"
)

// ErrUnavailable reports that this runtime cannot control appliance power.
var ErrUnavailable = errors.New("appliance power control unavailable")

// Controller performs orderly whole-appliance power transitions.
type Controller interface {
	Restart(context.Context) error
	PowerOff(context.Context) error
}

// Unavailable is the inert controller used outside the appliance profile.
type Unavailable struct{}

func (Unavailable) Restart(context.Context) error  { return ErrUnavailable }
func (Unavailable) PowerOff(context.Context) error { return ErrUnavailable }
