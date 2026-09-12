package network

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const deviceNetworkRecoveryMetadataSchema = 1

type deviceNetworkRecoveryMetadata struct {
	Schema    int                `json:"schema"`
	Snapshot  []DeviceNetwork    `json:"snapshot"`
	Claims    []AdapterRoleClaim `json:"claims"`
	CreatedID string             `json:"created_id,omitempty"`
}

// ApplianceBase is what the privileged helper client provides: adapters,
// counters, recovery, the interface and installation changes, key staging and
// readiness. It is the whole of what the live runtime had before profiles
// were managed.
type ApplianceBase interface {
	Provider
	CounterSource
	ConnectivityChanger
	ChangeRecoveryPreparer
	ChangeController
	StateObserver
	WireGuardReader
	SecretStager
	InstallationController
}

// Appliance is the live provider: the helper client for everything that
// touches a device or a checkpoint, and the profile manager for the saved
// tunnel inventory. It exists so `serve` hands one value to the service, the
// operation manager and the sampler, exactly as it does with the simulation.
type Appliance struct {
	base     ApplianceBase
	profiles *ProfileManager
	settings *applianceSettings

	forwarder        Forwarder
	deviceNetworks   *DeviceNetworkManager
	audit            AuditJournal
	managementAccess interface{ ConfiguredInterfaces() []string }
	roleCoordinator  *AdapterRoleCoordinator
	// armed holds the saved device networks as they were when a change was
	// armed, keyed by checkpoint, so a restoration puts the decisions back
	// together with the forwarding.
	armedMu         sync.Mutex
	planned         map[string]deviceNetworkRecoveryMetadata
	armed           map[string][]DeviceNetwork
	armedRoleOwners map[string]string
}

// UseManagementAccessPolicy connects listener intent to the existing
// management-path guard used by adapter and tunnel changes.
func (appliance *Appliance) UseManagementAccessPolicy(policy interface{ ConfiguredInterfaces() []string }) {
	appliance.managementAccess = policy
}

// NewAppliance composes the two halves. The profile manager is given the
// observations it needs for previews and startup from the base, so it stays
// free of any device knowledge of its own.
func NewAppliance(base ApplianceBase, profiles *ProfileManager) *Appliance {
	appliance := &Appliance{
		base: base, profiles: profiles, settings: newApplianceSettings(),
		planned: map[string]deviceNetworkRecoveryMetadata{},
		armed:   map[string][]DeviceNetwork{}, armedRoleOwners: map[string]string{},
		roleCoordinator: NewAdapterRoleCoordinator(),
	}
	if profiles.options.ManagementPath == nil {
		profiles.options.ManagementPath = appliance.managementPath
	}
	if profiles.options.Uplink == nil {
		profiles.options.Uplink = appliance.uplink
	}
	return appliance
}

// AdapterRoleCoordinator is shared by management access and device networks.
func (appliance *Appliance) AdapterRoleCoordinator() *AdapterRoleCoordinator {
	return appliance.roleCoordinator
}

func (appliance *Appliance) Source() Source { return appliance.base.Source() }

func (appliance *Appliance) Interfaces(ctx context.Context) ([]Interface, error) {
	return appliance.base.Interfaces(ctx)
}

func (appliance *Appliance) Counters(ctx context.Context) ([]CounterReading, error) {
	return appliance.base.Counters(ctx)
}

// UseProfileStore attaches the saved inventory to the profile manager.
func (appliance *Appliance) UseProfileStore(ctx context.Context, store ProfileStore) error {
	return appliance.profiles.UseProfileStore(ctx, store)
}

// WireGuard merges the helper's readiness with the saved inventory. When the
// inventory cannot be read the readiness stays readiness-only, which the
// interface shows as "profiles not reported" rather than as an empty list.
func (appliance *Appliance) WireGuard(ctx context.Context) (WireGuardStatus, error) {
	status, err := appliance.base.WireGuard(ctx)
	if err != nil {
		return status, err
	}
	profiles, err := appliance.profiles.Profiles(ctx)
	if err != nil {
		status.ReadinessOnly = true
		status.Detail = joinDetail(status.Detail, "Saved WireGuard profiles could not be read.")
		return status, nil
	}
	status.ReadinessOnly = false
	status.Profiles = profiles
	return status, nil
}

func joinDetail(existing, added string) string {
	if existing == "" {
		return added
	}
	return existing + " " + added
}

func (appliance *Appliance) Inspect(ctx context.Context, change Change) (map[string]string, error) {
	switch change.Action {
	case ActionVPNActivate, ActionVPNDeactivate, ActionInterfaceDisable:
		if err := appliance.guardManagement(ctx, change); err != nil {
			return nil, err
		}
	case ActionInterfaceSettings, ActionWiFiJoin:
		resolved, err := appliance.resolveSettingsChange(ctx, change)
		if err != nil {
			return nil, err
		}
		if err := appliance.guardManagement(ctx, resolved); err != nil {
			return nil, err
		}
		observed, err := appliance.base.Inspect(ctx, resolved)
		if err != nil {
			return nil, err
		}
		observed["adapter"] = resolved.Target
		return observed, nil
	}
	if deviceNetworkActions[change.Action] {
		if appliance.deviceNetworks == nil {
			return nil, ErrUnsupported
		}
		return appliance.deviceNetworks.Inspect(ctx, change)
	}
	if vpnActions[change.Action] {
		return appliance.profiles.Inspect(ctx, change)
	}
	return appliance.base.Inspect(ctx, change)
}

func (appliance *Appliance) Observe(ctx context.Context, action Action, target string) (map[string]string, error) {
	return appliance.Inspect(ctx, Change{Action: action, Target: target})
}

func (appliance *Appliance) ArmRecovery(ctx context.Context, operationID string, window time.Duration) (Checkpoint, error) {
	return appliance.base.ArmRecovery(ctx, operationID, window)
}

// PlanChangeRecovery captures public device-network rollback state before the
// first applying record is written. The helper is not contacted here.
func (appliance *Appliance) PlanChangeRecovery(ctx context.Context, operationID string, change Change) (json.RawMessage, error) {
	if !deviceNetworkActions[change.Action] {
		return nil, nil
	}
	if _, unsupported := appliance.forwarderOrUnsupported().(unsupportedForwarder); unsupported || appliance.deviceNetworks == nil {
		return nil, fmt.Errorf("%w: device network forwarding is not available on this appliance yet", ErrUnsupported)
	}
	saved, err := appliance.deviceNetworks.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	metadata := deviceNetworkRecoveryMetadata{
		Schema: deviceNetworkRecoveryMetadataSchema, Snapshot: saved,
	}
	if change.Action == ActionDeviceNetworkCreate {
		metadata.CreatedID = operationID
		if change.DeviceNetwork != nil {
			proposed := *change.DeviceNetwork
			proposed.ID = metadata.CreatedID
			change.DeviceNetwork = &proposed
		}
	}
	metadata.Claims = deviceNetworkReservationClaims(saved, change)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode device network recovery: %w", err)
	}
	if err := appliance.roleCoordinator.Reserve(operationID, metadata.Claims); err != nil {
		return nil, err
	}
	appliance.armedMu.Lock()
	appliance.planned[operationID] = metadata
	appliance.armedMu.Unlock()
	return encoded, nil
}

// CancelChangeRecovery releases a plan that never became an armed helper
// checkpoint or a durable device-network record.
func (appliance *Appliance) CancelChangeRecovery(operationID string) {
	appliance.armedMu.Lock()
	delete(appliance.planned, operationID)
	appliance.armedMu.Unlock()
	appliance.roleCoordinator.Release(operationID)
}

// ArmChange resolves a tunnel change to its connection before the helper binds
// recovery to it, so the record the helper keeps names the connection it will
// actually touch.
func (appliance *Appliance) ArmChange(ctx context.Context, operationID string, change Change, window time.Duration) (Checkpoint, error) {
	if deviceNetworkActions[change.Action] {
		// The helper snapshots its forwarding inventory; the saved decisions
		// are snapshotted here, so both go back together. Without a
		// privileged forwarder the change is refused before anything is
		// touched.
		if _, unsupported := appliance.forwarderOrUnsupported().(unsupportedForwarder); unsupported || appliance.deviceNetworks == nil {
			return Checkpoint{}, fmt.Errorf("%w: device network forwarding is not available on this appliance yet", ErrUnsupported)
		}
		appliance.armedMu.Lock()
		planned, present := appliance.planned[operationID]
		appliance.armedMu.Unlock()
		if !present {
			if _, err := appliance.PlanChangeRecovery(ctx, operationID, change); err != nil {
				return Checkpoint{}, err
			}
			appliance.armedMu.Lock()
			planned = appliance.planned[operationID]
			appliance.armedMu.Unlock()
		}
		metadata, err := json.Marshal(planned)
		if err != nil {
			appliance.CancelChangeRecovery(operationID)
			return Checkpoint{}, fmt.Errorf("encode device network recovery: %w", err)
		}
		checkpoint, err := appliance.base.ArmChange(ctx, operationID, change, window)
		if err != nil {
			appliance.CancelChangeRecovery(operationID)
			return Checkpoint{}, err
		}
		checkpoint.Metadata = metadata
		appliance.armedMu.Lock()
		delete(appliance.planned, operationID)
		appliance.armed[checkpoint.ID] = planned.Snapshot
		appliance.armedRoleOwners[checkpoint.ID] = operationID
		appliance.armedMu.Unlock()
		return checkpoint, nil
	}
	switch change.Action {
	case ActionVPNActivate, ActionVPNDeactivate:
		resolved, err := appliance.profiles.Resolve(ctx, change)
		if err != nil {
			return Checkpoint{}, err
		}
		change = resolved
	case ActionInterfaceSettings, ActionWiFiJoin:
		resolved, err := appliance.resolveSettingsChange(ctx, change)
		if err != nil {
			return Checkpoint{}, err
		}
		adapter, err := appliance.adapter(ctx, change.Target)
		if err != nil {
			return Checkpoint{}, err
		}
		change = resolved
		checkpoint, err := appliance.base.ArmChange(ctx, operationID, change, window)
		if err != nil {
			return Checkpoint{}, err
		}
		appliance.rememberIntent(checkpoint.ID, change, adapter)
		return checkpoint, nil
	}
	return appliance.base.ArmChange(ctx, operationID, change, window)
}

func decodeDeviceNetworkRecovery(checkpoint Checkpoint) (deviceNetworkRecoveryMetadata, bool, error) {
	if len(checkpoint.Metadata) == 0 {
		return deviceNetworkRecoveryMetadata{}, false, nil
	}
	var metadata deviceNetworkRecoveryMetadata
	if err := json.Unmarshal(checkpoint.Metadata, &metadata); err != nil {
		return deviceNetworkRecoveryMetadata{}, false, fmt.Errorf("decode device network recovery: %w", err)
	}
	if metadata.Schema == 0 {
		return deviceNetworkRecoveryMetadata{}, false, nil
	}
	if metadata.Schema != deviceNetworkRecoveryMetadataSchema {
		return deviceNetworkRecoveryMetadata{}, false, fmt.Errorf("unsupported device network recovery schema %d", metadata.Schema)
	}
	for _, claim := range metadata.Claims {
		if canonicalAdapterID(claim.AdapterID) == "" || claim.Role != AdapterRoleDevice {
			return deviceNetworkRecoveryMetadata{}, false, fmt.Errorf("invalid device network recovery role claim")
		}
	}
	return metadata, true, nil
}

// AdoptChangeRecovery reconstitutes the device-network snapshot and union role
// reservation before startup composes management access around persisted state.
func (appliance *Appliance) AdoptChangeRecovery(ctx context.Context, operationID string, change Change, checkpoint Checkpoint) error {
	if !deviceNetworkActions[change.Action] {
		return nil
	}
	metadata, present, err := decodeDeviceNetworkRecovery(checkpoint)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("device network recovery metadata is missing")
	}
	if appliance.deviceNetworks == nil {
		return fmt.Errorf("device network recovery has no attached store")
	}
	if err := appliance.roleCoordinator.Reserve(operationID, metadata.Claims); err != nil {
		return err
	}
	if checkpoint.ID != "" {
		appliance.armedMu.Lock()
		appliance.armed[checkpoint.ID] = metadata.Snapshot
		appliance.armedRoleOwners[checkpoint.ID] = operationID
		appliance.armedMu.Unlock()
	}
	return nil
}

// ReconcileUncheckpointedChange resolves a create from its durable record.
// Other checkpoint-less operations are pre-helper attempts and are failed by
// the operation manager, after which their persisted metadata restores state.
func (appliance *Appliance) ReconcileUncheckpointedChange(ctx context.Context, _ string, change Change, checkpoint Checkpoint) (OperationState, bool, error) {
	if change.Action != ActionDeviceNetworkCreate {
		return "", false, nil
	}
	if appliance.deviceNetworks == nil {
		return "", true, fmt.Errorf("device network recovery has no attached store")
	}
	createdID := change.Target
	if metadata, present, err := decodeDeviceNetworkRecovery(checkpoint); err != nil {
		return "", true, err
	} else if present && metadata.CreatedID != "" {
		createdID = metadata.CreatedID
	}
	records, err := appliance.deviceNetworks.Snapshot(ctx)
	if err != nil {
		return "", true, err
	}
	for _, record := range records {
		if record.ID == createdID {
			return StateConfirmed, true, nil
		}
	}
	return StateFailed, true, nil
}

// ReconcileChangeRecovery brings the unprivileged store and role reservation
// into the terminal state already observed from the privileged helper.
func (appliance *Appliance) ReconcileChangeRecovery(ctx context.Context, operationID string, change Change, checkpoint Checkpoint, state OperationState) error {
	if !deviceNetworkActions[change.Action] {
		return nil
	}
	metadata, present, err := decodeDeviceNetworkRecovery(checkpoint)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("device network recovery metadata is missing")
	}
	switch state {
	case StateFailed, StateRolledBack:
		if appliance.deviceNetworks == nil {
			return fmt.Errorf("device network recovery has no attached store")
		}
		if err := appliance.deviceNetworks.RestoreSnapshot(ctx, metadata.Snapshot); err != nil {
			return err
		}
	case StateRollbackFailed:
		// Both the old and proposed claims remain reserved because the helper
		// could not prove which connectivity state survived.
		return nil
	case StateConfirmed:
		// The persisted device inventory is already the confirmed proposal.
	default:
		return fmt.Errorf("unsupported device network recovery state %q", state)
	}
	appliance.armedMu.Lock()
	delete(appliance.armed, checkpoint.ID)
	delete(appliance.armedRoleOwners, checkpoint.ID)
	appliance.armedMu.Unlock()
	appliance.roleCoordinator.Release(operationID)
	return nil
}

func deviceNetworkReservationClaims(saved []DeviceNetwork, change Change) []AdapterRoleClaim {
	claims := make([]AdapterRoleClaim, 0, len(saved)+1)
	for _, record := range saved {
		claims = append(claims, AdapterRoleClaim{
			AdapterID: record.DeviceInterface, Role: AdapterRoleDevice,
			ResourceID: record.ID, ResourceName: record.Name,
		})
	}
	if change.DeviceNetwork != nil &&
		(change.Action == ActionDeviceNetworkCreate || change.Action == ActionDeviceNetworkUpdate) {
		resourceID := change.Target
		if change.DeviceNetwork.ID != "" {
			resourceID = change.DeviceNetwork.ID
		}
		claims = append(claims, AdapterRoleClaim{
			AdapterID: change.DeviceNetwork.DeviceInterface, Role: AdapterRoleDevice,
			ResourceID: resourceID, ResourceName: change.DeviceNetwork.Name,
		})
	}
	return claims
}

func (appliance *Appliance) Apply(ctx context.Context, action Action, target string) error {
	return appliance.ApplyChange(ctx, Change{Action: action, Target: target})
}

func (appliance *Appliance) ApplyChange(ctx context.Context, change Change) error {
	switch change.Action {
	case ActionProfileImport:
		_, err := appliance.ImportProfile(ctx, change)
		return err
	case ActionProfileRemove, ActionProfileRename, ActionAutostartSelect:
		return appliance.profiles.ApplyChange(ctx, change)
	case ActionVPNActivate, ActionVPNDeactivate:
		resolved, err := appliance.profiles.Resolve(ctx, change)
		if err != nil {
			return err
		}
		return appliance.base.ApplyChange(ctx, resolved)
	case ActionInterfaceSettings, ActionWiFiJoin:
		resolved, err := appliance.resolveSettingsChange(ctx, change)
		if err != nil {
			return err
		}
		return appliance.base.ApplyChange(ctx, resolved)
	case ActionDeviceNetworkCreate:
		if appliance.deviceNetworks == nil {
			return ErrUnsupported
		}
		appliance.armedMu.Lock()
		planned, present := appliance.planned[change.operationID]
		appliance.armedMu.Unlock()
		if present && planned.CreatedID != "" && change.DeviceNetwork != nil {
			proposed := *change.DeviceNetwork
			proposed.ID = planned.CreatedID
			change.DeviceNetwork = &proposed
		}
		// Creation saves a disabled definition and changes no live forwarding,
		// so it has no helper recovery record to apply.
		return appliance.deviceNetworks.Apply(ctx, change)
	case ActionDeviceNetworkUpdate, ActionDeviceNetworkEnable, ActionDeviceNetworkDisable, ActionDeviceNetworkDelete:
		if appliance.deviceNetworks == nil {
			return ErrUnsupported
		}
		// The helper marks its record applied first, which is what lets the
		// forwarding calls that follow through.
		if err := appliance.base.ApplyChange(ctx, change); err != nil {
			return err
		}
		return appliance.deviceNetworks.Apply(ctx, change)
	default:
		return appliance.base.ApplyChange(ctx, change)
	}
}

func (appliance *Appliance) ImportProfile(ctx context.Context, change Change) (string, error) {
	return appliance.profiles.ImportProfile(ctx, change)
}

func (appliance *Appliance) Restore(ctx context.Context, checkpoint Checkpoint) (bool, error) {
	appliance.dropIntent(checkpoint.ID)
	observed, err := appliance.base.Restore(ctx, checkpoint)
	appliance.armedMu.Lock()
	saved, armed := appliance.armed[checkpoint.ID]
	owner := appliance.armedRoleOwners[checkpoint.ID]
	appliance.armedMu.Unlock()
	if armed && appliance.deviceNetworks != nil {
		if restoreErr := appliance.deviceNetworks.RestoreSnapshot(ctx, saved); restoreErr != nil {
			return false, restoreErr
		}
	}
	if err == nil && observed {
		appliance.armedMu.Lock()
		delete(appliance.armed, checkpoint.ID)
		delete(appliance.armedRoleOwners, checkpoint.ID)
		appliance.armedMu.Unlock()
		appliance.roleCoordinator.Release(owner)
	}
	return observed, err
}

// Discard releases the helper's checkpoint and, for a configuration change,
// makes the confirmed intent saved configuration.
func (appliance *Appliance) Discard(ctx context.Context, checkpoint Checkpoint) error {
	if err := appliance.base.Discard(ctx, checkpoint); err != nil {
		return err
	}
	if err := appliance.commitIntent(ctx, checkpoint.ID); err != nil {
		return err
	}
	appliance.armedMu.Lock()
	owner := appliance.armedRoleOwners[checkpoint.ID]
	delete(appliance.armed, checkpoint.ID)
	delete(appliance.armedRoleOwners, checkpoint.ID)
	appliance.armedMu.Unlock()
	appliance.roleCoordinator.Release(owner)
	return nil
}

func (appliance *Appliance) StageSecrets(ctx context.Context, secrets Secrets) (string, error) {
	return appliance.base.StageSecrets(ctx, secrets)
}

func (appliance *Appliance) BindSecrets(ctx context.Context, handle, profileID string) error {
	return appliance.base.BindSecrets(ctx, handle, profileID)
}

func (appliance *Appliance) DropSecrets(ctx context.Context, handle string) error {
	return appliance.base.DropSecrets(ctx, handle)
}

func (appliance *Appliance) StartInstall(ctx context.Context, id, revision string) (Operation, error) {
	return appliance.base.StartInstall(ctx, id, revision)
}

func (appliance *Appliance) PollInstall(ctx context.Context, id string) (Operation, error) {
	return appliance.base.PollInstall(ctx, id)
}

func (appliance *Appliance) ConnectStartup(ctx context.Context) (StartupOutcome, error) {
	return appliance.profiles.ConnectStartup(ctx)
}

// LastRecovery reads the helper's record when the base can.
func (appliance *Appliance) LastRecovery(ctx context.Context) (RecoveryOutcome, bool, error) {
	observer, ok := appliance.base.(RecoveryObserver)
	if !ok {
		return RecoveryOutcome{}, false, nil
	}
	return observer.LastRecovery(ctx)
}

// managementPath names the connected uplink management keeps during a tunnel
// change: the first connected wired or wireless adapter with a gateway.
func (appliance *Appliance) managementPath(ctx context.Context) string {
	interfaces, err := appliance.base.Interfaces(ctx)
	if err != nil {
		return ""
	}
	for _, candidate := range interfaces {
		if isUplink(candidate) {
			gateway := candidate.Gateway4
			if gateway == "" {
				gateway = candidate.Gateway6
			}
			return fmt.Sprintf("%s via %s", candidate.Name, gateway)
		}
	}
	return ""
}

func (appliance *Appliance) uplink(ctx context.Context) (bool, string) {
	interfaces, err := appliance.base.Interfaces(ctx)
	if err != nil {
		return false, "the adapters could not be read"
	}
	for _, candidate := range interfaces {
		if isUplink(candidate) {
			return true, ""
		}
	}
	return false, "no connected uplink with a gateway"
}

func isUplink(candidate Interface) bool {
	return (candidate.Kind == KindEthernet || candidate.Kind == KindWiFi) &&
		candidate.State == StateConnected && (candidate.Gateway4 != "" || candidate.Gateway6 != "")
}

var (
	_ Provider            = (*Appliance)(nil)
	_ CounterSource       = (*Appliance)(nil)
	_ ConnectivityChanger = (*Appliance)(nil)
	_ ChangeController    = (*Appliance)(nil)
	_ ProfileImporter     = (*Appliance)(nil)
	_ SecretStager        = (*Appliance)(nil)
	_ StartupConnector    = (*Appliance)(nil)
	_ SavedSettingsReader = (*Appliance)(nil)
	_ WiFiScanner         = (*Appliance)(nil)
)
