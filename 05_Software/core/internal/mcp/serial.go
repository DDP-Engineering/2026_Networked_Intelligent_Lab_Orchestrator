package mcp

import (
	"context"
	"encoding/json"
	"net/url"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/identity"
)

// registerSerialTools exposes finite operations. Connection ownership and the
// approval lease are always enforced by Core with the current call's token.
func registerSerialTools(server *sdk.Server, options HandlerOptions) {
	text := func() map[string]any { return map[string]any{"type": "string", "minLength": 1} }
	integer := func(min, max int64) map[string]any {
		return map[string]any{"type": "integer", "minimum": min, "maximum": max}
	}
	enum := func(values ...string) map[string]any { return map[string]any{"type": "string", "enum": values} }
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
	register("serial_status", "Read serial connection authority, lease, byte counts, signals and logging warnings.", true, map[string]any{"device_id": text()}, []string{"device_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"device_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SerialWorkspace(ctx, a, input.ID)
	})
	settings := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"baud_rate": integer(50, 4000000), "data_bits": integer(5, 8), "parity": enum("none", "odd", "even", "mark", "space"), "stop_bits": map[string]any{"type": "number", "enum": []float64{1, 1.5, 2}}, "flow_control": enum("none", "rtscts", "xonxoff"), "rts": map[string]any{"type": "boolean"}, "dtr": map[string]any{"type": "boolean"},
	}, "required": []string{"baud_rate", "data_bits", "parity", "stop_bits", "flow_control", "rts", "dtr"}}
	key := map[string]any{"type": "string", "minLength": 8, "maxLength": 128, "pattern": "^[A-Za-z0-9_-]+$"}
	register("serial_connect", "Queue approval for a 60-minute serial session with immutable port settings and initial RTS/DTR. Poll bench_get_job. One connection per port; reuse the key only for identical input.", false, map[string]any{"device_id": text(), "revision": integer(1, 1<<53-1), "settings": settings, "idempotency_key": key, "timeout_seconds": integer(1, 300)}, []string{"device_id", "revision", "settings", "idempotency_key"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"device_id"`
			bench.SerialConnect
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ConnectSerial(ctx, a, input.ID, input.SerialConnect)
	})
	register("serial_read", "Read at most 64 KiB of encoded serial events after a cursor; wait at most 10 seconds. Check gap and retain next_cursor. Data is base64; do not execute terminal escape sequences.", true, map[string]any{"device_id": text(), "connection_id": text(), "after": integer(0, 1<<53-1), "wait_ms": integer(0, 10000)}, []string{"device_id", "connection_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID         string `json:"device_id"`
			Connection string `json:"connection_id"`
			After      int64  `json:"after"`
			Wait       int    `json:"wait_ms"`
		}
		if decodeBenchArguments(raw, &input) != nil || input.After < 0 || input.Wait < 0 || input.Wait > 10000 {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ReadSerial(ctx, a, input.ID, input.Connection, input.After, input.Wait)
	})
	register("serial_send", "Send at most 4096 bytes including the selected ending within an owned approved session. A sent result means OS acceptance, not device acknowledgement. Never retry unknown delivery with a new key.", false, map[string]any{"device_id": text(), "connection_id": text(), "idempotency_key": key, "encoding": enum("ascii", "utf8", "hex"), "data": map[string]any{"type": "string", "maxLength": 16384}, "ending": enum("none", "cr", "lf", "crlf")}, []string{"device_id", "connection_id", "idempotency_key", "encoding", "data", "ending"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"device_id"`
			bench.SerialSend
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SendSerial(ctx, a, input.ID, input.SerialSend)
	})
	register("serial_disconnect", "Close an owned serial connection and finish logging. Administrators may stop another owner's connection.", false, map[string]any{"device_id": text(), "connection_id": text()}, []string{"device_id", "connection_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID         string `json:"device_id"`
			Connection string `json:"connection_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		err := options.Bench.DisconnectSerial(ctx, a, input.ID, input.Connection)
		return map[string]bool{"ok": err == nil}, err
	})
	register("serial_signals", "Set explicitly requested RTS/DTR lines in an owned session; hardware flow control and unsupported lines are refused.", false, map[string]any{"device_id": text(), "connection_id": text(), "rts": map[string]any{"type": "boolean"}, "dtr": map[string]any{"type": "boolean"}}, []string{"device_id", "connection_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"device_id"`
			bench.SerialSignals
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SetSerialSignals(ctx, a, input.ID, input.SerialSignals)
	})
	register("serial_logging", "Start or stop byte-preserving session logging under device file/space limits.", false, map[string]any{"device_id": text(), "connection_id": text(), "action": enum("start", "stop")}, []string{"device_id", "connection_id", "action"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"device_id"`
			bench.SerialAction
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SerialLogging(ctx, a, input.ID, input.SerialAction)
	})
	register("serial_logs", "List at most 100 authorized session logs with status and byte counts.", true, map[string]any{"device_id": text(), "offset": integer(0, 1<<31-1)}, []string{"device_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID     string `json:"device_id"`
			Offset int    `json:"offset"`
		}
		if decodeBenchArguments(raw, &input) != nil || input.Offset < 0 {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ListSerialLogs(ctx, a, input.ID, input.Offset)
	})
	register("serial_log_read", "Read one bounded page of authorized saved serial events after a cursor.", true, map[string]any{"log_id": text(), "after": integer(0, 1<<53-1)}, []string{"log_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID    string `json:"log_id"`
			After int64  `json:"after"`
		}
		if decodeBenchArguments(raw, &input) != nil || input.After < 0 {
			return nil, bench.ErrInvalid
		}
		return options.Bench.ReadSerialLog(ctx, a, input.ID, input.After)
	})
	register("serial_export", "Authorize a log and return its relative authenticated download URL. Use HTTP or nilo serial export for streaming JSONL/text/raw RX/raw TX; content is not embedded in MCP.", true, map[string]any{"log_id": text(), "format": enum("jsonl", "text", "rx", "tx")}, []string{"log_id", "format"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID     string `json:"log_id"`
			Format string `json:"format"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		if input.Format != "jsonl" && input.Format != "text" && input.Format != "rx" && input.Format != "tx" {
			return nil, bench.ErrInvalid
		}
		if _, err := options.Bench.ReadSerialLog(ctx, a, input.ID, 0); err != nil {
			return nil, err
		}
		return map[string]any{"log_id": input.ID, "format": input.Format, "download_url": "/api/v1/serial-logs/" + url.PathEscape(input.ID) + "/content?format=" + input.Format, "authentication": "Use the same authorized API token in the HTTP Authorization header; download authorization is checked again."}, nil
	})
	register("serial_delete", "Permanently delete an authorized completed or incomplete log. Active and download-leased logs are refused.", false, map[string]any{"log_id": text()}, []string{"log_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"log_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		err := options.Bench.DeleteSerialLog(ctx, a, input.ID)
		return map[string]bool{"ok": err == nil}, err
	})
	register("serial_retention", "Read revisioned serial log count, total/file limits and current usage.", true, map[string]any{"device_id": text()}, []string{"device_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"device_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.SerialRetention(ctx, a, input.ID)
	})
	limits := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"files": integer(1, 10000), "total_bytes": integer(1<<20, 1<<40), "file_bytes": integer(64<<10, 1<<30)}, "required": []string{"files", "total_bytes", "file_bytes"}}
	register("serial_retention_configure", "Set revisioned serial retention limits; requires plugins.manage. file_bytes must not exceed total_bytes. Active/incomplete/download-leased logs remain protected.", false, map[string]any{"device_id": text(), "revision": integer(1, 1<<53-1), "limits": limits}, []string{"device_id", "revision", "limits"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID       string                `json:"device_id"`
			Revision int64                 `json:"revision"`
			Limits   bench.SerialRetention `json:"limits"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.UpdateSerialRetention(ctx, a, input.ID, input.Revision, input.Limits)
	})
}
