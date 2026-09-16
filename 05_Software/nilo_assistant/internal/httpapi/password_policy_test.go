package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/simulation"
)

const passwordPolicyPath = "/api/v1/password-policy"

type policyView struct {
	MinimumLength           int `json:"minimum_length"`
	MinimumCharacterClasses int `json:"minimum_character_classes"`
}

func decodePolicy(t *testing.T, response *httptest.ResponseRecorder) policyView {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var policy policyView
	if err := json.Unmarshal(response.Body.Bytes(), &policy); err != nil {
		t.Fatalf("decode policy %q: %v", response.Body.String(), err)
	}
	return policy
}

func TestPasswordPolicyRouteServesTheShippedDefaultAndStoresAReplacement(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)

	initial := decodePolicy(t, f.sessionSend(t, http.MethodGet, passwordPolicyPath, nil, session))
	if initial != (policyView{MinimumLength: 12, MinimumCharacterClasses: 0}) {
		t.Fatalf("initial policy = %#v, want the shipped default", initial)
	}

	replaced := decodePolicy(t, f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 16, MinimumCharacterClasses: 3}, session))
	if replaced != (policyView{MinimumLength: 16, MinimumCharacterClasses: 3}) {
		t.Fatalf("replacement returned %#v", replaced)
	}
	reread := decodePolicy(t, f.sessionSend(t, http.MethodGet, passwordPolicyPath, nil, session))
	if reread != replaced {
		t.Fatalf("policy read back as %#v, want %#v", reread, replaced)
	}
}

// A partial body is refused rather than merged, so a client can never silently
// relax the half it did not send.
func TestPasswordPolicyRouteRefusesAnIncompleteOrMalformedBody(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	if response := f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 16, MinimumCharacterClasses: 2}, session); response.Code != http.StatusOK {
		t.Fatalf("baseline replacement failed: %s", response.Body.String())
	}

	for name, body := range map[string]any{
		"only the minimum": map[string]any{"minimum_length": 20},
		"only the classes": map[string]any{"minimum_character_classes": 4},
		"not an object":    []int{20, 4},
		"wrong types":      map[string]any{"minimum_length": "20", "minimum_character_classes": 4},
	} {
		t.Run(name, func(t *testing.T) {
			response := f.sessionSend(t, http.MethodPut, passwordPolicyPath, body, session)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
			}
			if failure := decodeError(t, response); failure.Error != "invalid_password_policy" {
				t.Fatalf("error code = %q, want invalid_password_policy", failure.Error)
			}
		})
	}

	kept := decodePolicy(t, f.sessionSend(t, http.MethodGet, passwordPolicyPath, nil, session))
	if kept != (policyView{MinimumLength: 16, MinimumCharacterClasses: 2}) {
		t.Fatalf("a refused replacement changed the stored policy to %#v", kept)
	}
}

func TestPasswordPolicyRouteRefusesAMinimumBelowTheFloorNamingIt(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)

	response := f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 7, MinimumCharacterClasses: 0}, session)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	failure := decodeError(t, response)
	if failure.Error != "invalid_password_policy" {
		t.Fatalf("error code = %q", failure.Error)
	}
	if !strings.Contains(failure.Message, "8") {
		t.Fatalf("message %q does not name the floor", failure.Message)
	}

	classes := f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 12, MinimumCharacterClasses: 5}, session)
	if classes.Code != http.StatusBadRequest {
		t.Fatalf("class count 5 status = %d, want 400", classes.Code)
	}

	kept := decodePolicy(t, f.sessionSend(t, http.MethodGet, passwordPolicyPath, nil, session))
	if kept != (policyView{MinimumLength: 12, MinimumCharacterClasses: 0}) {
		t.Fatalf("a refused replacement changed the stored policy to %#v", kept)
	}
}

// The policy decides how weak a credential this appliance accepts, so reading it
// is reconnaissance and changing it is an attack. Neither is available without
// the permission, and a refusal discloses no part of the policy.
func TestPasswordPolicyRouteIsClosedToEveryCallerWithoutThePermission(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	administrator := f.administratorSession(t)
	if response := f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 18, MinimumCharacterClasses: 1}, administrator); response.Code != http.StatusOK {
		t.Fatalf("baseline replacement failed: %s", response.Body.String())
	}

	f.createUser(t, "operator", auth.RoleOperator, createdIdentityPassword)
	f.createUser(t, "viewer", auth.RoleViewer, createdIdentityPassword)
	operator := f.login(t, "operator", createdIdentityPassword)
	viewer := f.login(t, "viewer", createdIdentityPassword)
	scopedToken := f.issueToken(t, "admin", "no-policy-scope", []auth.Permission{auth.PermissionUsersManage}, nil)

	send := []struct {
		name     string
		response func(method string, body any) *httptest.ResponseRecorder
		want     int
	}{
		{"operator session", func(method string, body any) *httptest.ResponseRecorder {
			return f.sessionSend(t, method, passwordPolicyPath, body, operator)
		}, http.StatusForbidden},
		{"viewer session", func(method string, body any) *httptest.ResponseRecorder {
			return f.sessionSend(t, method, passwordPolicyPath, body, viewer)
		}, http.StatusForbidden},
		{"token without the scope", func(method string, body any) *httptest.ResponseRecorder {
			return f.bearerSend(t, method, passwordPolicyPath, body, scopedToken)
		}, http.StatusForbidden},
		{"no credential", func(method string, body any) *httptest.ResponseRecorder {
			return f.bearerSend(t, method, passwordPolicyPath, body, "")
		}, http.StatusUnauthorized},
	}
	for _, caller := range send {
		t.Run(caller.name, func(t *testing.T) {
			read := caller.response(http.MethodGet, nil)
			if read.Code != caller.want {
				t.Fatalf("GET status = %d, want %d", read.Code, caller.want)
			}
			if strings.Contains(read.Body.String(), "18") ||
				strings.Contains(read.Body.String(), "minimum_length") {
				t.Fatalf("refusal disclosed the policy: %s", read.Body.String())
			}
			change := caller.response(http.MethodPut, policyView{MinimumLength: 8})
			if change.Code != caller.want {
				t.Fatalf("PUT status = %d, want %d", change.Code, caller.want)
			}
		})
	}

	kept := decodePolicy(t, f.sessionSend(t, http.MethodGet, passwordPolicyPath, nil, administrator))
	if kept != (policyView{MinimumLength: 18, MinimumCharacterClasses: 1}) {
		t.Fatalf("an unauthorized caller changed the policy to %#v", kept)
	}
}

func TestIdentityRoutesHoldANewPasswordToTheConfiguredPolicy(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	if response := f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 20, MinimumCharacterClasses: 3}, session); response.Code != http.StatusOK {
		t.Fatalf("replacement failed: %s", response.Body.String())
	}

	create := newIdentityBody("newcomer", "operator")
	create["password"] = "Passw0rd-1234"
	response := f.sessionSend(t, http.MethodPost, "/api/v1/users", create, session)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400: %s", response.Code, response.Body.String())
	}
	failure := decodeError(t, response)
	if failure.Error != "invalid_password" {
		t.Fatalf("error code = %q, want invalid_password", failure.Error)
	}
	if !strings.Contains(failure.Message, "20") {
		t.Fatalf("message %q does not name the configured minimum", failure.Message)
	}

	create["password"] = "passwordpasswordpassword"
	classes := f.sessionSend(t, http.MethodPost, "/api/v1/users", create, session)
	if classes.Code != http.StatusBadRequest {
		t.Fatalf("too-few-classes status = %d, want 400: %s", classes.Code, classes.Body.String())
	}
	if message := decodeError(t, classes).Message; !strings.Contains(message, "3") {
		t.Fatalf("message %q does not name the required class count", message)
	}

	create["password"] = "Correct-Horse-Battery-1"
	if accepted := f.sessionSend(t, http.MethodPost, "/api/v1/users", create, session); accepted.Code != http.StatusCreated {
		t.Fatalf("compliant create status = %d: %s", accepted.Code, accepted.Body.String())
	}

	reset := f.sessionSend(t, http.MethodPut, "/api/v1/users/newcomer/password",
		map[string]string{"password": "Passw0rd-1234"}, session)
	if reset.Code != http.StatusBadRequest {
		t.Fatalf("reset status = %d, want 400: %s", reset.Code, reset.Body.String())
	}
}

// Tightening the policy never touches a stored verifier, so an identity created
// under the earlier rule still signs in over the same routes.
func TestSignInSurvivesATightenedPasswordPolicy(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, createdIdentityPassword)

	if response := f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 40, MinimumCharacterClasses: 4}, session); response.Code != http.StatusOK {
		t.Fatalf("replacement failed: %s", response.Body.String())
	}

	signedIn := f.login(t, "operator", createdIdentityPassword)
	if signedIn.sessionCookie == nil {
		t.Fatal("an identity created under the earlier policy could not sign in")
	}
}

// Every applied change is on record, and the record carries the rule rather
// than anybody's password.
func TestPasswordPolicyChangeAppendsOneAuditEventCarryingNoSecret(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)

	before := f.countAuditAction(t, "identity.password_policy.changed")
	if response := f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 22, MinimumCharacterClasses: 2}, session); response.Code != http.StatusOK {
		t.Fatalf("replacement failed: %s", response.Body.String())
	}
	if after := f.countAuditAction(t, "identity.password_policy.changed"); after != before+1 {
		t.Fatalf("applied change appended %d events, want exactly 1", after-before)
	}

	if refused := f.sessionSend(t, http.MethodPut, passwordPolicyPath,
		policyView{MinimumLength: 4}, session); refused.Code != http.StatusBadRequest {
		t.Fatalf("refusal status = %d, want 400", refused.Code)
	}
	if after := f.countAuditAction(t, "identity.password_policy.changed"); after != before+1 {
		t.Fatalf("a refused change appended an event")
	}

	details := f.auditDetails(t, "identity.password_policy.changed")
	for _, secret := range []string{administratorPassword, createdIdentityPassword, "$argon2id$"} {
		if strings.Contains(details, secret) {
			t.Fatalf("the audit event carries %q: %s", secret, details)
		}
	}
	for _, expected := range []string{"12", "22"} {
		if !strings.Contains(details, expected) {
			t.Fatalf("the audit event does not carry %q: %s", expected, details)
		}
	}
}

// countAuditAction and auditDetails read the appliance's own audit table, which
// is where an event has to be for the history to show it.
func (f apiFixture) countAuditAction(t *testing.T, action string) int {
	t.Helper()
	var count int
	if err := f.database.SQL().QueryRowContext(
		context.Background(), "SELECT COUNT(*) FROM audit_events WHERE action = ?", action,
	).Scan(&count); err != nil {
		t.Fatalf("count %q events: %v", action, err)
	}
	return count
}

func (f apiFixture) auditDetails(t *testing.T, action string) string {
	t.Helper()
	var payload string
	if err := f.database.SQL().QueryRowContext(
		context.Background(),
		"SELECT payload_json FROM audit_events WHERE action = ? ORDER BY sequence DESC LIMIT 1", action,
	).Scan(&payload); err != nil {
		t.Fatalf("read the latest %q event: %v", action, err)
	}
	return payload
}
