package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/operation"
	"nilo.local/nilo/internal/policy"
)

func TestOperationRepositoryAtomicallyPersistsAndLoadsRequestWithPolicySnapshot(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewOperationRepository(database)
	now := time.Date(2030, 8, 31, 12, 0, 0, 0, time.UTC)
	request, snapshot := operationRepositoryValues(t, now)
	if _, err := database.SQL().ExecContext(ctx,
		"INSERT INTO config_versions (id, configuration_json, digest, created_at) VALUES (?, '{}', ?, ?)",
		request.ConfigurationSnapshot().String(), make([]byte, 32), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed configuration snapshot: %v", err)
	}

	if err := repository.Create(ctx, request, snapshot); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	loaded, err := repository.Get(ctx, request.ID())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if loaded.ID() != request.ID() || string(loaded.CanonicalParameters()) != string(request.CanonicalParameters()) || loaded.PolicySnapshot() != snapshot.ID {
		t.Fatalf("loaded request = %#v, want identity %s and snapshot %s", loaded, request.ID(), snapshot.ID)
	}

	missingConfiguration, badSnapshot := operationRepositoryValues(t, now.Add(time.Second))
	if err := repository.Create(ctx, missingConfiguration, badSnapshot); err == nil {
		t.Fatal("Create() with missing configuration snapshot succeeded")
	}
	var snapshotCount int
	if err := database.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM policy_snapshots WHERE id = ?", badSnapshot.ID.String()).Scan(&snapshotCount); err != nil {
		t.Fatalf("count rolled-back snapshot: %v", err)
	}
	if snapshotCount != 0 {
		t.Fatalf("rolled-back snapshot count = %d, want 0", snapshotCount)
	}
	if _, err := repository.Get(ctx, domain.RequestID{}); !errors.Is(err, operation.ErrRequestNotFound) {
		t.Fatalf("Get(empty) error = %v, want ErrRequestNotFound", err)
	}
}

func operationRepositoryValues(t *testing.T, now time.Time) (domain.OperationRequest, policy.Snapshot) {
	t.Helper()
	actorID, _ := domain.NewActorID()
	integrationID, _ := domain.NewIntegrationID()
	deviceID, _ := domain.NewDeviceID()
	configurationID, _ := domain.NewSnapshotID()
	snapshotID, _ := domain.NewSnapshotID()
	snapshot, err := policy.Resolve(policy.ResolutionInput{
		SnapshotID: snapshotID, IntegrationID: integrationID, OperationName: "echo", DeviceID: deviceID,
		AppKnown: true, AppMode: policy.Auto, ManifestPolicy: policy.AutoEligible,
		DeviceKnown: true, DeviceMode: policy.Auto, ResolvedAt: now,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	request, err := domain.NewOperationRequest(domain.OperationRequestInput{
		Actor: domain.Actor{ID: actorID}, IntegrationID: integrationID, OperationName: "echo", DeviceID: deviceID,
		CanonicalParameters: json.RawMessage(`{"value":"hello"}`), ConfigurationSnapshot: configurationID,
		PolicySnapshot: snapshotID, Deadline: now.Add(time.Minute),
	}, now)
	if err != nil {
		t.Fatalf("NewOperationRequest() error = %v", err)
	}
	return request, snapshot
}
