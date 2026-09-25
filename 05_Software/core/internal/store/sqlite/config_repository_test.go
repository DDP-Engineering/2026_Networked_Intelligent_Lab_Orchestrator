package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"testing"
)

func TestConfigRepositoryActivatesAndReloadsVersionedSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "nilo.db")
	database, err := Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	repository := NewConfigRepository(database)

	first, err := repository.ActivateValidated(ctx, []byte(`{"bind":"127.0.0.1:8080","device":{"id":"plc-1"}}`))
	if err != nil {
		t.Fatalf("activate first snapshot: %v", err)
	}
	second, err := repository.ActivateValidated(ctx, []byte(`{"device":{"id":"plc-2"},"bind":"127.0.0.1:8081"}`))
	if err != nil {
		t.Fatalf("activate second snapshot: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("configuration versions reused an identity")
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	reopened, err := Open(ctx, databasePath)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	active, err := NewConfigRepository(reopened).Active(ctx)
	if err != nil {
		t.Fatalf("load active snapshot: %v", err)
	}
	if active.ID != second.ID {
		t.Fatalf("active ID = %s, want %s", active.ID, second.ID)
	}
	if !bytes.Equal(active.ConfigurationJSON, second.ConfigurationJSON) {
		t.Fatalf("active configuration = %s, want %s", active.ConfigurationJSON, second.ConfigurationJSON)
	}
	if active.Digest != sha256.Sum256(active.ConfigurationJSON) {
		t.Fatal("active digest does not match canonical configuration bytes")
	}
	if active.CreatedAt.IsZero() || active.CreatedAt.Location().String() != "UTC" {
		t.Fatalf("created_at is not canonical UTC: %v", active.CreatedAt)
	}

	var activeCount int
	if err := reopened.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM config_versions WHERE active = 1").Scan(&activeCount); err != nil {
		t.Fatalf("count active versions: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active version count = %d, want 1", activeCount)
	}
}

func TestConfigRepositoryRejectsInvalidSnapshotsAndReportsNoActiveVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := NewConfigRepository(database)

	if _, err := repository.Active(ctx); !errors.Is(err, ErrNoActiveConfiguration) {
		t.Fatalf("Active error = %v, want ErrNoActiveConfiguration", err)
	}
	for _, input := range [][]byte{nil, []byte(`{"valid":true} trailing`), bytes.Repeat([]byte("x"), maxConfigurationBytes+1)} {
		if _, err := repository.ActivateValidated(ctx, input); !errors.Is(err, ErrInvalidConfigurationSnapshot) {
			t.Fatalf("ActivateValidated(%d bytes) error = %v, want ErrInvalidConfigurationSnapshot", len(input), err)
		}
	}
}

func TestConfigRepositoryActivationRollsBackWithoutLosingCurrentSelection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := Open(ctx, filepath.Join(t.TempDir(), "nilo.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := NewConfigRepository(database)
	first, err := repository.ActivateValidated(ctx, []byte(`{"version":1}`))
	if err != nil {
		t.Fatalf("activate first snapshot: %v", err)
	}
	if _, err := database.SQL().ExecContext(ctx, `
		CREATE TRIGGER reject_config_insert
		BEFORE INSERT ON config_versions
		BEGIN
			SELECT RAISE(ABORT, 'injected configuration insert failure');
		END
	`); err != nil {
		t.Fatalf("install failure trigger: %v", err)
	}
	if _, err := repository.ActivateValidated(ctx, []byte(`{"version":2}`)); err == nil {
		t.Fatal("activation unexpectedly succeeded through failure trigger")
	}
	active, err := repository.Active(ctx)
	if err != nil {
		t.Fatalf("reload prior active snapshot: %v", err)
	}
	if active.ID != first.ID {
		t.Fatalf("active ID after rollback = %s, want %s", active.ID, first.ID)
	}
}
