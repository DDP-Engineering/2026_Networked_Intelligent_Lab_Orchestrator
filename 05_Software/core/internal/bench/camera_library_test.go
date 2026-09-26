package bench

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// Seed a capture in the artifact layout used before workspace media existed.
func seedArtifactCameraCapture(t *testing.T, m *Manager, actor identity.Actor, runner *controlledRunner) (string, []byte) {
	t.Helper()
	job, err := m.Submit(t.Context(), actor, submission(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "running")
	runner.release <- struct{}{}
	job = awaitJob(t, m, actor, job.ID, "completed")
	if job.ArtifactID == "" {
		t.Fatal("missing fixture artifact")
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 32, 24)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.options.DataDir, "artifacts", job.SessionID, job.ArtifactID), encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.db().Exec(`UPDATE artifacts SET media_type='image/jpeg',size_bytes=? WHERE id=?`, encoded.Len(), job.ArtifactID); err != nil {
		t.Fatal(err)
	}
	job.Submission.Operation = "snapshot"
	job.Submission.PluginID = "camera"
	job.Submission.DeviceID = "cam-1"
	raw, _ := json.Marshal(job.Submission)
	if _, err := m.db().Exec(`UPDATE bench_jobs SET plugin_id='camera',submission_json=?,created_at='2020-01-01T00:00:00Z' WHERE id=?`, string(raw), job.ID); err != nil {
		t.Fatal(err)
	}
	return job.ID, encoded.Bytes()
}

func TestCameraLibraryCombinesStorageLayouts(t *testing.T) {
	m, admin, runner := testManager(t, "auto")
	old, original := seedArtifactCameraCapture(t, m, admin, runner)
	for range 24 {
		seedCameraMedia(t, m, admin.User.ID, "completed")
	}
	first, err := m.CameraMediaList(t.Context(), admin, "cam-1", 0, "image")
	if err != nil || len(first) != 24 {
		t.Fatalf("first page: %d %v", len(first), err)
	}
	second, err := m.CameraMediaList(t.Context(), admin, "cam-1", 24, "image")
	if err != nil || len(second) != 1 || second[0].ID != old {
		t.Fatalf("merged page: %+v %v", second, err)
	}
	videos, err := m.CameraMediaList(t.Context(), admin, "cam-1", 0, "video")
	if err != nil || len(videos) != 0 {
		t.Fatalf("video filter: %+v %v", videos, err)
	}
	other := identity.Actor{User: auth.User{ID: uuid.NewString()}, Permissions: []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}}
	hidden, err := m.CameraMediaList(t.Context(), other, "cam-1", 0)
	if err != nil || len(hidden) != 0 {
		t.Fatalf("ownership list: %+v %v", hidden, err)
	}
	if _, _, err = m.OpenCameraMedia(t.Context(), other, old, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ownership content: %v", err)
	}
	if err = m.DeleteCameraMedia(t.Context(), other, old); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ownership delete: %v", err)
	}
	for _, thumb := range []bool{false, true} {
		file, mime, err := m.OpenCameraMedia(t.Context(), admin, old, thumb)
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(file)
		if err != nil || mime != "image/jpeg" {
			t.Fatalf("content: %s %v", mime, err)
		}
		if !thumb && !bytes.Equal(content, original) {
			t.Fatal("export changed original")
		}
		if _, _, err = image.DecodeConfig(bytes.NewReader(content)); err != nil {
			t.Fatal(err)
		}
		if err = m.DeleteCameraMedia(t.Context(), admin, old); !errors.Is(err, ErrBusy) {
			t.Fatalf("active read: %v", err)
		}
		file.Close()
	}
	if err = m.DeleteCameraMedia(t.Context(), admin, old); err != nil {
		t.Fatal(err)
	}
	second, err = m.CameraMediaList(t.Context(), admin, "cam-1", 24)
	if err != nil || len(second) != 0 {
		t.Fatalf("deleted capture remains: %+v %v", second, err)
	}
}

func TestCameraRetentionAppliesToEveryCapture(t *testing.T) {
	m, admin, runner := testManager(t, "auto")
	old, _ := seedArtifactCameraCapture(t, m, admin, runner)
	current := seedCameraMedia(t, m, admin.User.ID, "completed")
	m.cameraSpace = func(string) (int64, int64, error) { return 100 << 30, 50 << 30, nil }
	limits := defaultCameraRetention()
	// Stored settings from the previous build must not preserve a second policy.
	if err := json.Unmarshal([]byte(`{"snapshots":2,"include_legacy":false}`), &limits); err != nil {
		t.Fatal(err)
	}
	file, _, err := m.OpenCameraMedia(t.Context(), admin, old, false)
	if err != nil {
		t.Fatal(err)
	}
	// A leased capture stays protected even under the unified policy.
	if err = m.reserveCameraMedia(t.Context(), "cam-1", admin.User.ID, uuid.NewString(), "image", 9<<20, limits); err != nil {
		t.Fatal(err)
	}
	retained, _ := m.cameraMedia(t.Context(), admin, old)
	evicted, _ := m.cameraMedia(t.Context(), admin, current)
	if retained.State != "completed" || evicted.State != "deleted" {
		t.Fatal("lease not respected")
	}
	file.Close()
	// Complete the reserved item, then the oldest artifact becomes eligible.
	if _, err = m.db().Exec(`UPDATE camera_media SET state='completed' WHERE state='pending'`); err != nil {
		t.Fatal(err)
	}
	if err = m.reserveCameraMedia(t.Context(), "cam-1", admin.User.ID, uuid.NewString(), "image", 9<<20, limits); err != nil {
		t.Fatal(err)
	}
	job, err := m.job(t.Context(), old)
	if err != nil || job.ArtifactID != "" {
		t.Fatalf("old capture not retained under same policy: %+v %v", job, err)
	}
}
