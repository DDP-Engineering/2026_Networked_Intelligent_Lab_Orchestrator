package network

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"time"
)

// A connection check reports separate facts and never turns an unanswered
// probe into a claim that the device is offline. Access instructions name the
// machine each step belongs to and carry no key.

// FactResult is the outcome of one fact.
type FactResult string

const (
	FactPassed  FactResult = "passed"
	FactFailed  FactResult = "failed"
	FactUnknown FactResult = "unknown"
)

// DeviceNetworkFact is one independently reported observation.
type DeviceNetworkFact struct {
	Name   string     `json:"name"`
	Result FactResult `json:"result"`
	Detail string     `json:"detail"`
}

// DeviceNetworkProbe bounds what a check may do: only the configured target,
// optionally on one TCP port.
type DeviceNetworkProbe struct {
	TCPPort int
}

// DeviceNetworkCheck is the result of one check.
type DeviceNetworkCheck struct {
	DeviceNetworkID string
	Facts           []DeviceNetworkFact
	ObservedAt      time.Time
}

// ProbeTimeout bounds one target probe, as the contract states.
const ProbeTimeout = 10 * time.Second

// Check reports the separate facts of one device network.
func (manager *DeviceNetworkManager) Check(ctx context.Context, id string, probe DeviceNetworkProbe) (DeviceNetworkCheck, error) {
	if probe.TCPPort < 0 || probe.TCPPort > 65535 {
		return DeviceNetworkCheck{}, reject("port %d is outside the valid port range", probe.TCPPort)
	}
	state, err := manager.surroundings(ctx)
	if err != nil {
		return DeviceNetworkCheck{}, err
	}
	record, ok := state.record(id)
	if !ok {
		return DeviceNetworkCheck{}, fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, id)
	}
	check := DeviceNetworkCheck{DeviceNetworkID: id, ObservedAt: manager.options.Now().UTC()}

	adapter, present := state.adapter(record.DeviceInterface)
	switch {
	case !present:
		check.Facts = append(check.Facts, DeviceNetworkFact{"adapter", FactFailed, fmt.Sprintf("no adapter with the identity %s is present", record.DeviceInterface)})
	case adapter.State != StateConnected:
		check.Facts = append(check.Facts, DeviceNetworkFact{"adapter", FactFailed, fmt.Sprintf("%s is present but has no link", adapter.Name)})
	default:
		check.Facts = append(check.Facts, DeviceNetworkFact{"adapter", FactPassed, fmt.Sprintf("%s is present with a link", adapter.Name)})
	}
	configured := false
	if present {
		for _, address := range adapter.IPv4 {
			if address == record.ApplianceAddress.String() {
				configured = true
			}
		}
	}
	if configured {
		check.Facts = append(check.Facts, DeviceNetworkFact{"configuration", FactPassed, fmt.Sprintf("%s carries the appliance address %s", adapter.Name, record.ApplianceAddress)})
	} else {
		check.Facts = append(check.Facts, DeviceNetworkFact{"configuration", FactFailed, fmt.Sprintf("the appliance address %s is not applied on the device-side adapter", record.ApplianceAddress)})
	}
	if record.Access == AccessWireGuard || record.Access == AccessBoth {
		profile, ok := state.profile(record.VPNProfileID)
		switch {
		case !ok:
			check.Facts = append(check.Facts, DeviceNetworkFact{"tunnel", FactFailed, fmt.Sprintf("the profile %s is not saved", record.VPNProfileID)})
		case !profile.Active:
			check.Facts = append(check.Facts, DeviceNetworkFact{"tunnel", FactFailed, fmt.Sprintf("the profile %s is not connected", profile.Name)})
		case profile.LastHandshake == nil:
			check.Facts = append(check.Facts, DeviceNetworkFact{"tunnel", FactUnknown, fmt.Sprintf("the profile %s is connected but no handshake has been observed", profile.Name)})
		default:
			check.Facts = append(check.Facts, DeviceNetworkFact{"tunnel", FactPassed, fmt.Sprintf("the profile %s is connected; last handshake %s", profile.Name, profile.LastHandshake.UTC().Format(time.RFC3339))})
		}
	} else {
		check.Facts = append(check.Facts, DeviceNetworkFact{"tunnel", FactUnknown, "this device network does not use a tunnel"})
	}
	target := record.Target
	if record.Scope == ScopeSubnet {
		check.Facts = append(check.Facts, DeviceNetworkFact{"target", FactUnknown, "a whole-subnet network names no single target to probe"})
	} else {
		port := probe.TCPPort
		if port == 0 && len(record.Services.TCPPorts) == 1 {
			port = record.Services.TCPPorts[0]
		}
		if manager.options.Prober == nil {
			check.Facts = append(check.Facts, DeviceNetworkFact{"target", FactUnknown, "no probe is available on this runtime; this says nothing about whether the device is online"})
		} else {
			probeCtx, cancel := context.WithTimeout(ctx, ProbeTimeout)
			fact := manager.options.Prober(probeCtx, target, port)
			cancel()
			fact.Name = "target"
			check.Facts = append(check.Facts, fact)
		}
	}
	return check, nil
}

// TCPProber dials the configured target from this appliance. It is the
// unprivileged probe: a completed connect is evidence of reachability, and
// anything else is unknown, because a filtered port and a dead device look the
// same from here.
func TCPProber(ctx context.Context, target netip.Addr, tcpPort int) DeviceNetworkFact {
	if tcpPort == 0 {
		return DeviceNetworkFact{Result: FactUnknown, Detail: "no TCP port is configured or requested, so the target was not probed; this says nothing about whether the device is online"}
	}
	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(target.String(), strconv.Itoa(tcpPort)))
	if err != nil {
		return DeviceNetworkFact{Result: FactUnknown, Detail: fmt.Sprintf("TCP %d on %s did not answer within the probe window; this is not proof the device is offline", tcpPort, target)}
	}
	_ = connection.Close()
	return DeviceNetworkFact{Result: FactPassed, Detail: fmt.Sprintf("TCP %d on %s accepted a connection from this appliance", tcpPort, target)}
}

// InstructionStep is one thing to do on one machine.
type InstructionStep struct {
	Machine string `json:"machine"`
	Text    string `json:"text"`
}

// AccessInstructionSet is what an operator copies to reach a device network.
type AccessInstructionSet struct {
	DeviceNetworkID string
	// TargetAddress is the exact address to enter in engineering software.
	TargetAddress      string
	Steps              []InstructionStep
	UnmetPrerequisites []string
}

// AccessInstructions produces the machine-specific steps for one device
// network. Every step names where it belongs; nothing here is run anywhere.
func (manager *DeviceNetworkManager) AccessInstructions(ctx context.Context, id string) (AccessInstructionSet, error) {
	state, err := manager.surroundings(ctx)
	if err != nil {
		return AccessInstructionSet{}, err
	}
	record, ok := state.record(id)
	if !ok {
		return AccessInstructionSet{}, fmt.Errorf("%w: %s", ErrDeviceNetworkNotFound, id)
	}
	set := AccessInstructionSet{DeviceNetworkID: id, Steps: []InstructionStep{}, UnmetPrerequisites: []string{}}
	exposed := record.exposedScope()
	if record.Scope == ScopeHost {
		set.TargetAddress = exposed.Addr().String()
	} else {
		set.TargetAddress = exposed.String()
	}
	if !record.StartEnabled {
		set.UnmetPrerequisites = append(set.UnmetPrerequisites, fmt.Sprintf("the device network %s is disabled", record.Name))
	}
	if _, present := state.adapter(record.DeviceInterface); !present {
		set.UnmetPrerequisites = append(set.UnmetPrerequisites, fmt.Sprintf("the device-side adapter %s is not present", record.DeviceInterface))
	}
	if record.Access == AccessLocal || record.Access == AccessBoth {
		management := "the appliance's address on the local network"
		if adapter, ok := state.adapter(record.LocalInterface); ok && len(adapter.IPv4) > 0 {
			if prefix, err := netip.ParsePrefix(adapter.IPv4[0]); err == nil {
				management = prefix.Addr().String()
			}
		} else {
			set.UnmetPrerequisites = append(set.UnmetPrerequisites, fmt.Sprintf("the local ingress adapter %s is not present", record.LocalInterface))
		}
		set.Steps = append(set.Steps,
			InstructionStep{"client computer", fmt.Sprintf("add a route for %s via %s, or configure it on the router that serves the client", exposed, management)},
			InstructionStep{"router", fmt.Sprintf("optionally route %s to %s so every client on the local network reaches it without a per-computer route", exposed, management)})
	}
	if record.Access == AccessWireGuard || record.Access == AccessBoth {
		profile, ok := state.profile(record.VPNProfileID)
		name := record.VPNProfileID
		if ok {
			name = profile.Name
			if !profile.Active {
				set.UnmetPrerequisites = append(set.UnmetPrerequisites, fmt.Sprintf("the WireGuard profile %s is not connected", profile.Name))
			}
		} else {
			set.UnmetPrerequisites = append(set.UnmetPrerequisites, fmt.Sprintf("the WireGuard profile %s is not saved", record.VPNProfileID))
		}
		set.Steps = append(set.Steps,
			InstructionStep{"vpn hub", fmt.Sprintf("route %s to this appliance's peer and allow the client sources %s to reach it", exposed, prefixList(record.VPNSources))},
			InstructionStep{"client computer", fmt.Sprintf("route %s into the tunnel that reaches the hub used by profile %s", exposed, name)})
	}
	set.Steps = append(set.Steps, InstructionStep{"engineering software", fmt.Sprintf("enter %s as the device address", set.TargetAddress)})
	if len(record.Services.TCPPorts) > 0 {
		set.Steps = append(set.Steps, InstructionStep{"engineering software", fmt.Sprintf("only TCP ports %v are forwarded to the device", record.Services.TCPPorts)})
	}
	return set, nil
}

func prefixList(prefixes []netip.Prefix) string {
	texts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		texts = append(texts, prefix.String())
	}
	if len(texts) == 0 {
		return "none"
	}
	return fmt.Sprint(texts)
}
