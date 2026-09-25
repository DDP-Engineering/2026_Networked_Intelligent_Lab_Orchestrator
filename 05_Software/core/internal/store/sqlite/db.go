package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const driverName = "sqlite"

// DB owns the Increment 1 SQLite connection pool.
type DB struct {
	sql *sql.DB
}

// Open opens an absolute SQLite database path, configures the durable
// single-writer connection, and applies verified schema migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, errors.New("database path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("database path must be absolute: %q", path)
	}

	database, err := sql.Open(driverName, sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("connect SQLite database: %w", err)
	}
	if err := configureConnection(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := applyMigrations(ctx, database); err != nil {
		_ = database.Close()
		return nil, err
	}

	return &DB{sql: database}, nil
}

// SQL exposes the underlying connection for repository implementations.
func (db *DB) SQL() *sql.DB {
	return db.sql
}

// Close releases database resources.
func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

func configureConnection(ctx context.Context, database *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = FULL",
	}
	for _, pragma := range pragmas {
		if _, err := database.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure SQLite with %q: %w", pragma, err)
		}
	}
	return nil
}

func sqliteDSN(path string) string {
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		uriPath = "/" + uriPath
	}
	connectionURL := &url.URL{Scheme: "file", Path: uriPath}
	query := connectionURL.Query()
	query.Add("_pragma", "foreign_keys(ON)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(FULL)")
	connectionURL.RawQuery = query.Encode()
	return connectionURL.String()
}
