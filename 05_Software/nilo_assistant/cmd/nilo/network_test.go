package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordedRequest is one call the command made to the API.
type recordedRequest struct {
	method     string
	path       string
	rawQuery   string
	authorized string
	body       string
}

// networkStub answers the network routes and records every call, so a test can
// assert what the command asked for rather than what it printed.
func networkStub(t *testing.T, status int, body any) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var recorded []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		payload := new(bytes.Buffer)
		_, _ = payload.ReadFrom(request.Body)
		recorded = append(recorded, recordedRequest{
			method: request.Method, path: request.URL.Path, rawQuery: request.URL.RawQuery,
			authorized: request.Header.Get("Authorization"), body: payload.String(),
		})
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(status)
		_ = json.NewEncoder(response).Encode(body)
	}))
	t.Cleanup(server.Close)
	return server, &recorded
}

func tokenFile(t *testing.T, secret string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

// runNetwork runs one command and returns its exit code and streams.
func runNetwork(arguments []string) (int, string, string) {
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := runNetworkCommand(context.Background(), arguments, networkIO{stdout: stdout, stderr: stderr})
	return code, stdout.String(), stderr.String()
}

func TestNetworkStatusCallsTheAuthenticatedAPI(t *testing.T) {
	t.Parallel()

	server, recorded := networkStub(t, http.StatusOK, map[string]any{
		"api_version": "nilo.http/1", "measured_by": "simulation", "interfaces": []any{},
	})

	code, stdout, stderr := runNetwork([]string{
		"status", "--server", server.URL, "--token-file", tokenFile(t, "secret-token"),
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
	}
	if len(*recorded) != 1 {
		t.Fatalf("made %d requests, want 1", len(*recorded))
	}

	call := (*recorded)[0]
	if call.method != http.MethodGet || call.path != "/api/v1/network/status" {
		t.Fatalf("called %s %s", call.method, call.path)
	}
	// The command is an API client even locally: it authenticates like any other
	// caller rather than reaching past the boundary that authorizes it.
	if call.authorized != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", call.authorized)
	}
	if stdout == "" {
		t.Fatal("status printed nothing")
	}
}

func TestNetworkStatisticsSendsTheInterfaceAndPeriod(t *testing.T) {
	t.Parallel()

	server, recorded := networkStub(t, http.StatusOK, map[string]any{
		"api_version": "nilo.http/1", "interface_id": "sim-enp1s0", "period": "1h",
		"measured_by": "simulation", "stale": false, "points": []any{},
	})

	code, _, stderr := runNetwork([]string{
		"statistics", "--interface", "sim-enp1s0", "--period", "1h",
		"--server", server.URL, "--token-file", tokenFile(t, "secret-token"),
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
	}
	query := (*recorded)[0].rawQuery
	if !strings.Contains(query, "interface=sim-enp1s0") || !strings.Contains(query, "period=1h") {
		t.Fatalf("query = %q", query)
	}
}

// A token on a command line is visible to every other process and survives in
// shell history. There is no flag that accepts one.
func TestNoNetworkFlagAcceptsATokenValue(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("network.go")
	if err != nil {
		t.Fatalf("read network.go: %v", err)
	}
	for _, forbidden := range []string{`"token"`, `"bearer"`, `"api-token"`, `"secret"`} {
		if strings.Contains(string(source), "flags.String("+forbidden) {
			t.Fatalf("a flag named %s accepts a token value on the command line", forbidden)
		}
	}

	code, _, stderr := runNetwork([]string{"status", "--token", "secret-token"})
	if code != 2 {
		t.Fatalf("exit = %d, want 2 for an unknown flag", code)
	}
	if !strings.Contains(stderr, "token-file") {
		t.Fatalf("the refusal does not name the token-file option: %s", stderr)
	}
}

func TestNetworkRequiresAServerAndATokenFile(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		arguments []string
	}{
		{name: "no server", arguments: []string{"status", "--token-file", tokenFile(t, "t")}},
		{name: "no token file", arguments: []string{"status", "--server", "https://appliance.invalid"}},
		{name: "unknown subcommand", arguments: []string{"teleport"}},
		{name: "nothing at all", arguments: []string{}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if code, _, _ := runNetwork(test.arguments); code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
		})
	}
}

func TestAMissingTokenFileIsReportedRatherThanSentEmpty(t *testing.T) {
	t.Parallel()

	code, _, stderr := runNetwork([]string{
		"status", "--server", "https://appliance.invalid",
		"--token-file", filepath.Join(t.TempDir(), "absent"),
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "token") {
		t.Fatalf("stderr does not mention the token file: %s", stderr)
	}
}

// A refused request is the server's answer, not a usage mistake, so it exits 1
// and prints what the server said.
func TestARefusedRequestExitsOneAndReportsTheServerError(t *testing.T) {
	t.Parallel()

	server, _ := networkStub(t, http.StatusForbidden, map[string]any{"error": "forbidden"})
	code, _, stderr := runNetwork([]string{
		"status", "--server", server.URL, "--token-file", tokenFile(t, "narrowed"),
	})
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "forbidden") {
		t.Fatalf("stderr does not carry the server's error: %s", stderr)
	}
}

// An acknowledgement carries back the revision the preview returned, which is
// what binds it to that proposal and nothing else.
func TestAcknowledgeSendsTheRevisionItWasGiven(t *testing.T) {
	t.Parallel()

	server, recorded := networkStub(t, http.StatusAccepted, map[string]any{
		"api_version": "nilo.http/1", "id": "operation-1", "action": "interface.disable",
		"target": "enp1s0", "state": "awaiting_confirmation", "revision": "9f2c",
		"created_at": "2026-09-03T10:00:00Z",
	})

	code, _, stderr := runNetwork([]string{
		"operation", "acknowledge", "operation-1", "--revision", "9f2c",
		"--server", server.URL, "--token-file", tokenFile(t, "secret-token"),
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
	}
	call := (*recorded)[0]
	if call.method != http.MethodPost ||
		call.path != "/api/v1/network/operations/operation-1/acknowledge" {
		t.Fatalf("called %s %s", call.method, call.path)
	}
	if !strings.Contains(call.body, `"revision":"9f2c"`) {
		t.Fatalf("body = %s", call.body)
	}
}

// Acknowledging is not confirming. A connectivity change still enters its window
// and still needs a separate confirmation, so the two are separate commands.
func TestConfirmAndRollbackAreSeparateCommands(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ command, path string }{
		{command: "confirm", path: "/api/v1/network/operations/operation-1/confirm"},
		{command: "rollback", path: "/api/v1/network/operations/operation-1/rollback"},
	} {
		test := test
		t.Run(test.command, func(t *testing.T) {
			t.Parallel()
			server, recorded := networkStub(t, http.StatusOK, map[string]any{
				"api_version": "nilo.http/1", "id": "operation-1", "action": "interface.disable",
				"target": "enp1s0", "state": "confirmed", "revision": "9f2c",
				"created_at": "2026-09-03T10:00:00Z",
			})
			code, _, stderr := runNetwork([]string{
				"operation", test.command, "operation-1",
				"--server", server.URL, "--token-file", tokenFile(t, "secret-token"),
			})
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
			}
			if (*recorded)[0].path != test.path {
				t.Fatalf("called %s, want %s", (*recorded)[0].path, test.path)
			}
		})
	}
}

func TestJSONOutputIsTheServersOwnDocument(t *testing.T) {
	t.Parallel()

	body := map[string]any{
		"api_version": "nilo.http/1", "measured_by": "simulation", "interfaces": []any{},
	}
	server, _ := networkStub(t, http.StatusOK, body)

	code, stdout, _ := runNetwork([]string{
		"status", "--json", "--server", server.URL, "--token-file", tokenFile(t, "t"),
	})
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(stdout), &decoded); err != nil {
		t.Fatalf("--json did not print one JSON document: %v\n%s", err, stdout)
	}
	if decoded["measured_by"] != "simulation" {
		t.Fatalf("decoded = %v", decoded)
	}
}
