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
	"sync"
	"testing"
	"time"
)

// The remote administration commands are proven against a recording stub that
// answers the identity administration routes, so a test asserts what the
// command asked the appliance for rather than what it printed.

const (
	remoteAdminSecret     = "admin-bearer-secret"
	remoteCreatedPassword = "remote-created-0001"
	remoteIssuedSecret    = "nilo_issued-secret-value"
)

type adminStubAnswer struct {
	status int
	body   any
}

type adminStub struct {
	server  *httptest.Server
	mutex   sync.Mutex
	calls   []recordedRequest
	answers map[string]adminStubAnswer
}

func stubUserListing() map[string]any {
	return map[string]any{
		"api_version": "nilo.http/1",
		"items": []any{
			map[string]any{"id": "user-1", "username": "admin", "role": "administrator", "enabled": true, "created_at": "2026-09-01T09:00:00Z"},
			map[string]any{"id": "user-2", "username": "operator", "role": "operator", "enabled": true, "created_at": "2026-09-02T09:00:00Z"},
		},
	}
}

func stubTokenListing() map[string]any {
	return map[string]any{
		"api_version": "nilo.http/1",
		"items": []any{
			map[string]any{"id": "token-1", "name": "ci", "owner": "operator", "scopes": []string{"status.read"}, "created_at": "2026-09-02T09:00:00Z", "expires_at": nil, "revoked": false, "revoked_at": nil},
			map[string]any{"id": "token-2", "name": "old", "owner": "admin", "scopes": []string{}, "created_at": "2026-08-02T09:00:00Z", "expires_at": "2026-09-01T00:00:00Z", "revoked": true, "revoked_at": "2026-08-20T00:00:00Z"},
		},
	}
}

func newAdminStub(t *testing.T) *adminStub {
	t.Helper()
	stub := &adminStub{answers: map[string]adminStubAnswer{}}
	stub.server = httptest.NewServer(http.HandlerFunc(stub.serve))
	t.Cleanup(stub.server.Close)
	stub.answer("GET /api/v1/users", http.StatusOK, stubUserListing())
	stub.answer("GET /api/v1/tokens", http.StatusOK, stubTokenListing())
	stub.answer("POST /api/v1/users", http.StatusCreated, map[string]any{
		"api_version": "nilo.http/1", "id": "user-3", "username": "newcomer", "role": "operator",
		"enabled": true, "created_at": "2026-09-04T09:00:00Z",
	})
	stub.answer("POST /api/v1/tokens", http.StatusCreated, map[string]any{
		"api_version": "nilo.http/1",
		"token": map[string]any{
			"id": "token-9", "name": "ci", "owner": "operator", "scopes": []string{"inventory.read", "status.read"},
			"created_at": "2026-09-04T09:00:00Z", "expires_at": "2026-10-04T09:00:00Z", "revoked": false, "revoked_at": nil,
		},
		"secret": remoteIssuedSecret,
	})
	return stub
}

func (stub *adminStub) answer(route string, status int, body any) {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	stub.answers[route] = adminStubAnswer{status: status, body: body}
}

func (stub *adminStub) serve(response http.ResponseWriter, request *http.Request) {
	payload := new(bytes.Buffer)
	_, _ = payload.ReadFrom(request.Body)
	stub.mutex.Lock()
	stub.calls = append(stub.calls, recordedRequest{
		method: request.Method, path: request.URL.Path, rawQuery: request.URL.RawQuery,
		authorized: request.Header.Get("Authorization"), body: payload.String(),
	})
	answer, known := stub.answers[request.Method+" "+request.URL.Path]
	stub.mutex.Unlock()
	if !known {
		// Every other change the API offers answers 204 No Content.
		switch {
		case request.Method == http.MethodPut, request.Method == http.MethodDelete,
			request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/revoke"):
			response.WriteHeader(http.StatusNoContent)
		default:
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(response).Encode(map[string]string{"error": "stub_route_unknown"})
		}
		return
	}
	if answer.body == nil {
		response.WriteHeader(answer.status)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(answer.status)
	_ = json.NewEncoder(response).Encode(answer.body)
}

func (stub *adminStub) requests() []recordedRequest {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	return append([]recordedRequest(nil), stub.calls...)
}

// remote appends the remote-mode options to a command.
func (stub *adminStub) remote(t *testing.T, arguments ...string) []string {
	t.Helper()
	return append(arguments, "--server", stub.server.URL, "--token-file", tokenFile(t, remoteAdminSecret))
}

// runAdminInProcess runs one admin command with captured streams.
func runAdminInProcess(arguments []string, stdin string, interactive bool) (int, string, string) {
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := runAdminCommand(context.Background(), arguments, adminIO{
		stdin: strings.NewReader(stdin), stdout: stdout, stderr: stderr, interactive: interactive,
	})
	return code, stdout.String(), stderr.String()
}

func assertRequests(t *testing.T, got []recordedRequest, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("made %d requests %v, want %d %v", len(got), describeRequests(got), len(want), want)
	}
	for index, expected := range want {
		if actual := got[index].method + " " + got[index].path; actual != expected {
			t.Fatalf("request %d = %s, want %s", index, actual, expected)
		}
		if got[index].authorized != "Bearer "+remoteAdminSecret {
			t.Fatalf("request %d Authorization = %q", index, got[index].authorized)
		}
	}
}

func describeRequests(requests []recordedRequest) []string {
	described := make([]string, 0, len(requests))
	for _, request := range requests {
		described = append(described, request.method+" "+request.path)
	}
	return described
}

func TestRemoteUserCommandsCallTheAdministrationRoutes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		arguments []string
		stdin     string
		requests  []string
		body      []string
	}{
		{name: "list", arguments: []string{"user", "list"}, requests: []string{"GET /api/v1/users"}},
		{
			name:      "create",
			arguments: []string{"user", "create", "--username", "newcomer", "--role", "operator"},
			stdin:     password(remoteCreatedPassword),
			requests:  []string{"POST /api/v1/users"},
			body:      []string{`"username":"newcomer"`, `"role":"operator"`, `"password":"` + remoteCreatedPassword + `"`},
		},
		{
			name:      "set-role",
			arguments: []string{"user", "set-role", "--username", "operator", "--role", "viewer"},
			requests:  []string{"PUT /api/v1/users/operator/role", "GET /api/v1/users"},
			body:      []string{`"role":"viewer"`},
		},
		{
			name:      "enable",
			arguments: []string{"user", "enable", "--username", "operator"},
			requests:  []string{"PUT /api/v1/users/operator/enabled", "GET /api/v1/users"},
			body:      []string{`"enabled":true`},
		},
		{
			name:      "disable",
			arguments: []string{"user", "disable", "--username", "operator"},
			requests:  []string{"PUT /api/v1/users/operator/enabled", "GET /api/v1/users"},
			body:      []string{`"enabled":false`},
		},
		{
			name:      "reset-password",
			arguments: []string{"user", "reset-password", "--username", "operator"},
			stdin:     password(remoteCreatedPassword),
			requests:  []string{"PUT /api/v1/users/operator/password", "GET /api/v1/users"},
			body:      []string{`"password":"` + remoteCreatedPassword + `"`},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := newAdminStub(t)
			code, stdout, stderr := runAdminInProcess(stub.remote(t, test.arguments...), test.stdin, false)
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
			}
			requests := stub.requests()
			assertRequests(t, requests, test.requests...)
			for _, fragment := range test.body {
				if !strings.Contains(requests[0].body, fragment) {
					t.Fatalf("request body %q lacks %s", requests[0].body, fragment)
				}
			}
			// The password goes to the appliance and nowhere else.
			if strings.Contains(stdout+stderr, remoteCreatedPassword) {
				t.Fatalf("%s printed the password: %s%s", test.name, stdout, stderr)
			}
			if stdout == "" {
				t.Fatalf("%s printed nothing on standard output", test.name)
			}
		})
	}
}

func TestRemoteTokenCommandsCallTheAdministrationRoutes(t *testing.T) {
	t.Parallel()

	t.Run("list", func(t *testing.T) {
		t.Parallel()
		stub := newAdminStub(t)
		code, stdout, stderr := runAdminInProcess(stub.remote(t, "token", "list"), "", false)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
		}
		assertRequests(t, stub.requests(), "GET /api/v1/tokens")
		if !strings.Contains(stdout, "token-1") || !strings.Contains(stdout, "revoked") {
			t.Fatalf("listing does not render the tokens and their state: %s", stdout)
		}
	})

	t.Run("create discloses the secret once with a warning", func(t *testing.T) {
		t.Parallel()
		stub := newAdminStub(t)
		code, stdout, stderr := runAdminInProcess(stub.remote(t,
			"token", "create", "--owner", "operator", "--name", "ci",
			"--scopes", "status.read, inventory.read", "--expires-in", "720h",
		), "", false)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
		}
		requests := stub.requests()
		assertRequests(t, requests, "POST /api/v1/tokens")
		var body struct {
			Owner     string   `json:"owner"`
			Name      string   `json:"name"`
			Scopes    []string `json:"scopes"`
			ExpiresAt string   `json:"expires_at"`
		}
		if err := json.Unmarshal([]byte(requests[0].body), &body); err != nil {
			t.Fatalf("decode request body %q: %v", requests[0].body, err)
		}
		if body.Owner != "operator" || body.Name != "ci" || len(body.Scopes) != 2 || body.Scopes[1] != "inventory.read" {
			t.Fatalf("request body = %#v", body)
		}
		expiry, err := time.Parse(time.RFC3339, body.ExpiresAt)
		if err != nil || expiry.Before(time.Now().Add(700*time.Hour)) {
			t.Fatalf("expires_at = %q, want an RFC 3339 instant about 720h ahead", body.ExpiresAt)
		}
		if strings.Count(stdout, remoteIssuedSecret) != 1 {
			t.Fatalf("the secret appears %d times on standard output, want exactly once:\n%s",
				strings.Count(stdout, remoteIssuedSecret), stdout)
		}
		if !strings.Contains(stderr, "cannot be retrieved") {
			t.Fatalf("no warning that the secret cannot be retrieved later: %s", stderr)
		}
		if strings.Contains(stderr, remoteIssuedSecret) {
			t.Fatal("the secret was printed on standard error too")
		}
	})

	t.Run("create with --json prints only the appliance document", func(t *testing.T) {
		t.Parallel()
		stub := newAdminStub(t)
		code, stdout, stderr := runAdminInProcess(stub.remote(t,
			"token", "create", "--owner", "operator", "--name", "ci", "--json",
		), "", false)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
		}
		var document struct {
			Secret string `json:"secret"`
			Token  struct {
				ID string `json:"id"`
			} `json:"token"`
		}
		if err := json.Unmarshal([]byte(stdout), &document); err != nil {
			t.Fatalf("--json standard output is not one JSON document: %v\n%s", err, stdout)
		}
		if document.Secret != remoteIssuedSecret || document.Token.ID != "token-9" {
			t.Fatalf("--json document = %#v", document)
		}
		if strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
			t.Fatalf("--json standard output carries prose beside the document:\n%s", stdout)
		}
		if !strings.Contains(stderr, "cannot be retrieved") {
			t.Fatalf("the warning must still reach standard error: %s", stderr)
		}
	})

	t.Run("revoke", func(t *testing.T) {
		t.Parallel()
		stub := newAdminStub(t)
		code, _, stderr := runAdminInProcess(stub.remote(t, "token", "revoke", "--id", "token-1"), "", false)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
		}
		assertRequests(t, stub.requests(), "POST /api/v1/tokens/token-1/revoke", "GET /api/v1/tokens")
	})
}

func TestRemoteJSONOutputIsTheAppliancesDocument(t *testing.T) {
	t.Parallel()

	stub := newAdminStub(t)
	code, stdout, _ := runAdminInProcess(stub.remote(t, "user", "list", "--json"), "", false)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var document struct {
		APIVersion string `json:"api_version"`
		Items      []struct {
			Username string `json:"username"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("--json did not print one JSON document: %v\n%s", err, stdout)
	}
	if document.APIVersion != "nilo.http/1" || len(document.Items) != 2 {
		t.Fatalf("document = %#v", document)
	}

	code, readable, _ := runAdminInProcess(stub.remote(t, "user", "list"), "", false)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(readable, "USERNAME") || !strings.Contains(readable, "operator") || !strings.Contains(readable, "administrator") {
		t.Fatalf("the readable listing lacks a header or the identities:\n%s", readable)
	}
}

// A command runs in exactly one mode. Anything else is a usage error that
// reaches no appliance and opens no data directory.
func TestRemoteAdminRejectsAmbiguousOrIncompleteModes(t *testing.T) {
	t.Parallel()

	stub := newAdminStub(t)
	token := tokenFile(t, remoteAdminSecret)
	dataDir := t.TempDir()
	for _, test := range []struct {
		name      string
		arguments []string
		mention   string
	}{
		{name: "both modes", arguments: []string{"user", "list", "--data-dir", dataDir, "--server", stub.server.URL, "--token-file", token}, mention: "not both"},
		{name: "data dir with token file", arguments: []string{"token", "list", "--data-dir", dataDir, "--token-file", token}, mention: "not both"},
		{name: "server without token file", arguments: []string{"user", "list", "--server", stub.server.URL}, mention: "--token-file"},
		{name: "token file without server", arguments: []string{"user", "list", "--token-file", token}, mention: "--server"},
		{name: "no mode at all", arguments: []string{"user", "list"}, mention: "--data-dir"},
		{name: "actor in remote mode", arguments: []string{"user", "disable", "--username", "operator", "--actor", "admin", "--server", stub.server.URL, "--token-file", token}, mention: "--actor"},
		{name: "json in local mode", arguments: []string{"user", "list", "--data-dir", dataDir, "--json"}, mention: "--json"},
		{name: "confirm-delete in local mode", arguments: []string{"token", "list", "--data-dir", dataDir, "--confirm-delete"}, mention: "--confirm-delete"},
		{name: "local user deletion is not offered", arguments: []string{"user", "delete", "--data-dir", dataDir, "--actor", "admin", "--username", "operator"}, mention: "--server"},
		{name: "local token deletion is not offered", arguments: []string{"token", "delete", "--data-dir", dataDir, "--actor", "admin", "--id", "token-1"}, mention: "--server"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			code, _, stderr := runAdminInProcess(test.arguments, "", false)
			if code != 2 {
				t.Fatalf("exit = %d, want 2; stderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, test.mention) {
				t.Fatalf("stderr does not mention %s: %s", test.mention, stderr)
			}
		})
	}
	if requests := stub.requests(); len(requests) != 0 {
		t.Fatalf("usage errors still made requests: %v", describeRequests(requests))
	}
	if _, err := os.Stat(filepath.Join(dataDir, "nilo.db")); err == nil {
		t.Fatal("a usage error opened the data directory")
	}
}

func TestRemoteAdminReportsMissingOrMalformedTokenFiles(t *testing.T) {
	t.Parallel()

	stub := newAdminStub(t)
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(empty, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"missing":   filepath.Join(t.TempDir(), "absent"),
		"empty":     empty,
		"directory": t.TempDir(),
	} {
		code, _, stderr := runAdminInProcess([]string{
			"user", "list", "--server", stub.server.URL, "--token-file", path,
		}, "", false)
		if code != 1 {
			t.Fatalf("%s token file: exit = %d, want 1; stderr=%s", name, code, stderr)
		}
		if !strings.Contains(stderr, "token") {
			t.Fatalf("%s token file: stderr does not name the token file: %s", name, stderr)
		}
	}
	if requests := stub.requests(); len(requests) != 0 {
		t.Fatalf("an unusable token file still made requests: %v", describeRequests(requests))
	}
}

// A refusal is the appliance's decision and is reported with its stable code:
// the command adds no authorization of its own and invents no message.
func TestRemoteAdminRefusalsExitOneWithTheApplianceCode(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		route     string
		status    int
		code      string
		arguments []string
	}{
		{name: "forbidden listing", route: "GET /api/v1/users", status: http.StatusForbidden, code: "forbidden", arguments: []string{"user", "list"}},
		{name: "unauthorized listing", route: "GET /api/v1/tokens", status: http.StatusUnauthorized, code: "unauthorized", arguments: []string{"token", "list"}},
		{name: "unknown user", route: "PUT /api/v1/users/ghost/role", status: http.StatusNotFound, code: "user_not_found", arguments: []string{"user", "set-role", "--username", "ghost", "--role", "viewer"}},
		{name: "last administrator", route: "DELETE /api/v1/users/admin", status: http.StatusConflict, code: "last_administrator", arguments: []string{"user", "delete", "--username", "admin", "--confirm-delete"}},
		{name: "owned tokens", route: "DELETE /api/v1/users/operator", status: http.StatusConflict, code: "user_has_tokens", arguments: []string{"user", "delete", "--username", "operator", "--confirm-delete"}},
		{name: "already deleted token", route: "DELETE /api/v1/tokens/token-1", status: http.StatusNotFound, code: "token_not_found", arguments: []string{"token", "delete", "--id", "token-1", "--confirm-delete"}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := newAdminStub(t)
			stub.answer(test.route, test.status, map[string]string{"error": test.code, "message": "fixed text"})
			code, stdout, stderr := runAdminInProcess(stub.remote(t, test.arguments...), "", false)
			if code != 1 {
				t.Fatalf("exit = %d, want 1; stderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, test.code) {
				t.Fatalf("stderr does not carry the appliance's code %q: %s", test.code, stderr)
			}
			if strings.Contains(stdout, test.code) {
				t.Fatalf("the error was printed on standard output: %s", stdout)
			}
		})
	}
}

// Permanent deletion is explicit twice over: the command says exactly what it
// is about to remove, and then needs that subject typed back.
func TestRemoteUserDeletionRequiresTheUsernameTypedBack(t *testing.T) {
	t.Parallel()

	t.Run("typed back exactly", func(t *testing.T) {
		t.Parallel()
		stub := newAdminStub(t)
		code, stdout, stderr := runAdminInProcess(stub.remote(t, "user", "delete", "--username", "operator"), "operator\n", true)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
		}
		assertRequests(t, stub.requests(), "GET /api/v1/users", "DELETE /api/v1/users/operator", "GET /api/v1/users")
		description := stderr[:strings.Index(stderr, "confirm")]
		for _, want := range []string{`user "operator"`, "user-2", "operator", "enabled"} {
			if !strings.Contains(description, want) {
				t.Fatalf("the description before the prompt lacks %q: %s", want, stderr)
			}
		}
		if !strings.Contains(stdout, "USERNAME") {
			t.Fatalf("the listing after the deletion is missing: %s", stdout)
		}
	})

	for name, answer := range map[string]string{
		"different case": "Operator\n",
		"another name":   "admin\n",
		"empty":          "\n",
		"end of input":   "",
	} {
		answer := answer
		t.Run("cancelled by "+name, func(t *testing.T) {
			t.Parallel()
			stub := newAdminStub(t)
			code, _, stderr := runAdminInProcess(stub.remote(t, "user", "delete", "--username", "operator"), answer, true)
			if code != 1 {
				t.Fatalf("exit = %d, want 1; stderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, "cancelled") {
				t.Fatalf("stderr does not say the deletion was cancelled: %s", stderr)
			}
			assertRequests(t, stub.requests(), "GET /api/v1/users")
		})
	}

	t.Run("flag given up front on a terminal", func(t *testing.T) {
		t.Parallel()
		stub := newAdminStub(t)
		code, _, stderr := runAdminInProcess(stub.remote(t, "user", "delete", "--username", "operator", "--confirm-delete"), "", true)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
		}
		assertRequests(t, stub.requests(), "GET /api/v1/users", "DELETE /api/v1/users/operator", "GET /api/v1/users")
	})
}

func TestRemoteTokenDeletionRequiresTheTokenIDTypedBack(t *testing.T) {
	t.Parallel()

	t.Run("typed back exactly", func(t *testing.T) {
		t.Parallel()
		stub := newAdminStub(t)
		code, _, stderr := runAdminInProcess(stub.remote(t, "token", "delete", "--id", "token-2"), "token-2\n", true)
		if code != 0 {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
		}
		assertRequests(t, stub.requests(), "GET /api/v1/tokens", "DELETE /api/v1/tokens/token-2", "GET /api/v1/tokens")
		description := stderr[:strings.Index(stderr, "confirm")]
		for _, want := range []string{"token-2", `"old"`, "admin", "revoked"} {
			if !strings.Contains(description, want) {
				t.Fatalf("the description before the prompt lacks %q: %s", want, stderr)
			}
		}
	})

	t.Run("cancelled by a different identifier", func(t *testing.T) {
		t.Parallel()
		stub := newAdminStub(t)
		code, _, stderr := runAdminInProcess(stub.remote(t, "token", "delete", "--id", "token-2"), "token-1\n", true)
		if code != 1 {
			t.Fatalf("exit = %d, want 1; stderr=%s", code, stderr)
		}
		assertRequests(t, stub.requests(), "GET /api/v1/tokens")
	})
}

// Without a terminal nobody can be asked, so consent has to be given with the
// dedicated flag. Nothing else counts, least of all the network preview flag.
func TestNoninteractiveDeletionRequiresConfirmDelete(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		arguments []string
		listing   string
		deletion  string
	}{
		{name: "user", arguments: []string{"user", "delete", "--username", "operator"}, listing: "GET /api/v1/users", deletion: "DELETE /api/v1/users/operator"},
		{name: "token", arguments: []string{"token", "delete", "--id", "token-1"}, listing: "GET /api/v1/tokens", deletion: "DELETE /api/v1/tokens/token-1"},
	} {
		test := test
		t.Run(test.name+" without the flag", func(t *testing.T) {
			t.Parallel()
			stub := newAdminStub(t)
			code, _, stderr := runAdminInProcess(stub.remote(t, test.arguments...), "operator\ntoken-1\n", false)
			if code != 2 {
				t.Fatalf("exit = %d, want 2; stderr=%s", code, stderr)
			}
			if !strings.Contains(stderr, "--confirm-delete") {
				t.Fatalf("stderr does not name --confirm-delete: %s", stderr)
			}
			assertRequests(t, stub.requests(), test.listing)
		})
		t.Run(test.name+" with the flag", func(t *testing.T) {
			t.Parallel()
			stub := newAdminStub(t)
			code, _, stderr := runAdminInProcess(stub.remote(t, append(test.arguments, "--confirm-delete")...), "", false)
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
			}
			assertRequests(t, stub.requests(), test.listing, test.deletion, test.listing)
			if !strings.Contains(stderr, "deleted") {
				t.Fatalf("stderr does not report the deletion: %s", stderr)
			}
		})
		t.Run(test.name+" with --acknowledge instead", func(t *testing.T) {
			t.Parallel()
			stub := newAdminStub(t)
			code, _, _ := runAdminInProcess(stub.remote(t, append(test.arguments, "--acknowledge")...), "", false)
			if code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if requests := stub.requests(); len(requests) != 0 {
				t.Fatalf("--acknowledge reached the appliance: %v", describeRequests(requests))
			}
		})
	}
}

// A subject the listing does not know is still sent, so an already-deleted or
// mistyped subject gets the appliance's own answer rather than a guess.
func TestDeletingAnUnlistedSubjectIsDecidedByTheAppliance(t *testing.T) {
	t.Parallel()

	stub := newAdminStub(t)
	stub.answer("DELETE /api/v1/users/ghost", http.StatusNotFound, map[string]string{"error": "user_not_found"})
	code, _, stderr := runAdminInProcess(stub.remote(t, "user", "delete", "--username", "ghost", "--confirm-delete"), "", false)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "No listed user") || !strings.Contains(stderr, "user_not_found") {
		t.Fatalf("stderr = %s", stderr)
	}
	assertRequests(t, stub.requests(), "GET /api/v1/users", "DELETE /api/v1/users/ghost")
}

// No admin flag ever carries a credential: the token comes from a file and the
// password from standard input, exactly as the network commands do.
func TestNoAdminFlagAcceptsATokenOrPasswordValue(t *testing.T) {
	t.Parallel()

	for _, file := range []string{"admin.go", "admin_remote.go"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, forbidden := range []string{`"token"`, `"bearer"`, `"api-token"`, `"secret"`, `"password"`} {
			for _, declaration := range []string{"flags.String(", "flags.StringVar(&", "flag.String("} {
				if strings.Contains(string(source), declaration+forbidden) {
					t.Fatalf("%s declares a %s flag", file, forbidden)
				}
			}
		}
	}

	stub := newAdminStub(t)
	code, _, stderr := runAdminInProcess([]string{"user", "list", "--server", stub.server.URL, "--token", "leaked"}, "", false)
	if code != 2 || !strings.Contains(stderr, "token-file") {
		t.Fatalf("--token: exit = %d, stderr = %s; want 2 and a mention of --token-file", code, stderr)
	}
	code, _, _ = runAdminInProcess(stub.remote(t, "user", "create", "--username", "u", "--role", "viewer", "--password", "leaked"), "", false)
	if code != 2 {
		t.Fatalf("--password: exit = %d, want 2", code)
	}
	if requests := stub.requests(); len(requests) != 0 {
		t.Fatalf("a credential flag still made requests: %v", describeRequests(requests))
	}
}
