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

// OperatingSettingsValues bounds appliance-wide admissions and new work.
type OperatingSettingsValues struct {
	ExecutingJobs            int `json:"executing_jobs"`
	OutstandingJobs          int `json:"outstanding_jobs"`
	SerialConnections        int `json:"serial_connections"`
	CameraStreams            int `json:"camera_streams"`
	DefaultJobTimeoutSeconds int `json:"default_job_timeout_seconds"`
	MaxJobTimeoutSeconds     int `json:"max_job_timeout_seconds"`
	SerialLifetimeMinutes    int `json:"serial_lifetime_minutes"`
	SerialIdleMinutes        int `json:"serial_idle_minutes"`
	CameraLifetimeMinutes    int `json:"camera_lifetime_minutes"`
}

// OperatingUsage reports live occupancy, independent of the saved limits.
type OperatingUsage struct {
	ExecutingJobs     int `json:"executing_jobs"`
	OutstandingJobs   int `json:"outstanding_jobs"`
	SerialConnections int `json:"serial_connections"`
	CameraStreams     int `json:"camera_streams"`
}

// OperatingSettingsState is the revisioned representation shared by API clients.
type OperatingSettingsState struct {
	Revision int64                   `json:"revision"`
	Settings OperatingSettingsValues `json:"settings"`
	Usage    OperatingUsage          `json:"usage"`
}

func validOperatingSettings(s OperatingSettingsValues) bool {
	in := func(value, low, high int) bool { return value >= low && value <= high }
	return in(s.ExecutingJobs, 1, 8) && in(s.OutstandingJobs, 1, 256) &&
		in(s.SerialConnections, 1, 16) && in(s.CameraStreams, 1, 8) &&
		in(s.DefaultJobTimeoutSeconds, 1, 3600) && in(s.MaxJobTimeoutSeconds, 1, 3600) &&
		s.DefaultJobTimeoutSeconds <= s.MaxJobTimeoutSeconds &&
		in(s.SerialLifetimeMinutes, 1, 1440) && in(s.SerialIdleMinutes, 1, 60) &&
		s.SerialIdleMinutes <= s.SerialLifetimeMinutes && in(s.CameraLifetimeMinutes, 1, 240)
}

func (m *Manager) loadOperatingSettings(ctx context.Context) error {
	var s OperatingSettingsValues
	err := m.db().QueryRowContext(ctx, `SELECT revision,executing_jobs,outstanding_jobs,serial_connections,camera_streams,default_job_timeout_seconds,max_job_timeout_seconds,serial_lifetime_minutes,serial_idle_minutes,camera_lifetime_minutes FROM operating_settings WHERE id=1`).Scan(
		&m.settingsRevision, &s.ExecutingJobs, &s.OutstandingJobs, &s.SerialConnections, &s.CameraStreams,
		&s.DefaultJobTimeoutSeconds, &s.MaxJobTimeoutSeconds, &s.SerialLifetimeMinutes,
		&s.SerialIdleMinutes, &s.CameraLifetimeMinutes)
	if err != nil {
		return fmt.Errorf("load operating settings: %w", err)
	}
	if !validOperatingSettings(s) {
		return fmt.Errorf("load operating settings: %w", ErrInvalid)
	}
	m.settings = s
	return nil
}

// OperatingSettings returns the current settings and live usage to an administrator.
func (m *Manager) OperatingSettings(ctx context.Context, actor identity.Actor) (OperatingSettingsState, error) {
	if err := require(actor, auth.PermissionPlatformSettingsManage); err != nil {
		return OperatingSettingsState{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return OperatingSettingsState{}, ErrUnavailable
	}
	return m.operatingSettingsLocked(ctx)
}

func (m *Manager) operatingSettingsLocked(ctx context.Context) (OperatingSettingsState, error) {
	var outstanding int
	if err := m.db().QueryRowContext(ctx, `SELECT count(*) FROM bench_jobs j JOIN sessions s ON s.id=j.session_id WHERE s.state IN ('pending_approval','queued','running')`).Scan(&outstanding); err != nil {
		return OperatingSettingsState{}, err
	}
	usage := OperatingUsage{ExecutingJobs: len(m.active), OutstandingJobs: outstanding}
	for _, session := range m.serials {
		if !session.stopped {
			usage.SerialConnections++
		}
	}
	for id := range m.cameras {
		if m.cameraBusy(id) {
			usage.CameraStreams++
		}
	}
	return OperatingSettingsState{Revision: m.settingsRevision, Settings: m.settings, Usage: usage}, nil
}

// ReplaceOperatingSettings atomically swaps the full record and its audit event.
func (m *Manager) ReplaceOperatingSettings(ctx context.Context, actor identity.Actor, revision int64, settings OperatingSettingsValues) (OperatingSettingsState, error) {
	if err := require(actor, auth.PermissionPlatformSettingsManage); err != nil {
		return OperatingSettingsState{}, err
	}
	if !validOperatingSettings(settings) || revision < 1 {
		return OperatingSettingsState{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return OperatingSettingsState{}, ErrUnavailable
	}
	if revision != m.settingsRevision {
		return OperatingSettingsState{}, ErrConflict
	}
	transaction, err := m.db().BeginTx(ctx, nil)
	if err != nil {
		return OperatingSettingsState{}, err
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, `UPDATE operating_settings SET revision=revision+1,executing_jobs=?,outstanding_jobs=?,serial_connections=?,camera_streams=?,default_job_timeout_seconds=?,max_job_timeout_seconds=?,serial_lifetime_minutes=?,serial_idle_minutes=?,camera_lifetime_minutes=? WHERE id=1 AND revision=?`,
		settings.ExecutingJobs, settings.OutstandingJobs, settings.SerialConnections, settings.CameraStreams,
		settings.DefaultJobTimeoutSeconds, settings.MaxJobTimeoutSeconds, settings.SerialLifetimeMinutes,
		settings.SerialIdleMinutes, settings.CameraLifetimeMinutes, revision)
	if err != nil {
		return OperatingSettingsState{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return OperatingSettingsState{}, err
	}
	if rows != 1 {
		return OperatingSettingsState{}, ErrConflict
	}
	actorID, err := domain.ParseActorID(actor.User.ID)
	if err != nil {
		return OperatingSettingsState{}, ErrForbidden
	}
	kind := audit.ActorKindUser
	if actor.TokenID != "" {
		kind = audit.ActorKindToken
	}
	encoded, _ := json.Marshal(settings)
	event, err := m.options.Audit.Compose(audit.Record{
		Category: audit.CategoryAudit, Component: "platform", Action: "operating_settings.changed",
		SubjectType: "operating_settings", SubjectID: "1", ActorKind: kind, ActorID: actorID,
		Outcome: audit.OutcomeApplied, Details: map[string]any{"revision": revision + 1, "settings": json.RawMessage(encoded)},
	})
	if err != nil {
		return OperatingSettingsState{}, err
	}
	if err := sqlite.NewAuditRepository(m.options.Database).AppendInTransaction(ctx, transaction, event); err != nil {
		return OperatingSettingsState{}, err
	}
	if err := transaction.Commit(); err != nil {
		return OperatingSettingsState{}, err
	}
	m.settings = settings
	m.settingsRevision++
	select {
	case m.wake <- struct{}{}:
	default:
	}
	return m.operatingSettingsLocked(ctx)
}
