package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/operation"
	"nilo.local/nilo/internal/policy"
)

// OperationRepository persists the immutable request and its exact policy
// snapshot in one transaction so neither can exist without the other.
type OperationRepository struct {
	database *DB
}

func NewOperationRepository(database *DB) *OperationRepository {
	return &OperationRepository{database: database}
}

func (repository *OperationRepository) Create(ctx context.Context, request domain.OperationRequest, snapshot policy.Snapshot) error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("operation database is required")
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if request.PolicySnapshot() != snapshot.ID || request.IntegrationID() != snapshot.IntegrationID ||
		request.OperationName() != snapshot.OperationName || request.DeviceID() != snapshot.DeviceID {
		return errors.New("operation request does not match policy snapshot")
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin operation request save: %w", err)
	}
	defer transaction.Rollback()
	var operationOverride any
	if snapshot.OperationOverride != nil {
		operationOverride = string(*snapshot.OperationOverride)
	}
	var deviceMode any
	if snapshot.DeviceKnown {
		deviceMode = string(snapshot.DeviceMode)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO policy_snapshots (
			id, integration_id, operation_name, device_id,
			app_known, app_mode, manifest_policy, operation_override,
			device_known, device_mode, effective_mode, effective_policy,
			decision, reason, resolved_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, snapshot.ID.String(), snapshot.IntegrationID.String(), snapshot.OperationName, snapshot.DeviceID.String(),
		boolInteger(snapshot.AppKnown), string(snapshot.AppMode), string(snapshot.ManifestPolicy), operationOverride,
		boolInteger(snapshot.DeviceKnown), deviceMode, string(snapshot.EffectiveMode), string(snapshot.EffectivePolicy),
		string(snapshot.Decision), string(snapshot.Reason), snapshot.ResolvedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", policy.ErrSnapshotAlreadyStored, snapshot.ID.String())
		}
		return fmt.Errorf("insert operation policy snapshot: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO operation_requests (
			id, actor_id, integration_id, operation_name, device_id,
			canonical_parameters, configuration_snapshot_id, policy_snapshot_id,
			deadline_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, request.ID().String(), request.Actor().ID.String(), request.IntegrationID().String(), request.OperationName(),
		request.DeviceID().String(), []byte(request.CanonicalParameters()), request.ConfigurationSnapshot().String(),
		request.PolicySnapshot().String(), request.Deadline().UTC().Format(time.RFC3339Nano), request.CreatedAt().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("insert immutable operation request: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit operation request: %w", err)
	}
	return nil
}

func (repository *OperationRepository) Get(ctx context.Context, requestID domain.RequestID) (domain.OperationRequest, error) {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return domain.OperationRequest{}, errors.New("operation database is required")
	}
	if requestID.String() == "00000000-0000-0000-0000-000000000000" {
		return domain.OperationRequest{}, operation.ErrRequestNotFound
	}
	request, err := NewPolicyRepository(repository.database).loadOperationRequest(ctx, requestID)
	if errors.Is(err, policy.ErrApprovalNotFound) {
		return domain.OperationRequest{}, operation.ErrRequestNotFound
	}
	return request, err
}

var _ operation.RequestRepository = (*OperationRepository)(nil)
