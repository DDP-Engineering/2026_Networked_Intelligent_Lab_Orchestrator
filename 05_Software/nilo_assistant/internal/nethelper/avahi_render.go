package nethelper

import (
	"fmt"
	"strings"

	"nilo.local/nilo/internal/network"
)

// avahiOwnershipMarker is how the helper recognizes a configuration it wrote
// itself. It is the same discipline the nftables tables use: NILO manages only
// what it created, and refuses to overwrite anything else on the appliance.
const avahiOwnershipMarker = "# managed by nilo-nethelper for first-boot discovery"

// renderAvahiConfiguration produces the responder configuration for one
// advertisement.
//
// The interface list is closed and the address families are fixed: the
// responder answers on the interfaces NILO derived and publishes the addresses
// they currently hold. Nothing here is supplied by an unprivileged caller as
// text - the caller sends a validated advertisement, never configuration syntax.
func renderAvahiConfiguration(advertisement network.MDNSAdvertisement) string {
	var configuration strings.Builder
	configuration.WriteString(avahiOwnershipMarker + "\n")
	configuration.WriteString("[server]\n")
	fmt.Fprintf(&configuration, "host-name=%s\n", advertisement.HostName)
	configuration.WriteString("domain-name=local\n")
	fmt.Fprintf(&configuration, "allow-interfaces=%s\n", strings.Join(advertisement.Interfaces, ","))
	configuration.WriteString("use-ipv4=yes\n")
	configuration.WriteString("use-ipv6=yes\n")
	configuration.WriteString("[publish]\n")
	configuration.WriteString("publish-addresses=yes\n")
	configuration.WriteString("publish-hinfo=no\n")
	configuration.WriteString("publish-workstation=no\n")
	return configuration.String()
}

// avahiConfigurationState reports whether existing responder configuration is
// one NILO wrote, one somebody else owns, or none at all.
func avahiConfigurationState(existing string, present bool) TableState {
	if !present {
		return TableAbsent
	}
	if strings.Contains(existing, avahiOwnershipMarker) {
		return TableOwned
	}
	return TableForeign
}
