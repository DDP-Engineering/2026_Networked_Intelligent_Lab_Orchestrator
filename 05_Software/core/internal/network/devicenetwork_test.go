package network

import (
	"net/netip"
	"strings"
	"testing"
)

func prefix(t *testing.T, value string) netip.Prefix {
	t.Helper()
	parsed, err := netip.ParsePrefix(value)
	if err != nil {
		t.Fatalf("parse prefix %q: %v", value, err)
	}
	return parsed
}

func address(t *testing.T, value string) netip.Addr {
	t.Helper()
	parsed, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatalf("parse address %q: %v", value, err)
	}
	return parsed
}

// validHost is the plan's worked example: the appliance at 192.168.0.1/24 on
// its device side, one target at 192.168.0.130, reached from a management LAN.
func validHost(t *testing.T) DeviceNetwork {
	t.Helper()
	return DeviceNetwork{
		ID: "dn-1", Name: "line 4", Location: "hall B",
		DeviceInterface:  "adapter-usb-1",
		ApplianceAddress: prefix(t, "192.168.0.1/24"),
		Scope:            ScopeHost,
		Target:           address(t, "192.168.0.130"),
		Addressing:       AddressingOriginal,
		Access:           AccessLocal,
		LocalInterface:   "adapter-eth0",
		LocalSources:     []netip.Prefix{prefix(t, "192.168.1.0/24")},
	}
}

// TestAValidDeviceNetworkIsAccepted covers REQ-FUNC-2f8d63b5 for both scopes.
func TestAValidDeviceNetworkIsAccepted(t *testing.T) {
	t.Parallel()
	host := validHost(t)
	if err := host.Validate(); err != nil {
		t.Fatalf("a valid single-target device network was refused: %v", err)
	}

	subnet := validHost(t)
	subnet.Scope = ScopeSubnet
	subnet.Target = netip.Addr{}
	if err := subnet.Validate(); err != nil {
		t.Fatalf("a valid whole-subnet device network was refused: %v", err)
	}
}

// TestDeviceNetworkAddressingIsChecked covers REQ-FUNC-2f8d63b5's structural
// rules and the single-appliance half of REQ-FUNC-46b1d8e3.
func TestDeviceNetworkAddressingIsChecked(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*DeviceNetwork)
		mustSay string
	}{
		{"no name", func(dn *DeviceNetwork) { dn.Name = "" }, "name"},
		{"no device interface", func(dn *DeviceNetwork) { dn.DeviceInterface = "" }, "device-side interface"},
		{
			"appliance address is the subnet address",
			func(dn *DeviceNetwork) { dn.ApplianceAddress = prefix(t, "192.168.0.0/24") },
			"network address",
		},
		{
			"appliance address is the broadcast address",
			func(dn *DeviceNetwork) { dn.ApplianceAddress = prefix(t, "192.168.0.255/24") },
			"broadcast address",
		},
		{
			"appliance address is not IPv4",
			func(dn *DeviceNetwork) { dn.ApplianceAddress = prefix(t, "fd00::1/64") },
			"IPv4",
		},
		{
			"appliance address is a point-to-point mask",
			func(dn *DeviceNetwork) { dn.ApplianceAddress = prefix(t, "192.168.0.1/31") },
			"host subnet",
		},
		{
			"appliance address is public",
			func(dn *DeviceNetwork) { dn.ApplianceAddress = prefix(t, "8.8.8.1/24") },
			"private",
		},
		{
			"target outside the device subnet",
			func(dn *DeviceNetwork) { dn.Target = address(t, "10.9.9.9") },
			"inside the device subnet",
		},
		{
			"target is the appliance itself",
			func(dn *DeviceNetwork) { dn.Target = address(t, "192.168.0.1") },
			"appliance's own",
		},
		{
			"single-target scope without a target",
			func(dn *DeviceNetwork) { dn.Target = netip.Addr{} },
			"target",
		},
		{
			"whole-subnet scope with a target",
			func(dn *DeviceNetwork) { dn.Scope = ScopeSubnet },
			"whole device subnet",
		},
		{
			"local access without an ingress interface",
			func(dn *DeviceNetwork) { dn.LocalInterface = "" },
			"local ingress interface",
		},
		{
			"local access without permitted sources",
			func(dn *DeviceNetwork) { dn.LocalSources = nil },
			"permitted source",
		},
		{
			"ingress interface is the device interface",
			func(dn *DeviceNetwork) { dn.LocalInterface = "adapter-usb-1" },
			"same adapter",
		},
		{
			"VPN access without a profile",
			func(dn *DeviceNetwork) {
				dn.Access = AccessWireGuard
				dn.VPNSources = []netip.Prefix{prefix(t, "10.60.0.0/16")}
			},
			"WireGuard profile",
		},
		{
			"VPN access without permitted sources",
			func(dn *DeviceNetwork) {
				dn.Access = AccessWireGuard
				dn.VPNProfileID = "profile-1"
			},
			"permitted source",
		},
		{
			"translated addressing without an exposed address",
			func(dn *DeviceNetwork) { dn.Addressing = AddressingTranslated },
			"exposed",
		},
		{
			"original addressing with an exposed address",
			func(dn *DeviceNetwork) { dn.Exposed = prefix(t, "10.60.1.130/32") },
			"original",
		},
		{
			"translated single target exposed as a whole prefix",
			func(dn *DeviceNetwork) {
				dn.Addressing = AddressingTranslated
				dn.Exposed = prefix(t, "10.60.1.0/24")
			},
			"single address",
		},
		{
			"translated subnet exposed at a different prefix length",
			func(dn *DeviceNetwork) {
				dn.Scope = ScopeSubnet
				dn.Target = netip.Addr{}
				dn.Addressing = AddressingTranslated
				dn.Exposed = prefix(t, "10.60.1.0/25")
			},
			"same size",
		},
		{
			"an unoffered protocol",
			func(dn *DeviceNetwork) { dn.Services.Protocols = []string{"sctp"} },
			"protocol",
		},
		{
			"a port outside the range",
			func(dn *DeviceNetwork) {
				dn.Services.Protocols = []string{"tcp"}
				dn.Services.TCPPorts = []int{70000}
			},
			"port",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			candidate := validHost(t)
			testCase.mutate(&candidate)
			err := candidate.Validate()
			if err == nil {
				t.Fatalf("%s was accepted", testCase.name)
			}
			if !strings.Contains(err.Error(), testCase.mustSay) {
				t.Fatalf("refusal %q does not mention %q", err, testCase.mustSay)
			}
		})
	}
}

// TestTranslatedSubnetKeepsHostBits covers the mapping half of
// REQ-FUNC-1d93b76e: a prefix mapping is arithmetic, not a table.
func TestTranslatedSubnetKeepsHostBits(t *testing.T) {
	t.Parallel()
	subnet := validHost(t)
	subnet.Scope = ScopeSubnet
	subnet.Target = netip.Addr{}
	subnet.Addressing = AddressingTranslated
	subnet.Exposed = prefix(t, "10.60.1.0/24")
	if err := subnet.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	for _, mapping := range []struct{ exposed, device string }{
		{"10.60.1.130", "192.168.0.130"},
		{"10.60.1.1", "192.168.0.1"},
		{"10.60.1.254", "192.168.0.254"},
	} {
		got, err := subnet.TargetFor(address(t, mapping.exposed))
		if err != nil {
			t.Fatalf("map %s: %v", mapping.exposed, err)
		}
		if got.String() != mapping.device {
			t.Fatalf("%s maps to %s, want %s", mapping.exposed, got, mapping.device)
		}
	}
	if _, err := subnet.TargetFor(address(t, "10.60.2.130")); err == nil {
		t.Fatal("an address outside the exposed prefix was mapped")
	}
}

// TestConflictingDeviceNetworksAreRefused covers REQ-FUNC-46b1d8e3 against the
// rest of the appliance.
func TestConflictingDeviceNetworksAreRefused(t *testing.T) {
	t.Parallel()
	management := ConnectedSubnet{Interface: "adapter-eth0", Role: SubnetManagement, Prefix: prefix(t, "192.168.1.0/24")}
	uplink := ConnectedSubnet{Interface: "adapter-wlan0", Role: SubnetUplink, Prefix: prefix(t, "192.168.0.0/24")}
	tunnel := ConnectedSubnet{Interface: "nilo-wg0", Role: SubnetTunnel, Prefix: prefix(t, "10.60.0.0/16")}

	sibling := validHost(t)
	sibling.ID = "dn-existing"
	sibling.DeviceInterface = "adapter-usb-2"
	sibling.ApplianceAddress = prefix(t, "192.168.5.1/24")
	sibling.Addressing = AddressingTranslated
	sibling.Exposed = prefix(t, "10.61.1.130/32")

	cases := []struct {
		name      string
		connected []ConnectedSubnet
		siblings  []DeviceNetwork
		mutate    func(*DeviceNetwork)
		mustSay   string
	}{
		{
			name:      "device subnet overlaps the uplink",
			connected: []ConnectedSubnet{management, uplink},
			mustSay:   "192.168.0.0/24",
		},
		{
			name:      "exposed prefix overlaps the tunnel subnet",
			connected: []ConnectedSubnet{management, tunnel},
			mutate: func(dn *DeviceNetwork) {
				dn.Addressing = AddressingTranslated
				dn.Exposed = prefix(t, "10.60.1.130/32")
			},
			mustSay: "10.60.0.0/16",
		},
		{
			name:      "another device network already uses the adapter",
			connected: []ConnectedSubnet{management},
			siblings: func() []DeviceNetwork {
				other := sibling
				other.DeviceInterface = "adapter-usb-1"
				return []DeviceNetwork{other}
			}(),
			mustSay: "adapter-usb-1",
		},
		{
			name:      "another device network already uses the exposed address",
			connected: []ConnectedSubnet{management},
			siblings: func() []DeviceNetwork {
				other := sibling
				other.Exposed = prefix(t, "10.61.1.130/32")
				return []DeviceNetwork{other}
			}(),
			mutate: func(dn *DeviceNetwork) {
				dn.Addressing = AddressingTranslated
				dn.Exposed = prefix(t, "10.61.1.130/32")
			},
			mustSay: "10.61.1.130/32",
		},
		{
			name:      "another device network already uses the device subnet",
			connected: []ConnectedSubnet{management},
			siblings: func() []DeviceNetwork {
				other := sibling
				other.ApplianceAddress = prefix(t, "192.168.0.9/24")
				return []DeviceNetwork{other}
			}(),
			mustSay: "192.168.0.0/24",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			candidate := validHost(t)
			if testCase.mutate != nil {
				testCase.mutate(&candidate)
			}
			err := candidate.ValidateAgainst(testCase.connected, testCase.siblings)
			if err == nil {
				t.Fatalf("%s was accepted", testCase.name)
			}
			if !strings.Contains(err.Error(), testCase.mustSay) {
				t.Fatalf("refusal %q does not name %q", err, testCase.mustSay)
			}
		})
	}
}

// TestAnEditOfTheSameDeviceNetworkDoesNotConflictWithItself keeps editing a
// saved device network possible, which a naive sibling check would prevent.
func TestAnEditOfTheSameDeviceNetworkDoesNotConflictWithItself(t *testing.T) {
	t.Parallel()
	saved := validHost(t)
	edited := saved
	edited.Name = "line 4 renamed"
	connected := []ConnectedSubnet{
		{Interface: "adapter-eth0", Role: SubnetManagement, Prefix: prefix(t, "192.168.1.0/24")},
	}
	if err := edited.ValidateAgainst(connected, []DeviceNetwork{saved}); err != nil {
		t.Fatalf("editing a saved device network conflicted with itself: %v", err)
	}
}
