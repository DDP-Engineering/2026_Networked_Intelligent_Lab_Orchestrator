package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
)

type AuditRepository struct {
	database *DB
	// bound is set only on the repository handed to an administrative
	// transaction, so the event joins the change it attributes.
	bound statements
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
	if repository.bound != nil {
		return appendEvent(ctx, repository.bound, event)
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin audit append: %w", err)
	}
	defer transaction.Rollback()
	if err := appendEvent(ctx, transaction, event); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit audit append: %w", err)
	}
	return nil
}

// appendEvent allocates the next sequence and writes one event through the
// caller's transaction, so the same statements serve a standalone append and an
// append that belongs to an administrative change.
func appendEvent(ctx context.Context, execute statements, event audit.Event) error {
	sequence, err := allocateSequence(ctx, execute)
	if err != nil {
		return err
	}
	_, err = execute.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, sequence, actor_id, action, subject_type, subject_id, payload_json, created_at,
			envelope_version, appliance_id, boot_id, category, component,
			actor_kind, outcome, severity, operation_id, target_label, time_source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, sequence, event.ActorID.String(), event.Action, event.SubjectType, event.SubjectID,
		[]byte(event.Payload), event.CreatedAt.UTC().Format(time.RFC3339Nano),
		nullableEnvelopeVersion(event.EnvelopeVersion),
		nullableText(event.ApplianceID), nullableText(event.BootID),
		nullableText(string(event.Category)), nullableText(event.Component),
		nullableText(string(event.ActorKind)), nullableText(string(event.Outcome)),
		nullableText(string(event.Severity)), nullableText(event.OperationID),
		nullableText(event.TargetLabel), nullableText(string(event.TimeSource)))
	if err != nil {
		if isUniqueConstraint(err) {
			return fmt.Errorf("%w: %s", audit.ErrEventAlreadyAppended, event.ID)
		}
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

// allocateSequence hands out the next ordering number from the persisted
// allocator, inside the caller's transaction.
//
// It is deliberately not MAX(sequence)+1. That expression is correct only while
// rows are never deleted, and retention deletes them: once a sweep empties the
// table it would start again at one and reuse numbers that already identify
// exported events.
// The allocator also never falls behind a sequence that is already stored. It
// cannot happen through this repository, but a database restored from a partial
// copy, or a row written by an older binary, would otherwise collide once and
// then be indistinguishable from an ordinary duplicate.
func allocateSequence(ctx context.Context, execute statements) (int64, error) {
	var sequence int64
	err := execute.QueryRowContext(ctx, `
		UPDATE audit_sequence
		SET next = MAX(next, (SELECT COALESCE(MAX(sequence), 0) FROM audit_events) + 1) + 1
		WHERE id = 1
		RETURNING next - 1
	`).Scan(&sequence)
	if err != nil {
		return 0, fmt.Errorf("allocate audit sequence: %w", err)
	}
	return sequence, nil
}

// nullableText stores an unset envelope field as NULL rather than as an empty
// string, so a row written before the envelope existed and a row whose writer
// deliberately left a field empty are the same thing to a reader: nothing was
// recorded here.
func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableEnvelopeVersion(version int) any {
	if version <= 0 {
		return nil
	}
	return int64(version)
}

func (repository *AuditRepository) List(ctx context.Context, filter audit.Filter) ([]audit.Event, error) {
	if err := repository.validateDatabase(); err != nil {
		return nil, err
	}
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	query := `SELECT id, sequence, actor_id, action, subject_type, subject_id, payload_json, created_at,
		envelope_version, appliance_id, boot_id, category, component,
		actor_kind, outcome, severity, operation_id, target_label, time_source
		FROM audit_events`
	conditions, arguments := auditFilterConditions(filter, true)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	if filter.Sort == "" {
		query += " ORDER BY sequence ASC"
	} else {
		direction := "ASC"
		if filter.Order == audit.SortDescending {
			direction = "DESC"
		}
		query += " ORDER BY " + auditSortExpression(filter.Sort) + " " + direction + ", sequence " + direction
	}
	if filter.Limit > 0 {
		query += " LIMIT ?"
		arguments = append(arguments, filter.Limit)
		if filter.Offset > 0 {
			query += " OFFSET ?"
			arguments = append(arguments, filter.Offset)
		}
	}
	rows, err := repository.execute().QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	events := make([]audit.Event, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}

// Count reports the complete filtered result size. Cursor and page bounds are
// deliberately ignored so the UI can calculate real page numbers.
func (repository *AuditRepository) Count(ctx context.Context, filter audit.Filter) (int64, error) {
	if err := repository.validateDatabase(); err != nil {
		return 0, err
	}
	if err := filter.Validate(); err != nil {
		return 0, err
	}
	conditions, arguments := auditFilterConditions(filter, false)
	query := "SELECT COUNT(*) FROM audit_events"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	var count int64
	if err := repository.execute().QueryRowContext(ctx, query, arguments...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count audit events: %w", err)
	}
	return count, nil
}

func auditFilterConditions(filter audit.Filter, includeCursor bool) ([]string, []any) {
	conditions := make([]string, 0, 12)
	arguments := make([]any, 0, 14)
	if filter.ActorID.String() != "00000000-0000-0000-0000-000000000000" {
		conditions = append(conditions, "actor_id = ?")
		arguments = append(arguments, filter.ActorID.String())
	}
	for _, match := range []struct{ column, value string }{
		{"action", filter.Action},
		{"subject_type", filter.SubjectType},
		{"subject_id", filter.SubjectID},
		{"category", string(filter.Category)},
		{"outcome", string(filter.Outcome)},
		{"severity", string(filter.Severity)},
		{"operation_id", filter.OperationID},
	} {
		if match.value != "" {
			conditions = append(conditions, match.column+" = ?")
			arguments = append(arguments, match.value)
		}
	}
	if strings.HasSuffix(filter.Component, ".*") {
		prefix := strings.TrimSuffix(filter.Component, "*")
		conditions = append(conditions, `component LIKE ? ESCAPE '\'`)
		arguments = append(arguments, escapeLike(prefix)+"%")
	} else if filter.Component != "" {
		conditions = append(conditions, "component = ?")
		arguments = append(arguments, filter.Component)
	}
	if includeCursor && filter.Sort != "" && filter.AfterSequence > 0 {
		expression := auditSortExpression(filter.Sort)
		operator := ">"
		if filter.Order == audit.SortDescending {
			operator = "<"
		}
		conditions = append(conditions, fmt.Sprintf("(%s %s ? OR (%s = ? AND sequence %s ?))", expression, operator, expression, operator))
		arguments = append(arguments, filter.CursorValue, filter.CursorValue, filter.AfterSequence)
	} else if includeCursor && filter.AfterSequence > 0 {
		conditions = append(conditions, "sequence > ?")
		arguments = append(arguments, filter.AfterSequence)
	}
	if !filter.From.IsZero() {
		conditions = append(conditions, "created_at >= ?")
		arguments = append(arguments, filter.From.UTC().Format(time.RFC3339Nano))
	}
	if !filter.To.IsZero() {
		conditions = append(conditions, "created_at < ?")
		arguments = append(arguments, filter.To.UTC().Format(time.RFC3339Nano))
	}
	return conditions, arguments
}

func auditSortExpression(field audit.SortField) string {
	switch field {
	case audit.SortTime:
		return "strftime('%Y-%m-%dT%H:%M:%fZ', created_at)"
	case audit.SortActivity:
		return "action"
	case audit.SortOutcome:
		return "COALESCE(outcome, '')"
	case audit.SortActor:
		return "COALESCE(actor_kind, '') || char(31) || actor_id"
	case audit.SortTarget:
		return "COALESCE(NULLIF(target_label, ''), subject_id)"
	default:
		return "sequence"
	}
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func scanEvent(rows *sql.Rows) (audit.Event, error) {
	var event audit.Event
	var actorValue, createdValue string
	var payload []byte
	var envelopeVersion sql.NullInt64
	var applianceID, bootID, category, component sql.NullString
	var actorKind, outcome, severity, operationID, targetLabel, timeSource sql.NullString
	if err := rows.Scan(
		&event.ID, &event.Sequence, &actorValue, &event.Action, &event.SubjectType,
		&event.SubjectID, &payload, &createdValue,
		&envelopeVersion, &applianceID, &bootID, &category, &component,
		&actorKind, &outcome, &severity, &operationID, &targetLabel, &timeSource,
	); err != nil {
		return audit.Event{}, fmt.Errorf("scan audit event: %w", err)
	}
	actor, err := domain.ParseActorID(actorValue)
	if err != nil {
		return audit.Event{}, fmt.Errorf("parse stored audit actor: %w", err)
	}
	event.ActorID = actor
	event.CreatedAt, err = parseUTCTimestamp(createdValue)
	if err != nil {
		return audit.Event{}, fmt.Errorf("parse stored audit timestamp: %w", err)
	}
	event.Payload = append([]byte(nil), payload...)

	// A row with no recorded version was written before the envelope existed.
	// It reads back as version 1 with empty envelope fields, which says the
	// writer recorded nothing there rather than asserting a value it never held.
	event.EnvelopeVersion = 1
	if envelopeVersion.Valid {
		event.EnvelopeVersion = int(envelopeVersion.Int64)
	}
	event.ApplianceID = applianceID.String
	event.BootID = bootID.String
	event.Category = audit.Category(category.String)
	event.Component = component.String
	event.ActorKind = audit.ActorKind(actorKind.String)
	event.Outcome = audit.Outcome(outcome.String)
	event.Severity = audit.Severity(severity.String)
	event.OperationID = operationID.String
	event.TargetLabel = targetLabel.String
	event.TimeSource = audit.TimeSource(timeSource.String)
	return event, nil
}

// PurgeBefore removes at most limit expired records of one category and reports
// how many it removed, so retention runs in bounded batches rather than as one
// unbounded delete on the single writer connection.
//
// It never touches the sequence allocator, which is what keeps ordering
// monotonic across a sweep that empties the table.
func (repository *AuditRepository) PurgeBefore(
	ctx context.Context, category audit.Category, cutoff time.Time, limit int,
) (int, error) {
	if err := repository.validateDatabase(); err != nil {
		return 0, err
	}
	if category == "" {
		return 0, fmt.Errorf("%w: retention needs the category to purge", audit.ErrInvalidEvent)
	}
	if limit <= 0 {
		return 0, fmt.Errorf("%w: retention batch size must be positive", audit.ErrInvalidEvent)
	}
	// A row written before the envelope existed has no category. It is treated
	// as audit, which is the category it would have been given, so the oldest
	// records are not exempt from retention forever.
	result, err := repository.execute().ExecContext(ctx, `
		DELETE FROM audit_events WHERE id IN (
			SELECT id FROM audit_events
			WHERE COALESCE(category, ?) = ? AND created_at < ?
			ORDER BY sequence ASC LIMIT ?
		)
	`, string(audit.CategoryAudit), string(category), cutoff.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return 0, fmt.Errorf("purge expired audit events: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count purged audit events: %w", err)
	}
	return int(removed), nil
}

func (repository *AuditRepository) validateDatabase() error {
	if repository == nil {
		return errors.New("audit database is required")
	}
	if repository.bound != nil {
		return nil
	}
	if repository.database == nil || repository.database.SQL() == nil {
		return errors.New("audit database is required")
	}
	return nil
}

func (repository *AuditRepository) execute() statements {
	if repository.bound != nil {
		return repository.bound
	}
	return repository.database.SQL()
}

var _ audit.Repository = (*AuditRepository)(nil)
