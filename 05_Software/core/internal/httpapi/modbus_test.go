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
	"nilo.local/nilo/internal/modbus"
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
	manage := f.issueToken(t, "admin", "modbus-manage", []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute, auth.PermissionPluginsManage}, nil)
	for _, action := range []string{"connect", "read", "write", "preview", "poll", "poll-group", "display", "disconnect"} {
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
	if got := f.get("/api/v1/modbus/settings", read); got.Code != 200 {
		t.Fatalf("read settings: %d", got.Code)
	}
	settingsBody := `{"expected_revision":1,"settings":{"max_sessions":4,"session_lease_seconds":3600,"idle_seconds":300,"retention_count":100,"retention_bytes":2147483648,"part_bytes":104857600}}`
	for _, candidate := range []struct {
		path, method, body, token string
		want                      int
	}{
		{"/api/v1/modbus/settings", http.MethodPut, settingsBody, read, 403},
		{"/api/v1/modbus/settings", http.MethodPut, settingsBody + ` `, manage, 200},
		{"/api/v1/modbus/settings", http.MethodPut, `{"expected_revision":2,"settings":{"max_sessions":4},"extra":true}`, manage, 400},
		{"/api/v1/modbus/endpoints/peer/test", http.MethodPost, `{}`, execute, 403},
		{"/api/v1/modbus/endpoints/peer/test", http.MethodPost, `{"unknown":1}`, manage, 400},
	} {
		req := httptest.NewRequest(candidate.method, candidate.path, strings.NewReader(candidate.body))
		req.Header.Set("Authorization", "Bearer "+candidate.token)
		if got := f.do(req); got.Code != candidate.want {
			t.Fatalf("%s %s: %d, want %d: %s", candidate.method, candidate.path, got.Code, candidate.want, got.Body.String())
		}
	}
	if got := f.get("/api/v1/modbus/maps", read); got.Code != 200 {
		t.Fatalf("list maps: %d", got.Code)
	}
	mapBody := `{"expected_revision":0,"map":{"schema_version":1,"id":"motors","name":"Motors","points":[]}}`
	mapReq := httptest.NewRequest(http.MethodPut, "/api/v1/modbus/maps/motors", strings.NewReader(mapBody))
	mapReq.Header.Set("Authorization", "Bearer "+read)
	if got := f.do(mapReq); got.Code != 403 {
		t.Fatalf("read-only map save: %d", got.Code)
	}
	mapReq = httptest.NewRequest(http.MethodPut, "/api/v1/modbus/maps/motors", strings.NewReader(mapBody))
	mapReq.Header.Set("Authorization", "Bearer "+execute)
	if got := f.do(mapReq); got.Code != 200 {
		t.Fatalf("save map: %d %s", got.Code, got.Body.String())
	}
	for _, candidate := range []struct {
		action, body, token string
		want                int
	}{
		{"decode", `{"point_id":"missing","words":[1]}`, read, 404},
		{"decode", `{"point_id":"missing","words":[1],"extra":true}`, read, 400},
		{"encode", `{"point_id":"missing","value":1}`, read, 403},
		{"encode", `{"point_id":"missing","value":1}`, execute, 404},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/modbus/maps/motors/revisions/1/"+candidate.action, strings.NewReader(candidate.body))
		req.Header.Set("Authorization", "Bearer "+candidate.token)
		if got := f.do(req); got.Code != candidate.want {
			t.Fatalf("point codec %s: %d want %d: %s", candidate.action, got.Code, candidate.want, got.Body.String())
		}
	}
	workspacePreview := httptest.NewRequest(http.MethodPost, "/api/v1/modbus/workspaces/import-preview", strings.NewReader(`{"mode":"create","document":"{}"}`))
	workspacePreview.Header.Set("Authorization", "Bearer "+execute)
	if got := f.do(workspacePreview); got.Code != 200 {
		t.Fatalf("workspace preview route: %d", got.Code)
	}
	repository := sqlite.NewModbusRepository(f.database)
	endpoint, err := repository.PutEndpoint(t.Context(), 0, modbus.Endpoint{SchemaVersion: 1, ID: "peer", Name: "Peer", Transport: "tcp", ResourceID: "bus", Enabled: true, Presence: "configured", Readiness: "offline", Network: &modbus.Network{Host: "peer.local", Port: 502, UnitMax: 255}})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := repository.PutProfile(t.Context(), 0, modbus.Profile{SchemaVersion: 1, ID: "profile", OwnerID: "admin", Name: "Profile", EndpointID: endpoint.ID, Settings: []byte(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0}`)})
	if err != nil {
		t.Fatal(err)
	}
	workspaceBody := `{"expected_revision":0,"workspace":{"schema_version":1,"id":"desk","name":"Desk","endpoint_id":"peer","profile_ref":{"id":"profile","revision":1},"map_refs":[{"id":"motors","revision":1}],"poll_groups":[],"layout":{"columns":["name"],"address_base":"zero","series":[],"display_paused":false}}}`
	workspaceReq := httptest.NewRequest(http.MethodPut, "/api/v1/modbus/workspaces/desk", strings.NewReader(workspaceBody))
	workspaceReq.Header.Set("Authorization", "Bearer "+execute)
	if got := f.do(workspaceReq); got.Code != 200 {
		t.Fatalf("save workspace: %d %s", got.Code, got.Body.String())
	}
	if got := f.get("/api/v1/modbus/workspaces/desk/export", read); got.Code != 200 {
		t.Fatalf("export workspace: %d", got.Code)
	}
	if profile.Revision != 1 {
		t.Fatal(profile)
	}
}
