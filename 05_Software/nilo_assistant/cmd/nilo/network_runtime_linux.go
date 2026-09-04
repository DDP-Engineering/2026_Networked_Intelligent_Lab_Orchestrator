//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/nethelper"
	"nilo.local/nilo/internal/network"
)

// newApplianceProvider composes the live provider: the helper client for
// adapters, recovery, installation and key staging, and the profile manager
// for the saved WireGuard inventory. The saved-profile store is attached later
// by serve, once the database is open.
func newApplianceProvider(ctx context.Context, socket string) (networkProvider, error) {
	if os.Geteuid() == 0 {
		return nil, errors.New("Bench Core must run as an unprivileged service user; only nilo-nethelper runs as root")
	}
	if !filepath.IsAbs(socket) {
		return nil, errors.New("appliance requires an absolute --nethelper-socket")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	client := nethelper.NewClient(socket, network.Source(hostname))
	if err = client.Reconcile(ctx); err != nil {
		return nil, fmt.Errorf("network recovery blocks appliance startup: %w", err)
	}
	if _, err = client.Interfaces(ctx); err != nil {
		return nil, err
	}
	profiles, err := network.NewProfileManager(network.ProfileManagerOptions{
		Driver: client,
		Now:    func() time.Time { return time.Now().UTC() },
		NewID:  uuid.NewString,
	})
	if err != nil {
		return nil, err
	}
	appliance := network.NewAppliance(client, profiles)
	// The helper renders device-network forwarding into its own table.
	appliance.UseForwarder(client)
	return appliance, nil
}
