package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
)

type cameraSession struct {
	owner   string
	expires time.Time
	cancel  context.CancelFunc
	client  *http.Client
	done    chan struct{}
}

// CameraCommand is a policy-approved finite command controlling a dedicated stream.
// Paths, owner identities and media IDs are always injected by Core.
type CameraCommand struct {
	Action   string          `json:"action"`
	Revision int64           `json:"revision,omitempty"`
	Settings json.RawMessage `json:"settings,omitempty"`
	Controls json.RawMessage `json:"controls,omitempty"`
}

func validateCameraCommand(raw json.RawMessage) (CameraCommand, error) {
	var command CameraCommand
	if len(raw) > 12000 || strict(raw, &command) != nil {
		return command, ErrInvalid
	}
	switch command.Action {
	case "start", "stop", "snapshot", "record_start", "record_stop", "read_controls", "reset_controls":
		if len(command.Settings) > 0 || len(command.Controls) > 0 {
			return command, ErrInvalid
		}
	case "settings":
		if len(command.Settings) == 0 || len(command.Controls) > 0 {
			return command, ErrInvalid
		}
	case "controls":
		if len(command.Controls) == 0 || len(command.Settings) > 0 {
			return command, ErrInvalid
		}
	default:
		return command, ErrInvalid
	}
	return command, nil
}

func (m *Manager) cameraRoot(id string) string {
	return filepath.Join(m.options.DataDir, "camera-workspaces", id)
}

func cameraCall(ctx context.Context, session *cameraSession, method, path string, body any) ([]byte, error) {
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://camera"+path, input)
	if err != nil {
		return nil, err
	}
	response, err := session.client.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	value, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(value) > 2<<20 {
		return nil, ErrUnavailable
	}
	if response.StatusCode != 200 {
		var failure struct {
			Code string `json:"error"`
		}
		if json.Unmarshal(value, &failure) == nil {
			switch failure.Code {
			case "recording_active", "unsupported_format", "invalid_settings", "frame_unavailable", "encoding_failed", "image_too_large":
				return nil, cameraOperationError(failure.Code)
			}
		}
		return nil, ErrInvalid
	}
	return value, nil
}

func (m *Manager) cameraGate(id string) *sync.Mutex {
	gate, _ := m.cameraGates.LoadOrStore(id, &sync.Mutex{})
	return gate.(*sync.Mutex)
}

// cameraCallUnlocked releases the shared admission lock during runner I/O.
// The caller must hold Manager.mu and the camera's gate on both sides.
func (m *Manager) cameraCallUnlocked(ctx context.Context, session *cameraSession, method, path string, body any) ([]byte, error) {
	m.mu.Unlock()
	result, err := cameraCall(ctx, session, method, path, body)
	m.mu.Lock()
	return result, err
}

func (m *Manager) waitCameraReadyUnlocked(ctx context.Context, session *cameraSession) error {
	m.mu.Unlock()
	defer m.mu.Lock()
	ready := time.NewTicker(100 * time.Millisecond)
	defer ready.Stop()
	for {
		if _, err := cameraCall(ctx, session, "GET", "/status", nil); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-session.done:
			return ErrUnavailable
		case <-ready.C:
		}
	}
}

func (m *Manager) cameraSettings(ctx context.Context, id string) (int64, json.RawMessage, CameraRetention, error) {
	_, err := m.db().ExecContext(ctx, `INSERT OR IGNORE INTO camera_workspace_settings(camera_id) VALUES(?)`, id)
	if err != nil {
		return 0, nil, CameraRetention{}, err
	}
	var revision int64
	var settings, retention string
	err = m.db().QueryRowContext(ctx, `SELECT revision,settings_json,retention_json FROM camera_workspace_settings WHERE camera_id=?`, id).Scan(&revision, &settings, &retention)
	limits := defaultCameraRetention()
	if err == nil {
		err = json.Unmarshal([]byte(retention), &limits)
	}
	return revision, json.RawMessage(settings), limits, err
}

// Execute is reached only after the ordinary Bench approval and admission path.
func (m *Manager) executeCameraCommand(ctx context.Context, job Job, p Plugin) (protocol.ExecuteResult, error) {
	command, err := validateCameraCommand(job.Submission.Parameters)
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	id := job.Submission.DeviceID
	gate := m.cameraGate(id)
	gate.Lock()
	defer gate.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return protocol.ExecuteResult{}, ErrUnavailable
	}
	record, err := cameraRecord(ctx, m, id)
	if err != nil || !record.Enabled || !record.Configured || record.Readiness != "ready" {
		return protocol.ExecuteResult{}, ErrUnavailable
	}
	revision, settings, limits, err := m.cameraSettings(ctx, id)
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	if command.Action != "stop" && command.Action != "record_stop" && command.Action != "read_controls" && command.Revision != revision {
		return protocol.ExecuteResult{}, ErrConflict
	}
	session := m.cameras[id]
	if session != nil {
		select {
		case <-session.done:
			delete(m.cameras, id)
			session = nil
		default:
		}
	}
	if session != nil && session.owner != job.OwnerID {
		return protocol.ExecuteResult{}, ErrBusy
	}
	if command.Action == "start" && session == nil {
		activeStreams := 0
		for id := range m.cameras {
			if m.cameraBusy(id) {
				activeStreams++
			}
		}
		if activeStreams >= m.settings.CameraStreams {
			return protocol.ExecuteResult{}, ErrBusy
		}
		runner, err := m.processConfigured(p, id, 12*time.Second)
		if err != nil {
			return protocol.ExecuteResult{}, err
		}
		root := m.cameraRoot(id)
		if err = os.MkdirAll(root, 0700); err != nil {
			return protocol.ExecuteResult{}, err
		}
		lifetime := time.Duration(m.settings.CameraLifetimeMinutes) * time.Minute
		runCtx, cancel := context.WithTimeout(context.Background(), lifetime+time.Minute)
		socket := filepath.Join(root, "control.sock")
		transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		}, MaxConnsPerHost: 2}
		session = &cameraSession{owner: job.OwnerID, expires: m.now().Add(lifetime), cancel: cancel, client: &http.Client{Transport: transport, Timeout: 12 * time.Second}, done: make(chan struct{})}
		m.cameras[id] = session
		parameters, _ := json.Marshal(map[string]any{"workspace_directory": root, "hardware_identity": map[string]string{"vendor": record.Vendor, "product": record.Product, "serial": record.Serial, "port_path": record.PortPath}, "endpoint_id": record.EndpointID, "device_id": id, "display_name": record.DisplayName, "settings": settings, "session_lifetime_seconds": int(lifetime.Seconds())})
		m.cameraWorkers.Add(1)
		go func(s *cameraSession) {
			defer m.cameraWorkers.Done()
			defer cancel()
			defer transport.CloseIdleConnections()
			_, runErr := runner.Execute(runCtx, protocol.ExecuteRequest{Type: "execute", Operation: "stream", Parameters: parameters}, discardEvents{})
			shutdown, finish := context.WithTimeout(context.Background(), 3*time.Second)
			defer finish()
			_ = runner.Shutdown(shutdown)
			close(s.done)
			if runErr != nil && runCtx.Err() == nil {
				m.report(runErr)
				m.mu.Lock()
				if m.cameras[id] == s {
					final, finish := context.WithTimeout(context.Background(), 2*time.Second)
					m.markHardwareFailed(final, "camera", id)
					finish()
				}
				m.mu.Unlock()
			}
		}(session)
		if err := m.waitCameraReadyUnlocked(ctx, session); err != nil {
			cancel()
			return protocol.ExecuteResult{}, err
		}
	}
	if session == nil {
		if command.Action == "stop" || command.Action == "record_stop" {
			return protocol.ExecuteResult{Status: "completed", Data: json.RawMessage(`{"state":"stopped"}`)}, nil
		}
		return protocol.ExecuteResult{}, ErrUnavailable
	}
	if err = m.reconcileCameraMedia(ctx, id); err != nil {
		return protocol.ExecuteResult{}, err
	}
	var result []byte
	switch command.Action {
	case "start":
		result, err = m.cameraCallUnlocked(ctx, session, "GET", "/status", nil)
	case "settings":
		if command.Revision != revision {
			return protocol.ExecuteResult{}, ErrConflict
		}
		result, err = m.cameraCallUnlocked(ctx, session, "POST", "/command", command)
		if err == nil {
			_, err = m.db().ExecContext(ctx, `UPDATE camera_workspace_settings SET revision=revision+1,settings_json=? WHERE camera_id=? AND revision=?`, string(command.Settings), id, revision)
			if err != nil {
				// A running stream must not use settings that were not persisted.
				session.cancel()
			}
		}
	case "snapshot", "record_start":
		if command.Revision != revision {
			return protocol.ExecuteResult{}, ErrConflict
		}
		if command.Action == "record_start" {
			raw, readErr := m.cameraCallUnlocked(ctx, session, "GET", "/status", nil)
			if readErr != nil {
				return protocol.ExecuteResult{}, readErr
			}
			var current struct {
				Recording *json.RawMessage `json:"recording"`
			}
			if json.Unmarshal(raw, &current) != nil {
				return protocol.ExecuteResult{}, ErrInvalid
			}
			if current.Recording != nil {
				return protocol.ExecuteResult{}, ErrBusy
			}
			limits.VideoSeconds = min(limits.VideoSeconds, int(time.Until(session.expires).Seconds())-5)
			if limits.VideoSeconds < 1 {
				return protocol.ExecuteResult{}, ErrUnavailable
			}
		}
		kind, reservation := "image", int64(9<<20)
		if command.Action == "record_start" {
			kind = "video"
			reservation = limits.VideoBytes + (2 << 20)
		}
		if err = m.reserveCameraMedia(ctx, id, job.OwnerID, job.ID, kind, reservation, limits); err != nil {
			return protocol.ExecuteResult{}, err
		}
		result, err = m.cameraCallUnlocked(ctx, session, "POST", "/command", map[string]any{"action": command.Action, "id": job.ID, "max_bytes": limits.VideoBytes, "max_seconds": limits.VideoSeconds})
		if err != nil {
			_, _ = m.db().ExecContext(ctx, `UPDATE camera_media SET state='incomplete' WHERE id=?`, job.ID)
		}
	default:
		result, err = m.cameraCallUnlocked(ctx, session, "POST", "/command", command)
		if err == nil && (command.Action == "controls" || command.Action == "reset_controls") {
			_, err = m.db().ExecContext(ctx, `UPDATE camera_workspace_settings SET revision=revision+1 WHERE camera_id=?`, id)
		}
		if command.Action == "stop" {
			if err != nil {
				session.cancel()
			}
			m.mu.Unlock()
			select {
			case <-session.done:
				m.mu.Lock()
				delete(m.cameras, id)
			case <-ctx.Done():
				m.mu.Lock()
				session.cancel()
				return protocol.ExecuteResult{}, ctx.Err()
			}
		}
	}
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	if err = m.reconcileCameraMedia(ctx, id); err != nil {
		return protocol.ExecuteResult{}, err
	}
	return protocol.ExecuteResult{Status: "completed", Data: result}, nil
}

// CameraWorkspace reports the caller's live session and saved settings.
func (m *Manager) CameraWorkspace(ctx context.Context, actor identity.Actor, id string) (map[string]any, error) {
	if !actor.Has(auth.PermissionBenchRead) || !cameraIDPattern.MatchString(id) {
		return nil, ErrForbidden
	}
	gate := m.cameraGate(id)
	gate.Lock()
	defer gate.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := cameraRecord(ctx, m, id); err != nil {
		return nil, err
	}
	if err := m.reconcileCameraMedia(ctx, id); err != nil {
		return nil, err
	}
	revision, settings, limits, err := m.cameraSettings(ctx, id)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"state": "stopped", "settings": settings, "revision": revision, "retention": limits, "video_encodings": []string{"mp4-h264"}}
	session := m.cameras[id]
	if session != nil {
		select {
		case <-session.done:
			delete(m.cameras, id)
			result["state"] = "stopped"
		default:
			if session.owner != actor.User.ID {
				result["state"] = "busy"
			} else {
				raw, callErr := m.cameraCallUnlocked(ctx, session, "GET", "/status", nil)
				if callErr == nil {
					var live map[string]any
					if json.Unmarshal(raw, &live) == nil {
						if live["error"] == "capture_failed" {
							m.markHardwareFailed(ctx, "camera", id)
						}
						for key, value := range live {
							result[key] = value
						}
					}
				} else {
					result["state"] = "failed"
				}
				result["revision"] = revision
				result["expires_at"] = session.expires
			}
		}
	}
	used, err := m.cameraUsed(ctx, id)
	if err != nil {
		return nil, err
	}
	result["used_bytes"] = used
	if actor.Has(auth.PermissionPluginsManage) {
		var count int
		var eligibleBytes int64
		if err = m.db().QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(reserved_bytes),0) FROM (`+cameraMediaRows+`) WHERE camera_id=? AND state='completed'`, id).Scan(&count, &eligibleBytes); err != nil {
			return nil, err
		}
		result["retention_inventory"] = map[string]any{"count": count, "bytes": eligibleBytes}
	}
	return result, nil
}

// CameraFrame returns the latest processed frame; fetching never starts a camera.
func (m *Manager) CameraFrame(ctx context.Context, actor identity.Actor, id string) ([]byte, error) {
	if !actor.Has(auth.PermissionBenchRead) {
		return nil, ErrForbidden
	}
	m.mu.Lock()
	session := m.cameras[id]
	m.mu.Unlock()
	if session == nil {
		return nil, ErrNotFound
	}
	if session.owner != actor.User.ID {
		return nil, ErrForbidden
	}
	return cameraCall(ctx, session, "GET", "/frame", nil)
}

func (m *Manager) cameraBusy(id string) bool {
	for device, session := range m.cameras {
		select {
		case <-session.done:
			continue
		default:
			if id == "camera" || id == device {
				return true
			}
		}
	}
	return false
}

var errCameraStorage = errors.New("camera_storage_limit")

type cameraOperationError string

func (e cameraOperationError) Error() string { return string(e) }

// StopCamera cancels existing work without requesting approval for a new action.
func (m *Manager) StopCamera(ctx context.Context, actor identity.Actor, id, action string) error {
	if !actor.Has(auth.PermissionBenchExecute) {
		return ErrForbidden
	}
	if !cameraIDPattern.MatchString(id) || action != "stop" && action != "record_stop" {
		return ErrInvalid
	}
	gate := m.cameraGate(id)
	gate.Lock()
	defer gate.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.cameras[id]
	if session == nil {
		return nil
	}
	if session.owner != actor.User.ID && !actor.Has(auth.PermissionPluginsManage) {
		return ErrForbidden
	}
	select {
	case <-session.done:
		delete(m.cameras, id)
		return nil
	default:
	}
	if err := m.record(ctx, actor, "camera."+action+"_requested", id, nil); err != nil {
		return err
	}
	_, err := m.cameraCallUnlocked(ctx, session, "POST", "/command", map[string]string{"action": action})
	if action == "stop" {
		// A successful stop command lets Execute finish and flush its reply.
		// Cancelling it here races that reply and incurs the supervisor grace wait.
		if err != nil {
			session.cancel()
		}
		m.mu.Unlock()
		select {
		case <-session.done:
			m.mu.Lock()
			delete(m.cameras, id)
		case <-ctx.Done():
			m.mu.Lock()
			session.cancel()
			return ctx.Err()
		}
	}
	if err != nil {
		return err
	}
	return m.reconcileCameraMedia(ctx, id)
}
