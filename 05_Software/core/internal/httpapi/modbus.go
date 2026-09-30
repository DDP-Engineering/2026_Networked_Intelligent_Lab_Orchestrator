package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	register("GET /api/v1/modbus/settings", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusSettings(r.Context(), a)
		reply(w, v, err)
	})
	register("PUT /api/v1/modbus/settings", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ExpectedRevision int64                      `json:"expected_revision"`
			Settings         bench.ModbusSettingsValues `json:"settings"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ReplaceModbusSettings(r.Context(), a, body.ExpectedRevision, body.Settings)
		reply(w, v, err)
	})
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
	register("GET /api/v1/modbus/maps", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusMaps(r.Context(), a)
		reply(w, map[string]any{"maps": v}, err)
	})
	register("GET /api/v1/modbus/maps/{id}", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusMap(r.Context(), a, r.PathValue("id"))
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/maps/{id}/revisions/{revision}", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		revision, err := strconv.ParseInt(r.PathValue("revision"), 10, 64)
		if err != nil {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusMapRevision(r.Context(), a, r.PathValue("id"), revision)
		reply(w, v, err)
	})
	register("PUT /api/v1/modbus/maps/{id}", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ExpectedRevision int64      `json:"expected_revision"`
			Map              modbus.Map `json:"map"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		if body.Map.ID != r.PathValue("id") {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.SaveModbusMap(r.Context(), a, body.ExpectedRevision, body.Map)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/maps/{id}/duplicate", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			NewID string `json:"new_id"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.DuplicateModbusMap(r.Context(), a, r.PathValue("id"), body.NewID)
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/maps/{id}/export", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "json"
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ExportModbusMap(r.Context(), a, r.PathValue("id"), format)
		reply(w, map[string]any{"format": format, "document": v}, err)
	})
	register("POST /api/v1/modbus/maps/import-preview", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body bench.ModbusMapImport
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.PreviewModbusMapImport(r.Context(), a, body)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/maps/import", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body bench.ModbusMapImport
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ImportModbusMap(r.Context(), a, body)
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/workspaces", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusWorkspaces(r.Context(), a)
		reply(w, map[string]any{"workspaces": v}, err)
	})
	register("GET /api/v1/modbus/workspaces/{id}", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusWorkspace(r.Context(), a, r.PathValue("id"))
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/workspaces/{id}/revisions/{revision}", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		revision, err := strconv.ParseInt(r.PathValue("revision"), 10, 64)
		if err != nil {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusWorkspaceRevision(r.Context(), a, r.PathValue("id"), revision)
		reply(w, v, err)
	})
	register("PUT /api/v1/modbus/workspaces/{id}", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ExpectedRevision int64            `json:"expected_revision"`
			Workspace        modbus.Workspace `json:"workspace"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		if body.Workspace.ID != r.PathValue("id") {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.SaveModbusWorkspace(r.Context(), a, body.ExpectedRevision, body.Workspace)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/workspaces/{id}/duplicate", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			NewID string `json:"new_id"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.DuplicateModbusWorkspace(r.Context(), a, r.PathValue("id"), body.NewID)
		reply(w, v, err)
	})
	register("GET /api/v1/modbus/workspaces/{id}/export", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, err := m.ModbusWorkspace(r.Context(), a, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		raw, err := json.Marshal(v)
		reply(w, map[string]any{"format": "json", "document": string(raw)}, err)
	})
	register("POST /api/v1/modbus/workspaces/import-preview", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body bench.ModbusWorkspaceImport
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.PreviewModbusWorkspaceImport(r.Context(), a, body)
		reply(w, v, err)
	})
	register("POST /api/v1/modbus/workspaces/import", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body bench.ModbusWorkspaceImport
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.ImportModbusWorkspace(r.Context(), a, body)
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
	register("POST /api/v1/modbus/endpoints/{id}/test", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var body bench.ModbusConnect
		if !benchBody(w, r, &body) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, err := m.TestModbusConnection(r.Context(), a, r.PathValue("id"), body)
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
