package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	firstSessionID  = "018f3d8d-8d2c-7c20-8e0c-000000000001"
	secondSessionID = "018f3d8d-8d2c-7c20-8e0c-000000000002"
	firstLeaseID    = "018f3d8d-8d2c-7c20-8e0c-000000000003"
	secondLeaseID   = "018f3d8d-8d2c-7c20-8e0c-000000000004"
	unknownUserID   = "018f3d8d-8d2c-7c20-8e0c-000000000005"
	unknownTokenID  = "018f3d8d-8d2c-7c20-8e0c-000000000006"
	createdAt       = "2026-08-30T12:00:00.000000000Z"
	resourceName    = "fake-python/oscilloscope/bench-a"
)

func TestOpenConfiguresDurableDatabaseAndMigratesAllFoundationTables(t *testing.T) {
	db := openTemporaryDatabase(t)

	if got := queryInt(t, db.SQL(), "PRAGMA foreign_keys"); got != 1 {
		t.Fatalf("foreign_keys = %d, want 1", got)
	}
	if got := strings.ToLower(queryString(t, db.SQL(), "PRAGMA journal_mode")); got != "wal" {
		t.Fatalf("journal_mode = %q, want wal", got)
	}
	if got := queryInt(t, db.SQL(), "PRAGMA busy_timeout"); got != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", got)
	}
	if got := queryInt(t, db.SQL(), "PRAGMA synchronous"); got != 2 {
		t.Fatalf("synchronous = %d, want FULL (2)", got)
	}
	if got := db.SQL().Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}

	wantTables := []string{
		"activity_policy", "app_policies", "appliance_identity", "approval_records", "artifacts",
		"audit_events", "audit_sequence", "bench_device_sequences", "bench_devices", "bench_jobs", "bench_plugin_configuration", "bench_plugins", "bench_weak_serials", "commissioning",
		"config_versions", "device_networks", "device_policies", "identify_operations", "management_access", "mcp_tokens", "network_interface_settings", "network_operations", "network_recovery",
		"network_traffic_minute",
		"operation_policy_overrides",
		"operation_requests", "password_policy", "policy_snapshots", "resource_leases", "retention_watermarks", "schema_migrations", "sessions",
		"user_sessions", "users", "vpn_profiles",
	}
	if gotTables := nonInternalTableNames(t, db.SQL()); !slices.Equal(gotTables, wantTables) {
		t.Fatalf("non-internal tables = %q, want %q", gotTables, wantTables)
	}
}

func TestOpenIsIdempotentForAnAlreadyMigratedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	second, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("second close: %v", err)
		}
	})
	if got := queryInt(t, second.SQL(), "SELECT COUNT(*) FROM schema_migrations"); got != len(migrations) {
		t.Fatalf("schema_migrations count = %d, want %d after reopen", got, len(migrations))
	}
}

func TestNonInternalTableNamesIncludesSQLitePrefixWithoutInternalUnderscore(t *testing.T) {
	db := openTemporaryDatabase(t)
	const tableName = "sqliteXunexpected"
	if _, err := db.SQL().ExecContext(context.Background(), "CREATE TABLE sqliteXunexpected (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create discriminating non-internal SQLite table: %v", err)
	}
	if !slices.Contains(nonInternalTableNames(t, db.SQL()), tableName) {
		t.Fatalf("non-internal table names omitted %q", tableName)
	}
}

func TestOpenRejectsChangedRecordedMigrationDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("initial open: %v", err)
	}
	if _, err := db.SQL().ExecContext(context.Background(), "UPDATE schema_migrations SET digest = ?", "tampered-digest"); err != nil {
		t.Fatalf("tamper recorded migration digest: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close after tampering: %v", err)
	}

	_, err = Open(context.Background(), path)
	if err == nil {
		t.Fatal("Open accepted a changed recorded migration digest")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "digest") {
		t.Fatalf("Open error = %v, want migration digest rejection", err)
	}
}

func TestPolicySnapshotsEnforceEnumBooleanAndNullabilityChecks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*policySnapshotRow)
	}{
		{name: "app known boolean", mutate: func(row *policySnapshotRow) { row.appKnown = 2 }},
		{name: "app mode enum", mutate: func(row *policySnapshotRow) { row.appMode = "operator" }},
		{name: "manifest policy enum", mutate: func(row *policySnapshotRow) { row.manifestPolicy = "permit" }},
		{name: "override enum", mutate: func(row *policySnapshotRow) { row.operationOverride = "permit" }},
		{name: "device known boolean", mutate: func(row *policySnapshotRow) { row.deviceKnown = 2 }},
		{name: "known device requires mode", mutate: func(row *policySnapshotRow) { row.deviceMode = nil }},
		{name: "unknown device forbids mode", mutate: func(row *policySnapshotRow) { row.deviceKnown = 0 }},
		{name: "effective mode enum", mutate: func(row *policySnapshotRow) { row.effectiveMode = "operator" }},
		{name: "effective policy enum", mutate: func(row *policySnapshotRow) { row.effectivePolicy = "permit" }},
		{name: "decision enum", mutate: func(row *policySnapshotRow) { row.decision = "maybe" }},
		{name: "bounded reason enum", mutate: func(row *policySnapshotRow) { row.reason = strings.Repeat("x", 65) }},
		{name: "UTC resolution time", mutate: func(row *policySnapshotRow) { row.resolvedAt = "2026-08-30T15:04:05+03:00" }},
		{name: "malformed timestamp ending Z", mutate: func(row *policySnapshotRow) { row.resolvedAt = "xxxxxxxxxxxxxxxxxxxZ" }},
		{name: "noncanonical timestamp separator", mutate: func(row *policySnapshotRow) { row.resolvedAt = "2026-08-30 12:04:05Z" }},
		{name: "noncanonical fractional trailing zero", mutate: func(row *policySnapshotRow) { row.resolvedAt = "2026-08-30T12:04:05.120Z" }},
		{name: "invalid timestamp month", mutate: func(row *policySnapshotRow) { row.resolvedAt = "2026-13-30T12:04:05Z" }},
		{name: "invalid timestamp hour", mutate: func(row *policySnapshotRow) { row.resolvedAt = "2026-08-30T25:04:05Z" }},
		{name: "invalid timestamp calendar day", mutate: func(row *policySnapshotRow) { row.resolvedAt = "2026-02-30T12:04:05Z" }},
		{name: "invalid timestamp leap day", mutate: func(row *policySnapshotRow) { row.resolvedAt = "2025-02-29T12:04:05Z" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openTemporaryDatabase(t)
			row := validPolicySnapshotRow()
			test.mutate(&row)
			_, err := db.SQL().ExecContext(context.Background(), `
				INSERT INTO policy_snapshots (
					id, integration_id, operation_name, device_id,
					app_known, app_mode, manifest_policy, operation_override,
					device_known, device_mode, effective_mode, effective_policy,
					decision, reason, resolved_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				row.id, row.integrationID, row.operationName, row.deviceID,
				row.appKnown, row.appMode, row.manifestPolicy, row.operationOverride,
				row.deviceKnown, row.deviceMode, row.effectiveMode, row.effectivePolicy,
				row.decision, row.reason, row.resolvedAt,
			)
			if err == nil {
				t.Fatal("policy_snapshots accepted an invalid checked value")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "constraint") {
				t.Fatalf("insert error = %v, want constraint rejection", err)
			}
		})
	}
}

func TestPolicySnapshotsAcceptCanonicalUTCTimestampPrecision(t *testing.T) {
	for index, timestamp := range []string{
		"2026-08-30T12:04:05Z",
		"2026-08-30T12:04:05.1Z",
		"2026-08-30T12:04:05.999999999Z",
	} {
		t.Run(timestamp, func(t *testing.T) {
			db := openTemporaryDatabase(t)
			row := validPolicySnapshotRow()
			row.id = fmt.Sprintf("018f3d8d-8d2c-7c20-8e0c-%012d", 410+index)
			row.resolvedAt = timestamp
			_, err := db.SQL().ExecContext(context.Background(), `
				INSERT INTO policy_snapshots (
					id, integration_id, operation_name, device_id,
					app_known, app_mode, manifest_policy, operation_override,
					device_known, device_mode, effective_mode, effective_policy,
					decision, reason, resolved_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				row.id, row.integrationID, row.operationName, row.deviceID,
				row.appKnown, row.appMode, row.manifestPolicy, row.operationOverride,
				row.deviceKnown, row.deviceMode, row.effectiveMode, row.effectivePolicy,
				row.decision, row.reason, row.resolvedAt,
			)
			if err != nil {
				t.Fatalf("policy_snapshots rejected canonical timestamp %q: %v", timestamp, err)
			}
		})
	}
}

func TestDatabaseRejectsTokenForMissingUser(t *testing.T) {
	db := openTemporaryDatabase(t)
	_, err := db.SQL().ExecContext(
		context.Background(),
		"INSERT INTO mcp_tokens (id, user_id, verifier, created_at) VALUES (?, ?, ?, ?)",
		unknownTokenID,
		unknownUserID,
		[]byte("deterministic-token-verifier"),
		createdAt,
	)
	if err == nil {
		t.Fatal("mcp_tokens accepted a user_id with no users row")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("insert error = %v, want foreign-key rejection", err)
	}
}

func TestDatabaseRejectsSecondActiveLeaseForTheSameResource(t *testing.T) {
	db := openTemporaryDatabase(t)
	// This test isolates the partial unique index. Task 8 owns session creation,
	// so it uses deterministic placeholder session IDs without bypassing the
	// database's real lease constraint.
	if _, err := db.SQL().ExecContext(context.Background(), "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys while seeding lease-index fixture: %v", err)
	}
	if _, err := db.SQL().ExecContext(
		context.Background(),
		"INSERT INTO resource_leases (id, session_id, resource_name, acquired_at, released_at) VALUES (?, ?, ?, ?, NULL)",
		firstLeaseID,
		firstSessionID,
		resourceName,
		createdAt,
	); err != nil {
		t.Fatalf("insert first active lease: %v", err)
	}
	_, err := db.SQL().ExecContext(
		context.Background(),
		"INSERT INTO resource_leases (id, session_id, resource_name, acquired_at, released_at) VALUES (?, ?, ?, ?, NULL)",
		secondLeaseID,
		secondSessionID,
		resourceName,
		createdAt,
	)
	if err == nil {
		t.Fatal("second active lease for the same resource was accepted")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") {
		t.Fatalf("second lease error = %v, want partial unique-index rejection", err)
	}
}

func openTemporaryDatabase(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "nilo.sqlite"))
	if err != nil {
		t.Fatalf("open temporary database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close temporary database: %v", err)
		}
	})
	return db
}

func queryInt(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var got int
	if err := db.QueryRowContext(context.Background(), query).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return got
}

func queryString(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	var got string
	if err := db.QueryRowContext(context.Background(), query).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return got
}

func nonInternalTableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(
		context.Background(),
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT GLOB 'sqlite_*' ORDER BY name",
	)
	if err != nil {
		t.Fatalf("list non-internal SQLite tables: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan SQLite table name: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate SQLite table names: %v", err)
	}
	return names
}

type policySnapshotRow struct {
	id, integrationID, operationName, deviceID              string
	appKnown, deviceKnown                                   int
	appMode, manifestPolicy, effectiveMode, effectivePolicy string
	operationOverride, deviceMode                           any
	decision, reason, resolvedAt                            string
}

func validPolicySnapshotRow() policySnapshotRow {
	return policySnapshotRow{
		id:                "018f3d8d-8d2c-7c20-8e0c-000000000301",
		integrationID:     "018f3d8d-8d2c-7c20-8e0c-000000000302",
		operationName:     "capture",
		deviceID:          "018f3d8d-8d2c-7c20-8e0c-000000000303",
		appKnown:          1,
		appMode:           "auto",
		manifestPolicy:    "auto_eligible",
		operationOverride: nil,
		deviceKnown:       1,
		deviceMode:        "auto",
		effectiveMode:     "auto",
		effectivePolicy:   "auto_eligible",
		decision:          "allow",
		reason:            "policy_matrix_applied",
		resolvedAt:        "2026-08-30T12:04:05.123456789Z",
	}
}
