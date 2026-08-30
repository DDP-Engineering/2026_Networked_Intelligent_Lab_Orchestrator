package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"time"
)

type migration struct {
	version string
	sql     string
}

//go:embed migrations/001_initial.sql
var initialMigration string

//go:embed migrations/002_policy_snapshots.sql
var policySnapshotsMigration string

var migrations = []migration{
	{version: "001_initial", sql: initialMigration},
	{version: "002_policy_snapshots", sql: policySnapshotsMigration},
}

func applyMigrations(ctx context.Context, database *sql.DB) (err error) {
	connection, err := database.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve SQLite migration connection: %w", err)
	}
	defer func() {
		closeErr := connection.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("release SQLite migration connection: %w", closeErr)
		}
	}()

	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate SQLite migration transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	if _, err := connection.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			digest TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema migration ledger: %w", err)
	}

	for _, migration := range migrations {
		digest := migrationDigest(migration.sql)
		var recordedDigest string
		err := connection.QueryRowContext(
			ctx,
			"SELECT digest FROM schema_migrations WHERE version = ?",
			migration.version,
		).Scan(&recordedDigest)
		switch {
		case err == nil:
			if recordedDigest != digest {
				return fmt.Errorf("migration %s digest mismatch", migration.version)
			}
			continue
		case err != sql.ErrNoRows:
			return fmt.Errorf("read migration %s ledger record: %w", migration.version, err)
		}

		if _, err := connection.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.version, err)
		}
		if _, err := connection.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version, digest, applied_at) VALUES (?, ?, ?)",
			migration.version,
			digest,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.version, err)
		}
	}

	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit SQLite migration transaction: %w", err)
	}
	committed = true
	return nil
}

func migrationDigest(sql string) string {
	digest := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(digest[:])
}
