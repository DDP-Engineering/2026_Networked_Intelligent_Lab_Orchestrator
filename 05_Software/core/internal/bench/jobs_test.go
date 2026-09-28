package bench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/store/sqlite"
)

const defaultOutstandingJobs = 32

type controlledRunner struct {
	mu           sync.Mutex
	calls        int
	active       int
	maximum      int
	describeErr  error
	runErr       error
	mediaType    string
	artifactID   string
	artifactFile string
	started      chan struct{}
	release      chan struct{}
}

func TestArtifactMediaTypeMustMatchManifest(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	runner.mediaType = "image/jpeg"
	job, err := manager.Submit(context.Background(), actor, submission("wrong-media-type"))
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, manager, actor, job.ID, "running")
	runner.release <- struct{}{}
	finished := awaitJob(t, manager, actor, job.ID, "failed")
	if finished.ArtifactID != "" {
		t.Fatalf("artifact %q retained despite undeclared media type", finished.ArtifactID)
	}
}

func TestTerminalJobPreservesAllowlistedRemoteCode(t *testing.T) {
	for _, test := range []struct {
		code string
		want string
	}{
		{code: "device_busy", want: "device_busy"},
		{code: "device_not_found", want: "device_not_found"},
		{code: "image_too_large", want: "image_too_large"},
		{code: "capture_failed", want: "capture_failed"},
		{code: "vendor_private", want: "execution_failed"},
	} {
		t.Run(test.code, func(t *testing.T) {
			manager, actor, runner := testManager(t, "auto")
			runner.runErr = &supervisor.RunnerError{Kind: supervisor.ErrRunnerRemote, Cause: &supervisor.RemoteExecutionError{Code: test.code, Message: "private detail"}}
			job, err := manager.Submit(context.Background(), actor, submission("typed-error-test"))
			if err != nil {
				t.Fatal(err)
			}
			awaitJob(t, manager, actor, job.ID, "running")
			runner.release <- struct{}{}
			deadline := time.Now().Add(4 * time.Second)
			for time.Now().Before(deadline) {
				current, err := manager.GetJob(context.Background(), actor, job.ID)
				if err != nil {
					t.Fatal(err)
				}
				if current.State == "failed" && current.Error != "" {
					if current.Error != test.want {
						t.Fatalf("terminal error = %q, want %q", current.Error, test.want)
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("terminal job did not retain an error")
		})
	}
}

func (r *controlledRunner) Describe(context.Context, string) (protocol.DescribeResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return protocol.DescribeResult{}, r.describeErr
}

func rewriteCatalogVersion(t *testing.T, manager *Manager, version string) {
	t.Helper()
	root := filepath.Join(manager.options.BundleDir, "sigrok")
	manifestPath := filepath.Join(root, "manifest.json")
	var descriptor map[string]any
	encoded, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &descriptor); err != nil {
		t.Fatal(err)
	}
	packageDescriptor := descriptor["package"].(map[string]any)
	packageDescriptor["version"] = version
	encoded, err = json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o700); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, "files.sha256.json")
	var files map[string]string
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(index, &files); err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = fmt.Sprintf("%x", sha256.Sum256(encoded))
	index, err = json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		t.Fatal(err)
	}
}

func rewriteCatalogStateSchema(t *testing.T, manager *Manager, schema int, migrations []map[string]any) {
	t.Helper()
	root := filepath.Join(manager.options.BundleDir, "sigrok")
	manifestPath := filepath.Join(root, "manifest.json")
	var descriptor map[string]any
	encoded, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &descriptor); err != nil {
		t.Fatal(err)
	}
	descriptor["state_schema"] = schema
	descriptor["state_migrations"] = migrations
	encoded, err = json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o700); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, "files.sha256.json")
	var files map[string]string
	index, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(index, &files); err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = fmt.Sprintf("%x", sha256.Sum256(encoded))
	index, err = json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationUpgradePreservesStateRefusesBusyAndRollsBackFailedHealth(t *testing.T) {
	manager, actor, runner := testManager(t, "auto")
	rewriteCatalogVersion(t, manager, "2.0.0")
	upgraded, err := manager.Lifecycle(context.Background(), actor, "sigrok", "upgrade", "")
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.Version != "2.0.0" || !upgraded.Enabled || upgraded.Mode != "auto" {
		t.Fatalf("upgraded plugin = %+v", upgraded)
	}

	job, err := manager.Submit(context.Background(), actor, submission("upgrade-busy"))
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, manager, actor, job.ID, "running")
	busyContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := manager.Lifecycle(busyContext, actor, "sigrok", "upgrade", ""); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy upgrade = %v", err)
	}
	runner.release <- struct{}{}
	awaitJob(t, manager, actor, job.ID, "completed")

	rewriteCatalogVersion(t, manager, "3.0.0")
	runner.mu.Lock()
	runner.describeErr = errors.New("new runtime health failure")
	runner.mu.Unlock()
	if _, err := manager.Lifecycle(context.Background(), actor, "sigrok", "upgrade", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("failed-health upgrade = %v", err)
	}
	runner.mu.Lock()
	runner.describeErr = nil
	runner.mu.Unlock()
	plugins, err := manager.ListPlugins(context.Background(), actor)
	if err != nil || len(plugins) != 1 || plugins[0].Version != "2.0.0" || !plugins[0].Enabled {
		t.Fatalf("rolled-back plugin = %+v, %v", plugins, err)
	}

	rewriteCatalogVersion(t, manager, "4.0.0")
	rewriteCatalogStateSchema(t, manager, 2, nil)
	if _, err := manager.Lifecycle(context.Background(), actor, "sigrok", "upgrade", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing state migration upgrade = %v", err)
	}
	var stateSchema int
	var devicesJSON string
	if err := manager.db().QueryRow(`SELECT state_schema,devices_json FROM bench_plugins WHERE id='sigrok'`).Scan(&stateSchema, &devicesJSON); err != nil {
		t.Fatal(err)
	}
	if stateSchema != 1 || !strings.Contains(devicesJSON, "atk-dl16-1.3") {
		t.Fatalf("rolled-back integration state schema=%d devices=%s", stateSchema, devicesJSON)
	}
	rows, err := manager.db().Query(`SELECT action FROM audit_events WHERE subject_id='sigrok' AND action LIKE 'plugin.upgrade_%' ORDER BY sequence`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actions := []string{}
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
	}
	if !slices.Contains(actions, "plugin.upgrade_succeeded") || !slices.Contains(actions, "plugin.upgrade_rolled_back") {
		t.Fatalf("upgrade audit actions = %v", actions)
	}
}

func TestIntegrationUpgradeRecoversCandidateActivationInterruptedBeforeStateCommit(t *testing.T) {
	manager, actor, _ := testManager(t, "ask")
	before, err := manager.ListPlugins(context.Background(), actor)
	if err != nil || len(before) != 1 {
		t.Fatalf("load prior plugin = %+v, %v", before, err)
	}
	priorVersion := before[0].Version
	target, err := manager.packageRoot("sigrok")
	if err != nil {
		t.Fatal(err)
	}
	var priorDigest string
	if err := manager.db().QueryRow(`SELECT digest FROM bench_plugins WHERE id='sigrok'`).Scan(&priorDigest); err != nil {
		t.Fatal(err)
	}
	rewriteCatalogVersion(t, manager, "2.0.0")
	integrationRoot := filepath.Join(manager.options.DataDir, "integrations")
	stageName := ".upgrade-sigrok-candidate"
	backupName := ".upgrade-sigrok-backup"
	stage := filepath.Join(integrationRoot, stageName)
	backup := filepath.Join(integrationRoot, backupName)
	if err := os.CopyFS(stage, os.DirFS(filepath.Join(manager.options.BundleDir, "sigrok"))); err != nil {
		t.Fatal(err)
	}
	candidateDigest, err := verifyPackage(stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(backup, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(target, filepath.Join(backup, "previous")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, target); err != nil {
		t.Fatal(err)
	}
	journal := map[string]any{
		"backup": backupName, "candidate_digest": candidateDigest, "id": "sigrok",
		"phase": "candidate_active", "prior_digest": priorDigest, "schema": 1,
		"stage": stageName, "target": "sigrok",
	}
	encoded, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(integrationRoot, ".upgrade-sigrok.json")
	if err := os.WriteFile(journalPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	recovered, err := New(manager.options)
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	plugins, err := recovered.ListPlugins(context.Background(), actor)
	if err != nil || len(plugins) != 1 || plugins[0].Version != priorVersion || plugins[0].Digest != priorDigest {
		t.Fatalf("recovered plugin = %+v, %v", plugins, err)
	}
	for _, path := range []string{journalPath, backup, stage} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("recovery residue remains at %s: %v", path, err)
		}
	}
}
func (r *controlledRunner) Cancel(context.Context, string, domain.RequestID) error { return nil }
func (r *controlledRunner) Shutdown(context.Context) error                         { return nil }
func (r *controlledRunner) Execute(ctx context.Context, request protocol.ExecuteRequest, sink protocol.EventSink) (protocol.ExecuteResult, error) {
	r.mu.Lock()
	r.calls++
	r.active++
	if r.active > r.maximum {
		r.maximum = r.active
	}
	r.mu.Unlock()
	defer func() { r.mu.Lock(); r.active--; r.mu.Unlock() }()
	r.started <- struct{}{}
	select {
	case <-ctx.Done():
		return protocol.ExecuteResult{}, ctx.Err()
	case <-r.release:
	}
	if r.runErr != nil {
		return protocol.ExecuteResult{}, r.runErr
	}
	var params map[string]any
	_ = json.Unmarshal(request.Parameters, &params)
	data := []byte("synthetic integration evidence")
	if request.Operation == "capture" {
		name := r.artifactFile
		if name == "" {
			name = "capture.sr"
		}
		path := filepath.Join(params["staging_directory"].(string), name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			return protocol.ExecuteResult{}, err
		}
		digest := sha256.Sum256(data)
		mediaType := r.mediaType
		if mediaType == "" {
			mediaType = "application/x-sigrok-session"
		}
		id := r.artifactID
		if id == "" {
			id = "capture"
		}
		payload, _ := json.Marshal(protocol.ArtifactDeclaration{Type: "artifact", ID: id, MediaType: mediaType, SizeBytes: int64(len(data)), Digest: fmt.Sprintf("sha256:%x", digest)})
		if err := sink.Emit(ctx, protocol.Envelope{Kind: protocol.Event, Method: protocol.Execute, Payload: payload}); err != nil {
			return protocol.ExecuteResult{}, err
		}
	}
	return protocol.ExecuteResult{Type: "result", Status: "completed", Data: json.RawMessage(`{"simulation":true}`)}, nil
}
func testManager(t *testing.T, mode string) (*Manager, identity.Actor, *controlledRunner) {
	t.Helper()
	root := t.TempDir()
	bundle := filepath.Join(root, "bundles", "sigrok")
	if err := os.MkdirAll(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join("..", "..", "..", "integrations", "sigrok")
	files := map[string]string{}
	for _, name := range []string{"manifest.json", "runner", "schemas/discover.input.schema.json", "schemas/capture.input.schema.json", "schemas/decode.input.schema.json", "schemas/result.schema.json", "schemas/event.schema.json"} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "manifest.json" {
			var descriptor map[string]any
			_ = json.Unmarshal(content, &descriptor)
			descriptor["platforms"] = []map[string]string{{"os": runtime.GOOS, "architecture": runtime.GOARCH}}
			content, _ = json.Marshal(descriptor)
		}
		target := filepath.Join(bundle, name)
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(target, content, 0700); err != nil {
			t.Fatal(err)
		}
		files[name] = fmt.Sprintf("%x", sha256.Sum256(content))
	}
	runtimeFiles := map[string][]byte{
		"runtime/bin/python3":    []byte("#!/bin/sh\nexit 0\n"),
		"runtime/bin/sigrok-cli": []byte("#!/bin/sh\nexit 0\n"),
	}
	var runtimeBytes int64
	for name, content := range runtimeFiles {
		target := filepath.Join(bundle, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, 0o700); err != nil {
			t.Fatal(err)
		}
		runtimeBytes += int64(len(content))
		files[name] = fmt.Sprintf("%x", sha256.Sum256(content))
	}
	runtimeDescriptor, _ := json.Marshal(map[string]any{
		"architecture": runtime.GOARCH, "backend": "sigrok-cli",
		"measured_bytes": runtimeBytes, "maximum_bytes": runtimeBytes * 2,
		"project_version": "test", "self_contained": true,
	})
	if err := os.WriteFile(filepath.Join(bundle, "runtime.json"), runtimeDescriptor, 0o600); err != nil {
		t.Fatal(err)
	}
	files["runtime.json"] = fmt.Sprintf("%x", sha256.Sum256(runtimeDescriptor))
	index, _ := json.Marshal(files)
	if err := os.WriteFile(filepath.Join(bundle, "files.sha256.json"), index, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(context.Background(), filepath.Join(root, "bench.db"))
	if err != nil {
		t.Fatal(err)
	}
	writer, err := audit.NewWriter(audit.WriterOptions{Events: sqlite.NewAuditRepository(db), ApplianceID: uuid.NewString(), BootID: uuid.NewString(), Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	runner := &controlledRunner{started: make(chan struct{}, 100), release: make(chan struct{}, 100)}
	manager, err := New(Options{Database: db, DataDir: root, BundleDir: filepath.Dir(bundle), Backend: filepath.Join(root, "sigrok-cli"), Simulation: true, Audit: writer, runner: func(Plugin) (supervisor.Supervisor, error) { return runner, nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Close(); db.Close() })
	actor := identity.Actor{User: auth.User{ID: uuid.NewString(), Role: auth.RoleAdministrator}, Credential: identity.CredentialSession, Permissions: auth.RoleAdministrator.Permissions()}
	if _, err = manager.Lifecycle(context.Background(), actor, "sigrok", "install", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Lifecycle(context.Background(), actor, "sigrok", "enable", mode); err != nil {
		t.Fatal(err)
	}
	devices, _ := json.Marshal([]Device{{ID: "atk-dl16-1.3", Connection: "1.3"}, {ID: "atk-dl16-1.4", Connection: "1.4"}})
	if _, err = db.SQL().Exec(`UPDATE bench_plugins SET devices_json=? WHERE id='sigrok'`, string(devices)); err != nil {
		t.Fatal(err)
	}
	return manager, actor, runner
}
func submission(key string) Submission {
	return Submission{PluginID: "sigrok", Operation: "capture", DeviceID: "atk-dl16-1.3", Parameters: json.RawMessage(`{"samples":1024,"samplerate":1000000,"channels":["D0"]}`), IdempotencyKey: key, TimeoutSeconds: 10}
}
func awaitJob(t *testing.T, m *Manager, a identity.Actor, id, state string) Job {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		job, err := m.GetJob(context.Background(), a, id)
		if err != nil {
			t.Fatal(err)
		}
		if job.State == state {
			return job
		}
		if terminal(job.State) {
			t.Fatalf("job ended %s (%s), want %s", job.State, job.Error, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s", id, state)
	return Job{}
}
func TestQueueCorrelatesRetriesAndSerializesOneDevice(t *testing.T) {
	m, a, r := testManager(t, "auto")
	ctx := context.Background()
	first, err := m.Submit(ctx, a, submission("command-one"))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := m.Submit(ctx, a, submission("command-one"))
	if err != nil || duplicate.ID != first.ID {
		t.Fatalf("retry: %+v %v", duplicate, err)
	}
	changed := submission("command-one")
	changed.TimeoutSeconds = 11
	if _, err = m.Submit(ctx, a, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry: %v", err)
	}
	second, err := m.Submit(ctx, a, submission("command-two"))
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, a, first.ID, "running")
	pending, err := m.GetJob(ctx, a, second.ID)
	if err != nil || pending.State != "queued" {
		t.Fatalf("second started: %+v %v", pending, err)
	}
	r.release <- struct{}{}
	completed := awaitJob(t, m, a, first.ID, "completed")
	if completed.ArtifactID == "" || len(completed.Replies) != 1 || completed.Replies[0].TransactionID != first.ID || completed.Replies[0].Sequence != 1 {
		t.Fatalf("uncorrelated result: %+v", completed)
	}
	r.release <- struct{}{}
	awaitJob(t, m, a, second.ID, "completed")
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls != 2 || r.maximum != 1 {
		t.Fatalf("calls %d maximum %d", r.calls, r.maximum)
	}
}
func TestIndependentDevicesRunConcurrentlyAndCancelCleansUp(t *testing.T) {
	m, a, r := testManager(t, "auto")
	ctx := context.Background()
	one, err := m.Submit(ctx, a, submission("parallel-one"))
	if err != nil {
		t.Fatal(err)
	}
	input := submission("parallel-two")
	input.DeviceID = "atk-dl16-1.4"
	two, err := m.Submit(ctx, a, input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, a, one.ID, "running")
	awaitJob(t, m, a, two.ID, "running")
	if _, err = m.Lifecycle(ctx, a, "sigrok", "uninstall", ""); !errors.Is(err, ErrBusy) {
		t.Fatalf("busy uninstall: %v", err)
	}
	if _, err = m.Action(ctx, a, one.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, a, one.ID, "cancelled")
	r.release <- struct{}{}
	awaitJob(t, m, a, two.ID, "completed")
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.maximum != 2 || r.active != 0 {
		t.Fatalf("maximum %d active %d", r.maximum, r.active)
	}
}
func TestApprovalDeadlineQueueBoundAndDisable(t *testing.T) {
	m, a, _ := testManager(t, "ask")
	ctx := context.Background()
	first := submission("approval-one")
	first.TimeoutSeconds = 1
	pending, err := m.Submit(ctx, a, first)
	if err != nil {
		t.Fatal(err)
	}
	token := a
	token.Credential = identity.CredentialToken
	if _, err = m.Action(ctx, token, pending.ID, "approve"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("token approved: %v", err)
	}
	awaitJob(t, m, a, pending.ID, "incomplete")
	for i := 0; i < defaultOutstandingJobs; i++ {
		queued := submission(fmt.Sprintf("queued-%03d", i))
		queued.TimeoutSeconds = 300
		if _, err = m.Submit(ctx, a, queued); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = m.Submit(ctx, a, submission("overflow-command")); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("overflow: %v", err)
	}
	if _, err = m.Lifecycle(ctx, a, "sigrok", "disable", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Submit(ctx, a, submission("disabled-command")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled: %v", err)
	}
	if _, err = m.Lifecycle(ctx, a, "sigrok", "uninstall", ""); err != nil {
		t.Fatal(err)
	}
	jobs, err := m.ListJobs(ctx, a)
	if err != nil || len(jobs) != defaultOutstandingJobs+1 {
		t.Fatalf("history: %d %v", len(jobs), err)
	}
}
func TestRejectPWMAndUnexposedParameters(t *testing.T) {
	for _, raw := range []string{`{"channels":["PWM0"],"samples":1024,"samplerate":1000000}`, `{"channels":["D0"],"samples":1024,"samplerate":1000000,"conn":"1.3"}`, `{"channels":["D0","D0"],"samples":1024,"samplerate":1000000}`} {
		value := submission("bad-input")
		value.Parameters = json.RawMessage(raw)
		if _, err := canonicalSubmission(value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %s: %v", raw, err)
		}
	}
}

func TestRunningDeadlineReleasesDeviceAndRepliesCannotChangeTerminalJob(t *testing.T) {
	m, a, r := testManager(t, "auto")
	input := submission("running-timeout")
	input.TimeoutSeconds = 1
	job, err := m.Submit(context.Background(), a, input)
	if err != nil {
		t.Fatal(err)
	}
	ended := awaitJob(t, m, a, job.ID, "incomplete")
	// Terminal state may be visible one scheduling turn before its error detail.
	deadline := time.Now().Add(time.Second)
	for ended.Error == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		ended, _ = m.GetJob(context.Background(), a, job.ID)
	}
	if ended.Error != "deadline_exceeded" {
		t.Fatalf("timeout result: %+v", ended)
	}
	stale := &jobRunner{manager: m, job: job}
	if err = stale.reply(context.Background(), protocol.Envelope{Payload: json.RawMessage(`{"type":"progress","fraction":1}`)}); !errors.Is(err, ErrConflict) {
		t.Fatalf("late reply: %v", err)
	}
	next, err := m.Submit(context.Background(), a, submission("after-timeout"))
	if err != nil {
		t.Fatal(err)
	}
	r.release <- struct{}{}
	awaitJob(t, m, a, next.ID, "completed")
	var leases int
	if err = m.db().QueryRow(`SELECT count(*) FROM resource_leases WHERE released_at IS NULL`).Scan(&leases); err != nil || leases != 0 {
		t.Fatalf("leases %d: %v", leases, err)
	}
}

func TestRestartDoesNotReplayAndRetainsDisabledUninstalledChoices(t *testing.T) {
	m, a, r := testManager(t, "ask")
	ctx := context.Background()
	pending, err := m.Submit(ctx, a, submission("before-restart"))
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	resumed, err := New(m.options)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	ended, err := resumed.GetJob(ctx, a, pending.ID)
	if err != nil || ended.State != "incomplete" {
		t.Fatalf("recovery: %+v %v", ended, err)
	}
	if _, err = resumed.Lifecycle(ctx, a, "sigrok", "disable", ""); err != nil {
		t.Fatal(err)
	}
	if err = resumed.Close(); err != nil {
		t.Fatal(err)
	}
	disabled, err := New(m.options)
	if err != nil {
		t.Fatal(err)
	}
	defer disabled.Close()
	plugins, err := disabled.ListPlugins(ctx, a)
	if err != nil || plugins[0].Enabled {
		t.Fatalf("disabled persistence: %+v %v", plugins, err)
	}
	if _, err = disabled.Lifecycle(ctx, a, "sigrok", "uninstall", ""); err != nil {
		t.Fatal(err)
	}
	if err = disabled.Close(); err != nil {
		t.Fatal(err)
	}
	absent, err := New(m.options)
	if err != nil {
		t.Fatal(err)
	}
	defer absent.Close()
	plugins, err = absent.ListPlugins(ctx, a)
	if err != nil || plugins[0].Installed {
		t.Fatalf("uninstall persistence: %+v %v", plugins, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls != 0 {
		t.Fatal("restart replayed work")
	}
}

func TestTamperedPackageAndOtherOwnersAreRefused(t *testing.T) {
	m, a, r := testManager(t, "auto")
	ctx := context.Background()
	job, err := m.Submit(ctx, a, submission("private-evidence"))
	if err != nil {
		t.Fatal(err)
	}
	r.release <- struct{}{}
	awaitJob(t, m, a, job.ID, "completed")
	other := a
	other.User.ID = uuid.NewString()
	other.Permissions = auth.RoleOperator.Permissions()
	if _, err = m.GetJob(ctx, other, job.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other owner read: %v", err)
	}
	if _, _, err = m.OpenArtifact(ctx, other, job.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other owner artifact: %v", err)
	}
	root, _ := m.packageRoot("sigrok")
	if err = os.WriteFile(filepath.Join(root, "runner"), []byte("tampered"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Submit(ctx, a, submission("tampered-command")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("tampered package: %v", err)
	}
}
