package network

import "context"

// SimulatedProvider answers every network read from a fixed catalog.
//
// It deliberately holds no field that refers to anything: no privileged helper
// client, no D-Bus connection, no command runner, no file handle. That is what
// makes the simulation profile safe on a developer's workstation — not a flag
// checked at call time, but the absence of anything to call. A test asserts
// this structurally, so the guarantee cannot be lost by adding a field.
type SimulatedProvider struct{}

// NewSimulatedProvider returns the deterministic catalog used by the simulation
// runtime profile.
func NewSimulatedProvider() *SimulatedProvider {
	return &SimulatedProvider{}
}

// Source reports that every reading from this provider is simulated.
func (provider *SimulatedProvider) Source() Source { return SourceSimulation }

// Interfaces returns the catalog. The same call always returns the same
// answer, which is what lets a browser test assert an exact figure.
func (provider *SimulatedProvider) Interfaces(context.Context) ([]Interface, error) {
	copied := make([]Interface, 0, len(simulatedInterfaces))
	for _, candidate := range simulatedInterfaces {
		clone := candidate
		clone.IPv4 = append([]string(nil), candidate.IPv4...)
		clone.IPv6 = append([]string(nil), candidate.IPv6...)
		copied = append(copied, clone)
	}
	return copied, nil
}

// simulatedInterfaces is the fixed bench-shaped catalog: one wired device, one
// wireless device that is present but not connected, and the loopback device
// that exists on every host and is never controllable.
var simulatedInterfaces = []Interface{
	{
		ID:               "sim-enp1s0",
		Name:             "enp1s0",
		AdapterID:        "02:00:00:00:00:01",
		Kind:             KindEthernet,
		State:            StateConnected,
		IPv4:             []string{"192.168.10.24/24"},
		IPv6:             []string{"fd00:10::24/64"},
		Gateway4:         "192.168.10.1",
		Gateway6:         "fd00:10::1",
		LinkCapacityMbps: 1000,
		RXBytesPerSecond: 12480,
		TXBytesPerSecond: 3120,
		Controllable:     true,
	},
	{
		ID:               "sim-wlan0",
		Name:             "wlan0",
		AdapterID:        "02:00:00:00:00:02",
		Kind:             KindWiFi,
		State:            StateDisconnected,
		LinkCapacityMbps: 0,
		Controllable:     true,
	},
	{
		ID:                    "sim-lo",
		Name:                  "lo",
		Kind:                  KindLoopback,
		State:                 StateConnected,
		IPv4:                  []string{"127.0.0.1/8"},
		IPv6:                  []string{"::1/128"},
		Controllable:          false,
		NotControllableReason: "the loopback device is not an appliance connection",
	},
}
