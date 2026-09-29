package bench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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
	streamStarted   chan struct{}
	ready           chan struct{}
	cts             atomic.Bool
	shutdownStarted chan struct{}
	shutdownRelease chan struct{}
	shutdownErr     error
	crash           chan struct{}
}

func (r *serialLifecycleRunner) Shutdown(ctx context.Context) error {
	if r.shutdownRelease == nil {
		return r.shutdownErr
	}
	select {
	case r.shutdownStarted <- struct{}{}:
	default:
	}
	select {
	case <-r.shutdownRelease:
		return r.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestSerialStoppedRemainsBusyUntilRunnerCleanup(t *testing.T) {
	m, actor, input, runner := serialLifecycleFixture(t, "auto")
	runner.shutdownStarted = make(chan struct{}, 1)
	runner.shutdownRelease = make(chan struct{})
	t.Cleanup(func() { close(runner.shutdownRelease) })
	close(runner.ready)
	job, err := m.ConnectSerial(t.Context(), actor, "serial-1", input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "completed")
	m.mu.Lock()
	m.stopSerialLocked("serial-1", "disconnected")
	m.mu.Unlock()
	select {
	case <-runner.shutdownStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not start")
	}
	m.mu.Lock()
	busy := m.serialBusy("serial-1")
	m.mu.Unlock()
	if !busy {
		t.Fatal("stopped Serial session released physical admission before runner cleanup")
	}
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
	select {
	case <-ctx.Done():
	case <-r.crash:
	}
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

func TestConfiguredSerialLifetimeIsCapturedWhenConnectionStarts(t *testing.T) {
	m, actor, input, runner := serialLifecycleFixture(t, "auto")
	state, err := m.OperatingSettings(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	settings := state.Settings
	settings.SerialLifetimeMinutes = 2
	settings.SerialIdleMinutes = 2
	state, err = m.ReplaceOperatingSettings(t.Context(), actor, state.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	close(runner.ready)
	job, err := m.ConnectSerial(t.Context(), actor, "serial-1", input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "completed")
	m.mu.Lock()
	connection := m.serials["serial-1"]
	expiry := connection.expires
	idle := connection.idleTimeout
	m.mu.Unlock()
	if idle != 2*time.Minute || time.Until(expiry) < time.Minute || time.Until(expiry) > 2*time.Minute {
		t.Fatalf("new connection lifetime = %v, idle = %v", time.Until(expiry), idle)
	}
	settings.SerialLifetimeMinutes = 1
	settings.SerialIdleMinutes = 1
	if _, err := m.ReplaceOperatingSettings(t.Context(), actor, state.Revision, settings); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if connection.expires != expiry || connection.idleTimeout != idle {
		t.Fatal("existing connection lifetime changed with platform settings")
	}
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

func TestSerialTerminalEventsKeepPhysicalClaimUntilCleanup(t *testing.T) {
	for _, trigger := range []string{"plugin_disable", "device_disable", "unplug", "expiry", "crash", "shutdown"} {
		t.Run(trigger, func(t *testing.T) {
			m, a, input, runner := serialLifecycleFixture(t, "auto")
			runner.shutdownStarted = make(chan struct{}, 1)
			runner.shutdownRelease = make(chan struct{})
			runner.crash = make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(runner.shutdownRelease) }) }
			t.Cleanup(release)
			close(runner.ready)
			job, err := m.ConnectSerial(t.Context(), a, "serial-1", input)
			if err != nil {
				t.Fatal(err)
			}
			awaitJob(t, m, a, job.ID, "completed")
			shutdownDone := make(chan error, 1)
			switch trigger {
			case "plugin_disable":
				if _, err = m.Lifecycle(t.Context(), a, "serial", "disable", ""); err != nil {
					t.Fatal(err)
				}
			case "device_disable":
				if _, err = m.SetDeviceEnabled(t.Context(), a, "serial", "serial-1", input.Revision, false); err != nil {
					t.Fatal(err)
				}
			case "unplug":
				m.mu.Lock()
				_, err = m.reconcileSerialDiscovery(t.Context(), json.RawMessage(`{"devices":[]}`))
				m.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
			case "expiry":
				m.mu.Lock()
				m.serials["serial-1"].expires = m.now().Add(-time.Second)
				m.mu.Unlock()
			case "crash":
				close(runner.crash)
			case "shutdown":
				go func() { shutdownDone <- m.Close() }()
			}
			select {
			case <-runner.shutdownStarted:
			case <-time.After(3 * time.Second):
				t.Fatal("cleanup did not start", trigger)
			}
			m.mu.Lock()
			session := m.serials["serial-1"]
			busy := session.claim.held && m.transportBusyLocked(session.claim.keys)
			m.mu.Unlock()
			if !busy {
				t.Fatal("terminal event released physical claim", trigger)
			}
			if trigger == "shutdown" {
				select {
				case <-shutdownDone:
					t.Fatal("Core shutdown returned before cleanup")
				default:
				}
			}
			release()
			awaitTransportDone(t, session.runnerDone)
			m.mu.Lock()
			busy = m.transportBusyLocked(session.claim.keys)
			m.mu.Unlock()
			if busy {
				t.Fatal("successful cleanup retained claims")
			}
			if trigger == "shutdown" {
				if err = <-shutdownDone; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestSerialFailedCleanupFencesAndStartupFailureReleases(t *testing.T) {
	t.Run("failed cleanup", func(t *testing.T) {
		m, a, input, runner := serialLifecycleFixture(t, "auto")
		failure := errors.New("cleanup unconfirmed")
		runner.shutdownErr = failure
		close(runner.ready)
		job, err := m.ConnectSerial(t.Context(), a, "serial-1", input)
		if err != nil {
			t.Fatal(err)
		}
		awaitJob(t, m, a, job.ID, "completed")
		m.mu.Lock()
		session := m.serials["serial-1"]
		m.stopSerialLocked("serial-1", "disconnected")
		m.mu.Unlock()
		awaitTransportDone(t, session.runnerDone)
		m.mu.Lock()
		busy := m.serialBusy("serial-1") && m.transportBusyLocked(session.claim.keys)
		m.mu.Unlock()
		if !busy {
			t.Fatal("failed cleanup freed Serial port")
		}
		if err = m.Close(); !errors.Is(err, failure) {
			t.Fatal("cleanup failure lost", err)
		}
	})
	t.Run("failed startup", func(t *testing.T) {
		m, a, input, _ := serialLifecycleFixture(t, "ask")
		job, err := m.ConnectSerial(t.Context(), a, "serial-1", input)
		if err != nil {
			t.Fatal(err)
		}
		awaitJob(t, m, a, job.ID, "pending_approval")
		plugin, err := m.loadPlugin(t.Context(), "serial")
		if err != nil {
			t.Fatal(err)
		}
		// Exercise the internal setup/cleanup seam after normal submission validation.
		// The fixture never approves or executes a physical operation.
		startupFailure := errors.New("startup refused")
		m.mu.Lock()
		m.options.runner = func(Plugin) (supervisor.Supervisor, error) { return nil, startupFailure }
		m.mu.Unlock()
		if _, err = m.executeSerialConnect(t.Context(), job, plugin); !errors.Is(err, startupFailure) {
			t.Fatal("setup failure not observed", err)
		}
		m.mu.Lock()
		claims := len(m.transports)
		m.mu.Unlock()
		if claims != 0 {
			t.Fatal("pre-launch startup failure retained physical claim")
		}
	})
}
