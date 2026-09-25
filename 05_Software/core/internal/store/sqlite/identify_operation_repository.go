package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/audit"
)

// IdentifyOperationRepository keeps the physical indicator action separate
// from its audit append. A failed append can therefore be retried without
// replaying the visible action.
type IdentifyOperationRepository struct{ database *DB }

func NewIdentifyOperationRepository(database *DB) *IdentifyOperationRepository {
	return &IdentifyOperationRepository{database: database}
}

func (repository *IdentifyOperationRepository) State(ctx context.Context, id string) (string, error) {
	if repository == nil || repository.database == nil {
		return "", errors.New("identify operation repository is required")
	}
	var state string
	if err := repository.database.SQL().QueryRowContext(ctx, `SELECT state FROM identify_operations WHERE id = ?`, id).Scan(&state); err != nil {
		return "", fmt.Errorf("load identify operation state: %w", err)
	}
	return state, nil
}

type PendingIdentifyOperation struct {
	ID          string
	Source      string
	State       string
	ActiveUntil time.Time
}

func (repository *IdentifyOperationRepository) Begin(
	ctx context.Context, id, source string, activeUntil, now time.Time,
) (bool, error) {
	if repository == nil || repository.database == nil || id == "" {
		return false, errors.New("identify operation repository and identity are required")
	}
	if source != "interface" && source != "button" {
		return false, errors.New("identify source is invalid")
	}
	if activeUntil.IsZero() {
		return false, errors.New("identify action deadline is required")
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	result, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO identify_operations (
			id, source, state, supported, reannouncement, status, outcome, pending_audit_json, active_until, created_at, updated_at
		) VALUES (?, ?, 'pending', 0, 'not_requested', 'requested', 'requested', '{}', ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, source, activeUntil.UTC().Format(time.RFC3339Nano), stamp, stamp)
	if err != nil {
		return false, fmt.Errorf("begin identify operation: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (repository *IdentifyOperationRepository) Pending(ctx context.Context) ([]PendingIdentifyOperation, error) {
	if repository == nil || repository.database == nil {
		return nil, errors.New("identify operation repository is required")
	}
	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT id, source, state, active_until FROM identify_operations
		WHERE state IN ('reserved', 'performed', 'acted') ORDER BY created_at, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list reserved identify operations: %w", err)
	}
	defer rows.Close()
	var pending []PendingIdentifyOperation
	for rows.Next() {
		var operation PendingIdentifyOperation
		var activeUntil string
		if err := rows.Scan(&operation.ID, &operation.Source, &operation.State, &activeUntil); err != nil {
			return nil, err
		}
		operation.ActiveUntil, err = time.Parse(time.RFC3339Nano, activeUntil)
		if err != nil {
			return nil, fmt.Errorf("parse identify action deadline: %w", err)
		}
		pending = append(pending, operation)
	}
	return pending, rows.Err()
}

func (repository *IdentifyOperationRepository) RecordOutcome(
	ctx context.Context, id string, supported bool, reannouncement, status string, outcome audit.Outcome, now time.Time,
) error {
	if repository == nil || repository.database == nil {
		return errors.New("identify operation repository is required")
	}
	if reannouncement != "not_requested" && reannouncement != "complete" && reannouncement != "partial" && reannouncement != "unavailable" {
		return errors.New("identify reannouncement outcome is invalid")
	}
	if status != "accepted" && status != "unsupported" && status != "failed" {
		return errors.New("identify status is invalid")
	}
	if outcome != audit.OutcomeApplied && outcome != audit.OutcomeFailed {
		return errors.New("identify outcome is invalid")
	}
	var state, storedStatus, storedOutcome string
	var storedSupported bool
	var storedReannouncement string
	if err := repository.database.SQL().QueryRowContext(ctx, `
		SELECT state, supported, reannouncement, status, outcome
		FROM identify_operations WHERE id = ?
	`, id).Scan(&state, &storedSupported, &storedReannouncement, &storedStatus, &storedOutcome); err != nil {
		return fmt.Errorf("load reserved identify operation: %w", err)
	}
	if state == "performed" || state == "acted" {
		if storedSupported == supported && storedReannouncement == reannouncement &&
			storedStatus == status && storedOutcome == string(outcome) {
			return nil
		}
		return errors.New("identify operation outcome conflicts with its persisted result")
	}
	if state != "reserved" {
		return errors.New("identify operation is not reserved")
	}
	result, err := repository.database.SQL().ExecContext(ctx, `
		UPDATE identify_operations
		SET state = 'performed', supported = ?, reannouncement = ?, status = ?, outcome = ?, updated_at = ?
		WHERE id = ? AND state = 'reserved'
	`, supported, reannouncement, status, string(outcome), now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("record identify operation outcome: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("identify operation is not reserved")
	}
	return nil
}

func (repository *IdentifyOperationRepository) MarkActed(ctx context.Context, id string, now time.Time) error {
	if repository == nil || repository.database == nil {
		return errors.New("identify operation repository is required")
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin identify audit finalization: %w", err)
	}
	defer transaction.Rollback()
	var source, state, reannouncement, status, outcomeText, encoded string
	if err := transaction.QueryRowContext(ctx, `
		SELECT source, state, reannouncement, status, outcome, pending_audit_json
		FROM identify_operations WHERE id = ?
	`, id).Scan(&source, &state, &reannouncement, &status, &outcomeText, &encoded); err != nil {
		return fmt.Errorf("load performed identify operation: %w", err)
	}
	if state == "acted" {
		return transaction.Commit()
	}
	if state != "performed" {
		return errors.New("identify operation outcome is not recorded")
	}
	var event audit.Event
	if err := json.Unmarshal([]byte(encoded), &event); err != nil {
		return fmt.Errorf("decode reserved identify audit event: %w", err)
	}
	payload, err := json.Marshal(map[string]string{
		"source": source, "status": status, "reannouncement": identifyAuditReannouncement(source, reannouncement),
	})
	if err != nil {
		return fmt.Errorf("encode identify audit outcome: %w", err)
	}
	event.Outcome = audit.Outcome(outcomeText)
	event.Payload = payload
	if err := event.Validate(); err != nil {
		return err
	}
	encodedEvent, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode identify audit event: %w", err)
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE identify_operations
		SET state = 'acted', pending_audit_json = ?, updated_at = ?
		WHERE id = ? AND state = 'performed'
	`, string(encodedEvent), now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("mark identify operation acted: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("identify operation outcome is not recorded")
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit identify audit finalization: %w", err)
	}
	return nil
}

// Reserve persists the audit envelope before the physical action. A reserved
// operation is deliberately not flushable until its actual outcome is stored.
func (repository *IdentifyOperationRepository) Reserve(ctx context.Context, id string, event audit.Event, now time.Time) error {
	if repository == nil || repository.database == nil {
		return errors.New("identify operation repository is required")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode reserved identify audit event: %w", err)
	}
	result, err := repository.database.SQL().ExecContext(ctx, `
		UPDATE identify_operations
		SET state = 'reserved', pending_audit_json = ?, updated_at = ?
		WHERE id = ? AND state = 'pending'
	`, string(encoded), now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("reserve identify operation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("identify operation is not pending")
	}
	return nil
}

func identifyAuditReannouncement(source, outcome string) string {
	if source == "interface" {
		return "not_requested"
	}
	switch outcome {
	case "complete":
		return "applied"
	case "partial":
		return "failed"
	default:
		return "not_configured"
	}
}

func (repository *IdentifyOperationRepository) Flush(ctx context.Context, id string) error {
	if repository == nil || repository.database == nil {
		return errors.New("identify operation repository is required")
	}
	transaction, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin identify audit flush: %w", err)
	}
	defer transaction.Rollback()
	var state, encoded string
	if err := transaction.QueryRowContext(ctx, `SELECT state, pending_audit_json FROM identify_operations WHERE id = ?`, id).Scan(&state, &encoded); err != nil {
		return fmt.Errorf("load identify operation: %w", err)
	}
	if state == "settled" {
		return transaction.Commit()
	}
	if state != "acted" {
		return errors.New("identify operation has not acted")
	}
	var event audit.Event
	if err := json.Unmarshal([]byte(encoded), &event); err != nil {
		return fmt.Errorf("decode identify audit event: %w", err)
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if err := appendEvent(ctx, transaction, event); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `UPDATE identify_operations SET state = 'settled', updated_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return fmt.Errorf("settle identify operation: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit identify audit flush: %w", err)
	}
	return nil
}

func (repository *IdentifyOperationRepository) FlushPending(ctx context.Context) error {
	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT id, state FROM identify_operations
		WHERE state IN ('performed', 'acted') ORDER BY created_at, id
	`)
	if err != nil {
		return fmt.Errorf("list pending identify audits: %w", err)
	}
	type pendingOperation struct{ id, state string }
	var pending []pendingOperation
	for rows.Next() {
		var operation pendingOperation
		if err := rows.Scan(&operation.id, &operation.state); err != nil {
			_ = rows.Close()
			return err
		}
		pending = append(pending, operation)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, operation := range pending {
		if operation.state == "performed" {
			if err := repository.MarkActed(ctx, operation.id, time.Now().UTC()); err != nil {
				return err
			}
		}
		if err := repository.Flush(ctx, operation.id); err != nil {
			return err
		}
	}
	return nil
}
