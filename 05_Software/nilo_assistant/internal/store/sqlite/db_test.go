package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
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

	required := []string{
		"schema_migrations", "config_versions", "users", "mcp_tokens",
		"app_policies", "operation_policy_overrides", "device_policies",
		"operation_requests", "approval_records", "sessions", "resource_leases", "audit_events", "artifacts",
	}
	for _, table := range required {
		var count int
		if err := db.SQL().QueryRowContext(
			context.Background(),
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
			table,
		).Scan(&count); err != nil {
			t.Fatalf("look up table %q: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("table %q count = %d, want 1", table, count)
		}
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
	if got := queryInt(t, second.SQL(), "SELECT COUNT(*) FROM schema_migrations"); got != 1 {
		t.Fatalf("schema_migrations count = %d, want 1 after reopen", got)
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
