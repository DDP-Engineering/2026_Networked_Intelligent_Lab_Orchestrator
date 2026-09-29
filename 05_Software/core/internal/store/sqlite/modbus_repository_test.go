package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"nilo.local/nilo/internal/modbus"
)

func TestModbusDefinitionTablesAreDurableEntities(t *testing.T) {
	db := openTemporaryDatabase(t)
	for _, table := range []string{"modbus_endpoints", "modbus_profiles"} {
		var count int
		if err := db.SQL().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("missing durable Modbus entity table %s", table)
		}
	}
}

func TestModbusDefinitionsReopenAndRevisionConflicts(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "modbus.sqlite")
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := NewModbusRepository(db)
	e := modbus.Endpoint{SchemaVersion: 1, ID: "peer", Name: "Gateway", Transport: "tcp", ResourceID: "bus-a", Enabled: true, Presence: "configured", Readiness: "offline", Network: &modbus.Network{Host: "example.local", Port: 502, UnitMax: 255}}
	e, err = r.PutEndpoint(ctx, 0, e)
	if err != nil || e.Revision != 1 {
		t.Fatalf("create: %+v %v", e, err)
	}
	p, err := r.PutProfile(ctx, 0, modbus.Profile{SchemaVersion: 1, ID: "profile", Name: "Default", OwnerID: "creator", EndpointID: e.ID, Settings: json.RawMessage(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0}`)})
	if err != nil || p.Revision != 1 {
		t.Fatalf("profile: %+v %v", p, err)
	}
	e.Name = "Retained"
	e, err = r.PutEndpoint(ctx, e.Revision, e)
	if err != nil || e.Revision != 2 {
		t.Fatal(e, err)
	}
	stale := e
	stale.Name = "Stale"
	if _, err = r.PutEndpoint(ctx, 1, stale); !errors.Is(err, ErrModbusConflict) {
		t.Fatal("stale replacement", err)
	}
	p.OwnerID = "forged"
	p.Name = "Updated"
	p, err = r.PutProfile(ctx, p.Revision, p)
	if err != nil || p.OwnerID != "creator" || p.Revision != 2 {
		t.Fatal("retained creator", p, err)
	}
	if _, err = r.PutProfile(ctx, 1, p); !errors.Is(err, ErrModbusConflict) {
		t.Fatal("stale profile", err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	r = NewModbusRepository(db)
	got, err := r.Endpoint(ctx, e.ID)
	if err != nil || got.Name != "Retained" || got.Revision != 2 || got.Network == nil || got.SerialIdentity != nil {
		t.Fatalf("reopen: %+v %v", got, err)
	}
	profiles, err := r.Profiles(ctx)
	if err != nil || len(profiles) != 1 || profiles[0].OwnerID != "creator" || profiles[0].Revision != 2 {
		t.Fatal(profiles, err)
	}
	endpoints, err := r.Endpoints(ctx)
	if err != nil || len(endpoints) != 1 || endpoints[0].Readiness != "offline" {
		t.Fatal(endpoints, err)
	}
	got.Transport = "rtu_tcp"
	if _, err = r.PutEndpoint(ctx, got.Revision, got); !errors.Is(err, ErrModbusInvalid) {
		t.Fatal("profile transport invalidated", err)
	}
}

func TestModbusConcurrentCASHasOneWinnerAndNoPartialPublication(t *testing.T) {
	db := openTemporaryDatabase(t)
	r := NewModbusRepository(db)
	e, err := r.PutEndpoint(t.Context(), 0, validModbusNetworkEndpoint("race"))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, name := range []string{"A", "B"} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			copy := e
			copy.Name = name
			_, err := r.PutEndpoint(context.Background(), e.Revision, copy)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	winners, conflicts := 0, 0
	for err := range results {
		if err == nil {
			winners++
		} else if errors.Is(err, ErrModbusConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	got, err := r.Endpoint(t.Context(), e.ID)
	if err != nil || winners != 1 || conflicts != 1 || got.Revision != 2 || (got.Name != "A" && got.Name != "B") {
		t.Fatalf("winners=%d conflicts=%d stored=%+v err=%v", winners, conflicts, got, err)
	}
	p := modbus.Profile{SchemaVersion: 1, ID: "missing", OwnerID: "creator", EndpointID: "unknown", Settings: json.RawMessage(`{}`)}
	if _, err = r.PutProfile(t.Context(), 0, p); !errors.Is(err, ErrModbusNotFound) {
		t.Fatal("missing endpoint", err)
	}
	if _, err = r.Profile(t.Context(), p.ID); !errors.Is(err, ErrModbusNotFound) {
		t.Fatal("partially published profile", err)
	}
}

func validModbusNetworkEndpoint(id string) modbus.Endpoint {
	return modbus.Endpoint{SchemaVersion: 1, ID: id, Name: "Gateway", Transport: "tcp", ResourceID: "bus-a", Enabled: true, Presence: "configured", Readiness: "offline", Network: &modbus.Network{Host: "gateway.local", Port: 502, UnitMax: 255}}
}

func TestModbusRejectsInvalidTransportBeforePublication(t *testing.T) {
	r := NewModbusRepository(openTemporaryDatabase(t))
	for _, tc := range []struct {
		name   string
		change func(*modbus.Endpoint)
	}{
		{"url", func(e *modbus.Endpoint) { e.Network.Host = "tcp://gateway.local" }},
		{"path", func(e *modbus.Endpoint) { e.Network.Host = "/dev/ttyUSB0" }},
		{"zone", func(e *modbus.Endpoint) { e.Network.Host = "fe80::1%eth0" }},
		{"port", func(e *modbus.Endpoint) { e.Network.Port = 0 }},
		{"range", func(e *modbus.Endpoint) { e.Network.UnitMin = 4; e.Network.UnitMax = 3 }},
		{"unit", func(e *modbus.Endpoint) { e.Network.UnitMax = 256 }},
		{"rtu_tcp_zero", func(e *modbus.Endpoint) { e.Transport = "rtu_tcp" }},
		{"mixed", func(e *modbus.Endpoint) { e.SerialIdentity = &modbus.USBIdentity{} }},
		{"missing_network", func(e *modbus.Endpoint) { e.Network = nil }},
		{"rtu_network", func(e *modbus.Endpoint) { e.Transport = "rtu" }},
		{"bad_transport", func(e *modbus.Endpoint) { e.Transport = "udp" }},
		{"bad_id", func(e *modbus.Endpoint) { e.ID = "../bad" }},
		{"fake_presence", func(e *modbus.Endpoint) { e.Presence = "present" }},
		{"bad_name", func(e *modbus.Endpoint) { e.Name = "\x00" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := validModbusNetworkEndpoint(tc.name)
			tc.change(&e)
			if _, err := r.PutEndpoint(t.Context(), 0, e); !errors.Is(err, ErrModbusInvalid) {
				t.Fatalf("invalid transport persisted: %v", err)
			}
			if _, err := r.Endpoint(t.Context(), e.ID); !errors.Is(err, ErrModbusNotFound) {
				t.Fatalf("invalid endpoint published: %v", err)
			}
		})
	}
}

func TestModbusProfilesRequireExplicitTransportSettings(t *testing.T) {
	r := NewModbusRepository(openTemporaryDatabase(t))
	e, err := r.PutEndpoint(t.Context(), 0, validModbusNetworkEndpoint("peer"))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{}`, `null`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0}`,
		`{"response_timeout_ms":0,"connect_timeout_ms":1,"inter_request_ms":0,"read_retries":0}`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":-1,"read_retries":0}`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0,"read_retries":2147483648}`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0,"read_retries":null}`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0,"read_retries":0,"baud_rate":9600}`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0,"read_retries":0,"command":"open"}`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0,"READ_RETRIES":0}`,
	} {
		p := modbus.Profile{SchemaVersion: 1, ID: "invalid", OwnerID: "creator", Name: "Profile", EndpointID: e.ID, Settings: json.RawMessage(raw)}
		if _, err := r.PutProfile(t.Context(), 0, p); !errors.Is(err, ErrModbusInvalid) {
			t.Fatalf("settings %s persisted: %v", raw, err)
		}
	}
}
