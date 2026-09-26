package bench

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

type cameraLegacyReader struct {
	io.ReadCloser
	manager *Manager
	id      string
	once    sync.Once
}

func (r *cameraLegacyReader) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(func() {
		r.manager.mu.Lock()
		defer r.manager.mu.Unlock()
		r.manager.cameraReads[r.id]--
		if r.manager.cameraReads[r.id] == 0 {
			delete(r.manager.cameraReads, r.id)
		}
	})
	return err
}

// DeleteLegacyCameraMedia keeps transaction history while removing its image.
func (m *Manager) DeleteLegacyCameraMedia(ctx context.Context, actor identity.Actor, id string) error {
	if !actor.Has(auth.PermissionBenchExecute) {
		return ErrForbidden
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, err := m.job(ctx, id)
	if err != nil {
		return err
	}
	if !canRead(actor, job) {
		return ErrForbidden
	}
	if job.PluginID != "camera" || job.Submission.Operation != "snapshot" {
		return ErrInvalid
	}
	if err = m.record(ctx, actor, "camera.legacy_delete_requested", id, nil); err != nil {
		return err
	}
	return m.removeLegacyCameraMedia(ctx, job)
}

func (m *Manager) removeLegacyCameraMedia(ctx context.Context, job Job) error {
	if m.cameraReads[job.ID] > 0 {
		return ErrBusy
	}
	if job.ArtifactID == "" {
		return m.finishLegacyDeletions(ctx)
	}
	if _, err := m.db().ExecContext(ctx, `INSERT OR IGNORE INTO camera_legacy_deletions(job_id,session_id,artifact_id) VALUES(?,?,?)`, job.ID, job.SessionID, job.ArtifactID); err != nil {
		return err
	}
	return m.finishLegacyDeletions(ctx)
}

func (m *Manager) finishLegacyDeletions(ctx context.Context) error {
	rows, err := m.db().QueryContext(ctx, `SELECT job_id,session_id,artifact_id FROM camera_legacy_deletions WHERE complete=0`)
	if err != nil {
		return err
	}
	type removal struct{ job, session, artifact string }
	var entries []removal
	for rows.Next() {
		var item removal
		if err = rows.Scan(&item.job, &item.session, &item.artifact); err != nil {
			break
		}
		entries = append(entries, item)
	}
	rows.Close()
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Join(m.options.DataDir, "artifacts"))
	if err != nil {
		return err
	}
	defer root.Close()
	for _, item := range entries {
		if _, err = uuid.Parse(item.session); err != nil {
			return ErrInvalid
		}
		if _, err = uuid.Parse(item.artifact); err != nil {
			return ErrInvalid
		}
		if err = root.Remove(filepath.Join(item.session, item.artifact)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		tx, err := m.db().BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE bench_jobs SET artifact_id='' WHERE id=?`, item.job); err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE camera_legacy_deletions SET complete=1 WHERE job_id=?`, item.job)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
