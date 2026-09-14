package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sort"

	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/httpapi"
	"nilo.local/nilo/internal/network"
)

func requiredDiscoveryTLSNames(systemHostname string, includeRecoveryAlias bool) []string {
	names := []string{systemHostname + ".local"}
	if includeRecoveryAlias {
		names = append(names, "nilo.local")
	}
	return names
}

// missingDiscoveryNames reports which of the names the appliance will tell a
// user to visit are absent from the certificate it was given.
//
// A gap here is a misprepared image rather than a reason to stop. An appliance
// that refuses to start cannot be reached or commissioned at all, which is the
// very failure first-boot provisioning exists to remove, so the appliance
// serves on what it has, carries the gap in its management access status and
// reports it in the interface. A certificate that cannot be read or parsed is
// still an error: there is nothing to serve TLS from at all.
//
// The appliance reads the certificate it was given and never generates or
// replaces one.
func missingDiscoveryNames(certificatePath, systemHostname string, includeRecoveryAlias bool) ([]string, error) {
	required := requiredDiscoveryTLSNames(systemHostname, includeRecoveryAlias)
	return httpapi.ValidateCertificateNames(certificatePath, required)
}

// commissioningDependencies returns the two things completing commissioning
// needs: somewhere to record the lifecycle state, and a way to find out whether
// the credential published with the image still opens the appliance.
//
// Both start modes take them from here rather than assembling their own,
// because a hardware recovery reset turns a commissioned service back into a
// provisioning one while it runs. A mode wired with one and not the other would
// recover but refuse to recommission until the process was restarted, and that
// is exactly the kind of gap a second construction site invites.
func commissioningDependencies(application *app.App) (network.Commissioner, network.PublishedCredential) {
	return appliance.NewDurableCommissioner(application.CommissioningRepository()),
		appliance.NewPublishedCredential(application.IdentityService())
}

// mergeMissingNames joins two missing-name reports without repeating a name.
// The configured management names and the advertised discovery names overlap by
// design - the appliance's own host name is normally in both - so appending one
// list to the other would report that name twice, once in the log and again in
// the interface, as though two separate things were wrong.
func mergeMissingNames(first, second []string) []string {
	merged := append([]string(nil), first...)
	for _, name := range second {
		if !slices.Contains(merged, name) {
			merged = append(merged, name)
		}
	}
	return merged
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
