package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

// The administration routes are proven through the production handler over a
// real database, exactly like every other authorization case: a helper that
// called the identity service directly would prove the service and say nothing
// about the routes an administrator's browser actually reaches.

const createdIdentityPassword = "created-secret-0001"

type userView struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

type tokenView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Owner     string     `json:"owner"`
	Scopes    []string   `json:"scopes"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Revoked   bool       `json:"revoked"`
	RevokedAt *time.Time `json:"revoked_at"`
}

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (f apiFixture) jsonRequest(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode %s %s body: %v", method, path, err)
		}
		payload = bytes.NewReader(encoded)
	}
	request := httptest.NewRequest(method, path, payload)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

// sessionSend issues a request as a signed-in human, carrying the session
// cookie and the CSRF token that session was issued with.
func (f apiFixture) sessionSend(
	t *testing.T, method, path string, body any, session loginResult,
) *httptest.ResponseRecorder {
	t.Helper()
	request := f.jsonRequest(t, method, path, body)
	request.AddCookie(session.sessionCookie)
	request.Header.Set(auth.CSRFHeaderName, session.csrfToken)
	return f.do(request)
}

func (f apiFixture) bearerSend(
	t *testing.T, method, path string, body any, bearer string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := f.jsonRequest(t, method, path, body)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	return f.do(request)
}

// administratorSession signs the bootstrapped administrator in, which is how
// every administration test obtains the credential a browser would hold.
func (f apiFixture) administratorSession(t *testing.T) loginResult {
	t.Helper()
	return f.login(t, "admin", administratorPassword)
}

func (f apiFixture) listUsers(t *testing.T, session loginResult) []userView {
	t.Helper()
	response := f.sessionSend(t, http.MethodGet, "/api/v1/users", nil, session)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/users = %d; body=%s", response.Code, response.Body.String())
	}
	var listing struct {
		Items []userView `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode identity listing: %v", err)
	}
	return listing.Items
}

func (f apiFixture) listTokens(t *testing.T, session loginResult) []tokenView {
	t.Helper()
	response := f.sessionSend(t, http.MethodGet, "/api/v1/tokens", nil, session)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/tokens = %d; body=%s", response.Code, response.Body.String())
	}
	var listing struct {
		Items []tokenView `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode token listing: %v", err)
	}
	return listing.Items
}

func findUser(users []userView, username string) (userView, bool) {
	for _, user := range users {
		if user.Username == username {
			return user, true
		}
	}
	return userView{}, false
}

func findToken(tokens []tokenView, name string) (tokenView, bool) {
	for _, token := range tokens {
		if token.Name == name {
			return token, true
		}
	}
	return tokenView{}, false
}

func decodeError(t *testing.T, response *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", response.Body.String(), err)
	}
	return body
}

func newIdentityBody(username, role string) map[string]string {
	return map[string]string{"username": username, "role": role, "password": createdIdentityPassword}
}

func TestAdministrationCreatesAnIdentityThatCanSignIn(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)

	response := f.sessionSend(t, http.MethodPost, "/api/v1/users", newIdentityBody("jane", "operator"), session)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/users = %d, want 201; body=%s", response.Code, response.Body.String())
	}
	var created userView
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created identity: %v", err)
	}
	if created.Username != "jane" || created.Role != "operator" || !created.Enabled || created.ID == "" {
		t.Fatalf("created identity = %#v", created)
	}

	listed, found := findUser(f.listUsers(t, session), "jane")
	if !found || listed.Role != "operator" {
		t.Fatalf("identity listing after creation = %#v", f.listUsers(t, session))
	}
	if signedIn := f.attemptLogin("jane", createdIdentityPassword); signedIn.Code != http.StatusOK {
		t.Fatalf("the created identity could not sign in: %d; body=%s", signedIn.Code, signedIn.Body.String())
	}
}

func TestAdministrationRoleChangeAppliesToTheNextRequest(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	administrator := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	operator := f.login(t, "operator", operatorPassword)

	if refused := f.sessionGet("/api/v1/users", operator); refused.Code != http.StatusForbidden {
		t.Fatalf("operator GET /api/v1/users = %d, want 403", refused.Code)
	}

	promoted := f.sessionSend(
		t, http.MethodPut, "/api/v1/users/operator/role", map[string]string{"role": "administrator"}, administrator,
	)
	if promoted.Code != http.StatusNoContent {
		t.Fatalf("PUT role = %d, want 204; body=%s", promoted.Code, promoted.Body.String())
	}

	// The same unchanged session: permissions are resolved from the stored role
	// on every request, so nothing is re-issued and no cache is invalidated.
	if allowed := f.sessionGet("/api/v1/users", operator); allowed.Code != http.StatusOK {
		t.Fatalf("promoted session GET /api/v1/users = %d, want 200; body=%s", allowed.Code, allowed.Body.String())
	}
}

func TestAdministrationDisableAndEnableControlAuthentication(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	administrator := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	operator := f.login(t, "operator", operatorPassword)

	disabled := f.sessionSend(
		t, http.MethodPut, "/api/v1/users/operator/enabled", map[string]bool{"enabled": false}, administrator,
	)
	if disabled.Code != http.StatusNoContent {
		t.Fatalf("PUT enabled=false = %d, want 204; body=%s", disabled.Code, disabled.Body.String())
	}
	if held := f.sessionGet("/api/v1/status", operator); held.Code != http.StatusUnauthorized {
		t.Fatalf("a disabled identity's existing session = %d, want 401", held.Code)
	}
	if fresh := f.attemptLogin("operator", operatorPassword); fresh.Code != http.StatusUnauthorized {
		t.Fatalf("a disabled identity signed in with %d, want 401", fresh.Code)
	}
	if listed, _ := findUser(f.listUsers(t, administrator), "operator"); listed.Enabled {
		t.Fatal("the identity listing still reports the disabled identity as enabled")
	}

	enabled := f.sessionSend(
		t, http.MethodPut, "/api/v1/users/operator/enabled", map[string]bool{"enabled": true}, administrator,
	)
	if enabled.Code != http.StatusNoContent {
		t.Fatalf("PUT enabled=true = %d, want 204; body=%s", enabled.Code, enabled.Body.String())
	}
	if fresh := f.attemptLogin("operator", operatorPassword); fresh.Code != http.StatusOK {
		t.Fatalf("a re-enabled identity could not sign in: %d", fresh.Code)
	}
}

func TestAdministrationPasswordResetReplacesTheCredential(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	administrator := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)

	response := f.sessionSend(
		t, http.MethodPut, "/api/v1/users/operator/password",
		map[string]string{"password": createdIdentityPassword}, administrator,
	)
	if response.Code != http.StatusNoContent {
		t.Fatalf("PUT password = %d, want 204; body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); strings.Contains(body, createdIdentityPassword) || strings.Contains(body, operatorPassword) {
		t.Fatalf("the password reset response repeated a password: %s", body)
	}
	if previous := f.attemptLogin("operator", operatorPassword); previous.Code != http.StatusUnauthorized {
		t.Fatalf("the previous password still signs in: %d", previous.Code)
	}
	if replaced := f.attemptLogin("operator", createdIdentityPassword); replaced.Code != http.StatusOK {
		t.Fatalf("the replaced password does not sign in: %d; body=%s", replaced.Code, replaced.Body.String())
	}
}

func TestTokenListingReportsSafeMetadataOnly(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	expiry := f.clock.Now().Add(720 * time.Hour).UTC()
	unnarrowed := f.issueToken(t, "admin", "unnarrowed", nil, nil)
	scoped := f.issueToken(t, "operator", "scoped", []auth.Permission{auth.PermissionStatusRead}, &expiry)

	tokens := f.listTokens(t, session)
	if len(tokens) != 2 {
		t.Fatalf("token listing = %#v, want the two created tokens", tokens)
	}

	wide, found := findToken(tokens, "unnarrowed")
	if !found || wide.Owner != "admin" || len(wide.Scopes) != 0 || wide.ExpiresAt != nil || wide.Revoked {
		t.Fatalf("unnarrowed token = %#v", wide)
	}
	narrow, found := findToken(tokens, "scoped")
	if !found || narrow.Owner != "operator" || narrow.ID == "" {
		t.Fatalf("scoped token = %#v", narrow)
	}
	if len(narrow.Scopes) != 1 || narrow.Scopes[0] != string(auth.PermissionStatusRead) {
		t.Fatalf("scoped token scopes = %v, want the single declared scope", narrow.Scopes)
	}
	if narrow.ExpiresAt == nil || !narrow.ExpiresAt.Equal(expiry) {
		t.Fatalf("scoped token expiry = %v, want %v", narrow.ExpiresAt, expiry)
	}

	serialized := f.sessionSend(t, http.MethodGet, "/api/v1/tokens", nil, session).Body.String()
	for _, secret := range []string{unnarrowed, scoped} {
		if strings.Contains(serialized, secret) {
			t.Fatal("the token listing repeated a token secret")
		}
	}
}

func TestTokenCreationDisclosesItsSecretExactlyOnce(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)

	response := f.sessionSend(t, http.MethodPost, "/api/v1/tokens", map[string]any{
		"owner": "operator", "name": "bench-ci", "scopes": []string{"status.read"},
	}, session)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/tokens = %d, want 201; body=%s", response.Code, response.Body.String())
	}
	var issued struct {
		Token  tokenView `json:"token"`
		Secret string    `json:"secret"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode created token: %v", err)
	}
	if issued.Secret == "" || issued.Token.ID == "" || issued.Token.Owner != "operator" {
		t.Fatalf("created token = %#v", issued.Token)
	}

	if used := f.get("/api/v1/status", issued.Secret); used.Code != http.StatusOK {
		t.Fatalf("the created secret does not authenticate: %d", used.Code)
	}
	// The scope narrows the owner's role, so the same secret cannot read the
	// inventory the Operator role by itself would allow.
	if beyond := f.get("/api/v1/devices", issued.Secret); beyond.Code != http.StatusForbidden {
		t.Fatalf("the scoped secret read beyond its scope: %d", beyond.Code)
	}

	for attempt := 0; attempt < 2; attempt++ {
		listing := f.sessionSend(t, http.MethodGet, "/api/v1/tokens", nil, session)
		if strings.Contains(listing.Body.String(), issued.Secret) {
			t.Fatal("a later token listing disclosed the secret again")
		}
	}
}

func TestTokenRevocationStopsTheSecretAndMarksTheListing(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	secret := f.issueToken(t, "admin", "retired", nil, nil)
	listed, found := findToken(f.listTokens(t, session), "retired")
	if !found {
		t.Fatal("the created token is missing from the listing")
	}
	if used := f.get("/api/v1/status", secret); used.Code != http.StatusOK {
		t.Fatalf("the token does not work before revocation: %d", used.Code)
	}

	revoked := f.sessionSend(t, http.MethodPost, "/api/v1/tokens/"+listed.ID+"/revoke", nil, session)
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("POST revoke = %d, want 204; body=%s", revoked.Code, revoked.Body.String())
	}
	if used := f.get("/api/v1/status", secret); used.Code != http.StatusUnauthorized {
		t.Fatalf("the revoked secret still authenticates: %d", used.Code)
	}

	after, found := findToken(f.listTokens(t, session), "retired")
	if !found || !after.Revoked || after.RevokedAt == nil {
		t.Fatalf("the revoked token reads back as %#v", after)
	}
	if repeated := f.sessionSend(t, http.MethodPost, "/api/v1/tokens/"+listed.ID+"/revoke", nil, session); repeated.Code != http.StatusNotFound {
		t.Fatalf("revoking an already revoked token = %d, want 404", repeated.Code)
	}
}

func TestAdministrationRefusesRequestsItCannotApply(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	past := f.clock.Now().Add(-time.Hour).UTC().Format(time.RFC3339)

	for _, test := range []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		code    string
		mustSay string
	}{
		{
			name: "unknown role", method: http.MethodPost, path: "/api/v1/users",
			body: newIdentityBody("newcomer", "superuser"), status: http.StatusBadRequest,
			code: "invalid_role", mustSay: "role",
		},
		{
			name: "username outside the accepted form", method: http.MethodPost, path: "/api/v1/users",
			body: newIdentityBody("Not A Username!", "viewer"), status: http.StatusBadRequest,
			code: "invalid_username", mustSay: "username",
		},
		{
			name: "password below the accepted length", method: http.MethodPost, path: "/api/v1/users",
			body:   map[string]string{"username": "newcomer", "role": "viewer", "password": "short"},
			status: http.StatusBadRequest, code: "invalid_password", mustSay: "password",
		},
		{
			name: "username already used", method: http.MethodPost, path: "/api/v1/users",
			body: newIdentityBody("operator", "viewer"), status: http.StatusConflict,
			code: "username_taken", mustSay: "username",
		},
		{
			name: "identity that does not exist", method: http.MethodPut, path: "/api/v1/users/nobody/role",
			body: map[string]string{"role": "viewer"}, status: http.StatusNotFound, code: "user_not_found",
		},
		{
			name: "enabled state that is not a boolean", method: http.MethodPut,
			path: "/api/v1/users/operator/enabled", body: map[string]string{"enabled": "no"},
			status: http.StatusBadRequest, code: "invalid_enabled",
		},
		{
			name: "unknown token scope", method: http.MethodPost, path: "/api/v1/tokens",
			body:   map[string]any{"owner": "operator", "name": "bad", "scopes": []string{"hardware.flash"}},
			status: http.StatusBadRequest, code: "invalid_scope", mustSay: "scope",
		},
		{
			name: "unparseable expiry", method: http.MethodPost, path: "/api/v1/tokens",
			body:   map[string]any{"owner": "operator", "name": "bad", "expires_at": "next tuesday"},
			status: http.StatusBadRequest, code: "invalid_expiry", mustSay: "expiry",
		},
		{
			name: "expiry already past", method: http.MethodPost, path: "/api/v1/tokens",
			body:   map[string]any{"owner": "operator", "name": "bad", "expires_at": past},
			status: http.StatusBadRequest, code: "invalid_expiry", mustSay: "expiry",
		},
		{
			name: "token owner that does not exist", method: http.MethodPost, path: "/api/v1/tokens",
			body: map[string]any{"owner": "nobody", "name": "bad"}, status: http.StatusNotFound,
			code: "user_not_found",
		},
		{
			name: "token that does not exist", method: http.MethodPost,
			path: "/api/v1/tokens/0199aaaa-bbbb-cccc-dddd-eeeeeeeeeeee/revoke", body: nil,
			status: http.StatusNotFound, code: "token_not_found",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := f.sessionSend(t, test.method, test.path, test.body, session)
			if response.Code != test.status {
				t.Fatalf("%s %s = %d, want %d; body=%s", test.method, test.path, response.Code, test.status, response.Body.String())
			}
			body := decodeError(t, response)
			if body.Error != test.code {
				t.Fatalf("error code = %q, want %q", body.Error, test.code)
			}
			if test.mustSay != "" && !strings.Contains(strings.ToLower(body.Message), test.mustSay) {
				t.Fatalf("message %q does not name %q", body.Message, test.mustSay)
			}
			if strings.Contains(response.Body.String(), createdIdentityPassword) {
				t.Fatal("a refusal repeated the submitted password")
			}
			if strings.Contains(body.Message, "$argon2id$") || strings.Contains(body.Message, "sql") {
				t.Fatalf("the refusal leaked an internal detail: %q", body.Message)
			}
		})
	}

	// Nothing above may have changed anything.
	users := f.listUsers(t, session)
	if len(users) != 2 {
		t.Fatalf("identity listing after the refusals = %#v, want the two identities that existed", users)
	}
	if operator, _ := findUser(users, "operator"); operator.Role != "operator" || !operator.Enabled {
		t.Fatalf("a refusal changed the existing identity: %#v", operator)
	}
	if tokens := f.listTokens(t, session); len(tokens) != 0 {
		t.Fatalf("a refused token creation stored a token: %#v", tokens)
	}
}

func TestAdministrationRefusesRemovingTheLastAdministrator(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)

	for _, test := range []struct {
		name string
		path string
		body any
	}{
		{name: "demotion", path: "/api/v1/users/admin/role", body: map[string]string{"role": "viewer"}},
		{name: "disable", path: "/api/v1/users/admin/enabled", body: map[string]bool{"enabled": false}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := f.sessionSend(t, http.MethodPut, test.path, test.body, session)
			if response.Code != http.StatusConflict {
				t.Fatalf("%s of the last administrator = %d, want 409; body=%s", test.name, response.Code, response.Body.String())
			}
			if code := decodeError(t, response).Error; code != "last_administrator" {
				t.Fatalf("error code = %q, want last_administrator", code)
			}
		})
	}

	administrator, found := findUser(f.listUsers(t, session), "admin")
	if !found || administrator.Role != "administrator" || !administrator.Enabled {
		t.Fatalf("the last administrator changed: %#v", administrator)
	}
}

// administrationRoutes is every route this slice added, with the permission it
// requires. It drives the authorization, CSRF and attribution tables below.
var administrationRoutes = []struct {
	name       string
	method     string
	path       string
	body       any
	permission auth.Permission
	changes    bool
}{
	{
		name: "list identities", method: http.MethodGet, path: "/api/v1/users",
		permission: auth.PermissionUsersRead,
	},
	{
		name: "create identity", method: http.MethodPost, path: "/api/v1/users",
		body: newIdentityBody("intruder", "administrator"), permission: auth.PermissionUsersManage, changes: true,
	},
	{
		name: "change role", method: http.MethodPut, path: "/api/v1/users/subject/role",
		body: map[string]string{"role": "administrator"}, permission: auth.PermissionUsersManage, changes: true,
	},
	{
		name: "change enabled state", method: http.MethodPut, path: "/api/v1/users/subject/enabled",
		body: map[string]bool{"enabled": false}, permission: auth.PermissionUsersManage, changes: true,
	},
	{
		name: "reset password", method: http.MethodPut, path: "/api/v1/users/subject/password",
		body: map[string]string{"password": createdIdentityPassword}, permission: auth.PermissionUsersManage, changes: true,
	},
	{
		name: "list tokens", method: http.MethodGet, path: "/api/v1/tokens",
		permission: auth.PermissionTokensReadAll,
	},
	{
		name: "create token", method: http.MethodPost, path: "/api/v1/tokens",
		body: map[string]any{"owner": "subject", "name": "intruder"}, permission: auth.PermissionTokensManageAll, changes: true,
	},
	{
		name: "revoke token", method: http.MethodPost, path: "/api/v1/tokens/any-token/revoke",
		permission: auth.PermissionTokensManageAll, changes: true,
	},
	{
		name: "delete identity", method: http.MethodDelete, path: "/api/v1/users/subject",
		permission: auth.PermissionUsersManage, changes: true,
	},
	{
		name: "delete token", method: http.MethodDelete, path: "/api/v1/tokens/any-token",
		permission: auth.PermissionTokensManageAll, changes: true,
	},
}

func TestAdministrationRoutesSeparateUnauthenticatedFromForbidden(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	administrator := f.administratorSession(t)
	f.createUser(t, "subject", auth.RoleViewer, viewerPassword)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	operator := f.login(t, "operator", operatorPassword)
	viewer := f.login(t, "viewer", viewerPassword)
	// An administrator's token narrowed to a scope that excludes every
	// administration permission: a usable credential whose owner could
	// otherwise do all of this.
	narrowed := f.issueToken(t, "admin", "status-only", []auth.Permission{auth.PermissionStatusRead}, nil)

	for _, route := range administrationRoutes {
		route := route
		t.Run(route.name, func(t *testing.T) {
			unauthenticated := f.bearerSend(t, route.method, route.path, route.body, "")
			if unauthenticated.Code != http.StatusUnauthorized {
				t.Fatalf("no credential = %d, want 401; body=%s", unauthenticated.Code, unauthenticated.Body.String())
			}
			if unauthenticated.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("the unauthenticated refusal offered no authentication challenge")
			}

			for name, session := range map[string]loginResult{"operator": operator, "viewer": viewer} {
				refused := f.sessionSend(t, route.method, route.path, route.body, session)
				if refused.Code != http.StatusForbidden {
					t.Fatalf("%s session = %d, want 403; body=%s", name, refused.Code, refused.Body.String())
				}
				if refused.Header().Get("WWW-Authenticate") != "" {
					t.Fatalf("the %s refusal offered an authentication challenge", name)
				}
			}

			scoped := f.bearerSend(t, route.method, route.path, route.body, narrowed)
			if scoped.Code != http.StatusForbidden {
				t.Fatalf("narrowed administrator token = %d, want 403; body=%s", scoped.Code, scoped.Body.String())
			}
		})
	}

	users := f.listUsers(t, administrator)
	if len(users) != 4 {
		t.Fatalf("a refused request changed the identity listing: %#v", users)
	}
	if subject, _ := findUser(users, "subject"); subject.Role != "viewer" || !subject.Enabled {
		t.Fatalf("a refused request changed the subject identity: %#v", subject)
	}
	if tokens := f.listTokens(t, administrator); len(tokens) != 1 {
		t.Fatalf("a refused request created a token: %#v", tokens)
	}
}

func TestAdministrationRoutesRequireTheSessionCSRFToken(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "subject", auth.RoleViewer, viewerPassword)

	for _, route := range administrationRoutes {
		route := route
		if !route.changes {
			continue
		}
		t.Run(route.name, func(t *testing.T) {
			request := f.jsonRequest(t, route.method, route.path, route.body)
			request.AddCookie(session.sessionCookie)
			if response := f.do(request); response.Code != http.StatusForbidden {
				t.Fatalf("%s without a CSRF token = %d, want 403; body=%s",
					route.name, response.Code, response.Body.String())
			}
		})
	}

	users := f.listUsers(t, session)
	if len(users) != 2 {
		t.Fatalf("a request without a CSRF token changed the identity listing: %#v", users)
	}
	if subject, _ := findUser(users, "subject"); subject.Role != "viewer" || !subject.Enabled {
		t.Fatalf("a request without a CSRF token changed the subject identity: %#v", subject)
	}
}

func TestAdministrationAttributesChangesToTheAuthenticatedActor(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "second-admin", auth.RoleAdministrator, administratorPassword)
	acting := f.login(t, "second-admin", administratorPassword)

	// The body names a different actor on purpose: an actor a client can set is
	// an impersonation control, so the field has to be inert.
	response := f.sessionSend(t, http.MethodPost, "/api/v1/users", map[string]string{
		"username": "jane", "role": "viewer", "password": createdIdentityPassword,
		"actor": "admin", "actor_id": f.administrator.ID,
	}, acting)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/users = %d, want 201; body=%s", response.Code, response.Body.String())
	}

	events := listAuditEvents(t, f.database, "identity.user.created")
	if len(events) == 0 {
		t.Fatal("the identity creation recorded no audit event")
	}
	latest := events[len(events)-1]
	actor, err := f.service.UserByUsername(context.Background(), "second-admin")
	if err != nil {
		t.Fatalf("resolve the acting identity: %v", err)
	}
	if latest.ActorID.String() != actor.ID {
		t.Fatalf("the change was attributed to %s, want the authenticated identity %s", latest.ActorID, actor.ID)
	}
	if latest.ActorID.String() == f.administrator.ID {
		t.Fatal("the change was attributed to the actor named in the request body")
	}

	secret := f.issueToken(t, "second-admin", "administration", []auth.Permission{auth.PermissionUsersManage}, nil)
	var tokenID string
	for _, token := range f.listTokens(t, acting) {
		if token.Name == "administration" {
			tokenID = token.ID
			break
		}
	}
	if tokenID == "" {
		t.Fatal("the acting API token was not listed")
	}
	tokenResponse := f.bearerSend(t, http.MethodPost, "/api/v1/users", map[string]string{
		"username": "token-jane", "role": "viewer", "password": createdIdentityPassword,
		"actor": "admin", "actor_id": f.administrator.ID, "actor_kind": "user",
	}, secret)
	if tokenResponse.Code != http.StatusCreated {
		t.Fatalf("token-authenticated POST /api/v1/users = %d, want 201; body=%s", tokenResponse.Code, tokenResponse.Body.String())
	}
	tokenEvents := listAuditEvents(t, f.database, "identity.user.created")
	tokenEvent := tokenEvents[len(tokenEvents)-1]
	if tokenEvent.ActorKind != audit.ActorKindToken || tokenEvent.ActorID.String() != tokenID {
		t.Fatalf("token-authenticated actor = %s/%s, want token/%s", tokenEvent.ActorKind, tokenEvent.ActorID, tokenID)
	}

	// A non-administrator naming an administrator gets nowhere either.
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	operator := f.login(t, "operator", operatorPassword)
	refused := f.sessionSend(t, http.MethodPost, "/api/v1/users", map[string]string{
		"username": "smuggled", "role": "administrator", "password": createdIdentityPassword,
		"actor": "admin",
	}, operator)
	if refused.Code != http.StatusForbidden {
		t.Fatalf("an operator naming an administrator = %d, want 403", refused.Code)
	}
	if _, found := findUser(f.listUsers(t, acting), "smuggled"); found {
		t.Fatal("a refused request created the identity anyway")
	}
}

func TestAdministrationResponsesCarryNoVerifiers(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)

	created := f.sessionSend(t, http.MethodPost, "/api/v1/users", newIdentityBody("jane", "operator"), session)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/users = %d; body=%s", created.Code, created.Body.String())
	}
	token := f.sessionSend(t, http.MethodPost, "/api/v1/tokens",
		map[string]any{"owner": "jane", "name": "bench-ci"}, session)
	if token.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/tokens = %d; body=%s", token.Code, token.Body.String())
	}

	bodies := map[string]string{
		"created identity": created.Body.String(),
		"created token":    token.Body.String(),
		"identity listing": f.sessionSend(t, http.MethodGet, "/api/v1/users", nil, session).Body.String(),
		"token listing":    f.sessionSend(t, http.MethodGet, "/api/v1/tokens", nil, session).Body.String(),
		"current session":  f.sessionSend(t, http.MethodGet, "/api/v1/auth/session", nil, session).Body.String(),
	}
	for where, body := range bodies {
		for _, forbidden := range []string{"$argon2id$", "password_hash", "verifier", session.sessionCookie.Value} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("the %s response carried %q", where, forbidden)
			}
		}
	}
}

// listAuditEvents reads the durable audit history the identity service wrote
// inside the transaction of the change it attributes.
func listAuditEvents(t *testing.T, database *sqlite.DB, action string) []audit.Event {
	t.Helper()
	events, err := sqlite.NewAuditRepository(database).List(
		context.Background(), audit.Filter{Action: action},
	)
	if err != nil {
		t.Fatalf("list audit events %q: %v", action, err)
	}
	return events
}

// Permanent deletion over the routes an administrator's browser actually
// reaches. Everything the identity service guarantees is restated here as an
// HTTP outcome, because a client can only see status codes and error codes.

func (f apiFixture) deleteUser(t *testing.T, username string, session loginResult) *httptest.ResponseRecorder {
	t.Helper()
	return f.sessionSend(t, http.MethodDelete, "/api/v1/users/"+username, nil, session)
}

func (f apiFixture) deleteToken(t *testing.T, id string, session loginResult) *httptest.ResponseRecorder {
	t.Helper()
	return f.sessionSend(t, http.MethodDelete, "/api/v1/tokens/"+id, nil, session)
}

func TestUserDeletionRemovesTheIdentityAndItsSessions(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	operator := f.login(t, "operator", operatorPassword)
	if reachable := f.sessionGet("/api/v1/status", operator); reachable.Code != http.StatusOK {
		t.Fatalf("the operator session does not work before the deletion: %d", reachable.Code)
	}

	removed := f.deleteUser(t, "operator", session)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/v1/users/operator = %d, want 204; body=%s", removed.Code, removed.Body.String())
	}
	if body := removed.Body.String(); strings.TrimSpace(body) != "" {
		t.Fatalf("a deletion answered with a body: %q", body)
	}

	if _, found := findUser(f.listUsers(t, session), "operator"); found {
		t.Fatal("the deleted identity is still listed")
	}
	if stranded := f.sessionGet("/api/v1/status", operator); stranded.Code != http.StatusUnauthorized {
		t.Fatalf("the session of a deleted identity still authenticates: %d", stranded.Code)
	}
	if attempt := f.attemptLogin("operator", operatorPassword); attempt.Code != http.StatusUnauthorized {
		t.Fatalf("a deleted identity could sign in again: %d", attempt.Code)
	}
	repeated := f.deleteUser(t, "operator", session)
	if repeated.Code != http.StatusNotFound {
		t.Fatalf("deleting the same identity twice = %d, want 404", repeated.Code)
	}
	if code := decodeError(t, repeated).Error; code != "user_not_found" {
		t.Fatalf("error code = %q, want user_not_found", code)
	}
}

func TestUserDeletionIsRefusedWhileTheIdentityOwnsTokens(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	f.issueToken(t, "operator", "owned", nil, nil)
	owned, found := findToken(f.listTokens(t, session), "owned")
	if !found {
		t.Fatal("the created token is missing from the listing")
	}

	refused := f.deleteUser(t, "operator", session)
	if refused.Code != http.StatusConflict {
		t.Fatalf("deleting an identity that owns a token = %d, want 409; body=%s", refused.Code, refused.Body.String())
	}
	failure := decodeError(t, refused)
	if failure.Error != "user_has_tokens" {
		t.Fatalf("error code = %q, want user_has_tokens", failure.Error)
	}
	if !strings.Contains(failure.Message, "token") {
		t.Fatalf("the refusal does not say what to do about it: %q", failure.Message)
	}
	if _, still := findUser(f.listUsers(t, session), "operator"); !still {
		t.Fatal("a refused deletion removed the identity anyway")
	}
	if tokens := f.listTokens(t, session); len(tokens) != 1 {
		t.Fatalf("a refused deletion cascaded to the tokens: %#v", tokens)
	}

	// Revoking is not enough: the record still names this owner.
	if revoked := f.sessionSend(
		t, http.MethodPost, "/api/v1/tokens/"+owned.ID+"/revoke", nil, session,
	); revoked.Code != http.StatusNoContent {
		t.Fatalf("POST revoke = %d, want 204", revoked.Code)
	}
	if stillRefused := f.deleteUser(t, "operator", session); stillRefused.Code != http.StatusConflict {
		t.Fatalf("deleting an identity that owns a revoked token = %d, want 409", stillRefused.Code)
	}

	if gone := f.deleteToken(t, owned.ID, session); gone.Code != http.StatusNoContent {
		t.Fatalf("DELETE the owned token = %d, want 204; body=%s", gone.Code, gone.Body.String())
	}
	if applied := f.deleteUser(t, "operator", session); applied.Code != http.StatusNoContent {
		t.Fatalf("deleting the identity once its token was removed = %d, want 204; body=%s",
			applied.Code, applied.Body.String())
	}
}

func TestTokenDeletionRemovesActiveExpiredAndRevokedRecords(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	expiry := f.clock.Now().Add(time.Hour)
	activeSecret := f.issueToken(t, "admin", "active", nil, nil)
	revokedSecret := f.issueToken(t, "admin", "revoked", nil, nil)
	expiringSecret := f.issueToken(t, "admin", "expiring", nil, &expiry)
	listed := f.listTokens(t, session)
	revoked, _ := findToken(listed, "revoked")
	if response := f.sessionSend(
		t, http.MethodPost, "/api/v1/tokens/"+revoked.ID+"/revoke", nil, session,
	); response.Code != http.StatusNoContent {
		t.Fatalf("POST revoke = %d, want 204", response.Code)
	}
	if used := f.get("/api/v1/status", activeSecret); used.Code != http.StatusOK {
		t.Fatalf("the active token does not work before its deletion: %d", used.Code)
	}
	// Past the expiry but well inside the session lifetime, so the browser
	// credential this test administers with is untouched.
	f.clock.Advance(2 * time.Hour)

	secrets := map[string]string{"active": activeSecret, "revoked": revokedSecret, "expiring": expiringSecret}
	for _, name := range []string{"active", "revoked", "expiring"} {
		token, found := findToken(f.listTokens(t, session), name)
		if !found {
			t.Fatalf("the %s token is missing from the listing", name)
		}
		response := f.deleteToken(t, token.ID, session)
		if response.Code != http.StatusNoContent {
			t.Fatalf("DELETE the %s token = %d, want 204; body=%s", name, response.Code, response.Body.String())
		}
		if _, still := findToken(f.listTokens(t, session), name); still {
			t.Fatalf("the deleted %s token is still listed", name)
		}
		if used := f.get("/api/v1/status", secrets[name]); used.Code != http.StatusUnauthorized {
			t.Fatalf("the secret of the deleted %s token still authenticates: %d", name, used.Code)
		}
		repeated := f.deleteToken(t, token.ID, session)
		if repeated.Code != http.StatusNotFound {
			t.Fatalf("deleting the %s token twice = %d, want 404", name, repeated.Code)
		}
		if code := decodeError(t, repeated).Error; code != "token_not_found" {
			t.Fatalf("error code = %q, want token_not_found", code)
		}
	}
	if tokens := f.listTokens(t, session); len(tokens) != 0 {
		t.Fatalf("tokens after deleting every one of them = %#v", tokens)
	}
}

func TestDeletingTheLastAdministratorIsRefusedAndASecondOneMayGo(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)

	refused := f.deleteUser(t, "admin", session)
	if refused.Code != http.StatusConflict {
		t.Fatalf("deleting the last administrator = %d, want 409; body=%s", refused.Code, refused.Body.String())
	}
	if code := decodeError(t, refused).Error; code != "last_administrator" {
		t.Fatalf("error code = %q, want last_administrator", code)
	}
	if administrator, found := findUser(f.listUsers(t, session), "admin"); !found || !administrator.Enabled {
		t.Fatalf("the refused deletion changed the last administrator: %#v", administrator)
	}

	f.createUser(t, "second-admin", auth.RoleAdministrator, administratorPassword)
	if applied := f.deleteUser(t, "second-admin", session); applied.Code != http.StatusNoContent {
		t.Fatalf("deleting a second administrator = %d, want 204; body=%s", applied.Code, applied.Body.String())
	}
	if again := f.deleteUser(t, "admin", session); again.Code != http.StatusConflict {
		t.Fatalf("deleting the administrator that is last again = %d, want 409", again.Code)
	}
}

// TestSelfDeletionEndsTheAdministratorsOwnSession is the case the browser has
// to survive: the administrator removes their own account and the credential
// they were holding stops working, which is what returns the page to sign-in.
func TestSelfDeletionEndsTheAdministratorsOwnSession(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "second-admin", auth.RoleAdministrator, administratorPassword)
	acting := f.login(t, "second-admin", administratorPassword)

	response := f.deleteUser(t, "second-admin", acting)
	if response.Code != http.StatusNoContent {
		t.Fatalf("self deletion = %d, want 204; body=%s", response.Code, response.Body.String())
	}
	if after := f.sessionGet("/api/v1/status", acting); after.Code != http.StatusUnauthorized {
		t.Fatalf("the self-deleted administrator's session still authenticates: %d", after.Code)
	}
	// The appliance is still administrable, which is the whole point of
	// allowing this only while another enabled administrator remains.
	remaining := f.administratorSession(t)
	if _, found := findUser(f.listUsers(t, remaining), "second-admin"); found {
		t.Fatal("the self-deleted identity is still listed")
	}
}

// TestDeletionsAreAttributedToTheAuthenticatedActor proves the audit actor is
// taken from the credential and never from anything the client can set, for
// both credential kinds.
func TestDeletionsAreAttributedToTheAuthenticatedActor(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "second-admin", auth.RoleAdministrator, administratorPassword)
	acting := f.login(t, "second-admin", administratorPassword)
	actor, err := f.service.UserByUsername(context.Background(), "second-admin")
	if err != nil {
		t.Fatalf("resolve the acting identity: %v", err)
	}
	subject := f.createUser(t, "subject", auth.RoleViewer, viewerPassword)

	removed := f.sessionSend(t, http.MethodDelete, "/api/v1/users/subject", map[string]string{
		"actor": "admin", "actor_id": f.administrator.ID, "actor_kind": "system",
	}, acting)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE /api/v1/users/subject = %d, want 204; body=%s", removed.Code, removed.Body.String())
	}
	events := listAuditEvents(t, f.database, "identity.user.deleted")
	if len(events) != 1 {
		t.Fatalf("the identity deletion recorded %d events, want exactly 1", len(events))
	}
	deleted := events[0]
	if deleted.ActorKind != audit.ActorKindUser || deleted.ActorID.String() != actor.ID {
		t.Fatalf("the deletion was attributed to %s/%s, want user/%s",
			deleted.ActorKind, deleted.ActorID, actor.ID)
	}
	if deleted.SubjectType != "user" || deleted.SubjectID != subject.ID {
		t.Fatalf("deletion subject = %s/%s, want user/%s", deleted.SubjectType, deleted.SubjectID, subject.ID)
	}

	// The same removal through a bearer token is attributed to that token,
	// which is a different kind of actor and a different identity.
	secret := f.issueToken(t, "second-admin", "administration",
		[]auth.Permission{auth.PermissionTokensManageAll, auth.PermissionTokensReadAll}, nil)
	f.issueToken(t, "second-admin", "retired", nil, nil)
	var actingTokenID, retiredTokenID string
	for _, token := range f.listTokens(t, acting) {
		switch token.Name {
		case "administration":
			actingTokenID = token.ID
		case "retired":
			retiredTokenID = token.ID
		}
	}
	if actingTokenID == "" || retiredTokenID == "" {
		t.Fatal("the acting or the subject token was not listed")
	}
	response := f.bearerSend(t, http.MethodDelete, "/api/v1/tokens/"+retiredTokenID, nil, secret)
	if response.Code != http.StatusNoContent {
		t.Fatalf("token-authenticated DELETE = %d, want 204; body=%s", response.Code, response.Body.String())
	}
	tokenEvents := listAuditEvents(t, f.database, "identity.token.deleted")
	if len(tokenEvents) != 1 {
		t.Fatalf("the token deletion recorded %d events, want exactly 1", len(tokenEvents))
	}
	if tokenEvents[0].ActorKind != audit.ActorKindToken || tokenEvents[0].ActorID.String() != actingTokenID {
		t.Fatalf("the token deletion was attributed to %s/%s, want token/%s",
			tokenEvents[0].ActorKind, tokenEvents[0].ActorID, actingTokenID)
	}
}

// TestRefusedAndReadRequestsRecordNoDeletionEvent keeps the deletion actions
// meaning what they say over HTTP as well.
func TestRefusedAndReadRequestsRecordNoDeletionEvent(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	f.issueToken(t, "operator", "owned", nil, nil)
	viewer := f.login(t, "viewer", viewerPassword)

	// A refusal of every kind the two routes can produce, then two reads.
	f.deleteUser(t, "admin", session)
	f.deleteUser(t, "operator", session)
	f.deleteUser(t, "nobody", session)
	f.deleteToken(t, "no-such-token", session)
	f.deleteUser(t, "operator", viewer)
	f.bearerSend(t, http.MethodDelete, "/api/v1/users/operator", nil, "")
	f.listUsers(t, session)
	f.listTokens(t, session)

	for _, action := range []string{"identity.user.deleted", "identity.token.deleted"} {
		if events := listAuditEvents(t, f.database, action); len(events) != 0 {
			t.Fatalf("%d %q events were recorded for refused or read requests", len(events), action)
		}
	}
	if _, found := findUser(f.listUsers(t, session), "operator"); !found {
		t.Fatal("a refused deletion removed the identity anyway")
	}
}

// TestActivityFindsDeletionEventsAfterTheirSubjectsAreGone reads the history
// the way the Authentication page's "View activity" does, and exports it,
// after the identity and the token it names no longer exist.
func TestActivityFindsDeletionEventsAfterTheirSubjectsAreGone(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	f.issueToken(t, "operator", "retired", nil, nil)
	retired, _ := findToken(f.listTokens(t, session), "retired")
	if response := f.deleteToken(t, retired.ID, session); response.Code != http.StatusNoContent {
		t.Fatalf("DELETE the token = %d; body=%s", response.Code, response.Body.String())
	}
	if response := f.deleteUser(t, "operator", session); response.Code != http.StatusNoContent {
		t.Fatalf("DELETE the identity = %d; body=%s", response.Code, response.Body.String())
	}

	listing := f.sessionGet("/api/v1/activity?component=identity&limit=200", session)
	if listing.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/activity = %d; body=%s", listing.Code, listing.Body.String())
	}
	body := listing.Body.String()
	for _, action := range []string{"identity.user.deleted", "identity.token.deleted"} {
		if !strings.Contains(body, action) {
			t.Fatalf("component=identity did not list %q: %s", action, body)
		}
	}
	if !strings.Contains(body, retired.ID) {
		t.Fatal("the deleted token's history no longer names the token it was about")
	}

	for _, format := range []string{"jsonl", "csv"} {
		export := f.sessionGet("/api/v1/activity/export?component=identity&format="+format, session)
		if export.Code != http.StatusOK {
			t.Fatalf("GET the %s export = %d; body=%s", format, export.Code, export.Body.String())
		}
		for _, action := range []string{"identity.user.deleted", "identity.token.deleted"} {
			if !strings.Contains(export.Body.String(), action) {
				t.Fatalf("the %s export did not carry %q", format, action)
			}
		}
	}
}

// TestDeletionResponsesAndHistoryCarryNoSecrets is the canary: a token secret
// and a session secret exist in this test, and neither may appear anywhere a
// deletion writes or answers.
func TestDeletionResponsesAndHistoryCarryNoSecrets(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.administratorSession(t)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	secret := f.issueToken(t, "operator", "retired", nil, nil)
	retired, _ := findToken(f.listTokens(t, session), "retired")

	answers := map[string]string{
		"token deletion":    f.deleteToken(t, retired.ID, session).Body.String(),
		"identity deletion": f.deleteUser(t, "operator", session).Body.String(),
		"repeated deletion": f.deleteUser(t, "operator", session).Body.String(),
		"identity listing":  f.sessionSend(t, http.MethodGet, "/api/v1/users", nil, session).Body.String(),
		"token listing":     f.sessionSend(t, http.MethodGet, "/api/v1/tokens", nil, session).Body.String(),
		"identity activity": f.sessionGet("/api/v1/activity?component=identity&limit=200", session).Body.String(),
		"activity export":   f.sessionGet("/api/v1/activity/export?component=identity", session).Body.String(),
	}
	canaries := []string{
		secret, operatorPassword, administratorPassword, session.sessionCookie.Value,
		"$argon2id$", "password_hash", "verifier",
	}
	for where, body := range answers {
		for _, canary := range canaries {
			if strings.Contains(body, canary) {
				t.Fatalf("the %s response carried %q", where, canary)
			}
		}
	}

	// And the same canaries against the stored history itself, which outlives
	// everything the responses above described.
	for _, action := range []string{"identity.user.deleted", "identity.token.deleted"} {
		for _, event := range listAuditEvents(t, f.database, action) {
			for _, canary := range canaries {
				if strings.Contains(string(event.Payload), canary) {
					t.Fatalf("the %s event carried %q: %s", action, canary, event.Payload)
				}
			}
		}
	}
}
