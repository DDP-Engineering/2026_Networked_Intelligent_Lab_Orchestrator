package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/power"
)

const maxPowerRequestBytes = 4 << 10

type powerAction string

const (
	powerActionRestart  powerAction = "restart"
	powerActionPowerOff powerAction = "power_off"
)

type powerResponse struct {
	APIVersion string      `json:"api_version"`
	Action     powerAction `json:"action"`
	Status     string      `json:"status"`
}

func handlePower(controller power.Controller, journal AuditJournal) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok || actor.Credential != identity.CredentialSession {
			if ok {
				accessGuard{audit: journal}.denied(request, actor, auth.PermissionSystemPowerManage)
			}
			writeForbidden(response)
			return
		}

		var body struct {
			Action powerAction `json:"action"`
		}
		if decodePowerRequest(request, &body) != nil ||
			(body.Action != powerActionRestart && body.Action != powerActionPowerOff) {
			writeError(response, http.StatusBadRequest, "invalid_power_action", "")
			return
		}
		if controller == nil {
			writeError(response, http.StatusServiceUnavailable, "power_control_unavailable", "")
			return
		}
		if journal == nil {
			writeError(response, http.StatusInternalServerError, "power_audit_failed", "")
			return
		}

		record := audit.Record{
			Category: audit.CategoryAudit, Component: "system", Action: "system.power.requested",
			SubjectType: "appliance", SubjectID: "local", TargetLabel: string(body.Action),
			Outcome: audit.OutcomeRequested, Severity: audit.SeverityWarning,
			Details: map[string]string{"action": string(body.Action)},
		}
		record.ActorKind, record.ActorID = actorIdentity(actor)
		if err := journal.Record(request.Context(), record); err != nil {
			writeError(response, http.StatusInternalServerError, "power_audit_failed", "")
			return
		}

		var err error
		switch body.Action {
		case powerActionRestart:
			err = controller.Restart(request.Context())
		case powerActionPowerOff:
			err = controller.PowerOff(request.Context())
		}
		if errors.Is(err, power.ErrUnavailable) {
			writeError(response, http.StatusServiceUnavailable, "power_control_unavailable", "")
			return
		}
		if err != nil {
			writeError(response, http.StatusInternalServerError, "power_action_failed", "")
			return
		}
		writeJSON(response, http.StatusAccepted, powerResponse{APIVersion: apiVersion, Action: body.Action, Status: "accepted"})
	}
}

func decodePowerRequest(request *http.Request, into any) error {
	data, err := io.ReadAll(io.LimitReader(request.Body, maxPowerRequestBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxPowerRequestBytes {
		return errors.New("power request too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("power request must contain one JSON object")
	}
	return nil
}
