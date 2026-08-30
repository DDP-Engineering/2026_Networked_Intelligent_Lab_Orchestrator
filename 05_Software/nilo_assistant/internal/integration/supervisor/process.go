package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/config"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/protocol"
)

var (
	ErrInvalidProcessConfig = errors.New("invalid integration process configuration")
	ErrRunnerCrashed        = errors.New("integration runner crashed")
	ErrRunnerTimeout        = errors.New("integration runner timed out")
	ErrRunnerProtocol       = errors.New("integration runner protocol failure")
)

// ProcessConfig is the already validated package launch contract. Environment
// values are accepted only from the fixed allowlist in runnerEnvironment.
type ProcessConfig struct {
	PackageID   string
	PackageRoot string
	Runner      manifest.Runner
	GracePeriod time.Duration
	Environment map[string]string
}

// Supervisor is the Bench Core integration process boundary.
//
// The Execute signature is fixed by the Increment 1 contract and does not
// receive a domain.RequestID. The supervisor still validates and correlates a
// targeted Cancel at the runner protocol layer when an operation layer owns
// that same ID; until that wiring exists, callers must use context cancellation
// as the usable cancellation path.
type Supervisor interface {
	Describe(context.Context, string) (protocol.DescribeResult, error)
	Execute(context.Context, protocol.ExecuteRequest, protocol.EventSink) (protocol.ExecuteResult, error)
	Cancel(context.Context, string, domain.RequestID) error
	Shutdown(context.Context) error
}

// ProcessSupervisor starts one isolated runner process for each operation.
// Separate processes prevent a faulty integration from corrupting Bench Core.
type ProcessSupervisor struct {
	config ProcessConfig
	mu     sync.Mutex
	active map[string]*runnerProcess
}

func NewProcessSupervisor(config ProcessConfig) (*ProcessSupervisor, error) {
	if strings.TrimSpace(config.PackageID) == "" || strings.TrimSpace(config.PackageRoot) == "" || strings.TrimSpace(config.Runner.Executable) == "" {
		return nil, ErrInvalidProcessConfig
	}
	root, err := filepath.Abs(config.PackageRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: package root: %v", ErrInvalidProcessConfig, err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("%w: package root: %v", ErrInvalidProcessConfig, err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: package root is not a directory", ErrInvalidProcessConfig)
	}
	executable, err := resolveExecutable(root, config.Runner.Executable)
	if err != nil {
		return nil, err
	}
	config.PackageRoot = root
	config.Runner.Executable = executable
	if config.GracePeriod <= 0 {
		config.GracePeriod = 2 * time.Second
	}
	return &ProcessSupervisor{config: config, active: make(map[string]*runnerProcess)}, nil
}

func resolveExecutable(root, executable string) (string, error) {
	if !filepath.IsAbs(executable) {
		resolved, err := config.ResolveWithin(root, executable)
		if err != nil {
			return "", fmt.Errorf("%w: runner executable: %v", ErrInvalidProcessConfig, err)
		}
		executable = resolved
	}
	physical, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("%w: runner executable: %v", ErrInvalidProcessConfig, err)
	}
	relative, err := filepath.Rel(root, physical)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: runner executable escapes package root", ErrInvalidProcessConfig)
	}
	info, err := os.Stat(physical)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("%w: runner executable is unavailable", ErrInvalidProcessConfig)
	}
	return physical, nil
}

func (supervisor *ProcessSupervisor) Describe(ctx context.Context, packageID string) (protocol.DescribeResult, error) {
	if packageID != supervisor.config.PackageID {
		return protocol.DescribeResult{}, fmt.Errorf("%w: unknown package %q", ErrInvalidProcessConfig, packageID)
	}
	runner, err := supervisor.start(ctx)
	if err != nil {
		return protocol.DescribeResult{}, err
	}
	defer runner.close()
	messageID, err := runner.send(protocol.Describe, protocol.ControlPayload{Type: "describe"})
	if err != nil {
		return protocol.DescribeResult{}, runner.protocolError(err)
	}
	message, err := runner.receive(ctx)
	if err != nil {
		return protocol.DescribeResult{}, runner.classify(ctx, err)
	}
	if message.Kind != protocol.Response || message.Method != protocol.Describe || message.RequestID != messageID {
		return protocol.DescribeResult{}, runner.protocolError(fmt.Errorf("unexpected describe response"))
	}
	var result protocol.DescribeResult
	if err := decodeStrict(message.Payload, &result); err != nil || result.Validate() != nil {
		if err == nil {
			err = result.Validate()
		}
		return protocol.DescribeResult{}, runner.protocolError(err)
	}
	return result, nil
}

func (supervisor *ProcessSupervisor) Execute(ctx context.Context, request protocol.ExecuteRequest, sink protocol.EventSink) (protocol.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return protocol.ExecuteResult{}, fmt.Errorf("%w: execute request: %v", ErrRunnerProtocol, err)
	}
	if sink == nil {
		return protocol.ExecuteResult{}, fmt.Errorf("%w: event sink is required", ErrInvalidProcessConfig)
	}
	runner, err := supervisor.start(ctx)
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	requestID, err := domain.NewRequestID()
	if err != nil {
		runner.close()
		return protocol.ExecuteResult{}, fmt.Errorf("create request ID: %w", err)
	}
	supervisor.mu.Lock()
	supervisor.active[requestID.String()] = runner
	supervisor.mu.Unlock()
	defer func() {
		supervisor.mu.Lock()
		delete(supervisor.active, requestID.String())
		supervisor.mu.Unlock()
		runner.close()
	}()

	messageID, err := runner.send(protocol.Execute, request)
	if err != nil {
		return protocol.ExecuteResult{}, runner.protocolError(err)
	}
	for {
		message, err := runner.receive(ctx)
		if err != nil {
			return protocol.ExecuteResult{}, runner.classify(ctx, err)
		}
		if message.Method != protocol.Execute || message.RequestID != messageID {
			return protocol.ExecuteResult{}, runner.protocolError(fmt.Errorf("mismatched execute message"))
		}
		switch message.Kind {
		case protocol.Event:
			if err := sink.Emit(ctx, message); err != nil {
				return protocol.ExecuteResult{}, err
			}
		case protocol.Response:
			var result protocol.ExecuteResult
			if err := decodeStrict(message.Payload, &result); err == nil && result.Validate() == nil {
				return result, nil
			}
			var remoteError protocol.Error
			if err := decodeStrict(message.Payload, &remoteError); err == nil && remoteError.Validate() == nil {
				return protocol.ExecuteResult{}, fmt.Errorf("%w: %s: %s", ErrRunnerProtocol, remoteError.Code, remoteError.Message)
			}
			return protocol.ExecuteResult{}, runner.protocolError(fmt.Errorf("invalid execute terminal payload"))
		default:
			return protocol.ExecuteResult{}, runner.protocolError(fmt.Errorf("unexpected execute message kind"))
		}
	}
}

func (supervisor *ProcessSupervisor) Cancel(ctx context.Context, packageID string, requestID domain.RequestID) error {
	if packageID != supervisor.config.PackageID {
		return fmt.Errorf("%w: unknown package %q", ErrInvalidProcessConfig, packageID)
	}
	supervisor.mu.Lock()
	runner := supervisor.active[requestID.String()]
	supervisor.mu.Unlock()
	if runner == nil {
		return nil
	}
	return runner.stop(ctx, requestID.String())
}

func (supervisor *ProcessSupervisor) Shutdown(ctx context.Context) error {
	supervisor.mu.Lock()
	runners := make([]*runnerProcess, 0, len(supervisor.active))
	for _, runner := range supervisor.active {
		runners = append(runners, runner)
	}
	supervisor.mu.Unlock()
	var result error
	for _, runner := range runners {
		result = errors.Join(result, runner.stop(ctx, "shutdown"))
	}
	return result
}

type runnerProcess struct {
	cmd         *exec.Cmd
	encoder     *protocol.Encoder
	decoder     *protocol.Decoder
	stdin       io.WriteCloser
	diagnostics *diagnosticRing
	grace       time.Duration
	wait        chan error
	stopOnce    sync.Once
}

func (supervisor *ProcessSupervisor) start(ctx context.Context) (*runnerProcess, error) {
	command := exec.CommandContext(ctx, supervisor.config.Runner.Executable, supervisor.config.Runner.Arguments...)
	command.Dir = supervisor.config.PackageRoot
	command.Env = runnerEnvironment(supervisor.config.Environment)
	configureProcess(command)
	// Context cancellation is handled by stop so the runner first sees cancel
	// and safe_shutdown. The CommandContext watcher remains as a final guard.
	command.Cancel = func() error { return nil }
	command.WaitDelay = supervisor.config.GracePeriod * 2
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	diagnostics := &diagnosticRing{}
	command.Stderr = diagnostics
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("%w: start: %v", ErrRunnerCrashed, err)
	}
	runner := &runnerProcess{
		cmd:         command,
		stdin:       stdin,
		diagnostics: diagnostics,
		grace:       supervisor.config.GracePeriod,
		wait:        make(chan error, 1),
	}
	registry := protocol.NewRegistry()
	runner.encoder = protocol.NewEncoderWithRegistry(stdin, registry)
	runner.decoder = protocol.NewDecoderWithRegistry(stdout, registry)
	go func() { runner.wait <- command.Wait() }()
	if err := runner.handshake(ctx); err != nil {
		runner.close()
		return nil, runner.classify(ctx, err)
	}
	return runner, nil
}

func runnerEnvironment(extra map[string]string) []string {
	allowed := map[string]struct{}{"PATH": {}, "LANG": {}, "LC_ALL": {}, "TZ": {}, "SYSTEMROOT": {}, "WINDIR": {}}
	environment := make([]string, 0, len(extra)+4)
	for key, value := range extra {
		if _, ok := allowed[strings.ToUpper(key)]; ok && !strings.ContainsRune(key, '=') && !strings.ContainsRune(value, '\x00') {
			environment = append(environment, key+"="+value)
		}
	}
	for key := range allowed {
		if _, set := extra[key]; set {
			continue
		}
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

func (runner *runnerProcess) handshake(ctx context.Context) error {
	messageID, err := runner.send(protocol.Hello, protocol.HelloRequest{Type: "hello", Client: "nilo-core"})
	if err != nil {
		return err
	}
	message, err := runner.receive(ctx)
	if err != nil {
		return err
	}
	if message.Kind != protocol.Response || message.Method != protocol.Hello || message.RequestID != messageID {
		return fmt.Errorf("hello response is not correlated")
	}
	var response protocol.HelloResponse
	if err := decodeStrict(message.Payload, &response); err != nil {
		return err
	}
	return response.Validate()
}

func jsonPayload(payload any) (json.RawMessage, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func decodeStrict(payload json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func (runner *runnerProcess) send(method protocol.Method, payload any) (string, error) {
	encoded, err := jsonPayload(payload)
	if err != nil {
		return "", err
	}
	messageID := uuid.NewString()
	message := protocol.Envelope{Protocol: protocol.ProtocolV1, Kind: protocol.Request, MessageID: messageID, Method: method, Timestamp: time.Now().UTC(), Payload: encoded}
	return messageID, runner.encoder.Encode(message)
}

func (runner *runnerProcess) receive(ctx context.Context) (protocol.Envelope, error) {
	type result struct {
		message protocol.Envelope
		err     error
	}
	decoded := make(chan result, 1)
	go func() {
		message, err := runner.decoder.Decode()
		decoded <- result{message: message, err: err}
	}()
	select {
	case result := <-decoded:
		return result.message, result.err
	case <-ctx.Done():
		_ = runner.stop(context.Background(), "context")
		return protocol.Envelope{}, ctx.Err()
	}
}

func (runner *runnerProcess) stop(ctx context.Context, target string) error {
	var result error
	runner.stopOnce.Do(func() {
		_, result = runner.send(protocol.Cancel, protocol.CancelRequest{Type: "cancel", TargetRequestID: target})
		select {
		case <-time.After(runner.grace):
		case <-ctx.Done():
			result = errors.Join(result, ctx.Err())
		}
		if _, err := runner.send(protocol.SafeShutdown, protocol.ControlPayload{Type: "safe_shutdown"}); err != nil {
			result = errors.Join(result, err)
		}
		if err := terminateProcess(runner.cmd); err != nil {
			result = errors.Join(result, err)
		}
	})
	return result
}

func (runner *runnerProcess) close() {
	_ = runner.stdin.Close()
	select {
	case <-runner.wait:
	case <-time.After(runner.grace):
		_ = terminateProcess(runner.cmd)
		<-runner.wait
	}
}

func (runner *runnerProcess) protocolError(cause error) error {
	return &RunnerError{Kind: ErrRunnerProtocol, Cause: cause, Diagnostic: runner.diagnostics.String()}
}

func (runner *runnerProcess) classify(ctx context.Context, cause error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &RunnerError{Kind: ErrRunnerTimeout, Cause: ctx.Err(), Diagnostic: runner.diagnostics.String()}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return &RunnerError{Kind: context.Canceled, Cause: ctx.Err(), Diagnostic: runner.diagnostics.String()}
	}
	if errors.Is(cause, protocol.ErrInvalidMessage) || errors.Is(cause, protocol.ErrUnsupportedProtocol) || errors.Is(cause, protocol.ErrLineTooLong) {
		return runner.protocolError(cause)
	}
	return &RunnerError{Kind: ErrRunnerCrashed, Cause: cause, Diagnostic: runner.diagnostics.String()}
}

// RunnerError contains a classified, bounded runner failure.
type RunnerError struct {
	Kind       error
	Cause      error
	Diagnostic string
}

func (runnerError *RunnerError) Error() string {
	if runnerError.Diagnostic == "" {
		return fmt.Sprintf("%v: %v", runnerError.Kind, runnerError.Cause)
	}
	return fmt.Sprintf("%v: %v (stderr: %s)", runnerError.Kind, runnerError.Cause, runnerError.Diagnostic)
}

func (runnerError *RunnerError) Unwrap() []error { return []error{runnerError.Kind, runnerError.Cause} }

// Diagnostics returns the sanitized stderr retained by a RunnerError.
func Diagnostics(err error) string {
	var runnerError *RunnerError
	if errors.As(err, &runnerError) {
		return runnerError.Diagnostic
	}
	return ""
}
