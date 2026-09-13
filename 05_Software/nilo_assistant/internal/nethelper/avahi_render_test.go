package nethelper

import (
	"strings"
	"testing"

	"nilo.local/nilo/internal/network"
)

func advertisement() network.MDNSAdvertisement {
	return network.MDNSAdvertisement{
		HostName: "ddp-remote", Interfaces: []string{"enp1s0", "wlp2s0"},
	}
}

// The rendered configuration answers on the derived interfaces and nowhere
// else, and carries the marker that lets the helper recognize its own work.
func TestRenderAvahiConfigurationClosesTheInterfaceList(t *testing.T) {
	t.Parallel()

	rendered := renderAvahiConfiguration(advertisement())

	for _, wanted := range []string{
		avahiOwnershipMarker,
		"host-name=ddp-remote",
		"domain-name=local",
		"allow-interfaces=enp1s0,wlp2s0",
		"publish-addresses=yes",
	} {
		if !strings.Contains(rendered, wanted) {
			t.Fatalf("rendered configuration is missing %q:\n%s", wanted, rendered)
		}
	}
	// The responder must not volunteer anything about the appliance beyond the
	// name and addresses it was asked to publish.
	for _, refused := range []string{"publish-hinfo=yes", "publish-workstation=yes"} {
		if strings.Contains(rendered, refused) {
			t.Fatalf("rendered configuration publishes more than it was asked to: %q", refused)
		}
	}
	if strings.Contains(rendered, ".local.local") {
		t.Fatal("the host name was rendered with the domain suffix already attached")
	}
}

// NILO manages only configuration it wrote itself, the same way it treats its
// nftables tables, so an appliance that already runs a responder is not taken
// over.
func TestAvahiConfigurationStateRecognizesOwnership(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		existing string
		present  bool
		state    TableState
	}{
		{name: "nothing configured", state: TableAbsent},
		{name: "written by nilo", existing: renderAvahiConfiguration(advertisement()), present: true, state: TableOwned},
		{
			name:     "written by somebody else",
			existing: "[server]\nhost-name=bench\n", present: true, state: TableForeign,
		},
		{name: "empty file", existing: "", present: true, state: TableForeign},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if state := avahiConfigurationState(testCase.existing, testCase.present); state != testCase.state {
				t.Fatalf("avahiConfigurationState = %q, want %q", state, testCase.state)
			}
		})
	}
}

func TestMDNSAdvertisementRefusesWhatTheResponderCannotAnswer(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name          string
		advertisement network.MDNSAdvertisement
		accepted      bool
	}{
		{name: "one name on two interfaces", advertisement: advertisement(), accepted: true},
		{
			name: "host name carrying the domain suffix",
			advertisement: network.MDNSAdvertisement{
				HostName: "ddp-remote.local", Interfaces: []string{"enp1s0"},
			},
		},
		{
			name:          "no interface",
			advertisement: network.MDNSAdvertisement{HostName: "ddp-remote"},
		},
		{
			name: "loopback",
			advertisement: network.MDNSAdvertisement{
				HostName: "ddp-remote", Interfaces: []string{"lo"},
			},
		},
		{
			name: "duplicate interface",
			advertisement: network.MDNSAdvertisement{
				HostName: "ddp-remote", Interfaces: []string{"enp1s0", "enp1s0"},
			},
		},
		{
			name: "empty host name",
			advertisement: network.MDNSAdvertisement{
				HostName: "", Interfaces: []string{"enp1s0"},
			},
		},
		{
			name: "host name with a configuration separator",
			advertisement: network.MDNSAdvertisement{
				HostName: "ddp\nallow-interfaces=eth0", Interfaces: []string{"enp1s0"},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.advertisement.Validate()
			if testCase.accepted && err != nil {
				t.Fatalf("Validate() = %v, want the advertisement accepted", err)
			}
			if !testCase.accepted && err == nil {
				t.Fatal("Validate() accepted an advertisement the responder cannot answer")
			}
		})
	}
}

func TestMDNSLocalNameAppendsTheDomainOnce(t *testing.T) {
	t.Parallel()

	if got := advertisement().LocalName(); got != "ddp-remote.local" {
		t.Fatalf("LocalName() = %q, want %q", got, "ddp-remote.local")
	}
}
