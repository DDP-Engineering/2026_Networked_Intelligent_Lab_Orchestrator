package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
)

type AuditRepository struct {
	database *DB
}

func NewAuditRepository(database *DB) *AuditRepository {
	return &AuditRepository{database: database}
}

func (repository *AuditRepository) Append(ctx context.Context, event audit.Event) error {
	if err := repository.validateDatabase(); err != nil {
		return err
	}
	if err := event.Validate(); err != nil {
		return err
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin audit append: %w", err)
	}
	defer transaction.Rollback()
	var sequence int64
	if err := transaction.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM audit_events`).Scan(&sequence); err != nil {
		return fmt.Errorf("allocate audit sequence: %w", err)
	}
	_, err = transaction.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, sequence, actor_id, action, subject_type, subject_id, payload_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, sequence, event.ActorID.String(), event.Action, event.SubjectType, event.SubjectID,
		[]byte(event.Payload), event.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", audit.ErrEventAlreadyAppended, event.ID)
		}
		return fmt.Errorf("insert audit event: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit audit append: %w", err)
	}
	return nil
}

func (repository *AuditRepository) List(ctx context.Context, filter audit.Filter) ([]audit.Event, error) {
	if err := repository.validateDatabase(); err != nil {
		return nil, err
	}
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	query := `SELECT id, sequence, actor_id, action, subject_type, subject_id, payload_json, created_at FROM audit_events`
	conditions := make([]string, 0, 5)
	arguments := make([]any, 0, 6)
	if filter.ActorID.String() != "00000000-0000-0000-0000-000000000000" {
		conditions = append(conditions, "actor_id = ?")
		arguments = append(arguments, filter.ActorID.String())
	}
	if filter.Action != "" {
		conditions = append(conditions, "action = ?")
		arguments = append(arguments, filter.Action)
	}
	if filter.SubjectType != "" {
		conditions = append(conditions, "subject_type = ?")
		arguments = append(arguments, filter.SubjectType)
	}
	if filter.SubjectID != "" {
		conditions = append(conditions, "subject_id = ?")
		arguments = append(arguments, filter.SubjectID)
	}
	if filter.AfterSequence > 0 {
		conditions = append(conditions, "sequence > ?")
		arguments = append(arguments, filter.AfterSequence)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY sequence ASC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		arguments = append(arguments, filter.Limit)
	}
	rows, err := repository.database.SQL().QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	events := make([]audit.Event, 0)
	for rows.Next() {
		var event audit.Event
		var actorValue, createdValue string
		var payload []byte
		if err := rows.Scan(&event.ID, &event.Sequence, &actorValue, &event.Action, &event.SubjectType, &event.SubjectID, &payload, &createdValue); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		event.ActorID, err = domain.ParseActorID(actorValue)
		if err != nil {
			return nil, fmt.Errorf("parse stored audit actor: %w", err)
		}
		event.CreatedAt, err = parseUTCTimestamp(createdValue)
		if err != nil {
			return nil, fmt.Errorf("parse stored audit timestamp: %w", err)
		}
		event.Payload = append([]byte(nil), payload...)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

func (repository *AuditRepository) validateDatabase() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("audit database is required")
	}
	return nil
}

var _ audit.Repository = (*AuditRepository)(nil)
