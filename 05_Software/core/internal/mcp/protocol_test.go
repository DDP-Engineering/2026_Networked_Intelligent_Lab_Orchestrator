package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/network"
)

type bearerTransport struct{ token string }

func (tr bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+tr.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestStreamableDiscoveryReadsAndRevocationWithRealToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	identity := identityServiceForTest(t)
	admin, err := identity.Bootstrap(ctx, "admin", []byte("mcp-test-administrator"))
	if err != nil {
		t.Fatal(err)
	}
	issued, err := identity.CreateToken(ctx, admin, "admin", "mcp", []auth.Permission{auth.PermissionNetworkRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, _ := network.NewService(network.ServiceOptions{Provider: network.NewSimulation(time.Now)})
	handler, err := NewHandler(HandlerOptions{Identity: identity, Network: service})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "network-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearerTransport{issued.Secret}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 3 {
		t.Fatalf("discovery exposed %d tools", len(list.Tools))
	}
	for _, tool := range list.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("%s lacks read-only metadata", tool.Name)
		}
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "network_get_wireguard_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result.StructuredContent)
	var status network.WireGuardStatus
	if err := json.Unmarshal(encoded, &status); err != nil || !status.Reported || status.MeasuredBy != network.SourceSimulation {
		t.Fatalf("tool did not read the shared WireGuard service: %s", encoded)
	}
	if err := identity.RevokeToken(ctx, admin, issued.Token.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ListTools(ctx, nil); err == nil {
		t.Fatal("revoked token kept MCP access")
	}
}
