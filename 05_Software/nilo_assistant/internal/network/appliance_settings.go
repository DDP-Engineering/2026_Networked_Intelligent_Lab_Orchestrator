package network

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// The live appliance's adapter configuration: saved intent in the database,
// applied state on the helper's devices, management-path protection traced
// from what the helper observes, and a wireless password that reaches nothing
// in this process but the staging call.

type applianceSettings struct {
	mu      sync.Mutex
	store   InterfaceSettingsStore
	pending map[string]InterfaceSettings
	now     func() time.Time
}

func newApplianceSettings() *applianceSettings {
	return &applianceSettings{pending: map[string]InterfaceSettings{}, now: func() time.Time { return time.Now().UTC() }}
}

// UseInterfaceSettingsStore attaches the saved adapter configuration.
func (appliance *Appliance) UseInterfaceSettingsStore(_ context.Context, store InterfaceSettingsStore) error {
	if store == nil {
		return errors.New("an interface settings store is required")
	}
	appliance.settings.mu.Lock()
	defer appliance.settings.mu.Unlock()
	appliance.settings.store = store
	return nil
}

// SavedSettings returns the saved configuration for binding. Without a store
// there is nothing saved.
func (appliance *Appliance) SavedSettings(ctx context.Context) ([]InterfaceSettings, error) {
	appliance.settings.mu.Lock()
	store := appliance.settings.store
	appliance.settings.mu.Unlock()
	if store == nil {
		return []InterfaceSettings{}, nil
	}
	return store.List(ctx)
}

// WiFiScan scans through the helper.
func (appliance *Appliance) WiFiScan(ctx context.Context, interfaceID string) (WiFiScanResult, error) {
	scanner, ok := appliance.base.(WiFiScanner)
	if !ok {
		return WiFiScanResult{}, fmt.Errorf("%w: this helper cannot scan for wireless networks", ErrUnsupported)
	}
	return scanner.WiFiScan(ctx, interfaceID)
}

// adapter resolves the target of a change to the adapter the helper reports.
func (appliance *Appliance) adapter(ctx context.Context, target string) (Interface, error) {
	interfaces, err := appliance.base.Interfaces(ctx)
	if err != nil {
		return Interface{}, err
	}
	for _, candidate := range interfaces {
		if candidate.ID == target {
			return candidate, nil
		}
	}
	return Interface{}, fmt.Errorf("%w: no interface %q", ErrNotControllable, target)
}

// guardManagement refuses a change that would remove the last management
// path, traced from the helper's adapters, the saved protection and the
// active tunnel.
func (appliance *Appliance) guardManagement(ctx context.Context, change Change) error {
	interfaces, err := appliance.base.Interfaces(ctx)
	if err != nil {
		return err
	}
	saved, err := appliance.SavedSettings(ctx)
	if err != nil {
		return err
	}
	bound := BindSettings(interfaces, saved).Interfaces
	return ValidateManagementChange(change, bound, saved, managementContextOf(change, appliance.profiles.ActiveTunnelPath(ctx)))
}

// resolveSettingsChange binds proposed settings to the target adapter and
// validates them, so what reaches the helper names the adapter identity.
func (appliance *Appliance) resolveSettingsChange(ctx context.Context, change Change) (Change, error) {
	adapter, err := appliance.adapter(ctx, change.Target)
	if err != nil {
		return Change{}, err
	}
	switch change.Action {
	case ActionInterfaceSettings:
		settings, err := resolveSettings(change, adapter)
		if err != nil {
			return Change{}, err
		}
		change.Settings = &settings
	case ActionWiFiJoin:
		if change.WiFi == nil {
			return Change{}, fmt.Errorf("%w: a join needs the network to join", ErrInterfaceSettingsRejected)
		}
		if adapter.Kind != KindWiFi || !adapter.Controllable {
			return Change{}, fmt.Errorf("%w: %s is not a wireless adapter", ErrNotControllable, adapter.Name)
		}
		if !knownWiFiSecurity[change.WiFi.Security] {
			return Change{}, fmt.Errorf("%w: unsupported Wi-Fi security %q", ErrInterfaceSettingsRejected, change.WiFi.Security)
		}
		if change.WiFi.Security != WiFiOpen && change.SecretHandle == "" {
			return Change{}, fmt.Errorf("%w: %s needs a password", ErrInterfaceSettingsRejected, change.WiFi.Security)
		}
	}
	return change, nil
}

// rememberIntent keeps the saved intent of a change until it is confirmed.
func (appliance *Appliance) rememberIntent(operationID string, change Change, adapter Interface) {
	var settings InterfaceSettings
	switch change.Action {
	case ActionInterfaceSettings:
		settings = *change.Settings
		if existing, ok := findSettings(appliance.savedOrEmpty(context.Background()), adapter.AdapterID); ok && existing.WiFi != nil {
			settings.WiFi = existing.WiFi
		}
	case ActionWiFiJoin:
		settings = wifiJoinSettings(adapter, *change.WiFi, "nm:"+adapter.AdapterID)
	default:
		return
	}
	appliance.settings.mu.Lock()
	defer appliance.settings.mu.Unlock()
	appliance.settings.pending[operationID] = settings
}

func (appliance *Appliance) savedOrEmpty(ctx context.Context) []InterfaceSettings {
	saved, err := appliance.SavedSettings(ctx)
	if err != nil {
		return nil
	}
	return saved
}

// commitIntent persists the intent of a confirmed change.
func (appliance *Appliance) commitIntent(ctx context.Context, checkpointID string) error {
	appliance.settings.mu.Lock()
	settings, ok := appliance.settings.pending[checkpointID]
	delete(appliance.settings.pending, checkpointID)
	store := appliance.settings.store
	appliance.settings.mu.Unlock()
	if !ok || store == nil {
		return nil
	}
	_, err := store.Save(ctx, settings, appliance.settings.now())
	return err
}

func (appliance *Appliance) dropIntent(checkpointID string) {
	appliance.settings.mu.Lock()
	defer appliance.settings.mu.Unlock()
	delete(appliance.settings.pending, checkpointID)
}
