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

func (repository *IdentifyOperationRepository) Begin(ctx context.Context, id, source string, now time.Time) (bool, error) {
	if repository == nil || repository.database == nil || id == "" {
		return false, errors.New("identify operation repository and identity are required")
	}
	if source != "interface" && source != "button" {
		return false, errors.New("identify source is invalid")
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	result, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO identify_operations (
			id, source, state, supported, reannouncement, pending_audit_json, created_at, updated_at
		) VALUES (?, ?, 'pending', 0, 'not_requested', '{}', ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, source, stamp, stamp)
	if err != nil {
		return false, fmt.Errorf("begin identify operation: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (repository *IdentifyOperationRepository) MarkActed(
	ctx context.Context, id string, supported bool, reannouncement string, event audit.Event, now time.Time,
) error {
	if repository == nil || repository.database == nil {
		return errors.New("identify operation repository is required")
	}
	if reannouncement != "not_requested" && reannouncement != "complete" && reannouncement != "partial" && reannouncement != "unavailable" {
		return errors.New("identify reannouncement outcome is invalid")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode identify audit event: %w", err)
	}
	result, err := repository.database.SQL().ExecContext(ctx, `
		UPDATE identify_operations
		SET state = 'acted', supported = ?, reannouncement = ?, pending_audit_json = ?, updated_at = ?
		WHERE id = ? AND state = 'pending'
	`, supported, reannouncement, string(encoded), now.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("mark identify operation acted: %w", err)
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
	rows, err := repository.database.SQL().QueryContext(ctx, `SELECT id FROM identify_operations WHERE state = 'acted' ORDER BY created_at, id`)
	if err != nil {
		return fmt.Errorf("list pending identify audits: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := repository.Flush(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
