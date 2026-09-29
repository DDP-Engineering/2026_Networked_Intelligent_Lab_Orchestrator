package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"nilo.local/nilo/internal/bench"
)

type operatingSettingsReplacement struct {
	Revision int64                         `json:"revision"`
	Settings bench.OperatingSettingsValues `json:"settings"`
}

func handleOperatingSettings(manager *bench.Manager) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if manager == nil {
			writeError(response, http.StatusServiceUnavailable, "operating_settings_unavailable", "")
			return
		}
		actor, ok := actorFrom(request.Context())
		if !ok {
			writeUnauthorized(response)
			return
		}
		if request.Method == http.MethodGet {
			state, err := manager.OperatingSettings(request.Context(), actor)
			if err != nil {
				writeOperatingSettingsError(response, err)
				return
			}
			writeJSON(response, http.StatusOK, state)
			return
		}
		var replacement operatingSettingsReplacement
		decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&replacement) != nil || decoder.Decode(new(any)) != io.EOF {
			writeError(response, http.StatusBadRequest, "operating_settings_invalid", "")
			return
		}
		state, err := manager.ReplaceOperatingSettings(request.Context(), actor, replacement.Revision, replacement.Settings)
		if err != nil {
			writeOperatingSettingsError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, state)
	}
}

func writeOperatingSettingsError(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bench.ErrInvalid):
		writeError(response, http.StatusBadRequest, "operating_settings_invalid", "")
	case errors.Is(err, bench.ErrConflict):
		writeError(response, http.StatusConflict, "operating_settings_conflict", "")
	case errors.Is(err, bench.ErrForbidden):
		writeError(response, http.StatusForbidden, "forbidden", "")
	default:
		writeError(response, http.StatusServiceUnavailable, "operating_settings_unavailable", "")
	}
}
