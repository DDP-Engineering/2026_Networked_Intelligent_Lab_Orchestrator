package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CheckpointAndInspect flushes the WAL, verifies database integrity and returns
// the ordered migration identity used to bind an offline snapshot.
func CheckpointAndInspect(ctx context.Context, path string) (schema string, err error) {
	database, err := Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	if _, err := database.SQL().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return "", fmt.Errorf("checkpoint SQLite before snapshot: %w", err)
	}
	rows, err := database.SQL().QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return "", fmt.Errorf("check SQLite integrity: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return "", err
		}
		if result != "ok" {
			return "", fmt.Errorf("SQLite integrity check failed: %s", result)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	var activeSessions int
	if err := database.SQL().QueryRowContext(ctx,
		"SELECT count(*) FROM sessions WHERE state IN ('pending_approval','queued','running')",
	).Scan(&activeSessions); err != nil {
		return "", fmt.Errorf("inspect active sessions before snapshot: %w", err)
	}
	if activeSessions != 0 {
		return "", fmt.Errorf("snapshot requires no active sessions: found %d", activeSessions)
	}
	migrationRows, err := database.SQL().QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return "", fmt.Errorf("read SQLite schema identity: %w", err)
	}
	defer migrationRows.Close()
	versions := make([]string, 0)
	for migrationRows.Next() {
		var version string
		if err := migrationRows.Scan(&version); err != nil {
			return "", err
		}
		versions = append(versions, version)
	}
	if err := migrationRows.Err(); err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", errors.New("SQLite schema migration identity is empty")
	}
	return strings.Join(versions, ","), nil
}

// ApplianceIdentity returns the durable owner identity used to bind destructive
// maintenance to the exact NILO data directory that created the marker.
func ApplianceIdentity(ctx context.Context, path string) (identity string, err error) {
	database, err := Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, database.Close()) }()
	if err := database.SQL().QueryRowContext(ctx, "SELECT appliance_id FROM appliance_identity WHERE id=1").Scan(&identity); err != nil {
		return "", fmt.Errorf("read appliance ownership identity: %w", err)
	}
	if identity == "" {
		return "", errors.New("appliance ownership identity is empty")
	}
	return identity, nil
}
