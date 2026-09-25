// Package discovery contains platform-neutral parsing and reconciliation for
// local appliance discovery. Network transports feed observations into this
// package; it never opens a socket itself.
package discovery

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Source names one standard discovery protocol.
type Source string

const (
	SourceMDNS Source = "mdns"
	SourceSSDP Source = "ssdp"
)

var (
	ErrNotFound  = errors.New("appliance not found")
	ErrAmbiguous = errors.New("appliance name is ambiguous")
)

// Observation is one validated-candidate record received from one protocol.
type Observation struct {
	Source            Source
	ID                string
	Serial            string
	FriendlyName      string
	State             string
	Version           string
	URL               string
	Addresses         []string
	HardwareAddresses []string
}

// Appliance is the safe deterministic projection printed or selected by CLI
// commands.
type Appliance struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Serial            string   `json:"serial"`
	FriendlyName      string   `json:"friendly_name,omitempty"`
	Addresses         []string `json:"addresses"`
	HardwareAddresses []string `json:"management_macs,omitempty"`
	URL               string   `json:"url,omitempty"`
	State             string   `json:"state,omitempty"`
	Version           string   `json:"version,omitempty"`
	Sources           []Source `json:"sources"`
}

// Merge validates observations, joins only matching UUIDs and returns stable
// results plus bounded public diagnostics.
func Merge(observations []Observation) ([]Appliance, []string) {
	type aggregate struct {
		appliance         Appliance
		addresses         map[string]bool
		hardwareAddresses map[string]bool
		sources           map[Source]bool
		urlBad            bool
	}
	byID := make(map[string]*aggregate)
	diagnostics := make([]string, 0)
	for _, observation := range observations {
		if err := validateObservation(observation); err != nil {
			diagnostics = append(diagnostics, "ignored invalid "+string(observation.Source)+" observation")
			continue
		}
		item := byID[observation.ID]
		if item == nil {
			item = &aggregate{
				appliance: Appliance{ID: observation.ID, Serial: observation.Serial},
				addresses: map[string]bool{}, sources: map[Source]bool{},
				hardwareAddresses: map[string]bool{},
			}
			byID[observation.ID] = item
		}
		if item.appliance.Serial != observation.Serial {
			diagnostics = append(diagnostics, "ignored conflicting serial for "+observation.ID)
			continue
		}
		item.sources[observation.Source] = true
		for _, address := range observation.Addresses {
			if strings.TrimSpace(address) != "" {
				item.addresses[address] = true
			}
		}
		for _, address := range observation.HardwareAddresses {
			if parsed, err := net.ParseMAC(address); err == nil && len(parsed) == 6 {
				item.hardwareAddresses[parsed.String()] = true
			}
		}
		mergeOptional(&item.appliance.FriendlyName, observation.FriendlyName, observation.ID, "friendly name", &diagnostics)
		mergeOptional(&item.appliance.State, observation.State, observation.ID, "state", &diagnostics)
		mergeOptional(&item.appliance.Version, observation.Version, observation.ID, "version", &diagnostics)
		if observation.URL != "" && !item.urlBad {
			if item.appliance.URL == "" {
				item.appliance.URL = observation.URL
			} else if item.appliance.URL != observation.URL {
				item.appliance.URL = ""
				item.urlBad = true
				diagnostics = append(diagnostics, "omitted conflicting URL for "+observation.ID)
			}
		}
	}

	result := make([]Appliance, 0, len(byID))
	for _, item := range byID {
		for address := range item.addresses {
			item.appliance.Addresses = append(item.appliance.Addresses, address)
		}
		for address := range item.hardwareAddresses {
			item.appliance.HardwareAddresses = append(item.appliance.HardwareAddresses, address)
		}
		for source := range item.sources {
			item.appliance.Sources = append(item.appliance.Sources, source)
		}
		sort.Strings(item.appliance.Addresses)
		sort.Strings(item.appliance.HardwareAddresses)
		sort.Slice(item.appliance.Sources, func(i, j int) bool { return item.appliance.Sources[i] < item.appliance.Sources[j] })
		item.appliance.Name = item.appliance.Serial
		if item.appliance.FriendlyName != "" {
			item.appliance.Name = item.appliance.FriendlyName
		}
		result = append(result, item.appliance)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Serial == result[j].Serial {
			return result[i].ID < result[j].ID
		}
		return result[i].Serial < result[j].Serial
	})
	if len(diagnostics) > 64 {
		diagnostics = diagnostics[:64]
	}
	return result, diagnostics
}

// SelectUnique resolves a permanent or friendly name without guessing among
// duplicate friendly labels.
func SelectUnique(appliances []Appliance, name string) (Appliance, error) {
	var selected *Appliance
	for index := range appliances {
		if appliances[index].Serial != name && appliances[index].FriendlyName != name {
			continue
		}
		if selected != nil && selected.ID != appliances[index].ID {
			return Appliance{}, ErrAmbiguous
		}
		copy := appliances[index]
		selected = &copy
	}
	if selected == nil {
		return Appliance{}, ErrNotFound
	}
	return *selected, nil
}

func validateObservation(observation Observation) error {
	if observation.Source != SourceMDNS && observation.Source != SourceSSDP {
		return errors.New("unknown source")
	}
	if _, err := uuid.Parse(observation.ID); err != nil {
		return errors.New("invalid UUID")
	}
	if !validSerial(observation.Serial) {
		return errors.New("invalid serial")
	}
	if observation.URL != "" {
		parsed, err := url.Parse(observation.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("unsafe URL")
		}
	}
	return nil
}

func validSerial(value string) bool {
	const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
	if len(value) != 15 || !strings.HasPrefix(value, "nilo-") {
		return false
	}
	for _, character := range value[5:] {
		if !strings.ContainsRune(alphabet, character) {
			return false
		}
	}
	return true
}

func mergeOptional(target *string, candidate, id, field string, diagnostics *[]string) {
	if candidate == "" {
		return
	}
	if *target == "" {
		*target = candidate
		return
	}
	if *target != candidate {
		*diagnostics = append(*diagnostics, fmt.Sprintf("ignored conflicting %s for %s", field, id))
	}
}
