package network

import (
	"context"
	"errors"
	"net/netip"
	"sort"
)

// CommissioningLink identifies one safe Ethernet adapter without carrying
// profile or address configuration.
type CommissioningLink struct {
	AdapterID string `json:"adapter_id"`
	Interface string `json:"interface"`
}

type CommissioningLinkDriver interface {
	ReconcileCommissioningLinks(context.Context, []CommissioningLink) error
}

type CommissioningLinkService struct {
	provider    Provider
	driver      CommissioningLinkDriver
	coordinator *AdapterRoleCoordinator
}

func NewCommissioningLinkService(
	provider Provider, driver CommissioningLinkDriver, coordinator *AdapterRoleCoordinator,
) (*CommissioningLinkService, error) {
	if provider == nil || driver == nil {
		return nil, errors.New("commissioning links require a provider and driver")
	}
	if coordinator == nil {
		coordinator = NewAdapterRoleCoordinator()
	}
	return &CommissioningLinkService{provider: provider, driver: driver, coordinator: coordinator}, nil
}

// Reconcile derives all safe first-boot Ethernet adapters. Disabled removes
// only helper-owned commissioning profiles.
func (service *CommissioningLinkService) Reconcile(ctx context.Context, provisioning bool) error {
	if !provisioning {
		return service.driver.ReconcileCommissioningLinks(ctx, []CommissioningLink{})
	}
	interfaces, err := service.provider.Interfaces(ctx)
	if err != nil {
		return err
	}
	links := make([]CommissioningLink, 0)
	for _, candidate := range interfaces {
		if candidate.Kind != KindEthernet || !candidate.Physical || canonicalAdapterID(candidate.AdapterID) == "" {
			continue
		}
		if candidate.Settings != nil && candidate.Settings.Saved || hasNonLinkLocalPrefixes(candidate.IPv4) ||
			hasNonLinkLocalPrefixes(candidate.IPv6) || candidate.Gateway4 != "" || candidate.Gateway6 != "" {
			continue
		}
		if err := service.coordinator.Check([]AdapterRoleClaim{{
			AdapterID: candidate.AdapterID, Interface: candidate.Name, Role: AdapterRoleManagement,
		}}); err != nil {
			continue
		}
		links = append(links, CommissioningLink{AdapterID: canonicalAdapterID(candidate.AdapterID), Interface: candidate.Name})
	}
	sort.Slice(links, func(first, second int) bool { return links[first].AdapterID < links[second].AdapterID })
	return service.driver.ReconcileCommissioningLinks(ctx, links)
}

func hasNonLinkLocalPrefixes(addresses []string) bool {
	for _, value := range addresses {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !prefix.Addr().IsLinkLocalUnicast() {
			return true
		}
	}
	return false
}
