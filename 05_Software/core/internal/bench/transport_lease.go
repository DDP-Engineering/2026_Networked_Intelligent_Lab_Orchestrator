package bench

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

// transportLease is protected by Manager.mu; cancellation never releases keys.
// A failed cleanup keeps held=true and fences the resource for this Manager.
type transportLease struct {
	family, endpoint, profile string
	keys                      []string
	cancel                    context.CancelFunc
	done                      chan struct{}
	held                      bool
	cleanupErr                error
	endpointRevision          int64
}

func (m *Manager) transportBusyLocked(keys []string) bool {
	for _, key := range keys {
		if m.transports[key] != nil {
			return true
		}
	}
	return false
}

func (m *Manager) claimTransportLocked(family, endpoint, profile string, keys []string) (*transportLease, error) {
	if m.closed {
		return nil, ErrUnavailable
	}
	if len(keys) == 0 {
		return nil, ErrInvalid
	}
	if m.transportBusyLocked(keys) {
		return nil, ErrBusy
	}
	claim := &transportLease{family: family, endpoint: endpoint, profile: profile, keys: append([]string(nil), keys...), done: make(chan struct{}), held: true}
	for _, key := range keys {
		m.transports[key] = claim
	}
	return claim, nil
}

func (m *Manager) finishTransportLocked(claim *transportLease, cleanupErr error) {
	select {
	case <-claim.done:
		return
	default:
	}
	claim.cleanupErr = cleanupErr
	if cleanupErr == nil {
		for _, key := range claim.keys {
			if m.transports[key] == claim {
				delete(m.transports, key)
			}
		}
		claim.held = false
	}
	close(claim.done)
}

func (m *Manager) finishTransport(claim *transportLease, cleanupErr error) {
	m.mu.Lock()
	m.finishTransportLocked(claim, cleanupErr)
	m.mu.Unlock()
	if cleanupErr != nil {
		m.report(fmt.Errorf("%s/%s transport cleanup fenced: %w", claim.family, claim.endpoint, cleanupErr))
	}
}

func (m *Manager) transportFamilyBusyLocked(family string) bool {
	for _, claim := range m.transports {
		if claim.family == family {
			return true
		}
	}
	return false
}

func (m *Manager) cancelTransportsLocked(family, endpoint string) {
	for _, claim := range m.transports {
		if (family == "" || claim.family == family) && (endpoint == "" || claim.endpoint == endpoint) && claim.cancel != nil {
			claim.cancel()
		}
	}
}

func managedUSBIdentity(r sqlite.ManagedDeviceRecord) modbus.USBIdentity {
	h := modbus.USBIdentity{VendorID: r.Vendor, ProductID: r.Product, BusPath: r.PortPath}
	if !r.SerialWeak && r.IdentityRule == "serial" {
		serial := r.Serial
		h.SerialNumber = &serial
	}
	h.StableID = modbus.USBStableID(h)
	return h
}

func (m *Manager) sharedDeviceBusyLocked(ctx context.Context, family, id string) bool {
	if family != "serial" && family != "modbus" {
		return false
	}
	r, err := managedRecord(ctx, m, family, id)
	if err != nil {
		return false
	}
	return m.transportBusyLocked(modbus.USBResourceKeys(managedUSBIdentity(r)))
}

func (m *Manager) modbusProfileBusyLocked(id string) bool {
	for _, claim := range m.transports {
		if claim.family == "modbus" && claim.profile == id {
			return true
		}
	}
	return false
}

// runModbusConsumer is the trusted admission seam for an already authorized session.
// The lifetime function owns transport work and cleanup outside Manager.mu; its
// returned error means cleanup could not confirm release, not an operation error.
// Public policy, operations and the production runner are separate consumers.
func (m *Manager) runModbusConsumer(ctx context.Context, endpointID string, endpointRevision int64, profileID string, profileRevision int64, expires time.Time, lifetime func(context.Context, modbus.Endpoint, modbus.Profile) error) (context.CancelFunc, <-chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, nil, ErrUnavailable
	}
	if lifetime == nil || ctx.Err() != nil || !m.now().Before(expires) {
		return nil, nil, ErrInvalid
	}
	var installed, enabled bool
	if err := m.db().QueryRowContext(ctx, `SELECT installed,enabled FROM bench_plugins WHERE id='modbus'`).Scan(&installed, &enabled); err != nil || !installed || !enabled {
		return nil, nil, ErrUnavailable
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	e, err := r.Endpoint(ctx, endpointID)
	if err != nil {
		return nil, nil, modbusDefinitionError(err)
	}
	p, err := r.Profile(ctx, profileID)
	if err != nil {
		return nil, nil, modbusDefinitionError(err)
	}
	if e.Revision != endpointRevision || p.Revision != profileRevision || p.EndpointID != e.ID {
		return nil, nil, ErrConflict
	}
	if !e.Enabled || e.Readiness == "fault" || (e.Transport == "rtu" && (e.Presence != "present" || e.Readiness != "ready")) {
		return nil, nil, ErrUnavailable
	}
	claim, err := m.claimTransportLocked("modbus", e.ID, p.ID, e.ResourceKeys())
	if err != nil {
		return nil, nil, err
	}
	claim.endpointRevision = e.Revision
	runCtx, cancel := context.WithDeadline(ctx, expires)
	claim.cancel = cancel
	m.transportWorkers.Add(1)
	go func() {
		defer m.transportWorkers.Done()
		defer cancel()
		cleanupErr := lifetime(runCtx, e, p)
		m.finishTransport(claim, cleanupErr)
	}()
	return cancel, claim.done, nil
}

func (m *Manager) transportCleanupErrorsLocked() error {
	var result error
	seen := map[*transportLease]bool{}
	for _, claim := range m.transports {
		if !seen[claim] && claim.cleanupErr != nil {
			result = errors.Join(result, claim.cleanupErr)
			seen[claim] = true
		}
	}
	return result
}
