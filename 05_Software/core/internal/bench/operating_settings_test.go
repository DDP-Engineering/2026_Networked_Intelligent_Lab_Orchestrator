package bench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nilo.local/nilo/internal/store/sqlite"
)

type blockingCameraTransport struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (transport *blockingCameraTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.once.Do(func() { close(transport.started) })
	<-transport.release
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
}

func TestOperatingSettingsReplacementPersistsAndRejectsStaleOrInvalidValues(t *testing.T) {
	manager, actor, _ := testManager(t, "auto")
	ctx := context.Background()
	initial, err := manager.OperatingSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Revision != 1 || initial.Settings.ExecutingJobs != 2 || initial.Settings.OutstandingJobs != 32 || initial.Settings.SerialConnections != 4 || initial.Settings.CameraStreams != 2 {
		t.Fatalf("initial settings = %+v", initial)
	}
	changed := initial.Settings
	changed.ExecutingJobs = 3
	changed.SerialConnections = 5
	updated, err := manager.ReplaceOperatingSettings(ctx, actor, initial.Revision, changed)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("replace = %+v, %v", updated, err)
	}
	if _, err := manager.ReplaceOperatingSettings(ctx, actor, initial.Revision, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision = %v", err)
	}
	invalid := changed
	invalid.DefaultJobTimeoutSeconds = invalid.MaxJobTimeoutSeconds + 1
	if _, err := manager.ReplaceOperatingSettings(ctx, actor, updated.Revision, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid range = %v", err)
	}
	stored, err := manager.OperatingSettings(ctx, actor)
	if err != nil || stored.Revision != 2 || stored.Settings != changed {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	var revision int64
	if err := manager.db().QueryRowContext(ctx, `SELECT revision FROM operating_settings WHERE id=1`).Scan(&revision); err != nil || revision != 2 {
		t.Fatalf("persisted revision = %d, %v", revision, err)
	}
	reopened, err := sqlite.Open(ctx, filepath.Join(manager.options.DataDir, "bench.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reloaded := &Manager{options: Options{Database: reopened}}
	if err := reloaded.loadOperatingSettings(ctx); err != nil || reloaded.settingsRevision != 2 || reloaded.settings != changed {
		t.Fatalf("manager settings after database reopen = %+v revision %d, %v", reloaded.settings, reloaded.settingsRevision, err)
	}
	var executing, serial int
	if err := reopened.SQL().QueryRowContext(ctx, `SELECT executing_jobs,serial_connections FROM operating_settings WHERE id=1`).Scan(&executing, &serial); err != nil || executing != 3 || serial != 5 {
		t.Fatalf("settings after database reopen = jobs %d, serial %d, %v", executing, serial, err)
	}
	if _, err := manager.db().ExecContext(ctx, `CREATE TRIGGER reject_operating_audit BEFORE INSERT ON audit_events WHEN NEW.action='operating_settings.changed' BEGIN SELECT RAISE(ABORT, 'audit blocked'); END`); err != nil {
		t.Fatal(err)
	}
	changed.ExecutingJobs = 4
	if _, err := manager.ReplaceOperatingSettings(ctx, actor, 2, changed); err == nil {
		t.Fatal("settings update succeeded without its audit event")
	}
	state, err := manager.OperatingSettings(ctx, actor)
	if err != nil || state.Revision != 2 || state.Settings.ExecutingJobs != 3 {
		t.Fatalf("audit failure changed settings: %+v, %v", state, err)
	}
}

func TestConfiguredJobCapacitiesRaiseAndRelease(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	ctx := context.Background()
	state, err := manager.OperatingSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	settings := state.Settings
	settings.ExecutingJobs = 1
	settings.OutstandingJobs = 2
	state, err = manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Submit(ctx, actor, submission("limit-first"))
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, manager, actor, first.ID, "running")
	secondInput := submission("limit-second")
	secondInput.DeviceID = "atk-dl16-1.4"
	second, err := manager.Submit(ctx, actor, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Submit(ctx, actor, submission("limit-third")); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("outstanding cap = %v", err)
	}
	settings.ExecutingJobs = 2
	state, err = manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, manager, actor, second.ID, "running")
	settings.ExecutingJobs = 1
	if _, err = manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings); err != nil {
		t.Fatal(err)
	}
	if current, err := manager.GetJob(ctx, actor, first.ID); err != nil || current.State != "running" {
		t.Fatalf("lowered cap stopped active job: %+v, %v", current, err)
	}
	runner.release <- struct{}{}
	runner.release <- struct{}{}
	awaitJob(t, manager, actor, first.ID, "completed")
	awaitJob(t, manager, actor, second.ID, "completed")
	if _, err := manager.Submit(ctx, actor, submission("limit-after-release")); err != nil {
		t.Fatalf("capacity not released: %v", err)
	}
}

func TestConfiguredJobTimeoutAppliesOnlyToNewSubmissions(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	ctx := context.Background()
	state, err := manager.OperatingSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	settings := state.Settings
	settings.DefaultJobTimeoutSeconds = 120
	settings.MaxJobTimeoutSeconds = 120
	state, err = manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	input := submission("timeout-first")
	input.TimeoutSeconds = 0
	first, err := manager.Submit(ctx, actor, input)
	if err != nil || first.Submission.TimeoutSeconds != 120 {
		t.Fatalf("configured default = %+v, %v", first, err)
	}
	settings.DefaultJobTimeoutSeconds = 20
	settings.MaxJobTimeoutSeconds = 20
	if _, err = manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings); err != nil {
		t.Fatal(err)
	}
	secondInput := submission("timeout-second")
	secondInput.DeviceID = "atk-dl16-1.4"
	secondInput.TimeoutSeconds = 0
	second, err := manager.Submit(ctx, actor, secondInput)
	if err != nil || second.Submission.TimeoutSeconds != 20 {
		t.Fatalf("new default = %+v, %v", second, err)
	}
	if first.Deadline.Sub(first.CreatedAt) != 120*time.Second || second.Deadline.Sub(second.CreatedAt) != 20*time.Second {
		t.Fatalf("existing and new deadlines = %v, %v", first.Deadline.Sub(first.CreatedAt), second.Deadline.Sub(second.CreatedAt))
	}
	tooLong := submission("timeout-invalid")
	tooLong.TimeoutSeconds = 21
	if _, err := manager.Submit(ctx, actor, tooLong); !errors.Is(err, ErrInvalid) {
		t.Fatalf("over maximum = %v", err)
	}
	runner.release <- struct{}{}
	runner.release <- struct{}{}
}

func TestConfiguredTimeoutKeepsIdempotentRetriesStable(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	ctx := context.Background()
	state, err := manager.OperatingSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	settings := state.Settings
	settings.DefaultJobTimeoutSeconds = 120
	settings.MaxJobTimeoutSeconds = 120
	state, err = manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	input := submission("timeout-retry")
	input.TimeoutSeconds = 0
	first, err := manager.Submit(ctx, actor, input)
	if err != nil {
		t.Fatal(err)
	}
	settings.DefaultJobTimeoutSeconds = 20
	settings.MaxJobTimeoutSeconds = 20
	if _, err := manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings); err != nil {
		t.Fatal(err)
	}
	explicit := input
	explicit.TimeoutSeconds = 120
	for _, retry := range []Submission{input, explicit} {
		returned, err := manager.Submit(ctx, actor, retry)
		if err != nil || returned.ID != first.ID {
			t.Fatalf("idempotent retry changed after timeout setting: %+v, %v", returned, err)
		}
	}
	runner.release <- struct{}{}
}

func TestCameraControlWaitDoesNotBlockOperatingSettingsRead(t *testing.T) {
	manager, actor, serial := serialFixture(t)
	transport := &blockingCameraTransport{started: make(chan struct{}), release: make(chan struct{})}
	manager.mu.Lock()
	manager.cameras["cam-1"] = &cameraSession{owner: actor.User.ID, done: make(chan struct{}), cancel: func() {}, client: &http.Client{Transport: transport}}
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		delete(manager.cameras, "cam-1")
		manager.mu.Unlock()
	}()
	finished := make(chan error, 1)
	go func() { finished <- manager.StopCamera(context.Background(), actor, "cam-1", "record_stop") }()
	select {
	case <-transport.started:
	case <-time.After(2 * time.Second):
		t.Fatal("camera control did not reach the runner")
	}
	read := make(chan error, 1)
	go func() {
		_, err := manager.OperatingSettings(context.Background(), actor)
		if err == nil {
			_, err = manager.SerialWorkspace(context.Background(), actor, "serial-1")
		}
		if err == nil {
			event := SerialEvent{Sequence: 1, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Direction: "rx", DataBase64: "AA==", ByteCount: 1}
			page, marshalErr := json.Marshal(SerialRead{Events: []SerialEvent{event}, NextCursor: 1})
			if marshalErr != nil {
				err = marshalErr
			} else {
				manager.mu.Lock()
				err = manager.ingestSerial(serial, page)
				manager.mu.Unlock()
			}
		}
		if err == nil {
			var page SerialRead
			page, err = manager.ReadSerial(context.Background(), actor, serial.device, serial.id, 0, 0)
			if err == nil && len(page.Events) != 1 {
				err = errors.New("serial event was not ingested")
			}
		}
		if err == nil {
			err = manager.DisconnectSerial(context.Background(), actor, serial.device, serial.id)
		}
		read <- err
	}()
	select {
	case err := <-read:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(transport.release)
		<-finished
		<-read
		t.Fatal("camera runner wait blocked unrelated Manager access")
	}
	close(transport.release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredSerialCapacityPreservesAndReleasesExistingConnection(t *testing.T) {
	manager, actor, session := serialFixture(t)
	ctx := context.Background()
	state, err := manager.OperatingSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	settings := state.Settings
	settings.SerialConnections = 1
	state, err = manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings)
	if err != nil || state.Usage.SerialConnections != 1 {
		t.Fatalf("lowered serial cap = %+v, %v", state, err)
	}
	if session.stopped {
		t.Fatal("lowering serial capacity closed an active connection")
	}
	job := Job{Submission: Submission{DeviceID: "serial-2"}}
	if _, err := manager.executeSerialConnect(ctx, job, Plugin{}); !errors.Is(err, ErrBusy) {
		t.Fatalf("over-capacity serial connect = %v", err)
	}
	manager.mu.Lock()
	manager.stopSerialLocked(session.device, "disconnected")
	manager.mu.Unlock()
	state, err = manager.OperatingSettings(ctx, actor)
	if err != nil || state.Usage.SerialConnections != 0 {
		t.Fatalf("serial capacity after disconnect = %+v, %v", state, err)
	}
}

func TestConfiguredCameraCapacityCountsActiveStreamsSeparately(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	ctx := context.Background()
	copyCameraBundle(t, manager.options.BundleDir)
	if _, err := manager.Lifecycle(ctx, actor, "camera", "install", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Lifecycle(ctx, actor, "camera", "enable", "auto"); err != nil {
		t.Fatal(err)
	}
	devices, err := manager.reconcileCameraDiscovery(ctx, json.RawMessage(`{"devices":[{"id":"cam-2","vendor":"0458","product":"708f","serial":"","port_path":"1-1.4","endpoints":[{"id":"video0","capture":true,"formats":["MJPG"]}],"selected_endpoint":"video0"}]}`))
	if err != nil || len(devices) != 1 {
		t.Fatalf("camera discovery = %+v, %v", devices, err)
	}
	runner.release <- struct{}{}
	configured, err := manager.ConfigureDevice(ctx, actor, "camera", devices[0].ID, devices[0].Revision, "Bench Camera", "", json.RawMessage(`{}`))
	if err != nil || configured.Readiness != "ready" {
		t.Fatalf("configured camera = %+v, %v", configured, err)
	}
	plugin, err := manager.loadPlugin(ctx, "camera")
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.cameras["cam-99"] = &cameraSession{owner: actor.User.ID, done: make(chan struct{}), cancel: func() {}}
	manager.serials["serial-1"] = &serialSession{device: "serial-1"}
	manager.serials["serial-2"] = &serialSession{device: "serial-2"}
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		delete(manager.cameras, "cam-99")
		delete(manager.serials, "serial-1")
		delete(manager.serials, "serial-2")
		manager.mu.Unlock()
	}()
	state, err := manager.OperatingSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	settings := state.Settings
	settings.CameraStreams = 1
	settings.SerialConnections = 2
	state, err = manager.ReplaceOperatingSettings(ctx, actor, state.Revision, settings)
	if err != nil || state.Usage.CameraStreams != 1 || state.Usage.SerialConnections != 2 {
		t.Fatalf("separate stream occupancy = %+v, %v", state, err)
	}
	job := Job{OwnerID: actor.User.ID, Submission: Submission{DeviceID: configured.ID, Parameters: json.RawMessage(`{"action":"start"}`)}}
	if _, err := manager.executeCameraCommand(ctx, job, plugin); !errors.Is(err, ErrBusy) {
		t.Fatalf("over-capacity camera start = %v", err)
	}
}
