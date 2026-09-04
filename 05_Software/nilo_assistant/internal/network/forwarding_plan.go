package network

import (
	"fmt"
	"net/netip"
	"sort"
)

// The forwarding planner turns one saved device network into the exact set of
// resources this appliance would own for it. It produces a description, not an
// effect: nothing here reaches nftables, a routing table or a sysctl, so a plan
// can be previewed, compared, tested and refused without touching anything.
//
// The order matters and is part of the plan. Guards come first, because a
// moment between "forwarding is on" and "the guard exists" is a moment in which
// traffic leaves by a path nobody authorized.

// OwnedTable is the single nftables table this appliance creates. Everything
// NILO owns lives inside it, so removal never has to guess and the global
// ruleset is never flushed.
const OwnedTable = "nilo_device_networks"

// ResourceKind is one sort of thing the appliance owns for a device network.
type ResourceKind string

const (
	// ResourceDenyGuard refuses the exposed scope by default. It is installed
	// before any grant and removed after every grant, so the failure mode of a
	// half-applied or half-removed plan is "closed".
	ResourceDenyGuard ResourceKind = "deny_guard"
	// ResourceIPv6Deny refuses IPv6 to the exposed scope. IPv6 forwarding is out
	// of scope for this milestone, and out of scope has to mean denied rather
	// than unconsidered.
	ResourceIPv6Deny ResourceKind = "ipv6_deny"
	// ResourceForwardingSysctl is the forwarding switch, recorded as owned so
	// its previous value can be restored and so another service's need for it is
	// not undone.
	ResourceForwardingSysctl ResourceKind = "forwarding_sysctl"
	// ResourceDestinationNAT maps an exposed address or prefix onto the device
	// addresses behind it.
	ResourceDestinationNAT ResourceKind = "destination_nat"
	// ResourceSourceNAT replaces the client's address with the appliance's
	// device-side address, so the target replies locally and its own gateway
	// does not have to change.
	ResourceSourceNAT ResourceKind = "source_nat"
	// ResourceForwardGrant permits one protocol from one authorized source on
	// one ingress path to the exposed scope. It is installed last.
	ResourceForwardGrant ResourceKind = "forward_grant"
)

// Resource is one owned thing, described in terms of the decision rather than
// in terms of any particular firewall syntax. Rendering it is the Linux
// adapter's job, and keeping the two apart is what makes the decision testable
// without a Linux host.
type Resource struct {
	Kind ResourceKind
	// Name is unique within the appliance and names the device network it
	// belongs to, so an inventory can always say who owns what.
	Name string

	// Ingress is the interface or tunnel device traffic arrives on. Empty means
	// the resource is not bound to one path.
	Ingress string
	// Egress is the device-side link traffic leaves by.
	Egress string

	Source      netip.Prefix
	Destination netip.Prefix
	// Translated is the address or prefix the traffic is rewritten to.
	Translated netip.Prefix

	Protocol string
	Ports    []int
}

// ForwardingContext is what the planner needs to know about the appliance right
// now: which kernel devices the configuration's stable identities currently
// resolve to.
type ForwardingContext struct {
	// DeviceLink is the kernel device the device-side adapter is on. Empty means
	// the adapter is not present.
	DeviceLink string
	// TunnelDevice is the kernel device the selected WireGuard profile is on.
	// Empty means the tunnel is not up.
	TunnelDevice string
	// LocalLink is the kernel device the local-access adapter is on. Empty
	// means that adapter is not present.
	LocalLink string
}

// ForwardingPlan is the complete owned resource set for one device network.
type ForwardingPlan struct {
	DeviceNetworkID string
	Table           string
	Resources       []Resource

	// Waiting reports that a configured prerequisite is not present, so the
	// plan deliberately grants nothing. The guards are still planned: waiting is
	// a closed state, not an absent one.
	Waiting       bool
	WaitingReason string
}

// PlanForwarding computes the owned resources for one device network.
func PlanForwarding(network DeviceNetwork, context ForwardingContext) (ForwardingPlan, error) {
	if err := network.Validate(); err != nil {
		return ForwardingPlan{}, err
	}
	plan := ForwardingPlan{DeviceNetworkID: network.ID, Table: OwnedTable}
	exposedScope := network.exposedScope()

	// Guards first, always, and regardless of whether anything is granted.
	plan.add(Resource{
		Kind: ResourceDenyGuard, Name: network.resourceName("guard"),
		Destination: exposedScope,
	})
	plan.add(Resource{
		Kind: ResourceIPv6Deny, Name: network.resourceName("guard-ipv6"),
	})

	paths, waitingReason := network.ingressPaths(context)
	if context.DeviceLink == "" {
		waitingReason = fmt.Sprintf("the device-side adapter %s is not present", network.DeviceInterface)
	}
	if waitingReason != "" {
		plan.Waiting = true
		plan.WaitingReason = waitingReason
		return plan, nil
	}

	plan.add(Resource{
		Kind: ResourceForwardingSysctl, Name: network.resourceName("forwarding"),
		Egress: context.DeviceLink,
	})

	if network.Addressing == AddressingTranslated {
		plan.add(Resource{
			Kind: ResourceDestinationNAT, Name: network.resourceName("dnat"),
			Destination: network.Exposed, Translated: network.deviceScope(),
			Egress: context.DeviceLink,
		})
	}
	if !network.Services.PreserveSource {
		plan.add(Resource{
			Kind: ResourceSourceNAT, Name: network.resourceName("snat"),
			Destination: network.deviceScope(),
			Translated:  netip.PrefixFrom(network.ApplianceAddress.Addr(), 32),
			Egress:      context.DeviceLink,
		})
	}

	// Grants last. One per authorized source, per ingress path, per protocol,
	// so that removing one path or one protocol removes exactly that.
	for _, path := range paths {
		for _, source := range path.sources {
			for _, protocol := range network.Services.EffectiveProtocols() {
				plan.add(Resource{
					Kind:    ResourceForwardGrant,
					Name:    network.resourceName(fmt.Sprintf("grant-%s-%s-%s", path.name, source.String(), protocol)),
					Ingress: path.device, Egress: context.DeviceLink,
					Source: source, Destination: exposedScope,
					Protocol: protocol, Ports: network.portsFor(protocol),
				})
			}
		}
	}
	return plan, nil
}

// ingressPath is one explicitly configured way in.
type ingressPath struct {
	name    string
	device  string
	sources []netip.Prefix
}

// ingressPaths resolves the configured access methods against what is actually
// present. A configured path whose device is missing makes the whole plan wait
// rather than quietly leaving the other path serving traffic the operator
// expected to arrive over the missing one.
func (network DeviceNetwork) ingressPaths(context ForwardingContext) ([]ingressPath, string) {
	paths := make([]ingressPath, 0, 2)
	if network.Access == AccessLocal || network.Access == AccessBoth {
		if context.LocalLink == "" {
			return nil, fmt.Sprintf(
				"the local-access adapter %s is not present, so no device traffic is forwarded",
				network.LocalInterface)
		}
		paths = append(paths, ingressPath{
			name: "local", device: context.LocalLink, sources: sortedPrefixes(network.LocalSources),
		})
	}
	if network.Access == AccessWireGuard || network.Access == AccessBoth {
		if context.TunnelDevice == "" {
			return nil, fmt.Sprintf(
				"the selected WireGuard profile %s is not connected, so no device traffic is forwarded",
				network.VPNProfileID)
		}
		paths = append(paths, ingressPath{
			name: "wireguard", device: context.TunnelDevice, sources: sortedPrefixes(network.VPNSources),
		})
	}
	return paths, ""
}

// exposedScope is the destination an authorized client actually addresses.
//
// For a translated device network this is the exposed address or prefix and
// nothing else: the original device address is deliberately not granted as
// well, so that knowing an internal address cannot bypass the chosen mapping.
func (network DeviceNetwork) exposedScope() netip.Prefix {
	if network.Addressing == AddressingTranslated {
		return network.Exposed
	}
	return network.deviceScope()
}

// deviceScope is the destination behind the appliance.
func (network DeviceNetwork) deviceScope() netip.Prefix {
	if network.Scope == ScopeHost {
		return netip.PrefixFrom(network.Target, 32)
	}
	return network.DeviceSubnet()
}

// portsFor restricts one protocol where the policy says so. ICMP has no ports,
// and a protocol with no restriction carries none.
func (network DeviceNetwork) portsFor(protocol string) []int {
	switch protocol {
	case "tcp":
		return append([]int(nil), network.Services.TCPPorts...)
	case "udp":
		return append([]int(nil), network.Services.UDPPorts...)
	default:
		return nil
	}
}

// resourceName is deterministic and carries the device network's identity, so
// an inventory of what this appliance owns needs no separate bookkeeping and a
// removal never has to match on shape.
func (network DeviceNetwork) resourceName(suffix string) string {
	return fmt.Sprintf("nilo-dn-%s-%s", network.ID, suffix)
}

func (plan *ForwardingPlan) add(resource Resource) {
	plan.Resources = append(plan.Resources, resource)
}

// sortedPrefixes gives one configuration one resource order, so two plans of
// the same device network compare equal and a diff means a real change.
func sortedPrefixes(prefixes []netip.Prefix) []netip.Prefix {
	sorted := append([]netip.Prefix(nil), prefixes...)
	sort.Slice(sorted, func(first, second int) bool {
		return sorted[first].String() < sorted[second].String()
	})
	return sorted
}
