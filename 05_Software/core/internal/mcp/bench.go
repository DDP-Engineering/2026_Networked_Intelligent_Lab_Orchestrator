package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/identity"
)

// toolActor reads the current HTTP call's credential, never the actor that
// opened an MCP session earlier. Revocation and scope changes take effect now.
func toolActor(ctx context.Context, request *sdk.CallToolRequest, options HandlerOptions) (identity.Actor, error) {
	if request.Extra == nil {
		return identity.Actor{}, bench.ErrForbidden
	}
	token, ok := bearerToken(request.Extra.Header.Get("Authorization"))
	if !ok {
		return identity.Actor{}, bench.ErrForbidden
	}
	return options.Identity.AuthenticateToken(ctx, token)
}

func registerBenchTools(server *sdk.Server, options HandlerOptions) {
	registerSerialTools(server, options)
	registerModbusTools(server, options)
	register := func(name, description string, readOnly bool, properties map[string]any, required []string, call func(context.Context, identity.Actor, json.RawMessage) (any, error)) {
		server.AddTool(&sdk.Tool{Name: name, Description: description, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: readOnly}, InputSchema: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}}, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			actor, err := toolActor(ctx, request, options)
			if err != nil {
				return nil, err
			}
			if len(properties) == 0 {
				var empty struct{}
				if decodeBenchArguments(request.Params.Arguments, &empty) != nil {
					return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: bench.ErrInvalid.Error()}}}, nil
				}
			}
			value, err := call(ctx, actor, request.Params.Arguments)
			if err != nil {
				return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}}}, nil
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(encoded)}}}, nil
		})
	}
	text := func() map[string]any { return map[string]any{"type": "string"} }
	register("camera_workspace_status", "Read camera workspace state, settings revision, recording progress and storage limits.", true, map[string]any{"device_id": text()}, []string{"device_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"device_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.CameraWorkspace(ctx, a, input.ID)
	})
	register("camera_workspace_stop", "Stop an owned camera recording or live session without approving a new operation.", false, map[string]any{"device_id": text(), "action": map[string]any{"type": "string", "enum": []string{"stop", "record_stop"}}}, []string{"device_id", "action"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID     string `json:"device_id"`
			Action string `json:"action"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		err := options.Bench.StopCamera(ctx, a, input.ID, input.Action)
		return map[string]bool{"stopped": err == nil}, err
	})
	register("camera_media_list", "List 24 authorized captures. Content downloads use the authenticated /api/v1/camera-media/{id}/content HTTP endpoint.", true, map[string]any{"device_id": text(), "offset": map[string]any{"type": "integer", "minimum": 0}}, []string{"device_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID     string `json:"device_id"`
			Offset int    `json:"offset"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.CameraMediaList(ctx, a, input.ID, input.Offset)
	})
	register("camera_media_delete", "Permanently delete an owned completed or incomplete capture. Active media is refused.", false, map[string]any{"media_id": text()}, []string{"media_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"media_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		err := options.Bench.DeleteCameraMedia(ctx, a, input.ID)
		return map[string]bool{"deleted": err == nil}, err
	})
	register("camera_retention_configure", "Set camera retention limits at the observed revision. Requires plugins.manage and a stopped camera. All completed captures share the same automatic retention policy.", false, map[string]any{"device_id": text(), "revision": map[string]any{"type": "integer", "minimum": 0}, "limits": map[string]any{"type": "object"}}, []string{"device_id", "revision", "limits"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID       string                `json:"device_id"`
			Revision int64                 `json:"revision"`
			Limits   bench.CameraRetention `json:"limits"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		err := options.Bench.SetCameraRetention(ctx, a, input.ID, input.Revision, input.Limits)
		return map[string]bool{"saved": err == nil}, err
	})
	register("plugins_list", "List available and installed plugins and enabled state.", true, map[string]any{}, []string{}, func(ctx context.Context, a identity.Actor, _ json.RawMessage) (any, error) {
		return options.Bench.ListPlugins(ctx, a)
	})
	register("plugin_lifecycle", "Install, enable, disable, restart or uninstall a curated plugin. Requires plugins.manage. Busy restart/uninstall is refused; history is retained.", false, map[string]any{"plugin_id": text(), "action": map[string]any{"type": "string", "enum": []string{"install", "enable", "disable", "restart", "uninstall"}}, "mode": map[string]any{"type": "string", "enum": []string{"ask", "auto"}}}, []string{"plugin_id", "action"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID     string `json:"plugin_id"`
			Action string `json:"action"`
			Mode   string `json:"mode"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.Lifecycle(ctx, a, input.ID, input.Action, input.Mode)
	})
	register("plugin_discover", "Discover physical devices on an enabled plugin.", false, map[string]any{"plugin_id": text()}, []string{"plugin_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"plugin_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.Discover(ctx, a, input.ID)
	})
	register("bench_submit", "Queue a declared plugin operation. Returns transaction_id immediately; poll bench_get_job for delayed replies. Reuse idempotency_key only with identical inputs. Approval requires a human GUI session.", false, map[string]any{"plugin_id": text(), "operation": text(), "device_id": text(), "parameters": map[string]any{"type": "object"}, "idempotency_key": text(), "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 300}}, []string{"plugin_id", "operation", "parameters", "idempotency_key"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input bench.Submission
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.Submit(ctx, a, input)
	})
	register("plugin_devices", "List persisted managed devices for an installed plugin.", true, map[string]any{"plugin_id": text()}, []string{"plugin_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"plugin_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		plugins, err := options.Bench.ListPlugins(ctx, a)
		if err != nil {
			return nil, err
		}
		for _, plugin := range plugins {
			if plugin.ID == input.ID && plugin.Installed {
				return plugin.Devices, nil
			}
		}
		return nil, bench.ErrNotFound
	})
	register("plugin_configure", "Set revisioned plugin settings against the installed schema. Requires plugins.manage.", false, map[string]any{"plugin_id": text(), "revision": map[string]any{"type": "integer", "minimum": 0}, "settings": map[string]any{"type": "object"}}, []string{"plugin_id", "revision", "settings"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID       string          `json:"plugin_id"`
			Revision int64           `json:"revision"`
			Settings json.RawMessage `json:"settings"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ConfigurePlugin(ctx, a, input.ID, input.Revision, input.Settings)
	})
	register("plugin_configuration", "Read a plugin's current configuration and revision. Requires plugins.manage.", true, map[string]any{"plugin_id": text()}, []string{"plugin_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"plugin_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.PluginConfiguration(ctx, a, input.ID)
	})
	register("plugin_device_configure", "Name and configure a managed device at its current revision. Requires plugins.manage.", false, map[string]any{"plugin_id": text(), "device_id": text(), "revision": map[string]any{"type": "integer", "minimum": 1}, "name": text(), "purpose": text(), "settings": map[string]any{"type": "object"}}, []string{"plugin_id", "device_id", "revision", "name", "settings"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			PluginID string          `json:"plugin_id"`
			DeviceID string          `json:"device_id"`
			Revision int64           `json:"revision"`
			Name     string          `json:"name"`
			Purpose  string          `json:"purpose"`
			Settings json.RawMessage `json:"settings"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ConfigureDevice(ctx, a, input.PluginID, input.DeviceID, input.Revision, input.Name, input.Purpose, input.Settings)
	})
	register("plugin_device_rebind", "Rebind a provisional physical device to an absent record. Requires current revisions and exact record-ID confirmation.", false,
		map[string]any{"plugin_id": text(), "device_id": text(), "revision": map[string]any{"type": "integer", "minimum": 1}, "provisional_id": text(), "provisional_revision": map[string]any{"type": "integer", "minimum": 1}, "confirm_device_id": text()},
		[]string{"plugin_id", "device_id", "revision", "provisional_id", "provisional_revision", "confirm_device_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
			var input struct {
				PluginID            string `json:"plugin_id"`
				DeviceID            string `json:"device_id"`
				Revision            int64  `json:"revision"`
				ProvisionalID       string `json:"provisional_id"`
				ProvisionalRevision int64  `json:"provisional_revision"`
				Confirmation        string `json:"confirm_device_id"`
			}
			if decodeBenchArguments(raw, &input) != nil {
				return nil, bench.ErrInvalid
			}
			return options.Bench.RebindDevice(ctx, a, input.PluginID, input.DeviceID, input.Revision, input.ProvisionalID, input.ProvisionalRevision, input.Confirmation)
		})
	register("plugin_device_forget", "Forget a managed device record after checking its revision and exact record-ID confirmation. History remains retained.", false,
		map[string]any{"plugin_id": text(), "device_id": text(), "revision": map[string]any{"type": "integer", "minimum": 1}, "confirm_device_id": text()},
		[]string{"plugin_id", "device_id", "revision", "confirm_device_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
			var input struct {
				PluginID     string `json:"plugin_id"`
				DeviceID     string `json:"device_id"`
				Revision     int64  `json:"revision"`
				Confirmation string `json:"confirm_device_id"`
			}
			if decodeBenchArguments(raw, &input) != nil {
				return nil, bench.ErrInvalid
			}
			if err := options.Bench.ForgetDevice(ctx, a, input.PluginID, input.DeviceID, input.Revision, input.Confirmation); err != nil {
				return nil, err
			}
			return map[string]any{"forgotten": input.DeviceID}, nil
		})
	register("plugin_device_action", "Enable, disable or retry initialization for one physical device at its current revision. Requires plugins.manage.", false,
		map[string]any{"plugin_id": text(), "device_id": text(), "revision": map[string]any{"type": "integer", "minimum": 1}, "action": map[string]any{"type": "string", "enum": []string{"enable", "disable", "retry"}}},
		[]string{"plugin_id", "device_id", "revision", "action"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
			var input struct {
				PluginID string `json:"plugin_id"`
				DeviceID string `json:"device_id"`
				Revision int64  `json:"revision"`
				Action   string `json:"action"`
			}
			if decodeBenchArguments(raw, &input) != nil {
				return nil, bench.ErrInvalid
			}
			if input.Action == "retry" {
				return options.Bench.RetryDevice(ctx, a, input.PluginID, input.DeviceID, input.Revision)
			}
			if input.Action != "enable" && input.Action != "disable" {
				return nil, bench.ErrInvalid
			}
			return options.Bench.SetDeviceEnabled(ctx, a, input.PluginID, input.DeviceID, input.Revision, input.Action == "enable")
		})
	cameraProperties := map[string]any{"device_id": text(), "parameters": map[string]any{"type": "object"}, "idempotency_key": text(), "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 300}}
	for _, operation := range []string{"get_controls", "set_controls", "reset_controls"} {
		operation := operation
		register("camera_"+operation, "Queue a physical camera control operation and return its durable transaction.", false, cameraProperties, []string{"device_id", "parameters"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
			var input struct {
				DeviceID   string          `json:"device_id"`
				Parameters json.RawMessage `json:"parameters"`
				Key        string          `json:"idempotency_key"`
				Timeout    int             `json:"timeout_seconds"`
			}
			if decodeBenchArguments(raw, &input) != nil {
				return nil, bench.ErrInvalid
			}
			if input.Key == "" {
				input.Key = uuid.NewString()
			}
			return options.Bench.Submit(ctx, a, bench.Submission{PluginID: "camera", Operation: operation, DeviceID: input.DeviceID, Parameters: input.Parameters, IdempotencyKey: input.Key, TimeoutSeconds: input.Timeout})
		})
	}
	server.AddTool(&sdk.Tool{Name: "camera_snapshot", Description: "Capture a physical camera. Return a bounded JPEG when completed, or a durable pending transaction ID.", Annotations: &sdk.ToolAnnotations{ReadOnlyHint: false}, InputSchema: map[string]any{"type": "object", "properties": map[string]any{"device_id": text(), "parameters": map[string]any{"type": "object"}, "idempotency_key": text(), "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 300}, "wait_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 20}}, "required": []string{"device_id", "parameters"}, "additionalProperties": false}}, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		actor, err := toolActor(ctx, request, options)
		if err != nil {
			return nil, err
		}
		var input struct {
			DeviceID   string          `json:"device_id"`
			Parameters json.RawMessage `json:"parameters"`
			Key        string          `json:"idempotency_key"`
			Timeout    int             `json:"timeout_seconds"`
			Wait       int             `json:"wait_seconds"`
		}
		if decodeBenchArguments(request.Params.Arguments, &input) != nil || input.Wait < 0 || input.Wait > 20 {
			return cameraToolError(bench.ErrInvalid), nil
		}
		if input.Key == "" {
			input.Key = uuid.NewString()
		}
		job, err := options.Bench.Submit(ctx, actor, bench.Submission{PluginID: "camera", Operation: "snapshot", DeviceID: input.DeviceID, Parameters: input.Parameters, IdempotencyKey: input.Key, TimeoutSeconds: input.Timeout})
		if err != nil {
			return cameraToolError(err), nil
		}
		deadline := time.NewTimer(time.Duration(input.Wait) * time.Second)
		defer deadline.Stop()
		for input.Wait > 0 && job.State != "completed" && !cameraTerminal(job.State) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-deadline.C:
				input.Wait = 0
			case <-time.After(250 * time.Millisecond):
				job, err = options.Bench.GetJob(ctx, actor, job.ID)
				if err != nil {
					return cameraToolError(err), nil
				}
			}
		}
		if job.State == "completed" {
			preview, err := options.Bench.OpenArtifactPreview(ctx, actor, job.ID)
			if err != nil {
				return cameraToolError(err), nil
			}
			correlation, _ := json.Marshal(map[string]any{"transaction_id": job.ID, "state": "completed", "artifact_id": job.ArtifactID})
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(correlation)}, &sdk.ImageContent{MIMEType: "image/jpeg", Data: preview}}}, nil
		}
		pending, _ := json.Marshal(map[string]any{"transaction_id": job.ID, "state": job.State, "poll_tool": "bench_get_job", "error": job.Error})
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(pending)}}}, nil
	})
	register("bench_list_jobs", "Read recent owned transaction summaries; bench_get_job retrieves replies; administrators may read all jobs.", true, map[string]any{}, []string{}, func(ctx context.Context, a identity.Actor, _ json.RawMessage) (any, error) {
		return options.Bench.ListJobs(ctx, a)
	})
	register("bench_get_job", "Read one transaction, deadline, ordered replies, terminal result and artifact ID.", true, map[string]any{"transaction_id": text()}, []string{"transaction_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"transaction_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.GetJob(ctx, a, input.ID)
	})
	register("bench_cancel", "Cancel one transaction. Running cleanup may finish later; poll its state before reusing hardware.", false, map[string]any{"transaction_id": text()}, []string{"transaction_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"transaction_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.Action(ctx, a, input.ID, "cancel")
	})
	register("bench_read_artifact", "Read a bounded base64 chunk of a completed capture artifact, with transaction correlation and total size.", true, map[string]any{"transaction_id": text(), "offset": map[string]any{"type": "integer", "minimum": 0}, "length": map[string]any{"type": "integer", "minimum": 1, "maximum": 65536}}, []string{"transaction_id", "offset", "length"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID     string `json:"transaction_id"`
			Offset int64  `json:"offset"`
			Length int    `json:"length"`
		}
		if decodeBenchArguments(raw, &input) != nil || input.Offset < 0 || input.Length < 1 || input.Length > 65536 {
			return nil, bench.ErrInvalid
		}
		content, metadata, err := options.Bench.OpenArtifact(ctx, a, input.ID)
		if err != nil {
			return nil, err
		}
		defer content.Close()
		if input.Offset > metadata.SizeBytes {
			return nil, bench.ErrInvalid
		}
		if _, err = io.CopyN(io.Discard, content, input.Offset); err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(content, int64(input.Length)))
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		return map[string]any{"transaction_id": input.ID, "artifact_id": metadata.ID.String(), "size_bytes": metadata.SizeBytes, "offset": input.Offset, "next_offset": input.Offset + int64(len(data)), "base64": base64.StdEncoding.EncodeToString(data)}, nil
	})
}

// decodeBenchArguments enforces the same closed input contract as HTTP.
func decodeBenchArguments(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return bench.ErrInvalid
	}
	if decoder.Decode(new(any)) != io.EOF {
		return bench.ErrInvalid
	}
	return nil
}

func cameraToolError(err error) *sdk.CallToolResult {
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}}}
}

func cameraTerminal(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled" || state == "incomplete"
}
