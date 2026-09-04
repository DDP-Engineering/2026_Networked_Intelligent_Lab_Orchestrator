package network

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// The simulation's interface configuration and Wi-Fi. It keeps the same
// shape as the appliance: saved intent in a store, applied state on the
// simulated adapter, and the two reported apart.

// simulatedWiFiNetworks is what a scan finds on the simulated bench: a
// personal hotspot, an open network and an enterprise network this milestone
// does not join, listed as such.
var simulatedWiFiNetworks = []WiFiNetwork{
	{SSID: "lab-hotspot", Security: WiFiWPA2Personal, Strength: 72, Supported: true},
	{SSID: "open-cafe", Security: WiFiOpen, Strength: 41, Supported: true},
	{SSID: "site-gateway", Security: WiFiWPA3Personal, Strength: 58, Supported: true},
	{SSID: "corp-enterprise", Security: WiFiSecurityUnsupported, Strength: 63, Supported: false},
}

// simulatedLease is the address an automatic configuration or a join gets.
func simulatedLease(kind Kind) (address, gateway string) {
	if kind == KindWiFi {
		return "192.168.20.5/24", "192.168.20.1"
	}
	return "192.168.10.24/24", "192.168.10.1"
}

// UseInterfaceSettingsStore attaches the saved adapter configuration and
// applies the saved intent to the simulated adapters, which is what the
// appliance runtime does at start.
func (s *Simulation) UseInterfaceSettingsStore(ctx context.Context, store InterfaceSettingsStore) error {
	if store == nil {
		return errors.New("an interface settings store is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settingsStore = store
	saved, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("read saved interface settings: %w", err)
	}
	s.settings = saved
	for _, settings := range saved {
		if settings.WiFi != nil && !settings.WiFi.ConnectAtStartup {
			// A wireless network the operator did not select for startup is
			// remembered, not joined, when the appliance starts.
			continue
		}
		for index := range s.interfaces {
			if strings.EqualFold(s.interfaces[index].AdapterID, settings.AdapterID) {
				s.applySettings(&s.interfaces[index], settings)
			}
		}
	}
	return nil
}

// SavedSettings returns the saved configuration for binding.
func (s *Simulation) SavedSettings(context.Context) ([]InterfaceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]InterfaceSettings(nil), s.settings...), nil
}

// applySettings puts saved intent onto a simulated adapter.
func (s *Simulation) applySettings(item *Interface, settings InterfaceSettings) {
	switch settings.Method {
	case MethodStatic:
		item.IPv4 = []string{settings.Address.String()}
		item.Gateway4 = addrText(settings.Gateway)
	case MethodAutomatic:
		address, gateway := simulatedLease(item.Kind)
		item.IPv4 = []string{address}
		item.Gateway4 = gateway
	}
	if settings.WiFi != nil {
		item.SSID = settings.WiFi.SSID
		item.State = StateConnected
		item.LinkCapacityMbps = 300
		item.RXBytesPerSecond, item.TXBytesPerSecond = 5120, 1280
	}
}

func (s *Simulation) interfaceByID(id string) (*Interface, bool) {
	for index := range s.interfaces {
		if s.interfaces[index].ID == id {
			return &s.interfaces[index], true
		}
	}
	return nil, false
}

// guardManagement refuses a change that would remove the last management
// path. Called with the lock held.
func (s *Simulation) guardManagement(c Change) error {
	bound := BindSettings(s.interfaces, s.settings).Interfaces
	return ValidateManagementChange(c, bound, s.settings, managementContextOf(c, nil))
}

// validateSettings checks a settings change against the simulated adapter.
func (s *Simulation) validateSettings(c Change) (InterfaceSettings, error) {
	item, ok := s.interfaceByID(c.Target)
	if !ok {
		return InterfaceSettings{}, ErrNotControllable
	}
	settings, err := resolveSettings(c, *item)
	if err != nil {
		return InterfaceSettings{}, err
	}
	return settings, s.guardManagement(c)
}

func (s *Simulation) validateJoin(c Change) error {
	item, ok := s.interfaceByID(c.Target)
	if !ok {
		return ErrNotControllable
	}
	if err := validateJoin(c.WiFi, *item, simulatedWiFiNetworks); err != nil {
		return err
	}
	return s.guardManagement(c)
}

// WiFiScan lists the simulated networks for a wireless adapter.
func (s *Simulation) WiFiScan(_ context.Context, interfaceID string) (WiFiScanResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.interfaceByID(interfaceID)
	if !ok {
		return WiFiScanResult{}, ErrNotControllable
	}
	if item.Kind != KindWiFi {
		return WiFiScanResult{}, fmt.Errorf("%w: %s is not a wireless adapter", ErrUnsupported, item.Name)
	}
	return WiFiScanResult{
		InterfaceID: interfaceID, Supported: true, ObservedAt: s.now().UTC(),
		Networks: append([]WiFiNetwork(nil), simulatedWiFiNetworks...),
	}, nil
}

// applySettingsChange makes a settings or join change on the simulated adapter
// and remembers the intent to persist once the change is confirmed.
func (s *Simulation) applySettingsChange(c Change) error {
	item, ok := s.interfaceByID(c.Target)
	if !ok {
		return ErrNotControllable
	}
	var settings InterfaceSettings
	switch c.Action {
	case ActionInterfaceSettings:
		resolved, err := s.validateSettings(c)
		if err != nil {
			return err
		}
		settings = resolved
		if existing, ok := findSettings(s.settings, item.AdapterID); ok && existing.WiFi != nil && settings.WiFi == nil {
			// Re-addressing a wireless adapter keeps the network it is on.
			settings.WiFi = existing.WiFi
		}
		if settings.WiFi == nil {
			item.SSID = ""
		}
	case ActionWiFiJoin:
		if err := s.validateJoin(c); err != nil {
			return err
		}
		settings = wifiJoinSettings(*item, *c.WiFi, "simulated")
	default:
		return ErrUnsupported
	}
	s.applySettings(item, settings)
	s.pendingSettings[s.checkpoint] = append(s.pendingSettings[s.checkpoint], settings)
	if s.checkpoint == "" {
		// Applied outside a checkpoint, as the unit tests do: the intent is
		// saved at once.
		return s.persistSettings(context.Background(), settings)
	}
	return nil
}

// persistSettings writes one saved configuration and refreshes the cache.
func (s *Simulation) persistSettings(ctx context.Context, settings InterfaceSettings) error {
	now := s.now().UTC()
	if s.settingsStore != nil {
		saved, err := s.settingsStore.Save(ctx, settings, now)
		if err != nil {
			return err
		}
		settings = saved
	} else {
		settings.Revision = settings.ContentRevision()
		settings.UpdatedAt = now
	}
	replaced := false
	for index := range s.settings {
		if strings.EqualFold(s.settings[index].AdapterID, settings.AdapterID) {
			s.settings[index] = settings
			replaced = true
		}
	}
	if !replaced {
		s.settings = append(s.settings, settings)
	}
	return nil
}

// commitSettings persists the intent of a confirmed change. Called with the
// lock held once the checkpoint is released.
func (s *Simulation) commitSettings(ctx context.Context, checkpoint string) error {
	pending := s.pendingSettings[checkpoint]
	delete(s.pendingSettings, checkpoint)
	for _, settings := range pending {
		if err := s.persistSettings(ctx, settings); err != nil {
			return err
		}
	}
	return nil
}

func (s *Simulation) dropPendingSettings(checkpoint string) {
	delete(s.pendingSettings, checkpoint)
}

var _ time.Duration
