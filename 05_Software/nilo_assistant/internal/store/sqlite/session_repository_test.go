package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/session"
)

func TestSessionRepositoryUsesCompareAndSwapAndExclusiveLeases(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewSessionRepository(database)
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	first := createStoredSession(t, ctx, repository, domain.Queued, now)
	second := createStoredSession(t, ctx, repository, domain.Queued, now)

	if _, err := repository.Transition(ctx, first.ID, domain.Running, domain.Completed, now.Add(time.Second)); !errors.Is(err, session.ErrSessionStateConflict) {
		t.Fatalf("Transition() error = %v, want ErrSessionStateConflict", err)
	}
	resource := "018f3d8d-8d2c-7c20-8e0c-000000000201/oscilloscope/bench-a"
	if err := repository.AcquireLeases(ctx, first.ID, []string{resource, resource}, now); err != nil {
		t.Fatalf("first AcquireLeases() error = %v", err)
	}
	err := repository.AcquireLeases(ctx, second.ID, []string{resource}, now)
	var busy *session.ResourceBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("second AcquireLeases() error = %v, want ResourceBusyError", err)
	}
	if busy.OwnerSessionID != first.ID {
		t.Fatalf("busy owner = %s, want %s", busy.OwnerSessionID, first.ID)
	}
	if _, err := repository.Transition(ctx, first.ID, domain.Queued, domain.Running, now.Add(time.Second)); err != nil {
		t.Fatalf("Transition queued to running error = %v", err)
	}
	if _, err := repository.Transition(ctx, first.ID, domain.Running, domain.Completed, now.Add(2*time.Second)); err != nil {
		t.Fatalf("Transition running to completed error = %v", err)
	}
	if err := repository.ReleaseLeases(ctx, first.ID, now.Add(2*time.Second)); err != nil {
		t.Fatalf("ReleaseLeases() error = %v", err)
	}
	if err := repository.AcquireLeases(ctx, second.ID, []string{resource}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("AcquireLeases after release error = %v", err)
	}
}

func TestSessionRepositoryTerminalTransitionRollsBackWhenLeaseReleaseFails(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewSessionRepository(database)
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	value := createStoredSession(t, ctx, repository, domain.Queued, now)
	if _, err := repository.Transition(ctx, value.ID, domain.Queued, domain.Running, now.Add(time.Second)); err != nil {
		t.Fatalf("Transition queued to running error = %v", err)
	}
	resource := "018f3d8d-8d2c-7c20-8e0c-000000000201/oscilloscope/bench-a"
	if err := repository.AcquireLeases(ctx, value.ID, []string{resource}, now.Add(time.Second)); err != nil {
		t.Fatalf("AcquireLeases() error = %v", err)
	}
	if _, err := database.SQL().ExecContext(ctx, `
		CREATE TRIGGER fail_session_lease_release
		BEFORE UPDATE OF released_at ON resource_leases
		WHEN NEW.released_at IS NOT NULL
		BEGIN SELECT RAISE(ABORT, 'injected lease release failure'); END
	`); err != nil {
		t.Fatalf("create failing lease trigger: %v", err)
	}

	if _, err := repository.TransitionAndRelease(ctx, value.ID, domain.Running, domain.Completed, now.Add(2*time.Second)); err == nil {
		t.Fatal("TransitionAndRelease() succeeded despite an injected lease release failure")
	}
	stored, err := repository.Get(ctx, value.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.State != domain.Running {
		t.Fatalf("state after failed terminal operation = %q, want %q", stored.State, domain.Running)
	}
	if activeLeaseCount(t, database, value.ID) != 1 {
		t.Fatalf("active leases after failed terminal operation = %d, want 1", activeLeaseCount(t, database, value.ID))
	}
}

func TestSessionRepositoryRejectsTransitionTimeBeforePersistedUpdate(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewSessionRepository(database)
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	value := createStoredSession(t, ctx, repository, domain.Queued, now)

	_, err := repository.Transition(ctx, value.ID, domain.Queued, domain.Running, now.Add(-time.Nanosecond))
	if !errors.Is(err, session.ErrSessionTimeRegression) {
		t.Fatalf("Transition() error = %v, want ErrSessionTimeRegression", err)
	}
	stored, err := repository.Get(ctx, value.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.State != domain.Queued || !stored.UpdatedAt.Equal(now) {
		t.Fatalf("stored state/time = %q/%s, want queued/%s", stored.State, stored.UpdatedAt, now)
	}
}

func TestSessionRepositoryRejectsBackwardStartupReconciliationWithoutPartialRepair(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewSessionRepository(database)
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	value := createStoredSession(t, ctx, repository, domain.Queued, now)
	if _, err := repository.Transition(ctx, value.ID, domain.Queued, domain.Running, now.Add(time.Second)); err != nil {
		t.Fatalf("Transition queued to running error = %v", err)
	}
	resource := "018f3d8d-8d2c-7c20-8e0c-000000000201/oscilloscope/bench-a"
	if err := repository.AcquireLeases(ctx, value.ID, []string{resource}, now.Add(time.Second)); err != nil {
		t.Fatalf("AcquireLeases() error = %v", err)
	}

	err := repository.ReconcileStartup(ctx, now)
	if !errors.Is(err, session.ErrSessionTimeRegression) {
		t.Fatalf("ReconcileStartup() error = %v, want ErrSessionTimeRegression", err)
	}
	stored, err := repository.Get(ctx, value.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.State != domain.Running || !stored.UpdatedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("stored state/time = %q/%s, want running/%s", stored.State, stored.UpdatedAt, now.Add(time.Second))
	}
	if activeLeaseCount(t, database, value.ID) != 1 {
		t.Fatalf("active leases after failed reconciliation = %d, want 1", activeLeaseCount(t, database, value.ID))
	}
}

func TestSessionRepositoryReconcilesOnlyPersistedRunningSessionsAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	repository := NewSessionRepository(database)
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	running := createStoredSession(t, ctx, repository, domain.Queued, now)
	queued := createStoredSession(t, ctx, repository, domain.Queued, now)
	completed := createStoredSession(t, ctx, repository, domain.Queued, now)
	for _, value := range []struct {
		id domain.SessionID
		to domain.SessionState
	}{
		{running.ID, domain.Running}, {completed.ID, domain.Running}, {completed.ID, domain.Completed},
	} {
		from := domain.Queued
		if value.to == domain.Completed {
			from = domain.Running
		}
		if _, err := repository.Transition(ctx, value.id, from, value.to, now.Add(time.Second)); err != nil {
			t.Fatalf("Transition(%s, %s) error = %v", value.id, value.to, err)
		}
	}
	resource := "018f3d8d-8d2c-7c20-8e0c-000000000201/oscilloscope/bench-a"
	if err := repository.AcquireLeases(ctx, running.ID, []string{resource}, now); err != nil {
		t.Fatalf("AcquireLeases() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen database error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	repository = NewSessionRepository(reopened)
	if err := repository.ReconcileStartup(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatalf("ReconcileStartup() error = %v", err)
	}

	for _, want := range []struct {
		id    domain.SessionID
		state domain.SessionState
	}{
		{running.ID, domain.Incomplete}, {queued.ID, domain.Queued}, {completed.ID, domain.Completed},
	} {
		got, err := repository.Get(ctx, want.id)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", want.id, err)
		}
		if got.State != want.state {
			t.Fatalf("Get(%s).State = %q, want %q", want.id, got.State, want.state)
		}
	}
	if err := repository.AcquireLeases(ctx, queued.ID, []string{resource}, now.Add(3*time.Second)); err != nil {
		t.Fatalf("reconciled running lease was not released: %v", err)
	}
	var auditCount int
	if err := reopened.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events WHERE action = 'session.reconciled'").Scan(&auditCount); err != nil {
		t.Fatalf("count reconciliation audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("reconciliation audit count = %d, want 1", auditCount)
	}
}

func createStoredSession(t *testing.T, ctx context.Context, repository *SessionRepository, state domain.SessionState, now time.Time) domain.Session {
	t.Helper()
	requestID, err := domain.NewRequestID()
	if err != nil {
		t.Fatalf("NewRequestID() error = %v", err)
	}
	if _, err := repository.database.SQL().ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	_, err = repository.database.SQL().ExecContext(ctx, `
		INSERT INTO operation_requests (
			id, actor_id, integration_id, operation_name, device_id, canonical_parameters,
			configuration_snapshot_id, policy_snapshot_id, deadline_at, created_at
		) VALUES (?, ?, ?, 'capture', ?, '{}', ?, ?, ?, ?)
	`, requestID.String(), "018f3d8d-8d2c-7c20-8e0c-000000000202", "018f3d8d-8d2c-7c20-8e0c-000000000203",
		"018f3d8d-8d2c-7c20-8e0c-000000000204", "018f3d8d-8d2c-7c20-8e0c-000000000205",
		"018f3d8d-8d2c-7c20-8e0c-000000000206", now.Add(time.Minute).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("seed operation request: %v", err)
	}
	if _, err := repository.database.SQL().ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	value, err := domain.NewSession(requestID, state, now)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if err := repository.Create(ctx, value); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return value
}

func activeLeaseCount(t *testing.T, database *DB, sessionID domain.SessionID) int {
	t.Helper()
	var count int
	if err := database.SQL().QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM resource_leases WHERE session_id = ? AND released_at IS NULL
	`, sessionID.String()).Scan(&count); err != nil {
		t.Fatalf("count active leases: %v", err)
	}
	return count
}
