package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/network"
)

// ApplianceMetadata is the public projection shared by About, labels and
// discovery. It deliberately has no credential, key or raw hardware source.
type ApplianceMetadata struct {
	mu                  sync.RWMutex
	ID                  string   `json:"id"`
	SerialName          string   `json:"serial_name"`
	FriendlyName        string   `json:"friendly_name,omitempty"`
	State               string   `json:"state"`
	Version             string   `json:"version"`
	ManagementURL       string   `json:"management_url"`
	ManagementAddresses []string `json:"management_addresses"`
	ManagementMACs      []string `json:"management_macs"`
}

type applianceMetadataSnapshot struct {
	ID                  string   `json:"id"`
	SerialName          string   `json:"serial_name"`
	FriendlyName        string   `json:"friendly_name,omitempty"`
	State               string   `json:"state"`
	Version             string   `json:"version"`
	ManagementURL       string   `json:"management_url"`
	ManagementAddresses []string `json:"management_addresses"`
	ManagementMACs      []string `json:"management_macs"`
}

func (metadata *ApplianceMetadata) snapshot() applianceMetadataSnapshot {
	metadata.mu.RLock()
	defer metadata.mu.RUnlock()
	return applianceMetadataSnapshot{
		ID: metadata.ID, SerialName: metadata.SerialName, FriendlyName: metadata.FriendlyName,
		State: metadata.State, Version: metadata.Version, ManagementURL: metadata.ManagementURL,
		ManagementAddresses: append([]string(nil), metadata.ManagementAddresses...),
		ManagementMACs:      append([]string(nil), metadata.ManagementMACs...),
	}
}

func (metadata *ApplianceMetadata) replaceFriendlyName(name string) {
	metadata.mu.Lock()
	defer metadata.mu.Unlock()
	metadata.FriendlyName = name
}

// UpdateRuntime replaces the address and lifecycle facts that can change while
// the process remains running, without disturbing the permanent identity or a
// concurrently committed friendly name.
func (metadata *ApplianceMetadata) UpdateRuntime(state string, addresses, hardwareAddresses []string) {
	metadata.mu.Lock()
	defer metadata.mu.Unlock()
	metadata.State = state
	metadata.ManagementAddresses = append([]string(nil), addresses...)
	metadata.ManagementMACs = append([]string(nil), hardwareAddresses...)
}

type applianceIdentifyService interface {
	Identify(context.Context) (network.IdentifyResult, error)
}

type applianceNamingService interface {
	SetFriendlyName(context.Context, string) (appliance.Identity, error)
}

type identifyOperationStore interface {
	Begin(context.Context, string, string, time.Time) (bool, error)
	MarkActed(context.Context, string, bool, string, audit.Event, time.Time) error
	Flush(context.Context, string) error
}

type identifyAuditComposer interface {
	AuditJournal
	Compose(audit.Record) (audit.Event, error)
}

func handleApplianceMetadata(metadata *ApplianceMetadata) http.HandlerFunc {
	return func(response http.ResponseWriter, _ *http.Request) {
		snapshot := metadata.snapshot()
		writeJSON(response, http.StatusOK, struct {
			APIVersion string `json:"api_version"`
			*applianceMetadataSnapshot
		}{APIVersion: apiVersion, applianceMetadataSnapshot: &snapshot})
	}
}

func handleApplianceLabel(metadata *ApplianceMetadata) http.HandlerFunc {
	return func(response http.ResponseWriter, _ *http.Request) {
		snapshot := metadata.snapshot()
		label, err := appliance.RenderLabelSVG(appliance.Label{SerialName: snapshot.SerialName, HardwareAddresses: snapshot.ManagementMACs})
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "appliance_label_unavailable", "")
			return
		}
		response.Header().Set("Content-Type", "image/svg+xml")
		response.Header().Set("Content-Disposition", `inline; filename="nilo-label.svg"`)
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(label)
	}
}

func handleIdentify(service applianceIdentifyService, journal AuditJournal, operations identifyOperationStore) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if service == nil {
			writeError(response, http.StatusServiceUnavailable, "identify_unavailable", "")
			return
		}
		if request.Body != nil {
			data, err := io.ReadAll(io.LimitReader(request.Body, 2))
			if err != nil || len(data) != 0 {
				writeError(response, http.StatusBadRequest, "invalid_request", "")
				return
			}
		}
		actor, ok := actorFrom(request.Context())
		if !ok || journal == nil {
			writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
			return
		}
		record := audit.Record{
			Category: audit.CategoryAudit, Component: "appliance.identity", Action: "appliance.identify",
			SubjectType: "appliance", SubjectID: "local", Outcome: audit.OutcomeRequested,
			Severity: audit.SeverityInfo, Details: map[string]string{"source": "interface"},
		}
		record.ActorKind, record.ActorID = actorIdentity(actor)
		composer, durable := journal.(identifyAuditComposer)
		var operationID string
		if operations != nil && durable {
			generated, err := domain.NewRequestID()
			if err != nil {
				writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
				return
			}
			operationID = generated.String()
			if _, err := operations.Begin(request.Context(), operationID, "interface", time.Now().UTC()); err != nil {
				writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
				return
			}
		} else if err := journal.Record(request.Context(), record); err != nil {
			writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
			return
		}
		result, err := service.Identify(request.Context())
		if operationID != "" {
			completionContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			record.OperationID = operationID
			record.Outcome = audit.OutcomeApplied
			if err != nil {
				record.Outcome = audit.OutcomeFailed
			}
			record.Details = map[string]string{
				"source": "interface", "status": string(result.Status), "reannouncement": "not_requested",
			}
			event, composeErr := composer.Compose(record)
			if composeErr != nil || operations.MarkActed(completionContext, operationID, result.Status != network.IdentifyUnsupported, "not_requested", event, time.Now().UTC()) != nil || operations.Flush(completionContext, operationID) != nil {
				writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
				return
			}
		}
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "identify_unavailable", "")
			return
		}
		if result.Status == network.IdentifyUnsupported {
			writeJSON(response, http.StatusOK, struct {
				APIVersion string `json:"api_version"`
				Supported  bool   `json:"supported"`
				Reason     string `json:"reason"`
			}{APIVersion: apiVersion, Supported: false, Reason: "indicator_unavailable"})
			return
		}
		writeJSON(response, http.StatusAccepted, struct {
			APIVersion  string    `json:"api_version"`
			Supported   bool      `json:"supported"`
			ActiveUntil time.Time `json:"active_until"`
		}{APIVersion: apiVersion, Supported: true, ActiveUntil: result.ActiveUntil})
	}
}

func handleFriendlyName(service applianceNamingService, metadata *ApplianceMetadata) http.HandlerFunc {
	type requestBody struct {
		FriendlyName string `json:"friendly_name"`
	}
	return func(response http.ResponseWriter, request *http.Request) {
		if service == nil {
			writeError(response, http.StatusServiceUnavailable, "friendly_name_unavailable", "")
			return
		}
		var body requestBody
		if request.Method == http.MethodPut {
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1024))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&body); err != nil || decoder.Decode(new(any)) != io.EOF {
				writeError(response, http.StatusBadRequest, "invalid_request", "")
				return
			}
			if err := appliance.ValidateFriendlyName(body.FriendlyName); err != nil {
				writeError(response, http.StatusBadRequest, "invalid_friendly_name", "")
				return
			}
		}
		identity, err := service.SetFriendlyName(request.Context(), body.FriendlyName)
		if err != nil {
			switch {
			case errors.Is(err, appliance.ErrInvalidFriendlyName):
				writeError(response, http.StatusBadRequest, "invalid_friendly_name", "")
			case errors.Is(err, appliance.ErrFriendlyNameConflict):
				writeError(response, http.StatusConflict, "friendly_name_conflict", "")
			default:
				writeError(response, http.StatusServiceUnavailable, "friendly_name_unavailable", "")
			}
			return
		}
		metadata.replaceFriendlyName(identity.FriendlyName)
		if request.Method == http.MethodDelete {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		handleApplianceMetadata(metadata).ServeHTTP(response, request)
	}
}

func handleCurrentDiscoveryDescription(metadata *ApplianceMetadata) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		handler, err := validateApplianceMetadata(metadata)
		if err != nil || handler == nil {
			writeError(response, http.StatusServiceUnavailable, "appliance_description_unavailable", "")
			return
		}
		handler.ServeHTTP(response, request)
	}
}

func validateApplianceMetadata(metadata *ApplianceMetadata) (http.Handler, error) {
	if metadata == nil {
		return nil, nil
	}
	snapshot := metadata.snapshot()
	if snapshot.State != "provisioning" && snapshot.State != "commissioned" {
		return nil, errors.New("valid appliance lifecycle state is required")
	}
	return NewDiscoveryDescriptionHandler(DiscoveryDescription{
		ApplianceID: snapshot.ID, SerialName: snapshot.SerialName,
		FriendlyName: snapshot.FriendlyName, PresentationURL: snapshot.ManagementURL,
		HardwareAddresses: snapshot.ManagementMACs,
	})
}
