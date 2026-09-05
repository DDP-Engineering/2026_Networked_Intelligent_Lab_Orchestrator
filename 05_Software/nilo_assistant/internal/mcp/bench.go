package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

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
	register("plugin_discover", "Discover supported digital analyzers on an enabled plugin. Does not expose PWM.", false, map[string]any{"plugin_id": text()}, []string{"plugin_id"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input struct {
			ID string `json:"plugin_id"`
		}
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.Discover(ctx, a, input.ID)
	})
	register("bench_submit", "Queue a finite capture or saved UART decode. Returns transaction_id immediately; poll bench_get_job for delayed replies. Reuse idempotency_key only with identical inputs. Approval requires a human GUI session.", false, map[string]any{"plugin_id": text(), "operation": map[string]any{"type": "string", "enum": []string{"capture", "decode_uart"}}, "device_id": text(), "parameters": map[string]any{"type": "object", "description": "capture: channels D0-D15, samplerate Hz, samples 1..1000000. decode_uart: capture_id transaction, channel, baudrate."}, "idempotency_key": text(), "timeout_seconds": map[string]any{"type": "integer", "minimum": 1, "maximum": 300}}, []string{"plugin_id", "operation", "parameters", "idempotency_key"}, func(ctx context.Context, a identity.Actor, raw json.RawMessage) (any, error) {
		var input bench.Submission
		if decodeBenchArguments(raw, &input) != nil {
			return nil, bench.ErrInvalid
		}
		return options.Bench.Submit(ctx, a, input)
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
