package nethelper

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nilo.local/nilo/internal/network"
)

// The settings half of the privileged engine: adapter configuration and
// wireless joins, each an interface change checkpointed against its device.
// A wireless password reaches the driver from the keystore at the moment of
// the join and the staged copy is discarded in the same step.

// SettingsDriver is what the NetworkManager adapter provides for configuration.
type SettingsDriver interface {
	ApplySettings(ctx context.Context, device Device, settings network.InterfaceSettings) error
	ScanWiFi(ctx context.Context, device Device) ([]network.WiFiNetwork, error)
	JoinWiFi(ctx context.Context, device Device, join network.WiFiJoin, password string) error
}

// ErrSettingsUnsupported means this helper's driver cannot configure adapters.
var ErrSettingsUnsupported = errors.New("this helper cannot configure adapters")

func isSettingsAction(action network.Action) bool {
	return action == network.ActionInterfaceSettings || action == network.ActionWiFiJoin
}

func (e *Engine) settingsDriver() (SettingsDriver, error) {
	driver, ok := e.driver.(SettingsDriver)
	if !ok {
		return nil, ErrSettingsUnsupported
	}
	return driver, nil
}

// controllableDevice finds the device a change names and refuses anything
// that is not a managed wired or wireless adapter at rest.
func (e *Engine) controllableDevice(ctx context.Context, target string) (Device, error) {
	devices, err := e.driver.Devices(ctx)
	if err != nil {
		return Device{}, err
	}
	for _, d := range devices {
		if d.Interface.ID != target {
			continue
		}
		if !d.Interface.Controllable || (d.Interface.Kind != network.KindEthernet && d.Interface.Kind != network.KindWiFi) {
			return Device{}, network.ErrNotControllable
		}
		if d.Transitioning {
			return Device{}, network.ErrPreviewStale
		}
		return d, nil
	}
	return Device{}, network.ErrNotControllable
}

// inspectSettings validates a settings or join change against its device.
func (e *Engine) inspectSettings(ctx context.Context, c network.Change) (Device, network.Change, error) {
	if _, err := e.settingsDriver(); err != nil {
		return Device{}, c, err
	}
	if c.ProfileText != "" || c.Enabled || c.Name != "" || c.Profile != nil || c.ConnectionID != "" {
		return Device{}, c, network.ErrUnsupported
	}
	device, err := e.controllableDevice(ctx, c.Target)
	if err != nil {
		return Device{}, c, err
	}
	switch c.Action {
	case network.ActionInterfaceSettings:
		if c.Settings == nil || c.WiFi != nil || c.SecretHandle != "" {
			return Device{}, c, fmt.Errorf("%w: a settings change needs the proposed configuration", network.ErrInterfaceSettingsRejected)
		}
		if device.Interface.AdapterID != "" && !strings.EqualFold(c.Settings.AdapterID, device.Interface.AdapterID) {
			return Device{}, c, fmt.Errorf("%w: the configuration belongs to adapter %s, not to %s",
				network.ErrInterfaceSettingsRejected, c.Settings.AdapterID, device.Interface.Name)
		}
		if err := c.Settings.Validate(); err != nil {
			return Device{}, c, err
		}
	case network.ActionWiFiJoin:
		if c.WiFi == nil || c.Settings != nil {
			return Device{}, c, fmt.Errorf("%w: a join needs the network to join", network.ErrInterfaceSettingsRejected)
		}
		if device.Interface.Kind != network.KindWiFi {
			return Device{}, c, fmt.Errorf("%w: %s is not a wireless adapter", network.ErrNotControllable, device.Interface.Name)
		}
		if err := (network.InterfaceSettings{AdapterID: device.Interface.ID, Method: network.MethodAutomatic, DNSAutomatic: true,
			WiFi: &network.WiFiSettings{SSID: c.WiFi.SSID, Security: c.WiFi.Security, SecretRef: stagedReference(c)}}).Validate(); err != nil {
			return Device{}, c, err
		}
		if c.SecretHandle != "" {
			keystore, err := e.keysLocked()
			if err != nil {
				return Device{}, c, err
			}
			secrets, err := keystore.ReadStaged(c.SecretHandle)
			if err != nil {
				return Device{}, c, fmt.Errorf("%w: the staged password is gone; enter it again", network.ErrPreviewStale)
			}
			if secrets.Password == "" {
				return Device{}, c, fmt.Errorf("%w: the staged record is not a password", network.ErrInterfaceSettingsRejected)
			}
		}
	default:
		return Device{}, c, network.ErrUnsupported
	}
	return device, c, nil
}

// stagedReference turns a handle into the opaque reference the settings
// validation expects, so an open network is refused a password and a personal
// one is refused without.
func stagedReference(c network.Change) string {
	if c.SecretHandle == "" {
		return ""
	}
	return "staged"
}

// applySettings makes the recorded configuration change on its device.
func (e *Engine) applySettings(ctx context.Context, r *Record) error {
	driver, err := e.settingsDriver()
	if err != nil {
		return err
	}
	switch r.Change.Action {
	case network.ActionInterfaceSettings:
		return driver.ApplySettings(ctx, r.Before, *r.Change.Settings)
	case network.ActionWiFiJoin:
		password := ""
		if r.Change.SecretHandle != "" {
			keystore, err := e.keysLocked()
			if err != nil {
				return err
			}
			secrets, err := keystore.ReadStaged(r.Change.SecretHandle)
			if err != nil {
				return fmt.Errorf("read the staged password: %w", err)
			}
			password = secrets.Password
			// Consumed now, whether or not the join succeeds: a retry stages
			// the password again rather than reusing a record that outlived
			// its change.
			defer func() { _ = keystore.Discard(r.Change.SecretHandle) }()
		}
		return driver.JoinWiFi(ctx, r.Before, *r.Change.WiFi, password)
	default:
		return network.ErrUnsupported
	}
}

// WiFiScan scans on one wireless device. It changes nothing but the radio's
// scan state.
func (e *Engine) WiFiScan(ctx context.Context, target string) ([]network.WiFiNetwork, error) {
	driver, err := e.settingsDriver()
	if err != nil {
		return nil, err
	}
	devices, err := e.driver.Devices(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if d.Interface.ID != target {
			continue
		}
		if d.Interface.Kind != network.KindWiFi {
			return nil, fmt.Errorf("%w: %s is not a wireless adapter", network.ErrUnsupported, d.Interface.Name)
		}
		networks, err := driver.ScanWiFi(ctx, d)
		if err != nil {
			return nil, err
		}
		if networks == nil {
			networks = []network.WiFiNetwork{}
		}
		return networks, nil
	}
	return nil, network.ErrNotControllable
}
