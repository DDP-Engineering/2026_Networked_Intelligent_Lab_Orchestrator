package bench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

func seedCameraMedia(t *testing.T, m *Manager, owner, state string) string {
	t.Helper()
	id := uuid.NewString()
	if err := os.MkdirAll(m.cameraRoot("cam-1"), 0700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("retained physical capture test bytes")
	for suffix, value := range map[string][]byte{".jpg": contents, ".thumb.jpg": []byte("thumbnail")} {
		if err := os.WriteFile(filepath.Join(m.cameraRoot("cam-1"), id+suffix), value, 0600); err != nil {
			t.Fatal(err)
		}
	}
	metadata, _ := json.Marshal(map[string]any{"id": id, "extension": "jpg", "bytes": len(contents), "sha256": fmt.Sprintf("%x", sha256.Sum256(contents))})
	if err := os.WriteFile(filepath.Join(m.cameraRoot("cam-1"), id+".json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.db().Exec(`INSERT INTO camera_media(id,camera_id,owner_id,kind,state,reserved_bytes,metadata_json,created_at) VALUES(?,'cam-1',?,'image',?,1048576,?,?)`, id, owner, state, string(metadata), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCameraMediaOwnershipAndActiveDownload(t *testing.T) {
	m, admin, _ := testManager(t, "auto")
	owner := identity.Actor{User: auth.User{ID: uuid.NewString()}, Permissions: []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}}
	other := owner
	other.User.ID = uuid.NewString()
	id := seedCameraMedia(t, m, owner.User.ID, "completed")
	if _, _, err := m.OpenCameraMedia(t.Context(), other, id, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-owner read: %v", err)
	}
	if err := m.DeleteCameraMedia(t.Context(), other, id); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-owner delete: %v", err)
	}
	file, _, err := m.OpenCameraMedia(t.Context(), owner, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteCameraMedia(t.Context(), admin, id); !errors.Is(err, ErrBusy) {
		t.Fatalf("delete active download: %v", err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteCameraMedia(t.Context(), owner, id); err != nil {
		t.Fatal(err)
	}
	if _, _, err = m.OpenCameraMedia(t.Context(), owner, id, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted media read: %v", err)
	}
	if err = m.DeleteCameraMedia(t.Context(), owner, id); err != nil {
		t.Fatalf("repeat delete: %v", err)
	}
}

func TestCameraRecoveryPublishesVerifiedMediaAndMarksUnfinished(t *testing.T) {
	m, actor, _ := testManager(t, "auto")
	complete := seedCameraMedia(t, m, actor.User.ID, "pending")
	partial := seedCameraMedia(t, m, actor.User.ID, "pending")
	if err := os.Remove(filepath.Join(m.cameraRoot("cam-1"), partial+".json")); err != nil {
		t.Fatal(err)
	}
	if err := m.recoverCameraMedia(t.Context()); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{complete: "completed", partial: "incomplete"} {
		item, err := m.cameraMedia(t.Context(), actor, id)
		if err != nil || item.State != want {
			t.Fatalf("recovery %s: %+v %v", id, item, err)
		}
	}
	used, err := m.cameraUsed(t.Context(), "cam-1")
	if err != nil || used <= 1048576 {
		t.Fatalf("partial reservation lost: %d %v", used, err)
	}
}

func TestCameraInternalPathsCannotBeSubmitted(t *testing.T) {
	for _, raw := range []string{`{"action":"stream"}`, `{"action":"start","workspace_directory":"/tmp/escape"}`, `{"action":"snapshot","id":"chosen"}`, `{"action":"settings"}`, `{"action":"controls","controls":{},"settings":{}}`} {
		if _, err := validateCameraCommand(json.RawMessage(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted unsafe command %s: %v", raw, err)
		}
	}
	if _, err := validateCameraCommand(json.RawMessage(`{"action":"snapshot","revision":2}`)); err != nil {
		t.Fatal(err)
	}
}

func TestCameraStopRequiresOwnerOrAdministrator(t *testing.T) {
	m, admin, _ := testManager(t, "auto")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	close(done)
	m.cameras["cam-1"] = &cameraSession{owner: admin.User.ID, cancel: cancel, done: done}
	other := identity.Actor{User: auth.User{ID: uuid.NewString()}, Permissions: []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}}
	if err := m.StopCamera(ctx, other, "cam-1", "stop"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stop ownership: %v", err)
	}
	// Remove the synthetic session so Manager.Close never attempts a transport.
	delete(m.cameras, "cam-1")
	if err := m.StopCamera(t.Context(), admin, "cam-1", "record_stop"); err != nil {
		t.Fatalf("idempotent stop: %v", err)
	}
}

func TestCameraRetentionBounds(t *testing.T) {
	good := defaultCameraRetention()
	if !good.valid() {
		t.Fatal("invalid defaults")
	}
	for _, change := range []func(*CameraRetention){func(r *CameraRetention) { r.Snapshots = 0 }, func(r *CameraRetention) { r.VideoBytes = r.TotalBytes }, func(r *CameraRetention) { r.VideoSeconds = 0 }, func(r *CameraRetention) { r.TotalBytes = 1 << 20 }} {
		value := good
		change(&value)
		if value.valid() {
			t.Fatalf("accepted unbounded limits: %+v", value)
		}
	}
}

func TestCameraStopAllowsGracefulRunnerCompletion(t *testing.T) {
	m, actor, _ := testManager(t, "auto")
	done := make(chan struct{})
	cancelled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/command" {
			t.Errorf("unexpected stop request: %s %s", r.Method, r.URL.Path)
		}
		w.Write([]byte(`{"state":"stopped"}`))
		close(done)
	}))
	defer server.Close()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	m.cameras["cam-1"] = &cameraSession{owner: actor.User.ID, done: done, cancel: func() { cancelled = true }, client: &http.Client{Transport: transport}}
	if err := m.StopCamera(t.Context(), actor, "cam-1", "stop"); err != nil {
		t.Fatal(err)
	}
	if cancelled {
		t.Fatal("successful stop cancelled the runner instead of allowing its final reply")
	}
	if _, found := m.cameras["cam-1"]; found {
		t.Fatal("completed session retained the device")
	}
}

func TestCameraAdmissionReservesDiskAndEvictsOnlyEligibleCompletedMedia(t *testing.T) {
	m, actor, _ := testManager(t, "auto")
	m.cameraSpace = func(string) (int64, int64, error) { return 100 << 30, 50 << 30, nil }
	old := seedCameraMedia(t, m, actor.User.ID, "completed")
	limits := defaultCameraRetention()
	limits.Snapshots = 1
	if err := m.reserveCameraMedia(t.Context(), "cam-1", actor.User.ID, uuid.NewString(), "image", 9<<20, limits); err != nil {
		t.Fatal(err)
	}
	item, err := m.cameraMedia(t.Context(), actor, old)
	if err != nil || item.State != "deleted" {
		t.Fatalf("oldest image not evicted: %+v %v", item, err)
	}
	retained := seedCameraMedia(t, m, actor.User.ID, "completed")
	if err = m.reserveCameraMedia(t.Context(), "cam-1", actor.User.ID, uuid.NewString(), "image", 9<<20, limits); !errors.Is(err, errCameraStorage) {
		t.Fatalf("pending image not protected: %v", err)
	}
	item, _ = m.cameraMedia(t.Context(), actor, retained)
	if item.State != "completed" {
		t.Fatal("failed admission deleted media unnecessarily")
	}
	m.cameraSpace = func(string) (int64, int64, error) { return 100 << 30, 10 << 30, nil }
	limits.Snapshots = 100
	if err = m.reserveCameraMedia(t.Context(), "cam-2", actor.User.ID, uuid.NewString(), "video", 500<<20, limits); !errors.Is(err, errCameraStorage) {
		t.Fatalf("disk reserve bypassed: %v", err)
	}
}

func TestCameraRecoveryQuarantinesCorruptCaptureWithoutBlockingAppliance(t *testing.T) {
	m, actor, _ := testManager(t, "auto")
	id := seedCameraMedia(t, m, actor.User.ID, "pending")
	if err := os.WriteFile(filepath.Join(m.cameraRoot("cam-1"), id+".jpg"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.recoverCameraMedia(t.Context()); err != nil {
		t.Fatal(err)
	}
	item, err := m.cameraMedia(t.Context(), actor, id)
	if err != nil || item.State != "incomplete" {
		t.Fatalf("corrupt capture published: %+v %v", item, err)
	}
}

func TestCameraRecoveryPreservesReservationWhenThumbnailIsMissing(t *testing.T) {
	m, actor, _ := testManager(t, "auto")
	id := seedCameraMedia(t, m, actor.User.ID, "pending")
	if err := os.Remove(filepath.Join(m.cameraRoot("cam-1"), id+".thumb.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := m.recoverCameraMedia(t.Context()); err != nil {
		t.Fatal(err)
	}
	item, err := m.cameraMedia(t.Context(), actor, id)
	if err != nil || item.State != "incomplete" {
		t.Fatalf("missing thumbnail published: %+v %v", item, err)
	}
	used, err := m.cameraUsed(t.Context(), "cam-1")
	if err != nil || used < 1 {
		t.Fatalf("reservation lost: %d %v", used, err)
	}
}
