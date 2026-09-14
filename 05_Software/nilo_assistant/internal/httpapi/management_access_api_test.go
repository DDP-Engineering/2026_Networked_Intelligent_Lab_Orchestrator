package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

func TestManagementAccessPayloadUsesEmptyAddressArrays(t *testing.T) {
	payload := managementAccessPayload(network.ManagementAccessStatus{
		Interfaces: []network.ManagementAccessInterface{
			{
				Name: "wlan0", Kind: network.KindWiFi, State: network.StateConnected,
				IPv4: []string{"192.168.1.103/24"},
			},
			{
				Name: "eth1", Kind: network.KindEthernet, State: network.StateDisconnected,
			},
		},
	})

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Interfaces []struct {
			IPv4 json.RawMessage `json:"ipv4"`
			IPv6 json.RawMessage `json:"ipv6"`
		} `json:"interfaces"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	want := [][2]string{
		{`["192.168.1.103/24"]`, `[]`},
		{`[]`, `[]`},
	}
	if len(decoded.Interfaces) != len(want) {
		t.Fatalf("interfaces = %d, want %d: %s", len(decoded.Interfaces), len(want), body)
	}
	for index, entry := range decoded.Interfaces {
		if string(entry.IPv4) != want[index][0] || string(entry.IPv6) != want[index][1] {
			t.Fatalf("interface %d addresses = ipv4:%s ipv6:%s, want ipv4:%s ipv6:%s", index, entry.IPv4, entry.IPv6, want[index][0], want[index][1])
		}
	}
}

type noOpManagementFirewall struct{}

func (noOpManagementFirewall) ApplyManagementAccess(context.Context, network.ManagementAccessConfiguration) error {
	return nil
}

type recordingManagementFirewall struct {
	configuration network.ManagementAccessConfiguration
}

func (firewall *recordingManagementFirewall) ApplyManagementAccess(_ context.Context, configuration network.ManagementAccessConfiguration) error {
	firewall.configuration = configuration
	return nil
}

type recoveryManagementProvider struct{}

func (recoveryManagementProvider) Source() network.Source { return network.SourceSimulation }

func (recoveryManagementProvider) Interfaces(context.Context) ([]network.Interface, error) {
	return []network.Interface{
		{Name: "enp1s0", AdapterID: "02:00:00:00:00:01", Kind: network.KindEthernet, Physical: true, State: network.StateConnected, IPv4: []string{"192.168.10.24/24"}},
		{Name: "eth9", AdapterID: "02:00:00:00:00:09", Kind: network.KindEthernet, Physical: true, State: network.StateConnected, IPv4: []string{"169.254.44.21/16"}},
	}, nil
}

func TestTemporaryRecoveryReachabilityKeepsTheExistingHTTPSIdentityAndAccessControls(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieHTTPSDeployment)
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"enp1s0"}, Port: 8443,
		TLSNames: []string{"ddp-remote.local", "nilo.local"},
	}
	firewall := &recordingManagementFirewall{}
	store := sqlite.NewManagementAccessRepository(fixture.database)
	service, err := network.NewManagementAccessService(context.Background(), network.ManagementAccessOptions{
		Provider: recoveryManagementProvider{}, Store: store, Firewall: firewall, Initial: &configuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetRecoveryInterface(context.Background(), "eth9"); err != nil {
		t.Fatal(err)
	}
	if got := firewall.configuration.Interfaces; !slices.Equal(got, []string{"enp1s0", "eth9"}) {
		t.Fatalf("temporary admission = %v, want commissioned plus recovery interfaces", got)
	}
	persisted, present, err := store.LoadManagementAccess(context.Background())
	if err != nil || !present || !slices.Equal(persisted.Configuration.Interfaces, []string{"enp1s0"}) {
		t.Fatalf("persistent management selection changed during recovery: %+v present=%t err=%v", persisted, present, err)
	}

	options := fixture.options
	options.Profile = "appliance"
	options.CookieSecurity = CookieHTTPSDeployment
	options.ManagementAccess = service
	fixture.handler, err = NewHandler(options)
	if err != nil {
		t.Fatal(err)
	}
	if response := fixture.get("/api/v1/status", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated recovery request = %d, want 401", response.Code)
	}
	narrow := fixture.issueToken(t, "admin", "recovery-read-only", []auth.Permission{auth.PermissionNetworkRead}, nil)
	if response := fixture.get("/api/v1/network/management-access", narrow); response.Code != http.StatusForbidden {
		t.Fatalf("underprivileged recovery request = %d, want 403", response.Code)
	}
	session := fixture.login(t, "admin", administratorPassword)
	if !session.sessionCookie.Secure || !session.sessionCookie.HttpOnly {
		t.Fatalf("recovery session cookie is not the HTTPS deployment cookie: %+v", session.sessionCookie)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/network/management-access", bytes.NewBufferString(`{"interfaces":["enp1s0"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session.sessionCookie)
	if response := fixture.do(request); response.Code != http.StatusForbidden {
		t.Fatalf("recovery mutation without CSRF = %d, want 403", response.Code)
	}
	if response := fixture.sessionGet("/api/v1/status", session); response.Code != http.StatusOK {
		t.Fatalf("authenticated recovery request = %d, want 200", response.Code)
	}
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

func TestManagementAccessAPIReturnsStructuredAdapterRoleConflict(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	coordinator := network.NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []network.AdapterRoleClaim {
		return []network.AdapterRoleClaim{{
			AdapterID: "02:00:00:00:00:02", Interface: "wlan0", Role: network.AdapterRoleDevice,
			ResourceID: "device-network-1", ResourceName: "PLC network",
		}}
	})
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"enp1s0"}, Port: 8443, TLSNames: []string{"ddp-remote.local"},
	}
	service, err := network.NewManagementAccessService(context.Background(), network.ManagementAccessOptions{
		Provider: fixture.provider, Store: sqlite.NewManagementAccessRepository(fixture.database),
		Firewall: noOpManagementFirewall{}, Initial: &configuration, Coordinator: coordinator,
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
	token := fixture.issueToken(t, "admin", "role-conflict", nil, nil)
	response := fixture.put("/api/v1/network/management-access", token, `{"interfaces":["enp1s0","wlan0"],"confirm_current_interface":true}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"error":"network_interface_role_conflict"`) ||
		!strings.Contains(response.Body.String(), `"conflicting_resource_name":"PLC network"`) {
		t.Fatalf("conflict status=%d body=%s", response.Code, response.Body.String())
	}
}
