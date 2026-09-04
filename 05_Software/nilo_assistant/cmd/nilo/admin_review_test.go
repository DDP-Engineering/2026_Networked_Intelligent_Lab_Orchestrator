package main

import (
	"net/http"
	"strings"
	"testing"
)

// The listing read around a change is informational. It must never turn an
// applied change into a reported failure, and it must never stand between a
// token that may delete and the deletion it is allowed to make.

func TestAnAppliedChangeIsReportedEvenWhenTheListingCannotBeReread(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		arguments []string
		listing   string
		requests  []string
	}{
		{
			name: "user set-role", arguments: []string{"user", "set-role", "--username", "operator", "--role", "viewer"},
			listing: "GET /api/v1/users", requests: []string{"PUT /api/v1/users/operator/role", "GET /api/v1/users"},
		},
		{
			name: "token revoke", arguments: []string{"token", "revoke", "--id", "token-1"},
			listing: "GET /api/v1/tokens", requests: []string{"POST /api/v1/tokens/token-1/revoke", "GET /api/v1/tokens"},
		},
		{
			name: "user delete", arguments: []string{"user", "delete", "--username", "operator", "--confirm-delete"},
			listing: "GET /api/v1/users", requests: []string{"GET /api/v1/users", "DELETE /api/v1/users/operator", "GET /api/v1/users"},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := newAdminStub(t)
			if strings.HasPrefix(test.name, "user delete") {
				// The listing works before the deletion and breaks after it.
				stub.answerOnce(test.listing, http.StatusOK, stubUserListing())
			}
			stub.answer(test.listing, http.StatusServiceUnavailable, map[string]string{"error": "internal_error"})
			code, stdout, stderr := runAdminInProcess(stub.remote(t, test.arguments...), "", false)
			if code != 0 {
				t.Fatalf("an applied change exited %d; stderr=%s", code, stderr)
			}
			assertRequests(t, stub.requests(), test.requests...)
			if !strings.Contains(stderr, "applied") && !strings.Contains(stderr, "deleted") {
				t.Fatalf("stderr does not say the change was applied: %s", stderr)
			}
			if !strings.Contains(stderr, "listing") {
				t.Fatalf("stderr does not explain that the listing could not be re-read: %s", stderr)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Fatalf("standard output carries something other than data: %q", stdout)
			}
		})
	}
}

// A token narrowed to users.manage or tokens.manage.all may delete but may not
// list. The description is then skipped, not the deletion.
func TestDeletionDoesNotRequireTheReadPermission(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		arguments []string
		listing   string
		deletion  string
	}{
		{name: "user", arguments: []string{"user", "delete", "--username", "operator", "--confirm-delete"}, listing: "GET /api/v1/users", deletion: "DELETE /api/v1/users/operator"},
		{name: "token", arguments: []string{"token", "delete", "--id", "token-1", "--confirm-delete"}, listing: "GET /api/v1/tokens", deletion: "DELETE /api/v1/tokens/token-1"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := newAdminStub(t)
			stub.answer(test.listing, http.StatusForbidden, map[string]string{"error": "forbidden"})
			code, _, stderr := runAdminInProcess(stub.remote(t, test.arguments...), "", false)
			if code != 0 {
				t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
			}
			assertRequests(t, stub.requests(), test.listing, test.deletion, test.listing)
			if !strings.Contains(stderr, "forbidden") || !strings.Contains(stderr, "deleted") {
				t.Fatalf("stderr does not explain the skipped description and report the deletion: %s", stderr)
			}
		})
	}

	// An unreachable or unusable appliance is still a stop: nothing destructive
	// is attempted on the strength of a prompt alone.
	stub := newAdminStub(t)
	stub.answer("GET /api/v1/users", http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	code, _, _ := runAdminInProcess(stub.remote(t, "user", "delete", "--username", "operator", "--confirm-delete"), "", false)
	if code != 1 {
		t.Fatalf("an unauthorized listing exited %d, want 1", code)
	}
	assertRequests(t, stub.requests(), "GET /api/v1/users")
}

// The appliance folds usernames to lower case, so the command describes,
// addresses and confirms the identity the appliance would actually act on.
func TestUsernamesAreFoldedTheWayTheApplianceFoldsThem(t *testing.T) {
	t.Parallel()

	stub := newAdminStub(t)
	code, _, stderr := runAdminInProcess(stub.remote(t, "user", "delete", "--username", " Operator "), "OPERATOR\n", true)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr)
	}
	assertRequests(t, stub.requests(), "GET /api/v1/users", "DELETE /api/v1/users/operator", "GET /api/v1/users")
	if !strings.Contains(stderr, `user "operator"`) || !strings.Contains(stderr, "user-2") {
		t.Fatalf("the description does not name the folded identity: %s", stderr)
	}
	if strings.Contains(stderr, "No listed user") {
		t.Fatalf("the folded identity was reported as unlisted: %s", stderr)
	}

	stub = newAdminStub(t)
	code, _, _ = runAdminInProcess(stub.remote(t, "user", "disable", "--username", "Operator"), "", false)
	if code != 0 {
		t.Fatalf("disable with a mixed-case username exited %d", code)
	}
	assertRequests(t, stub.requests(), "PUT /api/v1/users/operator/enabled", "GET /api/v1/users")
}
