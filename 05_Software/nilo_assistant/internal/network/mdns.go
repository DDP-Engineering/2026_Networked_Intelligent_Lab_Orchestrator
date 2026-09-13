package network

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// mdnsHostName is one DNS label: what the appliance answers to on the local
// link, before the ".local" the responder appends.
var mdnsHostName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// ErrMDNSRejected is every refusal of a proposed advertisement.
var ErrMDNSRejected = errors.New("mDNS advertisement rejected")

// MDNSAdvertisement is what the appliance asks to be discoverable as while it
// is being commissioned: one name, on a named set of interfaces.
//
// It carries no addresses. The responder publishes whatever address each
// interface currently holds, which is what lets one URL keep working while DHCP
// renews or a cable moves.
type MDNSAdvertisement struct {
	// HostName is the single label, without the ".local" suffix.
	HostName string `json:"host_name"`
	// Interfaces are the interfaces to answer on, and nothing else.
	Interfaces []string `json:"interfaces"`
}

// Validate refuses an advertisement that could answer somewhere it should not,
// or that names something the responder cannot express.
func (advertisement MDNSAdvertisement) Validate() error {
	if !mdnsHostName.MatchString(advertisement.HostName) {
		return fmt.Errorf("%w: %q is not a usable local host name", ErrMDNSRejected, advertisement.HostName)
	}
	if len(advertisement.Interfaces) == 0 {
		return fmt.Errorf("%w: at least one interface is required", ErrMDNSRejected)
	}
	seen := map[string]bool{}
	for _, name := range advertisement.Interfaces {
		if !managementInterfaceName.MatchString(name) || name == "lo" {
			return fmt.Errorf("%w: invalid interface %q", ErrMDNSRejected, name)
		}
		if seen[name] {
			return fmt.Errorf("%w: duplicate interface %q", ErrMDNSRejected, name)
		}
		seen[name] = true
	}
	return nil
}

// LocalName is the name the advertisement publishes.
func (advertisement MDNSAdvertisement) LocalName() string {
	return advertisement.HostName + ".local"
}

// SameMDNSAdvertisement reports whether two advertisements ask for the same
// thing, so the helper is only asked to rewrite its configuration on a change.
func SameMDNSAdvertisement(first, second MDNSAdvertisement) bool {
	if !strings.EqualFold(first.HostName, second.HostName) || len(first.Interfaces) != len(second.Interfaces) {
		return false
	}
	for index := range first.Interfaces {
		if first.Interfaces[index] != second.Interfaces[index] {
			return false
		}
	}
	return true
}
