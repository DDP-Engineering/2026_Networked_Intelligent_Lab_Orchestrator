package network

import (
	"context"
	"fmt"
)

// The live appliance's device networks: the shared manager over whichever
// forwarder the runtime supplies. Until a privileged forwarding adapter is
// wired in, the forwarder refuses to install anything, so a saved device
// network is exactly that - saved - and never reported as applied.

// unsupportedForwarder is the live default before the privileged forwarding
// adapter exists. It installs nothing and says so.
type unsupportedForwarder struct{}

func (unsupportedForwarder) ApplyForwarding(context.Context, ForwardingPlan) error {
	return fmt.Errorf("%w: device network forwarding is not available on this appliance yet", ErrUnsupported)
}

func (unsupportedForwarder) RemoveForwarding(context.Context, string) error { return nil }

func (unsupportedForwarder) AppliedForwarding(context.Context) ([]ForwardingPlan, error) {
	return []ForwardingPlan{}, nil
}

// applianceEnvironment is the manager's view of the live appliance.
type applianceEnvironment struct{ appliance *Appliance }

func (env applianceEnvironment) Interfaces(ctx context.Context) ([]Interface, error) {
	interfaces, err := env.appliance.base.Interfaces(ctx)
	if err != nil {
		return nil, err
	}
	saved, err := env.appliance.SavedSettings(ctx)
	if err != nil {
		return nil, err
	}
	return BindSettings(interfaces, saved).Interfaces, nil
}

func (env applianceEnvironment) SavedSettings(ctx context.Context) ([]InterfaceSettings, error) {
	return env.appliance.SavedSettings(ctx)
}

func (env applianceEnvironment) Profiles(ctx context.Context) ([]ProfileStatus, error) {
	return env.appliance.profiles.Profiles(ctx)
}

// UseAudit gives the runtime's own re-plans a journal. It is called before
// the device-network store is attached.
func (appliance *Appliance) UseAudit(journal AuditJournal) { appliance.audit = journal }

// UseForwarder attaches the privileged forwarding adapter. It is called by the
// runtime before the store is attached; without it the unsupported forwarder
// stays in place.
func (appliance *Appliance) UseForwarder(forwarder Forwarder) {
	if forwarder != nil {
		appliance.forwarder = forwarder
	}
}

// UseDeviceNetworkStore attaches the saved device networks and builds the
// manager over the appliance's forwarder. Startup composition explicitly calls
// ReconcileDeviceNetworks only after management roles and firewall are safe.
func (appliance *Appliance) UseDeviceNetworkStore(ctx context.Context, store DeviceNetworkStore) error {
	if appliance.deviceNetworks == nil {
		forwarder := appliance.forwarder
		if forwarder == nil {
			forwarder = unsupportedForwarder{}
		}
		manager, err := NewDeviceNetworkManager(DeviceNetworkManagerOptions{
			Forwarder: forwarder, Environment: applianceEnvironment{appliance: appliance},
			Prober: TCPProber, Audit: appliance.audit, Coordinator: appliance.roleCoordinator,
		})
		if err != nil {
			return err
		}
		appliance.deviceNetworks = manager
		appliance.profiles.options.Dependencies = manager.Dependencies
	}
	if err := appliance.deviceNetworks.UseStore(ctx, store); err != nil {
		return err
	}
	return nil
}

func (appliance *Appliance) forwarderOrUnsupported() Forwarder {
	if appliance.forwarder == nil {
		return unsupportedForwarder{}
	}
	return appliance.forwarder
}

// ReconcileDeviceNetworks brings installed forwarding in step with what is
// present; the runtime calls it on its maintenance cadence.
func (appliance *Appliance) ReconcileDeviceNetworks(ctx context.Context) error {
	if appliance.deviceNetworks == nil {
		return nil
	}
	if _, unsupported := appliance.forwarderOrUnsupported().(unsupportedForwarder); unsupported {
		return nil
	}
	return appliance.deviceNetworks.Reconcile(ctx)
}

func (appliance *Appliance) DeviceNetworks(ctx context.Context) ([]DeviceNetworkStatus, error) {
	if appliance.deviceNetworks == nil {
		return []DeviceNetworkStatus{}, nil
	}
	statuses, err := appliance.deviceNetworks.List(ctx)
	if err != nil {
		return nil, err
	}
	if _, unsupported := appliance.forwarderOrUnsupported().(unsupportedForwarder); unsupported {
		for index := range statuses {
			if statuses[index].StartEnabled {
				statuses[index].State = DeviceNetworkUnsupported
				statuses[index].Detail = "device network forwarding is not available on this appliance yet"
				statuses[index].Applied = false
			}
		}
	}
	return statuses, nil
}

func (appliance *Appliance) DeviceNetwork(ctx context.Context, id string) (DeviceNetworkStatus, error) {
	statuses, err := appliance.DeviceNetworks(ctx)
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

func (appliance *Appliance) CheckDeviceNetwork(ctx context.Context, id string, probe DeviceNetworkProbe) (DeviceNetworkCheck, error) {
	if appliance.deviceNetworks == nil {
		return DeviceNetworkCheck{}, ErrDeviceNetworkNotFound
	}
	return appliance.deviceNetworks.Check(ctx, id, probe)
}

func (appliance *Appliance) AccessInstructions(ctx context.Context, id string) (AccessInstructionSet, error) {
	if appliance.deviceNetworks == nil {
		return AccessInstructionSet{}, ErrDeviceNetworkNotFound
	}
	return appliance.deviceNetworks.AccessInstructions(ctx, id)
}

var _ DeviceNetworkReader = (*Appliance)(nil)
