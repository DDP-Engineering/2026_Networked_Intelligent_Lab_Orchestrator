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

func (f apiFixture) put(path, bearer, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	return f.do(request)
}

type activityPage struct {
	Items []struct {
		ID          string          `json:"id"`
		Sequence    int64           `json:"sequence"`
		Category    string          `json:"category"`
		Component   string          `json:"component"`
		Action      string          `json:"action"`
		Outcome     string          `json:"outcome"`
		ActorKind   string          `json:"actor_kind"`
		OperationID string          `json:"operation_id"`
		Details     json.RawMessage `json:"details"`
	} `json:"items"`
	NextCursor     string `json:"next_cursor"`
	RetainedWindow map[string]struct {
		Count     int64 `json:"count"`
		Truncated bool  `json:"truncated"`
	} `json:"retained_window"`
	LoggingHealth struct {
		AuditWritable bool `json:"audit_writable"`
	} `json:"logging_health"`
}

func (f apiFixture) activity(t *testing.T, token, query string) activityPage {
	t.Helper()
	response := f.get("/api/v1/activity"+query, token)
	if response.Code != http.StatusOK {
		t.Fatalf("activity %s = %d %s", query, response.Code, response.Body.String())
	}
	var page activityPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

// TestActivityExplainsAChangeFromIntentToOutcome is the P7 exit criterion at
// the HTTP boundary (REQ-FUNC-d4f81a26, REQ-FUNC-6ba25f39,
// REQ-FUNC-35da79f1): a network change is read back stage by stage through
// its operation identity, exported in both formats with the export itself
// recorded, a timed debug session raises detail for the function and then
// stops, and the retention policy is read and changed under audit.
func TestActivityExplainsAChangeFromIntentToOutcome(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	token := f.issueToken(t, "admin", "activity", nil, nil)

	// A debug session for interfaces, then one interface change.
	started := f.post("/api/v1/diagnostics/sessions", token, `{"scope":"network.interfaces","target":"","minutes":5}`)
	if started.Code != http.StatusCreated || !strings.Contains(started.Body.String(), `"state":"active"`) {
		t.Fatalf("start session = %d %s", started.Code, started.Body.String())
	}
	var session struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if r := f.post("/api/v1/diagnostics/sessions", token, `{"scope":"network.interfaces","minutes":7}`); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "diagnostic_session_rejected") {
		t.Fatalf("seven minutes = %d %s", r.Code, r.Body.String())
	}
	preview := decodePreview(t, f.post("/api/v1/network/interfaces/sim-enp1s0/preview", token, `{"action":"disable"}`))
	if r := f.post("/api/v1/network/operations/"+preview.ID+"/acknowledge", token, `{"revision":"`+preview.Revision+`"}`); r.Code != http.StatusAccepted {
		t.Fatalf("acknowledge = %d %s", r.Code, r.Body.String())
	}
	if r := f.post("/api/v1/network/operations/"+preview.ID+"/confirm", token, `{}`); r.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", r.Code, r.Body.String())
	}

	// The change reads back from intent to outcome through its operation.
	page := f.activity(t, token, "?operation="+preview.ID+"&category=audit")
	outcomes := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		outcomes = append(outcomes, item.Outcome)
		if item.OperationID != preview.ID || item.Component != "network.interfaces" || item.Action != "interface.disable" {
			t.Fatalf("item = %+v", item)
		}
	}
	if joined := strings.Join(outcomes, ","); joined != "requested,applied,confirmed" {
		t.Fatalf("outcomes = %s", joined)
	}
	if page.RetainedWindow["audit"].Count == 0 || !page.LoggingHealth.AuditWritable {
		t.Fatalf("page = %+v", page)
	}
	// Debug detail was recorded for the function while the session ran.
	debug := f.activity(t, token, "?category=debug&operation="+preview.ID)
	if len(debug.Items) == 0 || debug.Items[0].Component != "network.interfaces" || debug.Items[0].ActorKind != "system" {
		t.Fatalf("debug = %+v", debug.Items)
	}
	// Paging tiles.
	first := f.activity(t, token, "?limit=2")
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second := f.activity(t, token, "?limit=2&cursor="+first.NextCursor)
	if len(second.Items) == 0 || second.Items[0].Sequence <= first.Items[1].Sequence {
		t.Fatalf("second page = %+v", second)
	}
	if r := f.get("/api/v1/activity?category=secrets", token); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "activity_filter_rejected") {
		t.Fatalf("bad filter = %d %s", r.Code, r.Body.String())
	}

	// Exports in both formats, each recorded.
	jsonl := f.get("/api/v1/activity/export?format=jsonl&operation="+preview.ID, token)
	if jsonl.Code != http.StatusOK || jsonl.Header().Get("Content-Type") != "application/x-ndjson" || !strings.Contains(jsonl.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("jsonl export = %d %v", jsonl.Code, jsonl.Header())
	}
	lines := strings.Split(strings.TrimSpace(jsonl.Body.String()), "\n")
	if !strings.Contains(lines[0], `"schema_version":"nilo.activity/1"`) || !strings.Contains(lines[0], `"appliance_id"`) || !strings.Contains(lines[len(lines)-1], `"trailer":true`) {
		t.Fatalf("jsonl body:\n%s", jsonl.Body.String())
	}
	csv := f.get("/api/v1/activity/export?format=csv", token)
	if csv.Code != http.StatusOK || !strings.HasPrefix(csv.Header().Get("Content-Type"), "text/csv") || !strings.HasPrefix(csv.Body.String(), "# schema_version=nilo.activity/1") {
		t.Fatalf("csv export = %d %s", csv.Code, csv.Body.String()[:80])
	}
	if r := f.get("/api/v1/activity/export?format=xml", token); r.Code != http.StatusBadRequest {
		t.Fatalf("xml export = %d", r.Code)
	}
	exports := f.activity(t, token, "?action=activity.exported")
	if len(exports.Items) != 2 || exports.Items[0].ActorKind != "token" {
		t.Fatalf("export events = %+v", exports.Items)
	}

	// The session is listed, stopped once, and refused a second time.
	list := f.get("/api/v1/diagnostics/sessions", token)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), session.ID) || !strings.Contains(list.Body.String(), `"scopes"`) || !strings.Contains(list.Body.String(), `"durations":[5,15,60]`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	if r := f.get("/api/v1/diagnostics/sessions/"+session.ID, token); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"effective":"debug"`) {
		t.Fatalf("read = %d %s", r.Code, r.Body.String())
	}
	if r := f.post("/api/v1/diagnostics/sessions/"+session.ID+"/stop", token, `{}`); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"state":"stopped"`) {
		t.Fatalf("stop = %d %s", r.Code, r.Body.String())
	}
	if r := f.post("/api/v1/diagnostics/sessions/"+session.ID+"/stop", token, `{}`); r.Code != http.StatusConflict {
		t.Fatalf("stop twice = %d %s", r.Code, r.Body.String())
	}
	if r := f.get("/api/v1/diagnostics/sessions/missing", token); r.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", r.Code)
	}
	lifecycle := f.activity(t, token, "?action=diagnostics.session.started")
	if len(lifecycle.Items) != 1 {
		t.Fatalf("session start events = %+v", lifecycle.Items)
	}

	// The policy reads as the default and changes under audit.
	settings := f.get("/api/v1/diagnostics/settings", token)
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `"audit":90`) {
		t.Fatalf("settings = %d %s", settings.Code, settings.Body.String())
	}
	changed := f.put("/api/v1/diagnostics/settings", token, `{"retention_days":{"audit":30,"event":7,"debug":1},"quota_bytes":{"audit":16777216,"event":8388608,"debug":4194304}}`)
	if changed.Code != http.StatusOK || !strings.Contains(changed.Body.String(), `"audit":30`) {
		t.Fatalf("change settings = %d %s", changed.Code, changed.Body.String())
	}
	if r := f.put("/api/v1/diagnostics/settings", token, `{"retention_days":{"audit":0,"event":7,"debug":1},"quota_bytes":{"audit":16777216,"event":8388608,"debug":4194304}}`); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "activity_policy_rejected") {
		t.Fatalf("bad settings = %d %s", r.Code, r.Body.String())
	}
	if again := f.get("/api/v1/diagnostics/settings", token); !strings.Contains(again.Body.String(), `"audit":30`) {
		t.Fatalf("settings did not persist: %s", again.Body.String())
	}
	policy := f.activity(t, token, "?action=diagnostics.policy.changed")
	if len(policy.Items) != 1 || !strings.Contains(string(policy.Items[0].Details), `"before"`) {
		t.Fatalf("policy events = %+v", policy.Items)
	}
}

// TestActivityAndDiagnosticsRoutesAreAuthorized covers REQ-SEC-d19a7b30:
// every route refuses a viewer, an operator, a scoped token lacking the
// permission and a revoked token, and a refusal is itself recorded.
func TestActivityAndDiagnosticsRoutesAreAuthorized(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	admin := f.issueToken(t, "admin", "admin", nil, nil)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	viewer := f.issueToken(t, "viewer", "viewer", nil, nil)
	operator := f.issueToken(t, "operator", "operator", nil, nil)
	scoped := f.issueToken(t, "admin", "scoped", []auth.Permission{auth.PermissionStatusRead, auth.PermissionNetworkRead}, nil)
	revokedToken, err := f.service.CreateToken(context.Background(), f.administrator, "admin", "revoked", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.RevokeToken(context.Background(), f.administrator, revokedToken.Token.ID); err != nil {
		t.Fatal(err)
	}
	routes := []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/v1/activity", ""},
		{http.MethodGet, "/api/v1/activity/export?format=jsonl", ""},
		{http.MethodGet, "/api/v1/diagnostics/sessions", ""},
		{http.MethodPost, "/api/v1/diagnostics/sessions", `{"scope":"system","minutes":5}`},
		{http.MethodGet, "/api/v1/diagnostics/sessions/x", ""},
		{http.MethodPost, "/api/v1/diagnostics/sessions/x/stop", `{}`},
		{http.MethodGet, "/api/v1/diagnostics/settings", ""},
		{http.MethodPut, "/api/v1/diagnostics/settings", `{}`},
	}
	call := func(method, path, body, token string) int {
		var request *http.Request
		if body == "" {
			request = httptest.NewRequest(method, path, nil)
		} else {
			request = httptest.NewRequest(method, path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
		}
		request.Header.Set("Authorization", "Bearer "+token)
		return f.do(request).Code
	}
	for _, route := range routes {
		for name, token := range map[string]string{"viewer": viewer, "operator": operator, "scoped": scoped} {
			if code := call(route.method, route.path, route.body, token); code != http.StatusForbidden {
				t.Fatalf("%s %s %s = %d, want 403", name, route.method, route.path, code)
			}
		}
		if code := call(route.method, route.path, route.body, revokedToken.Secret); code != http.StatusUnauthorized {
			t.Fatalf("revoked %s %s = %d, want 401", route.method, route.path, code)
		}
		if code := call(route.method, route.path, route.body, ""); code != http.StatusUnauthorized {
			t.Fatalf("anonymous %s %s = %d, want 401", route.method, route.path, code)
		}
	}
	denials := f.activity(t, admin, "?action=identity.access.denied&category=audit")
	if len(denials.Items) < len(routes)*3 || denials.Items[0].Outcome != "denied" {
		t.Fatalf("denials = %d items, first %+v", len(denials.Items), denials.Items)
	}
	// A refused export of the viewer's own scope is not a leak: the viewer
	// cannot read the denial either.
	if r := f.get("/api/v1/activity?action=identity.access.denied", viewer); r.Code != http.StatusForbidden {
		t.Fatalf("viewer reading denials = %d", r.Code)
	}
}
