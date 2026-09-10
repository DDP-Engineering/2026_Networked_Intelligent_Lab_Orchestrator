package nethelper

import (
	"context"
	"errors"
	"fmt"

	"nilo.local/nilo/internal/network"
)

// ManagementAccessDriver owns only NILO's dedicated input-filter table. It
// never receives raw nftables text from the unprivileged process.
type ManagementAccessDriver interface {
	ManagementAccessTable(context.Context) (TableState, error)
	InstallManagementAccess(context.Context, network.ManagementAccessConfiguration) error
}

var (
	ErrManagementAccessUnsupported = errors.New("this helper cannot enforce management access")
	ErrManagementAccessForeign     = errors.New("the nilo management firewall table exists but was not created by nilo-nethelper")
)

// ApplyManagementAccess validates and atomically installs the management
// listener guard through the privileged driver.
func (e *Engine) ApplyManagementAccess(ctx context.Context, configuration network.ManagementAccessConfiguration) error {
	if err := configuration.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, ok := e.driver.(ManagementAccessDriver)
	if !ok {
		return ErrManagementAccessUnsupported
	}
	state, err := driver.ManagementAccessTable(ctx)
	if err != nil {
		return err
	}
	if state == TableForeign {
		return ErrManagementAccessForeign
	}
	if err := driver.InstallManagementAccess(ctx, configuration); err != nil {
		return fmt.Errorf("install management firewall: %w", err)
	}
	return nil
}
