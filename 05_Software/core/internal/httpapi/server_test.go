package httpapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/buildinfo"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/systeminfo"
)

func TestIntegrationInventoryReturnsExactDeterministicSimulationEnvelope(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "inventory", nil, nil)
	const want = "{\"api_version\":\"nilo.http/1\",\"scenario\":\"healthy\",\"items\":[{\"id\":\"simulation.bench\",\"display_name\":\"SIMULATION - Demo Integration\",\"status\":\"healthy\",\"simulation\":true,\"operations\":[\"artifact\",\"crash\",\"echo\",\"fail\",\"sleep\"]}]}\n"

	for attempt := 0; attempt < 2; attempt++ {
		response := f.get("/api/v1/integrations", bearer)
		if response.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, http.StatusOK)
		}
		if response.Body.String() != want {
			t.Fatalf("attempt %d body = %s, want %s", attempt, response.Body.String(), want)
		}
	}
}

func TestServeAcceptsHelperBackedTLSCertificate(t *testing.T) {
	options := ServeOptions{
		Address: "127.0.0.1:0", Handler: http.NotFoundHandler(),
		ShutdownTimeout: time.Second, TLSCertificate: &tls.Certificate{},
	}
	if err := validateServeTLS(options); err != nil {
		t.Fatalf("helper-backed TLS certificate rejected: %v", err)
	}
	options.Certificate = "certificate.pem"
	if err := validateServeTLS(options); err == nil {
		t.Fatal("mixed file-backed and helper-backed TLS was accepted")
	}
	options = ServeOptions{
		Address: "127.0.0.1:0", Handler: http.NotFoundHandler(), ShutdownTimeout: time.Second,
		TLSCertificateProvider: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return &tls.Certificate{}, nil },
	}
	if err := validateServeTLS(options); err != nil {
		t.Fatalf("dynamic helper-backed TLS provider rejected: %v", err)
	}
}

func TestDeviceInventoryReturnsExactDeterministicSimulationEnvelope(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioUnavailable, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "inventory", nil, nil)
	const want = "{\"api_version\":\"nilo.http/1\",\"scenario\":\"unavailable\",\"items\":[{\"id\":\"simulation.bench.device.stable-001\",\"integration_id\":\"simulation.bench\",\"display_name\":\"SIMULATION - Electronics Bench 001\",\"kind\":\"simulated-electronics-bench\",\"status\":\"unavailable\",\"simulation\":true,\"capabilities\":[\"artifact\",\"crash\",\"echo\",\"fail\",\"sleep\"]}]}\n"

	for attempt := 0; attempt < 2; attempt++ {
		response := f.get("/api/v1/devices", bearer)
		if response.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, http.StatusOK)
		}
		if response.Body.String() != want {
			t.Fatalf("attempt %d body = %s, want %s", attempt, response.Body.String(), want)
		}
	}
}

func TestServeStopsAfterContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	listening := make(chan net.Addr, 1)
	returned := make(chan error, 1)
	go func() {
		returned <- Serve(ctx, ServeOptions{
			Address:         "127.0.0.1:0",
			Handler:         http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			ShutdownTimeout: time.Second,
			OnListening:     func(address net.Addr) { listening <- address },
		})
	}()

	var address net.Addr
	select {
	case address = <-listening:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not begin listening")
	}
	cancel()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("Serve after cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancellation")
	}

	connection, err := net.DialTimeout("tcp", address.String(), 200*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Fatalf("new connection to %s succeeded after shutdown", address)
	}
}

func TestStatusReturnsVersionedSimulationBuildInformation(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	catalog, err := simulation.NewCatalog(simulation.ScenarioHealthy)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	provider := network.NewSimulatedProvider()
	networkService, err := network.NewService(network.ServiceOptions{Provider: provider})
	if err != nil {
		t.Fatalf("network.NewService: %v", err)
	}
	operations, err := network.NewOperationManager(network.OperationManagerOptions{
		Changer: network.NewSimulatedChanger(), Observer: network.NewProviderObserver(provider),
		Now: time.Now, NewID: func() string { return "operation-1" },
	})
	if err != nil {
		t.Fatalf("network.NewOperationManager: %v", err)
	}
	handler, err := NewHandler(HandlerOptions{
		Identity:       f.service,
		Profile:        "simulation",
		Catalog:        catalog,
		Network:        networkService,
		Operations:     operations,
		CookieSecurity: CookieDevelopmentLocalhost,
		Build: buildinfo.Info{
			Name: "nilo", Version: "test-version", IntegrationProtocol: "nilo.integration/1",
		},
		SystemInformation: func() systeminfo.Info {
			return systeminfo.Info{
				OperatingSystem: "Test Linux 1", KernelVersion: "6.12.0-test",
				Architecture: "arm64", StorageFreeBytes: 5 << 30, StorageTotalBytes: 16 << 30,
			}
		},
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	request.Header.Set("Authorization", "Bearer "+f.issueToken(t, "admin", "status", nil, nil))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", response.Header().Get("Content-Type"))
	}
	var status struct {
		APIVersion string          `json:"api_version"`
		Status     string          `json:"status"`
		Profile    string          `json:"profile"`
		Simulation bool            `json:"simulation"`
		Build      buildinfo.Info  `json:"build"`
		System     systeminfo.Info `json:"system"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if status.APIVersion != "nilo.http/1" || status.Status != "healthy" || status.Profile != "simulation" || !status.Simulation {
		t.Fatalf("unexpected status response: %#v", status)
	}
	if status.Build.Name != "nilo" || status.Build.Version != "test-version" || status.Build.IntegrationProtocol != "nilo.integration/1" {
		t.Fatalf("unexpected build response: %#v", status.Build)
	}
	if status.System.OperatingSystem != "Test Linux 1" || status.System.KernelVersion != "6.12.0-test" ||
		status.System.Architecture != "arm64" || status.System.StorageFreeBytes != 5<<30 ||
		status.System.StorageTotalBytes != 16<<30 {
		t.Fatalf("unexpected system response: %#v", status.System)
	}
}

func TestProtectedAPIRejectsMissingAndMalformedCredentials(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)

	for _, test := range []struct {
		name          string
		path          string
		authorization string
	}{
		{name: "missing status", path: "/api/v1/status"},
		{name: "missing integrations", path: "/api/v1/integrations"},
		{name: "missing devices", path: "/api/v1/devices"},
		{name: "basic", path: "/api/v1/status", authorization: "Basic credentials"},
		{name: "empty bearer", path: "/api/v1/status", authorization: "Bearer"},
		{name: "extra bearer field", path: "/api/v1/status", authorization: "Bearer nilo_unknown extra"},
		{name: "unknown", path: "/api/v1/status", authorization: "Bearer nilo_unknown"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}

			response := f.do(request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
			if response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("WWW-Authenticate = %q, want Bearer", response.Header().Get("WWW-Authenticate"))
			}
			if strings.Contains(response.Body.String(), "simulation") || strings.Contains(response.Body.String(), "healthy") {
				t.Fatalf("unauthorized response leaked protected status: %s", response.Body.String())
			}
		})
	}
}

func TestNewHandlerRequiresItsDependencies(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	catalog, err := simulation.NewCatalog(simulation.ScenarioHealthy)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	for _, test := range []struct {
		name    string
		options HandlerOptions
	}{
		{name: "no identity service", options: HandlerOptions{Profile: "simulation", Catalog: catalog, CookieSecurity: CookieDevelopmentLocalhost}},
		{name: "wrong profile", options: HandlerOptions{Identity: f.service, Profile: "production", Catalog: catalog, CookieSecurity: CookieDevelopmentLocalhost}},
		{name: "no catalog", options: HandlerOptions{Identity: f.service, Profile: "simulation", CookieSecurity: CookieDevelopmentLocalhost}},
		{name: "no cookie mode", options: HandlerOptions{Identity: f.service, Profile: "simulation", Catalog: catalog}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewHandler(test.options); err == nil {
				t.Fatalf("NewHandler accepted options with %s", test.name)
			}
		})
	}
}
