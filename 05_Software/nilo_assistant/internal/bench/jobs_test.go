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

type controlledRunner struct {
	mu      sync.Mutex
	calls   int
	active  int
	maximum int
	started chan struct{}
	release chan struct{}
}

func (r *controlledRunner) Describe(context.Context, string) (protocol.DescribeResult, error) {
	return protocol.DescribeResult{}, nil
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
	var params map[string]any
	_ = json.Unmarshal(request.Parameters, &params)
	data := []byte("synthetic integration evidence")
	if request.Operation == "capture" {
		path := filepath.Join(params["staging_directory"].(string), "capture.sr")
		if err := os.WriteFile(path, data, 0600); err != nil {
			return protocol.ExecuteResult{}, err
		}
		digest := sha256.Sum256(data)
		payload, _ := json.Marshal(protocol.ArtifactDeclaration{Type: "artifact", ID: "capture", MediaType: "application/octet-stream", SizeBytes: int64(len(data)), Digest: fmt.Sprintf("sha256:%x", digest)})
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
	for i := 0; i < QueueLimit; i++ {
		if _, err = m.Submit(ctx, a, submission(fmt.Sprintf("queued-%03d", i))); err != nil {
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
	if err != nil || len(jobs) != QueueLimit+1 {
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
