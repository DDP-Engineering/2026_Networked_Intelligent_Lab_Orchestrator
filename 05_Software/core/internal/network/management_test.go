package network

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func benchInterfaces() []Interface {
	return []Interface{
		{ID: "nm-eth0", Name: "eth0", AdapterID: "aa:00:00:00:00:01", Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.0.2.2/24"}, Gateway4: "192.0.2.1", Controllable: true},
		{ID: "nm-wlan0", Name: "wlan0", AdapterID: "aa:00:00:00:00:02", Kind: KindWiFi, State: StateConnected, IPv4: []string{"198.51.100.2/24"}, Gateway4: "198.51.100.1", Controllable: true},
		{ID: "nm-eth1", Name: "eth1", AdapterID: "aa:00:00:00:00:03", Kind: KindEthernet, State: StateDisconnected, Controllable: true},
		{ID: "nm-lo", Name: "lo", Kind: KindLoopback, State: StateConnected, IPv4: []string{"127.0.0.1/8"}},
	}
}

func client(address string) ManagementContext {
	return ManagementContext{ClientAddress: netip.MustParseAddr(address)}
}

// TestASinglePathRefusesTheChangeAndNamesIt covers REQ-REL-4c07b1ea: the only
// usable management path cannot be disabled or repurposed, and the refusal
// says which path it would have removed.
func TestASinglePathRefusesTheChangeAndNamesIt(t *testing.T) {
	t.Parallel()
	single := benchInterfaces()[:1]
	for _, change := range []Change{
		{Action: ActionInterfaceDisable, Target: "nm-eth0"},
		{Action: ActionInterfaceSettings, Target: "nm-eth0", Settings: &InterfaceSettings{AdapterID: "aa:00:00:00:00:01", Method: MethodStatic, Address: netip.MustParsePrefix("10.0.0.2/24")}},
		{Action: ActionInterfaceSettings, Target: "nm-eth0", Settings: &InterfaceSettings{AdapterID: "aa:00:00:00:00:01", Method: MethodAutomatic, DNSAutomatic: true}},
		{Action: ActionWiFiJoin, Target: "nm-eth0", WiFi: &WiFiJoin{SSID: "other", Security: WiFiOpen}},
	} {
		err := ValidateManagementChange(change, single, nil, client("192.0.2.13"))
		if !errors.Is(err, ErrManagementPathRequired) {
			t.Fatalf("%s on the only path: %v, want ErrManagementPathRequired", change.Action, err)
		}
		if !strings.Contains(err.Error(), "eth0") || !strings.Contains(err.Error(), "192.0.2.13") {
			t.Fatalf("the refusal %q does not name the path and the client", err.Error())
		}
	}
	// A change that keeps the address keeps the path: protecting it, or
	// re-saving the same static address, removes nothing.
	keep := Change{Action: ActionInterfaceSettings, Target: "nm-eth0", Settings: &InterfaceSettings{
		AdapterID: "aa:00:00:00:00:01", Method: MethodStatic, Address: netip.MustParsePrefix("192.0.2.2/24"),
		Gateway: netip.MustParseAddr("192.0.2.1"), ManagementProtected: true}}
	if err := ValidateManagementChange(keep, single, nil, client("192.0.2.13")); err != nil {
		t.Fatalf("keeping the same address was refused: %v", err)
	}
}

// TestASecondUsablePathAllowsTheChange is the two-path half of REQ-REL-4c07b1ea.
// A second path counts only when the client could actually use it: a path the
// operator protected, or one with a gateway when the client is routed rather
// than on the link being changed.
func TestASecondUsablePathAllowsTheChange(t *testing.T) {
	t.Parallel()
	interfaces := benchInterfaces()
	disableEth0 := Change{Action: ActionInterfaceDisable, Target: "nm-eth0"}

	// The client is on eth0's own link. wlan0 is connected with a gateway, but
	// nothing says this client can reach it, so eth0 is still the only path.
	err := ValidateManagementChange(disableEth0, interfaces, nil, client("192.0.2.13"))
	if !errors.Is(err, ErrManagementPathRequired) {
		t.Fatalf("on-link client with an unrelated second adapter: %v, want a refusal", err)
	}
	// The operator protected wlan0 as a management path: it counts.
	protectedWlan := []InterfaceSettings{{AdapterID: "aa:00:00:00:00:02", Method: MethodAutomatic, DNSAutomatic: true, ManagementProtected: true}}
	if err := ValidateManagementChange(disableEth0, interfaces, protectedWlan, client("192.0.2.13")); err != nil {
		t.Fatalf("with a protected second path: %v, want allowed", err)
	}
	// A routed client reaches the appliance through whichever gateway path is
	// left, so a connected adapter with a gateway counts.
	if err := ValidateManagementChange(disableEth0, interfaces, nil, client("203.0.113.9")); err != nil {
		t.Fatalf("routed client with a second gateway path: %v, want allowed", err)
	}
	// An unplugged adapter never counts, protected or not.
	protectedEth1 := []InterfaceSettings{{AdapterID: "aa:00:00:00:00:03", Method: MethodAutomatic, DNSAutomatic: true, ManagementProtected: true}}
	err = ValidateManagementChange(disableEth0, interfaces[:1], protectedEth1, client("192.0.2.13"))
	if !errors.Is(err, ErrManagementPathRequired) {
		t.Fatalf("a protected but absent adapter counted as a path: %v", err)
	}
	// Disabling a spare adapter that carries no management path is always fine.
	if err := ValidateManagementChange(Change{Action: ActionInterfaceDisable, Target: "nm-eth1"}, interfaces, nil, client("192.0.2.13")); err != nil {
		t.Fatalf("disabling the spare adapter: %v", err)
	}
}

// TestTheTunnelIsAManagementPathWhenTheClientArrivesThroughIt covers the VPN
// clause: deactivating or switching the tunnel the administrator is connected
// through needs another path.
func TestTheTunnelIsAManagementPathWhenTheClientArrivesThroughIt(t *testing.T) {
	t.Parallel()
	interfaces := benchInterfaces()[:1]
	through := ManagementContext{
		ClientAddress: netip.MustParseAddr("10.60.0.9"),
		Tunnel:        &TunnelPath{ProfileID: "profile-a", Name: "office", Interface: "nilo0", Routes: []netip.Prefix{netip.MustParsePrefix("10.60.0.0/16")}},
	}
	for _, change := range []Change{
		{Action: ActionVPNDeactivate, Target: "profile-a"},
		{Action: ActionVPNActivate, Target: "profile-b"},
	} {
		err := ValidateManagementChange(change, interfaces, nil, through)
		if !errors.Is(err, ErrManagementPathRequired) || !strings.Contains(err.Error(), "office") {
			t.Fatalf("%s through the tunnel: %v, want a refusal naming office", change.Action, err)
		}
	}
	// A client on the wired link is not using the tunnel, so the tunnel may go.
	if err := ValidateManagementChange(Change{Action: ActionVPNDeactivate, Target: "profile-a"}, interfaces, nil,
		ManagementContext{ClientAddress: netip.MustParseAddr("192.0.2.13"), Tunnel: through.Tunnel}); err != nil {
		t.Fatalf("deactivating a tunnel the client does not use: %v", err)
	}
	// A protected wired adapter is a retained path even for a tunnel client.
	protectedEth0 := []InterfaceSettings{{AdapterID: "aa:00:00:00:00:01", Method: MethodAutomatic, DNSAutomatic: true, ManagementProtected: true}}
	if err := ValidateManagementChange(Change{Action: ActionVPNDeactivate, Target: "profile-a"}, interfaces, protectedEth0, through); err != nil {
		t.Fatalf("deactivating the tunnel with a protected wired path: %v", err)
	}
}

// TestAnUnknownClientOnlyProtectsWhatTheOperatorProtected: a local or loopback
// caller traces to no adapter, so only operator-protected interfaces are
// management paths, and the last protected one still cannot go.
func TestAnUnknownClientOnlyProtectsWhatTheOperatorProtected(t *testing.T) {
	t.Parallel()
	interfaces := benchInterfaces()
	loopback := ManagementContext{ClientAddress: netip.MustParseAddr("127.0.0.1")}
	if err := ValidateManagementChange(Change{Action: ActionInterfaceDisable, Target: "nm-eth0"}, interfaces, nil, loopback); err != nil {
		t.Fatalf("a loopback caller disabling an unprotected adapter: %v", err)
	}
	protectedEth0 := []InterfaceSettings{{AdapterID: "aa:00:00:00:00:01", Method: MethodAutomatic, DNSAutomatic: true, ManagementProtected: true}}
	err := ValidateManagementChange(Change{Action: ActionInterfaceDisable, Target: "nm-eth0"}, interfaces, protectedEth0, loopback)
	if !errors.Is(err, ErrManagementPathRequired) {
		t.Fatalf("disabling the only protected adapter: %v, want a refusal", err)
	}
	paths := TraceManagementPaths(interfaces, protectedEth0, loopback)
	if len(paths) != 1 || paths[0].Name != "eth0" || !strings.Contains(paths[0].Reason, "protected") {
		t.Fatalf("traced paths = %+v, want the protected adapter with its reason", paths)
	}
}
