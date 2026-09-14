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
	// ReplaceMDNS replaces only the helper process's Avahi entry group.
	ReplaceMDNS(context.Context, network.MDNSAdvertisement) error
	WithdrawMDNS(context.Context) error
}

type MDNSStateDriver interface {
	MDNSPublicationCurrent(context.Context) (bool, error)
}

func (e *Engine) MDNSPublicationCurrent(ctx context.Context) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, ok := e.driver.(MDNSStateDriver)
	if !ok {
		return false, ErrMDNSUnsupported
	}
	return driver.MDNSPublicationCurrent(ctx)
}

func (e *Engine) WithdrawMDNS(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, ok := e.driver.(MDNSDriver)
	if !ok {
		return ErrMDNSUnsupported
	}
	if err := driver.WithdrawMDNS(ctx); err != nil {
		return fmt.Errorf("withdraw local name: %w", err)
	}
	return nil
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
	if err := driver.ReplaceMDNS(ctx, advertisement); err != nil {
		return fmt.Errorf("publish local name: %w", err)
	}
	return nil
}
