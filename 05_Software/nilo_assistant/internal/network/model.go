// Package network owns NILO's view of the appliance's own networking: the
// interfaces it can see, the traffic they carry, the WireGuard tunnels it
// manages, and the acknowledged, reversible operations that change any of it.
//
// Everything here is expressed against a Provider. The provider is what knows
// whether it is looking at a real host or a deterministic simulation, and it is
// the only thing in this package that can reach either.
package network

// Kind is the sort of device an interface is. It decides what NILO offers for
// it, not how it is measured.
type Kind string

const (
	KindEthernet Kind = "ethernet"
	KindWiFi     Kind = "wifi"
	// KindLoopback and KindOther are reported but never controllable: the
	// loopback device is not the appliance's connection to anything, and an
	// unrecognized device is one NILO has no safe operation for.
	KindLoopback Kind = "loopback"
	KindOther    Kind = "other"
)

// State is an interface's current connection state.
type State string

const (
	StateConnected    State = "connected"
	StateDisconnected State = "disconnected"
	// StateUnavailable means the device is present but carries no usable
	// connection state, which is different from being disconnected.
	StateUnavailable State = "unavailable"
)

// Source names what measured a reading. It is carried on every status and
// statistics response so that a simulated figure can never be mistaken for a
// bench observation.
type Source string

// SourceSimulation is the source every reading carries under the simulation
// runtime profile. A live provider reports the backend host's identity instead.
const SourceSimulation Source = "simulation"

// Interface is one network device as NILO reports it.
type Interface struct {
	// ID is stable across restarts and is what every operation names.
	ID   string
	Name string
	Kind Kind
	// AdapterID is the permanent hardware identity, lower-case, empty when the
	// provider cannot read one. Saved configuration binds to it rather than to
	// the kernel name, which a replug can move.
	AdapterID string
	// Settings reports the saved configuration bound to this adapter, when one
	// exists. Nil means nothing is saved for it.
	Settings *SettingsState

	State State
	IPv4  []string
	IPv6  []string
	// SSID is the wireless network a Wi-Fi adapter is connected to, empty for
	// a wired adapter or a disconnected one.
	SSID string

	// Gateways are current IP configuration observations, empty when absent.
	Gateway4 string
	Gateway6 string

	// LinkCapacityMbps is the negotiated link speed, zero when unknown.
	LinkCapacityMbps uint64

	RXBytesPerSecond float64
	TXBytesPerSecond float64

	// Controllable reports whether NILO will accept an enable or disable for
	// this device. When it is false, NotControllableReason says why, so the
	// interface can explain a missing control rather than just omitting it.
	Controllable          bool
	NotControllableReason string
}

// Status is one complete network reading.
type Status struct {
	Interfaces []Interface
	MeasuredBy Source
	// Unattached is every saved adapter configuration whose adapter is not
	// present: still the operator's decision, just not in force.
	Unattached []InterfaceSettings
}
