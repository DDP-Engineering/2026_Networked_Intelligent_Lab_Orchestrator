package httpapi

import (
	"errors"
	"net/http"
	"time"

	"nilo.local/nilo/internal/network"
)

func handleWireGuardStatus(service *network.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := service.WireGuard(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "network_unavailable", "")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			APIVersion string `json:"api_version"`
			network.WireGuardStatus
		}{apiVersion, status})
	}
}

// networkStatusResponse is the wire shape of one network reading.
//
// measured_by is not decoration: it is the field that stops a simulated figure
// being read as a bench observation, so it is carried on the envelope rather
// than inferred by the caller from the runtime profile.
type networkStatusResponse struct {
	APIVersion string                    `json:"api_version"`
	MeasuredBy network.Source            `json:"measured_by"`
	Interfaces []networkInterfacePayload `json:"interfaces"`
	// UnattachedSettings are saved configurations whose adapter is absent:
	// still the operator's decision, reported as not in force.
	UnattachedSettings []unattachedSettingsPayload `json:"unattached_settings"`
}

// settingsStatePayload separates the saved intent from the applied state.
type settingsStatePayload struct {
	Saved               bool   `json:"saved"`
	Applied             bool   `json:"applied"`
	Revision            string `json:"revision"`
	Method              string `json:"method"`
	ManagementProtected bool   `json:"management_protected"`
	NeverDefault        bool   `json:"never_default"`
	SSID                string `json:"ssid,omitempty"`
}

type unattachedSettingsPayload struct {
	AdapterID string `json:"adapter_id"`
	Method    string `json:"method"`
	Address   string `json:"address,omitempty"`
	SSID      string `json:"ssid,omitempty"`
	Revision  string `json:"revision"`
}

type networkInterfacePayload struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	AdapterID string        `json:"adapter_id"`
	Kind      network.Kind  `json:"kind"`
	State     network.State `json:"state"`
	SSID      string        `json:"ssid,omitempty"`

	IPv4     []string `json:"ipv4"`
	IPv6     []string `json:"ipv6"`
	Gateway4 string   `json:"gateway4"`
	Gateway6 string   `json:"gateway6"`

	Settings *settingsStatePayload `json:"settings,omitempty"`

	LinkCapacityMbps uint64  `json:"link_capacity_mbps"`
	RXBytesPerSecond float64 `json:"rx_bytes_per_second"`
	TXBytesPerSecond float64 `json:"tx_bytes_per_second"`

	Controllable          bool   `json:"controllable"`
	NotControllableReason string `json:"not_controllable_reason"`
}

// handleNetworkStatus reads every interface the configured provider can see.
func handleNetworkStatus(service *network.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		status, err := service.Status(request.Context())
		if err != nil {
			// The reading failed, so there is nothing honest to report. An empty
			// interface list would be indistinguishable from an appliance that
			// genuinely has none.
			writeError(response, http.StatusServiceUnavailable, "network_unavailable", "")
			return
		}

		payload := networkStatusResponse{
			APIVersion:         apiVersion,
			MeasuredBy:         status.MeasuredBy,
			Interfaces:         make([]networkInterfacePayload, 0, len(status.Interfaces)),
			UnattachedSettings: make([]unattachedSettingsPayload, 0, len(status.Unattached)),
		}
		for _, settings := range status.Unattached {
			entry := unattachedSettingsPayload{AdapterID: settings.AdapterID, Method: string(settings.Method), Revision: settings.Revision}
			if settings.Address.IsValid() {
				entry.Address = settings.Address.String()
			}
			if settings.WiFi != nil {
				entry.SSID = settings.WiFi.SSID
			}
			payload.UnattachedSettings = append(payload.UnattachedSettings, entry)
		}
		for _, reading := range status.Interfaces {
			var settings *settingsStatePayload
			if reading.Settings != nil {
				settings = &settingsStatePayload{
					Saved: reading.Settings.Saved, Applied: reading.Settings.Applied, Revision: reading.Settings.Revision,
					Method: string(reading.Settings.Method), ManagementProtected: reading.Settings.ManagementProtected,
					NeverDefault: reading.Settings.NeverDefault, SSID: reading.Settings.SSID,
				}
			}
			payload.Interfaces = append(payload.Interfaces, networkInterfacePayload{
				ID: reading.ID, Name: reading.Name, AdapterID: reading.AdapterID, Kind: reading.Kind, State: reading.State,
				SSID: reading.SSID, Settings: settings,
				// An absent address list is an empty array rather than null, so a
				// consumer never has to distinguish "no addresses" from "no field".
				IPv4: emptyWhenNil(reading.IPv4), IPv6: emptyWhenNil(reading.IPv6),
				Gateway4: reading.Gateway4, Gateway6: reading.Gateway6,
				LinkCapacityMbps:      reading.LinkCapacityMbps,
				RXBytesPerSecond:      reading.RXBytesPerSecond,
				TXBytesPerSecond:      reading.TXBytesPerSecond,
				Controllable:          reading.Controllable,
				NotControllableReason: reading.NotControllableReason,
			})
		}
		writeJSON(response, http.StatusOK, payload)
	}
}

// networkStatisticsResponse is one bounded traffic series.
//
// stale is carried on the envelope and on every point, because a chart that
// stops refreshing must not keep looking live: the caller has to be able to say
// "this is old" without comparing timestamps itself.
type networkStatisticsResponse struct {
	APIVersion  string                   `json:"api_version"`
	InterfaceID string                   `json:"interface_id"`
	Period      network.Period           `json:"period"`
	From        time.Time                `json:"from"`
	To          time.Time                `json:"to"`
	MeasuredBy  network.Source           `json:"measured_by"`
	Stale       bool                     `json:"stale"`
	Points      []networkStatisticsPoint `json:"points"`
}

type networkStatisticsPoint struct {
	MinuteStart time.Time `json:"minute_start"`

	RXBytes uint64 `json:"rx_bytes"`
	TXBytes uint64 `json:"tx_bytes"`

	RXErrors  uint64 `json:"rx_errors"`
	TXErrors  uint64 `json:"tx_errors"`
	RXDropped uint64 `json:"rx_dropped"`
	TXDropped uint64 `json:"tx_dropped"`

	Samples int  `json:"samples"`
	Stale   bool `json:"stale"`
}

// handleNetworkStatistics returns one interface's traffic over one of the three
// offered windows. A minute that was never measured is simply absent, so a gap
// stays a gap rather than becoming a zero.
func handleNetworkStatistics(service *network.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		interfaceID := request.URL.Query().Get("interface")
		if interfaceID == "" {
			writeError(response, http.StatusBadRequest, "network_interface_required", "")
			return
		}
		period, err := network.ParsePeriod(request.URL.Query().Get("period"))
		if err != nil {
			writeError(response, http.StatusBadRequest, "network_period_unsupported", "")
			return
		}

		statistics, err := service.Statistics(request.Context(), interfaceID, period)
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "network_unavailable", "")
			return
		}

		payload := networkStatisticsResponse{
			APIVersion: apiVersion, InterfaceID: statistics.InterfaceID,
			Period: statistics.Period, From: statistics.From, To: statistics.To,
			MeasuredBy: statistics.MeasuredBy, Stale: statistics.Stale,
			Points: make([]networkStatisticsPoint, 0, len(statistics.Points)),
		}
		for _, point := range statistics.Points {
			payload.Points = append(payload.Points, networkStatisticsPoint{
				MinuteStart: point.MinuteStart,
				RXBytes:     point.RXBytes, TXBytes: point.TXBytes,
				RXErrors: point.RXErrors, TXErrors: point.TXErrors,
				RXDropped: point.RXDropped, TXDropped: point.TXDropped,
				Samples: point.Samples, Stale: point.Stale,
			})
		}
		writeJSON(response, http.StatusOK, payload)
	}
}

// wifiScanResponse is one adapter's scan: names, security and strength, and
// whether each network is one this appliance can join.
type wifiScanResponse struct {
	APIVersion  string                `json:"api_version"`
	InterfaceID string                `json:"interface_id"`
	Supported   bool                  `json:"supported"`
	ObservedAt  time.Time             `json:"observed_at"`
	Networks    []network.WiFiNetwork `json:"networks"`
}

// handleWiFiScan scans on one wireless adapter.
func handleWiFiScan(service *network.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		result, err := service.WiFiScan(request.Context(), request.PathValue("id"))
		if err != nil {
			switch {
			case errors.Is(err, network.ErrUnsupported):
				writeRefusal(response, http.StatusConflict, "network_unsupported", err.Error())
			case errors.Is(err, network.ErrNotControllable):
				writeError(response, http.StatusConflict, "network_not_controllable", "")
			default:
				writeError(response, http.StatusServiceUnavailable, "network_unavailable", "")
			}
			return
		}
		if result.Networks == nil {
			result.Networks = []network.WiFiNetwork{}
		}
		writeJSON(response, http.StatusOK, wifiScanResponse{
			APIVersion: apiVersion, InterfaceID: result.InterfaceID, Supported: result.Supported,
			ObservedAt: result.ObservedAt, Networks: result.Networks,
		})
	}
}

func emptyWhenNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
