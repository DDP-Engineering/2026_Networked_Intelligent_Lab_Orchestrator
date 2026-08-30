package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/protocol"
)

func TestSupervisorExecutesOnlyAfterHandshake(t *testing.T) {
	supervisor := newTestSupervisor(t, "success")
	result, err := supervisor.Execute(context.Background(), executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("Execute() status = %q, want completed", result.Status)
	}
}

func TestSupervisorDescribesTheConfiguredPackage(t *testing.T) {
	supervisor := newTestSupervisor(t, "describe")
	result, err := supervisor.Describe(context.Background(), "test.integration")
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("Describe() status = %q, want ok", result.Status)
	}
}

func TestSupervisorContainsInvalidRunnerOutput(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want error
	}{
		{name: "wrong protocol", mode: "wrong-protocol", want: protocol.ErrUnsupportedProtocol},
		{name: "malformed stdout", mode: "malformed", want: protocol.ErrInvalidMessage},
		{name: "one MiB overflow", mode: "overflow", want: protocol.ErrLineTooLong},
		{name: "exit before result", mode: "exit", want: ErrRunnerProtocol},
		{name: "mismatched execute event", mode: "mismatched-event", want: ErrRunnerProtocol},
		{name: "structured remote error", mode: "remote-error", want: ErrRunnerRemote},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newTestSupervisor(t, test.mode).Execute(context.Background(), executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want %v", err, test.want)
			}
			if test.mode == "remote-error" && errors.Is(err, ErrRunnerProtocol) {
				t.Fatalf("remote terminal error = %v, must not be classified as protocol failure", err)
			}
		})
	}
}

func TestSupervisorSanitizesControlCharactersInStderr(t *testing.T) {
	_, err := newTestSupervisor(t, "stderr-control").Execute(context.Background(), executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
	if err == nil {
		t.Fatal("Execute() error = nil, want runner crash")
	}
	if diagnostics := Diagnostics(err); strings.ContainsAny(diagnostics, "\x00\r\x1b") {
		t.Fatalf("Diagnostics() = %q, contains unsanitized control characters", diagnostics)
	}
}

func TestSupervisorKeepsOnlyLast64KiBOfStderr(t *testing.T) {
	_, err := newTestSupervisor(t, "stderr").Execute(context.Background(), executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
	if err == nil {
		t.Fatal("Execute() error = nil, want runner failure")
	}
	diagnostics := Diagnostics(err)
	if len(diagnostics) != maxDiagnosticBytes {
		t.Fatalf("diagnostics length = %d, want %d", len(diagnostics), maxDiagnosticBytes)
	}
	if !strings.HasSuffix(diagnostics, "END") {
		t.Fatalf("diagnostics = %q, want final stderr bytes", diagnostics[len(diagnostics)-32:])
	}
}

func TestSupervisorCancelTargetsActiveExecuteWireID(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "cancel-marker")
	supervisor := newTestSupervisor(t, "cooperative-cancel", marker)
	executeContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		_, err := supervisor.Execute(executeContext, executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
		completed <- err
	}()
	wireID := waitMarker(t, marker, func(value string) bool { return len(strings.TrimSpace(value)) == 36 })
	requestID, err := domain.ParseRequestID(strings.TrimSpace(wireID))
	if err != nil {
		t.Fatalf("ParseRequestID(%q) error = %v", wireID, err)
	}
	if err := supervisor.Cancel(context.Background(), "test.integration", requestID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	waitMarker(t, marker, func(value string) bool {
		return strings.Contains(value, "correct-cancel") && strings.Contains(value, "safe_shutdown")
	})
	cancel()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("Execute() did not finish after cancellation")
	}
}

func TestSupervisorShutdownTracksDescribeRunnerAndDoesNotBlockOnUnreadStdin(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "shutdown-marker")
	supervisor := newTestSupervisor(t, "describe-hang", marker)
	describeContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		_, err := supervisor.Describe(describeContext, "test.integration")
		completed <- err
	}()
	waitMarker(t, marker, func(value string) bool { return strings.Contains(value, "describe-ready") })
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer shutdownCancel()
	if err := supervisor.Shutdown(shutdownContext); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	waitMarker(t, marker, func(value string) bool { return strings.Contains(value, "safe_shutdown") })
	cancel()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("Describe() did not finish after Shutdown()")
	}
}

func TestSupervisorDeadlineIsNotDefeatedByBlockedEventSink(t *testing.T) {
	supervisor := newTestSupervisor(t, "event-hang")
	deadlineContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		_, err := supervisor.Execute(deadlineContext, executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error {
			select {}
		}))
		completed <- err
	}()
	select {
	case err := <-completed:
		if !errors.Is(err, ErrRunnerTimeout) {
			t.Fatalf("Execute() error = %v, want %v", err, ErrRunnerTimeout)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked event sink defeated Execute deadline")
	}
}

func TestSupervisorCancellationRaceUsesOnlyTheExecuteWireID(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "race-marker")
	supervisor := newTestSupervisor(t, "cooperative-cancel", marker)
	executionContext, cancelExecution := context.WithCancel(context.Background())
	defer cancelExecution()
	completed := make(chan error, 1)
	go func() {
		_, err := supervisor.Execute(executionContext, executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
		completed <- err
	}()
	wireID := waitMarker(t, marker, func(value string) bool { return len(strings.TrimSpace(value)) == 36 })
	requestID, err := domain.ParseRequestID(strings.TrimSpace(wireID))
	if err != nil {
		t.Fatal(err)
	}
	finishedCancel := make(chan error, 1)
	go func() { finishedCancel <- supervisor.Cancel(context.Background(), "test.integration", requestID) }()
	cancelExecution()
	select {
	case err := <-finishedCancel:
		if err != nil {
			t.Fatalf("Cancel() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent Cancel() did not finish")
	}
	waitMarker(t, marker, func(value string) bool { return strings.Contains(value, "correct-cancel") })
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("Execute() did not finish after cancellation race")
	}
}

func TestNewProcessSupervisorCopiesLaunchConfiguration(t *testing.T) {
	root := t.TempDir()
	binary := copyTestHelper(t, root)
	configuration := ProcessConfig{
		PackageID:   "test.integration",
		PackageRoot: root,
		Runner: manifest.Runner{
			Executable: filepath.Base(binary),
			Arguments:  []string{"success"},
		},
		Environment: map[string]string{"tz": "UTC"},
		GracePeriod: 25 * time.Millisecond,
	}
	supervisor, err := NewProcessSupervisor(configuration)
	if err != nil {
		t.Fatal(err)
	}
	configuration.Runner.Arguments[0] = "hang"
	configuration.Environment["tz"] = "Europe/Bucharest"
	deadlineContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := supervisor.Execute(deadlineContext, executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
	if err != nil {
		t.Fatalf("Execute() after caller config mutation error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("Execute() status = %q, want completed", result.Status)
	}
}

func TestCopiedHelperIsExecutableOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows executable permissions are not POSIX mode bits")
	}
	binary := copyTestHelper(t, t.TempDir())
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("copied helper mode = %o, want an executable bit", info.Mode().Perm())
	}
}

func TestSupervisorTimeoutAndCancellationTerminateRunner(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		supervisor := newTestSupervisor(t, "hang")
		deadlineContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := supervisor.Execute(deadlineContext, executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
		if !errors.Is(err, ErrRunnerTimeout) {
			t.Fatalf("Execute() error = %v, want %v", err, ErrRunnerTimeout)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		supervisor := newTestSupervisor(t, "hang")
		cancellationContext, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		_, err := supervisor.Execute(cancellationContext, executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error = %v, want context cancellation", err)
		}
	})
}

func TestSupervisorSerializesConcurrentProtocolWrites(t *testing.T) {
	supervisor := newTestSupervisor(t, "success")
	var group sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := supervisor.Execute(context.Background(), executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
			errors <- err
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent Execute() error = %v", err)
		}
	}
}

func TestRunnerEnvironmentDoesNotInheritUnallowlistedValues(t *testing.T) {
	t.Setenv("NILO_TEST_RUNNER_SECRET", "must-not-cross-boundary")
	environment := strings.Join(runnerEnvironment(map[string]string{"TZ": "UTC", "NILO_TEST_RUNNER_SECRET": "also-denied"}), "\x00")
	if strings.Contains(environment, "NILO_TEST_RUNNER_SECRET=") {
		t.Fatalf("runner environment contains unallowlisted value: %q", environment)
	}
	if !strings.Contains(environment, "TZ=UTC") {
		t.Fatalf("runner environment = %q, want configured allowlisted TZ", environment)
	}
}

func executeRequest() protocol.ExecuteRequest {
	return protocol.ExecuteRequest{Type: "execute", Operation: "capture", Parameters: json.RawMessage(`{}`)}
}

func newTestSupervisor(t *testing.T, mode string, arguments ...string) *ProcessSupervisor {
	t.Helper()
	root := t.TempDir()
	binary := copyTestHelper(t, root)
	runnerArguments := append([]string{mode}, arguments...)
	supervisor, err := NewProcessSupervisor(ProcessConfig{
		PackageID:   "test.integration",
		PackageRoot: root,
		Runner: manifest.Runner{
			Executable: filepath.Base(binary),
			Arguments:  runnerArguments,
		},
		GracePeriod: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewProcessSupervisor() error = %v", err)
	}
	return supervisor
}

func copyTestHelper(t *testing.T, root string) string {
	t.Helper()
	binary := filepath.Join(root, "runner-helper")
	if os.PathSeparator == '\\' {
		binary += ".exe"
	}
	source := buildHelper(t)
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.Create(binary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(output, input); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if os.PathSeparator != '\\' {
		if err := os.Chmod(binary, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return binary
}

func waitMarker(t *testing.T, path string, predicate func(string) bool) string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		value, err := os.ReadFile(path)
		if err == nil && predicate(string(value)) {
			return string(value)
		}
		time.Sleep(10 * time.Millisecond)
	}
	value, _ := os.ReadFile(path)
	t.Fatalf("marker %s did not reach expected state; got %q", path, value)
	return ""
}

func buildHelper(t *testing.T) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "runner-helper")
	if os.PathSeparator == '\\' {
		output += ".exe"
	}
	command := exec.Command("go", "build", "-o", output, "./testhelper")
	// The test binary's temp directory is not inside the module. Resolve the module from this source file instead.
	_, thisFile, _, _ := runtime.Caller(0)
	command.Dir = filepath.Dir(thisFile)
	if stdout, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build test helper: %v\n%s", err, stdout)
	}
	return output
}

type eventSinkFunc func(context.Context, protocol.Envelope) error

func (sink eventSinkFunc) Emit(context context.Context, event protocol.Envelope) error {
	return sink(context, event)
}
