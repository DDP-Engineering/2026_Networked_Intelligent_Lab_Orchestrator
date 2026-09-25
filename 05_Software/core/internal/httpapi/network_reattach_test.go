package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/simulation"
)

type operationListBody struct {
	APIVersion string `json:"api_version"`
	Items      []struct {
		ID              string `json:"id"`
		Action          string `json:"action"`
		Target          string `json:"target"`
		State           string `json:"state"`
		ActorID         string `json:"actor_id"`
		ConfirmDeadline string `json:"confirm_deadline"`
	} `json:"items"`
}

// TestOperationListLetsAReloadedInterfaceFindThePendingChange covers
// REQ-FUNC-97c1e5d8. A page reloaded mid-change no longer holds the operation
// identity, so without this route the countdown simply disappears and the
// administrator can neither keep nor restore what they started.
func TestOperationListLetsAReloadedInterfaceFindThePendingChange(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "network", nil, nil)

	preview := decodePreview(t, f.post(
		"/api/v1/network/interfaces/sim-enp1s0/preview", bearer, `{"action":"disable"}`))
	acknowledged := f.post("/api/v1/network/operations/"+preview.ID+"/acknowledge",
		bearer, `{"revision":"`+preview.Revision+`"}`)
	if acknowledged.Code != http.StatusAccepted {
		t.Fatalf("acknowledge = %d, want %d: %s", acknowledged.Code, http.StatusAccepted, acknowledged.Body.String())
	}

	response := f.get("/api/v1/network/operations", bearer)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var body operationListBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.APIVersion == "" {
		t.Fatal("the listing carries no API version")
	}

	var found bool
	for _, item := range body.Items {
		if item.ID != preview.ID {
			continue
		}
		found = true
		if item.State != "awaiting_confirmation" {
			t.Fatalf("state = %q, want awaiting_confirmation", item.State)
		}
		if item.ConfirmDeadline == "" {
			t.Fatal("the pending operation carries no deadline, so no countdown can be drawn")
		}
		if item.Action != "interface.disable" || item.Target != "sim-enp1s0" {
			t.Fatalf("action/target = %q/%q", item.Action, item.Target)
		}
		if item.ActorID == "" {
			t.Fatal("the pending operation names no actor, so nobody could be asked to confirm it")
		}
	}
	if !found {
		t.Fatalf("the pending operation is not listed: %s", response.Body.String())
	}
}

// TestOperationListIsRefusedWithoutTheNetworkReadPermission keeps the new route
// inside the same authorization table as the rest of the API.
func TestOperationListIsRefusedWithoutTheNetworkReadPermission(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	if anonymous := f.get("/api/v1/network/operations", ""); anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want %d", anonymous.Code, http.StatusUnauthorized)
	}
	narrowed := f.issueToken(t, "admin", "no-network", []auth.Permission{auth.PermissionStatusRead}, nil)
	if refused := f.get("/api/v1/network/operations", narrowed); refused.Code != http.StatusForbidden {
		t.Fatalf("narrowed token = %d, want %d", refused.Code, http.StatusForbidden)
	}
}
