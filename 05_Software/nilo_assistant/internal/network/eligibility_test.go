package network

import "testing"

// Eligibility has to follow what the provider observed about the hardware and
// never the kernel name. The last two rows are the ones that settle it: a
// physical adapter wearing a container name stays eligible, and a virtual
// device wearing a built-in adapter name stays ineligible.
func TestEligibilityFollowsHardwareFactsNotInterfaceNames(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		candidate Interface
		eligible  bool
	}{
		{
			name: "physical ethernet adapter",
			candidate: Interface{
				Name: "enp1s0", Kind: KindEthernet, AdapterID: "02:00:00:00:00:01",
				Physical: true, State: StateConnected,
			},
			eligible: true,
		},
		{
			name: "physical wireless adapter",
			candidate: Interface{
				Name: "wlp2s0", Kind: KindWiFi, AdapterID: "02:00:00:00:00:02",
				Physical: true, State: StateDisconnected,
			},
			eligible: true,
		},
		{
			name: "virtual ethernet device",
			candidate: Interface{
				Name: "veth9f2a41", Kind: KindEthernet, AdapterID: "02:00:00:00:00:03",
				State: StateConnected,
			},
		},
		{
			name: "bridge",
			candidate: Interface{
				Name: "br-lab0", Kind: KindOther, AdapterID: "02:00:00:00:00:04",
				State: StateConnected,
			},
		},
		{
			name:      "loopback device",
			candidate: Interface{Name: "lo", Kind: KindLoopback, Physical: true, State: StateConnected},
		},
		{
			name: "wireguard tunnel device",
			candidate: Interface{
				Name: "nilo0", Kind: KindWireGuard, AdapterID: "02:00:00:00:00:05",
				Physical: true, State: StateConnected,
			},
		},
		{
			name:      "physical adapter without a stable identity",
			candidate: Interface{Name: "enp2s0", Kind: KindEthernet, Physical: true, State: StateConnected},
		},
		{
			name: "physical adapter whose identity is only whitespace",
			candidate: Interface{
				Name: "enp3s0", Kind: KindEthernet, AdapterID: "   ",
				Physical: true, State: StateConnected,
			},
		},
		{
			name: "physical adapter named like a container bridge",
			candidate: Interface{
				Name: "docker0", Kind: KindEthernet, AdapterID: "02:00:00:00:00:06",
				Physical: true, State: StateConnected,
			},
			eligible: true,
		},
		{
			name: "virtual device named like a built-in adapter",
			candidate: Interface{
				Name: "enp1s0", Kind: KindEthernet, AdapterID: "02:00:00:00:00:07",
				State: StateConnected,
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if eligible := EligibleForManagement(testCase.candidate); eligible != testCase.eligible {
				t.Fatalf("EligibleForManagement(%q) = %t, want %t",
					testCase.candidate.Name, eligible, testCase.eligible)
			}
		})
	}
}
