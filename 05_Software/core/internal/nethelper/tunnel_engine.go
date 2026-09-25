package nethelper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"nilo.local/nilo/internal/network"
)

// The tunnel half of the privileged engine. A NILO-owned WireGuard connection
// is created here from key material the keystore holds, activated and
// deactivated under a system-wide checkpoint, and listed without its key.

// TunnelDriver is what the NetworkManager adapter provides for tunnels. The
// fake driver in the tests provides the same, so the engine's decisions are
// proven without a bus.
type TunnelDriver interface {
	// CreateTunnel creates an inactive, non-autoconnecting connection from the
	// public profile and its key material and returns its identity.
	CreateTunnel(ctx context.Context, profileID string, profile network.Profile, secrets network.Secrets) (string, error)
	DeleteTunnel(ctx context.Context, connectionRef string) error
	Tunnels(ctx context.Context) ([]network.TunnelObservation, error)
	ActivateTunnel(ctx context.Context, connectionRef string) error
	DeactivateTunnel(ctx context.Context, connectionRef string) error
}

// SystemSnapshot is what a tunnel change is put back to: the NILO-owned tunnel
// that was up, and a digest of every other device's connection, address, route
// and DNS state, so a restoration is verified against the whole appliance and
// not only the tunnel.
type SystemSnapshot struct {
	ActiveTunnel string `json:"active_tunnel"`
	Digest       string `json:"digest"`
}

// ErrTunnelUnsupported means this helper's driver cannot manage tunnels.
var ErrTunnelUnsupported = errors.New("this helper cannot manage WireGuard tunnels")

func isTunnelAction(action network.Action) bool {
	return action == network.ActionVPNActivate || action == network.ActionVPNDeactivate
}

func (e *Engine) tunnelDriver() (TunnelDriver, error) {
	driver, ok := e.driver.(TunnelDriver)
	if !ok {
		return nil, ErrTunnelUnsupported
	}
	return driver, nil
}

// findTunnel returns the observation for one connection reference.
func findTunnel(tunnels []network.TunnelObservation, connectionRef string) (network.TunnelObservation, bool) {
	for _, tunnel := range tunnels {
		if tunnel.ConnectionRef == connectionRef {
			return tunnel, true
		}
	}
	return network.TunnelObservation{}, false
}

// activeOwnedTunnel returns the NILO-owned tunnel that is up, if any.
func activeOwnedTunnel(tunnels []network.TunnelObservation) (network.TunnelObservation, bool) {
	for _, tunnel := range tunnels {
		if tunnel.NILOOwned && tunnel.Active {
			return tunnel, true
		}
	}
	return network.TunnelObservation{}, false
}

// inspectTunnel validates a tunnel change against the live inventory. The
// change names the connection the unprivileged side resolved, and it must be
// one this helper created: an external tunnel is never a target.
func (e *Engine) inspectTunnel(ctx context.Context, c network.Change) error {
	driver, err := e.tunnelDriver()
	if err != nil {
		return err
	}
	if c.ConnectionID == "" || c.Enabled || c.Name != "" || c.Profile != nil || c.SecretHandle != "" {
		return network.ErrUnsupported
	}
	tunnels, err := driver.Tunnels(ctx)
	if err != nil {
		return err
	}
	tunnel, found := findTunnel(tunnels, c.ConnectionID)
	if !found {
		return fmt.Errorf("%w: no connection %s", network.ErrUnsupported, c.ConnectionID)
	}
	if !tunnel.NILOOwned {
		return fmt.Errorf("%w: %s is not managed by NILO", network.ErrExternalProfile, tunnel.Name)
	}
	switch c.Action {
	case network.ActionVPNActivate:
		if tunnel.Active {
			return fmt.Errorf("%w: %s is already connected", network.ErrProfileActive, tunnel.Name)
		}
	case network.ActionVPNDeactivate:
		if !tunnel.Active {
			return fmt.Errorf("%w: %s is not connected", network.ErrNotAwaitingConfirm, tunnel.Name)
		}
	}
	return nil
}

// snapshot captures what a tunnel change is measured against afterwards.
func (e *Engine) snapshot(ctx context.Context) (SystemSnapshot, error) {
	driver, err := e.tunnelDriver()
	if err != nil {
		return SystemSnapshot{}, err
	}
	tunnels, err := driver.Tunnels(ctx)
	if err != nil {
		return SystemSnapshot{}, err
	}
	devices, err := e.driver.Devices(ctx)
	if err != nil {
		return SystemSnapshot{}, err
	}
	return systemSnapshot(devices, tunnels), nil
}

// systemSnapshot digests the non-tunnel devices and records the active
// NILO-owned tunnel. Tunnel devices are left out of the digest because the
// change under way creates and removes them; everything else must come back
// exactly.
func systemSnapshot(devices []Device, tunnels []network.TunnelObservation) SystemSnapshot {
	tunnelDevices := map[string]bool{}
	for _, tunnel := range tunnels {
		if tunnel.Interface != "" {
			tunnelDevices[tunnel.Interface] = true
		}
	}
	lines := make([]string, 0, len(devices))
	for _, device := range devices {
		if tunnelDevices[device.Interface.Name] || device.Tunnel {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s\x00%s\x00%s\x00%t\x00%s",
			device.Interface.ID, device.Connection, device.Interface.State, device.AutoConnect, device.NetworkState))
	}
	sort.Strings(lines)
	digest := sha256.New()
	for _, line := range lines {
		digest.Write([]byte(line))
		digest.Write([]byte{'\n'})
	}
	snapshot := SystemSnapshot{Digest: hex.EncodeToString(digest.Sum(nil))}
	if active, ok := activeOwnedTunnel(tunnels); ok {
		snapshot.ActiveTunnel = active.ConnectionRef
	}
	return snapshot
}

// applyTunnel makes the recorded tunnel change: a switch deactivates the tunnel
// that was up before it activates the new one, so at most one NILO-owned
// tunnel is ever connected.
func (e *Engine) applyTunnel(ctx context.Context, r *Record) error {
	driver, err := e.tunnelDriver()
	if err != nil {
		return err
	}
	switch r.Change.Action {
	case network.ActionVPNActivate:
		if r.System != nil && r.System.ActiveTunnel != "" && r.System.ActiveTunnel != r.Change.ConnectionID {
			if err := driver.DeactivateTunnel(ctx, r.System.ActiveTunnel); err != nil {
				return fmt.Errorf("deactivate the previous tunnel: %w", err)
			}
		}
		return driver.ActivateTunnel(ctx, r.Change.ConnectionID)
	case network.ActionVPNDeactivate:
		return driver.DeactivateTunnel(ctx, r.Change.ConnectionID)
	default:
		return network.ErrUnsupported
	}
}

// CreateProfile promotes the staged key material to the profile identity,
// creates the connection from it, and on failure removes the promoted copy so
// nothing is left on disk for a profile that does not exist.
func (e *Engine) CreateProfile(ctx context.Context, handle, profileID string, c network.Change) (string, error) {
	driver, err := e.tunnelDriver()
	if err != nil {
		return "", err
	}
	keystore, err := e.keys()
	if err != nil {
		return "", err
	}
	if err := keystore.Promote(handle, profileID); err != nil {
		return "", err
	}
	secrets, err := keystore.Read(profileID)
	if err != nil {
		_ = keystore.Remove(profileID)
		return "", err
	}
	if len(secrets.PresharedKeys) != len(c.Profile.Peers) {
		_ = keystore.Remove(profileID)
		return "", errors.New("the staged key material does not match the profile's peers")
	}
	connectionRef, err := driver.CreateTunnel(ctx, profileID, *c.Profile, secrets)
	if err != nil {
		_ = keystore.Remove(profileID)
		return "", fmt.Errorf("create the tunnel connection: %w", err)
	}
	return connectionRef, nil
}

// DeleteProfile deletes a NILO-owned connection and its key material. A
// connection that is up or that NILO did not create is refused; one that is
// already gone is not an error, so a retried deletion converges.
func (e *Engine) DeleteProfile(ctx context.Context, profileID, connectionRef string) error {
	driver, err := e.tunnelDriver()
	if err != nil {
		return err
	}
	tunnels, err := driver.Tunnels(ctx)
	if err != nil {
		return err
	}
	if tunnel, found := findTunnel(tunnels, connectionRef); found {
		if !tunnel.NILOOwned {
			return fmt.Errorf("%w: %s is not managed by NILO", network.ErrExternalProfile, tunnel.Name)
		}
		if tunnel.Active {
			return fmt.Errorf("%w: %s is connected", network.ErrProfileActive, tunnel.Name)
		}
		if err := driver.DeleteTunnel(ctx, connectionRef); err != nil {
			return fmt.Errorf("delete the tunnel connection: %w", err)
		}
	}
	keystore, err := e.keys()
	if err != nil {
		return err
	}
	return keystore.Remove(profileID)
}

// Tunnels lists every WireGuard connection on the host.
func (e *Engine) Tunnels(ctx context.Context) ([]network.TunnelObservation, error) {
	driver, err := e.tunnelDriver()
	if err != nil {
		return nil, err
	}
	tunnels, err := driver.Tunnels(ctx)
	if err != nil {
		return nil, err
	}
	if tunnels == nil {
		tunnels = []network.TunnelObservation{}
	}
	return tunnels, nil
}

// ConnectStartup activates the startup selection without a confirmation
// window. It is refused while any change is pending or being recovered, and
// while a different NILO-owned tunnel is up, because at most one is.
func (e *Engine) ConnectStartup(ctx context.Context, connectionRef string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, err := e.tunnelDriver()
	if err != nil {
		return err
	}
	if pending(e.record) || e.installationPending() {
		return network.ErrChangeInProgress
	}
	tunnels, err := driver.Tunnels(ctx)
	if err != nil {
		return err
	}
	tunnel, found := findTunnel(tunnels, connectionRef)
	if !found {
		return fmt.Errorf("%w: no connection %s", network.ErrUnsupported, connectionRef)
	}
	if !tunnel.NILOOwned {
		return fmt.Errorf("%w: %s is not managed by NILO", network.ErrExternalProfile, tunnel.Name)
	}
	if tunnel.Active {
		return nil
	}
	if active, ok := activeOwnedTunnel(tunnels); ok {
		return fmt.Errorf("%w: %s is already connected", network.ErrProfileActive, active.Name)
	}
	return driver.ActivateTunnel(ctx, connectionRef)
}
