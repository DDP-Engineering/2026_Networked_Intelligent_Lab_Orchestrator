package nethelper

import (
	"slices"
	"testing"

	"nilo.local/nilo/internal/network"
)

func advertisement() network.MDNSAdvertisement {
	return network.MDNSAdvertisement{
		HostName: "ddp-remote", Interfaces: []string{"enp1s0", "wlp2s0"},
	}
}

func TestAvahiPublicationPlanCarriesScopedAliasAndPermanentService(t *testing.T) {
	t.Parallel()

	wanted := advertisement()
	wanted.Service = &network.MDNSService{
		Type: "_nilo._tcp", Domain: "local", Instance: "NILO 7e799b52-624f-4cc8-9be6-213a3525a726", Port: 8443,
		TXT: []string{"txtvers=1", "id=7e799b52-624f-4cc8-9be6-213a3525a726", "hostname=ddp-remote", "url=https://ddp-remote.local:8443/", "path=/", "state=commissioned"},
	}
	plan, err := avahiPublicationPlan(wanted)
	if err != nil {
		t.Fatal(err)
	}
	if plan.HostFQDN != "ddp-remote.local" || !slices.Equal(plan.Interfaces, wanted.Interfaces) {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Service == nil || plan.Service.Type != "_nilo._tcp" || plan.Service.HostFQDN != "ddp-remote.local" {
		t.Fatalf("service plan = %+v", plan.Service)
	}
	if !slices.Equal(plan.Service.TXT, wanted.Service.TXT) {
		t.Fatalf("service TXT = %v", plan.Service.TXT)
	}
}

func TestAvahiEffectiveHostRefusesAnUncertifiedCollisionName(t *testing.T) {
	plan, err := avahiPublicationPlan(network.MDNSAdvertisement{
		HostName: "ddp-remote", Interfaces: []string{"enp1s0"},
		Service: &network.MDNSService{
			Type: "_nilo._tcp", Domain: "local", Instance: "NILO 7e799b52-624f-4cc8-9be6-213a3525a726", Port: 8443,
			TXT: []string{"txtvers=1", "id=7e799b52-624f-4cc8-9be6-213a3525a726", "hostname=ddp-remote", "url=https://ddp-remote.local:8443/", "path=/", "state=commissioned"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = applyAvahiEffectiveHost(plan, "ddp-remote-2.local"); err == nil {
		t.Fatal("an unpredictable collision-renamed hostname was advertised as a TLS URL")
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
