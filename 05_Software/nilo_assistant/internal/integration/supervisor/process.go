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
	"sort"
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
	ErrRunnerRemote         = errors.New("integration runner reported an execution error")
	ErrSupervisorClosed     = errors.New("integration supervisor is shut down")
	ErrEventSink            = errors.New("integration event sink failed")
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
	config  ProcessConfig
	mu      sync.Mutex
	active  map[string]*runnerProcess
	runners map[*runnerProcess]struct{}
	closed  bool
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
	config.Runner.Arguments = append([]string(nil), config.Runner.Arguments...)
	environment, err := canonicalEnvironment(config.Environment)
	if err != nil {
		return nil, err
	}
	config.Environment = environment
	if config.GracePeriod <= 0 {
		config.GracePeriod = 2 * time.Second
	}
	return &ProcessSupervisor{config: config, active: make(map[string]*runnerProcess), runners: make(map[*runnerProcess]struct{})}, nil
}

func canonicalEnvironment(source map[string]string) (map[string]string, error) {
	if source == nil {
		return nil, nil
	}
	canonical := make(map[string]string, len(source))
	for key, value := range source {
		name := strings.ToUpper(strings.TrimSpace(key))
		if name == "" || strings.ContainsRune(name, '=') || strings.ContainsRune(value, '\x00') {
			return nil, fmt.Errorf("%w: invalid environment entry", ErrInvalidProcessConfig)
		}
		if _, exists := canonical[name]; exists {
			return nil, fmt.Errorf("%w: duplicate environment key %q", ErrInvalidProcessConfig, name)
		}
		canonical[name] = value
	}
	return canonical, nil
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
	defer supervisor.release(runner)
	messageID, err := runner.send(ctx, protocol.Describe, protocol.ControlPayload{Type: "describe"})
	if err != nil {
		if ctx.Err() != nil {
			_ = runner.stop(context.Background())
		}
		return protocol.DescribeResult{}, runner.classify(ctx, err)
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
		supervisor.release(runner)
		return protocol.ExecuteResult{}, fmt.Errorf("create request ID: %w", err)
	}
	runner.setExecuteMessageID(requestID.String())
	supervisor.mu.Lock()
	supervisor.active[requestID.String()] = runner
	supervisor.mu.Unlock()
	defer supervisor.release(runner)

	messageID, err := runner.sendWithID(ctx, protocol.Execute, request, requestID.String())
	if err != nil {
		if ctx.Err() != nil {
			_ = runner.stop(context.Background())
		}
		return protocol.ExecuteResult{}, runner.classify(ctx, err)
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
			if err := runner.emit(ctx, sink, message); err != nil {
				if errors.Is(err, ErrEventSink) {
					return protocol.ExecuteResult{}, err
				}
				return protocol.ExecuteResult{}, runner.classify(ctx, err)
			}
		case protocol.Response:
			var result protocol.ExecuteResult
			if err := decodeStrict(message.Payload, &result); err == nil && result.Validate() == nil {
				return result, nil
			}
			var remoteError protocol.Error
			if err := decodeStrict(message.Payload, &remoteError); err == nil && remoteError.Validate() == nil {
				return protocol.ExecuteResult{}, &RunnerError{Kind: ErrRunnerRemote, Cause: &RemoteExecutionError{Code: remoteError.Code, Message: remoteError.Message, Retryable: remoteError.Retryable}, Diagnostic: runner.diagnostics.String()}
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
	return runner.stop(ctx)
}

func (supervisor *ProcessSupervisor) Shutdown(ctx context.Context) error {
	supervisor.mu.Lock()
	supervisor.closed = true
	runners := make([]*runnerProcess, 0, len(supervisor.runners))
	for runner := range supervisor.runners {
		runners = append(runners, runner)
	}
	supervisor.mu.Unlock()
	results := make(chan error, len(runners))
	for _, runner := range runners {
		go func() { results <- runner.stop(ctx) }()
	}
	var result error
	for range runners {
		select {
		case err := <-results:
			result = errors.Join(result, err)
		case <-ctx.Done():
			return errors.Join(result, ctx.Err())
		}
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
	closeOnce   sync.Once
	stateMu     sync.RWMutex
	executeID   string
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
	supervisor.mu.Lock()
	if supervisor.closed {
		supervisor.mu.Unlock()
		return nil, ErrSupervisorClosed
	}
	if err := command.Start(); err != nil {
		supervisor.mu.Unlock()
		return nil, fmt.Errorf("%w: start: %v", ErrRunnerCrashed, err)
	}
	go func() { runner.wait <- command.Wait() }()
	supervisor.runners[runner] = struct{}{}
	supervisor.mu.Unlock()
	if err := runner.handshake(ctx); err != nil {
		runner.close()
		supervisor.unregister(runner)
		return nil, runner.classify(ctx, err)
	}
	return runner, nil
}

func (supervisor *ProcessSupervisor) release(runner *runnerProcess) {
	runner.close()
	supervisor.unregister(runner)
}

func (supervisor *ProcessSupervisor) unregister(runner *runnerProcess) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	delete(supervisor.runners, runner)
	for requestID, active := range supervisor.active {
		if active == runner {
			delete(supervisor.active, requestID)
		}
	}
}

func runnerEnvironment(extra map[string]string) []string {
	allowed := map[string]struct{}{"PATH": {}, "LANG": {}, "LC_ALL": {}, "TZ": {}, "SYSTEMROOT": {}, "WINDIR": {}}
	values := make(map[string]string, len(extra)+len(allowed))
	for key, value := range extra {
		canonical := strings.ToUpper(strings.TrimSpace(key))
		if _, ok := allowed[canonical]; ok && !strings.ContainsRune(value, '\x00') {
			values[canonical] = value
		}
	}
	for key := range allowed {
		if _, set := values[key]; set {
			continue
		}
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}

func (runner *runnerProcess) handshake(ctx context.Context) error {
	messageID, err := runner.send(ctx, protocol.Hello, protocol.HelloRequest{Type: "hello", Client: "nilo-core"})
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

func (runner *runnerProcess) send(ctx context.Context, method protocol.Method, payload any) (string, error) {
	return runner.sendWithID(ctx, method, payload, uuid.NewString())
}

func (runner *runnerProcess) sendWithID(ctx context.Context, method protocol.Method, payload any, messageID string) (string, error) {
	encoded, err := jsonPayload(payload)
	if err != nil {
		return "", err
	}
	message := protocol.Envelope{Protocol: protocol.ProtocolV1, Kind: protocol.Request, MessageID: messageID, Method: method, Timestamp: time.Now().UTC(), Payload: encoded}
	completed := make(chan error, 1)
	go func() { completed <- runner.encoder.Encode(message) }()
	select {
	case err := <-completed:
		return messageID, err
	case <-ctx.Done():
		return messageID, ctx.Err()
	}
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
		_ = runner.stop(context.Background())
		return protocol.Envelope{}, ctx.Err()
	}
}

func (runner *runnerProcess) setExecuteMessageID(messageID string) {
	runner.stateMu.Lock()
	runner.executeID = messageID
	runner.stateMu.Unlock()
}

func (runner *runnerProcess) executeMessageID() string {
	runner.stateMu.RLock()
	defer runner.stateMu.RUnlock()
	return runner.executeID
}

func (runner *runnerProcess) control(ctx context.Context, method protocol.Method, payload any) error {
	deadline := runner.grace / 2
	if deadline <= 0 {
		deadline = time.Millisecond
	}
	controlContext, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	_, err := runner.send(controlContext, method, payload)
	return err
}

func (runner *runnerProcess) stop(ctx context.Context) error {
	var result error
	runner.stopOnce.Do(func() {
		if target := runner.executeMessageID(); target != "" {
			result = runner.control(ctx, protocol.Cancel, protocol.CancelRequest{Type: "cancel", TargetRequestID: target})
		}
		select {
		case <-time.After(runner.grace):
		case <-ctx.Done():
			result = errors.Join(result, ctx.Err())
		}
		if err := runner.control(ctx, protocol.SafeShutdown, protocol.ControlPayload{Type: "safe_shutdown"}); err != nil {
			result = errors.Join(result, err)
		}
		// Give a cooperative runner one bounded scheduling interval to receive
		// safe_shutdown before the platform termination escalation.
		select {
		case <-time.After(runner.grace):
		case <-ctx.Done():
			result = errors.Join(result, ctx.Err())
		}
		if err := terminateProcess(runner.cmd); err != nil {
			result = errors.Join(result, err)
		}
	})
	return result
}

func (runner *runnerProcess) close() {
	runner.closeOnce.Do(func() {
		_ = runner.stdin.Close()
		select {
		case <-runner.wait:
		case <-time.After(runner.grace):
			_ = terminateProcess(runner.cmd)
			<-runner.wait
		}
	})
}

func (runner *runnerProcess) emit(ctx context.Context, sink protocol.EventSink, message protocol.Envelope) error {
	completed := make(chan error, 1)
	go func() { completed <- sink.Emit(ctx, message) }()
	select {
	case err := <-completed:
		if err != nil {
			_ = runner.stop(context.Background())
			return fmt.Errorf("%w: %w", ErrEventSink, err)
		}
		return err
	case <-ctx.Done():
		_ = runner.stop(context.Background())
		return ctx.Err()
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
	for _, protocolError := range []error{
		protocol.ErrInvalidMessage,
		protocol.ErrUnsupportedProtocol,
		protocol.ErrInvalidKind,
		protocol.ErrInvalidMethod,
		protocol.ErrDuplicateMessageID,
		protocol.ErrUnknownRequest,
		protocol.ErrRequestMethodMismatch,
		protocol.ErrMultipleTerminalResponse,
		protocol.ErrEventAfterTerminal,
		protocol.ErrOutstandingExecute,
		protocol.ErrMissingNewline,
		protocol.ErrLineTooLong,
	} {
		if errors.Is(cause, protocolError) {
			return runner.protocolError(cause)
		}
	}
	return &RunnerError{Kind: ErrRunnerCrashed, Cause: cause, Diagnostic: runner.diagnostics.String()}
}

// RemoteExecutionError preserves a valid terminal error returned by an
// integration without treating the protocol transport as invalid.
type RemoteExecutionError struct {
	Code      string
	Message   string
	Retryable bool
}

func (error *RemoteExecutionError) Error() string {
	return fmt.Sprintf("%s: %s", error.Code, error.Message)
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
