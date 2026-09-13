package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// A saved interface configuration is the operator's intent for one adapter:
// how it gets its address, which address, gateway and resolvers, whether it is
// a management path, and for a wireless adapter which network it joins. It is
// bound to the adapter's permanent hardware identity, so a kernel rename keeps
// it and a replaced adapter does not silently inherit it.

// AddressMethod is how an adapter gets its IPv4 configuration.
type AddressMethod string

const (
	MethodAutomatic AddressMethod = "automatic"
	MethodStatic    AddressMethod = "static"
	// MethodLinkLocal claims an IPv4 link-local address through the standard
	// ARP probe. It is what makes direct Ethernet commissioning possible with
	// no DHCP server and no address to guess: each adapter probes its way to a
	// different 169.254 address, so two of them cannot collide on one route the
	// way a fixed conventional address would.
	MethodLinkLocal AddressMethod = "link-local"
)

// WiFiSecurity is the network security an adapter is asked to join with. The
// set is the personal security this milestone supports; enterprise onboarding
// is out of scope and refused by name.
type WiFiSecurity string

const (
	WiFiOpen         WiFiSecurity = "open"
	WiFiWPA2Personal WiFiSecurity = "wpa2-personal"
	WiFiWPA3Personal WiFiSecurity = "wpa3-personal"
)

var knownWiFiSecurity = map[WiFiSecurity]bool{WiFiOpen: true, WiFiWPA2Personal: true, WiFiWPA3Personal: true}

// WiFiSettings is the saved wireless intent. It carries a reference to the
// password, never the password: the value lives in root-owned storage and this
// record cannot be used to read it back.
type WiFiSettings struct {
	SSID             string
	Security         WiFiSecurity
	ConnectAtStartup bool
	SecretRef        string
}

// InterfaceSettings is one saved adapter configuration.
type InterfaceSettings struct {
	// AdapterID is the permanent hardware identity the configuration follows.
	AdapterID string
	Method    AddressMethod
	// Address and Gateway apply to a static configuration only.
	Address netip.Prefix
	Gateway netip.Addr
	// DNS lists resolvers; DNSAutomatic takes them from the network instead.
	DNS          []netip.Addr
	DNSAutomatic bool
	// ManagementProtected marks the adapter as a management path the operator
	// wants kept, whichever way a request arrives.
	ManagementProtected bool
	// NeverDefault keeps the adapter from taking the default route, which is
	// what a device-side adapter needs.
	NeverDefault bool
	WiFi         *WiFiSettings
	// ConnectionRef is the NetworkManager connection NILO owns for this
	// adapter, empty until one exists.
	ConnectionRef string

	Revision  string
	UpdatedAt time.Time
}

const (
	maximumDNSServers = 4
	maximumSSIDBytes  = 32
)

var (
	// ErrInterfaceSettingsRejected is every refusal of a proposed configuration.
	ErrInterfaceSettingsRejected = errors.New("interface settings rejected")
	// ErrInterfaceSettingsNotFound is an adapter nothing is saved for.
	ErrInterfaceSettingsNotFound = errors.New("interface_settings_not_found")
	// secretReference is the shape of an opaque secret reference. A value with
	// a space or punctuation outside this set is a passphrase somebody typed,
	// not a reference this appliance issued.
	secretReference = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,128}$`)
)

func rejectSettings(format string, arguments ...any) error {
	return fmt.Errorf("%w: %s", ErrInterfaceSettingsRejected, fmt.Sprintf(format, arguments...))
}

// Validate checks everything that can be decided from the configuration alone.
func (settings InterfaceSettings) Validate() error {
	if strings.TrimSpace(settings.AdapterID) == "" {
		return rejectSettings("a saved configuration needs the adapter identity it belongs to")
	}
	switch settings.Method {
	case MethodStatic:
		if err := settings.validateStatic(); err != nil {
			return err
		}
	case MethodAutomatic:
		if settings.Address.IsValid() || settings.Gateway.IsValid() {
			return rejectSettings("an automatic configuration takes no static address or gateway")
		}
	case MethodLinkLocal:
		if settings.Address.IsValid() || settings.Gateway.IsValid() {
			return rejectSettings("a link-local configuration takes no static address or gateway")
		}
		// A link-local address reaches only the segment the cable is on, which
		// is the wired commissioning case it exists for. Offering it on a
		// wireless adapter would promise a joined network that cannot route.
		if settings.WiFi != nil {
			return rejectSettings("link-local addressing applies to a wired adapter")
		}
	default:
		return rejectSettings("unknown address method %q", settings.Method)
	}
	if settings.DNSAutomatic && len(settings.DNS) > 0 {
		return rejectSettings("DNS servers are either taken from the network or listed, not both")
	}
	if len(settings.DNS) > maximumDNSServers {
		return rejectSettings("at most %d DNS servers are supported", maximumDNSServers)
	}
	for _, server := range settings.DNS {
		if !server.IsValid() || server.IsUnspecified() {
			return rejectSettings("%s is not a usable DNS server", server)
		}
	}
	if settings.WiFi != nil {
		return settings.WiFi.validate()
	}
	return nil
}

func (settings InterfaceSettings) validateStatic() error {
	address := settings.Address
	if !address.IsValid() {
		return rejectSettings("a static configuration needs the address and subnet mask")
	}
	if !address.Addr().Is4() {
		return rejectSettings("%s is not an IPv4 address; interface configuration is IPv4 only in this milestone", address)
	}
	if address.Bits() < 8 || address.Bits() > 30 {
		return rejectSettings("%s is not an ordinary host subnet; use a mask between /8 and /30", address)
	}
	subnet := address.Masked()
	if address.Addr() == subnet.Addr() {
		return rejectSettings("%s is the subnet's network address, not a host address", address.Addr())
	}
	if address.Addr() == broadcastOf(subnet) {
		return rejectSettings("%s is the subnet's broadcast address, not a host address", address.Addr())
	}
	if settings.Gateway.IsValid() {
		if !settings.Gateway.Is4() || !subnet.Contains(settings.Gateway) || settings.Gateway == address.Addr() ||
			settings.Gateway == subnet.Addr() || settings.Gateway == broadcastOf(subnet) {
			return rejectSettings("the gateway %s is not another host inside %s", settings.Gateway, subnet)
		}
	}
	return nil
}

func (wifi WiFiSettings) validate() error {
	if wifi.SSID == "" || len(wifi.SSID) > maximumSSIDBytes || strings.ContainsAny(wifi.SSID, "\x00\r\n") {
		return rejectSettings("the Wi-Fi SSID must be between 1 and %d bytes", maximumSSIDBytes)
	}
	if !knownWiFiSecurity[wifi.Security] {
		return rejectSettings("unsupported Wi-Fi security %q; open, wpa2-personal and wpa3-personal are supported", wifi.Security)
	}
	switch {
	case wifi.Security == WiFiOpen && wifi.SecretRef != "":
		return rejectSettings("an open network takes no password")
	case wifi.Security != WiFiOpen && wifi.SecretRef == "":
		return rejectSettings("%s needs a password", wifi.Security)
	case wifi.SecretRef != "" && !secretReference.MatchString(wifi.SecretRef):
		return rejectSettings("the password reference is not a reference this appliance issued")
	}
	return nil
}

// ContentRevision digests the configuration's content fields, so two saves of
// the same intent share a revision and any edit changes it.
func (settings InterfaceSettings) ContentRevision() string {
	digest := sha256.New()
	fmt.Fprintf(digest, "%s\x00%s\x00%s\x00%s\x00%t\x00%t\x00%t\x00%s\n",
		strings.ToLower(settings.AdapterID), settings.Method, prefixText(settings.Address), addrText(settings.Gateway),
		settings.DNSAutomatic, settings.ManagementProtected, settings.NeverDefault, settings.ConnectionRef)
	for _, server := range settings.DNS {
		fmt.Fprintf(digest, "dns\x00%s\n", server)
	}
	if settings.WiFi != nil {
		fmt.Fprintf(digest, "wifi\x00%s\x00%s\x00%t\x00%s\n", settings.WiFi.SSID, settings.WiFi.Security, settings.WiFi.ConnectAtStartup, settings.WiFi.SecretRef)
	}
	return hex.EncodeToString(digest.Sum(nil))[:16]
}

func prefixText(prefix netip.Prefix) string {
	if !prefix.IsValid() {
		return ""
	}
	return prefix.String()
}

func addrText(address netip.Addr) string {
	if !address.IsValid() {
		return ""
	}
	return address.String()
}

// InterfaceSettingsStore is the saved adapter configuration inventory.
type InterfaceSettingsStore interface {
	// Save writes one configuration and returns it with its revision and
	// timestamp filled in. An unchanged save keeps the revision.
	Save(ctx context.Context, settings InterfaceSettings, at time.Time) (InterfaceSettings, error)
	List(ctx context.Context) ([]InterfaceSettings, error)
	Delete(ctx context.Context, adapterID string) error
}

// SettingsState is what a reader learns about the configuration bound to an
// adapter: that one is saved, whether it is currently applied, and which
// revision it is at. Intent and applied state are reported apart on purpose.
type SettingsState struct {
	Saved               bool
	Applied             bool
	Revision            string
	Method              AddressMethod
	ManagementProtected bool
	NeverDefault        bool
	// SSID is the saved wireless network, empty for a wired adapter.
	SSID string
}

// BoundInterfaces is the adapter list with saved configuration attached, plus
// every saved configuration whose adapter is not present.
type BoundInterfaces struct {
	Interfaces []Interface
	Unattached []InterfaceSettings
}

// BindSettings attaches saved configuration to the adapters it belongs to. A
// configuration whose adapter is absent is reported as unattached rather than
// applied or dropped: it is still the operator's decision, just not in force.
func BindSettings(interfaces []Interface, saved []InterfaceSettings) BoundInterfaces {
	bound := BoundInterfaces{Interfaces: make([]Interface, len(interfaces))}
	attached := map[string]bool{}
	for index, candidate := range interfaces {
		clone := candidate
		clone.Settings = nil
		for _, settings := range saved {
			if candidate.AdapterID == "" || !strings.EqualFold(candidate.AdapterID, settings.AdapterID) {
				continue
			}
			attached[strings.ToLower(settings.AdapterID)] = true
			state := &SettingsState{
				Saved: true, Revision: settings.Revision, Method: settings.Method,
				ManagementProtected: settings.ManagementProtected, NeverDefault: settings.NeverDefault,
				Applied: settingsApplied(candidate, settings),
			}
			if settings.WiFi != nil {
				state.SSID = settings.WiFi.SSID
			}
			clone.Settings = state
			break
		}
		bound.Interfaces[index] = clone
	}
	for _, settings := range saved {
		if !attached[strings.ToLower(settings.AdapterID)] {
			bound.Unattached = append(bound.Unattached, settings)
		}
	}
	return bound
}

// settingsApplied reports whether the adapter currently carries what is saved
// for it. Applied is an observation of the adapter, never a restatement of the
// intent.
func settingsApplied(candidate Interface, settings InterfaceSettings) bool {
	if candidate.State != StateConnected {
		return false
	}
	switch settings.Method {
	case MethodStatic:
		for _, address := range candidate.IPv4 {
			if address == settings.Address.String() {
				return true
			}
		}
		return false
	case MethodAutomatic:
		return len(candidate.IPv4) > 0
	default:
		return false
	}
}

// findSettings returns the saved configuration for one adapter identity.
func findSettings(saved []InterfaceSettings, adapterID string) (InterfaceSettings, bool) {
	for _, settings := range saved {
		if adapterID != "" && strings.EqualFold(settings.AdapterID, adapterID) {
			return settings, true
		}
	}
	return InterfaceSettings{}, false
}
