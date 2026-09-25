package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nilo.local/nilo/internal/audit"
)

// The device-network manager owns the saved decisions and keeps the installed
// forwarding in step with them. It is shared by both runtimes behind two seams:
// an environment that reports adapters, saved settings and profiles, and a
// forwarder that installs and removes a planned resource set. Status is
// computed, never stored - intent from the record, the plan from what is
// present now, applied from what the forwarder holds.

// DeviceNetworkState is what a reader learns about one saved device network.
type DeviceNetworkState string

const (
	DeviceNetworkDisabled       DeviceNetworkState = "disabled"
	DeviceNetworkActive         DeviceNetworkState = "active"
	DeviceNetworkWaiting        DeviceNetworkState = "waiting"
	DeviceNetworkConflict       DeviceNetworkState = "conflict"
	DeviceNetworkRecoveryFailed DeviceNetworkState = "recovery_failed"
	DeviceNetworkUnsupported    DeviceNetworkState = "unsupported"
)

// DeviceNetworkStatus is a saved device network with its observed state.
type DeviceNetworkStatus struct {
	DeviceNetwork
	State  DeviceNetworkState
	Detail string
	// Applied reports that the forwarder holds the network's full grant set.
	Applied    bool
	ObservedAt time.Time
}

// Forwarder installs and removes the owned resources of one device network.
// Both calls are idempotent: applying the same plan twice changes nothing and
// removing what is absent is not an error.
type Forwarder interface {
	ApplyForwarding(ctx context.Context, plan ForwardingPlan) error
	RemoveForwarding(ctx context.Context, deviceNetworkID string) error
	AppliedForwarding(ctx context.Context) ([]ForwardingPlan, error)
}

// DeviceNetworkEnvironment is what the manager reads about the appliance.
type DeviceNetworkEnvironment interface {
	Interfaces(ctx context.Context) ([]Interface, error)
	SavedSettings(ctx context.Context) ([]InterfaceSettings, error)
	Profiles(ctx context.Context) ([]ProfileStatus, error)
}

// TargetProber answers a bounded probe of one configured target. A prober
// that cannot reach the target reports unknown, never offline.
type TargetProber func(ctx context.Context, target netip.Addr, tcpPort int) DeviceNetworkFact

// DeviceNetworkManagerOptions are the manager's collaborators.
type DeviceNetworkManagerOptions struct {
	Forwarder   Forwarder
	Environment DeviceNetworkEnvironment
	Now         func() time.Time
	NewID       func() string
	Prober      TargetProber
	// Audit records a re-plan the runtime made on its own - a tunnel that
	// came back opened the grants, one that went closed them. Optional.
	Audit AuditJournal
	// Coordinator serializes exclusive physical adapter role claims. Optional
	// callers receive a private coordinator with no management claims.
	Coordinator *AdapterRoleCoordinator
}

// DeviceNetworkManager keeps saved device networks and installed forwarding in
// step.
type DeviceNetworkManager struct {
	options     DeviceNetworkManagerOptions
	mu          sync.Mutex
	store       DeviceNetworkStore
	coordinator *AdapterRoleCoordinator
	roleClaims  atomic.Value
}

// NewDeviceNetworkManager binds a manager to its seams.
func NewDeviceNetworkManager(options DeviceNetworkManagerOptions) (*DeviceNetworkManager, error) {
	if options.Forwarder == nil || options.Environment == nil {
		return nil, errors.New("device network manager requires a forwarder and an environment")
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.NewID == nil {
		options.NewID = randomIdentity
	}
	if options.Coordinator == nil {
		options.Coordinator = NewAdapterRoleCoordinator()
	}
	manager := &DeviceNetworkManager{options: options, coordinator: options.Coordinator}
	manager.roleClaims.Store([]AdapterRoleClaim{})
	manager.coordinator.RegisterClaimSource(manager.adapterRoleClaims)
	return manager, nil
}

// UseStore attaches the saved inventory.
func (manager *DeviceNetworkManager) UseStore(ctx context.Context, store DeviceNetworkStore) error {
	if store == nil {
		return errors.New("a device network store is required")
	}
	manager.mu.Lock()
	manager.store = store
	manager.mu.Unlock()
	records, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("read device networks: %w", err)
	}
	manager.setAdapterRoleRecords(records)
	return nil
}

func (manager *DeviceNetworkManager) adapterRoleClaims() []AdapterRoleClaim {
	claims, _ := manager.roleClaims.Load().([]AdapterRoleClaim)
	return append([]AdapterRoleClaim(nil), claims...)
}

func (manager *DeviceNetworkManager) setAdapterRoleRecords(records []DeviceNetwork) {
	claims := make([]AdapterRoleClaim, 0, len(records))
	for _, record := range records {
		claims = append(claims, AdapterRoleClaim{
			AdapterID: record.DeviceInterface, Role: AdapterRoleDevice,
			ResourceID: record.ID, ResourceName: record.Name,
		})
	}
	manager.roleClaims.Store(claims)
}

func deviceRoleClaim(record DeviceNetwork, state surroundings) AdapterRoleClaim {
	claim := AdapterRoleClaim{
		AdapterID: record.DeviceInterface, Role: AdapterRoleDevice,
		ResourceID: record.ID, ResourceName: record.Name,
	}
	if adapter, ok := state.adapter(record.DeviceInterface); ok {
		claim.Interface = adapter.Name
	}
	return claim
}

func replaceDeviceRoleRecord(records []DeviceNetwork, saved DeviceNetwork) []DeviceNetwork {
	result := append([]DeviceNetwork(nil), records...)
	for index := range result {
		if result[index].ID == saved.ID {
			result[index] = saved
			return result
		}
	}
	return append(result, saved)
}

func (manager *DeviceNetworkManager) records(ctx context.Context) ([]DeviceNetwork, error) {
	manager.mu.Lock()
	store := manager.store
	manager.mu.Unlock()
	if store == nil {
		return nil, errors.New("the device network inventory is not attached")
	}
	records, err := store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("read device networks: %w", err)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Name < records[j].Name })
	return records, nil
}

func (manager *DeviceNetworkManager) storeOrErr() (DeviceNetworkStore, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.store == nil {
		return nil, errors.New("the device network inventory is not attached")
	}
	return manager.store, nil
}

// surroundings is everything a decision is checked against.
type surroundings struct {
	interfaces []Interface
	settings   []InterfaceSettings
	profiles   []ProfileStatus
	records    []DeviceNetwork
}

func (manager *DeviceNetworkManager) surroundings(ctx context.Context) (surroundings, error) {
	interfaces, err := manager.options.Environment.Interfaces(ctx)
	if err != nil {
		return surroundings{}, err
	}
	settings, err := manager.options.Environment.SavedSettings(ctx)
	if err != nil {
		return surroundings{}, err
	}
	profiles, err := manager.options.Environment.Profiles(ctx)
	if err != nil {
		return surroundings{}, err
	}
	records, err := manager.records(ctx)
	if err != nil {
		return surroundings{}, err
	}
	return surroundings{interfaces: BindSettings(interfaces, settings).Interfaces, settings: settings, profiles: profiles, records: records}, nil
}

func (state surroundings) adapter(adapterID string) (Interface, bool) {
	for _, candidate := range state.interfaces {
		if adapterID != "" && strings.EqualFold(candidate.AdapterID, adapterID) {
			return candidate, true
		}
	}
	return Interface{}, false
}

func (state surroundings) profile(profileID string) (ProfileStatus, bool) {
	for _, candidate := range state.profiles {
		if candidate.ID == profileID {
			return candidate, true
		}
	}
	return ProfileStatus{}, false
}

func (state surroundings) record(id string) (DeviceNetwork, bool) {
	for _, candidate := range state.records {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return DeviceNetwork{}, false
}

// connectedSubnets lists the subnets the appliance is attached to, with their
// roles: the adapters that face devices are device subnets, every other
// connected adapter is uplink or management, and the active tunnel is a tunnel.
func (state surroundings) connectedSubnets(candidateDeviceSides ...string) []ConnectedSubnet {
	deviceSide := map[string]bool{}
	for _, record := range state.records {
		deviceSide[strings.ToLower(record.DeviceInterface)] = true
	}
	for _, adapterID := range candidateDeviceSides {
		deviceSide[strings.ToLower(adapterID)] = true
	}
	subnets := make([]ConnectedSubnet, 0)
	for _, candidate := range state.interfaces {
		if candidate.Kind != KindEthernet && candidate.Kind != KindWiFi {
			continue
		}
		role := SubnetUplink
		if deviceSide[strings.ToLower(candidate.AdapterID)] {
			role = SubnetDevice
		} else if candidate.Settings != nil && candidate.Settings.ManagementProtected {
			role = SubnetManagement
		}
		for _, prefix := range prefixesOf(candidate.IPv4) {
			subnets = append(subnets, ConnectedSubnet{Interface: candidate.Name, Role: role, Prefix: prefix.Masked()})
		}
	}
	for _, profile := range state.profiles {
		if !profile.Active || !profile.NILOOwned {
			continue
		}
		for _, address := range profile.Profile.Addresses {
			if prefix, err := netip.ParsePrefix(address); err == nil && prefix.Addr().Is4() {
				subnets = append(subnets, ConnectedSubnet{Interface: profile.Interface, Role: SubnetTunnel, Prefix: prefix.Masked()})
			}
		}
	}
	return subnets
}

// context resolves the device network's stable identities to what is present.
func (state surroundings) context(record DeviceNetwork) ForwardingContext {
	var result ForwardingContext
	if adapter, ok := state.adapter(record.DeviceInterface); ok && adapter.State != StateUnavailable {
		result.DeviceLink = adapter.Name
	}
	if profile, ok := state.profile(record.VPNProfileID); ok && profile.Active && profile.NILOOwned {
		result.TunnelDevice = profile.Interface
	}
	if adapter, ok := state.adapter(record.LocalInterface); ok && adapter.State != StateUnavailable {
		result.LocalLink = adapter.Name
	}
	return result
}

// validate checks a proposal against the appliance: the pure rules, the
// adapters and profile it names, and the management path.
func (manager *DeviceNetworkManager) validate(state surroundings, record DeviceNetwork, change Change) error {
	if err := record.ValidateAgainst(state.connectedSubnets(record.DeviceInterface), state.records); err != nil {
		return err
	}
	deviceAdapter, ok := state.adapter(record.DeviceInterface)
	if !ok {
		return reject("no detected adapter has the identity %s", record.DeviceInterface)
	}
	if deviceAdapter.Kind != KindEthernet && deviceAdapter.Kind != KindWiFi {
		return reject("%s cannot face a device network", deviceAdapter.Name)
	}
	if record.Access == AccessLocal || record.Access == AccessBoth {
		if _, ok := state.adapter(record.LocalInterface); !ok {
			return reject("no detected adapter has the identity %s for local access", record.LocalInterface)
		}
	}
	if record.Access == AccessWireGuard || record.Access == AccessBoth {
		profile, ok := state.profile(record.VPNProfileID)
		if !ok {
			return fmt.Errorf("%w: %s is not a saved WireGuard profile", ErrProfileNotFound, record.VPNProfileID)
		}
		if !profile.NILOOwned {
			return fmt.Errorf("%w: %s is not managed by NILO", ErrExternalProfile, profile.Name)
		}
		// Replies to a VPN source travel back through the tunnel only when a
		// peer's AllowedIPs carry that source; anything else is silently lost,
		// so it is refused here instead.
		for _, source := range record.VPNSources {
			if !profileRoutes(profile.Profile, source) {
				return reject("the profile %s does not route %s back through the tunnel; no peer's AllowedIPs cover it", profile.Name, source)
			}
		}
	}
	// Turning the adapter into a device side repurposes it, which the trace
	// refuses when it is the last management path.
	guard := Change{Action: ActionInterfaceDisable, Target: deviceAdapter.ID, Origin: change.Origin}
	return ValidateManagementChange(guard, state.interfaces, state.settings, managementContextOf(change, nil))
}

// Inspect validates one proposed change and returns what its preview is
// computed from. It changes nothing.
func (manager *DeviceNetworkManager) Inspect(ctx context.Context, change Change) (map[string]string, error) {
	if !deviceNetworkActions[change.Action] {
		return nil, ErrUnsupported
	}
	state, err := manager.surroundings(ctx)
	if err != nil {
		return nil, err
	}
	observed := map[string]string{"action": string(change.Action), "target": change.Target, "inventory_revision": state.inventoryRevision()}
	switch change.Action {
	case ActionDeviceNetworkCreate:
		if change.DeviceNetwork == nil {
			return nil, reject("a device network needs its configuration")
		}
		proposed := *change.DeviceNetwork
		proposed.ID, proposed.StartEnabled = "", false
		if err := manager.validate(state, proposed, change); err != nil {
			return nil, err
		}
		if err := manager.coordinator.Check([]AdapterRoleClaim{deviceRoleClaim(proposed, state)}); err != nil {
			return nil, err
		}
		observed["name"] = proposed.Name
	case ActionDeviceNetworkUpdate:
		if change.DeviceNetwork == nil {
			return nil, reject("an update needs the new configuration")
		}
		existing, ok := state.record(change.Target)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, change.Target)
		}
		proposed := *change.DeviceNetwork
		proposed.ID, proposed.StartEnabled = existing.ID, existing.StartEnabled
		if err := manager.validate(state, proposed, change); err != nil {
			return nil, err
		}
		if err := manager.coordinator.Check([]AdapterRoleClaim{deviceRoleClaim(proposed, state)}); err != nil {
			return nil, err
		}
		observed["revision"] = existing.Revision
	default:
		existing, ok := state.record(change.Target)
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, change.Target)
		}
		observed["revision"] = existing.Revision
		observed["enabled"] = fmt.Sprint(existing.StartEnabled)
		if change.Action == ActionDeviceNetworkEnable {
			if err := manager.coordinator.Check([]AdapterRoleClaim{deviceRoleClaim(existing, state)}); err != nil {
				return nil, err
			}
			plan, err := PlanForwarding(existing, state.context(existing))
			if err != nil {
				return nil, err
			}
			observed["waiting"] = plan.WaitingReason
			observed["resources"] = fmt.Sprint(len(plan.Resources))
		}
	}
	return observed, nil
}

func (state surroundings) inventoryRevision() string {
	digest := sha256.New()
	for _, record := range state.records {
		fmt.Fprintf(digest, "%s\x00%s\x00%t\n", record.ID, record.Revision, record.StartEnabled)
	}
	return hex.EncodeToString(digest.Sum(nil))[:16]
}

// Apply makes one change: it saves the decision and installs or removes the
// forwarding it implies.
func (manager *DeviceNetworkManager) Apply(ctx context.Context, change Change) error {
	if !deviceNetworkActions[change.Action] {
		return ErrUnsupported
	}
	store, err := manager.storeOrErr()
	if err != nil {
		return err
	}
	state, err := manager.surroundings(ctx)
	if err != nil {
		return err
	}
	now := manager.options.Now().UTC()
	switch change.Action {
	case ActionDeviceNetworkCreate:
		if change.DeviceNetwork == nil {
			return reject("a device network needs its configuration")
		}
		proposed := *change.DeviceNetwork
		if proposed.ID == "" {
			proposed.ID = manager.options.NewID()
		}
		proposed.StartEnabled = false
		if err := manager.validate(state, proposed, change); err != nil {
			return err
		}
		return manager.coordinator.CheckAndRun([]AdapterRoleClaim{deviceRoleClaim(proposed, state)}, func() error {
			saved, err := store.Save(ctx, proposed, now)
			if err == nil {
				manager.setAdapterRoleRecords(replaceDeviceRoleRecord(state.records, saved))
			}
			return err
		})
	case ActionDeviceNetworkUpdate:
		existing, ok := state.record(change.Target)
		if !ok {
			return fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, change.Target)
		}
		proposed := *change.DeviceNetwork
		proposed.ID, proposed.StartEnabled, proposed.CreatedAt = existing.ID, existing.StartEnabled, existing.CreatedAt
		if err := manager.validate(state, proposed, change); err != nil {
			return err
		}
		return manager.coordinator.CheckAndRun([]AdapterRoleClaim{deviceRoleClaim(proposed, state)}, func() error {
			saved, err := store.Save(ctx, proposed, now)
			if err != nil {
				return err
			}
			manager.setAdapterRoleRecords(replaceDeviceRoleRecord(state.records, saved))
			if saved.StartEnabled {
				return manager.install(ctx, state, saved)
			}
			return nil
		})
	case ActionDeviceNetworkEnable:
		existing, ok := state.record(change.Target)
		if !ok {
			return fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, change.Target)
		}
		existing.StartEnabled = true
		return manager.coordinator.CheckAndRun([]AdapterRoleClaim{deviceRoleClaim(existing, state)}, func() error {
			saved, err := store.Save(ctx, existing, now)
			if err != nil {
				return err
			}
			manager.setAdapterRoleRecords(replaceDeviceRoleRecord(state.records, saved))
			return manager.install(ctx, state, saved)
		})
	case ActionDeviceNetworkDisable:
		existing, ok := state.record(change.Target)
		if !ok {
			return fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, change.Target)
		}
		if err := manager.options.Forwarder.RemoveForwarding(ctx, existing.ID); err != nil {
			return err
		}
		existing.StartEnabled = false
		saved, err := store.Save(ctx, existing, now)
		if err == nil {
			manager.setAdapterRoleRecords(replaceDeviceRoleRecord(state.records, saved))
		}
		return err
	case ActionDeviceNetworkDelete:
		existing, ok := state.record(change.Target)
		if !ok {
			return fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, change.Target)
		}
		if err := manager.options.Forwarder.RemoveForwarding(ctx, existing.ID); err != nil {
			return err
		}
		if err := store.Delete(ctx, existing.ID); err != nil {
			return err
		}
		remaining := make([]DeviceNetwork, 0, len(state.records)-1)
		for _, record := range state.records {
			if record.ID != existing.ID {
				remaining = append(remaining, record)
			}
		}
		manager.setAdapterRoleRecords(remaining)
		return nil
	}
	return ErrUnsupported
}

// install plans and installs one enabled network. A waiting plan installs its
// guards only, which is a closed state, not an absent one.
func (manager *DeviceNetworkManager) install(ctx context.Context, state surroundings, record DeviceNetwork) error {
	plan, err := PlanForwarding(record, state.context(record))
	if err != nil {
		return err
	}
	return manager.options.Forwarder.ApplyForwarding(ctx, plan)
}

// Reconcile brings the installed forwarding of every enabled network in step
// with what is present now: a tunnel that came up opens the grants, one that
// went down closes them. It is what the runtime calls after a change and on
// its maintenance cadence.
func (manager *DeviceNetworkManager) Reconcile(ctx context.Context) error {
	if _, err := manager.storeOrErr(); err != nil {
		return nil
	}
	state, err := manager.surroundings(ctx)
	if err != nil {
		return err
	}
	applied, err := manager.options.Forwarder.AppliedForwarding(ctx)
	if err != nil {
		return err
	}
	installed := map[string]ForwardingPlan{}
	for _, plan := range applied {
		installed[plan.DeviceNetworkID] = plan
	}
	for _, record := range state.records {
		current, present := installed[record.ID]
		if !record.StartEnabled {
			if present {
				if err := manager.options.Forwarder.RemoveForwarding(ctx, record.ID); err != nil {
					return err
				}
			}
			continue
		}
		plan, err := PlanForwarding(record, state.context(record))
		if err != nil {
			return err
		}
		if present && reflect.DeepEqual(current.Resources, plan.Resources) && current.Waiting == plan.Waiting {
			continue
		}
		if err := manager.options.Forwarder.ApplyForwarding(ctx, plan); err != nil {
			return err
		}
		manager.reconciled(ctx, record, plan)
	}
	for id := range installed {
		if _, ok := state.record(id); !ok {
			if err := manager.options.Forwarder.RemoveForwarding(ctx, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// reconciled records one re-plan the runtime made on its own.
func (manager *DeviceNetworkManager) reconciled(ctx context.Context, record DeviceNetwork, plan ForwardingPlan) {
	if manager.options.Audit == nil {
		return
	}
	severity := audit.SeverityInfo
	if plan.Waiting {
		severity = audit.SeverityWarning
	}
	_ = manager.options.Audit.Record(ctx, audit.Record{
		Category: audit.CategoryEvent, Component: "network.device_networks", Action: "device_network.reconciled",
		SubjectType: "device_network", SubjectID: record.ID, TargetLabel: record.Name,
		ActorKind: audit.ActorKindSystem, Outcome: audit.OutcomeApplied, Severity: severity,
		Details: map[string]any{"waiting": plan.Waiting, "reason": plan.WaitingReason, "resources": len(plan.Resources)},
	})
}

// List reports every saved device network with its observed state.
func (manager *DeviceNetworkManager) List(ctx context.Context) ([]DeviceNetworkStatus, error) {
	state, err := manager.surroundings(ctx)
	if err != nil {
		return nil, err
	}
	applied, err := manager.options.Forwarder.AppliedForwarding(ctx)
	if err != nil {
		return nil, err
	}
	installed := map[string]ForwardingPlan{}
	for _, plan := range applied {
		installed[plan.DeviceNetworkID] = plan
	}
	now := manager.options.Now().UTC()
	statuses := make([]DeviceNetworkStatus, 0, len(state.records))
	for _, record := range state.records {
		statuses = append(statuses, manager.status(state, record, installed, now))
	}
	return statuses, nil
}

// Get reports one saved device network.
func (manager *DeviceNetworkManager) Get(ctx context.Context, id string) (DeviceNetworkStatus, error) {
	statuses, err := manager.List(ctx)
	if err != nil {
		return DeviceNetworkStatus{}, err
	}
	for _, status := range statuses {
		if status.ID == id {
			return status, nil
		}
	}
	return DeviceNetworkStatus{}, fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, id)
}

func (manager *DeviceNetworkManager) status(state surroundings, record DeviceNetwork, installed map[string]ForwardingPlan, now time.Time) DeviceNetworkStatus {
	status := DeviceNetworkStatus{DeviceNetwork: record, ObservedAt: now, State: DeviceNetworkDisabled}
	current, present := installed[record.ID]
	if !record.StartEnabled {
		if present {
			status.Detail = "forwarding is still installed for a disabled network and will be removed"
		}
		return status
	}
	if err := record.ValidateAgainst(state.connectedSubnets(record.DeviceInterface), state.records); err != nil {
		status.State, status.Detail = DeviceNetworkConflict, err.Error()
		return status
	}
	plan, err := PlanForwarding(record, state.context(record))
	if err != nil {
		status.State, status.Detail = DeviceNetworkConflict, err.Error()
		return status
	}
	if plan.Waiting {
		status.State, status.Detail = DeviceNetworkWaiting, plan.WaitingReason
		return status
	}
	if present && !current.Waiting && reflect.DeepEqual(current.Resources, plan.Resources) {
		status.State, status.Applied = DeviceNetworkActive, true
		return status
	}
	status.State, status.Detail = DeviceNetworkWaiting, "the forwarding for this network has not been installed yet"
	return status
}

// Snapshot is the saved inventory as it is now, for putting back after a
// change that is not confirmed.
func (manager *DeviceNetworkManager) Snapshot(ctx context.Context) ([]DeviceNetwork, error) {
	if _, err := manager.storeOrErr(); err != nil {
		return nil, err
	}
	return manager.records(ctx)
}

// RestoreSnapshot makes the saved inventory match a snapshot exactly: records
// added since are deleted, records changed or removed are written back with
// the timestamps they had.
func (manager *DeviceNetworkManager) RestoreSnapshot(ctx context.Context, saved []DeviceNetwork) error {
	store, err := manager.storeOrErr()
	if err != nil {
		return err
	}
	current, err := manager.records(ctx)
	if err != nil {
		return err
	}
	wanted := map[string]DeviceNetwork{}
	for _, record := range saved {
		wanted[record.ID] = record
	}
	for _, record := range current {
		if _, keep := wanted[record.ID]; !keep {
			if err := store.Delete(ctx, record.ID); err != nil {
				return err
			}
		}
	}
	for _, record := range saved {
		if _, err := store.Save(ctx, record, record.UpdatedAt); err != nil {
			return err
		}
	}
	manager.setAdapterRoleRecords(saved)
	return nil
}

// Dependencies reports the device networks that use one profile, for the
// profile manager's removal check.
func (manager *DeviceNetworkManager) Dependencies(ctx context.Context, profileID string) ([]ProfileDependency, error) {
	if _, err := manager.storeOrErr(); err != nil {
		return nil, nil
	}
	records, err := manager.records(ctx)
	if err != nil {
		return nil, err
	}
	var dependencies []ProfileDependency
	for _, record := range records {
		if record.VPNProfileID == profileID && (record.Access == AccessWireGuard || record.Access == AccessBoth) {
			dependencies = append(dependencies, ProfileDependency{Kind: "device network", ID: record.ID, Name: record.Name})
		}
	}
	return dependencies, nil
}

// UsesAdapter reports the device network that faces devices on one adapter.
func (manager *DeviceNetworkManager) UsesAdapter(ctx context.Context, adapterID string) (DeviceNetwork, bool) {
	records, err := manager.records(ctx)
	if err != nil {
		return DeviceNetwork{}, false
	}
	for _, record := range records {
		if strings.EqualFold(record.DeviceInterface, adapterID) {
			return record, true
		}
	}
	return DeviceNetwork{}, false
}

// profileRoutes reports whether one peer of the profile carries the prefix in
// its AllowedIPs, which is what makes the tunnel accept and return its traffic.
func profileRoutes(profile Profile, prefix netip.Prefix) bool {
	for _, peer := range profile.Peers {
		for _, allowed := range peer.AllowedIPs {
			carried, err := netip.ParsePrefix(strings.TrimSpace(allowed))
			if err != nil {
				continue
			}
			if carried.Bits() <= prefix.Bits() && carried.Contains(prefix.Addr()) {
				return true
			}
		}
	}
	return false
}
