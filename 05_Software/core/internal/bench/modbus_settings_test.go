package bench

import (
	"context"
	"errors"
	"testing"

	"nilo.local/nilo/internal/auth"
)

func TestModbusSettingsPersistWithRevisionAndBounds(t *testing.T) {
	m, actor, _ := testManager(t, "auto")
	ctx := context.Background()
	initial, err := m.ModbusSettings(ctx, actor)
	if err != nil || initial.Revision != 1 || initial.Settings.MaxSessions != 4 || initial.Settings.SessionLeaseSeconds != 3600 {
		t.Fatalf("initial settings = %+v, %v", initial, err)
	}
	changed := initial.Settings
	changed.MaxSessions = 6
	updated, err := m.ReplaceModbusSettings(ctx, actor, 1, changed)
	if err != nil || updated.Revision != 2 || updated.Settings != changed {
		t.Fatalf("replace = %+v, %v", updated, err)
	}
	if _, err := m.ReplaceModbusSettings(ctx, actor, 1, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale replacement = %v", err)
	}
	invalid := changed
	invalid.PartBytes = invalid.RetentionBytes + 1
	if _, err := m.ReplaceModbusSettings(ctx, actor, 2, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid replacement = %v", err)
	}
	var revision, sessions int
	if err := m.db().QueryRowContext(ctx, `SELECT revision,max_sessions FROM modbus_settings WHERE id=1`).Scan(&revision, &sessions); err != nil || revision != 2 || sessions != 6 {
		t.Fatalf("persisted = %d/%d, %v", revision, sessions, err)
	}
	viewer := actor
	viewer.User.Role = auth.RoleViewer
	viewer.Permissions = auth.RoleViewer.Permissions()
	if _, err := m.ReplaceModbusSettings(ctx, viewer, 2, changed); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer replacement = %v", err)
	}
	if _, err := m.db().ExecContext(ctx, `CREATE TRIGGER reject_modbus_settings_audit BEFORE INSERT ON audit_events WHEN NEW.action='modbus.settings_changed' BEGIN SELECT RAISE(ABORT, 'audit blocked'); END`); err != nil {
		t.Fatal(err)
	}
	changed.MaxSessions = 7
	if _, err := m.ReplaceModbusSettings(ctx, actor, 2, changed); err == nil {
		t.Fatal("settings update succeeded without its audit event")
	}
	if err := m.db().QueryRowContext(ctx, `SELECT revision,max_sessions FROM modbus_settings WHERE id=1`).Scan(&revision, &sessions); err != nil || revision != 2 || sessions != 6 {
		t.Fatalf("audit failure changed settings: %d/%d, %v", revision, sessions, err)
	}
}
