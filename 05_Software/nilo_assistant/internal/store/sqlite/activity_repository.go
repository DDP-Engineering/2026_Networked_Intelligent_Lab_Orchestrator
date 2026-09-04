package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nilo.local/nilo/internal/activity"
	"nilo.local/nilo/internal/audit"
)

// ActivityRepository is the audit repository with what the Activity page and
// the retention sweeper need on top: the retained window, the policy, the
// watermarks and the bounded purges.
type ActivityRepository struct {
	*AuditRepository
	database *DB
}

func NewActivityRepository(database *DB) *ActivityRepository {
	return &ActivityRepository{AuditRepository: NewAuditRepository(database), database: database}
}

func (repository *ActivityRepository) ready() error {
	if repository == nil || repository.database == nil || repository.database.SQL() == nil {
		return errors.New("activity database is required")
	}
	return nil
}

// RetainedWindow reports what each category holds now.
func (repository *ActivityRepository) RetainedWindow(ctx context.Context) (map[audit.Category]activity.Window, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	rows, err := repository.database.SQL().QueryContext(ctx, `
		SELECT COALESCE(category, ?), MIN(created_at), MAX(created_at), COUNT(*), COALESCE(SUM(LENGTH(payload_json)), 0)
		FROM audit_events GROUP BY COALESCE(category, ?)
	`, string(audit.CategoryAudit), string(audit.CategoryAudit))
	if err != nil {
		return nil, fmt.Errorf("read retained window: %w", err)
	}
	defer rows.Close()
	windows := map[audit.Category]activity.Window{}
	for rows.Next() {
		var category, oldest, newest string
		var count, payloadBytes int64
		if err := rows.Scan(&category, &oldest, &newest, &count, &payloadBytes); err != nil {
			return nil, fmt.Errorf("scan retained window: %w", err)
		}
		window := activity.Window{Count: count, Bytes: payloadBytes + count*activity.RowOverheadBytes}
		if window.Oldest, err = time.Parse(time.RFC3339Nano, oldest); err != nil {
			return nil, fmt.Errorf("parse retained window: %w", err)
		}
		if window.Newest, err = time.Parse(time.RFC3339Nano, newest); err != nil {
			return nil, fmt.Errorf("parse retained window: %w", err)
		}
		windows[audit.Category(category)] = window
	}
	return windows, rows.Err()
}

// PurgeExpired deletes at most limit records of one category older than the
// cutoff, keeping every record of an operation that is still pending.
func (repository *ActivityRepository) PurgeExpired(
	ctx context.Context, category audit.Category, cutoff time.Time, limit int, keep []string,
) (int, error) {
	if err := repository.ready(); err != nil {
		return 0, err
	}
	if category == "" || limit <= 0 {
		return 0, fmt.Errorf("%w: retention needs a category and a positive batch", audit.ErrInvalidEvent)
	}
	query := `DELETE FROM audit_events WHERE id IN (
		SELECT id FROM audit_events
		WHERE COALESCE(category, ?) = ? AND created_at < ?`
	arguments := []any{string(audit.CategoryAudit), string(category), cutoff.UTC().Format(time.RFC3339Nano)}
	if len(keep) > 0 {
		query += " AND (operation_id IS NULL OR operation_id NOT IN (" + strings.TrimSuffix(strings.Repeat("?,", len(keep)), ",") + "))"
		for _, id := range keep {
			arguments = append(arguments, id)
		}
	}
	query += " ORDER BY sequence ASC LIMIT ?)"
	arguments = append(arguments, limit)
	result, err := repository.database.SQL().ExecContext(ctx, query, arguments...)
	if err != nil {
		return 0, fmt.Errorf("purge expired activity: %w", err)
	}
	removed, err := result.RowsAffected()
	return int(removed), err
}

// PurgeOldest deletes at most limit of the oldest records of one category and
// reports the newest timestamp it removed, which becomes the watermark.
func (repository *ActivityRepository) PurgeOldest(ctx context.Context, category audit.Category, limit int) (int, time.Time, error) {
	if err := repository.ready(); err != nil {
		return 0, time.Time{}, err
	}
	if category == "" || limit <= 0 {
		return 0, time.Time{}, fmt.Errorf("%w: retention needs a category and a positive batch", audit.ErrInvalidEvent)
	}
	var newest sql.NullString
	err := repository.database.SQL().QueryRowContext(ctx, `
		SELECT MAX(created_at) FROM (
			SELECT created_at FROM audit_events WHERE COALESCE(category, ?) = ? ORDER BY sequence ASC LIMIT ?
		)`, string(audit.CategoryAudit), string(category), limit).Scan(&newest)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("find oldest activity: %w", err)
	}
	if !newest.Valid {
		return 0, time.Time{}, nil
	}
	result, err := repository.database.SQL().ExecContext(ctx, `
		DELETE FROM audit_events WHERE id IN (
			SELECT id FROM audit_events WHERE COALESCE(category, ?) = ? ORDER BY sequence ASC LIMIT ?
		)`, string(audit.CategoryAudit), string(category), limit)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("purge oldest activity: %w", err)
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, time.Time{}, err
	}
	at, err := time.Parse(time.RFC3339Nano, newest.String)
	if err != nil {
		return int(removed), time.Time{}, fmt.Errorf("parse purged timestamp: %w", err)
	}
	return int(removed), at, nil
}

// LoadPolicy reads the administrator's policy; absent means the default.
func (repository *ActivityRepository) LoadPolicy(ctx context.Context) (*activity.Policy, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	var encoded string
	err := repository.database.SQL().QueryRowContext(ctx, `SELECT policy_json FROM activity_policy WHERE id = 1`).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read activity policy: %w", err)
	}
	var policy activity.Policy
	if err := json.Unmarshal([]byte(encoded), &policy); err != nil {
		return nil, fmt.Errorf("decode activity policy: %w", err)
	}
	return &policy, nil
}

// SavePolicy replaces the policy.
func (repository *ActivityRepository) SavePolicy(ctx context.Context, policy activity.Policy, at time.Time) error {
	if err := repository.ready(); err != nil {
		return err
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	_, err = repository.database.SQL().ExecContext(ctx, `
		INSERT INTO activity_policy (id, policy_json, updated_at) VALUES (1, ?, ?)
		ON CONFLICT (id) DO UPDATE SET policy_json = excluded.policy_json, updated_at = excluded.updated_at
	`, string(encoded), at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save activity policy: %w", err)
	}
	return nil
}

// Watermarks reads what retention has removed so far.
func (repository *ActivityRepository) Watermarks(ctx context.Context) (map[audit.Category]activity.Watermark, error) {
	if err := repository.ready(); err != nil {
		return nil, err
	}
	rows, err := repository.database.SQL().QueryContext(ctx, `SELECT category, purged_before, reason, updated_at FROM retention_watermarks`)
	if err != nil {
		return nil, fmt.Errorf("read retention watermarks: %w", err)
	}
	defer rows.Close()
	watermarks := map[audit.Category]activity.Watermark{}
	for rows.Next() {
		var category, purgedBefore, reason, updatedAt string
		if err := rows.Scan(&category, &purgedBefore, &reason, &updatedAt); err != nil {
			return nil, err
		}
		watermark := activity.Watermark{Reason: reason}
		if watermark.PurgedBefore, err = time.Parse(time.RFC3339Nano, purgedBefore); err != nil {
			return nil, err
		}
		if watermark.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, err
		}
		watermarks[audit.Category(category)] = watermark
	}
	return watermarks, rows.Err()
}

// SetWatermark records what retention removed for one category.
func (repository *ActivityRepository) SetWatermark(ctx context.Context, category audit.Category, watermark activity.Watermark) error {
	if err := repository.ready(); err != nil {
		return err
	}
	_, err := repository.database.SQL().ExecContext(ctx, `
		INSERT INTO retention_watermarks (category, purged_before, reason, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (category) DO UPDATE SET purged_before = excluded.purged_before, reason = excluded.reason, updated_at = excluded.updated_at
	`, string(category), watermark.PurgedBefore.UTC().Format(time.RFC3339Nano), watermark.Reason, watermark.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save retention watermark: %w", err)
	}
	return nil
}

var _ activity.Store = (*ActivityRepository)(nil)
