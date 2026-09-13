package network

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
)

var (
	// ErrAlreadyCommissioned is a second attempt to complete commissioning.
	// The step is one-way: after it, the allowlist changes only through an
	// ordinary management access request.
	ErrAlreadyCommissioned = errors.New("the appliance is already commissioned")
	// ErrCommissioningUnavailable is a service built without anywhere to record
	// the lifecycle state, which must not silently commission in memory only.
	ErrCommissioningUnavailable = errors.New("this service cannot record the appliance lifecycle state")
)

// Commissioner records that the appliance has finished commissioning. The
// management service calls it only after the chosen allowlist is safely in
// force, so a failure here leaves an appliance that is still provisioning
// rather than one that is half-committed.
type Commissioner interface {
	CompleteCommissioning(context.Context) error
}

// Commission fixes the management allowlist to the interfaces an administrator
// chose and records the appliance as commissioned.
//
// It refuses a selection that would leave the appliance unreachable, and it
// refuses to drop the interface the request itself arrived on unless the caller
// has confirmed that deliberately, because that is how an appliance gets locked
// out of its own network.
func (service *ManagementAccessService) Commission(
	ctx context.Context, interfaces []string, localAddress netip.Addr,
) (ManagementAccessStatus, error) {
	return service.commission(ctx, interfaces, false, localAddress)
}

// CommissionConfirmed is Commission with the caller's own arrival interface
// deliberately given up.
func (service *ManagementAccessService) CommissionConfirmed(
	ctx context.Context, interfaces []string, localAddress netip.Addr,
) (ManagementAccessStatus, error) {
	return service.commission(ctx, interfaces, true, localAddress)
}

func (service *ManagementAccessService) commission(
	ctx context.Context, interfaces []string, confirmCurrent bool, localAddress netip.Addr,
) (ManagementAccessStatus, error) {
	service.changeMu.Lock()
	defer service.changeMu.Unlock()

	service.mu.RLock()
	provisioning := service.provisioning
	service.mu.RUnlock()
	if !provisioning {
		return ManagementAccessStatus{}, ErrAlreadyCommissioned
	}
	if service.commissioner == nil {
		return ManagementAccessStatus{}, ErrCommissioningUnavailable
	}
	if err := service.refresh(ctx); err != nil {
		return ManagementAccessStatus{}, err
	}

	service.mu.Lock()
	previous := service.record
	observed := cloneInterfaces(service.interfaces)
	current := service.currentInterfaceLocked(localAddress)
	service.mu.Unlock()

	port := previous.Configuration.Port
	if port == 0 {
		port = ProvisioningPort
	}
	chosen := canonicalManagementConfiguration(resolveManagementConfiguration(
		ManagementAccessConfiguration{
			Interfaces: append([]string(nil), interfaces...), Port: port,
			TLSNames: append([]string(nil), service.provisionTLS...),
		}, observed))
	if err := chosen.Validate(); err != nil {
		// A selection the configuration rejects - empty, malformed, or the
		// loopback device - is the administrator's input being refused, not the
		// service failing, so it carries the refusal sentinel the interface
		// reports as a conflict rather than an outage.
		return ManagementAccessStatus{}, fmt.Errorf("%w: %v", ErrManagementAccessUnsafe, err)
	}
	// Every chosen interface has to be one that may carry management access at
	// all, so commissioning cannot pin the appliance to a container interface.
	eligible := map[string]bool{}
	for _, candidate := range observed {
		if EligibleForManagement(candidate) {
			eligible[candidate.Name] = true
		}
	}
	for _, name := range chosen.Interfaces {
		if !eligible[name] {
			return ManagementAccessStatus{}, fmt.Errorf(
				"%w: %s cannot carry management access", ErrManagementAccessUnsafe, name)
		}
	}
	if !managementConfigurationHasUsableInterface(observed, chosen.Interfaces) {
		return ManagementAccessStatus{}, ErrManagementAccessUnsafe
	}
	if current != "" && !containsString(chosen.Interfaces, current) && !confirmCurrent {
		return ManagementAccessStatus{}, fmt.Errorf("%w: %s", ErrCurrentAccessConfirmation, current)
	}

	committed := ManagementAccessRecord{
		Configuration: chosen, LastKnownGood: chosen,
		State: ManagementAccessApplied, UpdatedAt: service.now().UTC(),
	}
	if err := service.coordinator.CheckAndRun(managementRecordRoleClaims(committed), func() error {
		if err := service.firewall.ApplyManagementAccess(ctx, chosen); err != nil {
			return err
		}
		if err := service.store.SaveManagementAccess(ctx, committed); err != nil {
			return err
		}
		service.mu.Lock()
		service.record = committed
		service.provisioning = false
		service.rebuildAllowedLocked()
		service.mu.Unlock()
		return nil
	}); err != nil {
		service.restoreProvisioning(ctx, previous)
		return ManagementAccessStatus{}, fmt.Errorf("apply the commissioned management access: %w", err)
	}

	// The lifecycle state is recorded last, because it is the only step whose
	// failure can be undone completely: the allowlist above is still a valid
	// provisioning allowlist, and restoring it leaves the appliance exactly
	// where it was.
	if err := service.commissioner.CompleteCommissioning(ctx); err != nil {
		service.mu.Lock()
		service.provisioning = true
		service.mu.Unlock()
		service.restoreProvisioning(ctx, previous)
		return ManagementAccessStatus{}, fmt.Errorf("record the appliance as commissioned: %w", err)
	}
	return service.status(localAddress), nil
}

// restoreProvisioning puts back the allowlist the appliance was serving before
// a transition that did not complete, so a failed commissioning costs nothing.
func (service *ManagementAccessService) restoreProvisioning(ctx context.Context, previous ManagementAccessRecord) {
	restore := context.WithoutCancel(ctx)
	if len(previous.Configuration.Interfaces) > 0 {
		if err := service.firewall.ApplyManagementAccess(restore, previous.Configuration); err != nil {
			return
		}
		_ = service.store.SaveManagementAccess(restore, previous)
	}
	service.mu.Lock()
	service.record = previous
	service.rebuildAllowedLocked()
	service.mu.Unlock()
}
