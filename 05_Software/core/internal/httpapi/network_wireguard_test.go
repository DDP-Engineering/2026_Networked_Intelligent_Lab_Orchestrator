package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
)

func TestWireGuardReadAndAcknowledgedImportLifecycle(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	token := f.issueToken(t, "admin", "vpn", nil, nil)
	if r := f.get("/api/v1/network/wireguard", token); r.Code != 200 {
		t.Fatalf("readiness: %d %s", r.Code, r.Body.String())
	}
	const key = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	config := "[Interface]\nPrivateKey = " + key + "\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = ERERERERERERERERERERERERERERERERERERERERERE=\nAllowedIPs = 10.0.0.0/24\n"
	body, _ := json.Marshal(map[string]any{"action": "import", "target": "office", "profile_text": config})
	p := decodePreview(t, f.post("/api/v1/network/wireguard/preview", token, string(body)))
	r := f.post("/api/v1/network/operations/"+p.ID+"/acknowledge", token, `{"revision":"`+p.Revision+`"}`)
	if r.Code != http.StatusAccepted {
		t.Fatalf("import: %d %s", r.Code, r.Body.String())
	}
	r = f.get("/api/v1/network/wireguard", token)
	if strings.Contains(r.Body.String(), key) || strings.Contains(r.Body.String(), "PrivateKey") {
		t.Fatal("WireGuard status leaked private input")
	}
	var status network.WireGuardStatus
	if err := json.Unmarshal(r.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Profiles) != 1 || status.Profiles[0].Active || status.Profiles[0].AutoConnect {
		t.Fatalf("import result: %s", r.Body.String())
	}
}

// TestWireGuardRenameKeepsTheIdentityAndRefusesATakenName covers
// REQ-FUNC-72e0dc95 at the HTTP boundary: a rename is an ordinary acknowledged
// change, the profile keeps its identifier, and a name another profile holds
// is refused with a stable code.
func TestWireGuardRenameKeepsTheIdentityAndRefusesATakenName(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	token := f.issueToken(t, "admin", "rename", nil, nil)
	importProfile := func(name string) {
		t.Helper()
		config := "[Interface]\nPrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = ERERERERERERERERERERERERERERERERERERERERERE=\nAllowedIPs = 10.0.0.0/24\n"
		body, _ := json.Marshal(map[string]any{"action": "import", "target": name, "profile_text": config})
		p := decodePreview(t, f.post("/api/v1/network/wireguard/preview", token, string(body)))
		if r := f.post("/api/v1/network/operations/"+p.ID+"/acknowledge", token, `{"revision":"`+p.Revision+`"}`); r.Code != http.StatusAccepted {
			t.Fatalf("import %s: %d %s", name, r.Code, r.Body.String())
		}
	}
	importProfile("office")
	importProfile("site-b")
	var status network.WireGuardStatus
	if err := json.Unmarshal(f.get("/api/v1/network/wireguard", token).Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	var office string
	for _, profile := range status.Profiles {
		if profile.Name == "office" {
			office = profile.ID
		}
	}
	if office == "" {
		t.Fatal("office was not imported")
	}
	if r := f.post("/api/v1/network/wireguard/preview", token, `{"action":"rename","target":"`+office+`"}`); r.Code != http.StatusBadRequest {
		t.Fatalf("rename without a name: %d", r.Code)
	}
	taken := f.post("/api/v1/network/wireguard/preview", token, `{"action":"rename","target":"`+office+`","name":"site-b"}`)
	if taken.Code != http.StatusConflict || !strings.Contains(taken.Body.String(), "wireguard_profile_name_taken") {
		t.Fatalf("rename onto a taken name: %d %s", taken.Code, taken.Body.String())
	}
	p := decodePreview(t, f.post("/api/v1/network/wireguard/preview", token, `{"action":"rename","target":"`+office+`","name":"Head office"}`))
	if r := f.post("/api/v1/network/operations/"+p.ID+"/acknowledge", token, `{"revision":"`+p.Revision+`"}`); r.Code != http.StatusAccepted {
		t.Fatalf("rename: %d %s", r.Code, r.Body.String())
	}
	if err := json.Unmarshal(f.get("/api/v1/network/wireguard", token).Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	renamed := false
	for _, profile := range status.Profiles {
		if profile.ID == office && profile.Name == "Head office" {
			renamed = true
		}
		if profile.Name == "office" {
			t.Fatal("the old name survived the rename")
		}
	}
	if !renamed {
		t.Fatalf("the rename lost the identity: %+v", status.Profiles)
	}
}

func TestNetworkRequestRejectsTrailingJSONAndUnknownFields(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	token := f.issueToken(t, "admin", "strict", nil, nil)
	for _, body := range []string{`{"action":"disable"} {"action":"enable"}`, `{"action":"disable","script":"no"}`} {
		if r := f.post("/api/v1/network/interfaces/sim-enp1s0/preview", token, body); r.Code != 400 {
			t.Fatalf("accepted ambiguous body: %d", r.Code)
		}
	}
}
