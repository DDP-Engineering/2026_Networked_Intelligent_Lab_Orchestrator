package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"sort"

	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/httpapi"
	"nilo.local/nilo/internal/nethelper"
	"nilo.local/nilo/internal/network"
)

type runtimeFriendlyNamePublisher struct {
	provisioning *network.MDNSAnnouncer
	commissioned *network.MDNSAnnouncer
	selected     func() *network.MDNSAnnouncer
	interfaces   func(context.Context) ([]string, error)
}

func (publisher runtimeFriendlyNamePublisher) ReplaceFriendlyName(ctx context.Context, name string) error {
	if err := publisher.provisioning.SetFriendlyName(name); err != nil {
		return err
	}
	if err := publisher.commissioned.SetFriendlyName(name); err != nil {
		return err
	}
	interfaces, err := publisher.interfaces(ctx)
	if err != nil {
		return err
	}
	selected := publisher.selected()
	if selected == nil {
		return errors.New("active appliance discovery announcer is unavailable")
	}
	err = selected.Announce(ctx, interfaces)
	if err != nil && errors.Is(err, network.ErrMDNSCollision) {
		var collision *network.MDNSCollisionError
		if errors.As(err, &collision) && len(collision.Interfaces) != 0 {
			return collision
		}
		return network.NewMDNSCollisionError(interfaces)
	}
	return err
}

func applianceDiscoveryMetadata(
	helperIdentity nethelper.ApplianceIdentity,
	stored appliance.Identity,
	state, version string,
	port int,
	observed []network.Interface,
	managementInterfaces []string,
) (*httpapi.ApplianceMetadata, network.MDNSApplianceIdentity, error) {
	identity := network.MDNSApplianceIdentity{
		ApplianceID: stored.ID, SerialName: helperIdentity.SerialName,
		FriendlyName: stored.FriendlyName, Port: uint16(port), State: state, Version: version,
	}
	if helperIdentity.Schema != 1 || port < 1 || port > 65535 {
		return nil, identity, errors.New("valid helper identity and management port are required")
	}
	allowed := make(map[string]bool, len(managementInterfaces))
	for _, name := range managementInterfaces {
		allowed[name] = true
	}
	addresses := map[string]bool{}
	macs := map[string]bool{}
	for _, item := range observed {
		if !allowed[item.Name] {
			continue
		}
		if address, err := net.ParseMAC(item.AdapterID); err == nil && len(address) == 6 {
			macs[address.String()] = true
		}
		for _, text := range append(append([]string(nil), item.IPv4...), item.IPv6...) {
			prefix, err := netip.ParsePrefix(text)
			if err == nil && !prefix.Addr().IsUnspecified() {
				addresses[prefix.Addr().Unmap().String()] = true
			}
		}
	}
	metadata := &httpapi.ApplianceMetadata{
		ID: stored.ID, SerialName: helperIdentity.SerialName, FriendlyName: stored.FriendlyName,
		State: state, Version: version,
		ManagementURL: fmt.Sprintf("https://%s.local:%d/", helperIdentity.SerialName, port),
	}
	for address := range addresses {
		metadata.ManagementAddresses = append(metadata.ManagementAddresses, address)
	}
	for address := range macs {
		metadata.ManagementMACs = append(metadata.ManagementMACs, address)
	}
	sort.Strings(metadata.ManagementAddresses)
	sort.Strings(metadata.ManagementMACs)
	if _, err := httpapi.NewDiscoveryDescriptionHandler(httpapi.DiscoveryDescription{
		ApplianceID: metadata.ID, SerialName: metadata.SerialName,
		FriendlyName: metadata.FriendlyName, PresentationURL: metadata.ManagementURL,
		HardwareAddresses: metadata.ManagementMACs,
	}); err != nil {
		return nil, identity, err
	}
	return metadata, identity, nil
}

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
