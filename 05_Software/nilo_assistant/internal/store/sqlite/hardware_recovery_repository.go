package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/network"
)

// HardwareRecoveryRepository stores the one restart-safe core interlock.
type HardwareRecoveryRepository struct{ database *DB }

func NewHardwareRecoveryRepository(database *DB) *HardwareRecoveryRepository {
	return &HardwareRecoveryRepository{database: database}
}

func (repository *HardwareRecoveryRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("hardware recovery repository database is required")
	}
	return nil
}

func (repository *HardwareRecoveryRepository) LoadHardwareRecovery(ctx context.Context) (network.RecoveryRecord, bool, error) {
	if err := repository.ready(); err != nil {
		return network.RecoveryRecord{}, false, err
	}
	var record network.RecoveryRecord
	var state, armedAt, expiresAt, connectedJSON, pendingAuditsJSON, updatedAt string
	err := repository.database.SQL().QueryRowContext(ctx, `
		SELECT version, state, armed_at, expires_at, connected_at_arm_json,
		       adapter_id, interface_name, helper_record_id, hardware_generation, pending_audits_json, detail, updated_at
		FROM network_recovery WHERE singleton = 1
	`).Scan(&record.Version, &state, &armedAt, &expiresAt, &connectedJSON,
		&record.AdapterID, &record.Interface, &record.HelperRecordID, &record.HardwareGeneration, &pendingAuditsJSON, &record.Detail, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return network.RecoveryRecord{}, false, nil
	}
	if err != nil {
		return network.RecoveryRecord{}, false, fmt.Errorf("read hardware recovery: %w", err)
	}
	record.State = network.RecoveryState(state)
	if err := json.Unmarshal([]byte(connectedJSON), &record.ConnectedAtArm); err != nil {
		return network.RecoveryRecord{}, false, fmt.Errorf("decode hardware recovery links: %w", err)
	}
	if err := json.Unmarshal([]byte(pendingAuditsJSON), &record.PendingAudits); err != nil {
		return network.RecoveryRecord{}, false, fmt.Errorf("decode pending recovery audits: %w", err)
	}
	var parseErr error
	if record.ArmedAt, parseErr = optionalTimestamp(armedAt); parseErr != nil {
		return network.RecoveryRecord{}, false, fmt.Errorf("parse hardware recovery arm time: %w", parseErr)
	}
	if record.ExpiresAt, parseErr = optionalTimestamp(expiresAt); parseErr != nil {
		return network.RecoveryRecord{}, false, fmt.Errorf("parse hardware recovery expiry: %w", parseErr)
	}
	if record.UpdatedAt, parseErr = optionalTimestamp(updatedAt); parseErr != nil {
		return network.RecoveryRecord{}, false, fmt.Errorf("parse hardware recovery update: %w", parseErr)
	}
	return record, true, nil
}

func optionalTimestamp(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func formatOptionalTimestamp(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func (repository *HardwareRecoveryRepository) SaveHardwareRecovery(ctx context.Context, record network.RecoveryRecord) error {
	if err := repository.ready(); err != nil {
		return err
	}
	if record.Version != 1 || !validRecoveryState(record.State) || record.UpdatedAt.IsZero() {
		return errors.New("invalid hardware recovery record")
	}
	connected, err := json.Marshal(record.ConnectedAtArm)
	if err != nil {
		return err
	}
	pendingAudits, err := json.Marshal(record.PendingAudits)
	if err != nil {
		return err
	}
	_, err = repository.database.SQL().ExecContext(ctx, `
		INSERT INTO network_recovery (
			singleton, version, state, armed_at, expires_at, connected_at_arm_json,
			adapter_id, interface_name, helper_record_id, hardware_generation, pending_audits_json, detail, updated_at
		) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(singleton) DO UPDATE SET
			version=excluded.version, state=excluded.state, armed_at=excluded.armed_at,
			expires_at=excluded.expires_at, connected_at_arm_json=excluded.connected_at_arm_json,
			adapter_id=excluded.adapter_id, interface_name=excluded.interface_name,
			helper_record_id=excluded.helper_record_id, hardware_generation=excluded.hardware_generation,
			pending_audits_json=excluded.pending_audits_json, detail=excluded.detail, updated_at=excluded.updated_at
	`, record.Version, string(record.State), formatOptionalTimestamp(record.ArmedAt),
		formatOptionalTimestamp(record.ExpiresAt), string(connected), record.AdapterID,
		record.Interface, record.HelperRecordID, record.HardwareGeneration, string(pendingAudits), record.Detail, formatOptionalTimestamp(record.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store hardware recovery: %w", err)
	}
	return nil
}

func validRecoveryState(state network.RecoveryState) bool {
	switch state {
	case network.RecoveryIdle, network.RecoveryArmed, network.RecoveryActive,
		network.RecoveryRestoring, network.RecoveryFailed:
		return true
	default:
		return false
	}
}

var _ network.HardwareRecoveryStore = (*HardwareRecoveryRepository)(nil)
