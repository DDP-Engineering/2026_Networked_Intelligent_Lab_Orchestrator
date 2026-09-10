package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

type noOpManagementFirewall struct{}

func (noOpManagementFirewall) ApplyManagementAccess(context.Context, network.ManagementAccessConfiguration) error {
	return nil
}

func TestManagementAccessAPIIsAdministratorOnlyCSRFProtectedAndSecretFree(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"enp1s0"}, Port: 8443,
		TLSNames: []string{"ddp-remote.local", "10.205.148.11"},
	}
	service, err := network.NewManagementAccessService(context.Background(), network.ManagementAccessOptions{
		Provider: network.NewSimulatedProvider(),
		Store:    sqlite.NewManagementAccessRepository(fixture.database),
		Firewall: noOpManagementFirewall{}, Initial: &configuration,
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
	administrator := fixture.issueToken(t, "admin", "management-admin", nil, nil)
	narrow := fixture.issueToken(t, "admin", "management-read-only", []auth.Permission{auth.PermissionNetworkRead}, nil)

	response := fixture.get("/api/v1/network/management-access", administrator)
	if response.Code != http.StatusOK {
		t.Fatalf("administrator status=%d body=%s", response.Code, response.Body.String())
	}
	for _, secretField := range []string{"password", "private_key", "preshared"} {
		if strings.Contains(response.Body.String(), secretField) {
			t.Fatalf("management response exposed secret-shaped field %q: %s", secretField, response.Body.String())
		}
	}
	if response := fixture.get("/api/v1/network/management-access", narrow); response.Code != http.StatusForbidden {
		t.Fatalf("narrow token status=%d, want 403", response.Code)
	}

	login := fixture.login(t, "admin", administratorPassword)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/network/management-access", bytes.NewBufferString(`{"interfaces":["enp1s0"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(login.sessionCookie)
	response = fixture.do(request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cookie update without CSRF status=%d, want 403; body=%s", response.Code, response.Body.String())
	}
}
