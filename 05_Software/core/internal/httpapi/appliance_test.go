package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"nilo.local/nilo/internal/buildinfo"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

func TestApplianceStatusDoesNotAdvertiseSimulatedInstruments(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieHTTPSDeployment)
	// The fixture network driver is harmless; this assertion targets inventory
	// and runtime metadata, not physical device management.
	provider := network.NewSimulation(f.clock.Now)
	service, err := network.NewService(network.ServiceOptions{Provider: provider, History: sqlite.NewNetworkRepository(f.database), Now: f.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	operations, err := network.NewOperationManager(network.OperationManagerOptions{Changer: provider, Observer: provider, Now: f.clock.Now, NewID: func() string { return "test" }})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(HandlerOptions{Identity: f.service, Profile: "appliance", Build: buildinfo.Current(), Network: service, Operations: operations, CookieSecurity: CookieHTTPSDeployment})
	if err != nil {
		t.Fatal(err)
	}
	f.handler = handler
	token := f.issueToken(t, "admin", "appliance", nil, nil)
	r := f.get("/api/v1/status", token)
	var body map[string]any
	_ = json.Unmarshal(r.Body.Bytes(), &body)
	if r.Code != http.StatusOK || body["simulation"] != false || body["profile"] != "appliance" {
		t.Fatalf("wrong runtime: %s", r.Body.String())
	}
	for _, path := range []string{"/api/v1/integrations", "/api/v1/devices"} {
		r = f.get(path, token)
		var list struct {
			Items []any `json:"items"`
		}
		_ = json.Unmarshal(r.Body.Bytes(), &list)
		if r.Code != 200 || list.Items == nil || len(list.Items) != 0 {
			t.Fatalf("live inventory must be empty: %s", r.Body.String())
		}
	}
}
