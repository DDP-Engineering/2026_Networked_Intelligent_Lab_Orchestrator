package bench

import (
	"context"
	"encoding/json"
	"fmt"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

// ModbusSettingsValues controls new Modbus sessions and future retained recordings.
type ModbusSettingsValues struct {
	MaxSessions         int   `json:"max_sessions"`
	SessionLeaseSeconds int   `json:"session_lease_seconds"`
	IdleSeconds         int   `json:"idle_seconds"`
	RetentionCount      int   `json:"retention_count"`
	RetentionBytes      int64 `json:"retention_bytes"`
	PartBytes           int64 `json:"part_bytes"`
}

// ModbusSettingsState is the revisioned appliance-wide record.
type ModbusSettingsState struct {
	Revision int64                `json:"revision"`
	Settings ModbusSettingsValues `json:"settings"`
}

func validModbusSettings(s ModbusSettingsValues) bool {
	in := func(value, low, high int) bool { return value >= low && value <= high }
	return in(s.MaxSessions, 1, 16) && in(s.SessionLeaseSeconds, 60, 3600) &&
		in(s.IdleSeconds, 30, 3600) &&
		in(s.RetentionCount, 1, 1000) &&
		s.RetentionBytes >= 1<<20 && s.RetentionBytes <= 16<<30 &&
		s.PartBytes >= 1<<20 && s.PartBytes <= 256<<20 && s.PartBytes <= s.RetentionBytes
}

func (m *Manager) loadModbusSettings(ctx context.Context) error {
	var s ModbusSettingsValues
	err := m.db().QueryRowContext(ctx, `SELECT revision,max_sessions,session_lease_seconds,idle_seconds,retention_count,retention_bytes,part_bytes FROM modbus_settings WHERE id=1`).Scan(
		&m.modbusSettingsRevision, &s.MaxSessions, &s.SessionLeaseSeconds, &s.IdleSeconds,
		&s.RetentionCount, &s.RetentionBytes, &s.PartBytes)
	if err != nil {
		return fmt.Errorf("load Modbus settings: %w", err)
	}
	if !validModbusSettings(s) {
		return fmt.Errorf("load Modbus settings: %w", ErrInvalid)
	}
	m.modbusSettings = s
	return nil
}

// ModbusSettings returns settings to an authorized Bench reader.
func (m *Manager) ModbusSettings(_ context.Context, actor identity.Actor) (ModbusSettingsState, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return ModbusSettingsState{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ModbusSettingsState{}, ErrUnavailable
	}
	return ModbusSettingsState{Revision: m.modbusSettingsRevision, Settings: m.modbusSettings}, nil
}

// ReplaceModbusSettings saves one full revision with its audit event atomically.
func (m *Manager) ReplaceModbusSettings(ctx context.Context, actor identity.Actor, revision int64, settings ModbusSettingsValues) (ModbusSettingsState, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return ModbusSettingsState{}, err
	}
	if revision < 1 || !validModbusSettings(settings) {
		return ModbusSettingsState{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ModbusSettingsState{}, ErrUnavailable
	}
	if revision != m.modbusSettingsRevision {
		return ModbusSettingsState{}, ErrConflict
	}
	tx, err := m.db().BeginTx(ctx, nil)
	if err != nil {
		return ModbusSettingsState{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE modbus_settings SET revision=revision+1,max_sessions=?,session_lease_seconds=?,idle_seconds=?,retention_count=?,retention_bytes=?,part_bytes=? WHERE id=1 AND revision=?`,
		settings.MaxSessions, settings.SessionLeaseSeconds, settings.IdleSeconds, settings.RetentionCount, settings.RetentionBytes, settings.PartBytes, revision)
	if err != nil {
		return ModbusSettingsState{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return ModbusSettingsState{}, err
	}
	if rows != 1 {
		return ModbusSettingsState{}, ErrConflict
	}
	actorID, err := domain.ParseActorID(actor.User.ID)
	if err != nil {
		return ModbusSettingsState{}, ErrForbidden
	}
	kind := audit.ActorKindUser
	if actor.TokenID != "" {
		kind = audit.ActorKindToken
	}
	encoded, _ := json.Marshal(settings)
	event, err := m.options.Audit.Compose(audit.Record{
		Category: audit.CategoryAudit, Component: "modbus", Action: "modbus.settings_changed",
		SubjectType: "modbus_settings", SubjectID: "1", ActorKind: kind, ActorID: actorID,
		Outcome: audit.OutcomeApplied, Details: map[string]any{"revision": revision + 1, "settings": json.RawMessage(encoded)},
	})
	if err != nil {
		return ModbusSettingsState{}, err
	}
	if err := sqlite.NewAuditRepository(m.options.Database).AppendInTransaction(ctx, tx, event); err != nil {
		return ModbusSettingsState{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModbusSettingsState{}, err
	}
	m.modbusSettings, m.modbusSettingsRevision = settings, revision+1
	return ModbusSettingsState{Revision: m.modbusSettingsRevision, Settings: m.modbusSettings}, nil
}
