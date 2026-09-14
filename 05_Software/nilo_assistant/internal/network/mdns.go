package network

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
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
	// Service is the permanent commissioned DNS-SD service. Provisioning and
	// temporary recovery publish only the nilo.local address alias.
	Service *MDNSService `json:"service,omitempty"`
	// Aliases are additional interface-scoped address records. Commissioned
	// recovery uses exactly nilo.local on its one temporary link.
	Aliases []MDNSAlias `json:"aliases,omitempty"`
}

type MDNSAlias struct {
	HostName   string   `json:"host_name"`
	Interfaces []string `json:"interfaces"`
}

// MDNSService is the closed DNS-SD record published for a commissioned NILO.
// Its fields are values, never responder configuration or arbitrary TXT data.
type MDNSService struct {
	Type     string   `json:"type"`
	Domain   string   `json:"domain"`
	Instance string   `json:"instance"`
	Port     uint16   `json:"port"`
	TXT      []string `json:"txt"`
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
	if advertisement.Service != nil {
		if err := validateMDNSService(advertisement.HostName, *advertisement.Service); err != nil {
			return err
		}
	}
	aliasNames := map[string]bool{}
	for _, alias := range advertisement.Aliases {
		if !mdnsHostName.MatchString(alias.HostName) || len(alias.Interfaces) == 0 {
			return fmt.Errorf("%w: invalid scoped alias", ErrMDNSRejected)
		}
		key := strings.ToLower(alias.HostName)
		if aliasNames[key] || strings.EqualFold(alias.HostName, advertisement.HostName) {
			return fmt.Errorf("%w: duplicate scoped alias %q", ErrMDNSRejected, alias.HostName)
		}
		aliasNames[key] = true
		seenInterfaces := map[string]bool{}
		for _, name := range alias.Interfaces {
			if !managementInterfaceName.MatchString(name) || name == "lo" || seenInterfaces[name] {
				return fmt.Errorf("%w: invalid alias interface %q", ErrMDNSRejected, name)
			}
			seenInterfaces[name] = true
		}
	}
	return nil
}

func validateMDNSService(hostName string, service MDNSService) error {
	if service.Type != "_nilo._tcp" || service.Domain != "local" {
		return fmt.Errorf("%w: unsupported DNS-SD service", ErrMDNSRejected)
	}
	if service.Port == 0 {
		return fmt.Errorf("%w: service port is required", ErrMDNSRejected)
	}
	prefix := "NILO "
	id := strings.TrimPrefix(service.Instance, prefix)
	parsedIdentity, identityErr := uuid.Parse(id)
	if !strings.HasPrefix(service.Instance, prefix) || identityErr != nil || parsedIdentity.String() != id {
		return fmt.Errorf("%w: invalid service instance", ErrMDNSRejected)
	}
	want := []string{
		"txtvers=1",
		"id=" + id,
		"hostname=" + hostName,
		fmt.Sprintf("url=https://%s.local:%d/", hostName, service.Port),
		"path=/",
		"state=commissioned",
	}
	if !slices.Equal(service.TXT, want) {
		return fmt.Errorf("%w: service TXT fields are not the version-1 allowlist", ErrMDNSRejected)
	}
	parsed, err := url.Parse(strings.TrimPrefix(service.TXT[3], "url="))
	if err != nil || parsed.Scheme != "https" || parsed.Path != "/" {
		return fmt.Errorf("%w: invalid management URL", ErrMDNSRejected)
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
	if first.Service == nil || second.Service == nil {
		if first.Service != nil || second.Service != nil {
			return false
		}
	} else if first.Service.Type != second.Service.Type ||
		first.Service.Domain != second.Service.Domain ||
		first.Service.Instance != second.Service.Instance ||
		first.Service.Port != second.Service.Port ||
		!slices.Equal(first.Service.TXT, second.Service.TXT) {
		return false
	}
	if len(first.Aliases) != len(second.Aliases) {
		return false
	}
	for index := range first.Aliases {
		if !strings.EqualFold(first.Aliases[index].HostName, second.Aliases[index].HostName) ||
			!slices.Equal(first.Aliases[index].Interfaces, second.Aliases[index].Interfaces) {
			return false
		}
	}
	return true
}
