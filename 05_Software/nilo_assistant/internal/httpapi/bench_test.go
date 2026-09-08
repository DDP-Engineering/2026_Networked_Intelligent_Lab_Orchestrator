package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

func TestPluginManagementRouteRequiresAuthentication(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	response := f.get("/api/v1/plugins", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("plugin catalog without identity = %d; want authenticated plugin route (401)", response.Code)
	}
	token := f.issueToken(t, "admin", "plugins", nil, nil)
	response = f.get("/api/v1/plugins", token)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured plugin service = %d; want explicit service unavailable (503), body=%s", response.Code, response.Body.String())
	}
}

func TestBenchRoutesEnforceScopesCSRFAndStrictInputs(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	writer, err := audit.NewWriter(audit.WriterOptions{Events: sqlite.NewAuditRepository(f.database), ApplianceID: "test", BootID: "test", Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := bench.New(bench.Options{Database: f.database, DataDir: t.TempDir(), Audit: writer, Simulation: true})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	f.options.Bench = manager
	f.handler, err = NewHandler(f.options)
	if err != nil {
		t.Fatal(err)
	}
	read := f.issueToken(t, "admin", "bench read", []auth.Permission{auth.PermissionBenchRead}, nil)
	if response := f.get("/api/v1/plugins", read); response.Code != 200 {
		t.Fatalf("read catalog: %d %s", response.Code, response.Body.String())
	}
	if response := f.get("/api/v1/bench/jobs", read); response.Code != 200 {
		t.Fatalf("read jobs: %d", response.Code)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/sigrok/install", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+read)
	if response := f.do(request); response.Code != 403 {
		t.Fatalf("read token installed: %d", response.Code)
	}
	session := f.login(t, "admin", administratorPassword)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/plugins/sigrok/install", strings.NewReader(`{}`))
	request.AddCookie(session.sessionCookie)
	if response := f.do(request); response.Code != 403 {
		t.Fatalf("missing CSRF: %d", response.Code)
	}
	for _, body := range []string{`{"shell":"arbitrary"}`, `{} {}`} {
		request = httptest.NewRequest(http.MethodPost, "/api/v1/plugins/sigrok/install", strings.NewReader(body))
		request.AddCookie(session.sessionCookie)
		request.Header.Set(auth.CSRFHeaderName, session.csrfToken)
		if response := f.do(request); response.Code != 400 {
			t.Fatalf("invalid lifecycle input: %d", response.Code)
		}
	}
	execute := f.issueToken(t, "admin", "bench execute", []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}, nil)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/bench/jobs", strings.NewReader(`{"plugin_id":"sigrok","operation":"capture","device_id":"atk-dl16-1.3","idempotency_key":"invalid-pwm","parameters":{"channels":["PWM0"],"samples":1024,"samplerate":1000000}}`))
	request.Header.Set("Authorization", "Bearer "+execute)
	if response := f.do(request); response.Code != 400 {
		t.Fatalf("PWM input: %d %s", response.Code, response.Body.String())
	}
}
