package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"nilo.local/nilo/internal/modbus"
	"path/filepath"
	"testing"
)

func TestModbusMapRevisionTransaction(t *testing.T) {
	r := NewModbusRepository(openTemporaryDatabase(t))
	m := modbus.Map{SchemaVersion: 1, ID: "map", OwnerID: "alice", Name: "Map", Points: []modbus.Point{}}
	saved, err := r.PutMap(t.Context(), 0, m)
	if err != nil || saved.Revision != 1 {
		t.Fatal(saved, err)
	}
	saved.Name = "Changed"
	if _, err = r.PutMap(t.Context(), 0, saved); !errors.Is(err, ErrModbusConflict) {
		t.Fatal("stale save", err)
	}
	got, err := r.Map(t.Context(), "map")
	if err != nil || got.Name != "Map" {
		t.Fatal(got, err)
	}
	saved.OwnerID = "forged"
	saved, err = r.PutMap(t.Context(), 1, saved)
	if err != nil || saved.Revision != 2 || saved.OwnerID != "alice" {
		t.Fatal(saved, err)
	}
	first, err := r.MapRevision(t.Context(), "map", 1)
	if err != nil || first.Name != "Map" || first.Revision != 1 {
		t.Fatal("immutable revision", first, err)
	}
}

func TestModbusMigrationBackfillsExistingProfileRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "before-mb05.sqlite")
	earlier, err := sql.Open(driverName, sqliteDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	earlier.SetMaxOpenConns(1)
	if err = configureConnection(t.Context(), earlier); err != nil {
		t.Fatal(err)
	}
	if err = applyMigrationSet(t.Context(), earlier, migrationsUpToAndIncluding(t, "025_modbus_sessions")); err != nil {
		t.Fatal(err)
	}
	endpoint := modbus.Endpoint{SchemaVersion: 1, ID: "peer", Revision: 1, Name: "Peer", Transport: "tcp", ResourceID: "bus", Presence: "configured", Readiness: "offline", Network: &modbus.Network{Host: "peer.local", Port: 502, UnitMax: 255}}
	endpointRaw, _ := json.Marshal(endpoint)
	if _, err = earlier.Exec(`INSERT INTO modbus_endpoints(id,revision,document) VALUES(?,?,?)`, endpoint.ID, 1, string(endpointRaw)); err != nil {
		t.Fatal(err)
	}
	profile := modbus.Profile{SchemaVersion: 1, ID: "profile", Revision: 2, OwnerID: "owner", Name: "Saved", EndpointID: endpoint.ID, Settings: json.RawMessage(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0}`)}
	profileRaw, _ := json.Marshal(profile)
	if _, err = earlier.Exec(`INSERT INTO modbus_profiles(id,revision,endpoint_id,owner_id,document) VALUES(?,?,?,?,?)`, profile.ID, profile.Revision, profile.EndpointID, profile.OwnerID, string(profileRaw)); err != nil {
		t.Fatal(err)
	}
	if err = earlier.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	got, err := NewModbusRepository(upgraded).ProfileRevision(t.Context(), profile.ID, 2)
	if err != nil || got.Name != "Saved" {
		t.Fatal("backfill", got, err)
	}
}

func TestModbusProfileRevisionHistory(t *testing.T) {
	db := openTemporaryDatabase(t)
	r := NewModbusRepository(db)
	endpoint, err := r.PutEndpoint(t.Context(), 0, modbus.Endpoint{SchemaVersion: 1, ID: "peer", Name: "Peer", Transport: "tcp", ResourceID: "bus", Enabled: true, Presence: "configured", Readiness: "offline", Network: &modbus.Network{Host: "peer.local", Port: 502, UnitMax: 255}})
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0}`)
	profile, err := r.PutProfile(t.Context(), 0, modbus.Profile{SchemaVersion: 1, ID: "profile", OwnerID: "owner", Name: "Original", EndpointID: endpoint.ID, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	profile.Name = "Changed"
	if _, err = r.PutProfile(t.Context(), 1, profile); err != nil {
		t.Fatal(err)
	}
	old, err := r.ProfileRevision(t.Context(), "profile", 1)
	if err != nil || old.Name != "Original" {
		t.Fatal("historic profile", old, err)
	}
}
