package network

// EligibleForManagement reports whether one observed interface may carry
// appliance management access.
//
// The rule reads only what a provider recorded about the hardware: the device
// kind, whether the provider saw a physical adapter, and the permanent adapter
// identity. It deliberately never reads Name. A kernel name is assigned by
// whatever enumerated the device and changes when an adapter is replugged or a
// container creates an interface, so a name rule would put management access on
// a container network the first time either happened.
func EligibleForManagement(candidate Interface) bool {
	switch candidate.Kind {
	case KindEthernet, KindWiFi:
	default:
		return false
	}
	return candidate.Physical && canonicalAdapterID(candidate.AdapterID) != ""
}
