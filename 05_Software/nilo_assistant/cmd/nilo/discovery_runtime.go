package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"

	"nilo.local/nilo/internal/network"
)

func requiredDiscoveryTLSNames(systemHostname string, includeRecoveryAlias bool) []string {
	names := []string{systemHostname + ".local"}
	if includeRecoveryAlias {
		names = append(names, "nilo.local")
	}
	return names
}

// boundDiscoveryInterfaces maps a legacy literal listener address back to the
// interface that owns it. A loopback listener has no local-link audience and
// therefore publishes nothing.
func boundDiscoveryInterfaces(ctx context.Context, provider network.Provider, listenerAddress string) ([]string, error) {
	host, _, err := net.SplitHostPort(listenerAddress)
	if err != nil {
		return nil, fmt.Errorf("read discovery listener address: %w", err)
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return nil, fmt.Errorf("read discovery listener host: %w", err)
	}
	address = address.Unmap()
	if address.IsLoopback() {
		return []string{}, nil
	}
	observed, err := provider.Interfaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("observe bound discovery interface: %w", err)
	}
	interfaces := make([]string, 0, 1)
	for _, candidate := range observed {
		for _, value := range append(append([]string(nil), candidate.IPv4...), candidate.IPv6...) {
			prefix, parseErr := netip.ParsePrefix(value)
			if parseErr == nil && prefix.Addr().Unmap() == address {
				interfaces = append(interfaces, candidate.Name)
				break
			}
		}
	}
	sort.Strings(interfaces)
	return interfaces, nil
}

// reconcileApplianceDiscovery selects the advertisement from the live
// lifecycle, so commissioning and network reset take effect without relying on
// service arguments or a reboot.
func reconcileApplianceDiscovery(
	ctx context.Context,
	management *network.ManagementAccessService,
	current **network.MDNSAnnouncer,
	provisioning, commissioned *network.MDNSAnnouncer,
) error {
	selected := commissioned
	if management.Provisioning() {
		selected = provisioning
	}
	if selected != nil && selected != *current {
		if *current != nil {
			if err := (*current).Withdraw(ctx); err != nil {
				return fmt.Errorf("withdraw previous appliance discovery: %w", err)
			}
		}
		selected.Invalidate()
		*current = selected
	}
	if selected == nil {
		return nil
	}
	return selected.Announce(ctx, management.ConfiguredInterfaces())
}
