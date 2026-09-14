package nethelper

import (
	"errors"
	"regexp"
	"strings"

	"nilo.local/nilo/internal/network"
)

type avahiServicePlan struct {
	Type     string
	Domain   string
	Instance string
	HostFQDN string
	Port     uint16
	TXT      []string
}

type avahiPlan struct {
	HostFQDN   string
	Interfaces []string
	Aliases    []avahiAliasPlan
	Service    *avahiServicePlan
}

type avahiAliasPlan struct {
	HostFQDN   string
	Interfaces []string
}

// avahiPublicationPlan translates the closed network contract to values used
// by Avahi. It contains no configuration syntax and owns no daemon policy.
func avahiPublicationPlan(advertisement network.MDNSAdvertisement) (avahiPlan, error) {
	if err := advertisement.Validate(); err != nil {
		return avahiPlan{}, err
	}
	plan := avahiPlan{
		HostFQDN: advertisement.LocalName(), Interfaces: append([]string(nil), advertisement.Interfaces...),
	}
	if advertisement.Service != nil {
		plan.Service = &avahiServicePlan{
			Type: advertisement.Service.Type, Domain: advertisement.Service.Domain,
			Instance: advertisement.Service.Instance, HostFQDN: advertisement.LocalName(),
			Port: advertisement.Service.Port, TXT: append([]string(nil), advertisement.Service.TXT...),
		}
	}
	for _, alias := range advertisement.Aliases {
		plan.Aliases = append(plan.Aliases, avahiAliasPlan{
			HostFQDN: alias.HostName + ".local", Interfaces: append([]string(nil), alias.Interfaces...),
		})
	}
	return plan, nil
}

var avahiLocalFQDN = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.local$`)

// applyAvahiEffectiveHost refuses collision renaming because a suffixed name
// cannot be covered predictably by the appliance certificate. Operators must
// commission a unique system hostname rather than publish an unusable URL.
func applyAvahiEffectiveHost(plan avahiPlan, effectiveFQDN string) (avahiPlan, error) {
	if plan.Service == nil {
		return plan, nil
	}
	effectiveFQDN = strings.TrimSpace(effectiveFQDN)
	if !avahiLocalFQDN.MatchString(effectiveFQDN) {
		return avahiPlan{}, errors.New("Avahi returned an invalid effective local hostname")
	}
	if !strings.EqualFold(effectiveFQDN, plan.HostFQDN) {
		return avahiPlan{}, errors.New("Avahi renamed the appliance hostname after a collision; configure a unique hostname and certificate")
	}
	return plan, nil
}
