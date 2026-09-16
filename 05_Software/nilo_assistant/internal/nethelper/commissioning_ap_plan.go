package nethelper

import "nilo.local/nilo/internal/network"

// retainedCommissioningAccessPoint reconstructs desired AP state only from
// the exact helper-owned durable record. It never infers ownership from a
// connection name, active SSID, or caller-supplied adapter identity.
func retainedCommissioningAccessPoint(plan network.CommissioningAccessPoint, ownership CommissioningProfileOwnership) (network.CommissioningAccessPoint, bool) {
	record := ownership.AccessPoint
	if !plan.RetainOwned || record == nil || record.SSID != plan.SerialName ||
		!validPermanentSerialName(record.SSID) || record.AdapterID == "" || record.Interface == "" {
		return network.CommissioningAccessPoint{}, false
	}
	return network.CommissioningAccessPoint{
		Enabled: true, SerialName: record.SSID, AdapterID: record.AdapterID,
		Interface: record.Interface, Address: plan.Address, Port: plan.Port,
	}, true
}
