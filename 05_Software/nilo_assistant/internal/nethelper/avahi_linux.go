//go:build linux

package nethelper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"nilo.local/nilo/internal/network"
)

// avahiConfigurationPath is the responder configuration NILO owns. It is a
// variable so a system test can point it at a temporary tree.
var avahiConfigurationPath = "/etc/avahi/avahi-daemon.conf"

// avahiReloadCommand restarts the responder after its configuration changes.
var avahiReloadCommand = []string{"/bin/systemctl", "restart", "avahi-daemon"}

// MDNSConfigurationState reads the responder configuration and reports whether
// NILO wrote it. Anything else on the appliance is left strictly alone.
func (n *NetworkManager) MDNSConfigurationState(context.Context) (TableState, error) {
	raw, err := os.ReadFile(avahiConfigurationPath)
	if errors.Is(err, os.ErrNotExist) {
		return avahiConfigurationState("", false), nil
	}
	if err != nil {
		return "", fmt.Errorf("read responder configuration: %w", err)
	}
	return avahiConfigurationState(string(raw), true), nil
}

// InstallMDNS writes the rendered configuration and reloads the responder.
//
// The file is written beside its target and renamed over it, so a responder
// reading it never sees a half-written configuration.
func (n *NetworkManager) InstallMDNS(ctx context.Context, advertisement network.MDNSAdvertisement) error {
	if err := advertisement.Validate(); err != nil {
		return err
	}
	directory := filepath.Dir(avahiConfigurationPath)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("prepare responder configuration directory: %w", err)
	}
	staged, err := os.CreateTemp(directory, ".nilo-avahi-*")
	if err != nil {
		return fmt.Errorf("stage responder configuration: %w", err)
	}
	stagedPath := staged.Name()
	defer func() { _ = os.Remove(stagedPath) }()
	if _, err := staged.WriteString(renderAvahiConfiguration(advertisement)); err != nil {
		_ = staged.Close()
		return fmt.Errorf("write responder configuration: %w", err)
	}
	if err := staged.Chmod(0o644); err != nil {
		_ = staged.Close()
		return fmt.Errorf("set responder configuration mode: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close responder configuration: %w", err)
	}
	if err := os.Rename(stagedPath, avahiConfigurationPath); err != nil {
		return fmt.Errorf("install responder configuration: %w", err)
	}
	command := exec.CommandContext(ctx, avahiReloadCommand[0], avahiReloadCommand[1:]...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("reload responder: %w: %s", err, string(output))
	}
	return nil
}

var _ MDNSDriver = (*NetworkManager)(nil)
