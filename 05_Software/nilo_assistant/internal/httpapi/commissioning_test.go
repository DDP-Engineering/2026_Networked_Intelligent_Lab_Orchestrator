package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

type stubCommissioner struct{}

func (stubCommissioner) CompleteCommissioning(context.Context) error { return nil }

// Completing commissioning fixes where management access is offered for the
// rest of the appliance's life, so it is administrator-only and carries the
// same CSRF protection as every other management change.
func TestCommissioningAPIIsAdministratorOnlyAndCSRFProtected(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	service, err := network.NewManagementAccessService(context.Background(), network.ManagementAccessOptions{
		Provider:     network.NewSimulatedProvider(),
		Store:        sqlite.NewManagementAccessRepository(fixture.database),
		Firewall:     noOpManagementFirewall{},
		Provisioning: true, ProvisioningTLSNames: []string{"ddp-remote.local"},
		Commissioner: stubCommissioner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	options := fixture.options
	options.ManagementAccess = service
	fixture.handler, err = NewHandler(options)
	if err != nil {
		t.Fatal(err)
	}

	narrow := fixture.issueToken(t, "admin", "commissioning-read-only", []auth.Permission{auth.PermissionNetworkRead}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/commissioning/complete",
		bytes.NewBufferString(`{"interfaces":["enp1s0"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+narrow)
	if response := fixture.do(request); response.Code != http.StatusForbidden {
		t.Fatalf("narrow token status=%d, want 403", response.Code)
	}

	login := fixture.login(t, "admin", administratorPassword)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/commissioning/complete",
		bytes.NewBufferString(`{"interfaces":["enp1s0"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(login.sessionCookie)
	if response := fixture.do(request); response.Code != http.StatusForbidden {
		t.Fatalf("cookie request without CSRF status=%d, want 403", response.Code)
	}

	administrator := fixture.issueToken(t, "admin", "commissioning-admin", nil, nil)
	request = httptest.NewRequest(http.MethodPost, "/api/v1/commissioning/complete",
		bytes.NewBufferString(`{"interfaces":["enp1s0"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+administrator)
	response := fixture.do(request)
	if response.Code != http.StatusOK {
		t.Fatalf("administrator status=%d body=%s", response.Code, response.Body.String())
	}

	// The step is one way: a second attempt is refused rather than repeated.
	request = httptest.NewRequest(http.MethodPost, "/api/v1/commissioning/complete",
		bytes.NewBufferString(`{"interfaces":["wlan0"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+administrator)
	if response := fixture.do(request); response.Code != http.StatusConflict {
		t.Fatalf("second commissioning status=%d, want 409", response.Code)
	}
}

// An appliance must never be commissioned into unreachability.
func TestCommissioningAPIRefusesAnUnusableAllowlist(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	service, err := network.NewManagementAccessService(context.Background(), network.ManagementAccessOptions{
		Provider:     network.NewSimulatedProvider(),
		Store:        sqlite.NewManagementAccessRepository(fixture.database),
		Firewall:     noOpManagementFirewall{},
		Provisioning: true, ProvisioningTLSNames: []string{"ddp-remote.local"},
		Commissioner: stubCommissioner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	options := fixture.options
	options.ManagementAccess = service
	fixture.handler, err = NewHandler(options)
	if err != nil {
		t.Fatal(err)
	}
	administrator := fixture.issueToken(t, "admin", "commissioning-admin", nil, nil)

	for _, body := range []string{`{"interfaces":[]}`, `{"interfaces":["veth9f2a41"]}`, `{"interfaces":["lo"]}`} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/commissioning/complete", bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+administrator)
		if response := fixture.do(request); response.Code != http.StatusConflict {
			t.Fatalf("commissioning %s status=%d, want 409", body, response.Code)
		}
	}

	// The appliance is still provisioning after every refusal, so a further
	// attempt with a usable allowlist still works.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/commissioning/complete",
		bytes.NewBufferString(`{"interfaces":["enp1s0"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+administrator)
	if response := fixture.do(request); response.Code != http.StatusOK {
		t.Fatalf("commissioning after refusals status=%d, want 200", response.Code)
	}
}
