package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

var publicVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

// SSDPPresence is the closed public data from which the helper constructs
// root-device presence. It contains no packet bytes or arbitrary headers.
type SSDPPresence struct {
	ApplianceID string   `json:"appliance_id"`
	Version     string   `json:"version"`
	Port        uint16   `json:"port"`
	Interfaces  []string `json:"interfaces"`
	BootID      uint32   `json:"boot_id"`
	ConfigID    uint32   `json:"config_id"`
}

type SSDPPublisher interface {
	PublishSSDP(context.Context, SSDPPresence) error
	WithdrawSSDP(context.Context) error
}

type ssdpPublicationChecker interface {
	SSDPPublicationCurrent(context.Context) (bool, error)
}

type SSDPAnnouncer struct {
	mu        sync.Mutex
	publisher SSDPPublisher
	template  SSDPPresence
	current   []string
}

func NewSSDPAnnouncer(publisher SSDPPublisher, template SSDPPresence) (*SSDPAnnouncer, error) {
	if publisher == nil || template.ApplianceID == "" || template.Port == 0 || template.BootID == 0 || template.ConfigID == 0 {
		return nil, errors.New("SSDP announcer requires publisher and public identity")
	}
	probe := template
	probe.Interfaces = []string{"validate"}
	if _, err := NewSSDPPresence(probe); err != nil {
		return nil, err
	}
	template.Interfaces = nil
	return &SSDPAnnouncer{publisher: publisher, template: template}, nil
}

func (announcer *SSDPAnnouncer) Announce(ctx context.Context, interfaces []string) error {
	announcer.mu.Lock()
	defer announcer.mu.Unlock()
	wanted := append([]string(nil), interfaces...)
	sort.Strings(wanted)
	if slices.Equal(wanted, announcer.current) {
		checker, available := announcer.publisher.(ssdpPublicationChecker)
		if !available {
			return nil
		}
		current, err := checker.SSDPPublicationCurrent(ctx)
		if err != nil {
			return err
		}
		if current {
			return nil
		}
	}
	if len(wanted) == 0 {
		if len(announcer.current) == 0 {
			return nil
		}
		if err := announcer.publisher.WithdrawSSDP(ctx); err != nil {
			return err
		}
		announcer.current = nil
		return nil
	}
	presence := announcer.template
	presence.Interfaces = wanted
	validated, err := NewSSDPPresence(presence)
	if err != nil {
		return err
	}
	if err := announcer.publisher.PublishSSDP(ctx, validated); err != nil {
		return err
	}
	announcer.current = wanted
	return nil
}

func (announcer *SSDPAnnouncer) Invalidate() {
	announcer.mu.Lock()
	announcer.current = nil
	announcer.mu.Unlock()
}

func (announcer *SSDPAnnouncer) Withdraw(ctx context.Context) error {
	announcer.mu.Lock()
	defer announcer.mu.Unlock()
	if err := announcer.publisher.WithdrawSSDP(ctx); err != nil {
		return err
	}
	announcer.current = nil
	return nil
}

// NewSSDPPresence validates and canonicalizes one desired publication.
func NewSSDPPresence(value SSDPPresence) (SSDPPresence, error) {
	parsed, err := uuid.Parse(value.ApplianceID)
	if err != nil || parsed.String() != value.ApplianceID {
		return SSDPPresence{}, errors.New("canonical appliance UUID is required")
	}
	if !publicVersion.MatchString(value.Version) || value.Port == 0 || value.BootID == 0 || value.ConfigID == 0 {
		return SSDPPresence{}, errors.New("valid SSDP version, port and generation identifiers are required")
	}
	if len(value.Interfaces) == 0 {
		return SSDPPresence{}, errors.New("at least one management interface is required")
	}
	seen := make(map[string]bool, len(value.Interfaces))
	for _, name := range value.Interfaces {
		if name == "lo" || !managementInterfaceName.MatchString(name) || seen[name] {
			return SSDPPresence{}, fmt.Errorf("invalid SSDP interface %q", name)
		}
		seen[name] = true
	}
	value.Interfaces = append([]string(nil), value.Interfaces...)
	sort.Strings(value.Interfaces)
	return value, nil
}

// Alive renders one interface-address-specific ssdp:alive notification.
func (presence SSDPPresence) Alive(address string) (string, error) {
	location, err := presence.location(address)
	if err != nil {
		return "", err
	}
	return "NOTIFY * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"CACHE-CONTROL: max-age=120\r\n" +
		"LOCATION: " + location + "\r\n" +
		"NT: upnp:rootdevice\r\n" +
		"NTS: ssdp:alive\r\n" + presence.commonHeaders() + "\r\n", nil
}

// SearchResponse renders the unicast answer for an accepted root-device or
// ssdp:all search.
func (presence SSDPPresence) SearchResponse(address string) (string, error) {
	location, err := presence.location(address)
	if err != nil {
		return "", err
	}
	return "HTTP/1.1 200 OK\r\n" +
		"CACHE-CONTROL: max-age=120\r\n" +
		"EXT:\r\n" +
		"LOCATION: " + location + "\r\n" +
		"ST: upnp:rootdevice\r\n" + presence.commonHeaders() + "\r\n", nil
}

// Byebye renders a withdrawal without a stale location.
func (presence SSDPPresence) Byebye() string {
	return "NOTIFY * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"NT: upnp:rootdevice\r\n" +
		"NTS: ssdp:byebye\r\n" + presence.commonHeaders() + "\r\n"
}

func (presence SSDPPresence) commonHeaders() string {
	return "USN: uuid:" + presence.ApplianceID + "::upnp:rootdevice\r\n" +
		fmt.Sprintf("BOOTID.UPNP.ORG: %d\r\nCONFIGID.UPNP.ORG: %d\r\n", presence.BootID, presence.ConfigID) +
		"SERVER: NILO/" + presence.Version + " UPnP/1.1\r\n"
}

func (presence SSDPPresence) location(address string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(address))
	if ip == nil || ip.To4() == nil {
		return "", errors.New("an IPv4 interface address is required")
	}
	return fmt.Sprintf("https://%s:%d/.well-known/nilo-device.xml", ip.String(), presence.Port), nil
}
