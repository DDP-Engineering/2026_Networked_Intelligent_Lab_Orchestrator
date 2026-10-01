package mcp

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
)

func registerModbusTools(server *sdk.Server, options HandlerOptions) {
	text := map[string]any{"type": "string", "minLength": 1}
	integer := map[string]any{"type": "integer", "minimum": 0}
	boolean := map[string]any{"type": "boolean"}
	request := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"function": integer, "unit": integer, "address": integer, "count": integer, "values": map[string]any{"type": "array", "items": integer}, "and_mask": integer, "or_mask": integer, "read_address": integer, "read_count": integer}, "required": []string{"function", "unit", "address", "count"}}
	group := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"id": text, "interval_ms": integer, "timeout_ms": integer, "stop_on_error": boolean, "requests": map[string]any{"type": "array", "items": request}}, "required": []string{"id", "interval_ms", "requests"}}
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
	register("modbus_map_decode", "Decode exact words using one pinned map point without I/O.", true, map[string]any{"map_id": text, "revision": integer, "point_id": text, "words": map[string]any{"type": "array", "items": integer}}, []string{"map_id", "revision", "point_id", "words"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			MapID    string   `json:"map_id"`
			Revision int64    `json:"revision"`
			PointID  string   `json:"point_id"`
			Words    []uint16 `json:"words"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.DecodeModbusPoint(ctx, a, input.MapID, input.Revision, input.PointID, input.Words)
	})
	register("modbus_map_encode", "Validate exact raw words for one pinned engineering value without I/O.", true, map[string]any{"map_id": text, "revision": integer, "point_id": text, "value": map[string]any{}}, []string{"map_id", "revision", "point_id", "value"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			MapID    string          `json:"map_id"`
			Revision int64           `json:"revision"`
			PointID  string          `json:"point_id"`
			Value    json.RawMessage `json:"value"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.EncodeModbusPoint(ctx, a, input.MapID, input.Revision, input.PointID, input.Value)
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
	register("modbus_recording_start", "Start explicit retained logging for an owned live session.", false, map[string]any{"endpoint_id": text, "session_id": text, "mode": text, "sampling": text, "interval_ms": integer, "name": map[string]any{"type": "string"}, "tags": map[string]any{"type": "array", "items": text}}, []string{"endpoint_id", "session_id", "mode", "sampling"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
			bench.ModbusRecordingOptions
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.StartModbusRecording(ctx, a, input.ID, input.Session, input.ModbusRecordingOptions)
	})
	register("modbus_recording_stop", "Stop and finalize owned Modbus logging without stopping polling.", false, map[string]any{"endpoint_id": text, "session_id": text}, []string{"endpoint_id", "session_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.StopModbusRecording(ctx, a, input.ID, input.Session)
	})
	register("modbus_recordings", "Find authorized saved Modbus recording metadata by name, tag, endpoint, and date.", true, map[string]any{"before": text, "name": text, "tag": text, "endpoint_id": text, "started_from": text, "started_through": text}, nil, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input bench.ModbusRecordingSearch
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.FindModbusRecordings(ctx, a, input)
	})
	register("modbus_recording_get", "Read one authorized saved Modbus recording's metadata.", true, map[string]any{"recording_id": text}, []string{"recording_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"recording_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusRecordingByID(ctx, a, input.ID)
	})
	register("modbus_recording_find", "Read an indexed bounded page of authorized retained events.", true, map[string]any{"recording_id": text, "after": integer, "kind": text, "unit": integer, "function": integer, "address": integer, "outcome": text, "point_id": text, "text": text, "hex": text, "utc_from": text, "utc_through": text}, []string{"recording_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"recording_id"`
			bench.ModbusRecordingFind
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.FindModbusRecordingEvents(ctx, a, input.ID, input.ModbusRecordingFind)
	})
	register("modbus_recording_metadata", "Rename or tag an authorized Modbus recording without changing evidence.", false, map[string]any{"recording_id": text, "name": map[string]any{"type": "string"}, "tags": map[string]any{"type": "array", "items": text}}, []string{"recording_id", "name", "tags"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID   string   `json:"recording_id"`
			Name string   `json:"name"`
			Tags []string `json:"tags"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.UpdateModbusRecordingMetadata(ctx, a, input.ID, input.Name, input.Tags)
	})
	register("modbus_recording_export", "Authorize a streamed export and return its relative authenticated download URL.", true, map[string]any{"recording_id": text, "format": text, "after": integer, "through": integer}, []string{"recording_id", "format"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"recording_id"`
			Format  string `json:"format"`
			After   int64  `json:"after"`
			Through int64  `json:"through"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		if input.Format != "jsonl" && input.Format != "csv" && input.Format != "text" && input.Format != "bundle" || input.After < 0 || input.Through < 0 {
			return nil, bench.ErrInvalid
		}
		if _, err := options.Bench.ModbusRecordingByID(ctx, a, input.ID); err != nil {
			return nil, err
		}
		query := url.Values{"format": {input.Format}, "after": {strconv.FormatInt(input.After, 10)}, "through": {strconv.FormatInt(input.Through, 10)}}
		return map[string]any{"recording_id": input.ID, "download_url": "/api/v1/modbus/recordings/" + url.PathEscape(input.ID) + "/export?" + query.Encode(), "authentication": "Use the same authorized API token; download authorization is checked again."}, nil
	})
	register("modbus_recording_delete", "Delete a stopped non-leased authorized Modbus recording.", false, map[string]any{"recording_id": text}, []string{"recording_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"recording_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		err := options.Bench.DeleteModbusRecording(ctx, a, input.ID)
		return map[string]bool{"ok": err == nil}, err
	})
	register("modbus_connect", "Queue a finite pinned read-plan admission job; use bench_get_job and the human approval surface for Ask.", false, map[string]any{"endpoint_id": text, "endpoint_revision": integer, "profile_id": text, "profile_revision": integer, "workspace_id": text, "workspace_revision": integer, "groups": map[string]any{"type": "array", "items": group}, "idempotency_key": text, "timeout_seconds": integer}, []string{"endpoint_id", "endpoint_revision", "profile_id", "profile_revision", "groups", "idempotency_key"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
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
	scanFields := map[string]any{"endpoint_id": text, "session_id": text, "function": integer, "unit_from": integer, "unit_to": integer, "address_from": integer, "address_to": integer, "rate_per_second": integer, "deadline_seconds": integer}
	register("modbus_scan_start", "Start one explicit finite paced read scan on an owned live session.", false, scanFields, []string{"endpoint_id", "session_id", "function", "unit_from", "unit_to", "address_from", "address_to", "rate_per_second", "deadline_seconds"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"endpoint_id"`
			bench.ModbusScanPlan
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.StartModbusScan(ctx, a, input.ID, input.ModbusScanPlan)
	})
	register("modbus_scan_status", "Read current scan progress and settled probes.", true, map[string]any{"endpoint_id": text, "session_id": text}, []string{"endpoint_id", "session_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusScan(ctx, a, input.ID, input.Session)
	})
	register("modbus_scan_cancel", "Cancel future probes and the in-flight read of an owned scan.", false, map[string]any{"endpoint_id": text, "session_id": text}, []string{"endpoint_id", "session_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.CancelModbusScan(ctx, a, input.ID, input.Session)
	})
	register("modbus_scan_results", "Read a bounded page of settled scan probes, including failures.", true, map[string]any{"endpoint_id": text, "session_id": text, "offset": integer}, []string{"endpoint_id", "session_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
			Offset  int    `json:"offset"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ModbusScanResults(ctx, a, input.ID, input.Session, input.Offset)
	})
	register("modbus_scan_export", "Authorize a CSV or JSONL scan export and return its authenticated download URL.", true, map[string]any{"endpoint_id": text, "session_id": text, "format": text}, []string{"endpoint_id", "session_id", "format"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
			Format  string `json:"format"`
		}
		if decodeBenchArguments(raw, &input) != nil || input.Format != "csv" && input.Format != "jsonl" {
			return nil, bench.ErrInvalid
		}
		if _, err := options.Bench.ModbusScan(ctx, a, input.ID, input.Session); err != nil {
			return nil, err
		}
		query := url.Values{"session_id": {input.Session}, "format": {input.Format}}
		return map[string]any{"download_url": "/api/v1/modbus/endpoints/" + url.PathEscape(input.ID) + "/scans/export?" + query.Encode(), "authentication": "Use the same authorized API token; download authorization is checked again."}, nil
	})
	register("modbus_diagnostic", "Execute one typed read-only Modbus identity or serial diagnostic.", false, map[string]any{"endpoint_id": text, "session_id": text, "kind": text, "unit": integer}, []string{"endpoint_id", "session_id", "kind", "unit"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"endpoint_id"`
			bench.ModbusDiagnosticRequest
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ReadModbusDiagnostic(ctx, a, input.ID, input.ModbusDiagnosticRequest)
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
	register("modbus_poll_group", "Pause or resume one admitted poll group independently.", false, map[string]any{"endpoint_id": text, "session_id": text, "group_id": text, "paused": boolean}, []string{"endpoint_id", "session_id", "group_id", "paused"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID      string `json:"endpoint_id"`
			Session string `json:"session_id"`
			Group   string `json:"group_id"`
			Paused  bool   `json:"paused"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SetModbusPollGroupPaused(ctx, a, input.ID, input.Session, input.Group, input.Paused)
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
