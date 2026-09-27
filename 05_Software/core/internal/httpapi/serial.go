package httpapi

import (
	"io"
	"net/http"
	"strconv"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
)

func registerSerial(register func(string, auth.Permission, http.HandlerFunc), m *bench.Manager) {
	reply := func(w http.ResponseWriter, v any, err error) {
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, v)
	}
	register("GET /api/v1/serial-devices/{id}/workspace", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, e := m.SerialWorkspace(r.Context(), a, r.PathValue("id"))
		reply(w, v, e)
	})
	register("POST /api/v1/serial-devices/{id}/connect", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var b bench.SerialConnect
		if !benchBody(w, r, &b) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, e := m.ConnectSerial(r.Context(), a, r.PathValue("id"), b)
		if e != nil {
			benchError(w, e)
			return
		}
		writeJSON(w, 202, v)
	})
	register("GET /api/v1/serial-devices/{id}/read", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		after, e := serialQuery(r, "after", 0)
		wait, e2 := serialQuery(r, "wait_ms", 0)
		if e != nil || e2 != nil {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, e := m.ReadSerial(r.Context(), a, r.PathValue("id"), r.URL.Query().Get("connection_id"), after, int(wait))
		reply(w, v, e)
	})
	register("POST /api/v1/serial-devices/{id}/send", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var b bench.SerialSend
		if !benchBody(w, r, &b) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, e := m.SendSerial(r.Context(), a, r.PathValue("id"), b)
		reply(w, v, e)
	})
	register("POST /api/v1/serial-devices/{id}/signals", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var b bench.SerialSignals
		if !benchBody(w, r, &b) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, e := m.SetSerialSignals(r.Context(), a, r.PathValue("id"), b)
		reply(w, v, e)
	})
	register("POST /api/v1/serial-devices/{id}/logging", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var b bench.SerialAction
		if !benchBody(w, r, &b) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, e := m.SerialLogging(r.Context(), a, r.PathValue("id"), b)
		reply(w, v, e)
	})
	register("POST /api/v1/serial-devices/{id}/disconnect", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ConnectionID string `json:"connection_id"`
		}
		if !benchBody(w, r, &b) {
			return
		}
		a, _ := actorFrom(r.Context())
		e := m.DisconnectSerial(r.Context(), a, r.PathValue("id"), b.ConnectionID)
		reply(w, map[string]bool{"ok": true}, e)
	})
	register("GET /api/v1/serial-devices/{id}/logs", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		offset, e := serialQuery(r, "offset", 0)
		if e != nil {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, e := m.ListSerialLogs(r.Context(), a, r.PathValue("id"), int(offset))
		reply(w, v, e)
	})
	register("GET /api/v1/serial-devices/{id}/retention", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		v, e := m.SerialRetention(r.Context(), a, r.PathValue("id"))
		reply(w, v, e)
	})
	register("PUT /api/v1/serial-devices/{id}/retention", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Revision int64                 `json:"revision"`
			Limits   bench.SerialRetention `json:"limits"`
		}
		if !benchBody(w, r, &b) {
			return
		}
		a, _ := actorFrom(r.Context())
		v, e := m.UpdateSerialRetention(r.Context(), a, r.PathValue("id"), b.Revision, b.Limits)
		reply(w, v, e)
	})
	register("GET /api/v1/serial-logs/{id}/events", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		after, e := serialQuery(r, "after", 0)
		if e != nil {
			benchError(w, bench.ErrInvalid)
			return
		}
		a, _ := actorFrom(r.Context())
		v, e := m.ReadSerialLog(r.Context(), a, r.PathValue("id"), after)
		reply(w, v, e)
	})
	register("DELETE /api/v1/serial-logs/{id}", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		e := m.DeleteSerialLog(r.Context(), a, r.PathValue("id"))
		reply(w, map[string]bool{"ok": true}, e)
	})
	register("GET /api/v1/serial-logs/{id}/content", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		a, _ := actorFrom(r.Context())
		reader, name, mime, e := m.ExportSerialLog(r.Context(), a, r.PathValue("id"), r.URL.Query().Get("format"))
		if e != nil {
			benchError(w, e)
			return
		}
		defer reader.Close()
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(w, reader)
	})
}
func serialQuery(r *http.Request, key string, fallback int64) (int64, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseInt(value, 10, 64)
}
