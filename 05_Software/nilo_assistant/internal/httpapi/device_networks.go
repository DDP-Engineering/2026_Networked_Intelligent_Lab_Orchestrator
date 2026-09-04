package httpapi

import (
	"errors"
	"net/http"
	"net/netip"
	"time"

	"nilo.local/nilo/internal/network"
)

// The device-network routes. Reads answer from the shared service; every
// change goes through the preview and acknowledgement lifecycle like any other
// network change, so nothing here mutates directly.

// deviceNetworkPayload is the wire shape of one saved device network with its
// observed state. Intent (enabled) and applied state are separate fields.
type deviceNetworkPayload struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Location         string   `json:"location"`
	DeviceInterface  string   `json:"device_interface"`
	ApplianceAddress string   `json:"appliance_address"`
	DeviceSubnet     string   `json:"device_subnet"`
	Scope            string   `json:"scope"`
	Target           string   `json:"target,omitempty"`
	Addressing       string   `json:"addressing"`
	Exposed          string   `json:"exposed"`
	Access           string   `json:"access"`
	LocalInterface   string   `json:"local_interface,omitempty"`
	LocalSources     []string `json:"local_sources"`
	VPNProfileID     string   `json:"vpn_profile_id,omitempty"`
	VPNSources       []string `json:"vpn_sources"`
	Services         struct {
		Protocols      []string `json:"protocols"`
		TCPPorts       []int    `json:"tcp_ports"`
		UDPPorts       []int    `json:"udp_ports"`
		PreserveSource bool     `json:"preserve_source"`
	} `json:"services"`
	Enabled    bool      `json:"enabled"`
	Revision   string    `json:"revision"`
	State      string    `json:"state"`
	Detail     string    `json:"detail,omitempty"`
	Applied    bool      `json:"applied"`
	ObservedAt time.Time `json:"observed_at"`
}

func prefixTexts(prefixes []netip.Prefix) []string {
	texts := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		texts = append(texts, prefix.String())
	}
	return texts
}

func emptyInts(values []int) []int {
	if values == nil {
		return []int{}
	}
	return values
}

func deviceNetworkPayloadOf(status network.DeviceNetworkStatus) deviceNetworkPayload {
	payload := deviceNetworkPayload{
		ID: status.ID, Name: status.Name, Location: status.Location, DeviceInterface: status.DeviceInterface,
		ApplianceAddress: status.ApplianceAddress.String(), DeviceSubnet: status.DeviceSubnet().String(),
		Scope: string(status.Scope), Addressing: string(status.Addressing), Access: string(status.Access),
		LocalInterface: status.LocalInterface, LocalSources: prefixTexts(status.LocalSources),
		VPNProfileID: status.VPNProfileID, VPNSources: prefixTexts(status.VPNSources),
		Enabled: status.StartEnabled, Revision: status.Revision, State: string(status.State), Detail: status.Detail,
		Applied: status.Applied, ObservedAt: status.ObservedAt,
	}
	if status.Target.IsValid() {
		payload.Target = status.Target.String()
	}
	// Exposed is what a remote client addresses, whichever way it is reached.
	if status.Addressing == network.AddressingTranslated {
		payload.Exposed = status.Exposed.String()
	} else if status.Scope == network.ScopeHost {
		payload.Exposed = status.Target.String()
	} else {
		payload.Exposed = status.DeviceSubnet().String()
	}
	payload.Services.Protocols = status.Services.EffectiveProtocols()
	payload.Services.TCPPorts = emptyInts(status.Services.TCPPorts)
	payload.Services.UDPPorts = emptyInts(status.Services.UDPPorts)
	payload.Services.PreserveSource = status.Services.PreserveSource
	return payload
}

// deviceNetworkRequest is what a caller submits for a create or an update.
type deviceNetworkRequest struct {
	Name             string   `json:"name"`
	Location         string   `json:"location"`
	DeviceInterface  string   `json:"device_interface"`
	ApplianceAddress string   `json:"appliance_address"`
	Scope            string   `json:"scope"`
	Target           string   `json:"target"`
	Addressing       string   `json:"addressing"`
	Exposed          string   `json:"exposed"`
	Access           string   `json:"access"`
	LocalInterface   string   `json:"local_interface"`
	LocalSources     []string `json:"local_sources"`
	VPNProfileID     string   `json:"vpn_profile_id"`
	VPNSources       []string `json:"vpn_sources"`
	Services         *struct {
		Protocols      []string `json:"protocols"`
		TCPPorts       []int    `json:"tcp_ports"`
		UDPPorts       []int    `json:"udp_ports"`
		PreserveSource bool     `json:"preserve_source"`
	} `json:"services"`
}

// toDeviceNetwork parses the request. An address that does not parse is a
// rejection naming the field, so the operator learns which one.
func (request deviceNetworkRequest) toDeviceNetwork() (network.DeviceNetwork, error) {
	proposed := network.DeviceNetwork{
		Name: request.Name, Location: request.Location, DeviceInterface: request.DeviceInterface,
		Scope: network.Scope(request.Scope), Addressing: network.Addressing(request.Addressing),
		Access: network.AccessMethod(request.Access), LocalInterface: request.LocalInterface, VPNProfileID: request.VPNProfileID,
	}
	if request.Addressing == "" {
		proposed.Addressing = network.AddressingOriginal
	}
	if request.Scope == "" {
		proposed.Scope = network.ScopeHost
	}
	var err error
	if request.ApplianceAddress != "" {
		if proposed.ApplianceAddress, err = netip.ParsePrefix(request.ApplianceAddress); err != nil {
			return network.DeviceNetwork{}, rejectField("appliance_address", "must be an IPv4 address with its prefix length")
		}
	}
	if request.Target != "" {
		if proposed.Target, err = netip.ParseAddr(request.Target); err != nil {
			return network.DeviceNetwork{}, rejectField("target", "must be an IPv4 address")
		}
	}
	if request.Exposed != "" {
		if proposed.Exposed, err = netip.ParsePrefix(request.Exposed); err != nil {
			if address, addressErr := netip.ParseAddr(request.Exposed); addressErr == nil {
				proposed.Exposed = netip.PrefixFrom(address, 32)
			} else {
				return network.DeviceNetwork{}, rejectField("exposed", "must be an IPv4 address or prefix")
			}
		}
	}
	for _, text := range request.LocalSources {
		prefix, err := netip.ParsePrefix(text)
		if err != nil {
			return network.DeviceNetwork{}, rejectField("local_sources", "must be IPv4 prefixes")
		}
		proposed.LocalSources = append(proposed.LocalSources, prefix)
	}
	for _, text := range request.VPNSources {
		prefix, err := netip.ParsePrefix(text)
		if err != nil {
			return network.DeviceNetwork{}, rejectField("vpn_sources", "must be IPv4 prefixes")
		}
		proposed.VPNSources = append(proposed.VPNSources, prefix)
	}
	if request.Services != nil {
		proposed.Services = network.ServicePolicy{
			Protocols: request.Services.Protocols, TCPPorts: request.Services.TCPPorts,
			UDPPorts: request.Services.UDPPorts, PreserveSource: request.Services.PreserveSource,
		}
	}
	return proposed, nil
}

func rejectField(field, reason string) error {
	return &fieldRejection{field: field, reason: reason}
}

type fieldRejection struct{ field, reason string }

func (rejection *fieldRejection) Error() string { return rejection.field + " " + rejection.reason }

// deviceNetworkActions are the actions the preview route offers.
var deviceNetworkActions = map[string]network.Action{
	"create":  network.ActionDeviceNetworkCreate,
	"update":  network.ActionDeviceNetworkUpdate,
	"enable":  network.ActionDeviceNetworkEnable,
	"disable": network.ActionDeviceNetworkDisable,
	"delete":  network.ActionDeviceNetworkDelete,
}

// handlePreviewDeviceNetworkChange proposes one device-network change.
func handlePreviewDeviceNetworkChange(manager *network.OperationManager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			Action        string                `json:"action"`
			Target        string                `json:"target"`
			DeviceNetwork *deviceNetworkRequest `json:"device_network"`
		}
		if err := decodeOperationRequest(request, &body); err != nil {
			writeError(response, http.StatusBadRequest, "network_request_invalid", "")
			return
		}
		action, offered := deviceNetworkActions[body.Action]
		if !offered {
			writeError(response, http.StatusBadRequest, "network_action_unsupported", "")
			return
		}
		change := network.Change{Action: action, Target: body.Target}
		if action == network.ActionDeviceNetworkCreate || action == network.ActionDeviceNetworkUpdate {
			if body.DeviceNetwork == nil {
				writeRefusal(response, http.StatusBadRequest, "device_network_rejected", "a create or update needs the device network")
				return
			}
			proposed, err := body.DeviceNetwork.toDeviceNetwork()
			if err != nil {
				writeRefusal(response, http.StatusBadRequest, "device_network_rejected", err.Error())
				return
			}
			change.DeviceNetwork = &proposed
			if action == network.ActionDeviceNetworkCreate {
				change.Target = "new"
			}
		}
		previewChange(response, request, manager, change)
	}
}

type deviceNetworkListResponse struct {
	APIVersion string                 `json:"api_version"`
	Items      []deviceNetworkPayload `json:"items"`
}

func writeDeviceNetworkFailure(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, network.ErrDeviceNetworkNotFound):
		writeError(response, http.StatusNotFound, "device_network_not_found", "")
	case errors.Is(err, network.ErrDeviceNetworkRejected):
		writeRefusal(response, http.StatusBadRequest, "device_network_rejected", err.Error())
	case errors.Is(err, network.ErrUnsupported):
		writeRefusal(response, http.StatusConflict, "network_unsupported", err.Error())
	default:
		writeError(response, http.StatusServiceUnavailable, "network_unavailable", "")
	}
}

func handleListDeviceNetworks(service *network.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		statuses, err := service.DeviceNetworks(request.Context())
		if err != nil {
			writeDeviceNetworkFailure(response, err)
			return
		}
		items := make([]deviceNetworkPayload, 0, len(statuses))
		for _, status := range statuses {
			items = append(items, deviceNetworkPayloadOf(status))
		}
		writeJSON(response, http.StatusOK, deviceNetworkListResponse{APIVersion: apiVersion, Items: items})
	}
}

func handleReadDeviceNetwork(service *network.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		status, err := service.DeviceNetwork(request.Context(), request.PathValue("id"))
		if err != nil {
			writeDeviceNetworkFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, struct {
			APIVersion string `json:"api_version"`
			deviceNetworkPayload
		}{apiVersion, deviceNetworkPayloadOf(status)})
	}
}

// handleCheckDeviceNetwork runs the bounded check against the configured
// target only: the body may name a TCP port and nothing else.
func handleCheckDeviceNetwork(service *network.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			TCPPort int `json:"tcp_port"`
		}
		if err := decodeOperationRequest(request, &body); err != nil {
			writeError(response, http.StatusBadRequest, "network_request_invalid", "")
			return
		}
		if body.TCPPort < 0 || body.TCPPort > 65535 {
			writeRefusal(response, http.StatusBadRequest, "device_network_rejected", "tcp_port must be between 1 and 65535")
			return
		}
		check, err := service.CheckDeviceNetwork(request.Context(), request.PathValue("id"), network.DeviceNetworkProbe{TCPPort: body.TCPPort})
		if err != nil {
			writeDeviceNetworkFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, struct {
			APIVersion      string                      `json:"api_version"`
			DeviceNetworkID string                      `json:"device_network_id"`
			ObservedAt      time.Time                   `json:"observed_at"`
			Facts           []network.DeviceNetworkFact `json:"facts"`
		}{apiVersion, check.DeviceNetworkID, check.ObservedAt, check.Facts})
	}
}

func handleAccessInstructions(service *network.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		set, err := service.AccessInstructions(request.Context(), request.PathValue("id"))
		if err != nil {
			writeDeviceNetworkFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, struct {
			APIVersion         string                    `json:"api_version"`
			DeviceNetworkID    string                    `json:"device_network_id"`
			TargetAddress      string                    `json:"target_address"`
			Steps              []network.InstructionStep `json:"steps"`
			UnmetPrerequisites []string                  `json:"unmet_prerequisites"`
		}{apiVersion, set.DeviceNetworkID, set.TargetAddress, set.Steps, set.UnmetPrerequisites})
	}
}
