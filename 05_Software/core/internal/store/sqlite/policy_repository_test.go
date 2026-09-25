package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/policy"
)

func TestPolicyRepositoryPersistsCompleteSnapshotAcrossReload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	repository := NewPolicyRepository(database)
	input := policyResolutionInput(t)
	override := policy.FullOnly
	input.OperationOverride = &override
	input.AppMode = policy.Full
	input.DeviceMode = policy.Auto
	snapshot, err := policy.Resolve(input)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := repository.SaveSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}
	if err := repository.SaveSnapshot(ctx, snapshot); !errors.Is(err, policy.ErrSnapshotAlreadyStored) {
		t.Fatalf("duplicate SaveSnapshot() error = %v, want ErrSnapshotAlreadyStored", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var (
		integrationID, operationName, deviceID                  string
		appKnown, deviceKnown                                   int
		appMode, manifestPolicy, effectiveMode, effectivePolicy string
		operationOverride, deviceMode                           sql.NullString
		decision, reason, resolvedAt                            string
	)
	err = reopened.SQL().QueryRowContext(ctx, `
		SELECT integration_id, operation_name, device_id,
		       app_known, app_mode, manifest_policy, operation_override,
		       device_known, device_mode, effective_mode, effective_policy,
		       decision, reason, resolved_at
		FROM policy_snapshots WHERE id = ?
	`, snapshot.ID.String()).Scan(
		&integrationID, &operationName, &deviceID,
		&appKnown, &appMode, &manifestPolicy, &operationOverride,
		&deviceKnown, &deviceMode, &effectiveMode, &effectivePolicy,
		&decision, &reason, &resolvedAt,
	)
	if err != nil {
		t.Fatalf("reload policy snapshot: %v", err)
	}
	if integrationID != snapshot.IntegrationID.String() || operationName != snapshot.OperationName || deviceID != snapshot.DeviceID.String() {
		t.Fatalf("reloaded snapshot identity = %q/%q/%q, want %q/%q/%q", integrationID, operationName, deviceID, snapshot.IntegrationID, snapshot.OperationName, snapshot.DeviceID)
	}
	if appKnown != 1 || appMode != string(policy.Full) || manifestPolicy != string(policy.AutoEligible) || !operationOverride.Valid || operationOverride.String != string(policy.FullOnly) {
		t.Fatalf("reloaded app inputs = known %d, mode %q, manifest %q, override %#v", appKnown, appMode, manifestPolicy, operationOverride)
	}
	if deviceKnown != 1 || !deviceMode.Valid || deviceMode.String != string(policy.Auto) || effectiveMode != string(policy.Auto) || effectivePolicy != string(policy.FullOnly) {
		t.Fatalf("reloaded device/effective values = known %d, device %#v, mode %q, policy %q", deviceKnown, deviceMode, effectiveMode, effectivePolicy)
	}
	if decision != string(policy.Prompt) || reason != string(policy.DevicePolicyLowered) || resolvedAt != snapshot.ResolvedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("reloaded result = %q/%q/%q, want %q/%q/%q", decision, reason, resolvedAt, snapshot.Decision, snapshot.Reason, snapshot.ResolvedAt.UTC().Format(time.RFC3339Nano))
	}
}

func TestPolicyRepositoryPersistsUnknownAppAndDeviceAskDecision(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewPolicyRepository(database)
	input := policyResolutionInput(t)
	input.AppKnown = false
	input.AppMode = policy.Full
	input.DeviceKnown = false
	input.DeviceMode = ""
	snapshot, err := policy.Resolve(input)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if err := repository.SaveSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("SaveSnapshot() error = %v", err)
	}
	var appKnown, deviceKnown int
	var deviceMode sql.NullString
	var effectiveMode, decision string
	if err := database.SQL().QueryRowContext(ctx, `
		SELECT app_known, device_known, device_mode, effective_mode, decision
		FROM policy_snapshots WHERE id = ?
	`, snapshot.ID.String()).Scan(&appKnown, &deviceKnown, &deviceMode, &effectiveMode, &decision); err != nil {
		t.Fatalf("read unknown-identity snapshot: %v", err)
	}
	if appKnown != 0 || deviceKnown != 0 || deviceMode.Valid || effectiveMode != string(policy.Ask) || decision != string(policy.Prompt) {
		t.Fatalf("unknown snapshot = app %d, device %d/%#v, effective %q/%q; want unknown/NULL/ask/prompt", appKnown, deviceKnown, deviceMode, effectiveMode, decision)
	}
}

func TestPolicyRepositoryFindsOnlyMatchingUnexpiredApprovalAcrossReload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	repository := NewPolicyRepository(database)
	request := policyOperationRequest(t, `{"amplitude":2,"channel":"A"}`)
	seedPolicyOperationRequest(t, database, request)
	actorID := mustParseActorID(t, "018f3d8d-8d2c-7c20-8e0c-000000000207")
	recordedAt := request.CreatedAt().Add(5 * time.Minute)
	record, err := policy.BindApproval("018f3d8d-8d2c-7c20-8e0c-000000000208", request, policy.ApprovalGranted, actorID, recordedAt)
	if err != nil {
		t.Fatalf("BindApproval() error = %v", err)
	}
	if err := repository.RecordApproval(ctx, record); err != nil {
		t.Fatalf("RecordApproval() error = %v", err)
	}
	if err := repository.RecordApproval(ctx, record); !errors.Is(err, policy.ErrApprovalAlreadyRecorded) {
		t.Fatalf("duplicate RecordApproval() error = %v, want ErrApprovalAlreadyRecorded", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	repository = NewPolicyRepository(reopened)
	got, err := repository.FindValidApproval(ctx, request.ID(), record.RequestDigest, recordedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("FindValidApproval() error = %v", err)
	}
	if got != record {
		t.Fatalf("FindValidApproval() = %#v, want %#v", got, record)
	}

	wrongDigest := record.RequestDigest
	wrongDigest[0] ^= 0xff
	if _, err := repository.FindValidApproval(ctx, request.ID(), wrongDigest, recordedAt.Add(time.Minute)); !errors.Is(err, policy.ErrApprovalDigestMismatch) {
		t.Fatalf("digest mismatch error = %v, want ErrApprovalDigestMismatch", err)
	}
	if _, err := repository.FindValidApproval(ctx, request.ID(), record.RequestDigest, request.Deadline()); !errors.Is(err, policy.ErrApprovalExpired) {
		t.Fatalf("expired approval error = %v, want ErrApprovalExpired", err)
	}
	unknownRequestID := mustParseRequestID(t, "018f3d8d-8d2c-7c20-8e0c-000000000209")
	if _, err := repository.FindValidApproval(ctx, unknownRequestID, record.RequestDigest, recordedAt.Add(time.Minute)); !errors.Is(err, policy.ErrApprovalNotFound) {
		t.Fatalf("unknown request error = %v, want ErrApprovalNotFound", err)
	}
}

func TestPolicyRepositoryDeniedDecisionNeverBecomesValidApproval(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewPolicyRepository(database)
	request := policyOperationRequest(t, `{"amplitude":2,"channel":"A"}`)
	seedPolicyOperationRequest(t, database, request)
	record, err := policy.BindApproval(
		"018f3d8d-8d2c-7c20-8e0c-000000000210",
		request,
		policy.ApprovalRejected,
		mustParseActorID(t, "018f3d8d-8d2c-7c20-8e0c-000000000207"),
		request.CreatedAt().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("BindApproval() error = %v", err)
	}
	if err := repository.RecordApproval(ctx, record); err != nil {
		t.Fatalf("RecordApproval() error = %v", err)
	}
	if _, err := repository.FindValidApproval(ctx, request.ID(), record.RequestDigest, request.CreatedAt().Add(2*time.Minute)); !errors.Is(err, policy.ErrApprovalDenied) {
		t.Fatalf("denied decision error = %v, want ErrApprovalDenied", err)
	}
}

func TestPolicyRepositoryRejectsApprovalNotBoundToStoredRequest(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewPolicyRepository(database)
	request := policyOperationRequest(t, `{"amplitude":2,"channel":"A"}`)
	seedPolicyOperationRequest(t, database, request)
	record, err := policy.BindApproval(
		"018f3d8d-8d2c-7c20-8e0c-000000000211",
		request,
		policy.ApprovalGranted,
		mustParseActorID(t, "018f3d8d-8d2c-7c20-8e0c-000000000207"),
		request.CreatedAt().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("BindApproval() error = %v", err)
	}

	wrongDigest := record
	wrongDigest.RequestDigest[0] ^= 0xff
	if err := repository.RecordApproval(ctx, wrongDigest); !errors.Is(err, policy.ErrApprovalDigestMismatch) {
		t.Fatalf("RecordApproval(wrong digest) error = %v, want ErrApprovalDigestMismatch", err)
	}

	wrongExpiry := record
	wrongExpiry.ExpiresAt = wrongExpiry.ExpiresAt.Add(-time.Minute)
	if err := repository.RecordApproval(ctx, wrongExpiry); !errors.Is(err, policy.ErrInvalidApprovalRecord) {
		t.Fatalf("RecordApproval(wrong expiry) error = %v, want ErrInvalidApprovalRecord", err)
	}
}

func policyResolutionInput(t *testing.T) policy.ResolutionInput {
	t.Helper()
	return policy.ResolutionInput{
		SnapshotID:    mustParseSnapshotID(t, "018f3d8d-8d2c-7c20-8e0c-000000000206"),
		IntegrationID: mustParseIntegrationID(t, "018f3d8d-8d2c-7c20-8e0c-000000000203"),
		OperationName: "capture",
		DeviceID:      mustParseDeviceID(t, "018f3d8d-8d2c-7c20-8e0c-000000000204"),
		AppKnown:      true, AppMode: policy.Auto, ManifestPolicy: policy.AutoEligible,
		DeviceKnown: true, DeviceMode: policy.Auto,
		ResolvedAt: time.Date(2026, 8, 30, 15, 4, 5, 123456789, time.FixedZone("EEST", 3*60*60)),
	}
}

func policyOperationRequest(t *testing.T, parameters string) domain.OperationRequest {
	t.Helper()
	createdAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	request, err := domain.RehydrateOperationRequest(mustParseRequestID(t, "018f3d8d-8d2c-7c20-8e0c-000000000201"), domain.OperationRequestInput{
		Actor:                 domain.Actor{ID: mustParseActorID(t, "018f3d8d-8d2c-7c20-8e0c-000000000202")},
		IntegrationID:         mustParseIntegrationID(t, "018f3d8d-8d2c-7c20-8e0c-000000000203"),
		OperationName:         "capture",
		DeviceID:              mustParseDeviceID(t, "018f3d8d-8d2c-7c20-8e0c-000000000204"),
		CanonicalParameters:   []byte(parameters),
		ConfigurationSnapshot: mustParseSnapshotID(t, "018f3d8d-8d2c-7c20-8e0c-000000000205"),
		PolicySnapshot:        mustParseSnapshotID(t, "018f3d8d-8d2c-7c20-8e0c-000000000206"),
		Deadline:              createdAt.Add(time.Hour),
	}, createdAt)
	if err != nil {
		t.Fatalf("RehydrateOperationRequest() error = %v", err)
	}
	return request
}

func seedPolicyOperationRequest(t *testing.T, database *DB, request domain.OperationRequest) {
	t.Helper()
	ctx := context.Background()
	if _, err := database.SQL().ExecContext(ctx,
		"INSERT INTO config_versions (id, configuration_json, digest, created_at) VALUES (?, ?, ?, ?)",
		request.ConfigurationSnapshot().String(), `{}`, make([]byte, 32), request.CreatedAt().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed configuration snapshot: %v", err)
	}
	if _, err := database.SQL().ExecContext(ctx, `
		INSERT INTO operation_requests (
			id, actor_id, integration_id, operation_name, device_id, canonical_parameters,
			configuration_snapshot_id, policy_snapshot_id, deadline_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		request.ID().String(), request.Actor().ID.String(), request.IntegrationID().String(), request.OperationName(), request.DeviceID().String(), []byte(request.CanonicalParameters()),
		request.ConfigurationSnapshot().String(), request.PolicySnapshot().String(), request.Deadline().UTC().Format(time.RFC3339Nano), request.CreatedAt().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed immutable operation request: %v", err)
	}
}

func mustParseRequestID(t *testing.T, value string) domain.RequestID {
	t.Helper()
	id, err := domain.ParseRequestID(value)
	if err != nil {
		t.Fatalf("ParseRequestID(%q): %v", value, err)
	}
	return id
}

func mustParseActorID(t *testing.T, value string) domain.ActorID {
	t.Helper()
	id, err := domain.ParseActorID(value)
	if err != nil {
		t.Fatalf("ParseActorID(%q): %v", value, err)
	}
	return id
}

func mustParseIntegrationID(t *testing.T, value string) domain.IntegrationID {
	t.Helper()
	id, err := domain.ParseIntegrationID(value)
	if err != nil {
		t.Fatalf("ParseIntegrationID(%q): %v", value, err)
	}
	return id
}

func mustParseDeviceID(t *testing.T, value string) domain.DeviceID {
	t.Helper()
	id, err := domain.ParseDeviceID(value)
	if err != nil {
		t.Fatalf("ParseDeviceID(%q): %v", value, err)
	}
	return id
}

func mustParseSnapshotID(t *testing.T, value string) domain.SnapshotID {
	t.Helper()
	id, err := domain.ParseSnapshotID(value)
	if err != nil {
		t.Fatalf("ParseSnapshotID(%q): %v", value, err)
	}
	return id
}
