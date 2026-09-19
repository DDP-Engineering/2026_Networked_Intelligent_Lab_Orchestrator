package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
)

func benchError(response http.ResponseWriter, err error) {
	code := "integration_error"
	status := http.StatusInternalServerError
	for _, item := range []struct {
		err    error
		status int
	}{
		{bench.ErrInvalid, 400}, {bench.ErrForbidden, 403}, {bench.ErrNotFound, 404},
		{bench.ErrBusy, 409}, {bench.ErrConflict, 409}, {bench.ErrQueueFull, 429}, {bench.ErrUnavailable, 503},
	} {
		if errors.Is(err, item.err) {
			code = item.err.Error()
			status = item.status
			break
		}
	}
	writeError(response, status, code, "")
}
func benchBody(response http.ResponseWriter, request *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		benchError(response, bench.ErrInvalid)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		benchError(response, bench.ErrInvalid)
		return false
	}
	return true
}

func registerBench(mux *http.ServeMux, manager *bench.Manager, guard accessGuard) {
	register := func(pattern string, permission auth.Permission, handler http.HandlerFunc) {
		mux.Handle(pattern, guard.authorize(permission, func(w http.ResponseWriter, r *http.Request) {
			if manager == nil {
				writeError(w, 503, "integration_service_unavailable", "")
				return
			}
			handler(w, r)
		}))
	}
	register("GET /api/v1/plugins", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		plugins, err := manager.ListPlugins(r.Context(), actor)
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"plugins": plugins})
	})
	for _, action := range []string{"install", "upgrade", "enable", "disable", "restart", "uninstall"} {
		register("POST /api/v1/plugins/{id}/"+action, auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Mode string `json:"mode"`
			}
			if !benchBody(w, r, &input) {
				return
			}
			if action != "enable" && input.Mode != "" {
				benchError(w, bench.ErrInvalid)
				return
			}
			actor, _ := actorFrom(r.Context())
			plugin, err := manager.Lifecycle(r.Context(), actor, r.PathValue("id"), action, input.Mode)
			if err != nil {
				benchError(w, err)
				return
			}
			writeJSON(w, 200, plugin)
		})
	}
	register("POST /api/v1/plugins/{id}/discover", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var input struct{}
		if !benchBody(w, r, &input) {
			return
		}
		actor, _ := actorFrom(r.Context())
		devices, err := manager.Discover(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"devices": devices})
	})
	register("GET /api/v1/bench/jobs", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		jobs, err := manager.ListJobs(r.Context(), actor)
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"jobs": jobs})
	})
	register("POST /api/v1/bench/jobs", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var input bench.Submission
		if !benchBody(w, r, &input) {
			return
		}
		actor, _ := actorFrom(r.Context())
		job, err := manager.Submit(r.Context(), actor, input)
		if err != nil {
			benchError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/bench/jobs/"+job.ID)
		writeJSON(w, 202, job)
	})
	register("GET /api/v1/bench/jobs/{id}", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		job, err := manager.GetJob(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, job)
	})
	for _, action := range []string{"approve", "cancel"} {
		permission := auth.PermissionBenchExecute
		if action == "approve" {
			permission = auth.PermissionBenchApprove
		}
		register("POST /api/v1/bench/jobs/{id}/"+action, permission, func(w http.ResponseWriter, r *http.Request) {
			var input struct{}
			if !benchBody(w, r, &input) {
				return
			}
			actor, _ := actorFrom(r.Context())
			job, err := manager.Action(r.Context(), actor, r.PathValue("id"), action)
			if err != nil {
				benchError(w, err)
				return
			}
			writeJSON(w, 202, job)
		})
	}
	register("GET /api/v1/bench/jobs/{id}/artifact", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		content, metadata, err := manager.OpenArtifact(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		defer content.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="capture.sr"`)
		w.Header().Set("Content-Length", strconv.FormatInt(metadata.SizeBytes, 10))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(w, content)
	})
}
