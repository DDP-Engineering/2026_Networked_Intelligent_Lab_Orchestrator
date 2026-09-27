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

func TestSerialRoutesScopeCSRFAndStrictBody(t *testing.T) {
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
	read := f.issueToken(t, "admin", "serial-read", []auth.Permission{auth.PermissionBenchRead}, nil)
	execute := f.issueToken(t, "admin", "serial-execute", []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}, nil)
	for _, suffix := range []string{"connect", "send", "disconnect", "signals", "logging"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/serial-devices/serial-1/"+suffix, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer "+read)
		if got := f.do(request); got.Code != 403 {
			t.Fatalf("read-only %s: %d", suffix, got.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/serial-devices/serial-1/send", strings.NewReader(`{"shell":"forbidden"}`))
	request.Header.Set("Authorization", "Bearer "+execute)
	if got := f.do(request); got.Code != 400 {
		t.Fatalf("strict serial body: %d", got.Code)
	}
	session := f.login(t, "admin", administratorPassword)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/serial-devices/serial-1/connect", strings.NewReader(`{}`))
	request.AddCookie(session.sessionCookie)
	if got := f.do(request); got.Code != 403 {
		t.Fatalf("serial CSRF missing: %d", got.Code)
	}
	if got := f.get("/api/v1/serial-devices/serial-1/read?after=bad", read); got.Code != 400 {
		t.Fatalf("bad cursor accepted: %d", got.Code)
	}
	if got := f.get("/api/v1/serial-logs/unknown/content?format=rx", ""); got.Code != 401 {
		t.Fatalf("unauthenticated raw export: %d", got.Code)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/v1/serial-devices/serial-1/retention", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+execute)
	if got := f.do(request); got.Code != 403 {
		t.Fatalf("operator retention: %d", got.Code)
	}
}
