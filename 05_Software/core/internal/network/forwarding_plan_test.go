package network

import (
	"net/netip"
	"strings"
	"testing"
)

func planContext() ForwardingContext {
	return ForwardingContext{DeviceLink: "enx001", TunnelDevice: "nilo-wg0", LocalLink: "adapter-eth0"}
}

func kindsOf(plan ForwardingPlan) []ResourceKind {
	kinds := make([]ResourceKind, 0, len(plan.Resources))
	for _, resource := range plan.Resources {
		kinds = append(kinds, resource.Kind)
	}
	return kinds
}

func firstOf(plan ForwardingPlan, kind ResourceKind) (Resource, bool) {
	for _, resource := range plan.Resources {
		if resource.Kind == kind {
			return resource, true
		}
	}
	return Resource{}, false
}

func countOf(plan ForwardingPlan, kind ResourceKind) int {
	count := 0
	for _, resource := range plan.Resources {
		if resource.Kind == kind {
			count++
		}
	}
	return count
}

// TestThePlanGuardsBeforeItGrants covers the ordering half of REQ-SEC-a05f2c1b:
// the deny guard has to exist before anything is forwarded, or a moment exists
// in which traffic leaks by the wrong path.
func TestThePlanGuardsBeforeItGrants(t *testing.T) {
	t.Parallel()
	plan, err := PlanForwarding(validHost(t), planContext())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	kinds := kindsOf(plan)
	if len(kinds) == 0 {
		t.Fatal("the plan is empty")
	}
	if kinds[0] != ResourceDenyGuard {
		t.Fatalf("first resource = %q, want the deny guard", kinds[0])
	}
	firstGrant, lastGuard := -1, -1
	for index, kind := range kinds {
		if kind == ResourceForwardGrant && firstGrant < 0 {
			firstGrant = index
		}
		if kind == ResourceDenyGuard || kind == ResourceIPv6Deny {
			lastGuard = index
		}
	}
	if firstGrant < 0 {
		t.Fatal("the plan grants nothing at all")
	}
	if lastGuard > firstGrant {
		t.Fatalf("a guard at %d comes after the first grant at %d", lastGuard, firstGrant)
	}
	if _, present := firstOf(plan, ResourceIPv6Deny); !present {
		t.Fatal("the plan has no IPv6 denial, so IPv6 could bypass the IPv4 access policy")
	}
}

// TestALocalOriginalPlanTranslatesTheSourceOnly covers REQ-FUNC-64c8f2a9.
func TestALocalOriginalPlanTranslatesTheSourceOnly(t *testing.T) {
	t.Parallel()
	plan, err := PlanForwarding(validHost(t), planContext())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if countOf(plan, ResourceDestinationNAT) != 0 {
		t.Fatal("original addressing produced a destination translation")
	}
	source, present := firstOf(plan, ResourceSourceNAT)
	if !present {
		t.Fatal("no source translation, so the target would have to route back to the client itself")
	}
	if source.Translated.Addr().String() != "192.168.0.1" {
		t.Fatalf("source translated to %s, want the appliance's device-side address", source.Translated)
	}
	if source.Egress != "enx001" {
		t.Fatalf("source translation egress = %q, want the device link", source.Egress)
	}

	// The default policy is TCP, UDP and ICMP, one grant each, so a vendor
	// protocol that is not Modbus still works.
	if got := countOf(plan, ResourceForwardGrant); got != 3 {
		t.Fatalf("%d forward grants, want one per default protocol", got)
	}
	for _, resource := range plan.Resources {
		if resource.Kind != ResourceForwardGrant {
			continue
		}
		if resource.Ingress != "adapter-eth0" {
			t.Fatalf("grant ingress = %q, want the configured local interface", resource.Ingress)
		}
		if resource.Egress != "enx001" {
			t.Fatalf("grant egress = %q, want the device link", resource.Egress)
		}
		if resource.Source.String() != "192.168.1.0/24" {
			t.Fatalf("grant source = %s, want the permitted source", resource.Source)
		}
		if resource.Destination.String() != "192.168.0.130/32" {
			t.Fatalf("grant destination = %s, want the single target", resource.Destination)
		}
	}
}

// TestPreservedSourceRemovesTheSourceTranslation keeps the advanced setting
// honest: it is the absence of a translation, not a different one.
func TestPreservedSourceRemovesTheSourceTranslation(t *testing.T) {
	t.Parallel()
	preserved := validHost(t)
	preserved.Services.PreserveSource = true
	plan, err := PlanForwarding(preserved, planContext())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if countOf(plan, ResourceSourceNAT) != 0 {
		t.Fatal("source preservation still installed a source translation")
	}
	if countOf(plan, ResourceForwardGrant) == 0 {
		t.Fatal("source preservation removed the forwarding as well")
	}
}

// TestATranslatedSubnetIsOneRuleNotOnePerAddress covers REQ-FUNC-1d93b76e's
// explicit requirement that a prefix mapping does not enumerate addresses.
func TestATranslatedSubnetIsOneRuleNotOnePerAddress(t *testing.T) {
	t.Parallel()
	subnet := validHost(t)
	subnet.Scope = ScopeSubnet
	subnet.Target = netip.Addr{}
	subnet.Addressing = AddressingTranslated
	subnet.Exposed = prefix(t, "10.60.1.0/24")

	plan, err := PlanForwarding(subnet, planContext())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if got := countOf(plan, ResourceDestinationNAT); got != 1 {
		t.Fatalf("%d destination translations for a /24 exposure, want exactly 1", got)
	}
	destination, _ := firstOf(plan, ResourceDestinationNAT)
	if destination.Destination.String() != "10.60.1.0/24" {
		t.Fatalf("translation matches %s, want the exposed prefix", destination.Destination)
	}
	if destination.Translated.String() != "192.168.0.0/24" {
		t.Fatalf("translation maps to %s, want the device subnet", destination.Translated)
	}
	for _, resource := range plan.Resources {
		if resource.Kind == ResourceForwardGrant && resource.Destination.String() != "10.60.1.0/24" {
			t.Fatalf("grant destination = %s, want the exposed prefix", resource.Destination)
		}
	}
}

// TestATranslatedHostAlsoAuthorizesItsOriginalAddressOnlyWhenAsked covers the
// plan's rule that knowing an internal address must not bypass a chosen
// mapping: a translated device network grants the exposed address, and the
// original address is not silently reachable as well.
func TestATranslatedHostAlsoAuthorizesItsOriginalAddressOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	translated := validHost(t)
	translated.Addressing = AddressingTranslated
	translated.Exposed = prefix(t, "10.60.1.130/32")

	plan, err := PlanForwarding(translated, planContext())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	for _, resource := range plan.Resources {
		if resource.Kind != ResourceForwardGrant {
			continue
		}
		if resource.Destination.Addr().String() == "192.168.0.130" {
			t.Fatalf("a translated device network also grants the original address %s", resource.Destination)
		}
		if resource.Destination.String() != "10.60.1.130/32" {
			t.Fatalf("grant destination = %s, want the exposed address", resource.Destination)
		}
	}
}

// TestAVPNOnlyPlanWaitsWhenTheTunnelIsNotThere covers REQ-SEC-a05f2c1b and
// REQ-REL-b6d0e358 at the planning boundary: no tunnel means no grant, and the
// guard stays.
func TestAVPNOnlyPlanWaitsWhenTheTunnelIsNotThere(t *testing.T) {
	t.Parallel()
	viaTunnel := validHost(t)
	viaTunnel.Access = AccessWireGuard
	viaTunnel.LocalInterface = ""
	viaTunnel.LocalSources = nil
	viaTunnel.VPNProfileID = "profile-1"
	viaTunnel.VPNSources = []netip.Prefix{prefix(t, "10.60.0.0/16")}

	context := planContext()
	context.TunnelDevice = ""
	plan, err := PlanForwarding(viaTunnel, context)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !plan.Waiting {
		t.Fatal("a VPN-only device network with no tunnel is not reported as waiting")
	}
	if plan.WaitingReason == "" {
		t.Fatal("the waiting state has no reason, so an operator cannot act on it")
	}
	if countOf(plan, ResourceForwardGrant) != 0 {
		t.Fatal("a device network with no tunnel still granted forwarding")
	}
	if countOf(plan, ResourceDenyGuard) == 0 {
		t.Fatal("the deny guard was dropped along with the grants")
	}

	// With the tunnel present the same configuration grants only on the tunnel.
	running, err := PlanForwarding(viaTunnel, planContext())
	if err != nil {
		t.Fatalf("plan with tunnel: %v", err)
	}
	if running.Waiting {
		t.Fatal("the plan still waits although the tunnel is present")
	}
	for _, resource := range running.Resources {
		if resource.Kind == ResourceForwardGrant && resource.Ingress != "nilo-wg0" {
			t.Fatalf("grant ingress = %q, want the tunnel device only", resource.Ingress)
		}
	}
}

// TestBothAccessIsTheUnionOfTwoExplicitPaths covers the access-mode rule that
// both is two configured paths rather than a fallback.
func TestBothAccessIsTheUnionOfTwoExplicitPaths(t *testing.T) {
	t.Parallel()
	both := validHost(t)
	both.Access = AccessBoth
	both.VPNProfileID = "profile-1"
	both.VPNSources = []netip.Prefix{prefix(t, "10.60.0.0/16")}

	plan, err := PlanForwarding(both, planContext())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	ingresses := map[string]bool{}
	for _, resource := range plan.Resources {
		if resource.Kind == ResourceForwardGrant {
			ingresses[resource.Ingress] = true
		}
	}
	if !ingresses["adapter-eth0"] || !ingresses["nilo-wg0"] {
		t.Fatalf("grants cover %v, want both the local interface and the tunnel", ingresses)
	}
}

// TestOwnedResourceNamesAreUniqueAndAttributable covers REQ-PLAT-6d21f0b8 at
// the planning boundary: every resource this appliance creates has to be
// recognizable as its own, or removal has to guess.
func TestOwnedResourceNamesAreUniqueAndAttributable(t *testing.T) {
	t.Parallel()
	both := validHost(t)
	both.Access = AccessBoth
	both.VPNProfileID = "profile-1"
	both.VPNSources = []netip.Prefix{prefix(t, "10.60.0.0/16")}
	plan, err := PlanForwarding(both, planContext())
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !strings.HasPrefix(plan.Table, "nilo") {
		t.Fatalf("plan table = %q, want a NILO-owned name", plan.Table)
	}
	seen := map[string]bool{}
	for _, resource := range plan.Resources {
		if resource.Name == "" {
			t.Fatalf("resource %+v has no name", resource)
		}
		if !strings.Contains(resource.Name, both.ID) {
			t.Fatalf("resource name %q does not name its device network", resource.Name)
		}
		if seen[resource.Name] {
			t.Fatalf("resource name %q is used twice", resource.Name)
		}
		seen[resource.Name] = true
	}
}

// TestAnInvalidDeviceNetworkIsNotPlanned keeps the planner from producing
// resources for a configuration that was never acceptable.
func TestAnInvalidDeviceNetworkIsNotPlanned(t *testing.T) {
	t.Parallel()
	broken := validHost(t)
	broken.Target = address(t, "10.9.9.9")
	if _, err := PlanForwarding(broken, planContext()); err == nil {
		t.Fatal("an invalid device network produced a forwarding plan")
	}
}
