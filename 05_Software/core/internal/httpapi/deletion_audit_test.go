package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/simulation"
)

// Over the routes, a refused deletion and an applied one both leave a record
// that reads on its own once the subject row is gone: the actor's name, the
// subject's name and, for a refusal, the reason.
func TestRefusedAndAppliedDeletionsReadFromTheHistoryAlone(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)

	if response := f.deleteUser(t, "admin", session); response.Code != http.StatusConflict {
		t.Fatalf("deleting the last administrator returned %d, want 409", response.Code)
	}
	refusals := listAuditEvents(t, f.database, "identity.user.delete_refused")
	if len(refusals) != 1 {
		t.Fatalf("refused deletion records = %d, want 1", len(refusals))
	}
	refusal := refusals[0]
	if refusal.Outcome != audit.OutcomeDenied || refusal.Severity != audit.SeverityWarning ||
		refusal.ActorKind != audit.ActorKindUser || refusal.ActorID.String() != f.administrator.ID ||
		refusal.TargetLabel != "admin" {
		t.Fatalf("refusal record = %#v", refusal)
	}
	if details := decodeEventDetails(t, refusal); details["reason"] != "last_administrator" ||
		details["actor_username"] != "admin" || details["username"] != "admin" {
		t.Fatalf("refusal details = %#v", details)
	}

	// A credential that lacks the permission never reaches the service; the
	// boundary refuses it and records the access denial.
	f.createUser(t, "viewer", auth.RoleViewer, operatorPassword)
	viewerSecret := f.issueToken(t, "viewer", "viewer-token", nil, nil)
	if response := f.bearerSend(t, http.MethodDelete, "/api/v1/users/admin", nil, viewerSecret); response.Code != http.StatusForbidden {
		t.Fatalf("a viewer deleting an identity returned %d, want 403", response.Code)
	}
	denials := listAuditEvents(t, f.database, "identity.access.denied")
	if len(denials) == 0 {
		t.Fatal("the forbidden deletion was not recorded as an access denial")
	}
	if details := decodeEventDetails(t, denials[len(denials)-1]); details["permission"] != string(auth.PermissionUsersManage) {
		t.Fatalf("access denial details = %#v", details)
	}

	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	tokenSecret := f.issueToken(t, "operator", "ci", []auth.Permission{auth.PermissionStatusRead}, nil)
	token, found := findToken(f.listTokens(t, session), "ci")
	if !found {
		t.Fatal("the issued token is not listed")
	}
	if response := f.deleteToken(t, token.ID, session); response.Code != http.StatusNoContent {
		t.Fatalf("deleting the token returned %d, want 204", response.Code)
	}
	if response := f.deleteUser(t, "operator", session); response.Code != http.StatusNoContent {
		t.Fatalf("deleting the operator returned %d, want 204", response.Code)
	}

	tokenEvents := listAuditEvents(t, f.database, "identity.token.deleted")
	if len(tokenEvents) != 1 || tokenEvents[0].TargetLabel != "ci" {
		t.Fatalf("token deletion records = %#v", tokenEvents)
	}
	if details := decodeEventDetails(t, tokenEvents[0]); details["owner"] != "operator" ||
		details["actor_username"] != "admin" || details["token_name"] != "ci" {
		t.Fatalf("token deletion details = %#v", details)
	}
	userEvents := listAuditEvents(t, f.database, "identity.user.deleted")
	if len(userEvents) != 1 || userEvents[0].TargetLabel != "operator" {
		t.Fatalf("user deletion records = %#v", userEvents)
	}
	if details := decodeEventDetails(t, userEvents[0]); details["actor_username"] != "admin" || details["username"] != "operator" {
		t.Fatalf("user deletion details = %#v", details)
	}

	for _, events := range [][]audit.Event{refusals, denials, tokenEvents, userEvents} {
		for _, event := range events {
			for _, canary := range []string{tokenSecret, viewerSecret, administratorPassword, operatorPassword, "verifier", "$argon2id$"} {
				if strings.Contains(string(event.Payload), canary) {
					t.Fatalf("event %q leaked %q", event.Action, canary)
				}
			}
		}
	}
}

func decodeEventDetails(t *testing.T, event audit.Event) map[string]string {
	t.Helper()
	var details map[string]string
	if err := json.Unmarshal(event.Payload, &details); err != nil {
		t.Fatalf("decode %q payload %s: %v", event.Action, event.Payload, err)
	}
	return details
}
