package bench

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
)

type modbusDefinitionService interface {
	ModbusEndpoints(context.Context, identity.Actor) ([]modbus.Endpoint, error)
	ModbusProfiles(context.Context, identity.Actor) ([]modbus.Profile, error)
	SaveModbusEndpoint(context.Context, identity.Actor, int64, modbus.Endpoint) (modbus.Endpoint, error)
	SaveModbusProfile(context.Context, identity.Actor, int64, modbus.Profile) (modbus.Profile, error)
}

func modbusService(t *testing.T, m *Manager) modbusDefinitionService {
	t.Helper()
	service, ok := any(m).(modbusDefinitionService)
	if !ok {
		t.Fatal("Core shared Modbus definition service is unavailable")
	}
	return service
}

func networkDefinition(id, bus string) modbus.Endpoint {
	return modbus.Endpoint{SchemaVersion: 1, ID: id, Name: "Gateway", Transport: "tcp", ResourceID: bus, Enabled: true, Presence: "configured", Readiness: "offline", Network: &modbus.Network{Host: "gateway.local", Port: 502, UnitMax: 255}}
}

func profileDefinition(id, endpoint string) modbus.Profile {
	return modbus.Profile{SchemaVersion: 1, ID: id, Name: "Default", OwnerID: "forged", EndpointID: endpoint, Settings: json.RawMessage(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0}`)}
}

func TestModbusSharedDefinitionsAuthorizeRolesAndRetainCreator(t *testing.T) {
	m, admin, _ := testManager(t, "auto")
	service := modbusService(t, m)
	for _, role := range []auth.Role{auth.RoleOperator, auth.RoleViewer} {
		actor := admin
		actor.User.Role = role
		actor.Permissions = role.Permissions()
		if _, err := service.ModbusEndpoints(t.Context(), actor); err != nil {
			t.Fatal(role, err)
		}
		if _, err := service.ModbusProfiles(t.Context(), actor); err != nil {
			t.Fatal(role, err)
		}
		if _, err := service.SaveModbusEndpoint(t.Context(), actor, 0, networkDefinition("forbidden", "bus")); !errors.Is(err, ErrForbidden) {
			t.Fatal("nonadmin endpoint mutation", err)
		}
		if _, err := service.SaveModbusProfile(t.Context(), actor, 0, profileDefinition("forbidden", "peer")); !errors.Is(err, ErrForbidden) {
			t.Fatal("nonadmin profile mutation", err)
		}
	}
	e, err := service.SaveModbusEndpoint(t.Context(), admin, 0, networkDefinition("peer", "bus"))
	if err != nil || e.Revision != 1 || e.Readiness != "offline" {
		t.Fatal(e, err)
	}
	p, err := service.SaveModbusProfile(t.Context(), admin, 0, profileDefinition("profile", e.ID))
	if err != nil || p.OwnerID != admin.User.ID {
		t.Fatal("server owned creator", p, err)
	}
	p.OwnerID = "forged"
	p.Name = "Updated"
	p, err = service.SaveModbusProfile(t.Context(), admin, p.Revision, p)
	if err != nil || p.OwnerID != admin.User.ID || p.Revision != 2 {
		t.Fatal(p, err)
	}
	if _, err = service.SaveModbusEndpoint(t.Context(), admin, 0, e); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision", err)
	}
	var count int
	if err = m.db().QueryRow(`SELECT count(*) FROM audit_events WHERE action='modbus.profile_saved'`).Scan(&count); err != nil || count != 2 {
		t.Fatal("profile audit", count, err)
	}
	if _, err = service.SaveModbusEndpoint(t.Context(), admin, 0, modbus.Endpoint{SchemaVersion: 1, ID: "fake-usb", Transport: "rtu", SerialIdentity: &modbus.USBIdentity{}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("public USB identity accepted", err)
	}
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	if _, err = service.SaveModbusEndpoint(t.Context(), admin, e.Revision, e); !errors.Is(err, ErrUnavailable) {
		t.Fatal("closed manager mutation", err)
	}
}
