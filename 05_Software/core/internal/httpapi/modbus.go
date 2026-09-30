package httpapi

import (
	"net/http"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/modbus"
)

func registerModbus(register func(string, auth.Permission, http.HandlerFunc), m *bench.Manager) {
	reply := func(w http.ResponseWriter, value any, err error) {
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	}
	register("GET /api/v1/modbus/endpoints", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusEndpoints(r.Context(), a)
		reply(w, map[string]any{"endpoints": v}, err)
	})
	register("PUT /api/v1/modbus/endpoints/{id}", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ExpectedRevision int64           `json:"expected_revision"`
			Endpoint         modbus.Endpoint `json:"endpoint"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		if body.Endpoint.ID != r.PathValue("id") {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.SaveModbusEndpoint(r.Context(), a, body.ExpectedRevision, body.Endpoint)
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/profiles", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusProfiles(r.Context(), a)
		reply(w, map[string]any{"profiles": v}, err)
	})
	register("PUT /api/v1/modbus/profiles/{id}", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ExpectedRevision int64          `json:"expected_revision"`
			Profile          modbus.Profile `json:"profile"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		if body.Profile.ID != r.PathValue("id") {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.SaveModbusProfile(r.Context(), a, body.ExpectedRevision, body.Profile)
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/endpoints/{id}/session", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusSession(r.Context(), a, r.PathValue("id"))
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/endpoints/{id}/connect", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body bench.ModbusConnect
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ConnectModbus(r.Context(), a, r.PathValue("id"), body)
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, v)
	})
	register("POST /api/v1/modbus/endpoints/{id}/read", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string              `json:"session_id"`
			Request   bench.ModbusRequest `json:"request"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ReadModbus(r.Context(), a, r.PathValue("id"), body.SessionID, body.Request)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/endpoints/{id}/write", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body bench.ModbusWrite
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.WriteModbus(r.Context(), a, r.PathValue("id"), body)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/endpoints/{id}/preview", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body bench.ModbusWrite
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.PreviewModbusWrite(r.Context(), a, r.PathValue("id"), body)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/endpoints/{id}/poll", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string                  `json:"session_id"`
			Groups    []bench.ModbusPollGroup `json:"groups"`
			Paused    bool                    `json:"paused"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.SetModbusPolling(r.Context(), a, r.PathValue("id"), body.SessionID, body.Groups, body.Paused)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/endpoints/{id}/display", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string `json:"session_id"`
			Paused    bool   `json:"paused"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.SetModbusDisplayPaused(r.Context(), a, r.PathValue("id"), body.SessionID, body.Paused)
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/endpoints/{id}/events", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		after, err := serialQuery(r, "after", 0)
		if err != nil {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ReadModbusEvents(r.Context(), a, r.PathValue("id"), r.URL.Query().Get("session_id"), after)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/endpoints/{id}/disconnect", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string `json:"session_id"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		err := m.DisconnectModbus(r.Context(), a, r.PathValue("id"), body.SessionID)
		reply(w, map[string]bool{"ok": err == nil}, err)
	})
	register("GET /api/v1/modbus/commands/{id}", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusCommand(r.Context(), a, r.PathValue("id"))
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/endpoints/{id}/commands", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		offset, err := serialQuery(r, "offset", 0)
		if err != nil || offset < 0 || offset > 100000 {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ListModbusCommands(r.Context(), a, r.PathValue("id"), r.URL.Query().Get("session_id"), int(offset))
		reply(w, v, err)
	})
}
