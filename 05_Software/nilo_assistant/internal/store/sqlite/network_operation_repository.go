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

// maximumRecentOperations bounds a reattachment listing. The interface needs
// what is in flight and a little history around it, not the whole log.
const maximumRecentOperations = 200

// NetworkOperationRepository persists managed network changes, so that a change
// still in flight survives a page reload, a core restart and a reboot.
//
// It stores public metadata only. A record whose parameters name a key, a
// password or raw profile text is refused rather than redacted: redaction is a
// promise that has to be kept everywhere, and refusing at the boundary is one
// place instead of many.
type NetworkOperationRepository struct {
	database *DB
}

func NewNetworkOperationRepository(database *DB) *NetworkOperationRepository {
	return &NetworkOperationRepository{database: database}
}

func (repository *NetworkOperationRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("network operation repository database is required")
	}
	return nil
}

// Save writes one operation, replacing the previous state of the same
// operation. One change is one row for its whole life, so a reattachment
// listing never shows one change twice.
func (repository *NetworkOperationRepository) Save(ctx context.Context, record network.OperationRecord) error {
	if err := repository.ready(); err != nil {
		return err
	}
	if err := network.ValidateRecord(record); err != nil {
		return err
	}
	parameters := "{}"
	if len(record.Parameters) > 0 {
		parameters = string(record.Parameters)
	}
	var deadline any
	if !record.ConfirmDeadline.IsZero() {
		deadline = record.ConfirmDeadline.UTC().Format(time.RFC3339Nano)
	}
	if _, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO network_operations (
			id, action, target, actor_id, state, revision, parameters_json,
			checkpoint_id, failure, created_at, updated_at, confirm_deadline
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			state = excluded.state,
			revision = excluded.revision,
			parameters_json = excluded.parameters_json,
			checkpoint_id = excluded.checkpoint_id,
			failure = excluded.failure,
			updated_at = excluded.updated_at,
			confirm_deadline = excluded.confirm_deadline
	`,
		record.ID, record.Action, record.Target, record.ActorID, record.State,
		record.Revision, parameters, record.CheckpointID, record.Failure,
		record.CreatedAt.UTC().Format(time.RFC3339Nano),
		record.UpdatedAt.UTC().Format(time.RFC3339Nano),
		deadline,
	); err != nil {
		return fmt.Errorf("store network operation: %w", err)
	}
	return nil
}

// Unfinished returns every operation a restart still has to resolve, oldest
// first, because they are reconciled in the order they were started.
func (repository *NetworkOperationRepository) Unfinished(ctx context.Context) ([]network.OperationRecord, error) {
	return repository.query(ctx,
		"WHERE state IN ('previewed', 'applying', 'awaiting_confirmation', 'rolling_back') ORDER BY created_at ASC", 0)
}

// Recent returns the newest operations first, for an interface reattaching
// after a reload.
func (repository *NetworkOperationRepository) Recent(ctx context.Context, limit int) ([]network.OperationRecord, error) {
	if limit <= 0 || limit > maximumRecentOperations {
		limit = maximumRecentOperations
	}
	return repository.query(ctx, "ORDER BY created_at DESC, id DESC", limit)
}

func (repository *NetworkOperationRepository) query(
	ctx context.Context, clause string, limit int,
) (records []network.OperationRecord, err error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	query := `
		SELECT id, action, target, actor_id, state, revision, parameters_json,
		       checkpoint_id, failure, created_at, updated_at, confirm_deadline
		FROM network_operations ` + clause
	arguments := make([]any, 0, 1)
	if limit > 0 {
		query += " LIMIT ?"
		arguments = append(arguments, limit)
	}
	rows, err := repository.database.SQL().QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("read network operations: %w", err)
	}
	defer func() {
		closeErr := rows.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close network operation rows: %w", closeErr)
		}
	}()

	records = make([]network.OperationRecord, 0)
	for rows.Next() {
		var record network.OperationRecord
		var parameters, created, updated string
		var deadline sql.NullString
		if err := rows.Scan(
			&record.ID, &record.Action, &record.Target, &record.ActorID, &record.State,
			&record.Revision, &parameters, &record.CheckpointID, &record.Failure,
			&created, &updated, &deadline,
		); err != nil {
			return nil, fmt.Errorf("scan network operation: %w", err)
		}
		record.Parameters = json.RawMessage(parameters)
		if record.CreatedAt, err = parseUTCTimestamp(created); err != nil {
			return nil, fmt.Errorf("parse stored operation creation time: %w", err)
		}
		if record.UpdatedAt, err = parseUTCTimestamp(updated); err != nil {
			return nil, fmt.Errorf("parse stored operation update time: %w", err)
		}
		if deadline.Valid {
			if record.ConfirmDeadline, err = parseUTCTimestamp(deadline.String); err != nil {
				return nil, fmt.Errorf("parse stored operation deadline: %w", err)
			}
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate network operations: %w", err)
	}
	return records, nil
}

var _ network.OperationStore = (*NetworkOperationRepository)(nil)
