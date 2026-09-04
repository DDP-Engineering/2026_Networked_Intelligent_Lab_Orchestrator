package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// A device network is a saved decision: which adapter faces the devices, what
// the appliance is on that adapter, what may be reached through it, from where,
// and at which address a remote client asks for it.
//
// Everything in this file is arithmetic and refusal. Nothing here touches an
// interface, a route or a firewall: a decision that cannot be made without side
// effects cannot be previewed, and a preview is the whole point.

// Scope is how much of the device subnet one device network exposes.
type Scope string

const (
	// ScopeHost exposes exactly one target address. It is the default, because
	// exposing a whole subnet is a bigger decision than exposing one device.
	ScopeHost Scope = "host"
	// ScopeSubnet exposes the whole device subnet and needs no target list.
	ScopeSubnet Scope = "subnet"
)

// Addressing is whether a remote client uses the device's own address or one
// this appliance translates.
type Addressing string

const (
	AddressingOriginal   Addressing = "original"
	AddressingTranslated Addressing = "translated"
)

// AccessMethod is which paths reach this device network. Both is the union of
// two separately configured paths, never a fallback from one to the other.
type AccessMethod string

const (
	AccessLocal     AccessMethod = "local"
	AccessWireGuard AccessMethod = "wireguard"
	AccessBoth      AccessMethod = "both"
)

// SubnetRole says what a connected subnet on this appliance is for, which
// decides what an overlap with it means.
type SubnetRole string

const (
	SubnetManagement SubnetRole = "management"
	SubnetUplink     SubnetRole = "uplink"
	SubnetTunnel     SubnetRole = "tunnel"
	SubnetDevice     SubnetRole = "device"
)

// ConnectedSubnet is one subnet the appliance is currently attached to.
type ConnectedSubnet struct {
	Interface string
	Role      SubnetRole
	Prefix    netip.Prefix
}

// ServicePolicy narrows what may be reached. The zero value is the default
// policy: IPv4 TCP, UDP and ICMP to the configured scope, which supports vendor
// protocols without assuming that TCP 502 is all anyone needs.
type ServicePolicy struct {
	Protocols []string
	TCPPorts  []int
	UDPPorts  []int
	// PreserveSource turns off source translation. It needs the target to have a
	// return route back to the client, which is why it is not the default.
	PreserveSource bool
}

var offeredProtocols = map[string]bool{"tcp": true, "udp": true, "icmp": true}

// DefaultProtocols is what an unrestricted policy permits.
func DefaultProtocols() []string { return []string{"tcp", "udp", "icmp"} }

// EffectiveProtocols is the policy's protocol list, or the default when it
// names none.
func (policy ServicePolicy) EffectiveProtocols() []string {
	if len(policy.Protocols) == 0 {
		return DefaultProtocols()
	}
	return append([]string(nil), policy.Protocols...)
}

// DeviceNetwork is one saved device network.
type DeviceNetwork struct {
	ID       string
	Name     string
	Location string

	// DeviceInterface is the stable adapter identity of the device-side
	// interface, not a kernel name that a replug can move.
	DeviceInterface string
	// ApplianceAddress is the appliance's own address and the device subnet in
	// one value, because they are one decision and storing them apart invites
	// them to disagree.
	ApplianceAddress netip.Prefix

	Scope  Scope
	Target netip.Addr

	Addressing Addressing
	// Exposed is the address or prefix a remote client uses. For a translated
	// single target it is a /32.
	Exposed netip.Prefix

	Access         AccessMethod
	LocalInterface string
	LocalSources   []netip.Prefix
	VPNProfileID   string
	VPNSources     []netip.Prefix

	Services ServicePolicy
	// StartEnabled is the configured intent - enabled, applied whenever its
	// prerequisites hold, including at startup - which is not the same as the
	// current applied state and is never derived from it.
	StartEnabled bool
	Revision     string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ErrDeviceNetworkNotFound is an identity nothing is saved under.
var ErrDeviceNetworkNotFound = errors.New("device_network_not_found")

// ContentRevision digests the decision's content fields, so an unchanged save
// keeps the revision and any edit changes it.
func (network DeviceNetwork) ContentRevision() string {
	digest := sha256.New()
	fmt.Fprintf(digest, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t\x00%t\n",
		network.ID, network.Name, network.Location, network.DeviceInterface, prefixText(network.ApplianceAddress),
		network.Scope, addrText(network.Target), network.Addressing, prefixText(network.Exposed), network.Access,
		network.LocalInterface, network.StartEnabled, network.Services.PreserveSource)
	fmt.Fprintf(digest, "vpn\x00%s\n", network.VPNProfileID)
	for _, source := range network.LocalSources {
		fmt.Fprintf(digest, "local\x00%s\n", source)
	}
	for _, source := range network.VPNSources {
		fmt.Fprintf(digest, "vpn-source\x00%s\n", source)
	}
	fmt.Fprintf(digest, "services\x00%s\x00%v\x00%v\n", strings.Join(network.Services.Protocols, ","), network.Services.TCPPorts, network.Services.UDPPorts)
	return hex.EncodeToString(digest.Sum(nil))[:16]
}

// DeviceNetworkStore is the saved device network inventory.
type DeviceNetworkStore interface {
	Save(ctx context.Context, record DeviceNetwork, at time.Time) (DeviceNetwork, error)
	List(ctx context.Context) ([]DeviceNetwork, error)
	Delete(ctx context.Context, id string) error
}

const maximumDeviceNetworkLabel = 120

// ErrDeviceNetworkRejected is every refusal of a proposed device network. The
// message names what is wrong, because "invalid configuration" tells an
// operator standing in front of a cabinet nothing.
var ErrDeviceNetworkRejected = errors.New("device network rejected")

func reject(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrDeviceNetworkRejected, fmt.Sprintf(format, arguments...))
}

// DeviceSubnet is the subnet the devices live on.
func (network DeviceNetwork) DeviceSubnet() netip.Prefix {
	return network.ApplianceAddress.Masked()
}

// Validate checks everything that can be decided from the proposal alone.
func (network DeviceNetwork) Validate() error {
	if strings.TrimSpace(network.Name) == "" {
		return reject("a device network needs a name")
	}
	if len(network.Name) > maximumDeviceNetworkLabel || len(network.Location) > maximumDeviceNetworkLabel {
		return reject("the name and location are limited to %d bytes", maximumDeviceNetworkLabel)
	}
	if strings.TrimSpace(network.DeviceInterface) == "" {
		return reject("a device network needs a device-side interface")
	}
	if err := network.validateApplianceAddress(); err != nil {
		return err
	}
	if err := network.validateScope(); err != nil {
		return err
	}
	if err := network.validateAddressing(); err != nil {
		return err
	}
	if err := network.validateAccess(); err != nil {
		return err
	}
	return network.validateServices()
}

func (network DeviceNetwork) validateApplianceAddress() error {
	appliance := network.ApplianceAddress
	if !appliance.IsValid() {
		return reject("a device network needs the appliance's own address and subnet mask")
	}
	if !appliance.Addr().Is4() {
		return reject("device forwarding is IPv4 only, so %s cannot be the appliance address", appliance)
	}
	// /31 and /32 are point-to-point and single-host masks. A device network
	// needs an ordinary host subnet with room for devices and a broadcast.
	if appliance.Bits() < 8 || appliance.Bits() > 30 {
		return reject("%s is not an ordinary host subnet; use a mask between /8 and /30", appliance)
	}
	if !appliance.Addr().IsPrivate() {
		return reject("%s is not a private address; device subnets start from the private IPv4 ranges", appliance.Addr())
	}
	subnet := appliance.Masked()
	if appliance.Addr() == subnet.Addr() {
		return reject("%s is the subnet's network address, not a usable appliance address", appliance.Addr())
	}
	if appliance.Addr() == broadcastOf(subnet) {
		return reject("%s is the subnet's broadcast address, not a usable appliance address", appliance.Addr())
	}
	return nil
}

func (network DeviceNetwork) validateScope() error {
	switch network.Scope {
	case ScopeHost:
		if !network.Target.IsValid() {
			return reject("a single-device network needs the target IP address")
		}
		if !network.Target.Is4() {
			return reject("device forwarding is IPv4 only, so %s cannot be a target", network.Target)
		}
		subnet := network.DeviceSubnet()
		if !subnet.Contains(network.Target) {
			return reject("%s is not inside the device subnet %s", network.Target, subnet)
		}
		if network.Target == network.ApplianceAddress.Addr() {
			return reject("%s is the appliance's own device-side address and cannot also be a target", network.Target)
		}
		if network.Target == subnet.Addr() || network.Target == broadcastOf(subnet) {
			return reject("%s is the subnet's network or broadcast address, not a device", network.Target)
		}
	case ScopeSubnet:
		if network.Target.IsValid() {
			return reject("a whole device subnet exposure takes no single target address")
		}
	default:
		return reject("unknown scope %q", network.Scope)
	}
	return nil
}

func (network DeviceNetwork) validateAddressing() error {
	switch network.Addressing {
	case AddressingOriginal:
		if network.Exposed.IsValid() {
			return reject("original addressing exposes the device's own address, so no exposed address is set")
		}
	case AddressingTranslated:
		if !network.Exposed.IsValid() {
			return reject("translated addressing needs the exposed address or prefix")
		}
		if !network.Exposed.Addr().Is4() {
			return reject("device forwarding is IPv4 only, so %s cannot be exposed", network.Exposed)
		}
		if network.Scope == ScopeHost && network.Exposed.Bits() != 32 {
			return reject("one device is exposed at a single address, so %s must be a /32", network.Exposed)
		}
		if network.Scope == ScopeSubnet {
			if network.Exposed.Bits() != network.ApplianceAddress.Bits() {
				return reject(
					"the exposed prefix must be the same size as the device subnet, so %s cannot map %s",
					network.Exposed, network.DeviceSubnet())
			}
			if network.Exposed.Masked() != network.Exposed {
				return reject("%s is not the first address of its prefix", network.Exposed)
			}
		}
	default:
		return reject("unknown addressing %q", network.Addressing)
	}
	return nil
}

func (network DeviceNetwork) validateAccess() error {
	local := network.Access == AccessLocal || network.Access == AccessBoth
	tunnel := network.Access == AccessWireGuard || network.Access == AccessBoth
	if !local && !tunnel {
		return reject("unknown access method %q", network.Access)
	}
	if local {
		if strings.TrimSpace(network.LocalInterface) == "" {
			return reject("local access needs a local ingress interface")
		}
		if network.LocalInterface == network.DeviceInterface {
			return reject("the local ingress and the device side cannot be the same adapter %q", network.DeviceInterface)
		}
		if err := validateSources("local", network.LocalSources); err != nil {
			return err
		}
	}
	if tunnel {
		if strings.TrimSpace(network.VPNProfileID) == "" {
			return reject("VPN access needs the WireGuard profile it comes in on")
		}
		if err := validateSources("VPN", network.VPNSources); err != nil {
			return err
		}
	}
	return nil
}

// validateSources refuses an empty or unusable permitted-source list. An empty
// list is not "everyone": a device network with no stated source has no
// authorized client, and treating that as open is exactly the mistake that
// publishes a cabinet to the Internet.
func validateSources(kind string, sources []netip.Prefix) error {
	if len(sources) == 0 {
		return reject("%s access needs at least one permitted source range", kind)
	}
	for _, source := range sources {
		if !source.IsValid() || !source.Addr().Is4() {
			return reject("%s permitted source %s is not an IPv4 range", kind, source)
		}
		if source.Masked() != source {
			return reject("%s permitted source %s is not the first address of its prefix", kind, source)
		}
	}
	return nil
}

func (network DeviceNetwork) validateServices() error {
	protocols := map[string]bool{}
	for _, protocol := range network.Services.Protocols {
		name := strings.ToLower(strings.TrimSpace(protocol))
		if !offeredProtocols[name] {
			return reject("unsupported protocol %q; this milestone forwards tcp, udp and icmp", protocol)
		}
		protocols[name] = true
	}
	for name, ports := range map[string][]int{"tcp": network.Services.TCPPorts, "udp": network.Services.UDPPorts} {
		for _, port := range ports {
			if port < 1 || port > 65535 {
				return reject("port %d is outside the valid port range", port)
			}
		}
		if len(ports) > 0 && len(protocols) > 0 && !protocols[name] {
			return reject("%s ports are restricted but %s is not one of the permitted protocols", name, name)
		}
	}
	return nil
}

// ValidateAgainst adds the checks that need the rest of the appliance: the
// subnets it is already attached to and the device networks already saved.
//
// A conflict with something on a remote client or a VPN hub cannot be seen from
// here at all. This function never reports the absence of a conflict, only the
// presence of one.
func (network DeviceNetwork) ValidateAgainst(connected []ConnectedSubnet, siblings []DeviceNetwork) error {
	if err := network.Validate(); err != nil {
		return err
	}
	subnet := network.DeviceSubnet()
	for _, attached := range connected {
		if attached.Role == SubnetDevice {
			continue
		}
		if attached.Prefix.Overlaps(subnet) {
			return reject(
				"the device subnet %s overlaps the %s subnet %s on %s; change the other network or use a different device subnet",
				subnet, attached.Role, attached.Prefix, attached.Interface)
		}
		if network.Addressing == AddressingTranslated && attached.Prefix.Overlaps(network.Exposed) {
			return reject(
				"the exposed range %s overlaps the %s subnet %s on %s",
				network.Exposed, attached.Role, attached.Prefix, attached.Interface)
		}
	}
	for _, sibling := range siblings {
		if sibling.ID == network.ID {
			// The saved version of the device network being edited is not a
			// conflict with itself.
			continue
		}
		if sibling.DeviceInterface == network.DeviceInterface {
			return reject(
				"device network %q already uses adapter %s; one adapter carries one device network",
				sibling.Name, network.DeviceInterface)
		}
		if sibling.DeviceSubnet().Overlaps(subnet) {
			return reject(
				"device network %q already uses the device subnet %s",
				sibling.Name, sibling.DeviceSubnet())
		}
		if network.Addressing == AddressingTranslated && sibling.Addressing == AddressingTranslated &&
			sibling.Exposed.Overlaps(network.Exposed) {
			return reject(
				"device network %q is already exposed at %s",
				sibling.Name, sibling.Exposed)
		}
	}
	return nil
}

// TargetFor maps one exposed address to the device address behind it,
// preserving host bits. It is the arithmetic a prefix mapping is, which is why
// one rule can serve a whole subnet instead of one rule per address.
func (network DeviceNetwork) TargetFor(exposed netip.Addr) (netip.Addr, error) {
	if network.Addressing != AddressingTranslated {
		return netip.Addr{}, reject("this device network is not translated, so %s maps to nothing", exposed)
	}
	if !network.Exposed.Contains(exposed) {
		return netip.Addr{}, reject("%s is outside the exposed range %s", exposed, network.Exposed)
	}
	if network.Scope == ScopeHost {
		return network.Target, nil
	}
	exposedBytes := exposed.As4()
	baseBytes := network.Exposed.Masked().Addr().As4()
	deviceBytes := network.DeviceSubnet().Addr().As4()
	var mapped [4]byte
	for index := range mapped {
		mapped[index] = deviceBytes[index] | (exposedBytes[index] &^ baseBytes[index] & hostMaskByte(network.Exposed.Bits(), index))
	}
	return netip.AddrFrom4(mapped), nil
}

// hostMaskByte is the part of one address byte that belongs to the host rather
// than to the prefix.
func hostMaskByte(bits, index int) byte {
	remaining := bits - index*8
	switch {
	case remaining <= 0:
		return 0xff
	case remaining >= 8:
		return 0x00
	default:
		return byte(0xff >> remaining)
	}
}

// broadcastOf is the all-ones address of an IPv4 prefix.
func broadcastOf(subnet netip.Prefix) netip.Addr {
	bytes := subnet.Masked().Addr().As4()
	for index := range bytes {
		bytes[index] |= hostMaskByte(subnet.Bits(), index)
	}
	return netip.AddrFrom4(bytes)
}
