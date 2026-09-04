package main

import (
	"net/http"
	"strings"
	"testing"
)

// The browser lists operations to reattach to a pending change after a
// reload; a script needs the same listing to find the operation it must
// confirm. The route takes no filters, so the command offers none.
func TestOperationListCallsTheListingRoute(t *testing.T) {
	t.Parallel()

	server, recorded := networkStub(t, http.StatusOK, map[string]any{
		"api_version": "nilo.http/1",
		"items":       []any{map[string]any{"id": "operation-1", "state": "awaiting_confirmation"}},
	})

	code, stdout, stderr := runNetwork([]string{
		"operation", "list", "--json", "--server", server.URL, "--token-file", tokenFile(t, "secret-token"),
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
	}
	if len(*recorded) != 1 {
		t.Fatalf("made %d requests, want 1", len(*recorded))
	}
	call := (*recorded)[0]
	if call.method != http.MethodGet || call.path != "/api/v1/network/operations" || call.rawQuery != "" {
		t.Fatalf("called %s %s?%s, want GET /api/v1/network/operations with no query", call.method, call.path, call.rawQuery)
	}
	if call.authorized != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", call.authorized)
	}
	// --json is the appliance's own document, one line, unindented.
	if !strings.Contains(stdout, `"operation-1"`) || strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
		t.Fatalf("--json output is not the appliance document: %q", stdout)
	}
}

// The listing route accepts no filters, so the command invents none: an extra
// argument or a filter flag is a usage error that reaches no server.
func TestOperationListTakesNoFiltersOrArguments(t *testing.T) {
	t.Parallel()

	server, recorded := networkStub(t, http.StatusOK, map[string]any{"api_version": "nilo.http/1", "items": []any{}})
	for name, extra := range map[string][]string{
		"extra argument": {"operation-1"},
		"filter flag":    {"--state", "awaiting_confirmation"},
	} {
		arguments := append([]string{"operation", "list"}, extra...)
		arguments = append(arguments, "--server", server.URL, "--token-file", tokenFile(t, "t"))
		if code, _, _ := runNetwork(arguments); code != 2 {
			t.Fatalf("%s: exit = %d, want 2", name, code)
		}
	}
	if len(*recorded) != 0 {
		t.Fatalf("a usage error still made %d requests", len(*recorded))
	}
}

func TestOperationListRefusalExitsOneWithTheApplianceCode(t *testing.T) {
	t.Parallel()

	server, _ := networkStub(t, http.StatusForbidden, map[string]any{"error": "forbidden"})
	code, _, stderr := runNetwork([]string{
		"operation", "list", "--server", server.URL, "--token-file", tokenFile(t, "narrowed"),
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "forbidden") {
		t.Fatalf("stderr does not carry the appliance's code: %s", stderr)
	}
}
