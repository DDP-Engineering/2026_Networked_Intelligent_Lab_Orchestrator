package httpapi

import (
	"encoding/json"
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

func TestOperatingSettingsRoutesRequireAuthorityAndRevision(t *testing.T) {
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
	admin := f.issueToken(t, "admin", "platform settings", []auth.Permission{auth.PermissionPlatformSettingsManage}, nil)
	narrow := f.issueToken(t, "admin", "bench only", []auth.Permission{auth.PermissionBenchRead}, nil)
	f.createUser(t, "platform-operator", auth.RoleOperator, operatorPassword)
	f.createUser(t, "platform-viewer", auth.RoleViewer, viewerPassword)
	operator := f.issueToken(t, "platform-operator", "operator settings", nil, nil)
	viewer := f.issueToken(t, "platform-viewer", "viewer settings", nil, nil)
	if response := f.get("/api/v1/platform/operating-settings", admin); response.Code != 200 {
		t.Fatalf("administrator read = %d %s", response.Code, response.Body.String())
	}
	for _, token := range []string{narrow, operator, viewer} {
		if response := f.get("/api/v1/platform/operating-settings", token); response.Code != 403 {
			t.Fatalf("unauthorized read = %d", response.Code)
		}
	}
	body := `{"revision":1,"settings":{"executing_jobs":3,"outstanding_jobs":32,"serial_connections":4,"camera_streams":2,"default_job_timeout_seconds":60,"max_job_timeout_seconds":300,"serial_lifetime_minutes":60,"serial_idle_minutes":1,"camera_lifetime_minutes":55}}`
	put := func(token, value string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "/api/v1/platform/operating-settings", strings.NewReader(value))
		request.Header.Set("Authorization", "Bearer "+token)
		return f.do(request)
	}
	for _, token := range []string{narrow, operator, viewer} {
		if response := put(token, body); response.Code != 403 {
			t.Fatalf("unauthorized write = %d", response.Code)
		}
	}
	session := f.login(t, "admin", administratorPassword)
	withoutCSRF := httptest.NewRequest(http.MethodPut, "/api/v1/platform/operating-settings", strings.NewReader(body))
	withoutCSRF.AddCookie(session.sessionCookie)
	if response := f.do(withoutCSRF); response.Code != 403 {
		t.Fatalf("browser write without CSRF = %d", response.Code)
	}
	if response := put(admin, body); response.Code != 200 {
		t.Fatalf("administrator write = %d %s", response.Code, response.Body.String())
	}
	if response := put(admin, body); response.Code != 409 {
		t.Fatalf("stale revision = %d %s", response.Code, response.Body.String())
	}
	if response := put(admin, `{"revision":2,"settings":{}}`); response.Code != 400 {
		t.Fatalf("partial replacement = %d %s", response.Code, response.Body.String())
	}
	response := f.get("/api/v1/platform/operating-settings", admin)
	var state bench.OperatingSettingsState
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || state.Revision != 2 || state.Settings.ExecutingJobs != 3 {
		t.Fatalf("stored response = %+v, %v", state, err)
	}
	var changes int
	if err := f.database.SQL().QueryRow(`SELECT count(*) FROM audit_events WHERE action='operating_settings.changed'`).Scan(&changes); err != nil || changes != 1 {
		t.Fatalf("audit changes = %d, %v", changes, err)
	}
}
