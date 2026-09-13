package httpapi

import (
	"errors"
	"net/http"
	"time"

	"nilo.local/nilo/internal/network"
)

type managementAccessResponse struct {
	APIVersion       string                              `json:"api_version"`
	State            network.ManagementAccessState       `json:"state"`
	Port             int                                 `json:"port"`
	Interfaces       []network.ManagementAccessInterface `json:"interfaces"`
	Configured       []string                            `json:"configured_interfaces"`
	URLs             []network.ManagementURL             `json:"urls"`
	TLSNames         []string                            `json:"tls_names"`
	TLSMissing       []string                            `json:"tls_missing"`
	ConfirmBy        *time.Time                          `json:"confirm_by"`
	Detail           string                              `json:"detail"`
	Outcome          network.ManagementAccessOutcome     `json:"outcome"`
	CurrentInterface string                              `json:"current_interface"`
	Provisioning     bool                                `json:"provisioning"`
}

func managementAccessPayload(status network.ManagementAccessStatus) managementAccessResponse {
	payload := managementAccessResponse{
		APIVersion: apiVersion, State: status.State, Port: status.Port,
		Interfaces: status.Interfaces, Configured: status.Configured, URLs: status.URLs,
		TLSNames: status.TLSNames, TLSMissing: status.TLSMissing, Detail: status.Detail,
		Outcome:          status.Outcome,
		CurrentInterface: status.CurrentInterface,
		Provisioning:     status.Provisioning,
	}
	if !status.ConfirmBy.IsZero() {
		deadline := status.ConfirmBy
		payload.ConfirmBy = &deadline
	}
	if payload.Interfaces == nil {
		payload.Interfaces = []network.ManagementAccessInterface{}
	}
	if payload.Configured == nil {
		payload.Configured = []string{}
	}
	if payload.URLs == nil {
		payload.URLs = []network.ManagementURL{}
	}
	if payload.TLSNames == nil {
		payload.TLSNames = []string{}
	}
	if payload.TLSMissing == nil {
		payload.TLSMissing = []string{}
	}
	return payload
}

func handleReadManagementAccess(service *network.ManagementAccessService) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		status, err := service.Status(request.Context(), LocalDestination(request.Context()))
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "management_access_unavailable", "")
			return
		}
		writeJSON(response, http.StatusOK, managementAccessPayload(status))
	}
}

func handleUpdateManagementAccess(service *network.ManagementAccessService) http.HandlerFunc {
	type input struct {
		Interfaces              []string `json:"interfaces"`
		ConfirmCurrentInterface bool     `json:"confirm_current_interface"`
	}
	return func(response http.ResponseWriter, request *http.Request) {
		var body input
		if !decodeBody(response, request, &body, "invalid_management_access", "") {
			return
		}
		status, err := service.Update(request.Context(), body.Interfaces, body.ConfirmCurrentInterface, LocalDestination(request.Context()))
		if err != nil {
			if writeAdapterRoleConflict(response, err) {
				return
			}
			switch {
			case errors.Is(err, network.ErrCurrentAccessConfirmation):
				writeRefusal(response, http.StatusConflict, "current_interface_confirmation_required", err.Error())
			case errors.Is(err, network.ErrManagementAccessRequired), errors.Is(err, network.ErrManagementAccessUnsafe):
				writeRefusal(response, http.StatusConflict, "management_access_required", err.Error())
			case errors.Is(err, network.ErrManagementAccessPending):
				writeRefusal(response, http.StatusConflict, "management_access_pending", err.Error())
			default:
				writeError(response, http.StatusServiceUnavailable, "management_access_apply_failed", "")
			}
			return
		}
		writeJSON(response, http.StatusAccepted, managementAccessPayload(status))
	}
}

func handleConfirmManagementAccess(service *network.ManagementAccessService) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		status, err := service.Confirm(request.Context(), LocalDestination(request.Context()))
		if err != nil {
			writeRefusal(response, http.StatusConflict, "management_access_confirmation_rejected", err.Error())
			return
		}
		writeJSON(response, http.StatusOK, managementAccessPayload(status))
	}
}

func handleRecoverManagementAccess(service *network.ManagementAccessService) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if err := service.Recover(request.Context(), true); err != nil {
			writeError(response, http.StatusServiceUnavailable, "management_access_recovery_failed", "")
			return
		}
		status, err := service.Status(request.Context(), LocalDestination(request.Context()))
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "management_access_unavailable", "")
			return
		}
		writeJSON(response, http.StatusOK, managementAccessPayload(status))
	}
}
