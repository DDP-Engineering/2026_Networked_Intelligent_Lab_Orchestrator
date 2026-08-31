package operation

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/policy"
	"nilo.local/nilo/internal/session"
)

func TestAskCreatesPendingApprovalWithoutRunner(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Ask

	created, err := fixture.service.Submit(context.Background(), fixture.command)

	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if created.State != domain.PendingApproval || fixture.runner.executeCalls != 0 {
		t.Fatalf("session = %+v, execute calls = %d", created, fixture.runner.executeCalls)
	}
}

func TestApprovalQueuesLeasesRunsPersistsEvidenceAndCompletes(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Ask
	pending, err := fixture.service.Submit(context.Background(), fixture.command)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	completed, err := fixture.service.Approve(context.Background(), pending.ID, fixture.command.Actor)

	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if completed.State != domain.Completed || fixture.runner.executeCalls != 1 {
		t.Fatalf("session = %+v, execute calls = %d", completed, fixture.runner.executeCalls)
	}
	if len(fixture.approvals.values) != 1 || len(fixture.sessions.activeLeases) != 0 {
		t.Fatalf("approvals = %d, active leases = %v", len(fixture.approvals.values), fixture.sessions.activeLeases)
	}
	request := fixture.requests.values[pending.RequestID]
	digest, digestErr := policy.CanonicalRequestDigest(request)
	if digestErr != nil || fixture.approvals.values[pending.RequestID].RequestDigest != digest {
		t.Fatalf("approval was not bound to immutable request: %v", digestErr)
	}
}

func TestAutoEligibleRunsUnderAutoWithoutApproval(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Auto

	completed, err := fixture.service.Submit(context.Background(), fixture.command)

	if err != nil || completed.State != domain.Completed || fixture.runner.executeCalls != 1 {
		t.Fatalf("Submit() = (%+v, %v), execute calls = %d", completed, err, fixture.runner.executeCalls)
	}
}

func TestFullOnlyPromptsUnderAutoAndRunsUnderFull(t *testing.T) {
	fixture := newFixture(t)
	fixture.validator.validation.ManifestPolicy = policy.FullOnly
	fixture.command.AppMode = policy.Auto

	pending, err := fixture.service.Submit(context.Background(), fixture.command)
	if err != nil || pending.State != domain.PendingApproval || fixture.runner.executeCalls != 0 {
		t.Fatalf("Auto Submit() = (%+v, %v), execute calls = %d", pending, err, fixture.runner.executeCalls)
	}

	fullFixture := newFixture(t)
	fullFixture.validator.validation.ManifestPolicy = policy.FullOnly
	fullFixture.command.AppMode = policy.Full
	completed, err := fullFixture.service.Submit(context.Background(), fullFixture.command)
	if err != nil || completed.State != domain.Completed || fullFixture.runner.executeCalls != 1 {
		t.Fatalf("Full Submit() = (%+v, %v), execute calls = %d", completed, err, fullFixture.runner.executeCalls)
	}
}

func TestDeniedPolicyNeverCreatesRunnableSession(t *testing.T) {
	fixture := newFixture(t)
	fixture.validator.validation.ManifestPolicy = policy.Denied
	fixture.command.AppMode = policy.Full

	_, err := fixture.service.Submit(context.Background(), fixture.command)

	if !errors.Is(err, ErrPolicyDenied) || fixture.runner.executeCalls != 0 || len(fixture.sessions.values) != 0 {
		t.Fatalf("Submit() error = %v, execute calls = %d, sessions = %d", err, fixture.runner.executeCalls, len(fixture.sessions.values))
	}
}

func TestRunningCancellationInvokesSupervisorRecordsTerminalStateAndReleasesLease(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Ask
	started := make(chan struct{})
	fixture.runner.execute = func(ctx context.Context, _ protocol.ExecuteRequest, _ protocol.EventSink) (protocol.ExecuteResult, error) {
		close(started)
		<-ctx.Done()
		return protocol.ExecuteResult{}, ctx.Err()
	}
	pending, err := fixture.service.Submit(context.Background(), fixture.command)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	result := make(chan error, 1)
	go func() {
		_, approveErr := fixture.service.Approve(context.Background(), pending.ID, fixture.command.Actor)
		result <- approveErr
	}()
	<-started

	if err := fixture.service.Cancel(context.Background(), pending.ID, fixture.command.Actor); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Approve() error = %v, want context cancellation", err)
	}
	finished, err := fixture.service.Get(context.Background(), pending.ID)
	if err != nil || finished.State != domain.Cancelled || fixture.runner.cancelCalls != 1 || len(fixture.sessions.activeLeases) != 0 {
		t.Fatalf("finished = %+v, error = %v, cancels = %d, leases = %v", finished, err, fixture.runner.cancelCalls, fixture.sessions.activeLeases)
	}
}

func TestRunnerFailureUsesFailedOnlyForTypedRemoteFailure(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		want domain.SessionState
	}{
		{name: "typed remote", err: &supervisor.RunnerError{Kind: supervisor.ErrRunnerRemote, Cause: errors.New("selected failure")}, want: domain.Failed},
		{name: "crash without terminal", err: &supervisor.RunnerError{Kind: supervisor.ErrRunnerCrashed, Cause: errors.New("exit 70")}, want: domain.Incomplete},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newFixture(t)
			fixture.command.AppMode = policy.Full
			fixture.validator.validation.ManifestPolicy = policy.AlwaysPrompt
			fixture.runner.execute = func(context.Context, protocol.ExecuteRequest, protocol.EventSink) (protocol.ExecuteResult, error) {
				return protocol.ExecuteResult{}, testCase.err
			}
			pending, err := fixture.service.Submit(context.Background(), fixture.command)
			if err != nil {
				t.Fatalf("Submit() error = %v", err)
			}

			finished, err := fixture.service.Approve(context.Background(), pending.ID, fixture.command.Actor)

			if !errors.Is(err, testCase.err) || finished.State != testCase.want || len(fixture.sessions.activeLeases) != 0 {
				t.Fatalf("Approve() = (%+v, %v), want %s", finished, err, testCase.want)
			}
		})
	}
}

func TestArtifactPersistenceFailurePreventsCompleted(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Auto
	fixture.validator.validation.RequiredArtifacts = []string{"fake_artifact"}
	fixture.evidence.err = errors.New("artifact digest mismatch")
	fixture.runner.execute = func(ctx context.Context, _ protocol.ExecuteRequest, sink protocol.EventSink) (protocol.ExecuteResult, error) {
		event := protocol.Envelope{
			Protocol: "nilo.integration/1", Kind: protocol.Event, Method: protocol.Execute,
			MessageID: "artifact-event", RequestID: "execute-request", Timestamp: fixture.now,
			Payload: json.RawMessage(`{"type":"artifact","id":"fake_artifact","media_type":"application/octet-stream","size_bytes":21,"digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
		}
		if err := sink.Emit(ctx, event); err != nil {
			return protocol.ExecuteResult{}, err
		}
		return completedResult(), nil
	}

	finished, err := fixture.service.Submit(context.Background(), fixture.command)

	if !errors.Is(err, fixture.evidence.err) || finished.State != domain.Failed || len(fixture.sessions.activeLeases) != 0 {
		t.Fatalf("Submit() = (%+v, %v), leases = %v", finished, err, fixture.sessions.activeLeases)
	}
}

func TestTerminalAuditFailurePreventsCompletedAndReleasesLease(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Auto
	fixture.audit.errAction = "operation.terminal"
	fixture.audit.err = errors.New("audit disk full")
	fixture.audit.failures = -1

	finished, err := fixture.service.Submit(context.Background(), fixture.command)

	if !errors.Is(err, fixture.audit.err) || finished.State != domain.Incomplete || len(fixture.sessions.activeLeases) != 0 {
		t.Fatalf("Submit() = (%+v, %v), leases = %v", finished, err, fixture.sessions.activeLeases)
	}
}

func TestAtomicTerminalTransitionFailureNeverReportsCompleted(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Auto
	fixture.sessions.terminalErr = errors.New("terminal transaction failed")

	finished, err := fixture.service.Submit(context.Background(), fixture.command)

	if !errors.Is(err, fixture.sessions.terminalErr) || finished.State == domain.Completed {
		t.Fatalf("Submit() = (%+v, %v), must not report completed", finished, err)
	}
}

func TestApprovalRevalidatesCurrentPackageAndDeviceBeforeQueueing(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Ask
	pending, err := fixture.service.Submit(context.Background(), fixture.command)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	fixture.validator.err = errors.New("device is no longer present")

	current, err := fixture.service.Approve(context.Background(), pending.ID, fixture.command.Actor)

	if !errors.Is(err, fixture.validator.err) || current.State != domain.PendingApproval || fixture.runner.executeCalls != 0 || len(fixture.approvals.values) != 0 {
		t.Fatalf("Approve() = (%+v, %v), execute calls = %d, approvals = %d", current, err, fixture.runner.executeCalls, len(fixture.approvals.values))
	}
}

func TestApprovalRevalidationBlocksNewlyDeniedOperation(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Ask
	pending, err := fixture.service.Submit(context.Background(), fixture.command)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	fixture.validator.validation.ManifestPolicy = policy.Denied

	current, err := fixture.service.Approve(context.Background(), pending.ID, fixture.command.Actor)

	if !errors.Is(err, ErrPolicyDenied) || current.State != domain.PendingApproval || fixture.runner.executeCalls != 0 || len(fixture.approvals.values) != 0 {
		t.Fatalf("Approve() = (%+v, %v), execute calls = %d, approvals = %d", current, err, fixture.runner.executeCalls, len(fixture.approvals.values))
	}
}

func TestApprovalRetryRepairsAuditAfterDecisionWasPersisted(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Ask
	pending, err := fixture.service.Submit(context.Background(), fixture.command)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	fixture.audit.errAction = "operation.approved"
	fixture.audit.err = errors.New("temporary audit failure")
	fixture.audit.failures = 1

	current, err := fixture.service.Approve(context.Background(), pending.ID, fixture.command.Actor)
	if !errors.Is(err, fixture.audit.err) || current.State != domain.PendingApproval || len(fixture.approvals.values) != 1 {
		t.Fatalf("first Approve() = (%+v, %v), approvals = %d", current, err, len(fixture.approvals.values))
	}
	completed, err := fixture.service.Approve(context.Background(), pending.ID, fixture.command.Actor)
	if err != nil || completed.State != domain.Completed {
		t.Fatalf("retry Approve() = (%+v, %v)", completed, err)
	}
	approvedEvents := 0
	for _, event := range fixture.audit.events {
		if event.Action == "operation.approved" {
			approvedEvents++
		}
	}
	if approvedEvents != 1 {
		t.Fatalf("approval audit events = %d, want 1", approvedEvents)
	}
}

func TestPendingCancellationRecordsIdempotentDenialWithoutDispatch(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Ask
	pending, err := fixture.service.Submit(context.Background(), fixture.command)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := fixture.service.Cancel(context.Background(), pending.ID, fixture.command.Actor); err != nil {
		t.Fatalf("first Cancel() error = %v", err)
	}
	if err := fixture.service.Cancel(context.Background(), pending.ID, fixture.command.Actor); err != nil {
		t.Fatalf("second Cancel() error = %v", err)
	}
	decision := fixture.approvals.values[pending.RequestID]
	current, err := fixture.service.Get(context.Background(), pending.ID)
	if err != nil || decision.Decision != policy.ApprovalRejected || current.State != domain.PendingApproval || fixture.runner.executeCalls != 0 {
		t.Fatalf("decision = %+v, session = %+v, error = %v, execute calls = %d", decision, current, err, fixture.runner.executeCalls)
	}
}

func TestConflictingRequestReturnsTypedBusyWithoutStartingRunner(t *testing.T) {
	fixture := newFixture(t)
	fixture.command.AppMode = policy.Auto
	resource := fixture.validator.validation.Resources[0]
	ownerRequest := fixture.mustRequest(t)
	owner, err := domain.NewSession(ownerRequest.ID(), domain.Queued, fixture.now)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	fixture.sessions.values[owner.ID] = owner
	if err := fixture.sessions.AcquireLeases(context.Background(), owner.ID, []string{resource}, fixture.now); err != nil {
		t.Fatalf("AcquireLeases(owner) error = %v", err)
	}

	queued, err := fixture.service.Submit(context.Background(), fixture.command)

	var busy *session.ResourceBusyError
	if !errors.As(err, &busy) || queued.State != domain.Queued || fixture.runner.executeCalls != 0 {
		t.Fatalf("Submit() = (%+v, %v), execute calls = %d", queued, err, fixture.runner.executeCalls)
	}
}

type fixture struct {
	now       time.Time
	command   SubmitCommand
	validator *fakeValidator
	requests  *memoryRequests
	approvals *memoryApprovals
	sessions  *memorySessions
	runner    *fakeRunner
	evidence  *fakeEvidence
	audit     *memoryAudit
	service   Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	now := time.Date(2030, 8, 31, 12, 0, 0, 0, time.UTC)
	actorID, _ := domain.NewActorID()
	integrationID, _ := domain.NewIntegrationID()
	deviceID, _ := domain.NewDeviceID()
	configurationID, _ := domain.NewSnapshotID()
	requests := &memoryRequests{values: make(map[domain.RequestID]domain.OperationRequest)}
	approvals := &memoryApprovals{values: make(map[domain.RequestID]policy.ApprovalRecord)}
	sessions := newMemorySessions()
	sessionService, err := session.NewService(sessions, func() time.Time { return now })
	if err != nil {
		t.Fatalf("session.NewService() error = %v", err)
	}
	validator := &fakeValidator{validation: Validation{
		PackageID: "fake-python", ManifestPolicy: policy.AutoEligible,
		Resources: []string{integrationID.String() + "/device/stable-001"},
	}}
	runner := &fakeRunner{execute: func(context.Context, protocol.ExecuteRequest, protocol.EventSink) (protocol.ExecuteResult, error) {
		return completedResult(), nil
	}}
	evidence := &fakeEvidence{}
	auditRepository := &memoryAudit{}
	serviceValue, err := NewService(Dependencies{
		Validator: validator, Requests: requests, Approvals: approvals,
		Sessions: sessionService, SessionRepository: sessions,
		Audit: auditRepository, Supervisor: runner, Evidence: evidence,
		Now: func() time.Time { return now }, NewID: func() (string, error) { return uuid.NewString(), nil },
		NewSnapshotID: domain.NewSnapshotID,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return &fixture{
		now: now, validator: validator, requests: requests, approvals: approvals,
		sessions: sessions, runner: runner, evidence: evidence, audit: auditRepository, service: serviceValue,
		command: SubmitCommand{
			Actor: domain.Actor{ID: actorID}, IntegrationID: integrationID, OperationName: "echo",
			DeviceID: deviceID, CanonicalParameters: json.RawMessage(`{"value":"hello"}`),
			ConfigurationSnapshot: configurationID, Deadline: now.Add(time.Minute),
			AppKnown: true, AppMode: policy.Ask, DeviceKnown: true, DeviceMode: policy.Full,
		},
	}
}

func (fixture *fixture) mustRequest(t *testing.T) domain.OperationRequest {
	t.Helper()
	policyID, _ := domain.NewSnapshotID()
	request, err := domain.NewOperationRequest(domain.OperationRequestInput{
		Actor: fixture.command.Actor, IntegrationID: fixture.command.IntegrationID,
		OperationName: fixture.command.OperationName, DeviceID: fixture.command.DeviceID,
		CanonicalParameters:   fixture.command.CanonicalParameters,
		ConfigurationSnapshot: fixture.command.ConfigurationSnapshot, PolicySnapshot: policyID,
		Deadline: fixture.command.Deadline,
	}, fixture.now)
	if err != nil {
		t.Fatalf("NewOperationRequest() error = %v", err)
	}
	return request
}

func completedResult() protocol.ExecuteResult {
	return protocol.ExecuteResult{Type: "result", Status: "completed", Data: json.RawMessage(`{}`)}
}

type fakeValidator struct {
	validation Validation
	err        error
}

func (validator *fakeValidator) Validate(context.Context, SubmitCommand) (Validation, error) {
	return validator.validation, validator.err
}

func (validator *fakeValidator) Revalidate(context.Context, domain.OperationRequest) (Validation, error) {
	return validator.validation, validator.err
}

type memoryRequests struct {
	values map[domain.RequestID]domain.OperationRequest
}

func (repository *memoryRequests) Create(_ context.Context, request domain.OperationRequest, _ policy.Snapshot) error {
	repository.values[request.ID()] = request
	return nil
}

func (repository *memoryRequests) Get(_ context.Context, id domain.RequestID) (domain.OperationRequest, error) {
	value, ok := repository.values[id]
	if !ok {
		return domain.OperationRequest{}, ErrRequestNotFound
	}
	return value, nil
}

type memoryApprovals struct {
	values map[domain.RequestID]policy.ApprovalRecord
}

func (repository *memoryApprovals) RecordApproval(_ context.Context, record policy.ApprovalRecord) error {
	if _, exists := repository.values[record.RequestID]; exists {
		return policy.ErrApprovalAlreadyRecorded
	}
	repository.values[record.RequestID] = record
	return nil
}

func (repository *memoryApprovals) GetApproval(_ context.Context, requestID domain.RequestID) (policy.ApprovalRecord, error) {
	value, ok := repository.values[requestID]
	if !ok {
		return policy.ApprovalRecord{}, policy.ErrApprovalNotFound
	}
	return value, nil
}

type memorySessions struct {
	mu           sync.Mutex
	values       map[domain.SessionID]domain.Session
	activeLeases map[string]domain.SessionID
	terminalErr  error
}

func newMemorySessions() *memorySessions {
	return &memorySessions{values: make(map[domain.SessionID]domain.Session), activeLeases: make(map[string]domain.SessionID)}
}

func (repository *memorySessions) Create(_ context.Context, value domain.Session) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.values[value.ID] = value
	return nil
}

func (repository *memorySessions) Get(_ context.Context, id domain.SessionID) (domain.Session, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	value, ok := repository.values[id]
	if !ok {
		return domain.Session{}, session.ErrSessionNotFound
	}
	return value, nil
}

func (repository *memorySessions) Transition(_ context.Context, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return repository.transition(id, from, to, at, false)
}

func (repository *memorySessions) TransitionAndRelease(_ context.Context, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.terminalErr != nil {
		return domain.Session{}, repository.terminalErr
	}
	return repository.transition(id, from, to, at, true)
}

func (repository *memorySessions) transition(id domain.SessionID, from, to domain.SessionState, at time.Time, release bool) (domain.Session, error) {
	current := repository.values[id]
	if current.State != from {
		return domain.Session{}, session.ErrSessionStateConflict
	}
	if err := domain.ValidateTransition(from, to); err != nil {
		return domain.Session{}, err
	}
	var terminal *time.Time
	if to == domain.Completed || to == domain.Failed || to == domain.Cancelled || to == domain.Incomplete {
		terminal = &at
	}
	updated, err := domain.RehydrateSession(id, current.RequestID, to, current.CreatedAt, at, terminal)
	if err != nil {
		return domain.Session{}, err
	}
	repository.values[id] = updated
	if release {
		for resource, owner := range repository.activeLeases {
			if owner == id {
				delete(repository.activeLeases, resource)
			}
		}
	}
	return updated, nil
}

func (repository *memorySessions) AcquireLeases(_ context.Context, id domain.SessionID, resources []string, _ time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, resource := range resources {
		if owner, exists := repository.activeLeases[resource]; exists && owner != id {
			return &session.ResourceBusyError{OwnerSessionID: owner}
		}
	}
	for _, resource := range resources {
		repository.activeLeases[resource] = id
	}
	return nil
}

func (repository *memorySessions) ReleaseLeases(_ context.Context, id domain.SessionID, _ time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for resource, owner := range repository.activeLeases {
		if owner == id {
			delete(repository.activeLeases, resource)
		}
	}
	return nil
}

func (repository *memorySessions) ListInterrupted(context.Context) ([]domain.Session, error) {
	return nil, nil
}

func (repository *memorySessions) ReconcileStartup(context.Context, time.Time) error { return nil }

type memoryAudit struct {
	events    []audit.Event
	errAction string
	err       error
	failures  int
}

func (repository *memoryAudit) Append(_ context.Context, event audit.Event) error {
	if event.Action == repository.errAction && repository.failures != 0 {
		if repository.failures > 0 {
			repository.failures--
		}
		return repository.err
	}
	for _, existing := range repository.events {
		if existing.ID == event.ID {
			return audit.ErrEventAlreadyAppended
		}
	}
	repository.events = append(repository.events, event)
	return nil
}

func (repository *memoryAudit) List(context.Context, audit.Filter) ([]audit.Event, error) {
	return append([]audit.Event(nil), repository.events...), nil
}

type fakeRunner struct {
	mu           sync.Mutex
	execute      func(context.Context, protocol.ExecuteRequest, protocol.EventSink) (protocol.ExecuteResult, error)
	executeCalls int
	cancelCalls  int
}

func (runner *fakeRunner) Describe(context.Context, string) (protocol.DescribeResult, error) {
	return protocol.DescribeResult{Type: "describe", Status: "ok"}, nil
}

func (runner *fakeRunner) Execute(ctx context.Context, request protocol.ExecuteRequest, sink protocol.EventSink) (protocol.ExecuteResult, error) {
	runner.mu.Lock()
	runner.executeCalls++
	runner.mu.Unlock()
	return runner.execute(ctx, request, sink)
}

func (runner *fakeRunner) Cancel(context.Context, string, domain.RequestID) error {
	runner.mu.Lock()
	runner.cancelCalls++
	runner.mu.Unlock()
	return nil
}

func (runner *fakeRunner) Shutdown(context.Context) error { return nil }

type fakeEvidence struct {
	err   error
	items []Evidence
}

func (store *fakeEvidence) Persist(_ context.Context, evidence Evidence) error {
	store.items = append(store.items, evidence)
	return store.err
}
