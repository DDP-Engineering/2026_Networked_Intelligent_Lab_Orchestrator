package httpapi

import (
	"errors"
	"net/http"

	"nilo.local/nilo/internal/network"
)

func writeAdapterRoleConflict(response http.ResponseWriter, err error) bool {
	var conflict *network.AdapterRoleConflictError
	if !errors.As(err, &conflict) {
		return false
	}
	writeJSON(response, http.StatusConflict, struct {
		Error   string                            `json:"error"`
		Message string                            `json:"message"`
		Details *network.AdapterRoleConflictError `json:"details"`
	}{
		Error: "network_interface_role_conflict", Message: conflict.Error(), Details: conflict,
	})
	return true
}
