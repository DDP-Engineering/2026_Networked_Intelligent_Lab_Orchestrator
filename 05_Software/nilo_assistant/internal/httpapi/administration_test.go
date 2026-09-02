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
