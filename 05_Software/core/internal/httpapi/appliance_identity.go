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
	ExecuteIdentifyAction(context.Context, network.IdentifyActionRequest) (network.IdentifyActionResult, error)
	AcknowledgeIdentifyAction(context.Context, string) error
}

type applianceNamingService interface {
	SetFriendlyName(context.Context, string) (appliance.Identity, error)
}

type identifyOperationStore interface {
	Begin(context.Context, string, string, time.Time, time.Time) (bool, error)
	Reserve(context.Context, string, audit.Event, time.Time) error
	RecordOutcome(context.Context, string, bool, string, string, audit.Outcome, time.Time) error
	MarkActed(context.Context, string, time.Time) error
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
		label, err := appliance.RenderLabelSVG(appliance.Label{
			SerialName: snapshot.SerialName, ManagementURL: snapshot.ManagementURL, HardwareAddresses: snapshot.ManagementMACs,
		})
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
		generated, err := domain.NewRequestID()
		if err != nil {
			writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
			return
		}
		operationID := generated.String()
		action := network.IdentifyActionRequest{
			OperationID: operationID, Source: network.IdentifySourceInterface,
			ActiveUntil: time.Now().UTC().Add(10 * time.Second),
		}
		if operations != nil && durable {
			if _, err := operations.Begin(request.Context(), operationID, "interface", action.ActiveUntil, time.Now().UTC()); err != nil {
				writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
				return
			}
			completionContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			record.OperationID = operationID
			record.Details = map[string]string{
				"source": "interface", "status": "requested", "reannouncement": "not_requested",
			}
			reserved, composeErr := composer.Compose(record)
			if composeErr == nil {
				composeErr = operations.Reserve(completionContext, operationID, reserved, time.Now().UTC())
			}
			cancel()
			if composeErr != nil {
				writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
				return
			}
		} else if err := journal.Record(request.Context(), record); err != nil {
			writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
			return
		}
		result, err := service.ExecuteIdentifyAction(request.Context(), action)
		if err != nil {
			writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
			return
		}
		if operations != nil && durable {
			completionContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			outcome := audit.OutcomeApplied
			status := string(result.Status)
			if result.Status == network.IdentifyFailed {
				outcome = audit.OutcomeFailed
			}
			if persistErr := markIdentifyActed(completionContext, service, operations, operationID,
				result.Supported, network.IdentifyReannouncementNotRequested, status, outcome, time.Now().UTC()); persistErr != nil {
				writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
				return
			}
			if operations.Flush(completionContext, operationID) != nil {
				writeError(response, http.StatusInternalServerError, "identify_audit_failed", "")
				return
			}
		}
		if result.Status == network.IdentifyFailed {
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

func markIdentifyActed(
	ctx context.Context, service applianceIdentifyService, operations identifyOperationStore, operationID string, supported bool,
	reannouncement, status string, outcome audit.Outcome, now time.Time,
) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = operations.RecordOutcome(ctx, operationID, supported, reannouncement, status, outcome, now)
		if err == nil || ctx.Err() != nil {
			break
		}
	}
	if err != nil {
		return err
	}
	if err := service.AcknowledgeIdentifyAction(ctx, operationID); err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = operations.MarkActed(ctx, operationID, now)
		if err == nil || ctx.Err() != nil {
			return err
		}
	}
	return err
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
			var conflict *appliance.FriendlyNameConflictError
			switch {
			case errors.Is(err, appliance.ErrInvalidFriendlyName):
				writeError(response, http.StatusBadRequest, "invalid_friendly_name", "")
			case errors.As(err, &conflict):
				publicConflict := appliance.NewFriendlyNameConflictError(body.FriendlyName, conflict.Interfaces)
				writeJSON(response, http.StatusConflict, struct {
					Error   string                               `json:"error"`
					Details *appliance.FriendlyNameConflictError `json:"details"`
				}{Error: "friendly_name_conflict", Details: publicConflict})
			case errors.Is(err, appliance.ErrFriendlyNameConflict):
				writeJSON(response, http.StatusConflict, struct {
					Error   string                               `json:"error"`
					Details *appliance.FriendlyNameConflictError `json:"details"`
				}{Error: "friendly_name_conflict", Details: appliance.NewFriendlyNameConflictError(body.FriendlyName, nil)})
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
