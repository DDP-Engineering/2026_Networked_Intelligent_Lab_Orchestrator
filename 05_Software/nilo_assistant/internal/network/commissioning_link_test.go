package network

import (
	"context"
	"slices"
	"testing"
)

type commissioningLinkDriver struct {
	wanted []CommissioningLink
}

func (driver *commissioningLinkDriver) ReconcileCommissioningLinks(_ context.Context, links []CommissioningLink) error {
	driver.wanted = append([]CommissioningLink(nil), links...)
	return nil
}

func TestProvisioningArmsEverySafeUnconfiguredEthernetForLinkLocal(t *testing.T) {
	provider := &recoveryProvider{interfaces: []Interface{
		{Name: "eth0", AdapterID: "b", Kind: KindEthernet, Physical: true, State: StateDisconnected},
		{Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected},
		{Name: "eth2", AdapterID: "c", Kind: KindEthernet, Physical: true, State: StateConnected, IPv4: []string{"192.0.2.20/24"}},
		{Name: "wlan0", AdapterID: "wifi", Kind: KindWiFi, Physical: true, State: StateDisconnected},
	}}
	coordinator := NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{AdapterID: "b", Interface: "eth0", Role: AdapterRoleDevice}}
	})
	driver := &commissioningLinkDriver{}
	service, err := NewCommissioningLinkService(provider, driver, coordinator)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	want := []CommissioningLink{{AdapterID: "a", Interface: "eth1"}}
	if !slices.Equal(driver.wanted, want) {
		t.Fatalf("commissioning links = %+v, want %+v", driver.wanted, want)
	}
	if err := service.Reconcile(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(driver.wanted) != 0 {
		t.Fatalf("commissioned appliance retained automatic links: %+v", driver.wanted)
	}
}

func TestProvisioningRetainsOwnedLinkLocalInterfacesButRefusesRoutableAddresses(t *testing.T) {
	provider := &recoveryProvider{interfaces: []Interface{
		{Name: "eth0", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateConnected, IPv4: []string{"169.254.20.10/16"}, IPv6: []string{"fe80::1/64"}},
		{Name: "eth1", AdapterID: "b", Kind: KindEthernet, Physical: true, State: StateConnected, IPv4: []string{"192.0.2.10/24"}},
		{Name: "eth2", AdapterID: "c", Kind: KindEthernet, Physical: true, State: StateConnected, IPv6: []string{"2001:db8::10/64"}},
	}}
	driver := &commissioningLinkDriver{}
	service, err := NewCommissioningLinkService(provider, driver, NewAdapterRoleCoordinator())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	want := []CommissioningLink{{AdapterID: "a", Interface: "eth0"}}
	if !slices.Equal(driver.wanted, want) {
		t.Fatalf("commissioning links = %+v, want owned link-local interface to remain desired", driver.wanted)
	}
}
