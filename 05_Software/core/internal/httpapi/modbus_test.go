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

func TestModbusRoutesScopeAndStrictBody(t *testing.T) {
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
	read := f.issueToken(t, "admin", "modbus-read", []auth.Permission{auth.PermissionBenchRead}, nil)
	execute := f.issueToken(t, "admin", "modbus-execute", []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}, nil)
	for _, action := range []string{"connect", "read", "write", "preview", "poll", "display", "disconnect"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/modbus/endpoints/gateway/"+action, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+read)
		if got := f.do(req); got.Code != 403 {
			t.Fatalf("read-only %s: %d", action, got.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/modbus/endpoints/gateway/write", strings.NewReader(`{"shell":"unsafe"}`))
	req.Header.Set("Authorization", "Bearer "+execute)
	if got := f.do(req); got.Code != 400 {
		t.Fatalf("strict write body: %d", got.Code)
	}
	if got := f.get("/api/v1/modbus/endpoints", read); got.Code != 200 {
		t.Fatalf("list endpoints: %d", got.Code)
	}
}
