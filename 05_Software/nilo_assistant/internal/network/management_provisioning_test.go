package network

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"
)

// provisioningInterfaces is a first-boot Mini PC: built-in Ethernet, Wi-Fi, a
// USB Ethernet adapter, and the virtual devices that must never carry
// management access.
func provisioningInterfaces() []Interface {
	return []Interface{
		{
			Name: "enp1s0", AdapterID: "02:00:00:00:00:01", Kind: KindEthernet, Physical: true,
			State: StateConnected, IPv4: []string{"192.168.10.24/24"},
		},
		{
			Name: "wlp2s0", AdapterID: "02:00:00:00:00:02", Kind: KindWiFi, Physical: true,
			State: StateConnected, IPv4: []string{"192.168.20.15/24"},
		},
		{
			Name: "veth9f2a41", AdapterID: "02:00:00:00:00:03", Kind: KindEthernet,
			State: StateConnected, IPv4: []string{"10.88.0.2/16"},
		},
		{
			Name: "br-lab0", AdapterID: "02:00:00:00:00:04", Kind: KindOther,
			State: StateConnected, IPv4: []string{"172.18.0.1/16"},
		},
		{Name: "lo", Kind: KindLoopback, State: StateConnected, IPv4: []string{"127.0.0.1/8"}},
	}
}

func usbEthernet() Interface {
	return Interface{
		Name: "enx00e04c680001", AdapterID: "02:00:00:00:00:05", Kind: KindEthernet, Physical: true,
		State: StateConnected, IPv4: []string{"192.168.30.7/24"},
	}
}

func newProvisioningService(
	t *testing.T, now *time.Time, provider *accessProvider, store *accessStore, firewall *accessFirewall,
) *ManagementAccessService {
	t.Helper()
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Now: func() time.Time { return *now },
		Provisioning: true, ProvisioningTLSNames: []string{"ddp-remote.local"},
		ConfirmWindow: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewManagementAccessService: %v", err)
	}
	return service
}

func mustAddr(t *testing.T, text string) netip.Addr {
	t.Helper()
	prefix, err := netip.ParsePrefix(text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	return prefix.Addr()
}

// An installer cannot know which adapter will become the management path, so
// first boot answers on every eligible one and on nothing else.
func TestProvisioningDerivesTheAllowlistFromEligibleInterfaces(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}

	service := newProvisioningService(t, &now, provider, store, firewall)

	want := []string{"enp1s0", "wlp2s0"}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, want) {
		t.Fatalf("derived allowlist = %v, want %v", got, want)
	}
	if len(firewall.applied) == 0 {
		t.Fatal("the derived allowlist never reached the firewall")
	}
	if got := firewall.applied[len(firewall.applied)-1].Interfaces; !slices.Equal(got, want) {
		t.Fatalf("firewall allowlist = %v, want %v", got, want)
	}
	if !store.present {
		t.Fatal("the derived allowlist was not persisted")
	}

	for _, admitted := range []string{"192.168.10.24/24", "192.168.20.15/24"} {
		if !service.Allows(mustAddr(t, admitted)) {
			t.Fatalf("destination %s on an eligible interface was refused", admitted)
		}
	}
	for _, refused := range []string{"10.88.0.2/16", "172.18.0.1/16", "127.0.0.1/8"} {
		if service.Allows(mustAddr(t, refused)) {
			t.Fatalf("destination %s on an ineligible interface was admitted", refused)
		}
	}
}

// Acceptance 6 and 7: an adapter absent at boot begins serving when it appears,
// and one removed stops, without the service being rebuilt.
func TestProvisioningFollowsAdaptersAsTheyAppearAndLeave(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}

	service := newProvisioningService(t, &now, provider, store, firewall)
	if service.Allows(mustAddr(t, "192.168.30.7/24")) {
		t.Fatal("an adapter that is not present was admitted")
	}

	provider.set(append(provisioningInterfaces(), usbEthernet()))
	now = now.Add(time.Second)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile after attaching the adapter: %v", err)
	}

	// Canonical order is by stable adapter identity, not by kernel name.
	want := []string{"enp1s0", "wlp2s0", "enx00e04c680001"}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, want) {
		t.Fatalf("allowlist after attach = %v, want %v", got, want)
	}
	if !service.Allows(mustAddr(t, "192.168.30.7/24")) {
		t.Fatal("the attached adapter is not carrying management access")
	}
	if got := firewall.applied[len(firewall.applied)-1].Interfaces; !slices.Equal(got, want) {
		t.Fatalf("firewall allowlist after attach = %v, want %v", got, want)
	}

	provider.set(provisioningInterfaces())
	now = now.Add(time.Second)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile after removing the adapter: %v", err)
	}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0", "wlp2s0"}) {
		t.Fatalf("allowlist after removal = %v, want the two built-in adapters", got)
	}
	if service.Allows(mustAddr(t, "192.168.30.7/24")) {
		t.Fatal("the removed adapter is still admitted")
	}
}

// Acceptance 12: a restart resumes the same allowlist from what was persisted.
func TestProvisioningResumesThePersistedAllowlistAfterRestart(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(append(provisioningInterfaces(), usbEthernet()))
	store := &accessStore{}

	first := newProvisioningService(t, &now, provider, store, &accessFirewall{})
	before := first.ConfiguredInterfaces()

	restarted := newProvisioningService(t, &now, provider, store, &accessFirewall{})
	if after := restarted.ConfiguredInterfaces(); !slices.Equal(after, before) {
		t.Fatalf("allowlist after restart = %v, want %v", after, before)
	}
	if !restarted.Allows(mustAddr(t, "192.168.30.7/24")) {
		t.Fatal("the restarted service does not admit an adapter it served before")
	}
}

// A wildcard listener with nothing to guard it must never exist: with no
// eligible adapter present the service admits nothing and installs no rule,
// rather than relaxing the configuration validation.
func TestProvisioningWithNoEligibleInterfaceAdmitsNothing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set([]Interface{
		{Name: "veth9f2a41", AdapterID: "02:00:00:00:00:03", Kind: KindEthernet, State: StateConnected, IPv4: []string{"10.88.0.2/16"}},
		{Name: "lo", Kind: KindLoopback, State: StateConnected, IPv4: []string{"127.0.0.1/8"}},
	})
	store := &accessStore{}
	firewall := &accessFirewall{}

	service := newProvisioningService(t, &now, provider, store, firewall)
	if got := service.ConfiguredInterfaces(); len(got) != 0 {
		t.Fatalf("allowlist with no eligible adapter = %v, want empty", got)
	}
	if len(firewall.applied) != 0 {
		t.Fatalf("a firewall rule was installed with no eligible adapter: %v", firewall.applied)
	}
	if service.Allows(mustAddr(t, "10.88.0.2/16")) {
		t.Fatal("an ineligible destination was admitted with no eligible adapter")
	}

	provider.set(provisioningInterfaces())
	now = now.Add(time.Second)
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile after the first eligible adapter appeared: %v", err)
	}
	if !service.Allows(mustAddr(t, "192.168.10.24/24")) {
		t.Fatal("the first eligible adapter to appear is not carrying management access")
	}
}

// Acceptance 14: after commissioning the allowlist is frozen, so attaching an
// adapter never reopens management access on it.
func TestCommissionedAllowlistDoesNotAdmitANewlyAttachedAdapter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}

	configuration := ManagementAccessConfiguration{
		Interfaces: []string{"enp1s0"}, Port: 8443, TLSNames: []string{"ddp-remote.local"},
	}
	service, err := NewManagementAccessService(ctx, ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Now: func() time.Time { return now },
		Initial: &configuration, ConfirmWindow: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewManagementAccessService: %v", err)
	}

	provider.set(append(provisioningInterfaces(), usbEthernet()))
	now = now.Add(time.Second)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile after attaching the adapter: %v", err)
	}

	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0"}) {
		t.Fatalf("commissioned allowlist = %v, want only the configured interface", got)
	}
	if service.Allows(mustAddr(t, "192.168.30.7/24")) {
		t.Fatal("a newly attached adapter was admitted on a commissioned appliance")
	}
	if service.Allows(mustAddr(t, "192.168.20.15/24")) {
		t.Fatal("an eligible interface outside the configured allowlist was admitted")
	}
	for _, applied := range firewall.applied {
		if !slices.Equal(applied.Interfaces, []string{"enp1s0"}) {
			t.Fatalf("firewall received %v, want only the configured interface", applied.Interfaces)
		}
	}
}

// While provisioning the allowlist is derived, so it is not an administrator's
// to set: the only way to change it is to commission the appliance.
func TestProvisioningRefusesAnExplicitAllowlistChange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}

	service := newProvisioningService(t, &now, provider, store, firewall)
	applied := len(firewall.applied)

	if _, err := service.Update(ctx, []string{"enp1s0"}, false, netip.Addr{}); !errors.Is(err, ErrManagementAccessDerived) {
		t.Fatalf("Update while provisioning = %v, want ErrManagementAccessDerived", err)
	}
	if _, err := service.Confirm(ctx, netip.Addr{}); !errors.Is(err, ErrManagementAccessDerived) {
		t.Fatalf("Confirm while provisioning = %v, want ErrManagementAccessDerived", err)
	}
	if len(firewall.applied) != applied {
		t.Fatal("a refused request still reached the firewall")
	}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0", "wlp2s0"}) {
		t.Fatalf("allowlist after a refused update = %v, want the derived set", got)
	}
}
