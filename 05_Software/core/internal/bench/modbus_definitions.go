package bench

import (
	"context"
	"errors"
	"reflect"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

func modbusDefinitionError(err error) error {
	switch {
	case errors.Is(err, sqlite.ErrModbusConflict):
		return ErrConflict
	case errors.Is(err, sqlite.ErrModbusNotFound):
		return ErrNotFound
	case errors.Is(err, sqlite.ErrModbusInvalid), errors.Is(err, modbus.ErrInvalid):
		return ErrInvalid
	default:
		return err
	}
}

// ModbusEndpoints lists shared definitions, including disabled or offline endpoints.
func (m *Manager) ModbusEndpoints(ctx context.Context, actor identity.Actor) ([]modbus.Endpoint, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return nil, err
	}
	return sqlite.NewModbusRepository(m.options.Database).Endpoints(ctx)
}

// ModbusProfiles lists shared transport profiles for Bench readers.
func (m *Manager) ModbusProfiles(ctx context.Context, actor identity.Actor) ([]modbus.Profile, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return nil, err
	}
	return sqlite.NewModbusRepository(m.options.Database).Profiles(ctx)
}

// SaveModbusEndpoint changes administrator-owned configuration without transport I/O.
// USB binding and observed presence/readiness remain Core-owned.
func (m *Manager) SaveModbusEndpoint(ctx context.Context, actor identity.Actor, expected int64, e modbus.Endpoint) (modbus.Endpoint, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return modbus.Endpoint{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return modbus.Endpoint{}, ErrUnavailable
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	old, err := r.Endpoint(ctx, e.ID)
	if err != nil && !errors.Is(err, sqlite.ErrModbusNotFound) {
		return modbus.Endpoint{}, err
	}
	missing := errors.Is(err, sqlite.ErrModbusNotFound)
	if expected < 0 || expected >= 2147483647 {
		return modbus.Endpoint{}, ErrInvalid
	}
	if (missing && expected != 0) || (!missing && expected != old.Revision) {
		return modbus.Endpoint{}, ErrConflict
	}
	if e.Transport == "rtu" {
		if err != nil || old.Transport != "rtu" || !reflect.DeepEqual(e.SerialIdentity, old.SerialIdentity) || e.ResourceID != old.ResourceID {
			return modbus.Endpoint{}, ErrInvalid
		}
		e.Binding, e.BindingRevision, e.Presence, e.Readiness = old.Binding, old.BindingRevision, old.Presence, old.Readiness
		if !e.Enabled {
			e.Readiness = "unconfigured"
		} else if e.Presence == "present" {
			e.Readiness = "ready"
		} else {
			e.Readiness = "offline"
		}
	} else {
		if old.Transport == "rtu" || e.SerialIdentity != nil || e.Binding != "" {
			return modbus.Endpoint{}, ErrInvalid
		}
		e.Presence, e.Readiness = "configured", "offline"
		if err == nil && old.Transport == e.Transport && reflect.DeepEqual(old.Network, e.Network) {
			e.Readiness = old.Readiness
		}
	}
	e.Revision = expected + 1
	e, err = e.Canonical()
	if err != nil {
		return modbus.Endpoint{}, ErrInvalid
	}
	if !missing && (m.transportBusyLocked(old.ResourceKeys()) || m.transportBusyLocked(e.ResourceKeys())) {
		// Disable changes admission only; other edits wait for transport cleanup.
		disabled := old
		disabled.Enabled = false
		candidate := e
		candidate.Revision, candidate.Binding, candidate.BindingRevision, candidate.Presence, candidate.Readiness = old.Revision, old.Binding, old.BindingRevision, old.Presence, old.Readiness
		if e.Enabled || !reflect.DeepEqual(disabled, candidate) {
			return modbus.Endpoint{}, ErrBusy
		}
	}
	if err := m.record(ctx, actor, "modbus.endpoint_save_requested", e.ID, map[string]any{"revision": expected}); err != nil {
		return modbus.Endpoint{}, err
	}
	e, err = r.PutEndpoint(ctx, expected, e)
	if err != nil {
		return modbus.Endpoint{}, modbusDefinitionError(err)
	}
	if !e.Enabled {
		m.cancelTransportsLocked("modbus", e.ID)
	}
	if err := m.record(ctx, actor, "modbus.endpoint_saved", e.ID, map[string]any{"revision": e.Revision, "enabled": e.Enabled}); err != nil {
		return modbus.Endpoint{}, err
	}
	return e, nil
}

// SaveModbusProfile validates settings against the saved endpoint and retains its creator.
func (m *Manager) SaveModbusProfile(ctx context.Context, actor identity.Actor, expected int64, p modbus.Profile) (modbus.Profile, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return modbus.Profile{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return modbus.Profile{}, ErrUnavailable
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	old, err := r.Profile(ctx, p.ID)
	missing := errors.Is(err, sqlite.ErrModbusNotFound)
	if err != nil && !missing {
		return modbus.Profile{}, err
	}
	if expected < 0 || expected >= 2147483647 {
		return modbus.Profile{}, ErrInvalid
	}
	if (missing && expected != 0) || (!missing && expected != old.Revision) {
		return modbus.Profile{}, ErrConflict
	}
	if m.modbusProfileBusyLocked(p.ID) {
		return modbus.Profile{}, ErrBusy
	}
	e, err := r.Endpoint(ctx, p.EndpointID)
	if err != nil {
		return modbus.Profile{}, modbusDefinitionError(err)
	}
	p.OwnerID, p.Revision = actor.User.ID, expected+1
	if !missing {
		p.OwnerID = old.OwnerID
	}
	if _, err = p.Canonical(e); err != nil {
		return modbus.Profile{}, ErrInvalid
	}
	if err = m.record(ctx, actor, "modbus.profile_save_requested", p.ID, map[string]any{"revision": expected}); err != nil {
		return modbus.Profile{}, err
	}
	p, err = r.PutProfile(ctx, expected, p)
	if err != nil {
		return modbus.Profile{}, modbusDefinitionError(err)
	}
	if err = m.record(ctx, actor, "modbus.profile_saved", p.ID, map[string]any{"revision": p.Revision, "endpoint_id": p.EndpointID}); err != nil {
		return modbus.Profile{}, err
	}
	return p, nil
}
