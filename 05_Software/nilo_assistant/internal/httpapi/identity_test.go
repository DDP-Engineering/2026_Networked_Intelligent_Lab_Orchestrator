package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

const (
	administratorPassword = "administrator-secret-1"
	operatorPassword      = "operator-secret-1"
	viewerPassword        = "viewer-secret-0001"
)

type apiClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *apiClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.now
}

func (clock *apiClock) Advance(delta time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.now = clock.now.Add(delta)
}

type apiFixture struct {
	handler       http.Handler
	service       *identity.Service
	clock         *apiClock
	administrator auth.User
	database      *sqlite.DB
}

// newAPIFixture builds the production handler over a real database so that
// every authorization case is proven through the routes the server serves.
// Each adjustment refines the identity service the handler is given, which is
// how a test injects a randomness source it can hold a login inside.
func newAPIFixture(
	t *testing.T, scenario simulation.Scenario, cookies CookieSecurity, adjust ...func(*identity.Options),
) apiFixture {
	t.Helper()
	database, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "nilo.db"))
	if err != nil {
		t.Fatalf("open API database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	clock := &apiClock{now: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)}
	options := identity.Options{
		Repository:         sqlite.NewIdentityRepository(database),
		Now:                clock.Now,
		Random:             rand.Reader,
		SessionLifetime:    12 * time.Hour,
		LoginFailureLimit:  5,
		LoginFailureWindow: 15 * time.Minute,
	}
	for _, refine := range adjust {
		refine(&options)
	}
	service, err := identity.NewService(options)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	administrator, err := service.Bootstrap(context.Background(), "admin", []byte(administratorPassword))
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	catalog, err := simulation.NewCatalog(scenario)
	if err != nil {
		t.Fatalf("NewCatalog(%q): %v", scenario, err)
	}
	handler, err := NewHandler(HandlerOptions{
		Identity: service, Profile: "simulation", Catalog: catalog, CookieSecurity: cookies,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return apiFixture{
		handler: handler, service: service, clock: clock,
		administrator: administrator, database: database,
	}
}

func (f apiFixture) createUser(t *testing.T, username string, role auth.Role, password string) auth.User {
	t.Helper()
	user, err := f.service.CreateUser(context.Background(), f.administrator, username, role, []byte(password))
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", username, err)
	}
	return user
}

func (f apiFixture) issueToken(t *testing.T, owner, name string, scopes []auth.Permission, expiresAt *time.Time) string {
	t.Helper()
	issued, err := f.service.CreateToken(context.Background(), f.administrator, owner, name, scopes, expiresAt)
	if err != nil {
		t.Fatalf("CreateToken(%q): %v", name, err)
	}
	return issued.Secret
}

func (f apiFixture) do(request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

func (f apiFixture) get(path, bearer string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	return f.do(request)
}

type loginResult struct {
	sessionCookie *http.Cookie
	csrfToken     string
	body          map[string]any
}

func (f apiFixture) login(t *testing.T, username, password string) loginResult {
	t.Helper()
	response := f.attemptLogin(username, password)
	if response.Code != http.StatusOK {
		t.Fatalf("login as %q returned %d: %s", username, response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	result := loginResult{body: body}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName {
			result.sessionCookie = cookie
		}
	}
	if result.sessionCookie == nil {
		t.Fatal("login returned no session cookie")
	}
	token, ok := body["csrf_token"].(string)
	if !ok || token == "" {
		t.Fatalf("login returned no CSRF token: %v", body)
	}
	result.csrfToken = token
	return result
}

func (f apiFixture) attemptLogin(username, password string) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(map[string]string{"username": username, "password": password})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	return f.do(request)
}

func (f apiFixture) sessionGet(path string, session loginResult) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(session.sessionCookie)
	return f.do(request)
}

func TestProtectedRoutesSeparateUnauthenticatedFromForbidden(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	viewerToken := f.issueToken(t, "viewer", "viewer-token", nil, nil)
	administratorToken := f.issueToken(t, "admin", "admin-token", nil, nil)

	unauthenticated := f.get("/api/v1/users", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("no credential = %d, want 401", unauthenticated.Code)
	}
	if unauthenticated.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("401 WWW-Authenticate = %q, want Bearer", unauthenticated.Header().Get("WWW-Authenticate"))
	}
	if strings.Contains(unauthenticated.Body.String(), "username") {
		t.Fatalf("401 leaked the protected representation: %s", unauthenticated.Body.String())
	}

	forbidden := f.get("/api/v1/users", viewerToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer credential = %d, want 403; body=%s", forbidden.Code, forbidden.Body.String())
	}
	if forbidden.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("403 offered an authentication challenge")
	}
	if strings.Contains(forbidden.Body.String(), "username") {
		t.Fatalf("403 leaked the protected representation: %s", forbidden.Body.String())
	}
	if !strings.Contains(forbidden.Body.String(), "forbidden") {
		t.Fatalf("403 body = %s, want a forbidden marker", forbidden.Body.String())
	}

	permitted := f.get("/api/v1/users", administratorToken)
	if permitted.Code != http.StatusOK {
		t.Fatalf("administrator credential = %d, want 200; body=%s", permitted.Code, permitted.Body.String())
	}
	var listing struct {
		Items []struct {
			Username string `json:"username"`
			Role     string `json:"role"`
			Enabled  bool   `json:"enabled"`
		} `json:"items"`
	}
	if err := json.Unmarshal(permitted.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode user listing: %v", err)
	}
	if len(listing.Items) != 2 {
		t.Fatalf("user listing = %#v, want the two created identities", listing.Items)
	}
}

func TestViewerMayReadStatusAndInventoryButNotUsers(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	viewerToken := f.issueToken(t, "viewer", "viewer-token", nil, nil)

	for _, path := range []string{"/api/v1/status", "/api/v1/integrations", "/api/v1/devices"} {
		if response := f.get(path, viewerToken); response.Code != http.StatusOK {
			t.Fatalf("viewer GET %s = %d, want 200; body=%s", path, response.Code, response.Body.String())
		}
	}
	if response := f.get("/api/v1/users", viewerToken); response.Code != http.StatusForbidden {
		t.Fatalf("viewer GET /api/v1/users = %d, want 403", response.Code)
	}
}

func TestScopedTokenLosesTheRoutesItsScopesExclude(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	narrow := f.issueToken(t, "admin", "status-only", []auth.Permission{auth.PermissionStatusRead}, nil)

	if response := f.get("/api/v1/status", narrow); response.Code != http.StatusOK {
		t.Fatalf("scoped GET /api/v1/status = %d, want 200", response.Code)
	}
	for _, path := range []string{"/api/v1/devices", "/api/v1/users"} {
		if response := f.get(path, narrow); response.Code != http.StatusForbidden {
			t.Fatalf("scoped GET %s = %d, want 403", path, response.Code)
		}
	}
}

func TestUnusableBearerCredentialsAreRejectedAsUnauthorized(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	expiry := f.clock.Now().Add(time.Hour)
	expiring := f.issueToken(t, "admin", "expiring", nil, &expiry)
	revoked := f.issueToken(t, "admin", "revoked", nil, nil)
	orphaned := f.issueToken(t, "operator", "orphaned", nil, nil)
	surviving := f.issueToken(t, "admin", "surviving", nil, nil)

	views, err := f.service.ListTokens(context.Background())
	if err != nil {
		t.Fatalf("ListTokens: %v", err)
	}
	for _, view := range views {
		if view.Token.Name == "revoked" {
			if err := f.service.RevokeToken(context.Background(), f.administrator, view.Token.ID); err != nil {
				t.Fatalf("RevokeToken: %v", err)
			}
		}
	}
	if err := f.service.SetEnabled(context.Background(), f.administrator, "operator", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	f.clock.Advance(time.Hour)

	for _, test := range []struct {
		name   string
		bearer string
	}{
		{name: "expired", bearer: expiring},
		{name: "revoked", bearer: revoked},
		{name: "disabled owner", bearer: orphaned},
		{name: "unknown", bearer: "nilo_unknown"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := f.get("/api/v1/status", test.bearer)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s bearer = %d, want 401", test.name, response.Code)
			}
			if strings.Contains(response.Body.String(), "simulation") {
				t.Fatalf("%s bearer leaked the protected status: %s", test.name, response.Body.String())
			}
		})
	}
	if response := f.get("/api/v1/status", surviving); response.Code != http.StatusOK {
		t.Fatalf("an unaffected token stopped working: %d", response.Code)
	}
}

func TestLoginIssuesAScriptInaccessibleSessionCookieAndAnExpiringSession(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.login(t, "admin", administratorPassword)

	if !session.sessionCookie.HttpOnly {
		t.Fatal("the session cookie is readable by scripts")
	}
	if session.sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie SameSite = %v, want Strict", session.sessionCookie.SameSite)
	}
	if response := f.sessionGet("/api/v1/status", session); response.Code != http.StatusOK {
		t.Fatalf("session GET /api/v1/status = %d, want 200; body=%s", response.Code, response.Body.String())
	}

	f.clock.Advance(12 * time.Hour)

	if response := f.sessionGet("/api/v1/status", session); response.Code != http.StatusUnauthorized {
		t.Fatalf("expired session GET = %d, want 401", response.Code)
	}
}

func TestLoginRejectsWrongCredentialsWithoutRevealingWhichPartFailed(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)

	wrongPassword := f.attemptLogin("admin", "not-the-password-1")
	unknownUser := f.attemptLogin("nobody", administratorPassword)

	for name, response := range map[string]*httptest.ResponseRecorder{
		"wrong password": wrongPassword,
		"unknown user":   unknownUser,
	} {
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s login = %d, want 401", name, response.Code)
		}
		if len(response.Result().Cookies()) != 0 {
			t.Fatalf("%s login set a cookie", name)
		}
	}
	if wrongPassword.Body.String() != unknownUser.Body.String() {
		t.Fatalf("login responses differ between a wrong password and an unknown user: %s vs %s",
			wrongPassword.Body.String(), unknownUser.Body.String())
	}
}

func TestSessionCookieSecurityFollowsTheDeclaredDeploymentMode(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		mode       CookieSecurity
		wantSecure bool
	}{
		{name: "development localhost", mode: CookieDevelopmentLocalhost, wantSecure: false},
		{name: "https deployment", mode: CookieHTTPSDeployment, wantSecure: true},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			f := newAPIFixture(t, simulation.ScenarioHealthy, test.mode)
			session := f.login(t, "admin", administratorPassword)
			if session.sessionCookie.Secure != test.wantSecure {
				t.Fatalf("session cookie Secure = %v, want %v", session.sessionCookie.Secure, test.wantSecure)
			}
			if !session.sessionCookie.HttpOnly {
				t.Fatal("the session cookie is readable by scripts")
			}
		})
	}
}

func TestStateChangingSessionRequestsRequireTheMatchingCSRFToken(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.login(t, "admin", administratorPassword)
	other := f.login(t, "admin", administratorPassword)

	for _, test := range []struct {
		name  string
		token string
	}{
		{name: "missing"},
		{name: "another session's token", token: other.csrfToken},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
			request.AddCookie(session.sessionCookie)
			if test.token != "" {
				request.Header.Set(auth.CSRFHeaderName, test.token)
			}
			if response := f.do(request); response.Code != http.StatusForbidden {
				t.Fatalf("logout with a %s CSRF token = %d, want 403", test.name, response.Code)
			}
			if response := f.sessionGet("/api/v1/status", session); response.Code != http.StatusOK {
				t.Fatalf("a rejected request invalidated the session: %d", response.Code)
			}
		})
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(session.sessionCookie)
	request.Header.Set(auth.CSRFHeaderName, session.csrfToken)
	if response := f.do(request); response.Code != http.StatusNoContent {
		t.Fatalf("logout with the correct CSRF token = %d, want 204", response.Code)
	}
}

func TestLogoutInvalidatesTheSessionAndClearsItsCookie(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	session := f.login(t, "admin", administratorPassword)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(session.sessionCookie)
	request.Header.Set(auth.CSRFHeaderName, session.csrfToken)
	response := f.do(request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204; body=%s", response.Code, response.Body.String())
	}
	cleared := false
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout did not clear the session cookie")
	}
	if reused := f.sessionGet("/api/v1/status", session); reused.Code != http.StatusUnauthorized {
		t.Fatalf("the logged-out cookie still authenticates: %d", reused.Code)
	}
}

func TestSessionRouteReportsTheActorRoleAndEffectivePermissions(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	session := f.login(t, "operator", operatorPassword)

	var current struct {
		APIVersion  string   `json:"api_version"`
		Username    string   `json:"username"`
		Role        string   `json:"role"`
		Permissions []string `json:"permissions"`
		Credential  string   `json:"credential"`
	}
	response := f.sessionGet("/api/v1/auth/session", session)
	if response.Code != http.StatusOK {
		t.Fatalf("session route = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if current.APIVersion != "nilo.http/1" || current.Username != "operator" || current.Role != "operator" {
		t.Fatalf("session response = %#v", current)
	}
	if current.Credential != "session" {
		t.Fatalf("session credential = %q, want session", current.Credential)
	}
	if len(current.Permissions) != len(auth.RoleOperator.Permissions()) {
		t.Fatalf("session permissions = %v, want the Operator set", current.Permissions)
	}

	if err := f.service.SetRole(context.Background(), f.administrator, "operator", auth.RoleViewer); err != nil {
		t.Fatalf("SetRole: %v", err)
	}
	response = f.sessionGet("/api/v1/auth/session", session)
	if response.Code != http.StatusOK {
		t.Fatalf("session route after the role change = %d", response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode session response after the role change: %v", err)
	}
	if current.Role != "viewer" || len(current.Permissions) != len(auth.RoleViewer.Permissions()) {
		t.Fatalf("session response after the role change = %#v", current)
	}
}

func TestDisablingAnIdentityStopsItsSessionAtTheHTTPBoundary(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	session := f.login(t, "operator", operatorPassword)
	if response := f.sessionGet("/api/v1/status", session); response.Code != http.StatusOK {
		t.Fatalf("session GET before disabling = %d", response.Code)
	}

	if err := f.service.SetEnabled(context.Background(), f.administrator, "operator", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	if response := f.sessionGet("/api/v1/status", session); response.Code != http.StatusUnauthorized {
		t.Fatalf("a disabled identity's session GET = %d, want 401", response.Code)
	}
}

// gatedRandom holds the first read taken after it is armed until the test
// releases it. The login route reads randomness only once it has accepted the
// password and is about to establish the session, which is exactly the window a
// password reset has to survive.
type gatedRandom struct {
	source  io.Reader
	armed   atomic.Bool
	once    sync.Once
	reached chan struct{}
	release chan struct{}
}

func newGatedRandom() *gatedRandom {
	return &gatedRandom{source: rand.Reader, reached: make(chan struct{}), release: make(chan struct{})}
}

func (random *gatedRandom) Read(buffer []byte) (int, error) {
	if random.armed.Load() {
		random.once.Do(func() {
			close(random.reached)
			<-random.release
		})
	}
	return random.source.Read(buffer)
}

// TestPasswordResetDuringAnOverlappingLoginLeavesNoUsableSession proves that a
// completed password reset cannot be outrun by a login that already verified
// the replaced password: the session that login was about to establish must
// never read a protected route.
func TestPasswordResetDuringAnOverlappingLoginLeavesNoUsableSession(t *testing.T) {
	t.Parallel()

	const supersededPassword = "superseded-secret-1"
	random := newGatedRandom()
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost, func(options *identity.Options) {
		options.Random = random
	})
	f.createUser(t, "operator", auth.RoleOperator, supersededPassword)

	// The reset runs through its own service instance so that the login's gated
	// randomness cannot hold the reset up as well.
	administration, err := identity.NewService(identity.Options{
		Repository:         sqlite.NewIdentityRepository(f.database),
		Now:                f.clock.Now,
		Random:             rand.Reader,
		SessionLifetime:    12 * time.Hour,
		LoginFailureLimit:  5,
		LoginFailureWindow: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewService(administration): %v", err)
	}

	random.armed.Store(true)
	var login *httptest.ResponseRecorder
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		login = f.attemptLogin("operator", supersededPassword)
	}()

	<-random.reached
	if err := administration.ResetPassword(
		context.Background(), f.administrator, "operator", []byte(operatorPassword),
	); err != nil {
		t.Fatalf("ResetPassword during the login: %v", err)
	}
	close(random.release)
	<-finished

	if login.Code == http.StatusOK {
		for _, cookie := range login.Result().Cookies() {
			if cookie.Name != auth.SessionCookieName {
				continue
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
			request.AddCookie(cookie)
			if response := f.do(request); response.Code == http.StatusOK {
				t.Fatal("a session established from the replaced password read a protected route")
			}
		}
		t.Fatalf("the login on the replaced password returned %d, want a rejection", login.Code)
	}
	if response := f.attemptLogin("operator", operatorPassword); response.Code != http.StatusOK {
		t.Fatalf("the reset password does not log in: %d; body=%s", response.Code, response.Body.String())
	}
}
