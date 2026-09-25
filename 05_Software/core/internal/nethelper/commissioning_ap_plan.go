package nethelper

import (
	"context"
	"errors"
	"strings"

	"nilo.local/nilo/internal/network"
)

// retainedCommissioningAccessPoint reconstructs desired AP state only from
// the exact helper-owned durable record. It never infers ownership from a
// connection name, active SSID, or caller-supplied adapter identity.
func retainedCommissioningAccessPoint(plan network.CommissioningAccessPoint, ownership CommissioningProfileOwnership) (network.CommissioningAccessPoint, bool) {
	record := ownership.AccessPoint
	if !plan.RetainOwned || record == nil || record.SSID != plan.SerialName ||
		!validPermanentSerialName(record.SSID) || record.AdapterID == "" || record.Interface == "" ||
		!eligibleCommissioningAPAdapter(plan.EligibleAdapterIDs, record.AdapterID) {
		return network.CommissioningAccessPoint{}, false
	}
	return network.CommissioningAccessPoint{
		Enabled: true, SerialName: record.SSID, AdapterID: record.AdapterID,
		Interface: record.Interface, Address: plan.Address, Port: plan.Port,
	}, true
}

func eligibleCommissioningAPAdapter(eligible []string, adapterID string) bool {
	for _, candidate := range eligible {
		if strings.EqualFold(candidate, adapterID) {
			return true
		}
	}
	return false
}

func commissioningAPDeviceEligible(device Device, ownedUUID string) bool {
	if device.Interface.Kind != network.KindWiFi || !device.Interface.Physical || !device.Interface.Controllable || device.Transitioning {
		return false
	}
	if device.Connection != "" && device.Connection != ownedUUID {
		return false
	}
	for _, available := range device.Available {
		if available != ownedUUID {
			return false
		}
	}
	return true
}

// commissioningAPRollback reverses only state created by the current start
// attempt. Existing durable ownership and profiles are never attached to these
// callbacks and therefore cannot be removed by a failed reconciliation.
type commissioningAPRollback struct {
	stopServices      func(context.Context) error
	deactivateProfile func(context.Context) error
	deleteProfile     func(context.Context) error
	restoreOwnership  func() error
}

func (rollback commissioningAPRollback) Run(ctx context.Context) error {
	cleanupContext := context.WithoutCancel(ctx)
	var cleanupErrs []error
	if rollback.stopServices != nil {
		cleanupErrs = append(cleanupErrs, rollback.stopServices(cleanupContext))
	}
	if rollback.deactivateProfile != nil {
		cleanupErrs = append(cleanupErrs, rollback.deactivateProfile(cleanupContext))
	}
	if rollback.deleteProfile != nil {
		cleanupErrs = append(cleanupErrs, rollback.deleteProfile(cleanupContext))
	}
	if rollback.restoreOwnership != nil {
		cleanupErrs = append(cleanupErrs, rollback.restoreOwnership())
	}
	return errors.Join(cleanupErrs...)
}
