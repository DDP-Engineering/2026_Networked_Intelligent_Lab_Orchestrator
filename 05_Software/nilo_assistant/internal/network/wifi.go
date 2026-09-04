package network

import (
	"context"
	"fmt"
	"net/netip"
	"time"
)

// WiFiNetwork is one network a scan found. Security is reported as observed;
// a network this milestone cannot join, enterprise or WEP, is listed with
// Supported false rather than hidden, so an operator learns why it is missing.
type WiFiNetwork struct {
	SSID      string       `json:"ssid"`
	Security  WiFiSecurity `json:"security"`
	Strength  int          `json:"strength"`
	Supported bool         `json:"supported"`
}

// WiFiSecurityUnsupported labels a network whose security this milestone does
// not join.
const WiFiSecurityUnsupported WiFiSecurity = "unsupported"

// WiFiScanResult is one adapter's scan.
type WiFiScanResult struct {
	InterfaceID string
	// Supported reports whether the adapter can scan at all.
	Supported  bool
	Networks   []WiFiNetwork
	ObservedAt time.Time
}

// WiFiScanner is a provider that can scan for wireless networks.
type WiFiScanner interface {
	WiFiScan(ctx context.Context, interfaceID string) (WiFiScanResult, error)
}

// WiFiScan scans through the provider. A provider without wireless support
// answers unsupported rather than an empty list.
func (service *Service) WiFiScan(ctx context.Context, interfaceID string) (WiFiScanResult, error) {
	scanner, ok := service.provider.(WiFiScanner)
	if !ok {
		return WiFiScanResult{}, fmt.Errorf("%w: this runtime cannot scan for wireless networks", ErrUnsupported)
	}
	return scanner.WiFiScan(ctx, interfaceID)
}

// SavedSettingsReader is a provider that holds saved adapter configuration.
type SavedSettingsReader interface {
	SavedSettings(ctx context.Context) ([]InterfaceSettings, error)
}

// managementContextOf builds the trace context from a change's origin and the
// active tunnel, if any.
func managementContextOf(change Change, tunnel *TunnelPath) ManagementContext {
	context := ManagementContext{Tunnel: tunnel}
	if address, err := netip.ParseAddr(change.Origin); err == nil {
		context.ClientAddress = address
	}
	return context
}

// tunnelPathOf describes an active NILO-owned profile for the trace.
func tunnelPathOf(record ProfileRecord, tunnel TunnelObservation) *TunnelPath {
	path := &TunnelPath{ProfileID: record.ID, Name: record.Name, Interface: tunnel.Interface}
	for _, peer := range record.Profile.Peers {
		for _, allowed := range peer.AllowedIPs {
			if prefix, err := netip.ParsePrefix(allowed); err == nil {
				path.Routes = append(path.Routes, prefix)
			}
		}
	}
	return path
}

// resolveSettings binds proposed settings to the target adapter's identity and
// validates them. The caller supplies the adapter it resolved from the target.
func resolveSettings(change Change, adapter Interface) (InterfaceSettings, error) {
	if change.Settings == nil {
		return InterfaceSettings{}, fmt.Errorf("%w: a settings change needs the proposed configuration", ErrInterfaceSettingsRejected)
	}
	if adapter.Kind != KindEthernet && adapter.Kind != KindWiFi || !adapter.Controllable {
		return InterfaceSettings{}, fmt.Errorf("%w: %s cannot be configured", ErrNotControllable, adapter.Name)
	}
	settings := *change.Settings
	settings.AdapterID = adapter.AdapterID
	if settings.AdapterID == "" {
		return InterfaceSettings{}, fmt.Errorf("%w: %s reports no stable adapter identity to bind to", ErrInterfaceSettingsRejected, adapter.Name)
	}
	if err := settings.Validate(); err != nil {
		return InterfaceSettings{}, err
	}
	return settings, nil
}

// wifiJoinSettings is the saved intent a join persists once confirmed.
func wifiJoinSettings(adapter Interface, join WiFiJoin, secretRef string) InterfaceSettings {
	settings := InterfaceSettings{
		AdapterID: adapter.AdapterID, Method: MethodAutomatic, DNSAutomatic: true,
		WiFi: &WiFiSettings{SSID: join.SSID, Security: join.Security, ConnectAtStartup: join.ConnectAtStartup},
	}
	if join.Security != WiFiOpen {
		settings.WiFi.SecretRef = secretRef
	}
	return settings
}

// validateJoin checks a join against what a scan found.
func validateJoin(join *WiFiJoin, adapter Interface, networks []WiFiNetwork) error {
	if join == nil {
		return fmt.Errorf("%w: a join needs the network to join", ErrInterfaceSettingsRejected)
	}
	if adapter.Kind != KindWiFi || !adapter.Controllable {
		return fmt.Errorf("%w: %s is not a wireless adapter", ErrNotControllable, adapter.Name)
	}
	if !knownWiFiSecurity[join.Security] {
		return fmt.Errorf("%w: unsupported Wi-Fi security %q", ErrInterfaceSettingsRejected, join.Security)
	}
	for _, found := range networks {
		if found.SSID != join.SSID {
			continue
		}
		if !found.Supported {
			return fmt.Errorf("%w: %s uses security this appliance does not join", ErrUnsupported, join.SSID)
		}
		if found.Security != join.Security {
			return fmt.Errorf("%w: %s uses %s, not %s", ErrInterfaceSettingsRejected, join.SSID, found.Security, join.Security)
		}
		return nil
	}
	return fmt.Errorf("%w: no network named %s was found", ErrInterfaceSettingsRejected, join.SSID)
}
