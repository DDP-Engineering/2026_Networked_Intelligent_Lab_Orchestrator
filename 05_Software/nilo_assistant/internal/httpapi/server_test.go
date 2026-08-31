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
)

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
	handler, err := NewHandler(HandlerOptions{Tokens: verifier, Profile: "simulation"})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	for _, test := range []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "basic", authorization: "Basic credentials"},
		{name: "empty bearer", authorization: "Bearer"},
		{name: "extra bearer field", authorization: "Bearer nilo_unknown extra"},
		{name: "unknown", authorization: "Bearer nilo_unknown"},
		{name: "revoked", authorization: "Bearer nilo_revoked"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
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
