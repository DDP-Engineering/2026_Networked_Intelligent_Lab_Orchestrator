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

type previewBody struct {
	ID       string `json:"id"`
	Action   string `json:"action"`
	Target   string `json:"target"`
	State    string `json:"state"`
	Revision string `json:"revision"`
}

func (f apiFixture) post(path, bearer, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	return f.do(request)
}

func decodePreview(t *testing.T, response *httptest.ResponseRecorder) previewBody {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("preview = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body previewBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if body.ID == "" || body.Revision == "" || body.State != "previewed" {
		t.Fatalf("preview body is incomplete: %+v", body)
	}
	return body
}

// Reading network state is for everyone; changing it is for administrators.
// The separation is proven at the route, because the route is what is reachable.
func TestNetworkManagementRoutesAreAdministratorOnly(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)

	for _, owner := range []string{"operator", "viewer"} {
		owner := owner
		t.Run(owner, func(t *testing.T) {
			bearer := f.issueToken(t, owner, "network-"+owner, nil, nil)
			for _, path := range []string{
				"/api/v1/network/interfaces/sim-enp1s0/preview",
				"/api/v1/network/wireguard/preview",
				"/api/v1/network/wireguard/install/preview",
			} {
				response := f.post(path, bearer, `{"action":"disable","target":"sim-enp1s0"}`)
				if response.Code != http.StatusForbidden {
					t.Fatalf("%s as %s = %d, want %d: %s",
						path, owner, response.Code, http.StatusForbidden, response.Body.String())
				}
			}
		})
	}
}

// A token narrowed away from a management permission is refused even though its
// owner is an administrator, because a scope only ever subtracts.
func TestNetworkManagementRefusesANarrowedAdministratorToken(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	readOnly := f.issueToken(t, "admin", "read-only", []auth.Permission{auth.PermissionNetworkRead}, nil)

	response := f.post("/api/v1/network/interfaces/sim-enp1s0/preview", readOnly, `{"action":"disable"}`)
	if response.Code != http.StatusForbidden {
		t.Fatalf("narrowed token previewing a change = %d, want %d", response.Code, http.StatusForbidden)
	}
	if got := f.get("/api/v1/network/status", readOnly); got.Code != http.StatusOK {
		t.Fatalf("the same token reading status = %d, want %d", got.Code, http.StatusOK)
	}
}

// The whole journey an administrator takes: see what will happen, agree to it,
// then separately confirm that it did not cut their own access.
func TestAConnectivityChangeIsPreviewedAcknowledgedThenConfirmed(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "network", nil, nil)

	preview := decodePreview(t, f.post(
		"/api/v1/network/interfaces/sim-enp1s0/preview", bearer, `{"action":"disable"}`))
	if preview.Action != "interface.disable" || preview.Target != "sim-enp1s0" {
		t.Fatalf("preview describes %s on %s", preview.Action, preview.Target)
	}

	acknowledged := f.post("/api/v1/network/operations/"+preview.ID+"/acknowledge",
		bearer, `{"revision":"`+preview.Revision+`"}`)
	if acknowledged.Code != http.StatusAccepted {
		t.Fatalf("acknowledge = %d, want %d: %s",
			acknowledged.Code, http.StatusAccepted, acknowledged.Body.String())
	}

	var running struct {
		State           string `json:"state"`
		ConfirmDeadline string `json:"confirm_deadline"`
	}
	if err := json.Unmarshal(acknowledged.Body.Bytes(), &running); err != nil {
		t.Fatalf("decode acknowledgement: %v", err)
	}
	if running.State != "awaiting_confirmation" {
		t.Fatalf("state after acknowledgement = %q, want awaiting_confirmation", running.State)
	}
	if running.ConfirmDeadline == "" {
		t.Fatal("an operation awaiting confirmation carries no deadline")
	}

	confirmed := f.post("/api/v1/network/operations/"+preview.ID+"/confirm", bearer, `{}`)
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirm = %d, want %d: %s", confirmed.Code, http.StatusOK, confirmed.Body.String())
	}
	if !strings.Contains(confirmed.Body.String(), `"state":"confirmed"`) {
		t.Fatalf("confirmed body = %s", confirmed.Body.String())
	}
}

func TestAnAcknowledgementIsRefusedUnlessItMatchesThePreview(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "network", nil, nil)

	t.Run("a revision that was never shown", func(t *testing.T) {
		preview := decodePreview(t, f.post(
			"/api/v1/network/interfaces/sim-wlan0/preview", bearer, `{"action":"disable"}`))
		response := f.post("/api/v1/network/operations/"+preview.ID+"/acknowledge",
			bearer, `{"revision":"0000000000000000000000000000000000000000000000000000000000000000"}`)
		if response.Code != http.StatusConflict {
			t.Fatalf("mismatched revision = %d, want %d: %s",
				response.Code, http.StatusConflict, response.Body.String())
		}
	})

	t.Run("replayed", func(t *testing.T) {
		preview := decodePreview(t, f.post(
			"/api/v1/network/interfaces/sim-enp1s0/preview", bearer, `{"action":"enable"}`))
		first := f.post("/api/v1/network/operations/"+preview.ID+"/acknowledge",
			bearer, `{"revision":"`+preview.Revision+`"}`)
		if first.Code != http.StatusAccepted {
			t.Fatalf("first acknowledge = %d: %s", first.Code, first.Body.String())
		}
		second := f.post("/api/v1/network/operations/"+preview.ID+"/acknowledge",
			bearer, `{"revision":"`+preview.Revision+`"}`)
		if second.Code != http.StatusConflict {
			t.Fatalf("replayed acknowledge = %d, want %d: %s",
				second.Code, http.StatusConflict, second.Body.String())
		}
		if !strings.Contains(second.Body.String(), "already_acknowledged") {
			t.Fatalf("replayed acknowledge body = %s", second.Body.String())
		}
	})
}

// An unknown operation must not be distinguishable from one belonging to
// somebody else by its status code alone.
func TestReadingAnUnknownOperationIsNotFound(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "network", nil, nil)
	if response := f.get("/api/v1/network/operations/does-not-exist", bearer); response.Code != http.StatusNotFound {
		t.Fatalf("unknown operation = %d, want %d", response.Code, http.StatusNotFound)
	}
}

// Progress is readable by anyone who may read network state, because watching a
// change land is telemetry rather than control.
func TestOperationProgressIsReadableByEveryRole(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	administrator := f.issueToken(t, "admin", "network", nil, nil)

	preview := decodePreview(t, f.post(
		"/api/v1/network/interfaces/sim-enp1s0/preview", administrator, `{"action":"disable"}`))

	viewer := f.issueToken(t, "viewer", "watching", nil, nil)
	response := f.get("/api/v1/network/operations/"+preview.ID, viewer)
	if response.Code != http.StatusOK {
		t.Fatalf("viewer reading operation progress = %d, want %d: %s",
			response.Code, http.StatusOK, response.Body.String())
	}

	// Reading is not acting: the same viewer may not acknowledge it.
	acknowledged := f.post("/api/v1/network/operations/"+preview.ID+"/acknowledge",
		viewer, `{"revision":"`+preview.Revision+`"}`)
	if acknowledged.Code != http.StatusForbidden {
		t.Fatalf("viewer acknowledging = %d, want %d", acknowledged.Code, http.StatusForbidden)
	}
}

// A preview is a proposal. Nothing about the appliance may change until it is
// acknowledged, and the status route is where that would show.
func TestAPreviewLeavesTheReportedStateUnchanged(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "network", nil, nil)

	before := f.get("/api/v1/network/status", bearer).Body.String()
	decodePreview(t, f.post("/api/v1/network/interfaces/sim-enp1s0/preview", bearer, `{"action":"disable"}`))
	after := f.get("/api/v1/network/status", bearer).Body.String()

	if before != after {
		t.Fatalf("a preview changed the reported state:\nbefore %s\nafter  %s", before, after)
	}
}

func TestPreviewRejectsAnActionItDoesNotOffer(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "network", nil, nil)

	for _, body := range []string{`{"action":"reboot"}`, `{"action":""}`, `{}`, `not json`} {
		response := f.post("/api/v1/network/interfaces/sim-enp1s0/preview", bearer, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("preview of %s = %d, want %d", body, response.Code, http.StatusBadRequest)
		}
	}
}
