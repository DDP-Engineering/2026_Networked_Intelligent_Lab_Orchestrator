package nethelper

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/network"
)

// DirectRecoveryDriver applies one helper-owned volatile recovery profile and
// removes precisely that profile later.
type DirectRecoveryDriver interface {
	ActivateRecoveryLink(context.Context, string, string, string) error
	RestoreRecoveryLink(context.Context, DirectRecoveryRecord) error
}

// ErrDirectRecoveryUnsupported means this helper has no safe platform adapter.
var ErrDirectRecoveryUnsupported = errors.New("temporary direct-connect recovery is unavailable")

const maximumDirectRecoveryWindow = 24 * time.Hour

// DirectRecoveryRecord is the helper-owned durable intent for one volatile
// recovery profile. It contains only public adapter identity and expiry data.
type DirectRecoveryRecord struct {
	Version       int       `json:"version"`
	ID            string    `json:"id"`
	Phase         string    `json:"phase"`
	AdapterID     string    `json:"adapter_id"`
	InterfaceName string    `json:"interface_name"`
	Deadline      time.Time `json:"deadline"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// DirectRecoveryStore keeps the helper's independent recovery deadline.
type DirectRecoveryStore interface {
	LoadDirectRecovery() (*DirectRecoveryRecord, error)
	SaveDirectRecovery(DirectRecoveryRecord) error
	ClearDirectRecovery() error
}

func validDirectRecoveryRecord(record DirectRecoveryRecord) bool {
	if record.Version != 1 || uuid.Validate(record.ID) != nil || strings.TrimSpace(record.AdapterID) == "" ||
		strings.TrimSpace(record.InterfaceName) == "" || record.Deadline.IsZero() || record.UpdatedAt.IsZero() {
		return false
	}
	switch record.Phase {
	case "intent", "active", "activation_failed", "restoring", "restore_failed":
		return true
	default:
		return false
	}
}

func validateRecoveryDevice(device Device, adapterID, interfaceName string) error {
	observed := device.Interface
	if observed.Kind != network.KindEthernet || !observed.Physical || !observed.Controllable {
		return errors.New("recovery requires a managed physical Ethernet adapter")
	}
	if !strings.EqualFold(observed.AdapterID, strings.TrimSpace(adapterID)) || observed.Name != interfaceName {
		return errors.New("recovery adapter identity changed")
	}
	if !observed.LinkUp && observed.State != network.StateConnected || device.Transitioning {
		return errors.New("recovery link is not stably connected")
	}
	if device.Connection != "" || len(device.Available) != 0 || observed.Settings != nil {
		return errors.New("recovery adapter has an active or saved profile")
	}
	if len(observed.IPv4) != 0 || hasConfiguredIPv6(observed.IPv6) || observed.Gateway4 != "" || observed.Gateway6 != "" {
		return errors.New("recovery adapter already has network configuration")
	}
	return nil
}

func hasConfiguredIPv6(addresses []string) bool {
	for _, value := range addresses {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !prefix.Addr().Is6() || !prefix.Addr().IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}

func (e *Engine) ActivateRecoveryLink(ctx context.Context, adapterID, interfaceName string, deadline time.Time) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, ok := e.driver.(DirectRecoveryDriver)
	if !ok || e.directRecoveryStore == nil {
		return "", ErrDirectRecoveryUnsupported
	}
	if e.directRecovery != nil {
		return e.directRecovery.ID, network.ErrChangeInProgress
	}
	now := e.now().UTC()
	deadline = deadline.UTC()
	if !deadline.After(now) || deadline.After(now.Add(maximumDirectRecoveryWindow)) {
		return "", errors.New("invalid direct recovery deadline")
	}
	recordID := uuid.NewString()
	record := DirectRecoveryRecord{
		Version: 1, ID: recordID, Phase: "intent", AdapterID: adapterID,
		InterfaceName: interfaceName, Deadline: deadline, UpdatedAt: now,
	}
	if err := e.directRecoveryStore.SaveDirectRecovery(record); err != nil {
		return "", fmt.Errorf("save direct recovery intent: %w", err)
	}
	e.directRecovery = &record
	e.trackDirectRecoveryDeadline(now, deadline)
	if err := driver.ActivateRecoveryLink(ctx, recordID, adapterID, interfaceName); err != nil {
		record.Phase = "activation_failed"
		record.UpdatedAt = e.now().UTC()
		_ = e.directRecoveryStore.SaveDirectRecovery(record)
		e.directRecovery = &record
		return recordID, err
	}
	record.Phase = "active"
	if err := e.directRecoveryStore.SaveDirectRecovery(record); err != nil {
		return recordID, fmt.Errorf("save active direct recovery: %w", err)
	}
	e.directRecovery = &record
	return recordID, nil
}

func (e *Engine) ExtendRecoveryLink(_ context.Context, recordID string, deadline time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.directRecoveryStore == nil {
		return ErrDirectRecoveryUnsupported
	}
	if e.directRecovery == nil || e.directRecovery.ID != recordID || e.directRecovery.Phase != "active" {
		return network.ErrOperationNotFound
	}
	now := e.now().UTC()
	deadline = deadline.UTC()
	if !deadline.After(now) || deadline.After(now.Add(maximumDirectRecoveryWindow)) {
		return errors.New("invalid direct recovery deadline")
	}
	record := *e.directRecovery
	record.Deadline = deadline
	record.UpdatedAt = now
	if err := e.directRecoveryStore.SaveDirectRecovery(record); err != nil {
		return fmt.Errorf("extend direct recovery: %w", err)
	}
	e.directRecovery = &record
	e.trackDirectRecoveryDeadline(now, deadline)
	return nil
}

func (e *Engine) RestoreRecoveryLink(ctx context.Context, recordID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.restoreDirectRecoveryLocked(ctx, recordID, false)
}

func (e *Engine) restoreDirectRecoveryLocked(ctx context.Context, recordID string, force bool) error {
	driver, ok := e.driver.(DirectRecoveryDriver)
	if !ok || e.directRecoveryStore == nil {
		return ErrDirectRecoveryUnsupported
	}
	if e.directRecovery == nil {
		return nil
	}
	if !force && e.directRecovery.ID != recordID {
		return network.ErrOperationNotFound
	}
	record := *e.directRecovery
	record.Phase = "restoring"
	record.UpdatedAt = e.now().UTC()
	if err := e.directRecoveryStore.SaveDirectRecovery(record); err != nil {
		return fmt.Errorf("save direct recovery restoration: %w", err)
	}
	e.directRecovery = &record
	if err := driver.RestoreRecoveryLink(ctx, record); err != nil {
		record.Phase = "restore_failed"
		record.UpdatedAt = e.now().UTC()
		_ = e.directRecoveryStore.SaveDirectRecovery(record)
		e.directRecovery = &record
		return err
	}
	if err := e.directRecoveryStore.ClearDirectRecovery(); err != nil {
		return fmt.Errorf("clear direct recovery intent: %w", err)
	}
	e.directRecovery = nil
	return nil
}

// ExpireDirectRecovery is the helper-owned failsafe. It also rolls back any
// interrupted activation/restoration intent immediately after helper restart.
func (e *Engine) ExpireDirectRecovery(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.directRecovery == nil {
		return nil
	}
	wallNow := e.now().UTC()
	if e.directRecovery.Phase == "active" && !e.directRecoveryDeadlineElapsed(wallNow) {
		if wallNow.After(e.directRecovery.UpdatedAt) {
			record := *e.directRecovery
			record.UpdatedAt = wallNow
			if err := e.directRecoveryStore.SaveDirectRecovery(record); err != nil {
				return fmt.Errorf("save direct recovery clock observation: %w", err)
			}
			e.directRecovery = &record
		}
		return nil
	}
	return e.restoreDirectRecoveryLocked(ctx, e.directRecovery.ID, true)
}

func (e *Engine) trackDirectRecoveryDeadline(wallNow, deadline time.Time) {
	steadyNow := e.steadyNow()
	remaining := deadline.Sub(wallNow)
	if e.directRecovery != nil && !e.directRecovery.UpdatedAt.IsZero() && wallNow.Before(e.directRecovery.UpdatedAt) {
		remaining = 0
	}
	if remaining < 0 {
		remaining = 0
	}
	e.recoverySteadyUntil = steadyNow.Add(remaining)
	e.recoverySteadySeen = steadyNow
	e.recoveryWallSeen = wallNow
	if e.directRecovery != nil && e.directRecovery.UpdatedAt.After(e.recoveryWallSeen) {
		e.recoveryWallSeen = e.directRecovery.UpdatedAt
	}
}

func (e *Engine) directRecoveryDeadlineElapsed(wallNow time.Time) bool {
	steadyNow := e.steadyNow()
	rollback := !e.recoveryWallSeen.IsZero() && wallNow.Before(e.recoveryWallSeen)
	steadyRollback := !e.recoverySteadySeen.IsZero() && steadyNow.Before(e.recoverySteadySeen)
	if wallNow.After(e.recoveryWallSeen) {
		e.recoveryWallSeen = wallNow
	}
	if steadyNow.After(e.recoverySteadySeen) {
		e.recoverySteadySeen = steadyNow
	}
	return rollback || steadyRollback || !wallNow.Before(e.directRecovery.Deadline) ||
		e.recoverySteadyUntil.IsZero() || !steadyNow.Before(e.recoverySteadyUntil)
}

func (e *Engine) restoreDirectRecoveryNow(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.directRecovery == nil {
		return nil
	}
	return e.restoreDirectRecoveryLocked(ctx, e.directRecovery.ID, true)
}
