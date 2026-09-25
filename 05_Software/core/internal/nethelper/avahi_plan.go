package nethelper

import (
	"errors"
	"regexp"
	"sort"
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
	Key           string
	HostFQDN      string
	UseGlobalHost bool
	Interfaces    []string
	Aliases       []avahiAliasPlan
	Service       *avahiServicePlan
}

// avahiPublicationGroups gives each interface/purpose pair its own process-
// owned EntryGroup. A collision or restart in one alias cannot withdraw the
// permanent serial service on another group.
func avahiPublicationGroups(advertisement network.MDNSAdvertisement) ([]avahiPlan, error) {
	plan, err := avahiPublicationPlan(advertisement)
	if err != nil {
		return nil, err
	}
	groups := make([]avahiPlan, 0, len(plan.Interfaces)+len(plan.Aliases))
	for _, interfaceName := range plan.Interfaces {
		group := avahiPlan{
			Key:      "serial/" + plan.HostFQDN + "/" + interfaceName,
			HostFQDN: plan.HostFQDN, Interfaces: []string{interfaceName}, Service: plan.Service,
		}
		groups = append(groups, group)
	}
	for _, alias := range plan.Aliases {
		for _, interfaceName := range alias.Interfaces {
			groups = append(groups, avahiPlan{
				Key:      "alias/" + alias.HostFQDN + "/" + interfaceName,
				HostFQDN: alias.HostFQDN, Interfaces: []string{interfaceName},
			})
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Key < groups[j].Key })
	return groups, nil
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

// applyAvahiEffectiveHost reuses Avahi's implicit address record when it is
// already the requested serial name. A preserved administrator hostname stays
// independent: NILO publishes the serial as an explicit AddAddress alias.
func applyAvahiEffectiveHost(plan avahiPlan, effectiveFQDN string) (avahiPlan, error) {
	effectiveFQDN = strings.TrimSpace(effectiveFQDN)
	if !avahiLocalFQDN.MatchString(effectiveFQDN) {
		return avahiPlan{}, errors.New("Avahi returned an invalid effective local hostname")
	}
	if strings.EqualFold(effectiveFQDN, plan.HostFQDN) {
		plan.UseGlobalHost = true
		if plan.Service != nil {
			service := *plan.Service
			service.HostFQDN = ""
			plan.Service = &service
		}
	}
	return plan, nil
}
