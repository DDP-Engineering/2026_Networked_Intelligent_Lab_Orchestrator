package network

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// The profile manager is the unprivileged half of the live WireGuard
// lifecycle. It owns the saved inventory and the mapping from a profile's
// stable identity to the connection the privileged helper created for it, and
// it never sees key material: the helper binds the staged handle itself.

// ProfileManagerOptions are the manager's collaborators.
type ProfileManagerOptions struct {
	Driver TunnelDriver
	Now    func() time.Time
	NewID  func() string
	// Dependencies reports what would break if a profile were removed. It is
	// optional until something can depend on a profile; device networks supply
	// it.
	Dependencies func(ctx context.Context, profileID string) ([]ProfileDependency, error)
	// ManagementPath names the path management keeps during a switch, for the
	// preview. It is optional; the appliance composite supplies it from what
	// it observes.
	ManagementPath func(ctx context.Context) string
	// Uplink reports whether the appliance has a connected uplink for a tunnel
	// to run over, and names the reason when it does not.
	Uplink func(ctx context.Context) (bool, string)
}

// ProfileManager keeps the saved inventory and the helper's tunnels in step.
type ProfileManager struct {
	options ProfileManagerOptions

	mu    sync.Mutex
	store ProfileStore
}

// NewProfileManager binds a manager to the privileged driver.
func NewProfileManager(options ProfileManagerOptions) (*ProfileManager, error) {
	if options.Driver == nil {
		return nil, errors.New("profile manager requires a tunnel driver")
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.NewID == nil {
		options.NewID = randomIdentity
	}
	return &ProfileManager{options: options}, nil
}

// randomIdentity is the default profile identity: 16 random bytes as hex,
// which the keystore accepts as a file name and nothing can derive from.
func randomIdentity() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Sprintf("profile identity: %v", err))
	}
	return hex.EncodeToString(raw)
}

// vpnActions are the actions the manager answers for.
var vpnActions = map[Action]bool{
	ActionVPNActivate: true, ActionVPNDeactivate: true, ActionProfileImport: true,
	ActionProfileRemove: true, ActionAutostartSelect: true, ActionProfileRename: true,
}

// UseProfileStore attaches the saved inventory. It is called once at startup,
// before anything serves.
func (manager *ProfileManager) UseProfileStore(_ context.Context, store ProfileStore) error {
	if store == nil {
		return errors.New("a profile store is required")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.store = store
	return nil
}

// inventory is the saved records and the host's tunnels read together, which
// is the state every decision below is made against.
type inventory struct {
	records []ProfileRecord
	tunnels []TunnelObservation
}

func (manager *ProfileManager) inventory(ctx context.Context) (inventory, error) {
	manager.mu.Lock()
	store := manager.store
	manager.mu.Unlock()
	if store == nil {
		return inventory{}, errors.New("the saved profile inventory is not attached")
	}
	records, err := store.List(ctx)
	if err != nil {
		return inventory{}, fmt.Errorf("read saved profiles: %w", err)
	}
	tunnels, err := manager.options.Driver.Tunnels(ctx)
	if err != nil {
		return inventory{}, fmt.Errorf("read tunnels: %w", err)
	}
	return inventory{records: records, tunnels: tunnels}, nil
}

func (state inventory) tunnelOf(record ProfileRecord) (TunnelObservation, bool) {
	for _, tunnel := range state.tunnels {
		if tunnel.ConnectionRef == record.ConnectionRef && record.ConnectionRef != "" {
			return tunnel, true
		}
	}
	return TunnelObservation{}, false
}

func (state inventory) record(profileID string) (ProfileRecord, bool) {
	for _, record := range state.records {
		if record.ID == profileID {
			return record, true
		}
	}
	return ProfileRecord{}, false
}

// active returns the one NILO-owned tunnel that is up, if any.
func (state inventory) active() (ProfileRecord, TunnelObservation, bool) {
	for _, record := range state.records {
		if tunnel, ok := state.tunnelOf(record); ok && tunnel.Active {
			return record, tunnel, true
		}
	}
	return ProfileRecord{}, TunnelObservation{}, false
}

// external reports whether the identity names a tunnel NILO did not create.
func (state inventory) external(target string) bool {
	for _, tunnel := range state.tunnels {
		if tunnel.ConnectionRef == target && !tunnel.NILOOwned {
			return true
		}
	}
	return false
}

// resolve turns the identity a caller sent into a saved record, refusing an
// external tunnel by name.
func (state inventory) resolve(target string) (ProfileRecord, error) {
	if record, ok := state.record(target); ok {
		return record, nil
	}
	if state.external(target) {
		return ProfileRecord{}, fmt.Errorf("%w: %s is not managed by NILO", ErrExternalProfile, target)
	}
	return ProfileRecord{}, fmt.Errorf("%w: %s", ErrProfileNotFound, target)
}

// ActiveTunnelPath describes the connected NILO-owned tunnel for the
// management-path trace, or nil when none is up.
func (manager *ProfileManager) ActiveTunnelPath(ctx context.Context) *TunnelPath {
	state, err := manager.inventory(ctx)
	if err != nil {
		return nil
	}
	record, tunnel, ok := state.active()
	if !ok {
		return nil
	}
	return tunnelPathOf(record, tunnel)
}

// Profiles merges the saved inventory with what the host reports: every saved
// profile with its observed tunnel state, and every tunnel NILO did not create
// as an external, read-only entry.
func (manager *ProfileManager) Profiles(ctx context.Context) ([]ProfileStatus, error) {
	state, err := manager.inventory(ctx)
	if err != nil {
		return nil, err
	}
	statuses := make([]ProfileStatus, 0, len(state.records)+len(state.tunnels))
	for _, record := range state.records {
		status := ProfileStatus{
			ID: record.ID, Name: record.Name, NILOOwned: true,
			AutoConnect: record.StartSelected, Profile: record.Profile,
		}
		if tunnel, ok := state.tunnelOf(record); ok {
			status.Interface, status.Active, status.LastHandshake = tunnel.Interface, tunnel.Active, tunnel.LastHandshake
		}
		statuses = append(statuses, status)
	}
	for _, tunnel := range state.tunnels {
		if tunnel.NILOOwned {
			continue
		}
		statuses = append(statuses, ProfileStatus{
			ID: tunnel.ConnectionRef, Name: tunnel.Name, Interface: tunnel.Interface,
			NILOOwned: false, Active: tunnel.Active, LastHandshake: tunnel.LastHandshake,
			Profile: tunnel.Profile,
		})
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].NILOOwned != statuses[j].NILOOwned {
			return statuses[i].NILOOwned
		}
		return statuses[i].Name < statuses[j].Name
	})
	return statuses, nil
}

// Inspect validates one proposed change against the current inventory and
// returns what the preview is computed from. It changes nothing.
func (manager *ProfileManager) Inspect(ctx context.Context, change Change) (map[string]string, error) {
	if !vpnActions[change.Action] {
		return nil, ErrUnsupported
	}
	state, err := manager.inventory(ctx)
	if err != nil {
		return nil, err
	}
	observed := map[string]string{
		"action": string(change.Action), "target": change.Target,
		"profiles_revision": state.revision(),
	}
	switch change.Action {
	case ActionProfileImport:
		if change.Profile == nil {
			return nil, fmt.Errorf("%w: an import needs the parsed profile", ErrProfileRejected)
		}
		if err := ValidateProfileImport(change.Target, state.records); err != nil {
			return nil, err
		}
		observed["routes"] = routesOf(*change.Profile)
		observed["dns"] = strings.Join(change.Profile.DNS, ", ")
	case ActionProfileRemove:
		record, err := state.resolve(change.Target)
		if err != nil {
			return nil, err
		}
		tunnel, _ := state.tunnelOf(record)
		dependencies, err := manager.dependencies(ctx, record.ID)
		if err != nil {
			return nil, err
		}
		if err := ValidateProfileRemoval(record, tunnel.Active, dependencies); err != nil {
			return nil, err
		}
		observed["connection"] = record.ConnectionRef
	case ActionProfileRename:
		record, err := state.resolve(change.Target)
		if err != nil {
			return nil, err
		}
		if err := validateProfileName(change.Name, record.ID, state.records); err != nil {
			return nil, err
		}
		observed["name"] = record.Name
		observed["new_name"] = change.Name
	case ActionAutostartSelect:
		record, err := state.resolve(change.Target)
		if err != nil {
			return nil, err
		}
		observed["startup"] = fmt.Sprint(record.StartSelected)
	case ActionVPNActivate:
		record, err := state.resolve(change.Target)
		if err != nil {
			return nil, err
		}
		if record.ConnectionRef == "" {
			return nil, fmt.Errorf("%w: %s has no connection to activate", ErrUnsupported, record.Name)
		}
		if tunnel, ok := state.tunnelOf(record); ok && tunnel.Active {
			return nil, fmt.Errorf("%w: %s is already the connected tunnel", ErrProfileActive, record.Name)
		}
		manager.describeSwitch(ctx, observed, state, record)
	case ActionVPNDeactivate:
		record, err := manager.resolveDeactivation(state, change.Target)
		if err != nil {
			return nil, err
		}
		observed["connection"] = record.ConnectionRef
		observed["management_path"] = manager.managementPath(ctx)
	}
	return observed, nil
}

// describeSwitch fills the preview of an activation with what it affects: the
// tunnel that will be replaced, the routing and DNS intent that will apply, and
// the management path that stays in place.
func (manager *ProfileManager) describeSwitch(ctx context.Context, observed map[string]string, state inventory, record ProfileRecord) {
	observed["connection"] = record.ConnectionRef
	observed["routes"] = routesOf(record.Profile)
	observed["dns"] = strings.Join(record.Profile.DNS, ", ")
	observed["endpoints"] = endpointsOf(record.Profile)
	observed["active_profile"] = ""
	if current, _, ok := state.active(); ok {
		observed["active_profile"] = current.ID
		observed["active_profile_name"] = current.Name
	}
	observed["management_path"] = manager.managementPath(ctx)
	if dependencies, err := manager.dependencies(ctx, record.ID); err == nil && len(dependencies) > 0 {
		names := make([]string, 0, len(dependencies))
		for _, dependency := range dependencies {
			names = append(names, dependency.Name)
		}
		observed["affected"] = strings.Join(names, ", ")
	}
}

func (manager *ProfileManager) managementPath(ctx context.Context) string {
	if manager.options.ManagementPath == nil {
		return "not observed"
	}
	if path := manager.options.ManagementPath(ctx); path != "" {
		return path
	}
	return "not observed"
}

func (manager *ProfileManager) dependencies(ctx context.Context, profileID string) ([]ProfileDependency, error) {
	if manager.options.Dependencies == nil {
		return nil, nil
	}
	return manager.options.Dependencies(ctx, profileID)
}

// resolveDeactivation turns "active" into the one NILO-owned tunnel that is
// up, and checks a named profile is actually the connected one.
func (manager *ProfileManager) resolveDeactivation(state inventory, target string) (ProfileRecord, error) {
	if target == "active" {
		record, _, ok := state.active()
		if !ok {
			return ProfileRecord{}, fmt.Errorf("%w: no NILO-managed tunnel is connected", ErrExternalProfile)
		}
		return record, nil
	}
	record, err := state.resolve(target)
	if err != nil {
		return ProfileRecord{}, err
	}
	if tunnel, ok := state.tunnelOf(record); !ok || !tunnel.Active {
		return ProfileRecord{}, fmt.Errorf("%w: %s is not connected", ErrNotAwaitingConfirm, record.Name)
	}
	return record, nil
}

// Resolve fills in the connection reference the helper acts on. The API and
// the audit record keep the profile identity as the target; only the change
// handed to the privileged side carries the connection.
func (manager *ProfileManager) Resolve(ctx context.Context, change Change) (Change, error) {
	state, err := manager.inventory(ctx)
	if err != nil {
		return Change{}, err
	}
	switch change.Action {
	case ActionVPNActivate:
		record, err := state.resolve(change.Target)
		if err != nil {
			return Change{}, err
		}
		change.ConnectionID = record.ConnectionRef
	case ActionVPNDeactivate:
		record, err := manager.resolveDeactivation(state, change.Target)
		if err != nil {
			return Change{}, err
		}
		change.Target, change.ConnectionID = record.ID, record.ConnectionRef
	}
	if change.ConnectionID == "" {
		return Change{}, fmt.Errorf("%w: %s names no connection", ErrUnsupported, change.Target)
	}
	return change, nil
}

// ImportProfile creates the connection through the helper, which binds the
// staged key material to the new identity, then saves the public record. The
// identity is minted here so the record and the key share it.
func (manager *ProfileManager) ImportProfile(ctx context.Context, change Change) (string, error) {
	if change.Action != ActionProfileImport || change.Profile == nil {
		return "", fmt.Errorf("%w: an import needs the parsed profile", ErrProfileRejected)
	}
	state, err := manager.inventory(ctx)
	if err != nil {
		return "", err
	}
	if err := ValidateProfileImport(change.Target, state.records); err != nil {
		return "", err
	}
	now := manager.options.Now().UTC()
	record := ProfileRecord{
		ID: manager.options.NewID(), Name: change.Target, Provider: ProviderWireGuard,
		Profile: *change.Profile, CreatedAt: now, UpdatedAt: now,
	}
	record.SecretRef = record.ID
	record.Revision = profileRevision(record.ID, now)
	connectionRef, err := manager.options.Driver.CreateTunnel(ctx, record, change.SecretHandle)
	if err != nil {
		return "", fmt.Errorf("create the tunnel connection: %w", err)
	}
	record.ConnectionRef = connectionRef
	manager.mu.Lock()
	store := manager.store
	manager.mu.Unlock()
	if err := store.Save(ctx, record); err != nil {
		// The connection exists but nothing refers to it. Removing it keeps the
		// host in step with the inventory rather than leaving an orphan.
		_ = manager.options.Driver.DeleteTunnel(ctx, record.ID, connectionRef)
		return "", fmt.Errorf("save the imported profile: %w", err)
	}
	return record.ID, nil
}

// ApplyChange applies the non-connectivity profile actions. Activation and
// deactivation are connectivity changes and are applied by the helper under
// its checkpoint; asking this manager to apply one is a wiring mistake.
func (manager *ProfileManager) ApplyChange(ctx context.Context, change Change) error {
	state, err := manager.inventory(ctx)
	if err != nil {
		return err
	}
	manager.mu.Lock()
	store := manager.store
	manager.mu.Unlock()
	now := manager.options.Now().UTC()
	switch change.Action {
	case ActionProfileRemove:
		record, err := state.resolve(change.Target)
		if err != nil {
			return err
		}
		tunnel, _ := state.tunnelOf(record)
		dependencies, err := manager.dependencies(ctx, record.ID)
		if err != nil {
			return err
		}
		if err := ValidateProfileRemoval(record, tunnel.Active, dependencies); err != nil {
			return err
		}
		if err := manager.options.Driver.DeleteTunnel(ctx, record.ID, record.ConnectionRef); err != nil {
			return fmt.Errorf("delete the tunnel connection: %w", err)
		}
		return store.Delete(ctx, record.ID)
	case ActionProfileRename:
		record, err := state.resolve(change.Target)
		if err != nil {
			return err
		}
		if err := validateProfileName(change.Name, record.ID, state.records); err != nil {
			return err
		}
		_, err = store.Rename(ctx, record.ID, change.Name, now)
		return err
	case ActionAutostartSelect:
		record, err := state.resolve(change.Target)
		if err != nil {
			return err
		}
		if change.Enabled {
			return store.SelectStartup(ctx, record.ID, now)
		}
		if record.StartSelected {
			return store.ClearStartup(ctx, now)
		}
		return nil
	case ActionVPNActivate, ActionVPNDeactivate:
		return errors.New("tunnel activation is applied by the privileged helper under its checkpoint")
	default:
		return ErrUnsupported
	}
}

// ConnectStartup connects the startup selection when nothing NILO-owned is up
// and an uplink exists. It is what the runtime calls at start.
func (manager *ProfileManager) ConnectStartup(ctx context.Context) (StartupOutcome, error) {
	state, err := manager.inventory(ctx)
	if err != nil {
		return StartupOutcome{}, err
	}
	var selected ProfileRecord
	found := false
	for _, record := range state.records {
		if record.StartSelected {
			selected, found = record, true
			break
		}
	}
	if !found {
		return StartupOutcome{}, nil
	}
	outcome := StartupOutcome{Selected: true, ProfileID: selected.ID, Name: selected.Name}
	if current, _, active := state.active(); active {
		outcome.Active = true
		if current.ID != selected.ID {
			// Something else NILO-owned is already up, which only happens when an
			// administrator connected it before the runner ran. Their choice wins.
			outcome.Waiting = ""
		}
		return outcome, nil
	}
	if selected.ConnectionRef == "" {
		return startupWaiting(selected, "the profile has no connection"), nil
	}
	if manager.options.Uplink != nil {
		if up, reason := manager.options.Uplink(ctx); !up {
			return startupWaiting(selected, reason), nil
		}
	}
	if err := manager.options.Driver.ConnectTunnel(ctx, selected.ConnectionRef); err != nil {
		return outcome, fmt.Errorf("connect the startup profile %s: %w", selected.Name, err)
	}
	outcome.Connected, outcome.Active = true, true
	return outcome, nil
}

// revision digests the inventory so a preview taken before an import, a
// removal or a rename goes stale.
func (state inventory) revision() string {
	digest := sha256.New()
	for _, record := range state.records {
		fmt.Fprintf(digest, "%s\x00%s\x00%s\x00%t\n", record.ID, record.Revision, record.ConnectionRef, record.StartSelected)
	}
	for _, tunnel := range state.tunnels {
		fmt.Fprintf(digest, "tunnel\x00%s\x00%t\x00%t\n", tunnel.ConnectionRef, tunnel.Active, tunnel.NILOOwned)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func profileRevision(profileID string, at time.Time) string {
	digest := sha256.Sum256([]byte(profileID + "\x00" + at.Format(time.RFC3339Nano)))
	return hex.EncodeToString(digest[:8])
}

func validateProfileName(name, profileID string, existing []ProfileRecord) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || len(trimmed) > maxProfileNameLength {
		return fmt.Errorf("%w: a profile name must be between 1 and %d characters", ErrProfileRejected, maxProfileNameLength)
	}
	for _, record := range existing {
		if record.ID != profileID && strings.EqualFold(record.Name, trimmed) {
			return fmt.Errorf("%w: %s", ErrProfileNameTaken, trimmed)
		}
	}
	return nil
}

func routesOf(profile Profile) string {
	routes := make([]string, 0)
	for _, peer := range profile.Peers {
		routes = append(routes, peer.AllowedIPs...)
	}
	return strings.Join(routes, ", ")
}

func endpointsOf(profile Profile) string {
	endpoints := make([]string, 0, len(profile.Peers))
	for _, peer := range profile.Peers {
		if peer.Endpoint != "" {
			endpoints = append(endpoints, peer.Endpoint)
		}
	}
	return strings.Join(endpoints, ", ")
}
