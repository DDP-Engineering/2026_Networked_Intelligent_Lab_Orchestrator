package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/buildinfo"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/simulation"
)

const (
	simulationProfile = "simulation"
	apiVersion        = "nilo.http/1"
)

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
	Identity       *identity.Service
	Profile        string
	Build          buildinfo.Info
	Catalog        simulation.Catalog
	CookieSecurity CookieSecurity
}

// NewHandler builds the protected Bench Core HTTP API. Every route below
// declares the permission it requires, so authorization is decided in one
// place rather than inside each handler.
func NewHandler(options HandlerOptions) (http.Handler, error) {
	if options.Identity == nil {
		return nil, errors.New("identity service is required")
	}
	if options.Profile != simulationProfile {
		return nil, errors.New("HTTP status API requires the simulation profile")
	}
	if options.Catalog.Scenario() == "" {
		return nil, errors.New("simulation catalog is required")
	}
	if _, err := ParseCookieSecurity(string(options.CookieSecurity)); err != nil {
		return nil, err
	}

	protected := http.NewServeMux()
	protected.Handle("GET /api/v1/status", authorize(auth.PermissionStatusRead, func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, statusResponse{
			APIVersion: apiVersion,
			Status:     "healthy",
			Profile:    options.Profile,
			Simulation: true,
			Build:      options.Build,
		})
	}))
	protected.Handle("GET /api/v1/integrations", authorize(auth.PermissionInventoryRead, func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, integrationListResponse{
			APIVersion: apiVersion,
			Scenario:   options.Catalog.Scenario(),
			Items:      options.Catalog.Integrations(),
		})
	}))
	protected.Handle("GET /api/v1/devices", authorize(auth.PermissionInventoryRead, func(response http.ResponseWriter, _ *http.Request) {
		writeJSON(response, http.StatusOK, deviceListResponse{
			APIVersion: apiVersion,
			Scenario:   options.Catalog.Scenario(),
			Items:      options.Catalog.Devices(),
		})
	}))
	// Identity and token administration. Every route declares its permission
	// here, beside the read routes above, so one table answers "what may an
	// actor do" for the whole API rather than each handler answering for itself.
	protected.Handle("GET /api/v1/users", authorize(auth.PermissionUsersRead, handleListUsers(options.Identity)))
	protected.Handle("POST /api/v1/users", authorize(auth.PermissionUsersManage, handleCreateUser(options.Identity)))
	protected.Handle("PUT /api/v1/users/{username}/role", authorize(auth.PermissionUsersManage, handleSetRole(options.Identity)))
	protected.Handle("PUT /api/v1/users/{username}/enabled", authorize(auth.PermissionUsersManage, handleSetEnabled(options.Identity)))
	protected.Handle("PUT /api/v1/users/{username}/password", authorize(auth.PermissionUsersManage, handleResetPassword(options.Identity)))
	protected.Handle("GET /api/v1/tokens", authorize(auth.PermissionTokensReadAll, handleListTokens(options.Identity)))
	protected.Handle("POST /api/v1/tokens", authorize(auth.PermissionTokensManageAll, handleCreateToken(options.Identity)))
	protected.Handle("POST /api/v1/tokens/{id}/revoke", authorize(auth.PermissionTokensManageAll, handleRevokeToken(options.Identity)))
	// Reading one's own session is identity, not authorization, so it carries
	// no permission; logout is gated by the CSRF check in the middleware.
	protected.HandleFunc("GET /api/v1/auth/session", handleCurrentSession)
	protected.HandleFunc("POST /api/v1/auth/logout", handleLogout(options.Identity, options.CookieSecurity))

	root := http.NewServeMux()
	root.HandleFunc("POST /api/v1/auth/login", handleLogin(options.Identity, options.CookieSecurity))
	root.Handle("/", resolveIdentity(options.Identity, options.CookieSecurity, protected))
	return root, nil
}

type integrationListResponse struct {
	APIVersion string                   `json:"api_version"`
	Scenario   simulation.Scenario      `json:"scenario"`
	Items      []simulation.Integration `json:"items"`
}

type deviceListResponse struct {
	APIVersion string              `json:"api_version"`
	Scenario   simulation.Scenario `json:"scenario"`
	Items      []simulation.Device `json:"items"`
}

type statusResponse struct {
	APIVersion string         `json:"api_version"`
	Status     string         `json:"status"`
	Profile    string         `json:"profile"`
	Simulation bool           `json:"simulation"`
	Build      buildinfo.Info `json:"build"`
}
