package httpapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"

	"nilo.local/nilo/internal/activity"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/bench"
	"nilo.local/nilo/internal/buildinfo"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/power"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/systeminfo"
)

const (
	simulationProfile = "simulation"
	apiVersion        = "nilo.http/1"
)

// ServeOptions controls a loopback development or explicitly configured TLS server.
type ServeOptions struct {
	Certificate string
	Key         string
	// TLSCertificate is the appliance-only helper-backed certificate. It is
	// mutually exclusive with the development file-backed pair.
	TLSCertificate *tls.Certificate
	// TLSCertificateProvider loads the helper's current public generation for
	// each handshake so a friendly-name rotation takes effect without restart.
	TLSCertificateProvider func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	Address                string
	Handler                http.Handler
	ShutdownTimeout        time.Duration
	OnListening            func(net.Addr)
	// ManagementGate makes a wildcard appliance listener safe by admitting
	// only destination addresses currently carried by enabled interfaces.
	ManagementGate ManagementConnectionGate
}

// ManagementConnectionGate is the pre-HTTP interface allowlist.
type ManagementConnectionGate interface {
	Allows(netip.Addr) bool
}

type localAddressContextKey struct{}

// Serve runs until the context is cancelled or the HTTP server fails.
func Serve(ctx context.Context, options ServeOptions) error {
	if ctx == nil {
		return errors.New("server context is required")
	}
	if options.Handler == nil {
		return errors.New("HTTP handler is required")
	}
	if err := validateServeTLS(options); err != nil {
		return err
	}
	if options.ShutdownTimeout <= 0 {
		return errors.New("positive shutdown timeout is required")
	}
	if options.ManagementGate == nil {
		var err error
		if options.TLSCertificate != nil || options.TLSCertificateProvider != nil {
			err = validateListenerTopology(options.Address)
		} else {
			err = ValidateListener(options.Address, options.Certificate, options.Key)
		}
		if err != nil {
			return err
		}
	} else {
		var err error
		if options.TLSCertificate != nil || options.TLSCertificateProvider != nil {
			err = validateManagementListenerTopology(options.Address, options.ManagementGate)
		} else {
			err = ValidateManagementListener(options.Address, options.Certificate, options.Key, options.ManagementGate)
		}
		if err != nil {
			return err
		}
	}

	var tlsConfig *tls.Config
	if options.TLSCertificate != nil {
		certificate := *options.TLSCertificate
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	} else if options.TLSCertificateProvider != nil {
		tlsConfig = &tls.Config{GetCertificate: options.TLSCertificateProvider, MinVersion: tls.VersionTLS12}
	} else if options.Certificate != "" {
		certificate, err := tls.LoadX509KeyPair(options.Certificate, options.Key)
		if err != nil {
			return fmt.Errorf("load listener certificate: %w", err)
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	}
	listener, err := net.Listen("tcp", options.Address)
	if err != nil {
		return fmt.Errorf("listen on configured address: %w", err)
	}
	if options.ManagementGate != nil {
		listener = &managementListener{Listener: listener, gate: options.ManagementGate}
	}
	handler := options.Handler
	if options.ManagementGate != nil {
		handler = managementRequestGate(options.ManagementGate, handler)
	}
	server := &http.Server{
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		ConnContext: func(ctx context.Context, connection net.Conn) context.Context {
			return context.WithValue(ctx, localAddressContextKey{}, connection.LocalAddr())
		},
	}
	if options.OnListening != nil {
		options.OnListening(listener.Addr())
	}

	serveResult := make(chan error, 1)
	go func() {
		if tlsConfig != nil {
			serveResult <- server.ServeTLS(listener, "", "")
		} else {
			serveResult <- server.Serve(listener)
		}
	}()

	select {
	case err := <-serveResult:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP API: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), options.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shut down HTTP API: %w", err)
		}
		serveErr := <-serveResult
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP API during shutdown: %w", serveErr)
		}
		return nil
	}
}

func validateServeTLS(options ServeOptions) error {
	helperIdentities := 0
	if options.TLSCertificate != nil {
		helperIdentities++
	}
	if options.TLSCertificateProvider != nil {
		helperIdentities++
	}
	if helperIdentities > 1 || helperIdentities == 1 && (options.Certificate != "" || options.Key != "") {
		return errors.New("helper-backed and file-backed TLS identities are mutually exclusive")
	}
	return nil
}

type managementListener struct {
	net.Listener
	gate ManagementConnectionGate
}

// Accept rejects a disallowed destination before TLS parsing and before any
// application handler can observe the connection.
func (listener *managementListener) Accept() (net.Conn, error) {
	for {
		connection, err := listener.Listener.Accept()
		if err != nil {
			return nil, err
		}
		address := addressOf(connection.LocalAddr())
		if address.IsValid() && listener.gate.Allows(address) {
			return connection, nil
		}
		_ = connection.Close()
	}
}

// managementRequestGate rechecks keep-alive requests after an allowlist
// change. It remains outside the application handler and therefore rejects a
// disabled path before authentication, routing, CSRF, or business logic.
func managementRequestGate(gate ManagementConnectionGate, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !gate.Allows(LocalDestination(request.Context())) {
			response.Header().Set("Connection", "close")
			http.Error(response, "management interface not enabled", http.StatusForbidden)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func addressOf(address net.Addr) netip.Addr {
	if tcp, ok := address.(*net.TCPAddr); ok {
		parsed, _ := netip.AddrFromSlice(tcp.IP)
		return parsed.Unmap()
	}
	if address == nil {
		return netip.Addr{}
	}
	host, _, err := net.SplitHostPort(address.String())
	if err != nil {
		return netip.Addr{}
	}
	parsed, _ := netip.ParseAddr(host)
	return parsed.Unmap()
}

// LocalDestination returns the server-side address selected by the client.
func LocalDestination(ctx context.Context) netip.Addr {
	address, _ := ctx.Value(localAddressContextKey{}).(net.Addr)
	return addressOf(address)
}

// HandlerOptions contains the dependencies for the protected HTTP API.
type HandlerOptions struct {
	Bench              *bench.Manager
	Identity           *identity.Service
	Profile            string
	Build              buildinfo.Info
	SystemInformation  func() systeminfo.Info
	Catalog            simulation.Catalog
	Network            *network.Service
	Operations         *network.OperationManager
	ManagementAccess   *network.ManagementAccessService
	Power              power.Controller
	Appliance          *ApplianceMetadata
	Identify           network.IdentifyActionExecutor
	IdentifyOperations identifyOperationStore
	Naming             applianceNamingService
	// MCP is the read-only Model Context Protocol endpoint, mounted at /mcp
	// when one is configured. It is optional: a deployment that offers no MCP
	// access simply has no route there.
	MCP            http.Handler
	CookieSecurity CookieSecurity
	// Activity serves the shared history and its retention policy. It is
	// optional: a handler without it has no such routes.
	Activity *activity.Service
	// Audit records what the boundary itself decides: an export, a policy
	// change, a refused request. Optional.
	Audit AuditJournal
}

// AuditJournal records what the HTTP boundary decides.
type AuditJournal interface {
	Record(ctx context.Context, record audit.Record) error
}

// NewHandler builds the protected Bench Core HTTP API. Every route below
// declares the permission it requires, so authorization is decided in one
// place rather than inside each handler.
func NewHandler(options HandlerOptions) (http.Handler, error) {
	if options.Identity == nil {
		return nil, errors.New("identity service is required")
	}
	if options.Profile != simulationProfile && options.Profile != "appliance" {
		return nil, errors.New("HTTP API requires the simulation or appliance profile")
	}
	if options.Profile == simulationProfile && options.Catalog.Scenario() == "" {
		return nil, errors.New("simulation catalog is required")
	}
	if options.Network == nil {
		return nil, errors.New("network service is required")
	}
	if options.Operations == nil {
		return nil, errors.New("network operation manager is required")
	}
	if _, err := ParseCookieSecurity(string(options.CookieSecurity)); err != nil {
		return nil, err
	}
	if _, err := validateApplianceMetadata(options.Appliance); err != nil {
		return nil, err
	}

	if options.Profile == "appliance" {
		options.Catalog = simulation.Catalog{}
	}
	guard := accessGuard{audit: options.Audit}
	authorize := guard.authorize
	protected := http.NewServeMux()
	registerBench(protected, options.Bench, guard)
	protected.Handle("GET /api/v1/status", authorize(auth.PermissionStatusRead, func(response http.ResponseWriter, _ *http.Request) {
		var system *systeminfo.Info
		if options.SystemInformation != nil {
			value := options.SystemInformation()
			system = &value
		}
		writeJSON(response, http.StatusOK, statusResponse{
			APIVersion: apiVersion,
			Status:     "healthy",
			Profile:    options.Profile,
			Simulation: options.Profile == simulationProfile,
			Build:      options.Build,
			System:     system,
		})
	}))
	protected.Handle("GET /api/v1/integrations", authorize(auth.PermissionInventoryRead, func(response http.ResponseWriter, request *http.Request) {
		items := options.Catalog.Integrations()
		actor, _ := actorFrom(request.Context())
		if options.Bench != nil && actor.Has(auth.PermissionBenchRead) {
			plugins, err := options.Bench.ListPlugins(request.Context(), actor)
			if err != nil {
				benchError(response, err)
				return
			}
			for _, p := range plugins {
				if p.Installed {
					items = append(items, simulation.Integration{ID: p.ID, DisplayName: p.DisplayName, Status: p.Status, Simulation: p.Simulation, Operations: p.Operations})
				}
			}
		}
		writeJSON(response, http.StatusOK, integrationListResponse{
			APIVersion: apiVersion,
			Scenario:   options.Catalog.Scenario(),
			Items:      items,
		})
	}))
	protected.Handle("GET /api/v1/devices", authorize(auth.PermissionInventoryRead, func(response http.ResponseWriter, request *http.Request) {
		items := options.Catalog.Devices()
		actor, _ := actorFrom(request.Context())
		if options.Bench != nil && actor.Has(auth.PermissionBenchRead) {
			plugins, err := options.Bench.ListPlugins(request.Context(), actor)
			if err != nil {
				benchError(response, err)
				return
			}
			for _, p := range plugins {
				if p.Installed {
					for _, d := range p.Devices {
						kind := d.Kind
						if kind == "" {
							kind = "logic-analyzer"
						}
						items = append(items, simulation.Device{ID: d.ID, IntegrationID: p.ID, DisplayName: d.DisplayName, Kind: kind, Status: p.Status, Simulation: p.Simulation, Capabilities: d.Channels, Configured: d.Configured})
					}
				}
			}
		}
		writeJSON(response, http.StatusOK, deviceListResponse{
			APIVersion: apiVersion,
			Scenario:   options.Catalog.Scenario(),
			Items:      items,
		})
	}))
	// Network telemetry. Reading it is permitted to every role; the management
	// routes that will join them here carry network.manage, vpn.manage and
	// vpn.install instead, so this one table keeps answering "what may an actor
	// do" for the whole API.
	protected.Handle("GET /api/v1/network/status", authorize(auth.PermissionNetworkRead, handleNetworkStatus(options.Network)))
	protected.Handle("GET /api/v1/network/statistics", authorize(auth.PermissionNetworkRead, handleNetworkStatistics(options.Network)))
	protected.Handle("GET /api/v1/network/wireguard", authorize(auth.PermissionNetworkRead, handleWireGuardStatus(options.Network)))
	protected.Handle("GET /api/v1/network/wireguard/profiles", authorize(auth.PermissionNetworkRead, handleWireGuardStatus(options.Network)))
	if options.ManagementAccess != nil {
		protected.Handle("GET /api/v1/network/management-access", authorize(auth.PermissionNetworkManage, handleReadManagementAccess(options.ManagementAccess)))
		protected.Handle("PUT /api/v1/network/management-access", authorize(auth.PermissionNetworkManage, handleUpdateManagementAccess(options.ManagementAccess)))
		protected.Handle("POST /api/v1/network/management-access/confirm", authorize(auth.PermissionNetworkManage, handleConfirmManagementAccess(options.ManagementAccess)))
		protected.Handle("POST /api/v1/network/management-access/recover", authorize(auth.PermissionNetworkManage, handleRecoverManagementAccess(options.ManagementAccess)))
		protected.Handle("POST /api/v1/commissioning/complete", authorize(auth.PermissionNetworkManage, handleCompleteCommissioning(options.ManagementAccess)))
	}
	// The preview routes declare their permission here. Acknowledge, confirm and
	// rollback declare the read permission, because the action they authorize
	// against is only known once the operation is resolved; each then checks the
	// action's own permission before touching anything.
	protected.Handle("POST /api/v1/network/interfaces/{id}/preview",
		authorize(auth.PermissionNetworkManage, handlePreviewInterfaceChange(options.Operations)))
	protected.Handle("POST /api/v1/network/interfaces/{id}/settings/preview",
		authorize(auth.PermissionNetworkManage, handlePreviewInterfaceSettings(options.Operations)))
	// A scan drives the radio, so it carries the management permission even
	// though it changes no saved state.
	protected.Handle("GET /api/v1/network/interfaces/{id}/wifi",
		authorize(auth.PermissionNetworkManage, handleWiFiScan(options.Network)))
	protected.Handle("POST /api/v1/network/interfaces/{id}/wifi/preview",
		authorize(auth.PermissionNetworkManage, handlePreviewWiFiJoin(options.Operations)))
	// Device networks: reads for every role, changes and checks under
	// network.manage. A check probes a configured target from the appliance,
	// which is more than reading.
	protected.Handle("GET /api/v1/network/device-networks",
		authorize(auth.PermissionNetworkRead, handleListDeviceNetworks(options.Network)))
	protected.Handle("GET /api/v1/network/device-networks/{id}",
		authorize(auth.PermissionNetworkRead, handleReadDeviceNetwork(options.Network)))
	protected.Handle("POST /api/v1/network/device-networks/preview",
		authorize(auth.PermissionNetworkManage, handlePreviewDeviceNetworkChange(options.Operations)))
	protected.Handle("POST /api/v1/network/device-networks/{id}/checks",
		authorize(auth.PermissionNetworkManage, handleCheckDeviceNetwork(options.Network)))
	protected.Handle("GET /api/v1/network/device-networks/{id}/access-instructions",
		authorize(auth.PermissionNetworkRead, handleAccessInstructions(options.Network)))
	// The shared history and its retention policy, each behind its own fixed
	// permission.
	if options.Activity != nil {
		protected.Handle("GET /api/v1/activity", authorize(auth.PermissionActivityRead, handleActivityQuery(options.Activity)))
		protected.Handle("GET /api/v1/activity/export", authorize(auth.PermissionActivityExport, handleActivityExport(options.Activity, options.Audit)))
		protected.Handle("GET /api/v1/activity/retention", authorize(auth.PermissionActivityRetentionManage, handleReadPolicy(options.Activity)))
		protected.Handle("PUT /api/v1/activity/retention", authorize(auth.PermissionActivityRetentionManage, handleChangePolicy(options.Activity, options.Audit)))
	}
	protected.Handle("POST /api/v1/network/wireguard/preview",
		authorize(auth.PermissionVPNManage, handlePreviewWireGuardChange(options.Operations)))
	protected.Handle("POST /api/v1/network/wireguard/install/preview",
		authorize(auth.PermissionVPNInstall, handlePreviewWireGuardInstall(options.Operations)))
	protected.Handle("GET /api/v1/network/operations",
		authorize(auth.PermissionNetworkRead, handleListOperations(options.Operations)))
	protected.Handle("GET /api/v1/network/operations/{id}",
		authorize(auth.PermissionNetworkRead, handleReadOperation(options.Operations)))
	protected.Handle("POST /api/v1/network/operations/{id}/acknowledge",
		authorize(auth.PermissionNetworkRead, handleAcknowledgeOperation(options.Operations)))
	protected.Handle("POST /api/v1/network/operations/{id}/confirm",
		authorize(auth.PermissionNetworkRead, handleConfirmOperation(options.Operations)))
	protected.Handle("POST /api/v1/network/operations/{id}/rollback",
		authorize(auth.PermissionNetworkRead, handleRollbackOperation(options.Operations)))
	// Identity and token administration. Every route declares its permission
	// here, beside the read routes above, so one table answers "what may an
	// actor do" for the whole API rather than each handler answering for itself.
	protected.Handle("GET /api/v1/users", authorize(auth.PermissionUsersRead, handleListUsers(options.Identity)))
	protected.Handle("POST /api/v1/users", authorize(auth.PermissionUsersManage, handleCreateUser(options.Identity)))
	protected.Handle("PUT /api/v1/users/{username}/role", authorize(auth.PermissionUsersManage, handleSetRole(options.Identity)))
	protected.Handle("PUT /api/v1/users/{username}/enabled", authorize(auth.PermissionUsersManage, handleSetEnabled(options.Identity)))
	protected.Handle("PUT /api/v1/users/{username}/password", authorize(auth.PermissionUsersManage, handleResetPassword(options.Identity)))
	protected.Handle("DELETE /api/v1/users/{username}", authorize(auth.PermissionUsersManage, handleDeleteUser(options.Identity)))
	// The appliance password policy, beside the identities it governs. It is
	// administrator-only and separate from identity administration: the policy
	// decides how weak a credential this appliance accepts.
	protected.Handle("GET /api/v1/password-policy",
		authorize(auth.PermissionPasswordPolicyManage, handleReadPasswordPolicy(options.Identity)))
	protected.Handle("PUT /api/v1/password-policy",
		authorize(auth.PermissionPasswordPolicyManage, handleReplacePasswordPolicy(options.Identity)))
	protected.Handle("GET /api/v1/tokens", authorize(auth.PermissionTokensReadAll, handleListTokens(options.Identity)))
	protected.Handle("POST /api/v1/tokens", authorize(auth.PermissionTokensManageAll, handleCreateToken(options.Identity)))
	protected.Handle("POST /api/v1/tokens/{id}/revoke", authorize(auth.PermissionTokensManageAll, handleRevokeToken(options.Identity)))
	protected.Handle("DELETE /api/v1/tokens/{id}", authorize(auth.PermissionTokensManageAll, handleDeleteToken(options.Identity)))
	protected.Handle("POST /api/v1/system/power", authorize(auth.PermissionSystemPowerManage, handlePower(options.Power, options.Audit)))
	if options.Appliance != nil {
		protected.Handle("GET /api/v1/appliance", authorize(auth.PermissionStatusRead, handleApplianceMetadata(options.Appliance)))
		protected.Handle("GET /api/v1/appliance/label.svg", authorize(auth.PermissionStatusRead, handleApplianceLabel(options.Appliance)))
		protected.Handle("POST /api/v1/appliance/identify", authorize(auth.PermissionNetworkManage, sessionOnly(handleIdentify(options.Identify, options.Audit, options.IdentifyOperations))))
		protected.Handle("PUT /api/v1/appliance/friendly-name", authorize(auth.PermissionNetworkManage, handleFriendlyName(options.Naming, options.Appliance)))
		protected.Handle("DELETE /api/v1/appliance/friendly-name", authorize(auth.PermissionNetworkManage, handleFriendlyName(options.Naming, options.Appliance)))
	}
	// Reading one's own session is identity, not authorization, so it carries
	// no permission; logout is gated by the CSRF check in the middleware.
	protected.HandleFunc("GET /api/v1/auth/session", handleCurrentSession)
	protected.HandleFunc("POST /api/v1/auth/logout", handleLogout(options.Identity, options.CookieSecurity))

	root := http.NewServeMux()
	root.HandleFunc("POST /api/v1/auth/login", handleLogin(options.Identity, options.CookieSecurity))
	if options.Appliance != nil {
		root.Handle("GET /.well-known/nilo-device.xml", handleCurrentDiscoveryDescription(options.Appliance))
	}
	// The MCP endpoint authenticates its own bearer tokens and never accepts a
	// browser session, so it is mounted beside the protected tree rather than
	// inside it: a cookie must not be able to reach it.
	if options.MCP != nil {
		root.Handle("/mcp", options.MCP)
	}
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
	APIVersion string           `json:"api_version"`
	Status     string           `json:"status"`
	Profile    string           `json:"profile"`
	Simulation bool             `json:"simulation"`
	Build      buildinfo.Info   `json:"build"`
	System     *systeminfo.Info `json:"system,omitempty"`
}
