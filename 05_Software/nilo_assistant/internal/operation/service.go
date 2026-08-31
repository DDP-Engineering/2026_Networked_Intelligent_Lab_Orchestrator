// Package operation coordinates one immutable request from policy resolution
// through supervised execution and truthful durable termination.
package operation

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/policy"
	"nilo.local/nilo/internal/session"
)

var (
	ErrInvalidDependencies     = errors.New("invalid operation service dependencies")
	ErrInvalidValidation       = errors.New("invalid operation validation")
	ErrPolicyDenied            = errors.New("operation policy denied execution")
	ErrRequestNotFound         = errors.New("operation request not found")
	ErrApprovalAlreadyDecided  = errors.New("approval was already decided")
	ErrRequiredArtifactMissing = errors.New("required artifact evidence is missing")
	ErrExecutionNotActive      = errors.New("session execution is not active")
	ErrApprovalSessionState    = errors.New("session is not pending approval")
)

// SubmitCommand carries authenticated identity and already canonicalized JSON.
// Validator remains responsible for the current package, operation schema,
// device-presence, and compatibility checks.
type SubmitCommand struct {
	Actor                 domain.Actor
	IntegrationID         domain.IntegrationID
	OperationName         string
	DeviceID              domain.DeviceID
	CanonicalParameters   json.RawMessage
	ConfigurationSnapshot domain.SnapshotID
	Deadline              time.Time
	AppKnown              bool
	AppMode               policy.AppMode
	OperationOverride     *policy.OperationPolicy
	DeviceKnown           bool
	DeviceMode            policy.AppMode
}

// Validation is the current manifest-derived execution contract.
type Validation struct {
	PackageID         string
	ManifestPolicy    policy.OperationPolicy
	Resources         []string
	RequiredArtifacts []string
}

type Validator interface {
	Validate(context.Context, SubmitCommand) (Validation, error)
	Revalidate(context.Context, domain.OperationRequest) (Validation, error)
}

// RequestRepository atomically persists the immutable request and the exact
// policy snapshot from which its decision was derived.
type RequestRepository interface {
	Create(context.Context, domain.OperationRequest, policy.Snapshot) error
	Get(context.Context, domain.RequestID) (domain.OperationRequest, error)
}

type ApprovalRepository interface {
	RecordApproval(context.Context, policy.ApprovalRecord) error
	GetApproval(context.Context, domain.RequestID) (policy.ApprovalRecord, error)
}

type SessionService interface {
	Create(context.Context, domain.OperationRequest, policy.DecisionKind) (domain.Session, error)
	Transition(context.Context, domain.SessionID, domain.SessionState, domain.SessionState) (domain.Session, error)
	ReconcileStartup(context.Context) error
}

type SessionRepository interface {
	Get(context.Context, domain.SessionID) (domain.Session, error)
	AcquireLeases(context.Context, domain.SessionID, []string, time.Time) error
	ReleaseLeases(context.Context, domain.SessionID, time.Time) error
}

// Evidence gives an artifact adapter the complete immutable correlation and
// the already protocol-validated declaration.
type Evidence struct {
	Request domain.OperationRequest
	Session domain.Session
	Event   protocol.Envelope
}

type EvidenceStore interface {
	Persist(context.Context, Evidence) error
}

type Dependencies struct {
	Validator         Validator
	Requests          RequestRepository
	Approvals         ApprovalRepository
	Sessions          SessionService
	SessionRepository SessionRepository
	Audit             audit.Repository
	Supervisor        supervisor.Supervisor
	Evidence          EvidenceStore
	Now               func() time.Time
	NewID             func() (string, error)
	NewSnapshotID     func() (domain.SnapshotID, error)
}

type Service interface {
	Submit(context.Context, SubmitCommand) (domain.Session, error)
	Approve(context.Context, domain.SessionID, domain.Actor) (domain.Session, error)
	Deny(context.Context, domain.SessionID, domain.Actor, string) error
	Cancel(context.Context, domain.SessionID, domain.Actor) error
	Get(context.Context, domain.SessionID) (domain.Session, error)
}

type service struct {
	dependencies Dependencies
	activeMu     sync.Mutex
	active       map[domain.SessionID]activeExecution
}

type activeExecution struct {
	requestID domain.RequestID
	packageID string
	cancel    context.CancelFunc
}

func NewService(dependencies Dependencies) (Service, error) {
	if dependencies.Validator == nil || dependencies.Requests == nil || dependencies.Approvals == nil ||
		dependencies.Sessions == nil || dependencies.SessionRepository == nil || dependencies.Audit == nil ||
		dependencies.Supervisor == nil || dependencies.Evidence == nil || dependencies.Now == nil ||
		dependencies.NewID == nil || dependencies.NewSnapshotID == nil {
		return nil, ErrInvalidDependencies
	}
	return &service{dependencies: dependencies, active: make(map[domain.SessionID]activeExecution)}, nil
}

func (service *service) Submit(ctx context.Context, command SubmitCommand) (domain.Session, error) {
	validation, err := service.dependencies.Validator.Validate(ctx, command)
	if err != nil {
		return domain.Session{}, err
	}
	validation, err = validateExecution(validation)
	if err != nil {
		return domain.Session{}, err
	}
	now := service.dependencies.Now().UTC()
	snapshotID, err := service.dependencies.NewSnapshotID()
	if err != nil {
		return domain.Session{}, fmt.Errorf("allocate policy snapshot: %w", err)
	}
	snapshot, err := policy.Resolve(policy.ResolutionInput{
		SnapshotID: snapshotID, IntegrationID: command.IntegrationID,
		OperationName: command.OperationName, DeviceID: command.DeviceID,
		AppKnown: command.AppKnown, AppMode: command.AppMode,
		ManifestPolicy: validation.ManifestPolicy, OperationOverride: command.OperationOverride,
		DeviceKnown: command.DeviceKnown, DeviceMode: command.DeviceMode, ResolvedAt: now,
	})
	if err != nil {
		return domain.Session{}, err
	}
	request, err := domain.NewOperationRequest(domain.OperationRequestInput{
		Actor: command.Actor, IntegrationID: command.IntegrationID,
		OperationName: command.OperationName, DeviceID: command.DeviceID,
		CanonicalParameters:   command.CanonicalParameters,
		ConfigurationSnapshot: command.ConfigurationSnapshot, PolicySnapshot: snapshot.ID,
		Deadline: command.Deadline.UTC(),
	}, now)
	if err != nil {
		return domain.Session{}, err
	}
	if err := service.dependencies.Requests.Create(ctx, request, snapshot); err != nil {
		return domain.Session{}, err
	}
	if err := service.appendAudit(ctx, request.Actor().ID, "operation.policy_decided", "operation_request", request.ID().String(), map[string]any{
		"request_id": request.ID().String(), "actor_id": request.Actor().ID.String(),
		"integration_id": request.IntegrationID().String(), "device_id": request.DeviceID().String(),
		"operation": request.OperationName(), "decision": snapshot.Decision, "reason": snapshot.Reason,
	}); err != nil {
		return domain.Session{}, err
	}
	if snapshot.Decision == policy.Deny {
		return domain.Session{}, ErrPolicyDenied
	}
	created, err := service.dependencies.Sessions.Create(ctx, request, snapshot.Decision)
	if err != nil {
		return domain.Session{}, err
	}
	if created.State == domain.PendingApproval {
		return created, nil
	}
	return service.execute(ctx, request, created, validation)
}

func (service *service) Approve(ctx context.Context, sessionID domain.SessionID, actor domain.Actor) (domain.Session, error) {
	current, request, err := service.pendingRequest(ctx, sessionID)
	if err != nil {
		return domain.Session{}, err
	}
	now := service.dependencies.Now().UTC()
	if !now.Before(request.Deadline()) {
		return current, policy.ErrApprovalExpired
	}
	validation, err := service.dependencies.Validator.Revalidate(ctx, request)
	if err != nil {
		return current, err
	}
	validation, err = validateExecution(validation)
	if err != nil {
		return current, err
	}
	if validation.ManifestPolicy == policy.Denied {
		return current, ErrPolicyDenied
	}
	digest, err := policy.CanonicalRequestDigest(request)
	if err != nil {
		return current, err
	}
	existing, findErr := service.dependencies.Approvals.GetApproval(ctx, request.ID())
	var approval policy.ApprovalRecord
	switch {
	case findErr == nil:
		if existing.Decision != policy.ApprovalGranted || subtle.ConstantTimeCompare(existing.RequestDigest[:], digest[:]) != 1 || !now.Before(existing.ExpiresAt) {
			return current, ErrApprovalAlreadyDecided
		}
		approval = existing
	case !errors.Is(findErr, policy.ErrApprovalNotFound):
		return current, findErr
	default:
		approvalID, idErr := service.dependencies.NewID()
		if idErr != nil {
			return current, fmt.Errorf("allocate approval ID: %w", idErr)
		}
		record, bindErr := policy.BindApproval(approvalID, request, policy.ApprovalGranted, actor.ID, now)
		if bindErr != nil {
			return current, bindErr
		}
		if recordErr := service.dependencies.Approvals.RecordApproval(ctx, record); recordErr != nil {
			return current, recordErr
		}
		approval = record
	}
	if auditErr := service.appendAuditWithID(ctx, approval.ID, actor.ID, "operation.approved", "operation_request", request.ID().String(), map[string]any{
		"request_id": request.ID().String(), "session_id": current.ID.String(),
		"actor_id": actor.ID.String(), "request_digest": fmt.Sprintf("sha256:%x", digest),
	}); auditErr != nil {
		return current, auditErr
	}
	queued, err := service.dependencies.Sessions.Transition(ctx, current.ID, domain.PendingApproval, domain.Queued)
	if err != nil {
		return current, err
	}
	return service.execute(ctx, request, queued, validation)
}

func (service *service) Deny(ctx context.Context, sessionID domain.SessionID, actor domain.Actor, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("denial reason is required")
	}
	current, request, err := service.pendingRequest(ctx, sessionID)
	if err != nil {
		return err
	}
	existing, findErr := service.dependencies.Approvals.GetApproval(ctx, request.ID())
	if findErr == nil {
		if existing.Decision == policy.ApprovalRejected {
			return service.appendDenialAudit(ctx, existing, current, request, actor, reason)
		}
		return ErrApprovalAlreadyDecided
	}
	if !errors.Is(findErr, policy.ErrApprovalNotFound) {
		return findErr
	}
	recordID, err := service.dependencies.NewID()
	if err != nil {
		return fmt.Errorf("allocate denial ID: %w", err)
	}
	record, err := policy.BindApproval(recordID, request, policy.ApprovalRejected, actor.ID, service.dependencies.Now().UTC())
	if err != nil {
		return err
	}
	if err := service.dependencies.Approvals.RecordApproval(ctx, record); err != nil {
		return err
	}
	return service.appendDenialAudit(ctx, record, current, request, actor, reason)
}

func (service *service) appendDenialAudit(ctx context.Context, record policy.ApprovalRecord, current domain.Session, request domain.OperationRequest, actor domain.Actor, reason string) error {
	return service.appendAuditWithID(ctx, record.ID, actor.ID, "operation.denied", "operation_request", request.ID().String(), map[string]any{
		"request_id": request.ID().String(), "session_id": current.ID.String(),
		"actor_id": actor.ID.String(), "reason": strings.TrimSpace(reason),
	})
}

func (service *service) Cancel(ctx context.Context, sessionID domain.SessionID, actor domain.Actor) error {
	current, err := service.dependencies.SessionRepository.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	switch current.State {
	case domain.Completed, domain.Failed, domain.Cancelled, domain.Incomplete:
		return nil
	case domain.PendingApproval:
		return service.Deny(ctx, sessionID, actor, "cancelled before approval")
	}
	service.activeMu.Lock()
	active, ok := service.active[sessionID]
	service.activeMu.Unlock()
	if !ok {
		return ErrExecutionNotActive
	}
	cancelErr := service.dependencies.Supervisor.Cancel(ctx, active.packageID, active.requestID)
	active.cancel()
	return cancelErr
}

func (service *service) Get(ctx context.Context, sessionID domain.SessionID) (domain.Session, error) {
	return service.dependencies.SessionRepository.Get(ctx, sessionID)
}

func (service *service) pendingRequest(ctx context.Context, sessionID domain.SessionID) (domain.Session, domain.OperationRequest, error) {
	current, err := service.dependencies.SessionRepository.Get(ctx, sessionID)
	if err != nil {
		return domain.Session{}, domain.OperationRequest{}, err
	}
	if current.State != domain.PendingApproval {
		return current, domain.OperationRequest{}, ErrApprovalSessionState
	}
	request, err := service.dependencies.Requests.Get(ctx, current.RequestID)
	return current, request, err
}

func (service *service) execute(parent context.Context, request domain.OperationRequest, queued domain.Session, validation Validation) (domain.Session, error) {
	executionContext, cancel := context.WithDeadline(parent, request.Deadline())
	active := activeExecution{requestID: request.ID(), packageID: validation.PackageID, cancel: cancel}
	service.activeMu.Lock()
	service.active[queued.ID] = active
	service.activeMu.Unlock()
	defer func() {
		cancel()
		service.activeMu.Lock()
		delete(service.active, queued.ID)
		service.activeMu.Unlock()
	}()

	if err := service.dependencies.SessionRepository.AcquireLeases(executionContext, queued.ID, validation.Resources, service.dependencies.Now().UTC()); err != nil {
		return queued, err
	}
	running, err := service.dependencies.Sessions.Transition(executionContext, queued.ID, domain.Queued, domain.Running)
	if err != nil {
		releaseErr := service.dependencies.SessionRepository.ReleaseLeases(context.WithoutCancel(parent), queued.ID, service.dependencies.Now().UTC())
		return queued, errors.Join(err, releaseErr)
	}
	collector := &eventCollector{}
	result, executeErr := service.dependencies.Supervisor.Execute(
		supervisor.WithRequestID(executionContext, request.ID()),
		protocol.ExecuteRequest{Type: "execute", Operation: request.OperationName(), Parameters: request.CanonicalParameters()},
		collector,
	)
	terminalState := classifyTerminal(result, executeErr, executionContext)
	if executeErr == nil && terminalState == domain.Completed {
		executeErr = service.persistEvidence(parent, request, running, validation, collector.events)
		if executeErr != nil {
			terminalState = domain.Failed
		}
	}
	if auditErr := service.appendAudit(context.WithoutCancel(parent), request.Actor().ID, "operation.terminal", "session", running.ID.String(), map[string]any{
		"request_id": request.ID().String(), "session_id": running.ID.String(),
		"actor_id": request.Actor().ID.String(), "integration_id": request.IntegrationID().String(),
		"device_id": request.DeviceID().String(), "package_id": validation.PackageID,
		"operation": request.OperationName(), "state": terminalState, "event_count": len(collector.events),
	}); auditErr != nil {
		terminalState = domain.Incomplete
		executeErr = errors.Join(executeErr, auditErr)
	}
	terminal, transitionErr := service.dependencies.Sessions.Transition(
		context.WithoutCancel(parent), running.ID, domain.Running, terminalState,
	)
	return terminal, errors.Join(executeErr, transitionErr)
}

func (service *service) persistEvidence(ctx context.Context, request domain.OperationRequest, running domain.Session, validation Validation, events []protocol.Envelope) error {
	required := make(map[string]bool, len(validation.RequiredArtifacts))
	for _, id := range validation.RequiredArtifacts {
		required[id] = false
	}
	for _, event := range events {
		if event.Kind != protocol.Event || event.Method != protocol.Execute {
			continue
		}
		var declaration protocol.ArtifactDeclaration
		if err := json.Unmarshal(event.Payload, &declaration); err != nil || declaration.Validate() != nil {
			continue
		}
		if err := service.dependencies.Evidence.Persist(context.WithoutCancel(ctx), Evidence{Request: request, Session: running, Event: event}); err != nil {
			return err
		}
		if _, exists := required[declaration.ID]; exists {
			required[declaration.ID] = true
		}
	}
	for id, present := range required {
		if !present {
			return fmt.Errorf("%w: %s", ErrRequiredArtifactMissing, id)
		}
	}
	return nil
}

func (service *service) appendAudit(ctx context.Context, actorID domain.ActorID, action, subjectType, subjectID string, payload any) error {
	id, err := service.dependencies.NewID()
	if err != nil {
		return fmt.Errorf("allocate audit event ID: %w", err)
	}
	return service.appendAuditWithID(ctx, id, actorID, action, subjectType, subjectID, payload)
}

func (service *service) appendAuditWithID(ctx context.Context, id string, actorID domain.ActorID, action, subjectType, subjectID string, payload any) error {
	document, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode audit payload: %w", err)
	}
	event := audit.Event{
		ID: id, ActorID: actorID, Action: action, SubjectType: subjectType,
		SubjectID: subjectID, Payload: document, CreatedAt: service.dependencies.Now().UTC(),
	}
	if err := event.Validate(); err != nil {
		return err
	}
	err = service.dependencies.Audit.Append(ctx, event)
	if errors.Is(err, audit.ErrEventAlreadyAppended) {
		return nil
	}
	return err
}

func validateExecution(validation Validation) (Validation, error) {
	if strings.TrimSpace(validation.PackageID) == "" {
		return Validation{}, fmt.Errorf("%w: package ID is required", ErrInvalidValidation)
	}
	if err := validation.ManifestPolicy.Validate(); err != nil {
		return Validation{}, fmt.Errorf("%w: %v", ErrInvalidValidation, err)
	}
	resources, err := session.NormalizeResourceClaims(validation.Resources)
	if err != nil {
		return Validation{}, fmt.Errorf("%w: %v", ErrInvalidValidation, err)
	}
	seenArtifacts := make(map[string]struct{}, len(validation.RequiredArtifacts))
	for _, id := range validation.RequiredArtifacts {
		if strings.TrimSpace(id) == "" {
			return Validation{}, fmt.Errorf("%w: empty required artifact ID", ErrInvalidValidation)
		}
		if _, exists := seenArtifacts[id]; exists {
			return Validation{}, fmt.Errorf("%w: duplicate required artifact %q", ErrInvalidValidation, id)
		}
		seenArtifacts[id] = struct{}{}
	}
	validation.Resources = resources
	validation.RequiredArtifacts = append([]string(nil), validation.RequiredArtifacts...)
	return validation, nil
}

func classifyTerminal(result protocol.ExecuteResult, executeErr error, executionContext context.Context) domain.SessionState {
	if executeErr != nil {
		if errors.Is(executeErr, context.Canceled) || errors.Is(executionContext.Err(), context.Canceled) {
			return domain.Cancelled
		}
		if errors.Is(executeErr, supervisor.ErrRunnerRemote) {
			return domain.Failed
		}
		return domain.Incomplete
	}
	switch result.Status {
	case "completed":
		return domain.Completed
	case "cancelled":
		return domain.Cancelled
	case "failed":
		return domain.Failed
	default:
		return domain.Incomplete
	}
}

type eventCollector struct {
	mu     sync.Mutex
	events []protocol.Envelope
}

func (collector *eventCollector) Emit(_ context.Context, event protocol.Envelope) error {
	copyValue := event
	copyValue.Payload = append(json.RawMessage(nil), event.Payload...)
	collector.mu.Lock()
	collector.events = append(collector.events, copyValue)
	collector.mu.Unlock()
	return nil
}

var _ Service = (*service)(nil)
