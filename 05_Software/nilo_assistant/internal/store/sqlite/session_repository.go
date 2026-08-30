package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/session"
)

const reconciliationActorID = "018f3d8d-8d2c-7c20-8e0c-000000000001"

// SessionRepository persists durable session state, exclusive resource leases,
// and the all-or-nothing restart repair record.
type SessionRepository struct {
	database *DB
}

func NewSessionRepository(database *DB) *SessionRepository {
	return &SessionRepository{database: database}
}

func (repository *SessionRepository) Create(ctx context.Context, value domain.Session) error {
	if err := repository.validateDatabase(); err != nil {
		return err
	}
	if err := value.Validate(); err != nil {
		return err
	}
	var terminalAt any
	if value.TerminalAt != nil {
		terminalAt = value.TerminalAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO sessions (id, request_id, state, created_at, updated_at, terminal_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, value.ID.String(), value.RequestID.String(), string(value.State), value.CreatedAt.UTC().Format(time.RFC3339Nano),
		value.UpdatedAt.UTC().Format(time.RFC3339Nano), terminalAt)
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: request or session identity", session.ErrSessionAlreadyExists)
		}
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

func (repository *SessionRepository) Get(ctx context.Context, id domain.SessionID) (domain.Session, error) {
	if err := repository.validateDatabase(); err != nil {
		return domain.Session{}, err
	}
	if id.String() == "00000000-0000-0000-0000-000000000000" {
		return domain.Session{}, session.ErrSessionNotFound
	}
	return repository.get(ctx, repository.database.SQL(), id)
}

func (repository *SessionRepository) Transition(ctx context.Context, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error) {
	if err := repository.validateDatabase(); err != nil {
		return domain.Session{}, err
	}
	if err := domain.ValidateTransition(from, to); err != nil {
		return domain.Session{}, err
	}
	if at.IsZero() || at.Location() != time.UTC {
		return domain.Session{}, fmt.Errorf("transition time must be UTC")
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return domain.Session{}, fmt.Errorf("begin session transition: %w", err)
	}
	defer transaction.Rollback()
	value, err := repository.transition(ctx, transaction, id, from, to, at)
	if err != nil {
		return domain.Session{}, err
	}
	if err := transaction.Commit(); err != nil {
		return domain.Session{}, fmt.Errorf("commit session transition: %w", err)
	}
	return value, nil
}

// TransitionAndRelease is the terminal transition boundary used by Service.
// It never exposes a committed terminal state with that session's leases still
// active: either both writes commit or neither does.
func (repository *SessionRepository) TransitionAndRelease(ctx context.Context, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error) {
	if err := repository.validateDatabase(); err != nil {
		return domain.Session{}, err
	}
	if !isTerminalSessionState(to) {
		return domain.Session{}, fmt.Errorf("atomic transition requires a terminal state")
	}
	if err := domain.ValidateTransition(from, to); err != nil {
		return domain.Session{}, err
	}
	if at.IsZero() || at.Location() != time.UTC {
		return domain.Session{}, fmt.Errorf("transition time must be UTC")
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return domain.Session{}, fmt.Errorf("begin terminal session transition: %w", err)
	}
	defer transaction.Rollback()
	value, err := repository.transition(ctx, transaction, id, from, to, at)
	if err != nil {
		return domain.Session{}, err
	}
	if err := releaseLeases(ctx, transaction, id, at); err != nil {
		return domain.Session{}, err
	}
	if err := transaction.Commit(); err != nil {
		return domain.Session{}, fmt.Errorf("commit terminal session transition: %w", err)
	}
	return value, nil
}

func (repository *SessionRepository) transition(ctx context.Context, transaction *sql.Tx, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error) {
	current, err := repository.get(ctx, transaction, id)
	if err != nil {
		return domain.Session{}, err
	}
	if current.State != from {
		return domain.Session{}, session.ErrSessionStateConflict
	}
	if at.Before(current.UpdatedAt) {
		return domain.Session{}, fmt.Errorf("%w: %s before %s", session.ErrSessionTimeRegression, at, current.UpdatedAt)
	}
	var terminalAt any
	if isTerminalSessionState(to) {
		terminalAt = at.UTC().Format(time.RFC3339Nano)
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE sessions SET state = ?, updated_at = ?, terminal_at = ?
		WHERE id = ? AND state = ?
	`, string(to), at.UTC().Format(time.RFC3339Nano), terminalAt, id.String(), string(from))
	if err != nil {
		return domain.Session{}, fmt.Errorf("compare-and-swap session transition: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return domain.Session{}, fmt.Errorf("inspect session transition result: %w", err)
	}
	if affected == 0 {
		return domain.Session{}, session.ErrSessionStateConflict
	}
	var terminal *time.Time
	if isTerminalSessionState(to) {
		terminalValue := at
		terminal = &terminalValue
	}
	value, err := domain.RehydrateSession(id, current.RequestID, to, current.CreatedAt, at, terminal)
	if err != nil {
		return domain.Session{}, fmt.Errorf("validate transitioned session: %w", err)
	}
	return value, nil
}

func (repository *SessionRepository) AcquireLeases(ctx context.Context, id domain.SessionID, resources []string, at time.Time) error {
	if err := repository.validateDatabase(); err != nil {
		return err
	}
	if at.IsZero() || at.Location() != time.UTC {
		return errors.New("lease acquisition time must be UTC")
	}
	normalized, err := session.NormalizeResourceClaims(resources)
	if err != nil {
		return err
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin lease acquisition: %w", err)
	}
	defer transaction.Rollback()
	if err := sessionExists(ctx, transaction, id); err != nil {
		return err
	}
	for _, resource := range normalized {
		leaseID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("allocate lease identity: %w", err)
		}
		_, err = transaction.ExecContext(ctx, `
			INSERT INTO resource_leases (id, session_id, resource_name, acquired_at, released_at)
			VALUES (?, ?, ?, ?, NULL)
		`, leaseID.String(), id.String(), resource, at.UTC().Format(time.RFC3339Nano))
		if err == nil {
			continue
		}
		if !isUniqueConstraint(err) {
			return fmt.Errorf("insert resource lease: %w", err)
		}
		var ownerValue string
		ownerErr := transaction.QueryRowContext(ctx, `
			SELECT session_id FROM resource_leases WHERE resource_name = ? AND released_at IS NULL
		`, resource).Scan(&ownerValue)
		if ownerErr != nil {
			return fmt.Errorf("read resource lease owner: %w", ownerErr)
		}
		ownerID, parseErr := domain.ParseSessionID(ownerValue)
		if parseErr != nil {
			return fmt.Errorf("parse resource lease owner: %w", parseErr)
		}
		return &session.ResourceBusyError{OwnerSessionID: ownerID}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit lease acquisition: %w", err)
	}
	return nil
}

func (repository *SessionRepository) ReleaseLeases(ctx context.Context, id domain.SessionID, at time.Time) error {
	if err := repository.validateDatabase(); err != nil {
		return err
	}
	if at.IsZero() || at.Location() != time.UTC {
		return errors.New("lease release time must be UTC")
	}
	return releaseLeases(ctx, repository.database.SQL(), id, at)
}

type leaseExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func releaseLeases(ctx context.Context, executor leaseExecutor, id domain.SessionID, at time.Time) error {
	_, err := executor.ExecContext(ctx, `
		UPDATE resource_leases SET released_at = ? WHERE session_id = ? AND released_at IS NULL
	`, at.UTC().Format(time.RFC3339Nano), id.String())
	if err != nil {
		return fmt.Errorf("release session leases: %w", err)
	}
	return nil
}

func (repository *SessionRepository) ListInterrupted(ctx context.Context) ([]domain.Session, error) {
	if err := repository.validateDatabase(); err != nil {
		return nil, err
	}
	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT id, request_id, state, created_at, updated_at, terminal_at
		FROM sessions WHERE state = ? ORDER BY id
	`, string(domain.Running))
	if err != nil {
		return nil, fmt.Errorf("list interrupted sessions: %w", err)
	}
	defer rows.Close()
	return scanSessions(rows)
}

// ReconcileStartup marks only persisted running sessions incomplete, records an
// append-only audit event for each, and releases only leases owned by that
// session in the same database transaction.
func (repository *SessionRepository) ReconcileStartup(ctx context.Context, at time.Time) error {
	if err := repository.validateDatabase(); err != nil {
		return err
	}
	if at.IsZero() || at.Location() != time.UTC {
		return errors.New("startup reconciliation time must be UTC")
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin startup reconciliation: %w", err)
	}
	defer transaction.Rollback()
	rows, err := transaction.QueryContext(ctx, `
		SELECT id, request_id, state, created_at, updated_at, terminal_at
		FROM sessions WHERE state = ? ORDER BY id
	`, string(domain.Running))
	if err != nil {
		return fmt.Errorf("list running sessions for reconciliation: %w", err)
	}
	interrupted, scanErr := scanSessions(rows)
	closeErr := rows.Close()
	if scanErr != nil {
		return scanErr
	}
	if closeErr != nil {
		return fmt.Errorf("close interrupted session rows: %w", closeErr)
	}
	var nextSequence int64
	if err := transaction.QueryRowContext(ctx, "SELECT COALESCE(MAX(sequence), 0) FROM audit_events").Scan(&nextSequence); err != nil {
		return fmt.Errorf("allocate reconciliation audit sequence: %w", err)
	}
	for _, value := range interrupted {
		if at.Before(value.UpdatedAt) {
			return fmt.Errorf("%w: %s before %s", session.ErrSessionTimeRegression, at, value.UpdatedAt)
		}
		result, err := transaction.ExecContext(ctx, `
			UPDATE sessions SET state = ?, updated_at = ?, terminal_at = ? WHERE id = ? AND state = ?
		`, string(domain.Incomplete), at.UTC().Format(time.RFC3339Nano), at.UTC().Format(time.RFC3339Nano), value.ID.String(), string(domain.Running))
		if err != nil {
			return fmt.Errorf("mark interrupted session incomplete: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect interrupted session transition result: %w", err)
		}
		if affected != 1 {
			return errors.New("reconcile interrupted session compare-and-swap conflict")
		}
		if err := releaseLeases(ctx, transaction, value.ID, at); err != nil {
			return fmt.Errorf("release interrupted session leases: %w", err)
		}
		nextSequence++
		eventID, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("allocate reconciliation audit ID: %w", err)
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO audit_events (id, sequence, actor_id, action, subject_type, subject_id, payload_json, created_at)
			VALUES (?, ?, ?, 'session.reconciled', 'session', ?, '{}', ?)
		`, eventID.String(), nextSequence, reconciliationActorID, value.ID.String(), at.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("append startup reconciliation audit event: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit startup reconciliation: %w", err)
	}
	return nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func sessionExists(ctx context.Context, query queryer, id domain.SessionID) error {
	var exists int
	err := query.QueryRowContext(ctx, "SELECT 1 FROM sessions WHERE id = ?", id.String()).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return session.ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("check session existence: %w", err)
	}
	return nil
}

func (repository *SessionRepository) get(ctx context.Context, query queryer, id domain.SessionID) (domain.Session, error) {
	row := query.QueryRowContext(ctx, `
		SELECT id, request_id, state, created_at, updated_at, terminal_at FROM sessions WHERE id = ?
	`, id.String())
	value, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, session.ErrSessionNotFound
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("read session: %w", err)
	}
	return value, nil
}

type sessionScanner interface {
	Scan(...any) error
}

func scanSession(scanner sessionScanner) (domain.Session, error) {
	var idValue, requestValue, stateValue, createdValue, updatedValue string
	var terminalValue sql.NullString
	if err := scanner.Scan(&idValue, &requestValue, &stateValue, &createdValue, &updatedValue, &terminalValue); err != nil {
		return domain.Session{}, err
	}
	id, err := domain.ParseSessionID(idValue)
	if err != nil {
		return domain.Session{}, fmt.Errorf("parse stored session ID: %w", err)
	}
	requestID, err := domain.ParseRequestID(requestValue)
	if err != nil {
		return domain.Session{}, fmt.Errorf("parse stored request ID: %w", err)
	}
	createdAt, err := parseUTCTimestamp(createdValue)
	if err != nil {
		return domain.Session{}, fmt.Errorf("parse stored creation time: %w", err)
	}
	updatedAt, err := parseUTCTimestamp(updatedValue)
	if err != nil {
		return domain.Session{}, fmt.Errorf("parse stored update time: %w", err)
	}
	var terminalAt *time.Time
	if terminalValue.Valid {
		parsedTerminal, err := parseUTCTimestamp(terminalValue.String)
		if err != nil {
			return domain.Session{}, fmt.Errorf("parse stored terminal time: %w", err)
		}
		terminalAt = &parsedTerminal
	}
	return domain.RehydrateSession(id, requestID, domain.SessionState(stateValue), createdAt, updatedAt, terminalAt)
}

func scanSessions(rows *sql.Rows) ([]domain.Session, error) {
	values := make([]domain.Session, 0)
	for rows.Next() {
		value, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func (repository *SessionRepository) validateDatabase() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("session database is required")
	}
	return nil
}

func isTerminalSessionState(state domain.SessionState) bool {
	return state == domain.Completed || state == domain.Failed || state == domain.Cancelled || state == domain.Incomplete
}

var _ session.Repository = (*SessionRepository)(nil)
var _ session.StartupReconciler = (*SessionRepository)(nil)
var _ session.TerminalTransitioner = (*SessionRepository)(nil)
