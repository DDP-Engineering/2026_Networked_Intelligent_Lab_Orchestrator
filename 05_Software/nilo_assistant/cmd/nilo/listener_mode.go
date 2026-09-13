package main

import (
	"errors"
	"fmt"
	"strconv"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/network"
)

// listenerMode is how serve was asked to expose the management interface.
type listenerMode string

const (
	// listenerBind is the legacy single explicit listener address.
	listenerBind listenerMode = "bind"
	// listenerManaged is an administrator's explicit interface allowlist.
	listenerManaged listenerMode = "managed"
	// listenerProvisioning is first boot: no option was given, and the allowlist
	// is derived from the eligible interfaces until the appliance is
	// commissioned.
	listenerProvisioning listenerMode = "provisioning"
)

// errListenerOptionsRequired is the refusal for a commissioned appliance that
// was started with nothing to listen on.
var errListenerOptionsRequired = errors.New(
	"this appliance is commissioned; serve requires --bind or --management-port with persistent management interfaces")

// resolveListenerMode decides what serve was asked to do from its options
// alone. First boot is possible only for the appliance profile: the simulation
// profile has no eligible physical interface to derive from and is confined to
// a literal loopback address.
func resolveListenerMode(profile, bindAddress string, managementMode bool, managementPort int) (listenerMode, string, error) {
	switch {
	case bindAddress != "" && managementMode:
		return "", "", errors.New("--bind cannot be combined with interface-managed listener options")
	case managementMode:
		return listenerManaged, "0.0.0.0:" + strconv.Itoa(managementPort), nil
	case bindAddress != "":
		return listenerBind, bindAddress, nil
	case profile == "appliance":
		// Adopted before the database is open, because the commissioning state
		// cannot be read until then. checkProvisioningPermitted judges it after.
		return listenerProvisioning, "0.0.0.0:" + strconv.Itoa(network.ProvisioningPort), nil
	default:
		return "", "", errors.New("serve requires legacy --bind or --management-port with persistent management interfaces")
	}
}

// checkProvisioningPermitted refuses first-boot serving on an appliance that has
// already been commissioned. The refusal is made here, after the appliance has
// been opened, so it names the real reason rather than a command-line rule.
func checkProvisioningPermitted(mode listenerMode, state appliance.CommissioningState) error {
	if mode != listenerProvisioning {
		return nil
	}
	if state != appliance.CommissioningProvisioning {
		return fmt.Errorf("%w", errListenerOptionsRequired)
	}
	return nil
}
