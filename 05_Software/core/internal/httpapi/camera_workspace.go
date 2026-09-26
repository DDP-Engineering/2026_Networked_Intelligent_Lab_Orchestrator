package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
)

func registerCameraWorkspace(register func(string, auth.Permission, http.HandlerFunc), manager *bench.Manager) {
	register("DELETE /api/v1/camera-legacy/{id}", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		if err := manager.DeleteLegacyCameraMedia(r.Context(), actor, r.PathValue("id")); err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": true})
	})
	register("POST /api/v1/cameras/{id}/stop", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string `json:"action"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		actor, _ := actorFrom(r.Context())
		if err := manager.StopCamera(r.Context(), actor, r.PathValue("id"), body.Action); err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"stopped": true})
	})
	register("GET /api/v1/cameras/{id}/workspace", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		value, err := manager.CameraWorkspace(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, value)
	})
	register("GET /api/v1/cameras/{id}/frame", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		value, err := manager.CameraFrame(r.Context(), actor, r.PathValue("id"))
		if err != nil {
			benchError(w, err)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(value)
	})
	register("GET /api/v1/cameras/{id}/media", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		actor, _ := actorFrom(r.Context())
		value, err := manager.CameraMediaList(r.Context(), actor, r.PathValue("id"), offset, r.URL.Query().Get("kind"))
		if err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": value})
	})
	register("GET /api/v1/camera-media/{id}/content", auth.PermissionBenchRead, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		file, media, err := manager.OpenCameraMedia(r.Context(), actor, r.PathValue("id"), r.URL.Query().Get("thumbnail") == "true")
		if err != nil {
			benchError(w, err)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", media)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Query().Get("download") == "true" {
			extension := "jpg"
			if media == "video/mp4" {
				extension = "mp4"
			}
			if media == "image/png" {
				extension = "png"
			}
			w.Header().Set("Content-Disposition", `attachment; filename="capture.`+extension+`"`)
		}
		http.ServeContent(w, r, "capture", time.Time{}, file)
	})
	register("DELETE /api/v1/camera-media/{id}", auth.PermissionBenchExecute, func(w http.ResponseWriter, r *http.Request) {
		actor, _ := actorFrom(r.Context())
		if err := manager.DeleteCameraMedia(r.Context(), actor, r.PathValue("id")); err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"deleted": true})
	})
	register("PUT /api/v1/cameras/{id}/retention", auth.PermissionPluginsManage, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Revision int64                 `json:"revision"`
			Limits   bench.CameraRetention `json:"limits"`
		}
		if !benchBody(w, r, &body) {
			return
		}
		actor, _ := actorFrom(r.Context())
		if err := manager.SetCameraRetention(r.Context(), actor, r.PathValue("id"), body.Revision, body.Limits); err != nil {
			benchError(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"saved": true})
	})
}
