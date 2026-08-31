package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/buildinfo"
	"nilo.local/nilo/internal/simulation"
)

func TestIntegrationInventoryReturnsExactDeterministicSimulationEnvelope(t *testing.T) {
	t.Parallel()

	catalog, err := simulation.NewCatalog(simulation.ScenarioHealthy)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	verifier := validTokenRepository()
	handler, err := NewHandler(HandlerOptions{Tokens: verifier, Profile: "simulation", Catalog: catalog})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	const want = "{\"api_version\":\"nilo.http/1\",\"scenario\":\"healthy\",\"items\":[{\"id\":\"simulation.fake-python\",\"display_name\":\"SIMULATION - Fake Python Integration\",\"status\":\"healthy\",\"simulation\":true,\"operations\":[\"artifact\",\"crash\",\"echo\",\"fail\",\"sleep\"]}]}\n"

	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/integrations", nil)
		request.Header.Set("Authorization", "Bearer nilo_valid")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, http.StatusOK)
		}
		if response.Body.String() != want {
			t.Fatalf("attempt %d body = %s, want %s", attempt, response.Body.String(), want)
		}
	}
}

func TestDeviceInventoryReturnsExactDeterministicSimulationEnvelope(t *testing.T) {
	t.Parallel()

	catalog := mustCatalog(t, simulation.ScenarioUnavailable)
	handler, err := NewHandler(HandlerOptions{
		Tokens: validTokenRepository(), Profile: "simulation", Catalog: catalog,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	const want = "{\"api_version\":\"nilo.http/1\",\"scenario\":\"unavailable\",\"items\":[{\"id\":\"simulation.fake-python.device.stable-001\",\"integration_id\":\"simulation.fake-python\",\"display_name\":\"SIMULATION - Electronics Bench 001\",\"kind\":\"simulated-electronics-bench\",\"status\":\"unavailable\",\"simulation\":true,\"capabilities\":[\"artifact\",\"crash\",\"echo\",\"fail\",\"sleep\"]}]}\n"

	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
		request.Header.Set("Authorization", "Bearer nilo_valid")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, http.StatusOK)
		}
		if response.Body.String() != want {
			t.Fatalf("attempt %d body = %s, want %s", attempt, response.Body.String(), want)
		}
	}
}

func validTokenRepository() *tokenRepositoryStub {
	return &tokenRepositoryStub{verify: func(_ context.Context, plaintext string) (auth.TokenRecord, error) {
		if plaintext != "nilo_valid" {
			return auth.TokenRecord{}, auth.ErrTokenNotFound
		}
		return auth.TokenRecord{ID: "token-id", UserID: "user-id"}, nil
	}}
}

func mustCatalog(t *testing.T, scenario simulation.Scenario) simulation.Catalog {
	t.Helper()
	catalog, err := simulation.NewCatalog(scenario)
	if err != nil {
		t.Fatalf("NewCatalog(%q): %v", scenario, err)
	}
	return catalog
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

	verifier := &tokenRepositoryStub{verify: func(_ context.Context, plaintext string) (auth.TokenRecord, error) {
		if plaintext != "nilo_valid" {
			return auth.TokenRecord{}, auth.ErrTokenNotFound
		}
		return auth.TokenRecord{ID: "token-id", UserID: "user-id"}, nil
	}}
	handler, err := NewHandler(HandlerOptions{
		Tokens:  verifier,
		Profile: "simulation",
		Catalog: mustCatalog(t, simulation.ScenarioHealthy),
		Build: buildinfo.Info{
			Name: "nilo", Version: "test-version", IntegrationProtocol: "nilo.integration/1",
		},
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	request.Header.Set("Authorization", "Bearer nilo_valid")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", response.Header().Get("Content-Type"))
	}
	var status struct {
		APIVersion string         `json:"api_version"`
		Status     string         `json:"status"`
		Profile    string         `json:"profile"`
		Simulation bool           `json:"simulation"`
		Build      buildinfo.Info `json:"build"`
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
}

func TestProtectedAPIRejectsMissingMalformedUnknownAndRevokedTokens(t *testing.T) {
	t.Parallel()

	verifier := &tokenRepositoryStub{verify: func(_ context.Context, plaintext string) (auth.TokenRecord, error) {
		switch plaintext {
		case "nilo_revoked":
			return auth.TokenRecord{}, auth.ErrTokenRevoked
		default:
			return auth.TokenRecord{}, auth.ErrTokenNotFound
		}
	}}
	handler, err := NewHandler(HandlerOptions{
		Tokens: verifier, Profile: "simulation", Catalog: mustCatalog(t, simulation.ScenarioHealthy),
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

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
		{name: "revoked", path: "/api/v1/status", authorization: "Bearer nilo_revoked"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

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

type tokenRepositoryStub struct {
	verify func(context.Context, string) (auth.TokenRecord, error)
}

func (repository *tokenRepositoryStub) CreateToken(context.Context, string, string, time.Time, auth.TokenVerifier) error {
	return errors.New("unexpected CreateToken call")
}

func (repository *tokenRepositoryStub) VerifyToken(ctx context.Context, plaintext string) (auth.TokenRecord, error) {
	return repository.verify(ctx, plaintext)
}

func (repository *tokenRepositoryStub) RevokeToken(context.Context, string, time.Time) error {
	return errors.New("unexpected RevokeToken call")
}
