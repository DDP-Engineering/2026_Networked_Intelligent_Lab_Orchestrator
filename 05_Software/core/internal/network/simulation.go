package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Simulation owns only in-memory network state and synthetic counters. It has
// no file, process, socket, D-Bus or host-network dependency.
type Simulation struct {
	mu            sync.Mutex
	now           func() time.Time
	interfaces    []Interface
	profiles      map[string]ProfileStatus
	saved         []Interface
	savedProfiles map[string]ProfileStatus
	checkpoint    string
	deadline      time.Time
	counters      map[string]CounterReading
	lastSample    time.Time

	// store persists the saved profile inventory when one is attached, so an
	// import survives a restart. Activation is deliberately not persisted: it is
	// runtime state, and restoring a tunnel as connected because it was
	// connected before the process died would be a claim nobody observed.
	store ProfileStore

	// settingsStore persists saved adapter configuration; settings is the
	// cached inventory, savedSettings its snapshot under a checkpoint, and
	// pendingSettings the intent of a change waiting for its confirmation.
	settingsStore   InterfaceSettingsStore
	settings        []InterfaceSettings
	savedSettings   []InterfaceSettings
	pendingSettings map[string][]InterfaceSettings

	// forwarder and deviceNetworks are the device-network half; savedForwarding
	// and savedNetworks are their snapshot under a checkpoint.
	forwarder            *memoryForwarder
	deviceNetworks       *DeviceNetworkManager
	audit                AuditJournal
	savedForwarding      map[string]ForwardingPlan
	savedNetworks        []DeviceNetwork
	roleCoordinator      *AdapterRoleCoordinator
	roleReservationOwner string
}

func NewSimulation(now func() time.Time) *Simulation {
	items, _ := NewSimulatedProvider().Interfaces(context.Background())
	return &Simulation{
		now: now, interfaces: items, profiles: map[string]ProfileStatus{},
		counters: map[string]CounterReading{}, pendingSettings: map[string][]InterfaceSettings{},
		roleCoordinator: NewAdapterRoleCoordinator(),
	}
}

// AdapterRoleCoordinator is shared by simulated management and device seams.
func (s *Simulation) AdapterRoleCoordinator() *AdapterRoleCoordinator { return s.roleCoordinator }

func (*Simulation) Source() Source { return SourceSimulation }

// UseProfileStore attaches the saved profile inventory and loads what is
// already in it. It is called once at startup, before anything serves.
func (s *Simulation) UseProfileStore(ctx context.Context, store ProfileStore) error {
	if store == nil {
		return errors.New("a profile store is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
	return s.reload(ctx)
}

// reload replaces the in-memory inventory with what is saved. A profile always
// comes back inactive and keeps its stored startup selection, because importing
// is not connecting and a restart is not an activation.
func (s *Simulation) reload(ctx context.Context) error {
	records, err := s.store.List(ctx)
	if err != nil {
		return fmt.Errorf("read saved profiles: %w", err)
	}
	s.profiles = make(map[string]ProfileStatus, len(records))
	for _, record := range records {
		s.profiles[record.ID] = ProfileStatus{
			ID: record.ID, Name: record.Name, Interface: "sim-wg",
			NILOOwned: true, Active: false, AutoConnect: record.StartSelected,
			Profile: record.Profile,
		}
	}
	return nil
}

func (s *Simulation) Interfaces(context.Context) ([]Interface, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expire(); err != nil {
		return nil, err
	}
	result := BindSettings(s.interfaces, s.settings).Interfaces
	for i := range result {
		result[i].IPv4 = append([]string(nil), result[i].IPv4...)
		result[i].IPv6 = append([]string(nil), result[i].IPv6...)
	}
	return result, nil
}

func (s *Simulation) WireGuard(context.Context) (WireGuardStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expire(); err != nil {
		return WireGuardStatus{}, err
	}
	status := WireGuardStatus{Reported: true, MeasuredBy: SourceSimulation, NetworkManagerAvailable: true, NetworkManagerVersion: "simulated", KernelSupported: true, HelperAvailable: true, Tools: []ToolStatus{{Name: "wg", Installed: true, Version: "simulated"}}, Profiles: []ProfileStatus{}}
	for _, p := range s.profiles {
		status.Profiles = append(status.Profiles, p)
	}
	sort.Slice(status.Profiles, func(i, j int) bool { return status.Profiles[i].ID < status.Profiles[j].ID })
	return status, nil
}

func (s *Simulation) Observe(ctx context.Context, action Action, target string) (map[string]string, error) {
	return s.Inspect(ctx, Change{Action: action, Target: target})
}

func (s *Simulation) Inspect(_ context.Context, c Change) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expire(); err != nil {
		return nil, err
	}
	if deviceNetworkActions[c.Action] {
		if s.deviceNetworks == nil {
			return nil, ErrUnsupported
		}
		return s.deviceNetworks.Inspect(context.Background(), c)
	}
	if err := s.validate(c); err != nil {
		return nil, err
	}
	observed := map[string]string{"action": string(c.Action), "target": c.Target, "enabled": strconv.FormatBool(c.Enabled), "connection": c.ConnectionID}
	switch c.Action {
	case ActionInterfaceDisable, ActionInterfaceEnable, ActionInterfaceSettings, ActionWiFiJoin:
		for _, item := range s.interfaces {
			if item.ID == c.Target {
				observed["state"] = string(item.State)
				observed["addresses"] = strings.Join(item.IPv4, ", ")
				observed["ssid"] = item.SSID
				observed["adapter"] = item.AdapterID
			}
		}
	default:
		encoded, _ := json.Marshal(s.profiles)
		digest := sha256.Sum256(encoded)
		observed["profiles_revision"] = hex.EncodeToString(digest[:])
	}
	return observed, nil
}

func (s *Simulation) validate(c Change) error {
	switch c.Action {
	case ActionInterfaceDisable, ActionInterfaceEnable:
		for _, item := range s.interfaces {
			if item.ID == c.Target && item.Controllable {
				if c.Action == ActionInterfaceDisable {
					return s.guardManagement(c)
				}
				return nil
			}
		}
		return ErrNotControllable
	case ActionInterfaceSettings:
		_, err := s.validateSettings(c)
		return err
	case ActionWiFiJoin:
		return s.validateJoin(c)
	case ActionWireGuardInstall:
		return nil
	case ActionProfileImport:
		if len(s.profiles) >= maximumSavedProfiles {
			return fmt.Errorf("at most %d managed profiles are supported", maximumSavedProfiles)
		}
		if _, err := importedProfile(c); err != nil {
			return err
		}
		for _, p := range s.profiles {
			if p.Name == c.Target {
				return fmt.Errorf("%w: %s", ErrProfileNameTaken, c.Target)
			}
		}
		return nil
	case ActionVPNDeactivate:
		if c.Target == "active" {
			return nil
		}
	case ActionVPNActivate, ActionProfileRemove, ActionAutostartSelect, ActionProfileRename:
	default:
		return ErrUnsupported
	}
	p, ok := s.profiles[c.Target]
	if !ok || !p.NILOOwned {
		return ErrExternalProfile
	}
	if c.Action == ActionProfileRemove {
		dependencies, err := s.deviceNetworkDependencies(context.Background(), c.Target)
		if err != nil {
			return err
		}
		if err := ValidateProfileRemoval(ProfileRecord{ID: c.Target, Name: p.Name}, p.Active, dependencies); err != nil {
			return err
		}
	}
	if c.Action == ActionProfileRename {
		name := strings.TrimSpace(c.Name)
		if name == "" || len(name) > maxProfileNameLength {
			return fmt.Errorf("%w: a profile name must be between 1 and %d characters", ErrProfileRejected, maxProfileNameLength)
		}
		for id, other := range s.profiles {
			if id != c.Target && strings.EqualFold(other.Name, name) {
				return fmt.Errorf("%w: %s", ErrProfileNameTaken, name)
			}
		}
	}
	return nil
}

func (s *Simulation) Apply(ctx context.Context, action Action, target string) error {
	return s.ApplyChange(ctx, Change{Action: action, Target: target})
}

func (s *Simulation) ApplyChange(ctx context.Context, c Change) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expire(); err != nil {
		return err
	}
	if deviceNetworkActions[c.Action] {
		if s.deviceNetworks == nil {
			return ErrUnsupported
		}
		return s.deviceNetworks.Apply(ctx, c)
	}
	if err := s.validate(c); err != nil {
		return err
	}
	// The inventory is written down before the in-memory state moves, so a
	// crash between the two leaves a saved profile rather than a lost one.
	if err := s.persist(ctx, c); err != nil {
		return err
	}
	// Whatever changes below can change a device network's prerequisites, so
	// their forwarding is brought back in step afterwards.
	defer s.reconcileDeviceNetworks(ctx)
	switch c.Action {
	case ActionInterfaceDisable, ActionInterfaceEnable:
		for i := range s.interfaces {
			item := &s.interfaces[i]
			if item.ID == c.Target {
				item.State = StateDisconnected
				item.RXBytesPerSecond = 0
				item.TXBytesPerSecond = 0
				if c.Action == ActionInterfaceEnable {
					item.State = StateConnected
					item.RXBytesPerSecond = 12480
					item.TXBytesPerSecond = 3120
				}
			}
		}
	case ActionInterfaceSettings, ActionWiFiJoin:
		return s.applySettingsChange(c)
	case ActionProfileImport:
		profile, err := importedProfile(c)
		if err != nil {
			return err
		}
		id := simulatedProfileID(c.Target)
		s.profiles[id] = ProfileStatus{ID: id, Name: c.Target, Interface: "sim-wg", NILOOwned: true, Profile: profile}
	case ActionProfileRemove:
		delete(s.profiles, c.Target)
	case ActionProfileRename:
		p := s.profiles[c.Target]
		p.Name = strings.TrimSpace(c.Name)
		p.Profile.Name = p.Name
		s.profiles[c.Target] = p
	case ActionVPNActivate, ActionVPNDeactivate:
		for id, p := range s.profiles {
			p.Active = c.Action == ActionVPNActivate && id == c.Target
			s.profiles[id] = p
		}
	case ActionAutostartSelect:
		for id, p := range s.profiles {
			if c.Enabled {
				p.AutoConnect = id == c.Target
			} else if id == c.Target {
				p.AutoConnect = false
			}
			s.profiles[id] = p
		}
	}
	return nil
}

// simulatedProfileID is the identity the simulation assigns an import. It is
// derived from the name because the simulation refuses duplicate names, so the
// two are equivalent here and a stable identity is easier to reason about in a
// test than a fresh UUID.
func simulatedProfileID(name string) string { return "sim-" + name }

// ImportProfile applies an import and reports the identity it assigned, so the
// operation lifecycle can bind staged key material to it.
func (s *Simulation) ImportProfile(ctx context.Context, c Change) (string, error) {
	if err := s.ApplyChange(ctx, c); err != nil {
		return "", err
	}
	return simulatedProfileID(c.Target), nil
}

// importedProfile takes the public configuration a preview already parsed. The
// raw text is accepted as a fallback only for a caller that reaches this
// provider directly, which the operation lifecycle never does.
func importedProfile(c Change) (Profile, error) {
	if c.Profile != nil {
		return *c.Profile, nil
	}
	profile, _, err := ParseProfile(c.Target, []byte(c.ProfileText))
	return profile, err
}

// persist mirrors an inventory change into the saved profile store. Activation
// is absent on purpose: it is runtime state, not saved configuration.
func (s *Simulation) persist(ctx context.Context, c Change) error {
	if s.store == nil {
		return nil
	}
	now := s.now().UTC()
	switch c.Action {
	case ActionProfileImport:
		profile, err := importedProfile(c)
		if err != nil {
			return err
		}
		identity := simulatedProfileID(c.Target)
		return s.store.Save(ctx, ProfileRecord{
			ID: identity, Name: c.Target, Provider: ProviderWireGuard,
			Profile: profile, SecretRef: identity, Revision: identity,
			CreatedAt: now, UpdatedAt: now,
		})
	case ActionProfileRemove:
		return s.store.Delete(ctx, c.Target)
	case ActionProfileRename:
		_, err := s.store.Rename(ctx, c.Target, strings.TrimSpace(c.Name), now)
		return err
	case ActionAutostartSelect:
		if c.Enabled {
			return s.store.SelectStartup(ctx, c.Target, now)
		}
		return s.store.ClearStartup(ctx, now)
	}
	return nil
}

// ConnectStartup brings the startup selection up the way the appliance runtime
// would: only when nothing NILO-owned is already connected, and without a
// confirmation window, because the administrator chose it in advance.
func (s *Simulation) ConnectStartup(context.Context) (StartupOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expire(); err != nil {
		return StartupOutcome{}, err
	}
	for id, p := range s.profiles {
		if !p.AutoConnect {
			continue
		}
		outcome := StartupOutcome{Selected: true, ProfileID: id, Name: p.Name}
		for _, other := range s.profiles {
			if other.Active && other.NILOOwned {
				outcome.Active = true
				return outcome, nil
			}
		}
		p.Active = true
		s.profiles[id] = p
		outcome.Connected, outcome.Active = true, true
		return outcome, nil
	}
	return StartupOutcome{}, nil
}

func (s *Simulation) ArmRecovery(_ context.Context, id string, window time.Duration) (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expire(); err != nil {
		return Checkpoint{}, err
	}
	return s.armRecoveryLocked(id, window)
}

// ArmChange retains every saved and proposed device-side identity through the
// recovery window, matching the appliance lifecycle while keeping simulation
// free of privileged network operations.
func (s *Simulation) ArmChange(_ context.Context, id string, change Change, window time.Duration) (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expire(); err != nil {
		return Checkpoint{}, err
	}
	if s.checkpoint != "" {
		return Checkpoint{}, ErrChangeInProgress
	}
	if deviceNetworkActions[change.Action] {
		s.snapshotDeviceNetworks()
		if err := s.roleCoordinator.Reserve(id, deviceNetworkReservationClaims(s.savedNetworks, change)); err != nil {
			return Checkpoint{}, err
		}
		s.roleReservationOwner = id
	}
	checkpoint, err := s.armRecoveryLocked(id, window)
	if err != nil {
		s.roleCoordinator.Release(id)
		s.roleReservationOwner = ""
	}
	return checkpoint, err
}

func (s *Simulation) armRecoveryLocked(id string, window time.Duration) (Checkpoint, error) {
	if s.checkpoint != "" {
		return Checkpoint{}, ErrChangeInProgress
	}
	s.checkpoint = "simulated-" + id
	s.deadline = s.now().Add(window)
	s.saved = append([]Interface(nil), s.interfaces...)
	s.savedSettings = append([]InterfaceSettings(nil), s.settings...)
	s.snapshotDeviceNetworks()
	s.savedProfiles = make(map[string]ProfileStatus, len(s.profiles))
	for id, p := range s.profiles {
		s.savedProfiles[id] = p
	}
	return Checkpoint{ID: s.checkpoint}, nil
}

func (s *Simulation) Restore(ctx context.Context, cp Checkpoint) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cp.ID != s.checkpoint {
		return false, ErrRecoveryUnavailable
	}
	s.interfaces = append([]Interface(nil), s.saved...)
	s.profiles = s.savedProfiles
	s.settings = s.savedSettings
	s.dropPendingSettings(cp.ID)
	if err := s.restoreDeviceNetworks(ctx); err != nil {
		return false, fmt.Errorf("restore simulated device networks: %w", err)
	}
	// A rollback restores connectivity, not the inventory. An import that
	// happened alongside is saved configuration and stays saved, so the
	// in-memory view is brought back in line with what is stored.
	if s.store != nil {
		if err := s.reload(ctx); err != nil {
			return false, err
		}
	}
	s.roleCoordinator.Release(s.roleReservationOwner)
	s.roleReservationOwner = ""
	s.checkpoint = ""
	return true, nil
}

func (s *Simulation) Discard(ctx context.Context, cp Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cp.ID != s.checkpoint || !s.now().Before(s.deadline) {
		return ErrRecoveryUnavailable
	}
	// A confirmed change is when its intent becomes saved configuration; an
	// unconfirmed one never reaches the store.
	if err := s.commitSettings(ctx, cp.ID); err != nil {
		return err
	}
	s.roleCoordinator.Release(s.roleReservationOwner)
	s.roleReservationOwner = ""
	s.checkpoint = ""
	s.saved = nil
	s.savedProfiles = nil
	s.savedSettings = nil
	return nil
}

// expire also runs on reads, so a paused runtime loop never reports an expired
// simulated change as still active. Keep the checkpoint for verified Restore.
func (s *Simulation) expire() error {
	if s.checkpoint != "" && !s.now().Before(s.deadline) {
		s.interfaces = append([]Interface(nil), s.saved...)
		s.profiles = s.savedProfiles
		s.settings = s.savedSettings
		s.dropPendingSettings(s.checkpoint)
		if err := s.restoreDeviceNetworks(context.Background()); err != nil {
			return fmt.Errorf("expire simulated device networks: %w", err)
		}
		if s.store != nil {
			if err := s.reload(context.Background()); err != nil {
				return err
			}
		}
		s.roleCoordinator.Release(s.roleReservationOwner)
		s.roleReservationOwner = ""
	}
	return nil
}

func (s *Simulation) Counters(context.Context) ([]CounterReading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.expire(); err != nil {
		return nil, err
	}
	now := s.now()
	elapsed := 0.0
	if !s.lastSample.IsZero() {
		elapsed = now.Sub(s.lastSample).Seconds()
	}
	s.lastSample = now
	items := make([]CounterReading, 0, len(s.interfaces))
	for _, item := range s.interfaces {
		c := s.counters[item.ID]
		c.InterfaceID = item.ID
		if elapsed > 0 && item.State == StateConnected {
			c.RXBytes += uint64(item.RXBytesPerSecond * elapsed)
			c.TXBytes += uint64(item.TXBytesPerSecond * elapsed)
		}
		s.counters[item.ID] = c
		items = append(items, c)
	}
	return items, nil
}
