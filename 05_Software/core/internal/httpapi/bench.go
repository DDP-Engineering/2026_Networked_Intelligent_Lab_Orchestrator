package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/domain"
)

var artifactMediaTypePattern = regexp.MustCompile(`^[a-z0-9.+-]+/[a-z0-9.+-]+$`)
var artifactFilenamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,79}$`)

func artifactPresentation(metadata domain.Artifact) (string, string) {
	if !artifactMediaTypePattern.MatchString(metadata.MediaType) {
		return "application/octet-stream", `attachment; filename="artifact.bin"`
	}
	filename := "artifact.bin"
	var hints struct {
		Filename string `json:"filename"`
	}
	if json.Unmarshal(metadata.PresentationHints, &hints) == nil && artifactFilenamePattern.MatchString(hints.Filename) && !strings.Contains(hints.Filename, "..") {
		filename = hints.Filename
	}
	disposition := "attachment"
	if metadata.MediaType == "image/jpeg" || metadata.MediaType == "image/png" {
		disposition = "inline"
	}
	return metadata.MediaType, disposition + `; filename="` + filename + `"`
}

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
	registerCameraWorkspace(register, manager)
	registerSerial(register, manager)
	registerModbus(register, manager)
	register("GET /api/v1/plugins", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		plugins, err := manager.ListPlugins(r.Context(), actor)
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"plugins": plugins})
	})
	register("GET /api/v1/plugins/{id}/configuration", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		configuration, err := manager.PluginConfiguration(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"revision": configuration.Revision, "settings": configuration.Settings})
	})
	register("PUT /api/v1/plugins/{id}/configuration", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Revision int64           `json:"revision"`
			Settings json.RawMessage `json:"settings"`
		}
		if !benchBody(w, r, &input) {
			return
		}
		actor, _ := actorFrom(r.Context())
		configuration, err := manager.ConfigurePlugin(r.Context(), actor, r.PathValue("id"), input.Revision, input.Settings)
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"revision": configuration.Revision, "settings": configuration.Settings})
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
	register("GET /api/v1/plugins/{id}/devices", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		devices, err := manager.ListManagedDevices(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"devices": devices})
	})
	configureDevice := func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Revision int64           `json:"revision"`
			Name     string          `json:"name"`
			Purpose  string          `json:"purpose"`
			Settings json.RawMessage `json:"settings"`
		}
		if !benchBody(w, r, &input) {
			return
		}
		actor, _ := actorFrom(r.Context())
		device, err := manager.ConfigureDevice(r.Context(), actor, r.PathValue("id"), r.PathValue("device"), input.Revision, input.Name, input.Purpose, input.Settings)
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, device)
	}
	register("PUT /api/v1/plugins/{id}/devices/{device}", auth.PermissionPluginsManage, configureDevice)
	register("POST /api/v1/plugins/{id}/devices/{device}/configure", auth.PermissionPluginsManage, configureDevice)
	register("POST /api/v1/plugins/{id}/devices/{device}/rebind", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Revision            int64  `json:"revision"`
			ProvisionalID       string `json:"provisional_id"`
			ProvisionalRevision int64  `json:"provisional_revision"`
			ConfirmDeviceID     string `json:"confirm_device_id"`
		}
		if !benchBody(w, r, &input) {
			return
		}
		actor, _ := actorFrom(r.Context())
		device, err := manager.RebindDevice(r.Context(), actor, r.PathValue("id"), r.PathValue("device"), input.Revision, input.ProvisionalID, input.ProvisionalRevision, input.ConfirmDeviceID)
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, device)
	})
	register("DELETE /api/v1/plugins/{id}/devices/{device}", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Revision        int64  `json:"revision"`
			ConfirmDeviceID string `json:"confirm_device_id"`
		}
		if !benchBody(w, r, &input) {
			return
		}
		actor, _ := actorFrom(r.Context())
		if err := manager.ForgetDevice(r.Context(), actor, r.PathValue("id"), r.PathValue("device"), input.Revision, input.ConfirmDeviceID); err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"forgotten": r.PathValue("device")})
	})
	for _, action := range []string{"enable", "disable", "retry"} {
		register("POST /api/v1/plugins/{id}/devices/{device}/"+action, auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Revision int64 `json:"revision"`
			}
			if !benchBody(w, r, &input) {
				return
			}
			actor, _ := actorFrom(r.Context())
			var device bench.Device
			var err error
			if action == "retry" {
				device, err = manager.RetryDevice(r.Context(), actor, r.PathValue("id"), r.PathValue("device"), input.Revision)
			} else {
				device, err = manager.SetDeviceEnabled(r.Context(), actor, r.PathValue("id"), r.PathValue("device"), input.Revision, action == "enable")
			}
			if err != nil {
				benchError(w, err)
				return
			}
			writeJSON(w, 200, device)
		})
	}
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
		mediaType, disposition := artifactPresentation(metadata)
		w.Header().Set("Content-Type", mediaType)
		w.Header().Set("Content-Disposition", disposition)
		w.Header().Set("Content-Length", strconv.FormatInt(metadata.SizeBytes, 10))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.Copy(w, content)
	})
	register("GET /api/v1/bench/jobs/{id}/artifact/preview", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		preview, err := manager.OpenArtifactPreview(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", strconv.Itoa(len(preview)))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", `inline; filename="preview.jpg"`)
		_, _ = w.Write(preview)
	})
}
