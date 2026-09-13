package nethelper

import (
	"context"
	"errors"
	"fmt"

	"nilo.local/nilo/internal/network"
)

// MDNSDriver owns the appliance's local-name responder. It never receives
// responder configuration as text from the unprivileged process: the caller
// sends a validated advertisement and this package renders it.
type MDNSDriver interface {
	// MDNSConfigurationState reports whether the responder configuration on
	// this appliance is one NILO wrote, one somebody else owns, or absent.
	MDNSConfigurationState(context.Context) (TableState, error)
	// InstallMDNS writes the rendered configuration and reloads the responder.
	InstallMDNS(context.Context, network.MDNSAdvertisement) error
}

var (
	// ErrMDNSUnsupported is a helper with no responder to drive, which leaves
	// the appliance reachable by address while name discovery is unavailable.
	ErrMDNSUnsupported = errors.New("this helper cannot publish a local name")
	// ErrMDNSForeign is responder configuration this appliance already had.
	// NILO manages only what it wrote itself and never takes over another.
	ErrMDNSForeign = errors.New("the responder configuration exists but was not created by nilo-nethelper")
)

// PublishMDNS validates the advertisement and installs it through the
// privileged driver.
func (e *Engine) PublishMDNS(ctx context.Context, advertisement network.MDNSAdvertisement) error {
	if err := advertisement.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, ok := e.driver.(MDNSDriver)
	if !ok {
		return ErrMDNSUnsupported
	}
	state, err := driver.MDNSConfigurationState(ctx)
	if err != nil {
		return err
	}
	if state == TableForeign {
		return ErrMDNSForeign
	}
	if err := driver.InstallMDNS(ctx, advertisement); err != nil {
		return fmt.Errorf("publish local name: %w", err)
	}
	return nil
}
