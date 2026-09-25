package httpapi

import (
	"encoding/xml"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
)

var discoveryLocalLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// DiscoveryDescription contains only public stable metadata used by an SSDP
// control point to present and open the appliance.
type DiscoveryDescription struct {
	ApplianceID       string
	SerialName        string
	FriendlyName      string
	PresentationURL   string
	HardwareAddresses []string
}

type upnpRoot struct {
	XMLName xml.Name   `xml:"root"`
	XMLNS   string     `xml:"xmlns,attr"`
	Spec    upnpSpec   `xml:"specVersion"`
	Device  upnpDevice `xml:"device"`
}

type upnpSpec struct {
	Major int `xml:"major"`
	Minor int `xml:"minor"`
}

type upnpDevice struct {
	DeviceType        string   `xml:"deviceType"`
	FriendlyName      string   `xml:"friendlyName"`
	Manufacturer      string   `xml:"manufacturer"`
	ModelName         string   `xml:"modelName"`
	SerialNumber      string   `xml:"serialNumber"`
	UDN               string   `xml:"UDN"`
	PresentationURL   string   `xml:"presentationURL"`
	HardwareAddresses []string `xml:"X_NILO_HardwareAddress,omitempty"`
}

// NewDiscoveryDescriptionHandler validates immutable metadata before creating
// a GET-only XML handler with no UPnP control or event surface.
func NewDiscoveryDescriptionHandler(description DiscoveryDescription) (http.Handler, error) {
	parsedID, err := uuid.Parse(description.ApplianceID)
	if err != nil || parsedID.String() != description.ApplianceID {
		return nil, errors.New("canonical appliance UUID is required")
	}
	if !validDiscoverySerial(description.SerialName) {
		return nil, errors.New("valid permanent appliance name is required")
	}
	if description.FriendlyName != "" && !discoveryLocalLabel.MatchString(description.FriendlyName) {
		return nil, errors.New("valid friendly appliance name is required")
	}
	presentation, err := url.Parse(description.PresentationURL)
	if err != nil || presentation.Scheme != "https" || presentation.Host == "" || presentation.User != nil || presentation.RawQuery != "" || presentation.Fragment != "" {
		return nil, errors.New("safe HTTPS presentation URL is required")
	}
	name := description.SerialName
	if description.FriendlyName != "" {
		name = description.FriendlyName
	}
	hardwareAddresses := make([]string, 0, len(description.HardwareAddresses))
	seenHardware := map[string]bool{}
	for _, value := range description.HardwareAddresses {
		address, err := net.ParseMAC(value)
		if err != nil || len(address) != 6 || seenHardware[address.String()] {
			return nil, errors.New("valid unique hardware addresses are required")
		}
		seenHardware[address.String()] = true
		hardwareAddresses = append(hardwareAddresses, address.String())
	}
	sort.Strings(hardwareAddresses)
	document, err := xml.Marshal(upnpRoot{
		XMLNS: "urn:schemas-upnp-org:device-1-0",
		Spec:  upnpSpec{Major: 1, Minor: 1},
		Device: upnpDevice{
			DeviceType:        "urn:schemas-upnp-org:device:Basic:1",
			FriendlyName:      name,
			Manufacturer:      "DDP Engineering",
			ModelName:         "NILO Networked Intelligent Lab Orchestrator",
			SerialNumber:      description.SerialName,
			UDN:               "uuid:" + description.ApplianceID,
			PresentationURL:   description.PresentationURL,
			HardwareAddresses: hardwareAddresses,
		},
	})
	if err != nil {
		return nil, err
	}
	document = append([]byte(xml.Header), document...)
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		response.Header().Set("Content-Type", "application/xml; charset=utf-8")
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(document)
	}), nil
}

func validDiscoverySerial(value string) bool {
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
