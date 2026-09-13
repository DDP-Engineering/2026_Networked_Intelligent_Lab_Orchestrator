package app

import (
	"context"
	"testing"
	"time"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/domain"
)

func TestNewOpensFoundationServicesAndReconcilesInterruptedSession(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2030, 8, 31, 12, 0, 0, 0, time.UTC)
	options := Options{DataDir: t.TempDir(), BindAddress: "127.0.0.1:0", Now: func() time.Time { return now }}
	first, err := New(ctx, options)
	if err != nil {
		t.Fatalf("New(first) error = %v", err)
	}
	requestID, _ := domain.NewRequestID()
	if _, err := first.database.SQL().ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := first.database.SQL().ExecContext(ctx, `
		INSERT INTO operation_requests (
			id, actor_id, integration_id, operation_name, device_id, canonical_parameters,
			configuration_snapshot_id, policy_snapshot_id, deadline_at, created_at
		) VALUES (?, ?, ?, 'echo', ?, '{}', ?, ?, ?, ?)
	`, requestID.String(), "018f3d8d-8d2c-7c20-8e0c-000000000202", "018f3d8d-8d2c-7c20-8e0c-000000000203",
		"018f3d8d-8d2c-7c20-8e0c-000000000204", "018f3d8d-8d2c-7c20-8e0c-000000000205",
		"018f3d8d-8d2c-7c20-8e0c-000000000206", now.Add(time.Minute).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed operation request: %v", err)
	}
	if _, err := first.database.SQL().ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	queued, err := domain.NewSession(requestID, domain.Queued, now)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if err := first.sessionRepository.Create(ctx, queued); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := first.sessionRepository.AcquireLeases(ctx, queued.ID, []string{"018f3d8d-8d2c-7c20-8e0c-000000000203/device/stable-001"}, now); err != nil {
		t.Fatalf("AcquireLeases() error = %v", err)
	}
	running, err := first.sessionService.Transition(ctx, queued.ID, domain.Queued, domain.Running)
	if err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close(first) error = %v", err)
	}

	second, err := New(ctx, Options{DataDir: options.DataDir, BindAddress: options.BindAddress, Now: func() time.Time { return now.Add(time.Second) }})
	if err != nil {
		t.Fatalf("New(second) error = %v", err)
	}
	defer second.Close()
	reconciled, err := second.sessionRepository.Get(ctx, running.ID)
	if err != nil || reconciled.State != domain.Incomplete {
		t.Fatalf("reconciled = %+v, error = %v", reconciled, err)
	}
	var activeLeases int
	if err := second.database.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM resource_leases WHERE session_id = ? AND released_at IS NULL", running.ID.String()).Scan(&activeLeases); err != nil {
		t.Fatalf("count active leases: %v", err)
	}
	if activeLeases != 0 {
		t.Fatalf("active leases = %d, want 0", activeLeases)
	}
}

// TestOpenLeavesInterruptedSessionsForTheServerToReconcile proves that opening
// the foundation services is not starting the server: only a server start may
// declare persisted running work interrupted, because every other command may
// be running beside a live server that still owns it.
func TestOpenLeavesInterruptedSessionsForTheServerToReconcile(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2030, 8, 31, 12, 0, 0, 0, time.UTC)
	options := Options{DataDir: t.TempDir(), BindAddress: "127.0.0.1:0", Now: func() time.Time { return now }}
	first, err := New(ctx, options)
	if err != nil {
		t.Fatalf("New(first) error = %v", err)
	}
	running := seedRunningSession(t, first, now)
	if err := first.Close(); err != nil {
		t.Fatalf("Close(first) error = %v", err)
	}

	opened, err := Open(ctx, options)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	untouched, err := opened.sessionRepository.Get(ctx, running)
	if err != nil {
		t.Fatalf("Get after Open: %v", err)
	}
	if untouched.State != domain.Running {
		t.Fatalf("session state after Open = %q, want %q", untouched.State, domain.Running)
	}
	if leases := activeLeaseCount(t, opened, running); leases != 1 {
		t.Fatalf("active leases after Open = %d, want 1", leases)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("Close(opened) error = %v", err)
	}

	started, err := New(ctx, Options{DataDir: options.DataDir, BindAddress: options.BindAddress, Now: func() time.Time { return now.Add(time.Second) }})
	if err != nil {
		t.Fatalf("New(started) error = %v", err)
	}
	defer started.Close()
	reconciled, err := started.sessionRepository.Get(ctx, running)
	if err != nil || reconciled.State != domain.Incomplete {
		t.Fatalf("reconciled = %+v, error = %v", reconciled, err)
	}
	if leases := activeLeaseCount(t, started, running); leases != 0 {
		t.Fatalf("active leases after the server start = %d, want 0", leases)
	}
}

// seedRunningSession writes one running session holding one active lease, the
// state a server keeps on disk while it executes an operation.
func seedRunningSession(t *testing.T, application *App, now time.Time) domain.SessionID {
	t.Helper()
	ctx := context.Background()
	requestID, _ := domain.NewRequestID()
	if _, err := application.database.SQL().ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	if _, err := application.database.SQL().ExecContext(ctx, `
		INSERT INTO operation_requests (
			id, actor_id, integration_id, operation_name, device_id, canonical_parameters,
			configuration_snapshot_id, policy_snapshot_id, deadline_at, created_at
		) VALUES (?, ?, ?, 'echo', ?, '{}', ?, ?, ?, ?)
	`, requestID.String(), "018f3d8d-8d2c-7c20-8e0c-000000000202", "018f3d8d-8d2c-7c20-8e0c-000000000203",
		"018f3d8d-8d2c-7c20-8e0c-000000000204", "018f3d8d-8d2c-7c20-8e0c-000000000205",
		"018f3d8d-8d2c-7c20-8e0c-000000000206", now.Add(time.Minute).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed operation request: %v", err)
	}
	if _, err := application.database.SQL().ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	queued, err := domain.NewSession(requestID, domain.Queued, now)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	if err := application.sessionRepository.Create(ctx, queued); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := application.sessionRepository.AcquireLeases(ctx, queued.ID, []string{"018f3d8d-8d2c-7c20-8e0c-000000000203/device/stable-001"}, now); err != nil {
		t.Fatalf("AcquireLeases() error = %v", err)
	}
	if _, err := application.sessionService.Transition(ctx, queued.ID, domain.Queued, domain.Running); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	return queued.ID
}

func activeLeaseCount(t *testing.T, application *App, id domain.SessionID) int {
	t.Helper()
	var active int
	if err := application.database.SQL().QueryRowContext(
		context.Background(),
		"SELECT COUNT(*) FROM resource_leases WHERE session_id = ? AND released_at IS NULL",
		id.String(),
	).Scan(&active); err != nil {
		t.Fatalf("count active leases: %v", err)
	}
	return active
}

// The lifecycle state decides first-boot management exposure, so it has to come
// back from the data directory exactly as recorded rather than being re-derived
// or defaulted on every start.
func TestOpenReportsTheRecordedCommissioningStateAcrossReopen(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2030, 8, 31, 12, 0, 0, 0, time.UTC)
	options := Options{DataDir: t.TempDir(), BindAddress: "127.0.0.1:0", Now: func() time.Time { return now }}

	first, err := Open(ctx, options)
	if err != nil {
		t.Fatalf("Open(first) error = %v", err)
	}
	if state := first.Commissioning(); state != appliance.CommissioningProvisioning {
		t.Fatalf("commissioning state on a new data directory = %q, want %q",
			state, appliance.CommissioningProvisioning)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close(first) error = %v", err)
	}

	second, err := Open(ctx, options)
	if err != nil {
		t.Fatalf("Open(second) error = %v", err)
	}
	if state := second.Commissioning(); state != appliance.CommissioningProvisioning {
		t.Fatalf("commissioning state after reopen = %q, want %q",
			state, appliance.CommissioningProvisioning)
	}
	if _, err := second.database.SQL().ExecContext(ctx,
		"UPDATE commissioning SET state = ? WHERE singleton = 1", appliance.CommissioningCommissioned); err != nil {
		t.Fatalf("record the commissioned state: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close(second) error = %v", err)
	}

	third, err := Open(ctx, options)
	if err != nil {
		t.Fatalf("Open(third) error = %v", err)
	}
	t.Cleanup(func() { _ = third.Close() })
	if state := third.Commissioning(); state != appliance.CommissioningCommissioned {
		t.Fatalf("commissioning state after reopen = %q, want %q",
			state, appliance.CommissioningCommissioned)
	}
}

// Refusing to start is the safe direction: a guessed commissioned would skip
// first boot on an appliance that needs it, and a guessed provisioning would
// widen management exposure on one that does not.
func TestOpenRefusesADatabaseWithNoRecordedCommissioningState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2030, 8, 31, 12, 0, 0, 0, time.UTC)
	options := Options{DataDir: t.TempDir(), BindAddress: "127.0.0.1:0", Now: func() time.Time { return now }}

	first, err := Open(ctx, options)
	if err != nil {
		t.Fatalf("Open(first) error = %v", err)
	}
	if _, err := first.database.SQL().ExecContext(ctx, "DELETE FROM commissioning"); err != nil {
		t.Fatalf("remove the recorded state: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close(first) error = %v", err)
	}

	if _, err := Open(ctx, options); err == nil {
		t.Fatal("Open accepted a data directory with no recorded commissioning state")
	}
}
