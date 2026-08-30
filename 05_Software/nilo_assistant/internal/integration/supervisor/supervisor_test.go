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
		{name: "exit before result", mode: "exit", want: ErrRunnerCrashed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newTestSupervisor(t, test.mode).Execute(context.Background(), executeRequest(), eventSinkFunc(func(context.Context, protocol.Envelope) error { return nil }))
			if !errors.Is(err, test.want) {
				t.Fatalf("Execute() error = %v, want %v", err, test.want)
			}
		})
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

func newTestSupervisor(t *testing.T, mode string) *ProcessSupervisor {
	t.Helper()
	root := t.TempDir()
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
	supervisor, err := NewProcessSupervisor(ProcessConfig{
		PackageID:   "test.integration",
		PackageRoot: root,
		Runner: manifest.Runner{
			Executable: filepath.Base(binary),
			Arguments:  []string{mode},
		},
		GracePeriod: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewProcessSupervisor() error = %v", err)
	}
	return supervisor
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
