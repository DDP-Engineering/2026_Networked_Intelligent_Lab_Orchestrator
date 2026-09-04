package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/simulation"
)

const lineBody = `{"action":"create","device_network":{"name":"Line 3","location":"Hall B","device_interface":"02:00:00:00:00:02","appliance_address":"192.168.0.1/24","scope":"host","target":"192.168.0.130","addressing":"original","access":"local","local_interface":"02:00:00:00:00:01","local_sources":["192.168.10.0/24"]}}`

// TestDeviceNetworkRoutesDriveTheLifecycle covers REQ-FUNC-9b17ce40 at the
// HTTP boundary: list, read, create, enable, check, instructions, disable and
// delete, every mutation through the acknowledged lifecycle.
func TestDeviceNetworkRoutesDriveTheLifecycle(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	token := f.issueToken(t, "admin", "device-networks", nil, nil)
	if r := f.get("/api/v1/network/device-networks", token); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), `"items":[]`) {
		t.Fatalf("empty list: %d %s", r.Code, r.Body.String())
	}
	if r := f.post("/api/v1/network/device-networks/preview", token, strings.Replace(lineBody, `"target":"192.168.0.130",`, "", 1)); r.Code != http.StatusBadRequest || !strings.Contains(r.Body.String(), "device_network_rejected") {
		t.Fatalf("host scope without a target: %d %s", r.Code, r.Body.String())
	}
	p := decodePreview(t, f.post("/api/v1/network/device-networks/preview", token, lineBody))
	if p.Action != "device_network.create" {
		t.Fatalf("preview action = %s", p.Action)
	}
	if r := f.post("/api/v1/network/operations/"+p.ID+"/acknowledge", token, `{"revision":"`+p.Revision+`"}`); r.Code != http.StatusAccepted || !strings.Contains(r.Body.String(), "confirmed") {
		t.Fatalf("create: %d %s", r.Code, r.Body.String())
	}
	var list struct {
		Items []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			State       string `json:"state"`
			Enabled     bool   `json:"enabled"`
			Applied     bool   `json:"applied"`
			ExposedText string `json:"exposed"`
		} `json:"items"`
	}
	if err := json.Unmarshal(f.get("/api/v1/network/device-networks", token).Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "Line 3" || list.Items[0].State != "disabled" || list.Items[0].Enabled {
		t.Fatalf("list after create = %+v", list.Items)
	}
	id := list.Items[0].ID
	if r := f.get("/api/v1/network/device-networks/"+id, token); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "192.168.0.130") {
		t.Fatalf("read: %d %s", r.Code, r.Body.String())
	}
	enable := decodePreview(t, f.post("/api/v1/network/device-networks/preview", token, `{"action":"enable","target":"`+id+`"}`))
	r := f.post("/api/v1/network/operations/"+enable.ID+"/acknowledge", token, `{"revision":"`+enable.Revision+`"}`)
	if r.Code != http.StatusAccepted || !strings.Contains(r.Body.String(), "awaiting_confirmation") {
		t.Fatalf("enable: %d %s", r.Code, r.Body.String())
	}
	f.post("/api/v1/network/operations/"+enable.ID+"/confirm", token, `{}`)
	if err := json.Unmarshal(f.get("/api/v1/network/device-networks", token).Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Items[0].State != "active" || !list.Items[0].Enabled || !list.Items[0].Applied {
		t.Fatalf("after enable = %+v", list.Items)
	}
	check := f.post("/api/v1/network/device-networks/"+id+"/checks", token, `{}`)
	if check.Code != http.StatusOK || !strings.Contains(check.Body.String(), `"name":"target"`) || !strings.Contains(check.Body.String(), `"name":"adapter"`) {
		t.Fatalf("check: %d %s", check.Code, check.Body.String())
	}
	if r := f.post("/api/v1/network/device-networks/"+id+"/checks", token, `{"tcp_port":70000}`); r.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range probe port: %d", r.Code)
	}
	instructions := f.get("/api/v1/network/device-networks/"+id+"/access-instructions", token)
	if instructions.Code != http.StatusOK || !strings.Contains(instructions.Body.String(), "client computer") || !strings.Contains(instructions.Body.String(), "192.168.0.130") {
		t.Fatalf("instructions: %d %s", instructions.Code, instructions.Body.String())
	}
	disable := decodePreview(t, f.post("/api/v1/network/device-networks/preview", token, `{"action":"disable","target":"`+id+`"}`))
	f.post("/api/v1/network/operations/"+disable.ID+"/acknowledge", token, `{"revision":"`+disable.Revision+`"}`)
	f.post("/api/v1/network/operations/"+disable.ID+"/confirm", token, `{}`)
	remove := decodePreview(t, f.post("/api/v1/network/device-networks/preview", token, `{"action":"delete","target":"`+id+`"}`))
	f.post("/api/v1/network/operations/"+remove.ID+"/acknowledge", token, `{"revision":"`+remove.Revision+`"}`)
	f.post("/api/v1/network/operations/"+remove.ID+"/confirm", token, `{}`)
	if r := f.get("/api/v1/network/device-networks/"+id, token); r.Code != http.StatusNotFound {
		t.Fatalf("read after delete: %d", r.Code)
	}
}

// TestDeviceNetworkRoutesAreAuthorized keeps the routes under the fixed
// permissions: reads for every role, changes for network.manage.
func TestDeviceNetworkRoutesAreAuthorized(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	viewer := f.issueToken(t, "viewer", "viewer", nil, nil)
	if r := f.get("/api/v1/network/device-networks", viewer); r.Code != http.StatusOK {
		t.Fatalf("viewer list = %d", r.Code)
	}
	if r := f.post("/api/v1/network/device-networks/preview", viewer, lineBody); r.Code != http.StatusForbidden {
		t.Fatalf("viewer preview = %d", r.Code)
	}
	if r := f.post("/api/v1/network/device-networks/none/checks", viewer, `{}`); r.Code != http.StatusForbidden {
		t.Fatalf("viewer check = %d", r.Code)
	}
}
