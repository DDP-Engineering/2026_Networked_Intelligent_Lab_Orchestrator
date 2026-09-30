package mcp

import (
	"context"
	"encoding/json"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
)

func registerModbusTools(server *sdk.Server, options HandlerOptions) {
	text := map[string]any{"type": "string", "minLength": 1}
	integer := map[string]any{"type": "integer", "minimum": 0}
	boolean := map[string]any{"type": "boolean"}
	request := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"function": integer, "unit": integer, "address": integer, "count": integer, "values": map[string]any{"type": "array", "items": integer}}, "required": []string{"function", "unit", "address", "count"}}
	group := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"id": text, "interval_ms": integer, "requests": map[string]any{"type": "array", "items": request}}, "required": []string{"id", "interval_ms", "requests"}}
	register := func(name, description string, readOnly bool, properties map[string]any, required []string, call func(context.Context, identity.Actor, json.RawMessage) (any, error)) {
		server.AddTool(&sdk.Tool{Name: name, Description: description, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: readOnly}, InputSchema: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}}, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			actor, err := toolActor(ctx, request, options)
			if err != nil {
				return nil, err
			}
			value, err := call(ctx, actor, request.Params.Arguments)
			if err != nil {
				return cameraToolError(err), nil
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(encoded)}}}, nil
		})
	}
	register("modbus_settings", "Read revisioned Modbus admission and future retention limits.", true, map[string]any{}, nil, func(ctx context.Context, a identity.Actor, _ json.RawMessage) (any, error) {
		return options.Bench.ModbusSettings(ctx, a)
	})
	register("modbus_settings_save", "Replace administrator-owned Modbus settings using an expected revision.", false, map[string]any{"expected_revision": integer, "settings": map[string]any{"type": "object"}}, []string{"expected_revision", "settings"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ExpectedRevision int64                      `json:"expected_revision"`
			Settings         bench.ModbusSettingsValues `json:"settings"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ReplaceModbusSettings(ctx, a, input.ExpectedRevision, input.Settings)
	})
	register("modbus_endpoints", "List shared configured Modbus endpoints and revisions.", true, map[string]any{}, nil, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var empty struct{}
		if decodeBenchArguments(raw, &empty) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusEndpoints(ctx, a)
	})
	register("modbus_endpoint_save", "Save an administrator-owned endpoint using an expected revision.", false, map[string]any{"expected_revision": integer, "endpoint": map[string]any{"type": "object"}}, []string{"expected_revision", "endpoint"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			Expected int64           `json:"expected_revision"`
			Endpoint modbus.Endpoint `json:"endpoint"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SaveModbusEndpoint(ctx, a, input.Expected, input.Endpoint)
	})
	register("modbus_profiles", "List shared Modbus profiles and revisions.", true, map[string]any{}, nil, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var empty struct{}
		if decodeBenchArguments(raw, &empty) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusProfiles(ctx, a)
	})
	register("modbus_profile_save", "Save a Modbus profile using an expected revision.", false, map[string]any{"expected_revision": integer, "profile": map[string]any{"type": "object"}}, []string{"expected_revision", "profile"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			Expected int64          `json:"expected_revision"`
			Profile  modbus.Profile `json:"profile"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SaveModbusProfile(ctx, a, input.Expected, input.Profile)
	})
	register("modbus_maps", "List shared named register maps.", true, map[string]any{}, nil, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct{}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusMaps(ctx, a)
	})
	register("modbus_map_get", "Open one current or pinned named register map without device I/O.", true, map[string]any{"map_id": text, "revision": integer}, []string{"map_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID       string `json:"map_id"`
			Revision int64  `json:"revision"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		if input.Revision > 0 {
			return options.Bench.ModbusMapRevision(ctx, a, input.ID, input.Revision)
		}
		return options.Bench.ModbusMap(ctx, a, input.ID)
	})
	register("modbus_map_save", "Save one map with an expected revision.", false, map[string]any{"expected_revision": integer, "map": map[string]any{"type": "object"}}, []string{"expected_revision", "map"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			Expected int64      `json:"expected_revision"`
			Map      modbus.Map `json:"map"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SaveModbusMap(ctx, a, input.Expected, input.Map)
	})
	register("modbus_map_duplicate", "Duplicate a readable map under a new ID.", false, map[string]any{"map_id": text, "new_id": text}, []string{"map_id", "new_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID    string `json:"map_id"`
			NewID string `json:"new_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.DuplicateModbusMap(ctx, a, input.ID, input.NewID)
	})
	register("modbus_map_export", "Export one map as JSON or CSV metadata.", true, map[string]any{"map_id": text, "format": text}, []string{"map_id", "format"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID     string `json:"map_id"`
			Format string `json:"format"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		document, err := options.Bench.ExportModbusMap(ctx, a, input.ID, input.Format)
		return map[string]any{"format": input.Format, "document": document}, err
	})
	importProperties := map[string]any{"format": text, "mode": text, "target_id": text, "expected_revision": integer, "accept_conflicts": boolean, "document": text}
	register("modbus_map_import_preview", "Validate a bounded map document and report conflicts without publication.", true, importProperties, []string{"format", "mode", "document"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input bench.ModbusMapImport
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.PreviewModbusMapImport(ctx, a, input)
	})
	register("modbus_map_import", "Atomically publish a fully validated map document.", false, importProperties, []string{"format", "mode", "document"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input bench.ModbusMapImport
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ImportModbusMap(ctx, a, input)
	})
	register("modbus_workspaces", "List shared saved workspace definitions.", true, map[string]any{}, nil, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct{}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusWorkspaces(ctx, a)
	})
	register("modbus_workspace_get", "Open current or pinned inert saved workspace metadata.", true, map[string]any{"workspace_id": text, "revision": integer}, []string{"workspace_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID       string `json:"workspace_id"`
			Revision int64  `json:"revision"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		if input.Revision > 0 {
			return options.Bench.ModbusWorkspaceRevision(ctx, a, input.ID, input.Revision)
		}
		return options.Bench.ModbusWorkspace(ctx, a, input.ID)
	})
	register("modbus_workspace_save", "Save a workspace using current pinned references.", false, map[string]any{"expected_revision": integer, "workspace": map[string]any{"type": "object"}}, []string{"expected_revision", "workspace"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			Expected  int64            `json:"expected_revision"`
			Workspace modbus.Workspace `json:"workspace"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SaveModbusWorkspace(ctx, a, input.Expected, input.Workspace)
	})
	register("modbus_workspace_duplicate", "Duplicate a readable workspace under a new ID.", false, map[string]any{"workspace_id": text, "new_id": text}, []string{"workspace_id", "new_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID    string `json:"workspace_id"`
			NewID string `json:"new_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.DuplicateModbusWorkspace(ctx, a, input.ID, input.NewID)
	})
	register("modbus_workspace_export", "Export inert workspace JSON metadata.", true, map[string]any{"workspace_id": text}, []string{"workspace_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"workspace_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		w, err := options.Bench.ModbusWorkspace(ctx, a, input.ID)
		if err != nil {
			return nil, err
		}
		document, err := json.Marshal(w)
		return map[string]any{"format": "json", "document": string(document)}, err
	})
	workspaceImportProperties := map[string]any{"mode": text, "target_id": text, "expected_revision": integer, "document": text}
	register("modbus_workspace_import_preview", "Validate an inert workspace JSON definition before publication.", true, workspaceImportProperties, []string{"mode", "document"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input bench.ModbusWorkspaceImport
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.PreviewModbusWorkspaceImport(ctx, a, input)
	})
	register("modbus_workspace_import", "Publish one validated workspace JSON definition.", false, workspaceImportProperties, []string{"mode", "document"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input bench.ModbusWorkspaceImport
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ImportModbusWorkspace(ctx, a, input)
	})
	register("modbus_status", "Read the current credential-bound Modbus endpoint session.", true, map[string]any{"endpoint_id": text}, []string{"endpoint_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"endpoint_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusSession(ctx, a, input.ID)
	})
	register("modbus_connect", "Queue a finite pinned read-plan admission job; use bench_get_job and the human approval surface for Ask.", false, map[string]any{"endpoint_id": text, "endpoint_revision": integer, "profile_id": text, "profile_revision": integer, "groups": map[string]any{"type": "array", "items": group}, "idempotency_key": text, "timeout_seconds": integer}, []string{"endpoint_id", "endpoint_revision", "profile_id", "profile_revision", "groups", "idempotency_key"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"endpoint_id"`
			bench.ModbusConnect
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ConnectModbus(ctx, a, input.ID, input.ModbusConnect)
	})
	register("modbus_test", "Queue a finite policy-controlled connection check with no Modbus PDU or retained session.", false, map[string]any{"endpoint_id": text, "endpoint_revision": integer, "profile_id": text, "profile_revision": integer, "idempotency_key": text, "timeout_seconds": integer}, []string{"endpoint_id", "endpoint_revision", "profile_id", "profile_revision", "idempotency_key"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"endpoint_id"`
			bench.ModbusConnect
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.TestModbusConnection(ctx, a, input.ID, input.ModbusConnect)
	})
	register("modbus_read", "Execute one bounded raw Modbus read in an owned live session.", false, map[string]any{"endpoint_id": text, "session_id": text, "request": request}, []string{"endpoint_id", "session_id", "request"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string              `json:"endpoint_id"`
			Session string              `json:"session_id"`
			Request bench.ModbusRequest `json:"request"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ReadModbus(ctx, a, input.ID, input.Session, input.Request)
	})
	register("modbus_write", "Reserve one exact raw write before transmission; delivery_unknown is terminal and never replayed.", false, map[string]any{"endpoint_id": text, "session_id": text, "idempotency_key": text, "request": request}, []string{"endpoint_id", "session_id", "idempotency_key", "request"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"endpoint_id"`
			bench.ModbusWrite
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.WriteModbus(ctx, a, input.ID, input.ModbusWrite)
	})
	register("modbus_preview", "Inspect exact raw PDU and request digest without transmitting.", true, map[string]any{"endpoint_id": text, "session_id": text, "idempotency_key": text, "request": request}, []string{"endpoint_id", "session_id", "idempotency_key", "request"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"endpoint_id"`
			bench.ModbusWrite
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.PreviewModbusWrite(ctx, a, input.ID, input.ModbusWrite)
	})
	register("modbus_poll", "Replace bounded read groups and set polling pause without changing display pause.", false, map[string]any{"endpoint_id": text, "session_id": text, "groups": map[string]any{"type": "array", "items": group}, "paused": boolean}, []string{"endpoint_id", "session_id", "groups", "paused"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string                  `json:"endpoint_id"`
			Session string                  `json:"session_id"`
			Groups  []bench.ModbusPollGroup `json:"groups"`
			Paused  bool                    `json:"paused"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SetModbusPolling(ctx, a, input.ID, input.Session, input.Groups, input.Paused)
	})
	register("modbus_display", "Pause or resume display attachment independently of polling.", false, map[string]any{"endpoint_id": text, "session_id": text, "paused": boolean}, []string{"endpoint_id", "session_id", "paused"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
			Paused  bool   `json:"paused"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SetModbusDisplayPaused(ctx, a, input.ID, input.Session, input.Paused)
	})
	register("modbus_events", "Read bounded live events after a cursor with explicit gap detection.", true, map[string]any{"endpoint_id": text, "session_id": text, "after": integer}, []string{"endpoint_id", "session_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
			After   int64  `json:"after"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ReadModbusEvents(ctx, a, input.ID, input.Session, input.After)
	})
	register("modbus_disconnect", "Stop an owned Modbus session and release its claim after runner cleanup.", false, map[string]any{"endpoint_id": text, "session_id": text}, []string{"endpoint_id", "session_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		err := options.Bench.DisconnectModbus(ctx, a, input.ID, input.Session)
		return map[string]bool{"ok": err == nil}, err
	})
	register("modbus_command", "Read one durable Modbus write outcome by command ID.", true, map[string]any{"command_id": text}, []string{"command_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"command_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusCommand(ctx, a, input.ID)
	})
	register("modbus_commands", "List one bounded page of durable writes for an authorized session.", true, map[string]any{"endpoint_id": text, "session_id": text, "offset": integer}, []string{"endpoint_id", "session_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
			Offset  int    `json:"offset"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ListModbusCommands(ctx, a, input.ID, input.Session, input.Offset)
	})
}
