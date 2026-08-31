package app

import (
	"context"
	"testing"
	"time"

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
