package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/network"
)

func snapshotTableRows(t *testing.T, database *sql.DB, table string) string {
	t.Helper()
	return snapshotRows(t, database, "SELECT * FROM "+table+" ORDER BY rowid")
}

func snapshotRows(t *testing.T, database *sql.DB, query string) string {
	t.Helper()
	rows, err := database.Query(query)
	if err != nil {
		t.Fatalf("snapshot %q: %v", query, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	result := make([][]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for index := range values {
			targets[index] = &values[index]
		}
		if err := rows.Scan(targets...); err != nil {
			t.Fatal(err)
		}
		for index, value := range values {
			if bytes, ok := value.([]byte); ok {
				values[index] = append([]byte(nil), bytes...)
			}
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// A clean installation has no appliance identity when the migration runs,
// because sqlite.Open applies the whole set before anything provisions one.
func TestNewDatabaseRecordsProvisioningCommissioningState(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "nilo.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})

	state, err := NewCommissioningRepository(database).LoadCommissioning(context.Background())
	if err != nil {
		t.Fatalf("LoadCommissioning: %v", err)
	}
	if state != appliance.CommissioningProvisioning {
		t.Fatalf("state = %q, want %q", state, appliance.CommissioningProvisioning)
	}
}

// An upgraded appliance already holds its identity when the migration arrives,
// and must not be pulled back into first-boot provisioning. This covers the
// interface-managed and the legacy single-listener deployment alike, because
// both hold the identity row and neither is detected separately.
func TestUpgradedDatabaseRecordsCommissionedCommissioningState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")

	earlier, err := sql.Open(driverName, sqliteDSN(path))
	if err != nil {
		t.Fatalf("open at the earlier schema: %v", err)
	}
	earlier.SetMaxOpenConns(1)
	if err := configureConnection(ctx, earlier); err != nil {
		t.Fatalf("configure: %v", err)
	}
	// Everything up to and including 012_management_access, which is what an
	// appliance running the previous release has.
	previous := migrations[:len(migrations)-2]
	if previous[len(previous)-1].version != "012_management_access" {
		t.Fatalf("migration before the commissioning one = %q, want 012_management_access",
			previous[len(previous)-1].version)
	}
	if err := applyMigrationSet(ctx, earlier, previous); err != nil {
		t.Fatalf("apply the earlier schema: %v", err)
	}
	if _, err := earlier.ExecContext(ctx, `
		INSERT INTO appliance_identity (id, appliance_id, name, location, commissioned_at)
		VALUES (1, '0b6e0d3a-1f2c-4a5b-9c8d-000000000001', 'deployed bench', 'lab', '2026-09-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("write the identity row at the earlier schema: %v", err)
	}
	if err := earlier.Close(); err != nil {
		t.Fatalf("close the earlier database: %v", err)
	}

	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade in place: %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })

	state, err := NewCommissioningRepository(upgraded).LoadCommissioning(ctx)
	if err != nil {
		t.Fatalf("LoadCommissioning: %v", err)
	}
	if state != appliance.CommissioningCommissioned {
		t.Fatalf("state = %q, want %q", state, appliance.CommissioningCommissioned)
	}

	var applianceID, name string
	if err := upgraded.SQL().QueryRowContext(ctx,
		"SELECT appliance_id, name FROM appliance_identity WHERE id = 1").Scan(&applianceID, &name); err != nil {
		t.Fatalf("read the identity row after upgrade: %v", err)
	}
	if applianceID != "0b6e0d3a-1f2c-4a5b-9c8d-000000000001" || name != "deployed bench" {
		t.Fatalf("the stored identity changed on upgrade: %q/%q", applianceID, name)
	}
	if got := queryInt(t, upgraded.SQL(), "SELECT COUNT(*) FROM schema_migrations"); got != len(migrations) {
		t.Fatalf("schema_migrations count = %d, want %d", got, len(migrations))
	}
}

func TestNetworkCommissioningResetClearsOnlyLifecycleManagementAndRecovery(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewCommissioningRepository(database)
	if err := repository.SaveCommissioning(ctx, appliance.CommissioningCommissioned); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL().ExecContext(ctx, `
		INSERT INTO appliance_identity (id, appliance_id, name, location, commissioned_at)
		VALUES (1, '0b6e0d3a-1f2c-4a5b-9c8d-000000000001', 'deployed bench', 'lab', '2026-09-01T00:00:00Z')
	`); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO users (id, username, password_hash, created_at, updated_at, role)
		 VALUES ('user-1', 'admin', 'password-verifier', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z', 'administrator')`,
		`INSERT INTO mcp_tokens (id, user_id, verifier, created_at, name, scopes)
		 VALUES ('token-1', 'user-1', X'010203', '2026-09-01T00:00:00Z', 'automation', 'network.read')`,
		`INSERT INTO user_sessions (id, user_id, verifier, csrf_verifier, created_at, expires_at)
		 VALUES ('browser-1', 'user-1', X'040506', X'070809', '2026-09-01T00:00:00Z', '2026-10-01T00:00:00Z')`,
		`INSERT INTO app_policies (id, app_id, mode, created_at, updated_at)
		 VALUES ('integration-policy-1', 'sigrok', 'ask', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`,
		`INSERT INTO bench_plugins (id, instance_id, installed, enabled, mode, digest, devices_json)
		 VALUES ('sigrok', 'instance-1', 1, 1, 'ask', 'sha256:test', '[{"id":"plc-tool"}]')`,
		`INSERT INTO vpn_profiles (id, name, provider, public_config_json, secret_ref, connection_ref, revision, start_selected, created_at, updated_at)
		 VALUES ('vpn-1', 'uplink', 'wireguard', '{}', 'secret-1', 'connection-1', 'rev-1', 1, '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`,
		`INSERT INTO network_interface_settings (adapter_id, connection_ref, method, address, gateway, dns_json, dns_automatic, management_protected, never_default, wifi_ssid, wifi_security, wifi_startup, wifi_secret_ref, revision, updated_at)
		 VALUES ('plc-adapter', 'plc-profile', 'static', '192.168.0.2/24', '', '[]', 0, 0, 1, '', '', 0, '', 'rev-plc', '2026-09-01T00:00:00Z')`,
		`INSERT INTO device_networks (id, name, location, device_interface, appliance_address, scope, target, addressing, exposed, access, local_interface, local_sources_json, vpn_profile_id, vpn_sources_json, services_json, enabled, revision, created_at, updated_at)
		 VALUES ('device-network-1', 'PLC line', 'lab', 'plc-adapter', '192.168.0.2/24', 'host', '192.168.0.130', 'original', '', 'wireguard', '', '[]', 'vpn-1', '["10.205.148.0/24"]', '[{"protocol":"tcp","port":502}]', 1, 'rev-device', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`,
		`INSERT INTO audit_events (id, sequence, actor_id, action, subject_type, subject_id, payload_json, created_at)
		 VALUES ('audit-before', 1, '00000000-0000-0000-0000-000000000001', 'historical.event', 'appliance', '0b6e0d3a-1f2c-4a5b-9c8d-000000000001', '{}', '2026-09-01T00:00:00Z')`,
	} {
		if _, err := database.SQL().ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	access := network.ManagementAccessRecord{
		Configuration: network.ManagementAccessConfiguration{Interfaces: []string{"eth0"}, Port: 8443, TLSNames: []string{"ddp-remote.local"}},
		LastKnownGood: network.ManagementAccessConfiguration{Interfaces: []string{"eth0"}, Port: 8443, TLSNames: []string{"ddp-remote.local"}},
		State:         network.ManagementAccessApplied, UpdatedAt: now,
	}
	if err := NewManagementAccessRepository(database).SaveManagementAccess(ctx, access); err != nil {
		t.Fatal(err)
	}
	if err := NewHardwareRecoveryRepository(database).SaveHardwareRecovery(ctx, network.RecoveryRecord{
		Version: 1, State: network.RecoveryArmed, ArmedAt: now, ExpiresAt: now.Add(10 * time.Minute), UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	derived := network.ManagementAccessRecord{
		Configuration: network.ManagementAccessConfiguration{Interfaces: []string{"eth1"}, Port: 8443, TLSNames: []string{"nilo.local"}},
		LastKnownGood: network.ManagementAccessConfiguration{Interfaces: []string{"eth1"}, Port: 8443, TLSNames: []string{"nilo.local"}},
		State:         network.ManagementAccessApplied, UpdatedAt: now.Add(time.Second),
	}
	protectedTables := []string{
		"appliance_identity", "users", "mcp_tokens", "user_sessions", "app_policies", "bench_plugins",
		"vpn_profiles", "network_interface_settings", "device_networks",
	}
	before := make(map[string]string, len(protectedTables))
	for _, table := range protectedTables {
		before[table] = snapshotTableRows(t, database.SQL(), table)
	}
	historicalAudit := snapshotRows(t, database.SQL(), "SELECT * FROM audit_events WHERE sequence = 1")
	actor, err := domain.ParseActorID(audit.SystemActorID)
	if err != nil {
		t.Fatal(err)
	}
	resetAudit := audit.Event{
		ID: "00000000-0000-4000-8000-000000000099", ActorID: actor,
		Action: "network.commissioning_reset", SubjectType: "appliance",
		SubjectID: "0b6e0d3a-1f2c-4a5b-9c8d-000000000001", Payload: json.RawMessage(`{"scope":"network_access_only"}`),
		CreatedAt: now.Add(time.Second),
	}
	if err := repository.ResetNetworkCommissioningAudited(ctx, derived, resetAudit, 23); err != nil {
		t.Fatal(err)
	}
	state, err := repository.LoadCommissioning(ctx)
	if err != nil || state != appliance.CommissioningProvisioning {
		t.Fatalf("commissioning after reset = %q/%v", state, err)
	}
	resetAccess, found, err := NewManagementAccessRepository(database).LoadManagementAccess(ctx)
	if err != nil || !found || !slices.Equal(resetAccess.Configuration.Interfaces, []string{"eth1"}) {
		t.Fatalf("derived management access after reset = %+v found=%t err=%v", resetAccess, found, err)
	}
	recovery, found, err := NewHardwareRecoveryRepository(database).LoadHardwareRecovery(ctx)
	if err != nil || !found || recovery.State != network.RecoveryIdle || recovery.HardwareGeneration != 23 || recovery.HelperRecordID != "" {
		t.Fatalf("recovery tombstone after reset=%+v found=%t err=%v", recovery, found, err)
	}
	for _, table := range protectedTables {
		if after := snapshotTableRows(t, database.SQL(), table); after != before[table] {
			t.Fatalf("protected table %s changed:\nbefore=%s\nafter=%s", table, before[table], after)
		}
	}
	if after := snapshotRows(t, database.SQL(), "SELECT * FROM audit_events WHERE sequence = 1"); after != historicalAudit {
		t.Fatalf("historical audit row changed:\nbefore=%s\nafter=%s", historicalAudit, after)
	}
	if got := queryString(t, database.SQL(), "SELECT action FROM audit_events WHERE id = '00000000-0000-4000-8000-000000000099'"); got != "network.commissioning_reset" {
		t.Fatalf("reset audit event = %q", got)
	}
	if err := repository.ResetNetworkCommissioning(ctx, derived); err == nil {
		t.Fatal("a second reset from provisioning was accepted")
	}
}

// The two CHECK constraints are what make the boundary observable at the
// database, not only in Go.
func TestCommissioningTableRejectsAnUnknownStateAndASecondRow(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "nilo.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})

	for _, testCase := range []struct {
		name      string
		statement string
	}{
		{
			name:      "unknown state",
			statement: "UPDATE commissioning SET state = 'decommissioned' WHERE singleton = 1",
		},
		{
			name:      "second row",
			statement: "INSERT INTO commissioning (singleton, state) VALUES (2, 'provisioning')",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := database.SQL().ExecContext(context.Background(), testCase.statement); err == nil {
				t.Fatalf("statement %q was accepted", testCase.statement)
			}
		})
	}
}

func TestCommissioningRepositoryRefusesAnAbsentRow(t *testing.T) {
	t.Parallel()

	database, err := Open(context.Background(), filepath.Join(t.TempDir(), "nilo.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	if _, err := database.SQL().ExecContext(context.Background(), "DELETE FROM commissioning"); err != nil {
		t.Fatalf("remove the recorded state: %v", err)
	}

	_, err = NewCommissioningRepository(database).LoadCommissioning(context.Background())
	if err == nil {
		t.Fatal("LoadCommissioning returned no error for a database with no recorded state")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "commissioning") {
		t.Fatalf("error %q does not name the missing commissioning state", err)
	}
}
