package mcp

import (
	"context"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/store/sqlite"
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
	if len(list.Tools) != 22 {
		t.Fatalf("tools: %d", len(list.Tools))
	}
	for _, expected := range []string{"camera_snapshot", "camera_get_controls", "camera_set_controls", "camera_reset_controls", "plugin_devices", "plugin_configuration", "plugin_configure", "plugin_device_configure", "plugin_device_rebind", "plugin_device_forget", "plugin_device_action"} {
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
		{"plugin_lifecycle", map[string]any{"plugin_id": "sigrok", "action": "install"}, true},
		{"network_get_status", map[string]any{}, true},
	} {
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: test.name, Arguments: test.input})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != test.denied {
			t.Fatalf("%s isError=%v", test.name, result.IsError)
		}
	}
	if err = identity.RevokeToken(ctx, admin, issued.Token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = session.ListTools(ctx, nil); err == nil {
		t.Fatal("revoked token retained access")
	}
}
