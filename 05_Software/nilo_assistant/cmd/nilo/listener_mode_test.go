package main

import (
	"errors"
	"strings"
	"testing"

	"nilo.local/nilo/internal/appliance"
)

func TestResolveListenerModeSelectsFirstBootOnlyForTheApplianceProfile(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name           string
		profile        string
		bindAddress    string
		managementMode bool
		managementPort int
		mode           listenerMode
		address        string
		refused        bool
	}{
		{
			name: "appliance with no listener option", profile: "appliance",
			mode: listenerProvisioning, address: "0.0.0.0:8443",
		},
		{
			name: "appliance with an explicit allowlist", profile: "appliance",
			managementMode: true, managementPort: 9443,
			mode: listenerManaged, address: "0.0.0.0:9443",
		},
		{
			name: "appliance with a legacy bind", profile: "appliance",
			bindAddress: "192.168.1.103:8443",
			mode:        listenerBind, address: "192.168.1.103:8443",
		},
		{
			name: "simulation with a loopback bind", profile: "simulation",
			bindAddress: "127.0.0.1:8443",
			mode:        listenerBind, address: "127.0.0.1:8443",
		},
		{
			name: "simulation with no listener option", profile: "simulation",
			refused: true,
		},
		{
			name: "both a bind and an allowlist", profile: "appliance",
			bindAddress: "192.168.1.103:8443", managementMode: true, managementPort: 8443,
			refused: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mode, address, err := resolveListenerMode(
				testCase.profile, testCase.bindAddress, testCase.managementMode, testCase.managementPort)
			if testCase.refused {
				if err == nil {
					t.Fatalf("resolveListenerMode = %q/%q, want a refusal", mode, address)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveListenerMode: %v", err)
			}
			if mode != testCase.mode || address != testCase.address {
				t.Fatalf("resolveListenerMode = %q/%q, want %q/%q", mode, address, testCase.mode, testCase.address)
			}
		})
	}
}

// First boot is a deliberate, temporary widening. An appliance that has already
// been commissioned must not re-enter it by being started with no options.
func TestCheckProvisioningPermittedRefusesACommissionedAppliance(t *testing.T) {
	t.Parallel()

	if err := checkProvisioningPermitted(listenerProvisioning, appliance.CommissioningProvisioning); err != nil {
		t.Fatalf("first boot on a provisioning appliance was refused: %v", err)
	}

	err := checkProvisioningPermitted(listenerProvisioning, appliance.CommissioningCommissioned)
	if !errors.Is(err, errListenerOptionsRequired) {
		t.Fatalf("checkProvisioningPermitted on a commissioned appliance = %v, want errListenerOptionsRequired", err)
	}
	for _, named := range []string{"--bind", "--management-port"} {
		if !strings.Contains(err.Error(), named) {
			t.Fatalf("refusal %q does not name %s", err, named)
		}
	}

	for _, mode := range []listenerMode{listenerBind, listenerManaged} {
		if err := checkProvisioningPermitted(mode, appliance.CommissioningCommissioned); err != nil {
			t.Fatalf("%q on a commissioned appliance was refused: %v", mode, err)
		}
	}
}
