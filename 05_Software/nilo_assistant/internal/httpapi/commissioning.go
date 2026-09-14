package httpapi

import (
	"errors"
	"net/http"

	"nilo.local/nilo/internal/network"
)

// handleCompleteCommissioning fixes the management allowlist to the interfaces
// an administrator chose and records the appliance as commissioned. It is an
// administrator-only route on the same session, CSRF and same-origin machinery
// as every other management change, and it returns the ordinary management
// access payload so the interface can show the result it produced.
func handleCompleteCommissioning(service *network.ManagementAccessService) http.HandlerFunc {
	type input struct {
		Interfaces              []string `json:"interfaces"`
		ConfirmCurrentInterface bool     `json:"confirm_current_interface"`
	}
	return func(response http.ResponseWriter, request *http.Request) {
		var body input
		if !decodeBody(response, request, &body, "invalid_commissioning", "") {
			return
		}
		local := LocalDestination(request.Context())
		commission := service.Commission
		if body.ConfirmCurrentInterface {
			commission = service.CommissionConfirmed
		}
		status, err := commission(request.Context(), body.Interfaces, local)
		if err != nil {
			if writeAdapterRoleConflict(response, err) {
				return
			}
			switch {
			case errors.Is(err, network.ErrCurrentAccessConfirmation):
				writeRefusal(response, http.StatusConflict, "current_interface_confirmation_required", err.Error())
			case errors.Is(err, network.ErrAlreadyCommissioned):
				writeRefusal(response, http.StatusConflict, "already_commissioned", err.Error())
			case errors.Is(err, network.ErrPublishedCredentialUnchanged):
				// Something the administrator can fix from the interface they
				// are already signed into, so it is a conflict naming the
				// change rather than a failure. The sentinel's own text says
				// what to change and carries no credential.
				writeRefusal(response, http.StatusConflict, "published_password_unchanged", err.Error())
			case errors.Is(err, network.ErrPublishedCredentialUnknown):
				writeError(response, http.StatusServiceUnavailable, "commissioning_unavailable", "")
			case errors.Is(err, network.ErrManagementAccessRequired),
				errors.Is(err, network.ErrManagementAccessUnsafe):
				writeRefusal(response, http.StatusConflict, "management_access_required", err.Error())
			case errors.Is(err, network.ErrCommissioningUnavailable):
				writeError(response, http.StatusServiceUnavailable, "commissioning_unavailable", "")
			default:
				writeError(response, http.StatusServiceUnavailable, "commissioning_failed", "")
			}
			return
		}
		writeJSON(response, http.StatusOK, managementAccessPayload(status))
	}
}
