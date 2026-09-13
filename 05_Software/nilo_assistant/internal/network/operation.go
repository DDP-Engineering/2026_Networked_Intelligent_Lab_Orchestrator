package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
)

// ConfirmationWindow is how long a connectivity change waits for an explicit
// confirmation before it is put back.
//
// The administrator who cuts their own access is exactly the person who cannot
// confirm, so the window assumes nobody is coming back and restores on its own.
const ConfirmationWindow = 120 * time.Second

// previewSchema versions the canonical encoding the revision digest is taken
// over, so a stored digest can never be compared against a different shape.
const previewSchema = "nilo.network_preview/1"

var (
	ErrOperationNotFound   = errors.New("network operation not found")
	ErrAlreadyAcknowledged = errors.New("network operation was already acknowledged")
	ErrRevisionMismatch    = errors.New("acknowledgement does not match the previewed revision")
	ErrNotTheActorShown    = errors.New("acknowledgement comes from a different actor than the preview")
	ErrPreviewStale        = errors.New("the state the preview was computed from has changed")
	ErrChangeInProgress    = errors.New("another connectivity change is already in progress")
	ErrRecoveryUnavailable = errors.New("a recovery checkpoint could not be armed")
	ErrNotAwaitingConfirm  = errors.New("network operation is not awaiting confirmation")
	// ErrNotControllable covers a target NILO will not act on: the loopback
	// device, an unmanaged device, or one that is simply not there.
	ErrNotControllable = errors.New("target is not controllable")
	// ErrAuditUnavailable refuses a change nobody could be held to. It is
	// returned before anything is touched, so the appliance is unchanged rather
	// than changed without a record.
	ErrAuditUnavailable = errors.New("this change cannot be recorded, so it was not made")
)

// Action names one managed change. The set is fixed and compiled in: an action
// exists only where an implementation and a permission exist to authorize it.
type Action string

const (
	ActionInterfaceDisable     Action = "interface.disable"
	ActionInterfaceEnable      Action = "interface.enable"
	ActionVPNActivate          Action = "vpn.activate"
	ActionVPNDeactivate        Action = "vpn.deactivate"
	ActionWireGuardInstall     Action = "wireguard.install"
	ActionProfileImport        Action = "wireguard.profile.import"
	ActionProfileRemove        Action = "wireguard.profile.remove"
	ActionAutostartSelect      Action = "wireguard.autostart"
	ActionProfileRename        Action = "wireguard.profile.rename"
	ActionInterfaceSettings    Action = "interface.settings"
	ActionWiFiJoin             Action = "wifi.join"
	ActionDeviceNetworkCreate  Action = "device_network.create"
	ActionDeviceNetworkUpdate  Action = "device_network.update"
	ActionDeviceNetworkEnable  Action = "device_network.enable"
	ActionDeviceNetworkDisable Action = "device_network.disable"
	ActionDeviceNetworkDelete  Action = "device_network.delete"
)

// connectivityActions are the changes that can cut the caller's own path to the
// appliance. Only these get a confirmation window; making a package install
// wait for a confirmation would be ceremony, not safety.
var connectivityActions = map[Action]bool{
	ActionInterfaceDisable:  true,
	ActionInterfaceEnable:   true,
	ActionVPNActivate:       true,
	ActionVPNDeactivate:     true,
	ActionInterfaceSettings: true,
	ActionWiFiJoin:          true,
	// Creating a device network saves a disabled decision and touches no
	// forwarding; everything after that changes or removes exposure.
	ActionDeviceNetworkUpdate:  true,
	ActionDeviceNetworkEnable:  true,
	ActionDeviceNetworkDisable: true,
	ActionDeviceNetworkDelete:  true,
}

var knownActions = map[Action]bool{
	ActionInterfaceDisable: true, ActionInterfaceEnable: true,
	ActionVPNActivate: true, ActionVPNDeactivate: true,
	ActionWireGuardInstall: true, ActionProfileImport: true,
	ActionProfileRemove: true, ActionAutostartSelect: true,
	ActionProfileRename: true, ActionInterfaceSettings: true,
	ActionWiFiJoin: true, ActionDeviceNetworkCreate: true,
	ActionDeviceNetworkUpdate: true, ActionDeviceNetworkEnable: true,
	ActionDeviceNetworkDisable: true, ActionDeviceNetworkDelete: true,
}

// deviceNetworkActions are the actions the device-network manager answers for.
var deviceNetworkActions = map[Action]bool{
	ActionDeviceNetworkCreate: true, ActionDeviceNetworkUpdate: true, ActionDeviceNetworkEnable: true,
	ActionDeviceNetworkDisable: true, ActionDeviceNetworkDelete: true,
}

// ChangesConnectivity reports whether this action needs a confirmation window.
func (action Action) ChangesConnectivity() bool { return connectivityActions[action] }

// OperationState is where one managed change has got to.
type OperationState string

const (
	StatePreviewed            OperationState = "previewed"
	StateApplying             OperationState = "applying"
	StateAwaitingConfirmation OperationState = "awaiting_confirmation"
	StateConfirmed            OperationState = "confirmed"
	StateFailed               OperationState = "failed"
	StateRollingBack          OperationState = "rolling_back"
	StateRolledBack           OperationState = "rolled_back"
	StateRollbackFailed       OperationState = "rollback_failed"
)

// Checkpoint identifies armed recovery state. Metadata is public, changer-owned
// state that a restarted process needs to reconstitute the recovery boundary.
// This package only persists and carries it back and forth.
type Checkpoint struct {
	ID       string
	Metadata json.RawMessage
}

// ChangeRecoveryPreparer binds privileged recovery to the exact validated
// change. Legacy simulation changers can still use ArmRecovery.
type ChangeRecoveryPreparer interface {
	ArmChange(context.Context, string, Change, time.Duration) (Checkpoint, error)
}

// ChangeRecoveryPlanner captures changer-owned public rollback metadata before
// the first durable applying record. CancelChangeRecovery releases any
// process-local reservation when that first write does not succeed.
type ChangeRecoveryPlanner interface {
	PlanChangeRecovery(context.Context, string, Change) (json.RawMessage, error)
	CancelChangeRecovery(string)
}

// ChangeRecoveryAdopter restores changer-owned recovery state around an
// unfinished operation and reconciles it after the helper reports a terminal
// outcome. Changers without durable in-process state need not implement it.
type ChangeRecoveryAdopter interface {
	AdoptChangeRecovery(context.Context, string, Change, Checkpoint) error
	ReconcileChangeRecovery(context.Context, string, Change, Checkpoint, OperationState) error
}

// UncheckpointedChangeReconciler resolves deliberately checkpoint-less
// operations whose side effect is itself durable. The boolean reports whether
// the changer recognizes the action.
type UncheckpointedChangeReconciler interface {
	ReconcileUncheckpointedChange(context.Context, string, Change, Checkpoint) (OperationState, bool, error)
}

// ConnectivityChanger applies changes under recoverable state. The simulated
// profile supplies one that records and restores nothing real; the appliance
// profile supplies one backed by the privileged helper.
type ConnectivityChanger interface {
	// ArmRecovery captures enough to put the appliance back, and arranges for
	// that to happen on its own if nothing confirms within window.
	ArmRecovery(ctx context.Context, operationID string, window time.Duration) (Checkpoint, error)
	Apply(ctx context.Context, action Action, target string) error
	// Restore puts the appliance back and reports whether the previous state
	// was actually observed afterwards. A restoration nobody verified is not a
	// restoration.
	Restore(ctx context.Context, checkpoint Checkpoint) (observed bool, err error)
	// Discard releases recovery state after a confirmation.
	Discard(ctx context.Context, checkpoint Checkpoint) error
}

// StateObserver reports the state a preview is computed against. The observed
// values go into the revision digest, which is what makes a preview go stale
// when the appliance moves underneath it.
type StateObserver interface {
	Observe(ctx context.Context, action Action, target string) (map[string]string, error)
}

// Operation is one managed change and everything a caller may learn about it.
// It carries no secret and no parameter value that could hold one.
type Operation struct {
	ID       string
	Action   Action
	Target   string
	ActorID  string
	State    OperationState
	Revision string

	CreatedAt       time.Time
	ConfirmDeadline time.Time
	Failure         string
	Progress        string

	// observed is the state the revision was computed from. It stays unexported
	// so it cannot leak through a response body.
	observed   map[string]string
	checkpoint Checkpoint
	change     Change
}

// actorID is the acting identity in the form an audit record needs. An actor
// this appliance cannot parse yields the zero identity rather than a guess; the
// audit repository refuses that, which is the right place to notice it.
func (operation *Operation) actorID() domain.ActorID {
	parsed, err := domain.ParseActorID(operation.ActorID)
	if err != nil {
		return domain.ActorID{}
	}
	return parsed
}

// AuditJournal is where the accountable stages of a change are recorded. The
// manager writes them itself rather than leaving it to the HTTP boundary,
// because the expiry ticker and the startup reconciler reach this same
// lifecycle and neither of them is a request.
type AuditJournal interface {
	Record(ctx context.Context, record audit.Record) error
}

// OperationManagerOptions are the manager's collaborators.
type OperationManagerOptions struct {
	Changer  ConnectivityChanger
	Observer StateObserver
	Now      func() time.Time
	NewID    func() string

	// Store persists the lifecycle so a change in flight survives a page
	// reload, a restart or a reboot. It is optional: the simulation profile and
	// the unit tests build a manager with no database, and requiring one would
	// turn durability into a construction failure.
	Store OperationStore
	// Audit records the stages. It is optional for the same reason; where it is
	// present, a change that cannot be recorded is refused rather than made.
	Audit AuditJournal
	// Stager keeps imported key material out of this process. Where there is
	// none, an import's secrets are parsed and dropped rather than kept, which
	// is what the simulation profile wants: it manages no real tunnel, so
	// storing a real key on a developer's machine would be a liability with no
	// purpose.
	Stager SecretStager
	// Recovery reads the privileged helper's own record of the last change,
	// which is what a restarted core reconciles its adopted operations against.
	// It is optional: the simulation has no helper and keeps its adopted
	// operations as they were recorded.
	Recovery RecoveryObserver
}

// RecoveryOutcome is the helper's own account of the last change it made.
type RecoveryOutcome struct {
	OperationID string
	// Phase is the helper's recovery phase: intent, prepared, applied,
	// confirmed, restored or recovery_failed.
	Phase string
}

// RecoveryObserver reads the helper's record. The record is the helper's, not
// this process's, so a restart learns what actually happened rather than
// assuming success or failure.
type RecoveryObserver interface {
	LastRecovery(ctx context.Context) (RecoveryOutcome, bool, error)
}

// componentOf names the product function an action belongs to, which is what
// the Activity page filters on. It is a function scope, not a Go package.
func componentOf(action Action) string {
	switch action {
	case ActionInterfaceDisable, ActionInterfaceEnable, ActionInterfaceSettings:
		return "network.interfaces"
	case ActionWiFiJoin:
		return "network.wifi"
	case ActionDeviceNetworkCreate, ActionDeviceNetworkUpdate, ActionDeviceNetworkEnable, ActionDeviceNetworkDisable, ActionDeviceNetworkDelete:
		return "network.device_networks"
	default:
		return "network.wireguard"
	}
}

// subjectTypeOf names what an action acts on, so a reader can filter history by
// the kind of thing rather than by parsing the action name.
func subjectTypeOf(action Action) string {
	switch action {
	case ActionInterfaceDisable, ActionInterfaceEnable, ActionInterfaceSettings, ActionWiFiJoin:
		return "interface"
	case ActionProfileImport, ActionProfileRemove, ActionAutostartSelect, ActionProfileRename:
		return "vpn_profile"
	case ActionDeviceNetworkCreate, ActionDeviceNetworkUpdate, ActionDeviceNetworkEnable, ActionDeviceNetworkDisable, ActionDeviceNetworkDelete:
		return "device_network"
	case ActionWireGuardInstall:
		return "appliance"
	default:
		return "vpn_tunnel"
	}
}

// OperationManager owns the lifecycle every managed network change runs
// through: preview, acknowledgement bound to that preview, application under a
// recovery checkpoint, and a confirmation that is a separate act.
type OperationManager struct {
	options OperationManagerOptions

	mutex sync.Mutex
	// changeMutex reserves execution before any observation or recovery I/O.
	// Reads use mutex independently so progress remains observable during I/O.
	changeMutex sync.Mutex
	operations  map[string]*Operation
	// inFlight is the one connectivity operation currently applying or waiting
	// to be confirmed. Two at once cannot be reasoned about, so there is at
	// most one.
	inFlight string
}

func NewOperationManager(options OperationManagerOptions) (*OperationManager, error) {
	switch {
	case options.Changer == nil:
		return nil, errors.New("operation manager requires a connectivity changer")
	case options.Observer == nil:
		return nil, errors.New("operation manager requires a state observer")
	case options.Now == nil:
		return nil, errors.New("operation manager requires a clock")
	case options.NewID == nil:
		return nil, errors.New("operation manager requires an identity source")
	}
	return &OperationManager{options: options, operations: make(map[string]*Operation)}, nil
}

// Preview computes what a change would do and returns it for acknowledgement.
// It changes nothing.
func (manager *OperationManager) Preview(
	ctx context.Context, actorID string, action Action, target string,
) (Operation, error) {
	return manager.PreviewChange(ctx, actorID, Change{Action: action, Target: target})
}

// PreviewChange keeps transient input private and binds every parameter into
// the revision. Only a successful acknowledgement passes it to the controller.
func (manager *OperationManager) PreviewChange(ctx context.Context, actorID string, change Change) (Operation, error) {
	action, target := change.Action, change.Target
	if actorID == "" {
		return Operation{}, errors.New("a preview requires the actor it is shown to")
	}
	if !knownActions[action] {
		return Operation{}, fmt.Errorf("unknown network action %q", action)
	}
	if target == "" {
		return Operation{}, errors.New("a preview requires a target")
	}

	// The uploaded file is consumed here and nowhere else. What continues is the
	// safe public configuration and, where key material was staged, an opaque
	// handle. This happens before the revision is computed, so the digest the
	// administrator acknowledges is taken over the same shape the
	// acknowledgement recomputes.
	change, err := manager.stageImport(ctx, change)
	if err != nil {
		return Operation{}, err
	}

	observed, err := manager.observe(ctx, change)
	if err != nil {
		return Operation{}, fmt.Errorf("observe %s on %s: %w", action, target, err)
	}

	operationID := manager.options.NewID()
	change.operationID = operationID
	operation := &Operation{
		ID: operationID, Action: action, Target: target,
		ActorID: actorID, State: StatePreviewed,
		CreatedAt: manager.options.Now().UTC(), observed: observed,
		change: change,
	}
	revision, err := revisionOf(operation)
	if err != nil {
		return Operation{}, err
	}
	operation.Revision = revision

	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	manager.operations[operation.ID] = operation
	return *operation, nil
}

// Acknowledge runs a previewed change, but only for the exact target, action
// and revision that were shown to this actor, and only once.
func (manager *OperationManager) Acknowledge(
	ctx context.Context, actorID, operationID, revision string,
) (Operation, error) {
	if !manager.changeMutex.TryLock() {
		return Operation{}, ErrChangeInProgress
	}
	defer manager.changeMutex.Unlock()
	manager.mutex.Lock()
	operation, found := manager.operations[operationID]
	if !found {
		manager.mutex.Unlock()
		return Operation{}, ErrOperationNotFound
	}
	if operation.State != StatePreviewed {
		manager.mutex.Unlock()
		return Operation{}, ErrAlreadyAcknowledged
	}
	if operation.ActorID != actorID {
		manager.mutex.Unlock()
		return Operation{}, ErrNotTheActorShown
	}
	if revision != operation.Revision {
		manager.mutex.Unlock()
		return Operation{}, ErrRevisionMismatch
	}
	connectivity := operation.Action.ChangesConnectivity()
	if manager.inFlight != "" {
		manager.mutex.Unlock()
		return Operation{}, ErrChangeInProgress
	}
	manager.mutex.Unlock()

	// Recompute the preview against the world as it is now. If the digest moved,
	// the administrator agreed to something that is no longer true.
	current, err := manager.observe(ctx, operation.change)
	if err != nil {
		return Operation{}, fmt.Errorf("re-observe %s on %s: %w", operation.Action, operation.Target, err)
	}
	fresh := *operation
	fresh.observed = current
	freshRevision, err := revisionOf(&fresh)
	if err != nil {
		return Operation{}, err
	}
	if freshRevision != operation.Revision {
		return Operation{}, ErrPreviewStale
	}

	// The accepted intent is written down before anything happens, so a change
	// that cannot be recorded is a change that was not made. Recording first
	// also means a crash between here and the apply leaves an operation a
	// restart can find, rather than a silent alteration.
	if err := manager.journal(ctx, operation, audit.OutcomeRequested, audit.ActorKindUser, ""); err != nil {
		return Operation{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	planner, _ := manager.options.Changer.(ChangeRecoveryPlanner)
	if planner != nil {
		metadata, err := planner.PlanChangeRecovery(ctx, operation.ID, operation.change)
		if err != nil {
			return Operation{}, fmt.Errorf("%w: plan recovery: %w", ErrRecoveryUnavailable, err)
		}
		operation.checkpoint.Metadata = append(json.RawMessage(nil), metadata...)
	}
	if err := manager.persist(ctx, operation, StateApplying); err != nil {
		if planner != nil {
			planner.CancelChangeRecovery(operation.ID)
			operation.checkpoint = Checkpoint{}
		}
		return Operation{}, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}

	return manager.apply(ctx, operation, connectivity)
}

// PendingIDs names the operation whose evidence retention must keep until it
// is settled: the one applying or awaiting confirmation.
func (manager *OperationManager) PendingIDs(context.Context) ([]string, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.inFlight == "" {
		return nil, nil
	}
	return []string{manager.inFlight}, nil
}

// journal records one stage of one change. A failure is returned to the caller,
// which decides whether it can proceed: before a side effect it cannot, and
// after one it must.
func (manager *OperationManager) journal(
	ctx context.Context, operation *Operation, outcome audit.Outcome, actorKind audit.ActorKind, failure string,
) error {
	if manager.options.Audit == nil {
		return nil
	}
	severity := audit.SeverityInfo
	details := map[string]string{"state": string(operation.State)}
	if failure != "" {
		severity = audit.SeverityError
		details["failure"] = failure
	}
	if outcome == audit.OutcomeRecovered {
		severity = audit.SeverityWarning
	}
	return manager.options.Audit.Record(ctx, audit.Record{
		Category:    audit.CategoryAudit,
		Component:   componentOf(operation.Action),
		Action:      string(operation.Action),
		SubjectType: subjectTypeOf(operation.Action),
		SubjectID:   operation.Target,
		ActorKind:   actorKind,
		ActorID:     operation.actorID(),
		Outcome:     outcome,
		Severity:    severity,
		OperationID: operation.ID,
		Details:     details,
	})
}

// persist writes the operation down at the state it is about to be in. It
// stores only changer-owned public recovery metadata; transient change
// parameters, which may carry an imported profile, never reach it.
func (manager *OperationManager) persist(ctx context.Context, operation *Operation, state OperationState) error {
	if manager.options.Store == nil {
		return nil
	}
	now := manager.options.Now().UTC()
	record := OperationRecord{
		ID: operation.ID, Action: string(operation.Action), Target: operation.Target,
		ActorID: operation.ActorID, State: string(state), Revision: operation.Revision,
		CheckpointID: operation.checkpoint.ID, Failure: operation.Failure,
		Parameters: append(json.RawMessage(nil), operation.checkpoint.Metadata...),
		CreatedAt:  operation.CreatedAt, UpdatedAt: now,
	}
	if state == StateAwaitingConfirmation {
		record.ConfirmDeadline = operation.ConfirmDeadline
	}
	return manager.options.Store.Save(ctx, record)
}

// settle records a stage that has already happened. Its failure cannot undo the
// effect, so it is reported to the caller's error sink rather than returned:
// refusing to finish a rollback because the log is full would strand the
// appliance in the state the rollback exists to leave.
func (manager *OperationManager) settle(
	ctx context.Context, operation *Operation, state OperationState,
	outcome audit.Outcome, actorKind audit.ActorKind, failure string,
) {
	_ = manager.persist(ctx, operation, state)
	_ = manager.journal(ctx, operation, outcome, actorKind, failure)
}

// apply arms recovery, makes the change, and either finishes it or opens the
// confirmation window.
func (manager *OperationManager) apply(
	ctx context.Context, operation *Operation, connectivity bool,
) (Operation, error) {
	if installer, ok := manager.options.Changer.(InstallationController); ok && operation.Action == ActionWireGuardInstall {
		return manager.startInstall(ctx, installer, operation)
	}
	checkpoint := operation.checkpoint
	deadline := manager.options.Now().UTC().Add(ConfirmationWindow)
	if connectivity {
		// Recovery is armed before anything is touched. If it cannot be armed,
		// the change does not happen at all: an appliance that cannot be put
		// back must not be altered.
		var armed Checkpoint
		var err error
		if preparer, ok := manager.options.Changer.(ChangeRecoveryPreparer); ok {
			armed, err = preparer.ArmChange(ctx, operation.ID, operation.change, ConfirmationWindow)
		} else {
			armed, err = manager.options.Changer.ArmRecovery(ctx, operation.ID, ConfirmationWindow)
		}
		if err != nil {
			if planner, ok := manager.options.Changer.(ChangeRecoveryPlanner); ok {
				planner.CancelChangeRecovery(operation.ID)
			}
			manager.mutex.Lock()
			operation.State = StateFailed
			operation.Failure = "recovery could not be prepared"
			manager.mutex.Unlock()
			settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			manager.settle(settleCtx, operation, StateFailed, audit.OutcomeFailed, audit.ActorKindUser, operation.Failure)
			cancel()
			return Operation{}, fmt.Errorf("%w: %w", ErrRecoveryUnavailable, err)
		}
		checkpoint = armed
		operation.checkpoint = checkpoint
		// The helper checkpoint and any changer-owned recovery metadata must be
		// durable before the first mutation. The earlier applying record captures
		// accepted intent; this second write binds that intent to its recovery.
		if err := manager.persist(ctx, operation, StateApplying); err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			observed, cleanupErr := manager.options.Changer.Restore(cleanupCtx, checkpoint)
			manager.mutex.Lock()
			operation.State = StateFailed
			operation.Failure = "armed recovery could not be recorded"
			if cleanupErr != nil || !observed {
				operation.State = StateRollbackFailed
				operation.Failure = "armed recovery could not be recorded and the previous state could not be verified"
				manager.inFlight = operation.ID
			}
			failedState, failure := operation.State, operation.Failure
			manager.mutex.Unlock()
			manager.settle(cleanupCtx, operation, failedState, audit.OutcomeFailed, audit.ActorKindUser, failure)
			cancel()
			if cleanupErr != nil {
				return Operation{}, fmt.Errorf("%w: persist armed recovery: %v; restore armed recovery: %w", ErrRecoveryUnavailable, err, cleanupErr)
			}
			if !observed {
				return Operation{}, fmt.Errorf("%w: persist armed recovery: %v; previous state was not observed", ErrRecoveryUnavailable, err)
			}
			operation.checkpoint = Checkpoint{}
			return Operation{}, fmt.Errorf("%w: persist armed recovery: %v", ErrAuditUnavailable, err)
		}
	}

	manager.mutex.Lock()
	operation.State = StateApplying
	operation.checkpoint = checkpoint
	if connectivity {
		manager.inFlight = operation.ID
	}
	manager.mutex.Unlock()

	var applyErr error
	importer, importing := manager.options.Changer.(ProfileImporter)
	switch {
	case importing && operation.Action == ActionProfileImport:
		_, applyErr = importer.ImportProfile(ctx, operation.change)
	default:
		if controller, ok := manager.options.Changer.(ChangeController); ok {
			applyErr = controller.ApplyChange(ctx, operation.change)
		} else {
			applyErr = manager.options.Changer.Apply(ctx, operation.Action, operation.Target)
		}
	}
	if operation.Action == ActionProfileImport {
		manager.releaseImport(ctx, operation, importing && applyErr == nil)
	}
	if err := applyErr; err != nil {
		if !connectivity {
			if planner, ok := manager.options.Changer.(ChangeRecoveryPlanner); ok {
				planner.CancelChangeRecovery(operation.ID)
			}
		}
		observed, restoreErr := true, error(nil)
		if connectivity {
			// The request may already be disconnected. Recovery has its own
			// bounded context and must not inherit a cancelled HTTP request.
			recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			observed, restoreErr = manager.options.Changer.Restore(recoveryCtx, checkpoint)
			cancel()
		}
		manager.mutex.Lock()
		operation.State = StateFailed
		operation.Failure = err.Error()
		if restoreErr != nil || !observed {
			operation.State = StateRollbackFailed
			operation.Failure = "application failed and previous connectivity could not be verified"
		} else {
			manager.inFlight = ""
		}
		failedState, failure := operation.State, operation.Failure
		manager.mutex.Unlock()
		manager.settle(ctx, operation, failedState, audit.OutcomeFailed, audit.ActorKindUser, failure)
		return Operation{}, fmt.Errorf("apply %s on %s: %w", operation.Action, operation.Target, err)
	}

	manager.mutex.Lock()
	if !connectivity {
		if planner, ok := manager.options.Changer.(ChangeRecoveryPlanner); ok {
			planner.CancelChangeRecovery(operation.ID)
		}
		operation.State = StateConfirmed
		result := *operation
		manager.mutex.Unlock()
		// A change that cannot cut the caller off needs no window, so applying
		// it is the whole of it: one applied record, no separate confirmation.
		manager.settle(ctx, operation, StateConfirmed, audit.OutcomeApplied, audit.ActorKindUser, "")
		return result, nil
	}
	operation.State = StateAwaitingConfirmation
	operation.ConfirmDeadline = deadline
	result := *operation
	manager.mutex.Unlock()
	manager.settle(ctx, operation, StateAwaitingConfirmation, audit.OutcomeApplied, audit.ActorKindUser, "")
	return result, nil
}

// StagePassword puts a wireless password beyond this process's reach before a
// join is even previewed, and returns the handle the change carries instead.
// Without a stager there is nothing that could use the password, so nothing is
// kept and the handle is empty.
func (manager *OperationManager) StagePassword(ctx context.Context, password string) (string, error) {
	if password == "" || manager.options.Stager == nil {
		return "", nil
	}
	handle, err := manager.options.Stager.StageSecrets(ctx, Secrets{Password: password})
	if err != nil {
		return "", fmt.Errorf("stage the wireless password: %w", err)
	}
	return handle, nil
}

// stageImport parses an uploaded profile, stages its key material where this
// process cannot read it, and returns the change with the raw text removed.
//
// Without a stager the secrets are parsed and dropped. That is deliberate
// rather than a gap: a profile is only worth keeping a key for if something can
// actually raise the tunnel, and nothing that cannot should be holding one.
func (manager *OperationManager) stageImport(ctx context.Context, change Change) (Change, error) {
	if change.Action != ActionProfileImport || change.ProfileText == "" {
		return change, nil
	}
	profile, secrets, err := ParseProfile(change.Target, []byte(change.ProfileText))
	if err != nil {
		return Change{}, err
	}
	change.ProfileText = ""
	change.Profile = &profile
	if manager.options.Stager == nil {
		return change, nil
	}
	handle, err := manager.options.Stager.StageSecrets(ctx, secrets)
	if err != nil {
		return Change{}, fmt.Errorf("stage imported key material: %w", err)
	}
	change.SecretHandle = handle
	return change, nil
}

// releaseImport forgets the staged handle once an import has settled.
//
// The importer binds the material itself, atomically with creating the
// profile, because the privileged side needs the key to create the connection
// and must never hand it back; after a success there is nothing left to do
// here. After a failure, or where no importer could have bound it, the staged
// copy is discarded so an abandoned import cannot be replayed and leaves no key
// on disk. Either way the handle stops being live.
func (manager *OperationManager) releaseImport(ctx context.Context, operation *Operation, bound bool) {
	manager.mutex.Lock()
	handle := operation.change.SecretHandle
	operation.change.SecretHandle = ""
	manager.mutex.Unlock()
	if handle == "" || manager.options.Stager == nil || bound {
		return
	}
	_ = manager.options.Stager.DropSecrets(ctx, handle)
}

func (manager *OperationManager) observe(ctx context.Context, change Change) (map[string]string, error) {
	// Recovery correlation belongs to apply, not to the inspected parameters.
	change.operationID = ""
	var observed map[string]string
	var err error
	if controller, ok := manager.options.Changer.(ChangeController); ok {
		observed, err = controller.Inspect(ctx, change)
	} else {
		observed, err = manager.options.Observer.Observe(ctx, change.Action, change.Target)
	}
	if err != nil {
		return nil, err
	}
	if observed == nil {
		observed = map[string]string{}
	}
	// Hash transient input rather than retaining it in public operation state.
	encoded, _ := json.Marshal(change)
	digest := sha256.Sum256(encoded)
	observed["parameters_revision"] = hex.EncodeToString(digest[:])
	return observed, nil
}

// Confirm keeps a change that is inside its window.
func (manager *OperationManager) Confirm(
	ctx context.Context, actorID, operationID string,
) (Operation, error) {
	if !manager.changeMutex.TryLock() {
		return Operation{}, ErrChangeInProgress
	}
	defer manager.changeMutex.Unlock()
	manager.mutex.Lock()
	operation, found := manager.operations[operationID]
	if !found {
		manager.mutex.Unlock()
		return Operation{}, ErrOperationNotFound
	}
	if operation.State != StateAwaitingConfirmation {
		manager.mutex.Unlock()
		return Operation{}, ErrNotAwaitingConfirm
	}
	if operation.ActorID != actorID {
		manager.mutex.Unlock()
		return Operation{}, ErrNotTheActorShown
	}
	if !manager.options.Now().UTC().Before(operation.ConfirmDeadline) {
		manager.mutex.Unlock()
		return Operation{}, ErrNotAwaitingConfirm
	}
	checkpoint := operation.checkpoint
	manager.mutex.Unlock()
	if err := manager.options.Changer.Discard(ctx, checkpoint); err != nil {
		return Operation{}, fmt.Errorf("release recovery checkpoint: %w", err)
	}
	manager.mutex.Lock()
	operation.State = StateConfirmed
	operation.ConfirmDeadline = time.Time{}
	manager.inFlight = ""
	result := *operation
	manager.mutex.Unlock()
	manager.settle(ctx, operation, StateConfirmed, audit.OutcomeConfirmed, audit.ActorKindUser, "")

	return result, nil
}

// Rollback puts a change back before its window runs out, at the
// administrator's request.
func (manager *OperationManager) Rollback(
	ctx context.Context, actorID, operationID string,
) (Operation, error) {
	if !manager.changeMutex.TryLock() {
		return Operation{}, ErrChangeInProgress
	}
	defer manager.changeMutex.Unlock()
	manager.mutex.Lock()
	operation, found := manager.operations[operationID]
	if !found {
		manager.mutex.Unlock()
		return Operation{}, ErrOperationNotFound
	}
	if operation.State != StateAwaitingConfirmation {
		manager.mutex.Unlock()
		return Operation{}, ErrNotAwaitingConfirm
	}
	if operation.ActorID != actorID {
		manager.mutex.Unlock()
		return Operation{}, ErrNotTheActorShown
	}
	manager.mutex.Unlock()

	// An explicit rollback is the administrator acting; an expiry is the
	// appliance acting. The two are not the same event and are not recorded as
	// if they were.
	manager.restore(ctx, operation, audit.ActorKindUser)
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	return *operation, nil
}

// ExpireDue restores every connectivity change whose window has run out. It is
// driven by the runtime's ticker, and it is what makes the guarantee hold when
// the browser, the session or the administrator is simply gone.
func (manager *OperationManager) ExpireDue(ctx context.Context) {
	if !manager.changeMutex.TryLock() {
		return
	}
	defer manager.changeMutex.Unlock()
	now := manager.options.Now().UTC()
	manager.pollInstall(ctx)

	manager.mutex.Lock()
	var due []*Operation
	for _, operation := range manager.operations {
		if operation.State == StateAwaitingConfirmation && !now.Before(operation.ConfirmDeadline) {
			due = append(due, operation)
		}
	}
	manager.mutex.Unlock()

	sort.Slice(due, func(first, second int) bool { return due[first].ID < due[second].ID })
	for _, operation := range due {
		manager.restore(ctx, operation, audit.ActorKindSystem)
	}
}

// restore rolls one operation back and records whether the previous state was
// actually seen afterwards.
func (manager *OperationManager) restore(ctx context.Context, operation *Operation, actorKind audit.ActorKind) {
	manager.mutex.Lock()
	if operation.State != StateAwaitingConfirmation {
		manager.mutex.Unlock()
		return
	}
	operation.State = StateRollingBack
	checkpoint := operation.checkpoint
	manager.mutex.Unlock()

	observed, err := manager.options.Changer.Restore(ctx, checkpoint)

	manager.mutex.Lock()
	switch {
	case err != nil:
		operation.State = StateRollbackFailed
		operation.Failure = fmt.Sprintf("restoring the previous state failed: %v", err)
	case !observed:
		// The restore was requested and did not error, but the previous state
		// was not seen. Reporting this as restored would be the worst lie this
		// service could tell.
		operation.State = StateRollbackFailed
		operation.Failure = "the previous connection, routing and DNS state was not observed after restoring"
	default:
		operation.State = StateRolledBack
		operation.ConfirmDeadline = time.Time{}
		manager.inFlight = ""
	}
	state, failure := operation.State, operation.Failure
	manager.mutex.Unlock()

	outcome := audit.OutcomeRecovered
	if state == StateRollbackFailed {
		outcome = audit.OutcomeFailed
	}
	manager.settle(ctx, operation, state, outcome, actorKind, failure)
}

// Adopt loads the operations a previous run left unfinished, so a restarted
// appliance can still answer for a change that was in flight and can still put
// it back when its window runs out.
//
// The change's transient parameters are not restored, because they were never
// stored. Public checkpoint metadata restores changer-owned rollback state;
// the action and target remain enough to expire, roll back and report.
func (manager *OperationManager) Adopt(ctx context.Context) error {
	if manager.options.Store == nil {
		return nil
	}
	records, err := manager.options.Store.Unfinished(ctx)
	if err != nil {
		return fmt.Errorf("read unfinished network operations: %w", err)
	}
	for _, record := range records {
		manager.mutex.Lock()
		if _, known := manager.operations[record.ID]; known {
			manager.mutex.Unlock()
			continue
		}
		manager.mutex.Unlock()
		checkpoint := Checkpoint{
			ID:       record.CheckpointID,
			Metadata: append(json.RawMessage(nil), record.Parameters...),
		}
		change := Change{Action: Action(record.Action), Target: record.Target}
		if adopter, ok := manager.options.Changer.(ChangeRecoveryAdopter); ok && (checkpoint.ID != "" || len(checkpoint.Metadata) != 0) {
			if err := adopter.AdoptChangeRecovery(ctx, record.ID, change, checkpoint); err != nil {
				return fmt.Errorf("adopt recovery for network operation %s: %w", record.ID, err)
			}
		}
		operation := &Operation{
			ID: record.ID, Action: Action(record.Action), Target: record.Target,
			ActorID: record.ActorID, State: OperationState(record.State),
			Revision: record.Revision, CreatedAt: record.CreatedAt,
			ConfirmDeadline: record.ConfirmDeadline, Failure: record.Failure,
			checkpoint: checkpoint,
			change:     change,
		}
		manager.mutex.Lock()
		manager.operations[record.ID] = operation
		if operation.Action.ChangesConnectivity() && manager.inFlight == "" {
			switch operation.State {
			case StateApplying, StateAwaitingConfirmation, StateRollingBack, StateRollbackFailed:
				manager.inFlight = operation.ID
			}
		}
		manager.mutex.Unlock()
	}
	return nil
}

// Reconcile resolves every adopted operation that was still in flight against
// the helper's own record of what it did, recording the observed outcome once
// as the system. A change the helper still holds inside its window stays
// awaiting; one the helper never received was never applied.
func (manager *OperationManager) Reconcile(ctx context.Context) error {
	if manager.options.Recovery == nil {
		return nil
	}
	if !manager.changeMutex.TryLock() {
		return nil
	}
	defer manager.changeMutex.Unlock()
	outcome, present, err := manager.options.Recovery.LastRecovery(ctx)
	if err != nil {
		return fmt.Errorf("read the helper's recovery record: %w", err)
	}
	manager.mutex.Lock()
	var unfinished []*Operation
	for _, operation := range manager.operations {
		switch operation.State {
		case StateApplying, StateAwaitingConfirmation, StateRollingBack, StateRollbackFailed:
			unfinished = append(unfinished, operation)
		}
	}
	manager.mutex.Unlock()
	sort.Slice(unfinished, func(first, second int) bool { return unfinished[first].CreatedAt.Before(unfinished[second].CreatedAt) })

	for _, operation := range unfinished {
		state, result, failure := OperationState(""), audit.Outcome(""), ""
		if operation.State == StateRollbackFailed {
			state, result, failure = resolveUnverifiedRecoveryAgainstHelper(operation, outcome, present)
			if state == "" {
				continue
			}
		}
		uncheckpointed := operation.checkpoint.ID == ""
		if state == "" && present && outcome.OperationID == operation.ID {
			// The helper may have armed recovery before the core could save its
			// checkpoint ID. Its matching record still owns that recovery.
			state, result, failure = resolveAgainstHelper(operation, outcome, present)
			if state == "" {
				continue
			}
		} else if state == "" && uncheckpointed {
			if reconciler, ok := manager.options.Changer.(UncheckpointedChangeReconciler); ok {
				resolved, handled, reconcileErr := reconciler.ReconcileUncheckpointedChange(ctx, operation.ID, operation.change, operation.checkpoint)
				if reconcileErr != nil {
					return fmt.Errorf("reconcile checkpoint-less network operation %s: %w", operation.ID, reconcileErr)
				}
				if handled {
					state = resolved
				}
			}
			if state == "" {
				state = StateFailed
			}
			if state == StateConfirmed {
				result = audit.OutcomeApplied
			} else {
				result = audit.OutcomeFailed
				failure = "the change had no recovery checkpoint after restart, so it is not applied"
			}
		} else if state == "" {
			state, result, failure = resolveAgainstHelper(operation, outcome, present)
		}
		if state == "" {
			continue
		}
		if adopter, ok := manager.options.Changer.(ChangeRecoveryAdopter); ok && (operation.checkpoint.ID != "" || len(operation.checkpoint.Metadata) != 0) {
			if err := adopter.ReconcileChangeRecovery(ctx, operation.ID, operation.change, operation.checkpoint, state); err != nil {
				return fmt.Errorf("reconcile recovery for network operation %s: %w", operation.ID, err)
			}
		}
		manager.mutex.Lock()
		operation.State = state
		operation.Failure = failure
		if state != StateRollbackFailed {
			operation.ConfirmDeadline = time.Time{}
			if manager.inFlight == operation.ID {
				manager.inFlight = ""
			}
		} else {
			// An unverified restoration keeps blocking new changes, exactly as
			// it would have without the restart.
			manager.inFlight = operation.ID
		}
		manager.mutex.Unlock()
		manager.settle(ctx, operation, state, result, audit.ActorKindSystem, failure)
	}
	return nil
}

// resolveUnverifiedRecoveryAgainstHelper is deliberately narrower than the
// ordinary restart mapping. Once restoration was unverified, a missing,
// applied, confirmed, mismatched or failed helper record cannot prove the old
// connectivity state. Only a matching restored record releases the interlock.
func resolveUnverifiedRecoveryAgainstHelper(
	operation *Operation, outcome RecoveryOutcome, present bool,
) (OperationState, audit.Outcome, string) {
	if present && outcome.OperationID == operation.ID && outcome.Phase == "restored" {
		return StateRolledBack, audit.OutcomeRecovered, ""
	}
	return "", "", ""
}

// resolveAgainstHelper maps the helper's phase to the operation's terminal
// state. An empty state means the change is still the helper's to finish.
func resolveAgainstHelper(operation *Operation, outcome RecoveryOutcome, present bool) (OperationState, audit.Outcome, string) {
	if !present || outcome.OperationID != operation.ID {
		return StateFailed, audit.OutcomeFailed, "the change was not found in the helper's record after a restart, so it is not applied"
	}
	switch outcome.Phase {
	case "restored":
		return StateRolledBack, audit.OutcomeRecovered, ""
	case "confirmed":
		return StateConfirmed, audit.OutcomeConfirmed, ""
	case "recovery_failed":
		return StateRollbackFailed, audit.OutcomeFailed, "the helper could not verify the previous state after a restart"
	default:
		return "", "", ""
	}
}

// Recent returns the operations an interface reattaching after a reload needs,
// newest first. Without a store it answers from the working set, which is what
// the simulation profile has.
func (manager *OperationManager) Recent(ctx context.Context, limit int) ([]Operation, error) {
	if manager.options.Store != nil {
		records, err := manager.options.Store.Recent(ctx, limit)
		if err != nil {
			return nil, fmt.Errorf("read recent network operations: %w", err)
		}
		operations := make([]Operation, 0, len(records))
		for _, record := range records {
			operations = append(operations, Operation{
				ID: record.ID, Action: Action(record.Action), Target: record.Target,
				ActorID: record.ActorID, State: OperationState(record.State),
				Revision: record.Revision, CreatedAt: record.CreatedAt,
				ConfirmDeadline: record.ConfirmDeadline, Failure: record.Failure,
			})
		}
		return operations, nil
	}
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	operations := make([]Operation, 0, len(manager.operations))
	for _, operation := range manager.operations {
		operations = append(operations, *operation)
	}
	sort.Slice(operations, func(first, second int) bool {
		if operations[first].CreatedAt.Equal(operations[second].CreatedAt) {
			return operations[first].ID > operations[second].ID
		}
		return operations[first].CreatedAt.After(operations[second].CreatedAt)
	})
	if limit > 0 && len(operations) > limit {
		operations = operations[:limit]
	}
	return operations, nil
}

// Get returns one operation's public state.
func (manager *OperationManager) Get(operationID string) (Operation, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	operation, found := manager.operations[operationID]
	if !found {
		return Operation{}, ErrOperationNotFound
	}
	return *operation, nil
}

// canonicalPreview is the exact encoding the revision digest is taken over.
// Go sorts map keys when marshalling, so the observed state encodes
// deterministically without a bespoke canonicalizer.
type canonicalPreview struct {
	Schema   string            `json:"schema"`
	Action   Action            `json:"action"`
	Target   string            `json:"target"`
	Observed map[string]string `json:"observed"`
}

// revisionOf digests everything the administrator was shown, including the
// state it was computed from. Two previews of the same proposal against the
// same world agree; one taken after the world moved does not.
//
// The operation identity is deliberately excluded, so that re-previewing the
// same proposal yields the same revision and a caller can tell "the same change"
// from "a different change".
func revisionOf(operation *Operation) (string, error) {
	encoded, err := json.Marshal(canonicalPreview{
		Schema: previewSchema, Action: operation.Action,
		Target: operation.Target, Observed: operation.observed,
	})
	if err != nil {
		return "", fmt.Errorf("encode preview for digest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
