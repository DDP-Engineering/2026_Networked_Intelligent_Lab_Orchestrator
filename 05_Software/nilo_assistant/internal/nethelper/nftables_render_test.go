package nethelper

import (
	"net/netip"
	"strings"
	"testing"

	"nilo.local/nilo/internal/network"
)

// TestRenderNftables covers the rendering of a plan into the owned table:
// grants before guards, translated grants matched on the address the client
// used, and a policy that closes forwarding only where NILO turned it on.
func TestRenderNftables(t *testing.T) {
	translated, err := network.PlanForwarding(network.DeviceNetwork{
		ID: "site-b", Name: "Site B", DeviceInterface: "02:00:00:00:00:02",
		ApplianceAddress: netip.MustParsePrefix("192.168.0.1/24"), Scope: network.ScopeHost,
		Target: netip.MustParseAddr("192.168.0.130"), Addressing: network.AddressingTranslated,
		Exposed: netip.MustParsePrefix("10.7.0.130/32"),
		Access:  network.AccessWireGuard, VPNProfileID: "office",
		VPNSources: []netip.Prefix{netip.MustParsePrefix("10.8.0.0/24")},
		Services:   network.ServicePolicy{Protocols: []string{"tcp", "icmp"}, TCPPorts: []int{502, 44818}},
	}, network.ForwardingContext{DeviceLink: "eth1", TunnelDevice: "nilo0"})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := network.PlanForwarding(network.DeviceNetwork{
		ID: "line3", Name: "Line 3", DeviceInterface: "02:00:00:00:00:03",
		ApplianceAddress: netip.MustParsePrefix("192.168.5.1/24"), Scope: network.ScopeSubnet,
		Addressing: network.AddressingOriginal, Access: network.AccessLocal, LocalInterface: "02:00:00:00:00:01",
		LocalSources: []netip.Prefix{netip.MustParsePrefix("192.168.10.0/24")},
	}, network.ForwardingContext{DeviceLink: ""})
	if err != nil {
		t.Fatal(err)
	}
	script := renderNftables(ForwardingInventory{Plans: []network.ForwardingPlan{translated, waiting}, Switch: SwitchOwnership{Owned: true, Previous: "0"}})
	for _, want := range []string{
		`table inet nilo {}`,
		`delete table inet nilo`,
		`comment "managed by nilo-nethelper"`,
		`type filter hook forward priority filter; policy drop;`,
		`type nat hook prerouting priority dstnat;`,
		`type nat hook postrouting priority srcnat;`,
		`jump dn_site_b`,
		`jump dn_line3`,
		`iifname "nilo0" oifname "eth1" ip saddr 10.8.0.0/24 ct original ip daddr 10.7.0.130 tcp dport { 502, 44818 } accept`,
		`iifname "nilo0" oifname "eth1" ip saddr 10.8.0.0/24 ct original ip daddr 10.7.0.130 meta l4proto icmp accept`,
		`oifname "eth1" ip daddr 192.168.0.130 drop`,
		`ip daddr 10.7.0.130 drop`,
		`meta nfproto ipv6 oifname "eth1" drop`,
		`ip daddr 10.7.0.130 dnat ip to 192.168.0.130`,
		`oifname "eth1" ip daddr 192.168.0.130 snat ip to 192.168.0.1`,
		`ip daddr 192.168.5.0/24 drop`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("missing %q in:\n%s", want, script)
		}
	}
	if strings.Contains(script, "192.168.10.0/24 ") && strings.Contains(script, "dn_line3 {\n\t\tip saddr") {
		t.Error("a waiting plan must grant nothing")
	}
	if strings.Index(script, "ct original ip daddr 10.7.0.130 tcp") > strings.Index(script, "ip daddr 10.7.0.130 drop") {
		t.Error("grants must come before the guard")
	}
	// Where the operator already forwarded, NILO does not close the rest.
	open := renderNftables(ForwardingInventory{Plans: []network.ForwardingPlan{translated}})
	if !strings.Contains(open, "policy accept;") || strings.Contains(open, "policy drop;") {
		t.Errorf("operator-owned forwarding must keep policy accept:\n%s", open)
	}
	// A subnet translation maps prefix onto prefix.
	subnet, err := network.PlanForwarding(network.DeviceNetwork{
		ID: "site-c", Name: "Site C", DeviceInterface: "02:00:00:00:00:02",
		ApplianceAddress: netip.MustParsePrefix("192.168.0.1/24"), Scope: network.ScopeSubnet,
		Addressing: network.AddressingTranslated, Exposed: netip.MustParsePrefix("10.9.0.0/24"),
		Access: network.AccessLocal, LocalInterface: "02:00:00:00:00:01",
		LocalSources: []netip.Prefix{netip.MustParsePrefix("192.168.10.0/24")},
	}, network.ForwardingContext{DeviceLink: "eth1", LocalLink: "eth0"})
	if err != nil {
		t.Fatal(err)
	}
	mapped := renderNftables(ForwardingInventory{Plans: []network.ForwardingPlan{subnet}})
	if !strings.Contains(mapped, `ip daddr 10.9.0.0/24 dnat ip prefix to ip daddr map { 10.9.0.0/24 : 192.168.0.0/24 }`) {
		t.Errorf("prefix translation missing in:\n%s", mapped)
	}
	if !strings.Contains(mapped, `iifname "eth0" oifname "eth1" ip saddr 192.168.10.0/24 ct original ip daddr 10.9.0.0/24 meta l4proto tcp accept`) {
		t.Errorf("unrestricted protocol grant missing in:\n%s", mapped)
	}
	if !strings.Contains(renderNftables(ForwardingInventory{}), "delete table inet nilo") {
		t.Error("an empty inventory must remove the owned table")
	}
}
