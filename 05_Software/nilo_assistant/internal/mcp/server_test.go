package mcp

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nilo.local/nilo/internal/network"
)

// readerStub answers the two reads with fixed values.
type readerStub struct{}

func (readerStub) WireGuard(context.Context) (network.WireGuardStatus, error) {
	return network.WireGuardStatus{MeasuredBy: network.SourceSimulation}, nil
}

func (readerStub) Status(context.Context) (network.Status, error) {
	return network.Status{MeasuredBy: network.SourceSimulation}, nil
}

func (readerStub) Statistics(
	context.Context, string, network.Period,
) (network.Statistics, error) {
	return network.Statistics{MeasuredBy: network.SourceSimulation}, nil
}

func TestTheEndpointOffersExactlyThreeReadTools(t *testing.T) {
	t.Parallel()

	if len(ToolNames) != 3 {
		t.Fatalf("ToolNames has %d entries, want 3: %v", len(ToolNames), ToolNames)
	}
	want := map[string]bool{
		"network_get_status": true, "network_get_statistics": true,
		"network_get_wireguard_status": true,
	}
	for _, name := range ToolNames {
		if !want[name] {
			t.Fatalf("unexpected tool %q", name)
		}
		// Every name reads. A tool whose name implies a change would be one
		// somebody later gave a mutating implementation.
		for _, mutating := range []string{"set_", "enable", "disable", "install", "import", "remove", "activate"} {
			if strings.Contains(name, mutating) {
				t.Fatalf("tool %q names a mutation", name)
			}
		}
	}
}

// The read-only guarantee is structural: this package is given an interface
// with two read methods, so there is no mutating method here to call. A test
// over the source keeps that true as the file changes.
func TestTheEndpointHoldsNoMutatingCapability(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "server.go", nil, 0)
	if err != nil {
		t.Fatalf("parse server.go: %v", err)
	}

	var methods []string
	ast.Inspect(parsed, func(node ast.Node) bool {
		declaration, ok := node.(*ast.TypeSpec)
		if !ok || declaration.Name.Name != "reader" {
			return true
		}
		iface, ok := declaration.Type.(*ast.InterfaceType)
		if !ok {
			return true
		}
		for _, field := range iface.Methods.List {
			for _, name := range field.Names {
				methods = append(methods, name.Name)
			}
		}
		return false
	})

	if len(methods) != 3 {
		t.Fatalf("the reader interface exposes %v, want exactly Status, Statistics and WireGuard", methods)
	}
	for _, name := range methods {
		if name != "Status" && name != "Statistics" && name != "WireGuard" {
			t.Fatalf("the reader interface exposes %q, which is not a read", name)
		}
	}
}

func TestEveryUnauthorizedRequestIsRefusedWithoutNetworkData(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(HandlerOptions{
		Identity: identityServiceForTest(t), Network: readerStub{},
		AllowedOrigins: []string{"https://appliance.local"},
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	for _, test := range []struct {
		name       string
		authorize  string
		origin     string
		wantStatus int
	}{
		{name: "no credential", wantStatus: http.StatusUnauthorized},
		{name: "an unknown token", authorize: "Bearer not-a-real-token", wantStatus: http.StatusUnauthorized},
		{name: "an empty bearer", authorize: "Bearer ", wantStatus: http.StatusUnauthorized},
		{name: "a credential that is not a bearer", authorize: "Basic abc", wantStatus: http.StatusUnauthorized},
		{
			name: "an origin this endpoint does not serve", origin: "https://elsewhere.invalid",
			authorize: "Bearer not-a-real-token", wantStatus: http.StatusForbidden,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
			if test.authorize != "" {
				request.Header.Set("Authorization", test.authorize)
			}
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			// Nothing about the appliance's network may appear in a refusal.
			for _, leaked := range []string{"simulation", "interfaces", "measured_by"} {
				if strings.Contains(response.Body.String(), leaked) {
					t.Fatalf("a refusal carried network data: %s", response.Body.String())
				}
			}
		})
	}
}

func TestNewHandlerRequiresItsCollaborators(t *testing.T) {
	t.Parallel()

	if _, err := NewHandler(HandlerOptions{Network: readerStub{}}); err == nil {
		t.Fatal("NewHandler accepted options with no identity service")
	}
	if _, err := NewHandler(HandlerOptions{Identity: identityServiceForTest(t)}); err == nil {
		t.Fatal("NewHandler accepted options with no network service")
	}
}
