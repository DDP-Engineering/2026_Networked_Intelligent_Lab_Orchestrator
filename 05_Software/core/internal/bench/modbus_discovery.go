package bench

import (
	"context"
	"encoding/json"

	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

// reconcileModbusDiscovery accepts only the trusted discovery runner's metadata.
func (m *Manager) reconcileModbusDiscovery(ctx context.Context, raw json.RawMessage) ([]modbus.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reconcileModbusDiscoveryLocked(ctx, raw)
}

func (m *Manager) reconcileModbusDiscoveryLocked(ctx context.Context, raw json.RawMessage) ([]modbus.Endpoint, error) {
	if m.closed {
		return nil, ErrUnavailable
	}
	observations, err := serialDiscoveryObservations(raw)
	if err != nil {
		return nil, err
	}
	endpoints, err := sqlite.NewModbusRepository(m.options.Database).ReconcileUSB(ctx, observations, m.now())
	if err != nil {
		return nil, err
	}
	for _, e := range endpoints {
		if !e.Enabled || e.Presence != "present" || e.Readiness != "ready" {
			m.cancelTransportsLocked("modbus", e.ID)
			continue
		}
		// A changed tty/physical binding invalidates the captured transport snapshot.
		for _, claim := range m.transports {
			if claim.family == "modbus" && claim.endpoint == e.ID && claim.endpointRevision != e.Revision && claim.cancel != nil {
				claim.cancel()
			}
		}
	}
	return endpoints, nil
}

// observeModbusReadiness publishes a trusted transport observation without changing configuration.
func (m *Manager) observeModbusReadiness(ctx context.Context, id string, expected int64, readiness string) (modbus.Endpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return modbus.Endpoint{}, ErrUnavailable
	}
	if readiness != "ready" && readiness != "offline" && readiness != "fault" {
		return modbus.Endpoint{}, ErrInvalid
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	e, err := r.Endpoint(ctx, id)
	if err != nil {
		return modbus.Endpoint{}, modbusDefinitionError(err)
	}
	if e.Revision != expected {
		return modbus.Endpoint{}, ErrConflict
	}
	if !e.Enabled || (e.Transport == "rtu" && e.Presence != "present") {
		return modbus.Endpoint{}, ErrUnavailable
	}
	if e.Readiness != readiness {
		e.Readiness = readiness
		e, err = r.PutEndpoint(ctx, expected, e)
		if err != nil {
			return modbus.Endpoint{}, modbusDefinitionError(err)
		}
	}
	if readiness != "ready" {
		m.cancelTransportsLocked("modbus", id)
	}
	return e, nil
}
