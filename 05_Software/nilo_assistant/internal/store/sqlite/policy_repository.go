package sqlite

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/policy"
)

// PolicyRepository persists immutable resolution snapshots and single-request
// administrator decisions.
type PolicyRepository struct {
	database *DB
}

func NewPolicyRepository(database *DB) *PolicyRepository {
	return &PolicyRepository{database: database}
}

func (repository *PolicyRepository) SaveSnapshot(ctx context.Context, snapshot policy.Snapshot) error {
	if err := repository.validateDatabase(); err != nil {
		return err
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	var operationOverride any
	if snapshot.OperationOverride != nil {
		operationOverride = string(*snapshot.OperationOverride)
	}
	var deviceMode any
	if snapshot.DeviceKnown {
		deviceMode = string(snapshot.DeviceMode)
	}
	_, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO policy_snapshots (
			id, integration_id, operation_name, device_id,
			app_known, app_mode, manifest_policy, operation_override,
			device_known, device_mode, effective_mode, effective_policy,
			decision, reason, resolved_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		snapshot.ID.String(), snapshot.IntegrationID.String(), snapshot.OperationName, snapshot.DeviceID.String(),
		boolInteger(snapshot.AppKnown), string(snapshot.AppMode), string(snapshot.ManifestPolicy), operationOverride,
		boolInteger(snapshot.DeviceKnown), deviceMode, string(snapshot.EffectiveMode), string(snapshot.EffectivePolicy),
		string(snapshot.Decision), string(snapshot.Reason), snapshot.ResolvedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", policy.ErrSnapshotAlreadyStored, snapshot.ID.String())
		}
		return fmt.Errorf("store policy snapshot: %w", err)
	}
	return nil
}

func (repository *PolicyRepository) RecordApproval(ctx context.Context, record policy.ApprovalRecord) error {
	if err := repository.validateDatabase(); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}
	request, err := repository.loadOperationRequest(ctx, record.RequestID)
	if err != nil {
		return err
	}
	expectedDigest, err := policy.CanonicalRequestDigest(request)
	if err != nil {
		return fmt.Errorf("digest stored immutable request: %w", err)
	}
	if subtle.ConstantTimeCompare(record.RequestDigest[:], expectedDigest[:]) != 1 {
		return policy.ErrApprovalDigestMismatch
	}
	if !record.ExpiresAt.Equal(request.Deadline().UTC()) {
		return fmt.Errorf("%w: approval expiry does not match request deadline", policy.ErrInvalidApprovalRecord)
	}
	if record.RecordedAt.Before(request.CreatedAt()) {
		return fmt.Errorf("%w: approval predates the request", policy.ErrInvalidApprovalRecord)
	}
	_, err = repository.database.SQL().ExecContext(ctx, `
		INSERT INTO approval_records (
			id, request_id, request_digest, decision, actor_id, expires_at, recorded_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		record.ID, record.RequestID.String(), record.RequestDigest[:], string(record.Decision), record.ActorID.String(),
		record.ExpiresAt.UTC().Format(time.RFC3339Nano), record.RecordedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", policy.ErrApprovalAlreadyRecorded, record.RequestID.String())
		}
		return fmt.Errorf("store approval decision: %w", err)
	}
	return nil
}

func (repository *PolicyRepository) loadOperationRequest(ctx context.Context, requestID domain.RequestID) (domain.OperationRequest, error) {
	var (
		requestValue, actorValue, integrationValue, operationName string
		deviceValue, configurationValue, policyValue              string
		parameters                                                []byte
		deadlineAt, createdAt                                     string
	)
	err := repository.database.SQL().QueryRowContext(ctx, `
		SELECT id, actor_id, integration_id, operation_name, device_id,
		       canonical_parameters, configuration_snapshot_id, policy_snapshot_id,
		       deadline_at, created_at
		FROM operation_requests WHERE id = ?
	`, requestID.String()).Scan(
		&requestValue, &actorValue, &integrationValue, &operationName, &deviceValue,
		&parameters, &configurationValue, &policyValue, &deadlineAt, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OperationRequest{}, policy.ErrApprovalNotFound
	}
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("read immutable operation request: %w", err)
	}
	parsedRequestID, err := domain.ParseRequestID(requestValue)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("parse stored request ID: %w", err)
	}
	actorID, err := domain.ParseActorID(actorValue)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("parse stored actor ID: %w", err)
	}
	integrationID, err := domain.ParseIntegrationID(integrationValue)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("parse stored integration ID: %w", err)
	}
	deviceID, err := domain.ParseDeviceID(deviceValue)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("parse stored device ID: %w", err)
	}
	configurationID, err := domain.ParseSnapshotID(configurationValue)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("parse stored configuration snapshot ID: %w", err)
	}
	policyID, err := domain.ParseSnapshotID(policyValue)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("parse stored policy snapshot ID: %w", err)
	}
	deadline, err := parseUTCTimestamp(deadlineAt)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("parse stored request deadline: %w", err)
	}
	created, err := parseUTCTimestamp(createdAt)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("parse stored request creation time: %w", err)
	}
	request, err := domain.NewOperationRequest(domain.OperationRequestInput{
		ID: parsedRequestID, Actor: domain.Actor{ID: actorID}, IntegrationID: integrationID,
		OperationName: operationName, DeviceID: deviceID, CanonicalParameters: parameters,
		ConfigurationSnapshot: configurationID, PolicySnapshot: policyID, Deadline: deadline,
	}, created)
	if err != nil {
		return domain.OperationRequest{}, fmt.Errorf("validate stored immutable operation request: %w", err)
	}
	return request, nil
}

func (repository *PolicyRepository) FindValidApproval(ctx context.Context, requestID domain.RequestID, digest [32]byte, now time.Time) (policy.ApprovalRecord, error) {
	if err := repository.validateDatabase(); err != nil {
		return policy.ApprovalRecord{}, err
	}
	if requestID.String() == "00000000-0000-0000-0000-000000000000" || now.IsZero() {
		return policy.ApprovalRecord{}, fmt.Errorf("%w: request identity and current time are required", policy.ErrInvalidApprovalRecord)
	}
	var (
		record       policy.ApprovalRecord
		requestValue string
		digestBytes  []byte
		decision     string
		actorValue   string
		expiresAt    string
		recordedAt   string
	)
	err := repository.database.SQL().QueryRowContext(ctx, `
		SELECT id, request_id, request_digest, decision, actor_id, expires_at, recorded_at
		FROM approval_records WHERE request_id = ?
	`, requestID.String()).Scan(
		&record.ID, &requestValue, &digestBytes, &decision, &actorValue, &expiresAt, &recordedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return policy.ApprovalRecord{}, policy.ErrApprovalNotFound
	}
	if err != nil {
		return policy.ApprovalRecord{}, fmt.Errorf("read approval decision: %w", err)
	}
	if len(digestBytes) != len(record.RequestDigest) {
		return policy.ApprovalRecord{}, fmt.Errorf("%w: stored digest length is %d", policy.ErrInvalidApprovalRecord, len(digestBytes))
	}
	copy(record.RequestDigest[:], digestBytes)
	record.RequestID, err = domain.ParseRequestID(requestValue)
	if err != nil {
		return policy.ApprovalRecord{}, fmt.Errorf("%w: parse request ID: %v", policy.ErrInvalidApprovalRecord, err)
	}
	record.ActorID, err = domain.ParseActorID(actorValue)
	if err != nil {
		return policy.ApprovalRecord{}, fmt.Errorf("%w: parse actor ID: %v", policy.ErrInvalidApprovalRecord, err)
	}
	record.Decision, err = policy.ParseApprovalDecision(decision)
	if err != nil {
		return policy.ApprovalRecord{}, fmt.Errorf("%w: %v", policy.ErrInvalidApprovalRecord, err)
	}
	record.ExpiresAt, err = parseUTCTimestamp(expiresAt)
	if err != nil {
		return policy.ApprovalRecord{}, fmt.Errorf("%w: parse expiry: %v", policy.ErrInvalidApprovalRecord, err)
	}
	record.RecordedAt, err = parseUTCTimestamp(recordedAt)
	if err != nil {
		return policy.ApprovalRecord{}, fmt.Errorf("%w: parse decision time: %v", policy.ErrInvalidApprovalRecord, err)
	}
	if err := record.Validate(); err != nil {
		return policy.ApprovalRecord{}, err
	}
	if subtle.ConstantTimeCompare(record.RequestDigest[:], digest[:]) != 1 {
		return policy.ApprovalRecord{}, policy.ErrApprovalDigestMismatch
	}
	if !now.Before(record.ExpiresAt) {
		return policy.ApprovalRecord{}, policy.ErrApprovalExpired
	}
	if record.Decision != policy.ApprovalGranted {
		return policy.ApprovalRecord{}, policy.ErrApprovalDenied
	}
	return record, nil
}

func (repository *PolicyRepository) validateDatabase() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("policy database is required")
	}
	return nil
}

func boolInteger(value bool) int {
	if value {
		return 1
	}
	return 0
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func parseUTCTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	if parsed.Location() != time.UTC {
		return time.Time{}, errors.New("timestamp is not UTC")
	}
	return parsed, nil
}

var _ policy.Repository = (*PolicyRepository)(nil)
