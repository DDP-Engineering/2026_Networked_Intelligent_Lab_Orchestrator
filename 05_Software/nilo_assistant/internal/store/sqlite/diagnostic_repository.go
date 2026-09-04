package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/diagnostics"
)

// DiagnosticRepository keeps timed debug sessions.
type DiagnosticRepository struct{ database *DB }

func NewDiagnosticRepository(database *DB) *DiagnosticRepository {
	return &DiagnosticRepository{database: database}
}

func (repository *DiagnosticRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("diagnostics database is required")
	}
	return nil
}

// SaveSession inserts or replaces one session.
func (repository *DiagnosticRepository) SaveSession(ctx context.Context, session diagnostics.Session) error {
	if err := repository.ready(); err != nil {
		return err
	}
	var endedAt sql.NullString
	if session.EndedAt != nil {
		endedAt = sql.NullString{String: session.EndedAt.UTC().Format(time.RFC3339Nano), Valid: true}
	}
	_, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO diagnostic_sessions (id, scope, target, actor_id, minutes, started_at, expires_at, boot_id, state, ended_at, end_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET state = excluded.state, ended_at = excluded.ended_at, end_reason = excluded.end_reason
	`, session.ID, string(session.Scope), session.Target, session.ActorID, session.Minutes,
		session.StartedAt.UTC().Format(time.RFC3339Nano), session.ExpiresAt.UTC().Format(time.RFC3339Nano),
		session.BootID, string(session.State), endedAt, session.EndReason)
	if err != nil {
		return fmt.Errorf("save diagnostic session: %w", err)
	}
	return nil
}

// Sessions lists every session, newest first, bounded.
func (repository *DiagnosticRepository) Sessions(ctx context.Context) ([]diagnostics.Session, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT id, scope, target, actor_id, minutes, started_at, expires_at, boot_id, state, ended_at, end_reason
		FROM diagnostic_sessions ORDER BY started_at DESC LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("list diagnostic sessions: %w", err)
	}
	defer rows.Close()
	sessions := make([]diagnostics.Session, 0)
	for rows.Next() {
		var session diagnostics.Session
		var scope, state, startedAt, expiresAt string
		var endedAt sql.NullString
		if err := rows.Scan(&session.ID, &scope, &session.Target, &session.ActorID, &session.Minutes, &startedAt, &expiresAt, &session.BootID, &state, &endedAt, &session.EndReason); err != nil {
			return nil, err
		}
		session.Scope, session.State = diagnostics.Scope(scope), diagnostics.State(state)
		if session.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt); err != nil {
			return nil, err
		}
		if session.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt); err != nil {
			return nil, err
		}
		if endedAt.Valid {
			ended, err := time.Parse(time.RFC3339Nano, endedAt.String)
			if err != nil {
				return nil, err
			}
			session.EndedAt = &ended
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

var _ diagnostics.Store = (*DiagnosticRepository)(nil)
