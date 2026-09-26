package bench

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
)

// CameraRetention bounds acquisition independently of browser connectivity.
type CameraRetention struct {
	Snapshots    int   `json:"snapshots"`
	TotalBytes   int64 `json:"total_bytes"`
	VideoBytes   int64 `json:"video_bytes"`
	VideoSeconds int   `json:"video_seconds"`
}

func defaultCameraRetention() CameraRetention {
	return CameraRetention{Snapshots: 100, TotalBytes: 2 << 30, VideoBytes: 500 << 20, VideoSeconds: 600}
}
func (r CameraRetention) valid() bool {
	return r.Snapshots >= 1 && r.Snapshots <= 10000 && r.TotalBytes >= 64<<20 && r.TotalBytes <= 100<<30 && r.VideoBytes >= 8<<20 && r.VideoBytes+(2<<20) <= r.TotalBytes && r.VideoSeconds >= 1 && r.VideoSeconds <= 3500
}

// cameraMediaRows unifies both storage layouts without copying retained files.
const cameraMediaRows = `SELECT id,camera_id,owner_id,kind,state,reserved_bytes,created_at,metadata_json,'' AS artifact_id FROM camera_media
UNION ALL SELECT j.id,json_extract(j.submission_json,'$.device_id'),j.owner_id,'image',s.state,a.size_bytes,j.created_at,
json_object('extension',CASE a.media_type WHEN 'image/png' THEN 'png' ELSE 'jpg' END),j.artifact_id
FROM bench_jobs j JOIN artifacts a ON a.id=j.artifact_id JOIN sessions s ON s.id=j.session_id
WHERE j.plugin_id='camera' AND json_extract(j.submission_json,'$.operation')='snapshot'`

type CameraMedia struct {
	artifactID string
	ID         string          `json:"id"`
	CameraID   string          `json:"camera_id"`
	OwnerID    string          `json:"owner_id"`
	Kind       string          `json:"kind"`
	State      string          `json:"state"`
	Bytes      int64           `json:"bytes"`
	CreatedAt  string          `json:"created_at"`
	Metadata   json.RawMessage `json:"metadata"`
}

func (m *Manager) recoverCameraMedia(ctx context.Context) error {
	if err := m.finishLegacyDeletions(ctx); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(m.options.DataDir, "camera-workspaces"), 0700); err != nil {
		return err
	}
	rows, err := m.db().QueryContext(ctx, `SELECT DISTINCT camera_id FROM camera_media WHERE state='pending'`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !cameraIDPattern.MatchString(id) {
			return ErrInvalid
		}
		if err = m.reconcileCameraMedia(ctx, id); err != nil {
			return err
		}
	}
	deleting, err := m.db().QueryContext(ctx, `SELECT camera_id,id FROM camera_media WHERE state='deleting'`)
	if err != nil {
		return err
	}
	type removal struct{ camera, id string }
	var removals []removal
	for deleting.Next() {
		var value removal
		if err = deleting.Scan(&value.camera, &value.id); err != nil {
			break
		}
		removals = append(removals, value)
	}
	deleting.Close()
	if err != nil {
		return err
	}
	for _, value := range removals {
		if err = m.removeCameraMedia(ctx, value.camera, value.id); err != nil {
			return err
		}
	}
	_, err = m.db().ExecContext(ctx, `UPDATE camera_media SET state='incomplete' WHERE state='pending'`)
	return err
}

func (m *Manager) reconcileCameraMedia(ctx context.Context, camera string) error {
	rows, err := m.db().QueryContext(ctx, `SELECT id,reserved_bytes FROM camera_media WHERE camera_id=? AND state='pending'`, camera)
	if err != nil {
		return err
	}
	type pending struct {
		id       string
		reserved int64
	}
	pendingItems := []pending{}
	for rows.Next() {
		var item pending
		if err = rows.Scan(&item.id, &item.reserved); err != nil {
			break
		}
		pendingItems = append(pendingItems, item)
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range pendingItems {
		metadata, readErr := os.Open(filepath.Join(m.cameraRoot(camera), item.id+".json"))
		if errors.Is(readErr, os.ErrNotExist) {
			if !m.cameraBusy(camera) {
				if _, err := m.db().ExecContext(ctx, `UPDATE camera_media SET state='incomplete' WHERE id=? AND state='pending'`, item.id); err != nil {
					return err
				}
			}
			continue
		}
		if readErr != nil {
			return readErr
		}
		raw, readErr := io.ReadAll(io.LimitReader(metadata, 32769))
		metadata.Close()
		if readErr != nil {
			return readErr
		}
		var meta struct {
			ID        string `json:"id"`
			Extension string `json:"extension"`
			Bytes     int64  `json:"bytes"`
			SHA256    string `json:"sha256"`
		}
		if len(raw) > 32768 || json.Unmarshal(raw, &meta) != nil || meta.ID != item.id || !mediaExtension(meta.Extension) || meta.Bytes < 1 || meta.Bytes > item.reserved || len(meta.SHA256) != 64 {
			if err := m.incompleteCameraMedia(ctx, item.id); err != nil {
				return err
			}
			continue
		}
		info, err := os.Lstat(filepath.Join(m.cameraRoot(camera), item.id+"."+meta.Extension))
		if err != nil || !info.Mode().IsRegular() || info.Size() != meta.Bytes {
			if err := m.incompleteCameraMedia(ctx, item.id); err != nil {
				return err
			}
			continue
		}
		file, err := os.Open(filepath.Join(m.cameraRoot(camera), item.id+"."+meta.Extension))
		if err != nil {
			return err
		}
		digest := sha256.New()
		_, err = io.Copy(digest, file)
		file.Close()
		if err != nil || fmt.Sprintf("%x", digest.Sum(nil)) != meta.SHA256 {
			if err := m.incompleteCameraMedia(ctx, item.id); err != nil {
				return err
			}
			continue
		}
		thumb, err := os.Lstat(filepath.Join(m.cameraRoot(camera), item.id+".thumb.jpg"))
		if err != nil || !thumb.Mode().IsRegular() {
			if err := m.incompleteCameraMedia(ctx, item.id); err != nil {
				return err
			}
			continue
		}
		total := meta.Bytes + thumb.Size() + int64(len(raw))
		if total > item.reserved {
			if err := m.incompleteCameraMedia(ctx, item.id); err != nil {
				return err
			}
			continue
		}
		if _, err = m.db().ExecContext(ctx, `UPDATE camera_media SET state='completed',reserved_bytes=?,metadata_json=? WHERE id=? AND state='pending'`, total, string(raw), item.id); err != nil {
			return err
		}
	}
	return nil
}
func (m *Manager) incompleteCameraMedia(ctx context.Context, id string) error {
	_, err := m.db().ExecContext(ctx, `UPDATE camera_media SET state='incomplete' WHERE id=? AND state='pending'`, id)
	return err
}

func mediaExtension(value string) bool { return value == "jpg" || value == "png" || value == "mp4" }

func (m *Manager) cameraUsed(ctx context.Context, id string) (int64, error) {
	var used int64
	err := m.db().QueryRowContext(ctx, `SELECT COALESCE(SUM(reserved_bytes),0) FROM (`+cameraMediaRows+`) WHERE camera_id=? AND state!='deleted'`, id).Scan(&used)
	return used, err
}

func (m *Manager) reserveCameraMedia(ctx context.Context, camera, owner, id, kind string, reservation int64, limits CameraRetention) error {
	if !limits.valid() {
		return ErrInvalid
	}
	used, err := m.cameraUsed(ctx, camera)
	if err != nil {
		return err
	}
	var count int
	if err = m.db().QueryRowContext(ctx, `SELECT count(*) FROM (`+cameraMediaRows+`) WHERE camera_id=? AND kind='image' AND state!='deleted'`, camera).Scan(&count); err != nil {
		return err
	}
	var protectedBytes int64
	var protectedCount int
	if err = m.db().QueryRowContext(ctx, `SELECT COALESCE(SUM(reserved_bytes),0),COALESCE(SUM(CASE WHEN kind='image' THEN 1 ELSE 0 END),0) FROM (`+cameraMediaRows+`) WHERE camera_id=? AND state NOT IN ('completed','deleted')`, camera).Scan(&protectedBytes, &protectedCount); err != nil {
		return err
	}
	if protectedBytes+reservation > limits.TotalBytes || kind == "image" && protectedCount >= limits.Snapshots {
		return errCameraStorage
	}
	total, free, err := m.cameraSpace(m.options.DataDir)
	if err != nil {
		return err
	}
	var reserved int64
	if err = m.db().QueryRowContext(ctx, `SELECT COALESCE(SUM(reserved_bytes),0) FROM camera_media WHERE state='pending'`).Scan(&reserved); err != nil {
		return err
	}
	if free-reserved-reservation < max(int64(2<<30), total/10) {
		return errCameraStorage
	}
	// All completed captures follow the same oldest-first retention policy.
	for used+reservation > limits.TotalBytes || kind == "image" && count >= limits.Snapshots {
		query := `SELECT id,reserved_bytes,kind,artifact_id!='' FROM (` + cameraMediaRows + `) WHERE camera_id=? AND state='completed'`
		args := []any{camera}
		if kind == "image" && count >= limits.Snapshots {
			query += ` AND kind='image'`
		}
		for held := range m.cameraReads {
			query += ` AND id!=?`
			args = append(args, held)
		}
		query += ` ORDER BY created_at,id LIMIT 1`
		var old, oldKind string
		var size int64
		var legacy bool
		if err = m.db().QueryRowContext(ctx, query, args...).Scan(&old, &size, &oldKind, &legacy); err != nil {
			return errCameraStorage
		}
		if err = m.auditCameraEviction(ctx, old); err != nil {
			return err
		}
		if legacy {
			job, loadErr := m.job(ctx, old)
			if loadErr != nil {
				return loadErr
			}
			err = m.removeLegacyCameraMedia(ctx, job)
		} else {
			err = m.removeCameraMedia(ctx, camera, old)
		}
		if err != nil {
			return err
		}
		used -= size
		if oldKind == "image" {
			count--
		}
	}
	_, err = m.db().ExecContext(ctx, `INSERT INTO camera_media(id,camera_id,owner_id,kind,state,reserved_bytes,created_at) VALUES(?,?,?,?,'pending',?,?)`, id, camera, owner, kind, reservation, m.now().Format(time.RFC3339Nano))
	return err
}

func (m *Manager) auditCameraEviction(ctx context.Context, id string) error {
	return m.options.Audit.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: audit.ActorKindSystem, Action: "camera.retention_delete_requested", SubjectType: "camera_media", SubjectID: id, Outcome: audit.OutcomeRequested})
}

func (m *Manager) removeCameraMedia(ctx context.Context, camera, id string) error {
	if m.cameraReads[id] > 0 {
		return ErrBusy
	}
	if !cameraIDPattern.MatchString(camera) {
		return ErrInvalid
	}
	if _, err := uuid.Parse(id); err != nil {
		return ErrInvalid
	}
	// The tombstone precedes unlinking, so a retry can finish interrupted cleanup.
	if _, err := m.db().ExecContext(ctx, `UPDATE camera_media SET state='deleting' WHERE id=?`, id); err != nil {
		return err
	}
	for _, suffix := range []string{".jpg", ".png", ".mp4", ".thumb.jpg", ".json", ".json.tmp"} {
		if err := os.Remove(filepath.Join(m.cameraRoot(camera), id+suffix)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_, err := m.db().ExecContext(ctx, `UPDATE camera_media SET state='deleted',reserved_bytes=0 WHERE id=?`, id)
	return err
}

func (m *Manager) CameraMediaList(ctx context.Context, actor identity.Actor, camera string, offset int, kinds ...string) ([]CameraMedia, error) {
	if !actor.Has(auth.PermissionBenchRead) {
		return nil, ErrForbidden
	}
	if !cameraIDPattern.MatchString(camera) || offset < 0 {
		return nil, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.reconcileCameraMedia(ctx, camera); err != nil {
		return nil, err
	}
	query := `SELECT id,camera_id,owner_id,kind,state,reserved_bytes,created_at,metadata_json,artifact_id FROM (` + cameraMediaRows + `) WHERE camera_id=? AND state NOT IN ('deleted','deleting')`
	args := []any{camera}
	if len(kinds) > 0 && kinds[0] != "" && kinds[0] != "all" {
		if kinds[0] != "image" && kinds[0] != "video" {
			return nil, ErrInvalid
		}
		query += ` AND kind=?`
		args = append(args, kinds[0])
	}
	if !actor.Has(auth.PermissionPluginsManage) {
		query += ` AND owner_id=?`
		args = append(args, actor.User.ID)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT 24 OFFSET ?`
	args = append(args, offset)
	rows, err := m.db().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CameraMedia{}
	for rows.Next() {
		var item CameraMedia
		var metadata string
		if err = rows.Scan(&item.ID, &item.CameraID, &item.OwnerID, &item.Kind, &item.State, &item.Bytes, &item.CreatedAt, &metadata, &item.artifactID); err != nil {
			return nil, err
		}
		item.Metadata = json.RawMessage(metadata)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (m *Manager) cameraMedia(ctx context.Context, actor identity.Actor, id string) (CameraMedia, error) {
	var item CameraMedia
	var raw string
	err := m.db().QueryRowContext(ctx, `SELECT id,camera_id,owner_id,kind,state,reserved_bytes,created_at,metadata_json,artifact_id FROM (`+cameraMediaRows+`) WHERE id=?`, id).Scan(&item.ID, &item.CameraID, &item.OwnerID, &item.Kind, &item.State, &item.Bytes, &item.CreatedAt, &raw, &item.artifactID)
	if err != nil {
		return item, normalizedError(err)
	}
	item.Metadata = json.RawMessage(raw)
	if item.OwnerID != actor.User.ID && !actor.Has(auth.PermissionPluginsManage) {
		return item, ErrForbidden
	}
	return item, nil
}

type cameraMediaFile struct {
	io.ReadSeeker
	closer  io.Closer
	manager *Manager
	id      string
	once    sync.Once
}

func (f *cameraMediaFile) Close() error {
	err := f.closer.Close()
	f.once.Do(func() {
		f.manager.mu.Lock()
		defer f.manager.mu.Unlock()
		f.manager.cameraReads[f.id]--
		if f.manager.cameraReads[f.id] == 0 {
			delete(f.manager.cameraReads, f.id)
		}
	})
	return err
}

// OpenCameraMedia leases the file until the response closes its descriptor.
func (m *Manager) OpenCameraMedia(ctx context.Context, actor identity.Actor, id string, thumbnail bool) (*cameraMediaFile, string, error) {
	if !actor.Has(auth.PermissionBenchRead) {
		return nil, "", ErrForbidden
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.cameraMedia(ctx, actor, id)
	if err != nil {
		return nil, "", err
	}
	if item.State != "completed" {
		return nil, "", ErrNotFound
	}
	if item.artifactID != "" {
		artifactID, err := domain.ParseArtifactID(item.artifactID)
		if err != nil {
			return nil, "", err
		}
		source, metadata, err := m.artifacts.Open(ctx, artifactID)
		if err != nil {
			return nil, "", err
		}
		if thumbnail {
			preview, err := boundedImagePreview(source, metadata.MediaType)
			source.Close()
			if err != nil {
				return nil, "", err
			}
			m.cameraReads[id]++
			reader := bytes.NewReader(preview)
			return &cameraMediaFile{ReadSeeker: reader, closer: io.NopCloser(reader), manager: m, id: id}, "image/jpeg", nil
		}
		reader, ok := source.(io.ReadSeeker)
		if !ok {
			source.Close()
			return nil, "", ErrUnavailable
		}
		m.cameraReads[id]++
		return &cameraMediaFile{ReadSeeker: reader, closer: source, manager: m, id: id}, metadata.MediaType, nil
	}
	var meta struct {
		Extension string `json:"extension"`
	}
	if json.Unmarshal(item.Metadata, &meta) != nil || !mediaExtension(meta.Extension) {
		return nil, "", ErrInvalid
	}
	suffix, media := "."+meta.Extension, "image/jpeg"
	if meta.Extension == "png" {
		media = "image/png"
	}
	if meta.Extension == "mp4" {
		media = "video/mp4"
	}
	if thumbnail {
		suffix = ".thumb.jpg"
		media = "image/jpeg"
	}
	filename := filepath.Join(m.cameraRoot(item.CameraID), item.ID+suffix)
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() {
		return nil, "", ErrNotFound
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, "", err
	}
	m.cameraReads[id]++
	return &cameraMediaFile{ReadSeeker: file, closer: file, manager: m, id: id}, media, nil
}

func (m *Manager) DeleteCameraMedia(ctx context.Context, actor identity.Actor, id string) error {
	if !actor.Has(auth.PermissionBenchExecute) {
		return ErrForbidden
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, err := m.cameraMedia(ctx, actor, id)
	if err != nil {
		return err
	}
	if item.State == "pending" {
		return ErrBusy
	}
	if err = m.record(ctx, actor, "camera.media_delete_requested", id, nil); err != nil {
		return err
	}
	if item.artifactID != "" {
		job, err := m.job(ctx, id)
		if err != nil {
			return err
		}
		return m.removeLegacyCameraMedia(ctx, job)
	}
	return m.removeCameraMedia(ctx, item.CameraID, id)
}

func (m *Manager) SetCameraRetention(ctx context.Context, actor identity.Actor, camera string, revision int64, limits CameraRetention) error {
	if !actor.Has(auth.PermissionPluginsManage) {
		return ErrForbidden
	}
	if !limits.valid() || !cameraIDPattern.MatchString(camera) {
		return ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cameraBusy(camera) {
		return ErrBusy
	}
	if _, err := cameraRecord(ctx, m, camera); err != nil {
		return err
	}
	current, _, _, err := m.cameraSettings(ctx, camera)
	if err != nil {
		return err
	}
	if current != revision {
		return ErrConflict
	}
	raw, _ := json.Marshal(limits)
	if err = m.record(ctx, actor, "camera.retention_configured", camera, limits); err != nil {
		return err
	}
	_, err = m.db().ExecContext(ctx, `UPDATE camera_workspace_settings SET retention_json=?,revision=revision+1 WHERE camera_id=?`, string(raw), camera)
	return err
}

var _ io.ReadSeeker = (*os.File)(nil)
