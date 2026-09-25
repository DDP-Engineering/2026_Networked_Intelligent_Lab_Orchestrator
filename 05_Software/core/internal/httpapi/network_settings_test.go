package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/simulation"
)

func (f apiFixture) postFrom(path, bearer, body, remote string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+bearer)
	request.RemoteAddr = remote
	return f.do(request)
}

// TestInterfaceSettingsRouteAppliesAndReportsTheBinding covers
// REQ-FUNC-5e2b91d7 at the HTTP boundary: a settings preview is an ordinary
// acknowledged connectivity change, and the status afterwards reports the
// adapter identity, the saved intent and whether it is applied.
func TestInterfaceSettingsRouteAppliesAndReportsTheBinding(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	token := f.issueToken(t, "admin", "settings", nil, nil)
	if r := f.post("/api/v1/network/interfaces/sim-enp1s0/settings/preview", token, `{"method":"static","address":"192.168.0.0/24"}`); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "interface_settings_rejected") {
		t.Fatalf("invalid address: %d %s", r.Code, r.Body.String())
	}
	body := `{"method":"static","address":"192.168.0.1/24","dns":["192.168.0.53"],"never_default":true}`
	p := decodePreview(t, f.post("/api/v1/network/interfaces/sim-enp1s0/settings/preview", token, body))
	if p.Action != "interface.settings" {
		t.Fatalf("preview action = %s", p.Action)
	}
	r := f.post("/api/v1/network/operations/"+p.ID+"/acknowledge", token, `{"revision":"`+p.Revision+`"}`)
	if r.Code != http.StatusAccepted || !strings.Contains(r.Body.String(), "awaiting_confirmation") {
		t.Fatalf("acknowledge: %d %s", r.Code, r.Body.String())
	}
	if r := f.post("/api/v1/network/operations/"+p.ID+"/confirm", token, `{}`); r.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", r.Code, r.Body.String())
	}
	var status struct {
		Interfaces []struct {
			ID        string   `json:"id"`
			AdapterID string   `json:"adapter_id"`
			IPv4      []string `json:"ipv4"`
			Settings  *struct {
				Saved               bool   `json:"saved"`
				Applied             bool   `json:"applied"`
				Method              string `json:"method"`
				ManagementProtected bool   `json:"management_protected"`
				NeverDefault        bool   `json:"never_default"`
			} `json:"settings"`
		} `json:"interfaces"`
		Unattached []map[string]any `json:"unattached_settings"`
	}
	if err := json.Unmarshal(f.get("/api/v1/network/status", token).Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range status.Interfaces {
		if entry.ID != "sim-enp1s0" {
			continue
		}
		found = true
		if entry.AdapterID == "" || len(entry.IPv4) != 1 || entry.IPv4[0] != "192.168.0.1/24" {
			t.Fatalf("status after settings = %+v", entry)
		}
		if entry.Settings == nil || !entry.Settings.Saved || !entry.Settings.Applied || entry.Settings.Method != "static" || !entry.Settings.NeverDefault {
			t.Fatalf("settings state = %+v", entry.Settings)
		}
	}
	if !found || status.Unattached == nil {
		t.Fatalf("status = %+v", status)
	}
}

// TestWiFiRoutesNeverReturnThePassword covers REQ-FUNC-7c40aa18 at the HTTP
// boundary: the scan lists networks, the join stages the password and no
// response, operation or status ever carries it.
func TestWiFiRoutesNeverReturnThePassword(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	token := f.issueToken(t, "admin", "wifi", nil, nil)
	scan := f.get("/api/v1/network/interfaces/sim-wlan0/wifi", token)
	if scan.Code != http.StatusOK || !strings.Contains(scan.Body.String(), "lab-hotspot") || !strings.Contains(scan.Body.String(), `"supported":false`) {
		t.Fatalf("scan: %d %s", scan.Code, scan.Body.String())
	}
	if r := f.get("/api/v1/network/interfaces/sim-enp1s0/wifi", token); r.Code != http.StatusConflict {
		t.Fatalf("scan on a wired adapter: %d", r.Code)
	}
	const password = "SENTINELwifiPassword9931"
	body := `{"ssid":"lab-hotspot","security":"wpa2-personal","password":"` + password + `","connect_at_startup":true}`
	preview := f.post("/api/v1/network/interfaces/sim-wlan0/wifi/preview", token, body)
	p := decodePreview(t, preview)
	if strings.Contains(preview.Body.String(), password) {
		t.Fatal("the preview echoes the password")
	}
	r := f.post("/api/v1/network/operations/"+p.ID+"/acknowledge", token, `{"revision":"`+p.Revision+`"}`)
	if r.Code != http.StatusAccepted || strings.Contains(r.Body.String(), password) {
		t.Fatalf("acknowledge: %d %s", r.Code, r.Body.String())
	}
	f.post("/api/v1/network/operations/"+p.ID+"/confirm", token, `{}`)
	for _, path := range []string{"/api/v1/network/status", "/api/v1/network/operations", "/api/v1/network/operations/" + p.ID} {
		if r := f.get(path, token); strings.Contains(r.Body.String(), password) {
			t.Fatalf("%s carries the password", path)
		}
	}
	status := f.get("/api/v1/network/status", token).Body.String()
	if !strings.Contains(status, `"ssid":"lab-hotspot"`) {
		t.Fatalf("status after the join does not name the network: %s", status)
	}
	if r := f.post("/api/v1/network/interfaces/sim-wlan0/wifi/preview", token, `{"ssid":"corp-enterprise","security":"wpa2-personal","password":"x"}`); r.Code != http.StatusConflict {
		t.Fatalf("joining an unsupported network: %d %s", r.Code, r.Body.String())
	}
}

// TestManagementPathRefusalIsReportedWithItsCode covers REQ-REL-4c07b1ea at
// the HTTP boundary: the refusal carries a stable code and names the path.
func TestManagementPathRefusalIsReportedWithItsCode(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	token := f.issueToken(t, "admin", "management", nil, nil)
	// A client on the wired link is refused without any protection configured.
	r := f.postFrom("/api/v1/network/interfaces/sim-enp1s0/preview", token, `{"action":"disable"}`, "192.168.10.13:41000")
	if r.Code != http.StatusConflict || !strings.Contains(r.Body.String(), "management_path_required") || !strings.Contains(r.Body.String(), "enp1s0") {
		t.Fatalf("on-link disable: %d %s", r.Code, r.Body.String())
	}
	// The loopback development client is not on any adapter, so it is allowed.
	if r := f.postFrom("/api/v1/network/interfaces/sim-enp1s0/preview", token, `{"action":"disable"}`, "127.0.0.1:41000"); r.Code != http.StatusOK {
		t.Fatalf("loopback disable: %d %s", r.Code, r.Body.String())
	}
}

// TestSettingsRoutesAreAdministratorOnly keeps the new routes under the same
// permission as every other interface change.
func TestSettingsRoutesAreAdministratorOnly(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	viewer := f.issueToken(t, "viewer", "viewer", nil, nil)
	for _, route := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/network/interfaces/sim-enp1s0/settings/preview"},
		{http.MethodPost, "/api/v1/network/interfaces/sim-wlan0/wifi/preview"},
		{http.MethodGet, "/api/v1/network/interfaces/sim-wlan0/wifi"},
	} {
		var r *httptest.ResponseRecorder
		if route.method == http.MethodGet {
			r = f.get(route.path, viewer)
		} else {
			r = f.post(route.path, viewer, `{"method":"automatic","dns_automatic":true,"ssid":"x","security":"open"}`)
		}
		if r.Code != http.StatusForbidden {
			t.Fatalf("%s %s as viewer = %d", route.method, route.path, r.Code)
		}
	}
}
