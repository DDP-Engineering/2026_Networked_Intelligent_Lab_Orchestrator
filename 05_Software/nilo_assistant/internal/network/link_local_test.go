package network

import (
	"net/netip"
	"testing"
)

// Direct commissioning over a crossover cable has no DHCP server, so an
// Ethernet adapter has to be able to claim an IPv4 link-local address. Each
// adapter reaches a different one through the standard ARP probe, which is what
// keeps two of them from colliding on one route.
func TestLinkLocalIsAcceptedOnEthernetAndRefusedElsewhere(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		settings InterfaceSettings
		accepted bool
	}{
		{
			name:     "link-local on its own",
			settings: InterfaceSettings{AdapterID: "02:00:00:00:00:01", Method: MethodLinkLocal},
			accepted: true,
		},
		{
			name: "link-local with automatic DNS",
			settings: InterfaceSettings{
				AdapterID: "02:00:00:00:00:01", Method: MethodLinkLocal, DNSAutomatic: true,
			},
			accepted: true,
		},
		{
			name: "link-local with a static address",
			settings: InterfaceSettings{
				AdapterID: "02:00:00:00:00:01", Method: MethodLinkLocal,
				Address: netip.MustParsePrefix("192.168.1.10/24"),
			},
		},
		{
			name: "link-local with a gateway",
			settings: InterfaceSettings{
				AdapterID: "02:00:00:00:00:01", Method: MethodLinkLocal,
				Gateway: netip.MustParseAddr("192.168.1.1"),
			},
		},
		{
			name: "link-local on a wireless adapter",
			settings: InterfaceSettings{
				AdapterID: "02:00:00:00:00:02", Method: MethodLinkLocal,
				WiFi: &WiFiSettings{SSID: "DDP", Security: WiFiWPA2Personal},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.settings.Validate()
			if testCase.accepted && err != nil {
				t.Fatalf("Validate() = %v, want the configuration to be accepted", err)
			}
			if !testCase.accepted && err == nil {
				t.Fatal("Validate() accepted a link-local configuration it should refuse")
			}
		})
	}
}

// A link-local address is an ordinary observation on the interface, so an
// adapter carrying one is a usable management path like any other.
func TestLinkLocalAddressesCarryManagementAccess(t *testing.T) {
	t.Parallel()

	interfaces := []Interface{{
		Name: "enp1s0", AdapterID: "02:00:00:00:00:01", Kind: KindEthernet, Physical: true,
		State: StateConnected, IPv4: []string{"169.254.12.87/16"},
	}}
	if !managementConfigurationHasUsableInterface(interfaces, []string{"enp1s0"}) {
		t.Fatal("an adapter holding only a link-local address is not treated as usable")
	}
	if !EligibleForManagement(interfaces[0]) {
		t.Fatal("an adapter holding only a link-local address is not eligible")
	}
}
