package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/buildinfo"
)

const simulationProfile = "simulation"

// ServeOptions controls one loopback-confined simulation HTTP server.
type ServeOptions struct {
	Address         string
	Handler         http.Handler
	ShutdownTimeout time.Duration
	OnListening     func(net.Addr)
}

// Serve runs until the context is cancelled or the HTTP server fails.
func Serve(ctx context.Context, options ServeOptions) error {
	if ctx == nil {
		return errors.New("server context is required")
	}
	if options.Handler == nil {
		return errors.New("HTTP handler is required")
	}
	if options.ShutdownTimeout <= 0 {
		return errors.New("positive shutdown timeout is required")
	}
	if err := ValidateSimulationBind(options.Address); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", options.Address)
	if err != nil {
		return fmt.Errorf("listen on simulation address: %w", err)
	}
	server := &http.Server{
		Handler:           options.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if options.OnListening != nil {
		options.OnListening(listener.Addr())
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve simulation HTTP API: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), options.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shut down simulation HTTP API: %w", err)
		}
		serveErr := <-serveResult
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve simulation HTTP API during shutdown: %w", serveErr)
		}
		return nil
	}
}

// HandlerOptions contains the dependencies for the protected HTTP API.
type HandlerOptions struct {
	Tokens  auth.TokenRepository
	Profile string
	Build   buildinfo.Info
}

// NewHandler builds the protected Bench Core HTTP API.
func NewHandler(options HandlerOptions) (http.Handler, error) {
	if options.Tokens == nil {
		return nil, errors.New("token repository is required")
	}
	if options.Profile != simulationProfile {
		return nil, errors.New("HTTP status API requires the simulation profile")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(statusResponse{
			APIVersion: "nilo.http/1",
			Status:     "healthy",
			Profile:    options.Profile,
			Simulation: true,
			Build:      options.Build,
		})
	})
	return authenticate(options.Tokens, mux), nil
}

type statusResponse struct {
	APIVersion string         `json:"api_version"`
	Status     string         `json:"status"`
	Profile    string         `json:"profile"`
	Simulation bool           `json:"simulation"`
	Build      buildinfo.Info `json:"build"`
}

func authenticate(tokens auth.TokenRepository, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		plaintext, ok := bearerToken(request.Header.Get("Authorization"))
		if !ok {
			writeUnauthorized(response)
			return
		}
		if _, err := tokens.VerifyToken(request.Context(), plaintext); err != nil {
			writeUnauthorized(response)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func bearerToken(header string) (string, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 || fields[0] != "Bearer" || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}

func writeUnauthorized(response http.ResponseWriter) {
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("WWW-Authenticate", "Bearer")
	response.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(response).Encode(struct {
		Error string `json:"error"`
	}{Error: "unauthorized"})
}
