package bench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
)

// serialLifecycleRunner exercises the real Core approval/connection path over
// a private Unix socket. It is a test fixture and never opens physical hardware.
type serialLifecycleRunner struct {
	controlledRunner
	streamStarted chan struct{}
	ready         chan struct{}
	cts           atomic.Bool
}

func (r *serialLifecycleRunner) Execute(ctx context.Context, request protocol.ExecuteRequest, _ protocol.EventSink) (protocol.ExecuteResult, error) {
	if request.Operation != "stream" {
		return protocol.ExecuteResult{}, ErrInvalid
	}
	var input struct {
		Directory string `json:"workspace_directory"`
	}
	if json.Unmarshal(request.Parameters, &input) != nil || input.Directory == "" {
		return protocol.ExecuteResult{}, ErrInvalid
	}
	select {
	case r.streamStarted <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return protocol.ExecuteResult{}, ctx.Err()
	case <-r.ready:
	}
	listener, err := net.Listen("unix", filepath.Join(input.Directory, "control.sock"))
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"connected": true, "last_sequence": 0, "signals": map[string]any{"rts": true, "dtr": true, "cts": r.cts.Load(), "dsr": nil, "ri": nil, "cd": nil}, "warning": ""})
		case "/read":
			_ = json.NewEncoder(w).Encode(SerialRead{Events: []SerialEvent{}})
		default:
			http.NotFound(w, req)
		}
	})}
	finished := make(chan struct{})
	go func() { defer close(finished); _ = server.Serve(listener) }()
	<-ctx.Done()
	_ = server.Close()
	<-finished
	return protocol.ExecuteResult{}, ctx.Err()
}

func serialLifecycleFixture(t *testing.T, mode string) (*Manager, identity.Actor, SerialConnect, *serialLifecycleRunner) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("private Unix runner boundary is a Linux appliance contract")
	}
	m, actor, existing := serialFixture(t)
	m.mu.Lock()
	delete(m.serials, existing.device)
	m.mu.Unlock()
	// A short root keeps sockaddr_un below its platform path limit even when
	// the test's descriptive name makes testing.T.TempDir quite long.
	shortRoot, err := os.MkdirTemp("", "nilo-serial-")
	if err != nil {
		t.Fatal(err)
	}
	m.options.DataDir = shortRoot
	t.Cleanup(func() { _ = m.Close(); _ = os.RemoveAll(shortRoot) })
	source := filepath.Join("..", "..", "..", "integrations", "serial")
	target := filepath.Join(shortRoot, "integrations", "serial")
	files := map[string]string{}
	for _, name := range []string{"manifest.json", "runner", "schemas/plugin.schema.json", "schemas/device.schema.json", "schemas/discover_devices.input.schema.json", "schemas/connect.input.schema.json", "schemas/stream.input.schema.json", "schemas/result.schema.json", "schemas/event.schema.json"} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(target, name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, content, 0700); err != nil {
			t.Fatal(err)
		}
		files[name] = fmt.Sprintf("%x", sha256.Sum256(content))
	}
	index, _ := json.Marshal(files)
	if err = os.WriteFile(filepath.Join(target, "files.sha256.json"), index, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := verifyPackage(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.db().Exec(`UPDATE bench_plugins SET digest=?,mode=? WHERE id='serial'`, digest, mode); err != nil {
		t.Fatal(err)
	}
	runner := &serialLifecycleRunner{streamStarted: make(chan struct{}, 1), ready: make(chan struct{})}
	m.options.runner = func(Plugin) (supervisor.Supervisor, error) { return runner, nil }
	return m, actor, SerialConnect{Revision: existing.revision, Settings: existing.settings, IdempotencyKey: "lifecycle-connect", TimeoutSeconds: 10}, runner
}

func TestSerialLifecycleApprovalSignalsAndBackgroundRevocation(t *testing.T) {
	m, actor, input, runner := serialLifecycleFixture(t, "ask")
	close(runner.ready)
	job, err := m.ConnectSerial(t.Context(), actor, "serial-1", input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "pending_approval")
	select {
	case <-runner.streamStarted:
		t.Fatal("serial opened before human approval")
	default:
	}
	if _, err = m.Action(t.Context(), actor, job.ID, "approve"); err != nil {
		t.Fatal(err)
	}
	completed := awaitJob(t, m, actor, job.ID, "completed")
	var result struct {
		ConnectionID string `json:"connection_id"`
	}
	if json.Unmarshal(completed.Result, &result) != nil || result.ConnectionID == "" {
		t.Fatalf("missing durable connection result: %s", completed.Result)
	}
	waitSerialLifecycle(t, 2*time.Second, func() bool {
		status, e := m.SerialWorkspace(t.Context(), actor, "serial-1")
		return e == nil && status.Connected && status.ConnectionID == result.ConnectionID && status.Signals["cts"] != nil && !*status.Signals["cts"]
	}, "initial signal status")
	runner.cts.Store(true)
	waitSerialLifecycle(t, 2*time.Second, func() bool {
		status, e := m.SerialWorkspace(t.Context(), actor, "serial-1")
		return e == nil && status.Signals["cts"] != nil && *status.Signals["cts"]
	}, "changed hardware signal")
	if _, err = m.db().Exec(`UPDATE user_sessions SET revoked_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), actor.SessionID); err != nil {
		t.Fatal(err)
	}
	waitSerialLifecycle(t, 2*time.Second, func() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.serials["serial-1"].stopped }, "background revocation shutdown")
}

func TestSerialLifecycleSlowStartupAllowsOtherManagerRequests(t *testing.T) {
	m, actor, input, runner := serialLifecycleFixture(t, "auto")
	job, err := m.ConnectSerial(t.Context(), actor, "serial-1", input)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.streamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("stream startup was not attempted")
	}
	finished := make(chan error, 1)
	go func() { _, err := m.SerialWorkspace(t.Context(), actor, "serial-1"); finished <- err }()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		close(runner.ready)
		t.Fatal("slow startup held the global manager lock")
	}
	close(runner.ready)
	awaitJob(t, m, actor, job.ID, "completed")
}

func waitSerialLifecycle(t *testing.T, maximum time.Duration, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(maximum)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
