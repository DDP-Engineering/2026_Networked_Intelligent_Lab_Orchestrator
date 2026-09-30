package mcp

import (
	"context"
	"encoding/json"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/store/sqlite"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBenchToolsUseCurrentTokenScopes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	identity := identityServiceForTest(t)
	admin, err := identity.Bootstrap(ctx, "admin", []byte("mcp-bench-test-password"))
	if err != nil {
		t.Fatal(err)
	}
	issued, err := identity.CreateToken(ctx, admin, "admin", "bench read", []auth.Permission{auth.PermissionBenchRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	db, err := sqlite.Open(ctx, filepath.Join(root, "bench.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	writer, err := audit.NewWriter(audit.WriterOptions{Events: sqlite.NewAuditRepository(db), ApplianceID: "test", BootID: "test", Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := bench.New(bench.Options{Database: db, DataDir: root, Audit: writer, Simulation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	const ownLog = "924c289b-1b93-4c1e-8c3a-ff0866b15e71"
	const otherLog = "42be3946-cb34-4934-bdcd-13b17e756823"
	if err = os.MkdirAll(filepath.Join(root, "serial-logs"), 0700); err != nil {
		t.Fatal(err)
	}
	for id, owner := range map[string]string{ownLog: admin.ID, otherLog: "different-user"} {
		_, err = db.SQL().ExecContext(ctx, `INSERT INTO serial_logs(id,device_id,connection_id,owner_id,started_at,status) VALUES(?,?,?,?,?,'completed')`, id, "serial-1", "finished-connection", owner, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
	}
	logContent := []byte("{\"schema_version\":1,\"type\":\"header\"}\n{\"sequence\":1,\"direction\":\"rx\",\"data_base64\":\"AP8=\",\"byte_count\":2}\n")
	if err = os.WriteFile(filepath.Join(root, "serial-logs", ownLog+".jsonl"), logContent, 0600); err != nil {
		t.Fatal(err)
	}
	// Readers expose the committed prefix recorded in SQLite, just as the
	// production log writer publishes its size only after the bytes are saved.
	if _, err = db.SQL().ExecContext(ctx, `UPDATE serial_logs SET size_bytes=?,rx_bytes=2 WHERE id=?`, len(logContent), ownLog); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(HandlerOptions{Identity: identity, Network: readerStub{}, Bench: manager})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "bench-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearerTransport{issued.Secret}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 69 {
		t.Fatalf("tools: %d", len(list.Tools))
	}
	for _, expected := range []string{"modbus_endpoints", "modbus_endpoint_save", "modbus_profiles", "modbus_profile_save", "modbus_maps", "modbus_map_get", "modbus_map_save", "modbus_map_duplicate", "modbus_map_export", "modbus_map_import_preview", "modbus_map_import", "modbus_workspaces", "modbus_workspace_get", "modbus_workspace_save", "modbus_workspace_duplicate", "modbus_workspace_export", "modbus_workspace_import_preview", "modbus_workspace_import", "modbus_status", "modbus_connect", "modbus_read", "modbus_write", "modbus_preview", "modbus_poll", "modbus_display", "modbus_events", "modbus_disconnect", "modbus_command", "modbus_commands", "serial_status", "serial_connect", "serial_read", "serial_send", "serial_disconnect", "serial_signals", "serial_logging", "serial_logs", "serial_log_read", "serial_export", "serial_delete", "serial_retention", "serial_retention_configure", "camera_snapshot", "camera_get_controls", "camera_set_controls", "camera_reset_controls", "plugin_devices", "plugin_configuration", "plugin_configure", "plugin_device_configure", "plugin_device_rebind", "plugin_device_forget", "plugin_device_action"} {
		found := false
		for _, tool := range list.Tools {
			if tool.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s", expected)
		}
	}
	for _, test := range []struct {
		name   string
		input  map[string]any
		denied bool
	}{
		{"plugins_list", map[string]any{}, false},
		{"plugins_list", map[string]any{"unexpected": true}, true},
		{"bench_list_jobs", map[string]any{}, false},
		{"modbus_maps", map[string]any{}, false},
		{"modbus_map_import_preview", map[string]any{"format": "json", "mode": "create", "document": "{}"}, true},
		{"modbus_map_save", map[string]any{"expected_revision": 0, "map": map[string]any{"schema_version": 1, "id": "read-denied", "name": "Map", "points": []any{}}}, true},
		{"plugin_lifecycle", map[string]any{"plugin_id": "sigrok", "action": "install"}, true},
		{"network_get_status", map[string]any{}, true},
		{"camera_workspace_stop", map[string]any{"device_id": "cam-1", "action": "record_stop"}, true},
		{"camera_media_delete", map[string]any{"media_id": "other-capture"}, true},
		{"serial_send", map[string]any{"device_id": "serial-1", "connection_id": "connection-one", "idempotency_key": "send-once", "encoding": "utf8", "data": "test", "ending": "none"}, true},
		{"serial_disconnect", map[string]any{"device_id": "serial-1", "connection_id": "connection-one"}, true},
		{"serial_logging", map[string]any{"device_id": "serial-1", "connection_id": "connection-one", "action": "start"}, true},
		{"serial_retention_configure", map[string]any{"device_id": "serial-1", "revision": 1, "limits": map[string]any{"files": 100, "total_bytes": 2147483648, "file_bytes": 104857600}}, true},
		{"serial_export", map[string]any{"log_id": "not-authorized", "format": "jsonl"}, true},
		{"serial_export", map[string]any{"log_id": otherLog, "format": "jsonl"}, true},
		{"serial_log_read", map[string]any{"log_id": otherLog}, true},
		{"serial_logs", map[string]any{"device_id": "serial-1"}, false},
	} {
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: test.name, Arguments: test.input})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != test.denied {
			t.Fatalf("%s isError=%v", test.name, result.IsError)
		}
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "serial_export", Arguments: map[string]any{"log_id": ownLog, "format": "rx"}})
	if err != nil || result.IsError {
		t.Fatalf("authorized export: %v %+v", err, result)
	}
	var exported map[string]any
	if len(result.Content) != 1 {
		t.Fatal(result.Content)
	}
	content, ok := result.Content[0].(*sdk.TextContent)
	if !ok || json.Unmarshal([]byte(content.Text), &exported) != nil || exported["download_url"] != "/api/v1/serial-logs/"+ownLog+"/content?format=rx" {
		t.Fatalf("export metadata: %+v", result)
	}
	result, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "serial_log_read", Arguments: map[string]any{"log_id": ownLog, "after": 0}})
	if err != nil || result.IsError {
		t.Fatalf("authorized bounded log: %v %+v", err, result)
	}
	var page bench.SerialRead
	if json.Unmarshal([]byte(result.Content[0].(*sdk.TextContent).Text), &page) != nil || len(page.Events) != 1 || page.Events[0].DataBase64 != "AP8=" || page.NextCursor != 1 {
		t.Fatalf("page=%+v", page)
	}
	for _, input := range []map[string]any{
		{"device_id": "serial-1", "connection_id": "connection-one", "wait_ms": 10001},
		{"device_id": "serial-1", "connection_id": "connection-one", "after": -1},
		{"device_id": "serial-1", "connection_id": "connection-one", "unexpected": true},
	} {
		result, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "serial_read", Arguments: input})
		if err == nil && !result.IsError {
			t.Fatalf("invalid bounded-read arguments accepted: %+v", input)
		}
	}
	writerToken, err := identity.CreateToken(ctx, admin, "admin", "modbus definitions", []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}, nil)
	if err != nil {
		t.Fatal(err)
	}
	writerClient := sdk.NewClient(&sdk.Implementation{Name: "modbus-definition-test", Version: "1"}, nil)
	writerSession, err := writerClient.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearerTransport{writerToken.Secret}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writerSession.Close()
	for _, call := range []struct {
		name      string
		arguments map[string]any
	}{
		{"modbus_map_save", map[string]any{"expected_revision": 0, "map": map[string]any{"schema_version": 1, "id": "mcp-map", "name": "MCP Map", "points": []any{}}}},
		{"modbus_map_get", map[string]any{"map_id": "mcp-map"}},
		{"modbus_map_duplicate", map[string]any{"map_id": "mcp-map", "new_id": "mcp-copy"}},
		{"modbus_map_export", map[string]any{"map_id": "mcp-copy", "format": "json"}},
	} {
		result, callErr := writerSession.CallTool(ctx, &sdk.CallToolParams{Name: call.name, Arguments: call.arguments})
		if callErr != nil || result.IsError {
			t.Fatalf("%s: %v %+v", call.name, callErr, result)
		}
	}
	if err = identity.RevokeToken(ctx, admin, issued.Token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = session.ListTools(ctx, nil); err == nil {
		t.Fatal("revoked token retained access")
	}
}
