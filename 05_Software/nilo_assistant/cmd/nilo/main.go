package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/activity"
	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/buildinfo"
	"nilo.local/nilo/internal/httpapi"
	"nilo.local/nilo/internal/mcp"
	"nilo.local/nilo/internal/nethelper"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/power"
	"nilo.local/nilo/internal/simulation"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:]))
}

func run(ctx context.Context, arguments []string) int {
	if len(arguments) == 1 && arguments[0] == "version" {
		if err := json.NewEncoder(os.Stdout).Encode(buildinfo.Current()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if len(arguments) == 3 && arguments[0] == "serve-foundation-check" && arguments[1] == "--data-dir" {
		application, err := app.New(context.Background(), app.Options{
			DataDir: arguments[2], BindAddress: "127.0.0.1:0", Now: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		health := struct {
			Status     string `json:"status"`
			DataDir    string `json:"data_dir"`
			Reconciled bool   `json:"reconciled"`
		}{Status: "ok", DataDir: application.Configuration().DataDir, Reconciled: true}
		if err := json.NewEncoder(os.Stdout).Encode(health); err != nil {
			_ = application.Close()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := application.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if len(arguments) == 4 && arguments[0] == "development" && arguments[1] == "issue-token" && arguments[2] == "--data-dir" {
		// Issuing a token is not a server start, and this command is normally run
		// against a data directory a development server is already serving.
		application, err := app.Open(ctx, app.Options{
			DataDir: arguments[3], BindAddress: "127.0.0.1:0", Now: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		issued, err := application.IssueDevelopmentToken(ctx)
		if err != nil {
			_ = application.Close()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		output := struct {
			TokenID   string    `json:"token_id"`
			UserID    string    `json:"user_id"`
			Token     string    `json:"token"`
			CreatedAt time.Time `json:"created_at"`
		}{TokenID: issued.Token.ID, UserID: issued.UserID, Token: issued.Token.Plaintext, CreatedAt: issued.Token.CreatedAt}
		if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
			_ = application.Close()
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := application.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	if len(arguments) > 0 && arguments[0] == "admin" {
		return runAdminCommand(ctx, arguments[1:], adminIO{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, interactive: networkTerminal()})
	}
	if len(arguments) > 0 && arguments[0] == "network" {
		return runNetworkCommand(ctx, arguments[1:], networkIO{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin, interactive: networkTerminal()})
	}
	if len(arguments) > 0 && (arguments[0] == "plugin" || arguments[0] == "bench") {
		return runBenchCommand(ctx, arguments[0], arguments[1:], networkIO{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin})
	}
	if len(arguments) > 0 && arguments[0] == "activity" {
		return runActivityCommand(ctx, arguments[1:], networkIO{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin, interactive: networkTerminal()})
	}
	if len(arguments) > 0 && arguments[0] == "serve" {
		return runServe(ctx, arguments[1:])
	}

	writeUsage()
	return 2
}

func runServe(ctx context.Context, arguments []string) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profile := flags.String("profile", "", "runtime profile")
	scenarioValue := flags.String("scenario", string(simulation.ScenarioHealthy), "deterministic simulation scenario")
	dataDir := flags.String("data-dir", "", "absolute runtime data directory")
	bindAddress := flags.String("bind", "", "explicit listener address")
	var managementInterfaces repeatedStringFlag
	var managementTLSNames repeatedStringFlag
	flags.Var(&managementInterfaces, "management-interface", "allowed appliance management interface (repeatable)")
	managementPort := flags.Int("management-port", 0, "persistent interface-managed HTTPS port")
	managementReconfigure := flags.Bool("management-reconfigure", false, "stage a recoverable change to an existing management allowlist")
	flags.Var(&managementTLSNames, "management-tls-name", "required certificate DNS or IP SAN (repeatable)")
	bundleDir := flags.String("integration-bundles", "/opt/nilo/integration-bundles", "curated integration bundle directory")
	backend := flags.String("sigrok-cli", "/usr/bin/sigrok-cli", "trusted sigrok CLI executable")
	helperSocket := flags.String("nethelper-socket", "/run/nilo/nethelper.sock", "installed Linux helper socket")
	guiDir := flags.String("gui-dir", "", "built GUI directory served on the API origin")
	tlsCert := flags.String("tls-cert", "", "listener PEM certificate")
	tlsKey := flags.String("tls-key", "", "listener PEM private key")
	developmentAdministrator := flags.Bool(
		"development-administrator",
		false,
		"create a first administrator with a fixed, published development password (simulation profile only)",
	)
	cookieMode := flags.String(
		"session-cookies",
		string(httpapi.CookieDevelopmentLocalhost),
		"browser session cookie mode: development-localhost or https",
	)
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 || (*profile != "simulation" && *profile != "appliance") || *dataDir == "" {
		fmt.Fprintln(os.Stderr, "serve requires --profile simulation|appliance and --data-dir")
		return 2
	}
	managementMode := len(managementInterfaces) > 0 || *managementPort != 0 || len(managementTLSNames) > 0
	if *bindAddress != "" && managementMode {
		fmt.Fprintln(os.Stderr, "--bind cannot be combined with interface-managed listener options")
		return 2
	}
	if *managementReconfigure && len(managementInterfaces) == 0 {
		fmt.Fprintln(os.Stderr, "--management-reconfigure requires at least one --management-interface")
		return 2
	}
	if len(managementInterfaces) == 0 && len(managementTLSNames) > 0 {
		fmt.Fprintln(os.Stderr, "--management-tls-name requires --management-interface")
		return 2
	}
	if *profile == "simulation" && managementMode {
		fmt.Fprintln(os.Stderr, "simulation requires a literal loopback --bind and does not accept management interfaces")
		return 2
	}
	if *bindAddress == "" && !managementMode {
		fmt.Fprintln(os.Stderr, "serve requires legacy --bind or --management-port with persistent management interfaces")
		return 2
	}
	if len(managementInterfaces) > 0 && *managementPort == 0 {
		*managementPort = 8443
	}
	if managementMode && (*managementPort < 1 || *managementPort > 65535) {
		fmt.Fprintln(os.Stderr, "management port must be between 1 and 65535")
		return 2
	}
	if len(managementInterfaces) > 0 && len(managementTLSNames) == 0 {
		fmt.Fprintln(os.Stderr, "initial interface-managed access requires at least one --management-tls-name")
		return 2
	}
	if len(managementInterfaces) > 0 {
		configuration := network.ManagementAccessConfiguration{
			Interfaces: append([]string(nil), managementInterfaces...), Port: *managementPort,
			TLSNames: append([]string(nil), managementTLSNames...),
		}
		if err := configuration.Validate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	listenerAddress := *bindAddress
	if managementMode {
		listenerAddress = "0.0.0.0:" + strconv.Itoa(*managementPort)
	}
	cookieSecurity, err := httpapi.ParseCookieSecurity(*cookieMode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	var catalog simulation.Catalog
	if *profile == "simulation" {
		if err := httpapi.ValidateSimulationBind(listenerAddress); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		scenario, err := simulation.ParseScenario(*scenarioValue)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		catalog, err = simulation.NewCatalog(scenario)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else {
		if *developmentAdministrator {
			fmt.Fprintln(os.Stderr, "development administrator is allowed only with the simulation profile")
			return 2
		}
		if *tlsCert == "" || *tlsKey == "" || *guiDir == "" {
			fmt.Fprintln(os.Stderr, "appliance requires --tls-cert, --tls-key and --gui-dir")
			return 2
		}
		cookieSecurity = httpapi.CookieHTTPSDeployment
	}
	if !managementMode {
		if err := httpapi.ValidateListener(listenerAddress, *tlsCert, *tlsKey); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	provider, err := newNetworkProvider(ctx, *profile, *helperSocket)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var powerController power.Controller = power.Unavailable{}
	if *profile == "appliance" {
		powerController = nethelper.NewClient(*helperSocket, "")
	}

	application, err := app.New(ctx, app.Options{
		DataDir: *dataDir, BindAddress: listenerAddress, Now: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := application.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()

	if *developmentAdministrator {
		created, err := ensureDevelopmentAdministrator(ctx, application, *profile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprintln(os.Stderr, developmentAdministratorNotice(created))
	}

	// The shared Activity history and its retention policy.
	activityService, err := activity.NewService(activity.Options{
		Store: application.ActivityRepository(), ApplianceID: application.ApplianceRuntime().Identity.ID,
		Now: func() time.Time { return time.Now().UTC() }, Monitor: application.AuditMonitor(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// The runtime's own re-plans of device networks are recorded too.
	if audited, ok := provider.(interface{ UseAudit(network.AuditJournal) }); ok {
		audited.UseAudit(application.AuditWriter())
	}
	// Saved profiles are configuration, so they outlive the process that
	// imported them. Activation is not restored: a tunnel is reported connected
	// only when it is observed connected.
	if inventory, ok := provider.(interface {
		UseProfileStore(context.Context, network.ProfileStore) error
	}); ok {
		if err := inventory.UseProfileStore(ctx, application.VPNProfileRepository()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	// Saved adapter configuration binds to the adapter's hardware identity and
	// is reported beside what the adapter actually carries.
	if configured, ok := provider.(interface {
		UseInterfaceSettingsStore(context.Context, network.InterfaceSettingsStore) error
	}); ok {
		if err := configured.UseInterfaceSettingsStore(ctx, application.InterfaceSettingsRepository()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	// Saved device networks are attached last, because their validation and
	// reconciliation read the profiles and adapter settings attached above.
	if devices, ok := provider.(interface {
		UseDeviceNetworkStore(context.Context, network.DeviceNetworkStore) error
	}); ok {
		if err := devices.UseDeviceNetworkStore(ctx, application.DeviceNetworkRepository()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	networkService, err := network.NewService(network.ServiceOptions{
		Provider: provider,
		History:  application.NetworkRepository(),
		Now:      func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var managementAccess *network.ManagementAccessService
	if managementMode {
		repository := application.ManagementAccessRepository()
		var initial *network.ManagementAccessConfiguration
		if len(managementInterfaces) > 0 {
			configured := network.ManagementAccessConfiguration{
				Interfaces: append([]string(nil), managementInterfaces...), Port: *managementPort,
				TLSNames: append([]string(nil), managementTLSNames...),
			}
			initial = &configured
		} else {
			record, present, loadErr := repository.LoadManagementAccess(ctx)
			if loadErr != nil || !present {
				if loadErr != nil {
					fmt.Fprintln(os.Stderr, loadErr)
				} else {
					fmt.Fprintln(os.Stderr, "no persistent management access configuration; specify --management-interface at the appliance console")
				}
				return 1
			}
			if record.Configuration.Port != *managementPort {
				fmt.Fprintf(os.Stderr, "--management-port %d does not match the persisted port %d\n", *managementPort, record.Configuration.Port)
				return 2
			}
			managementTLSNames = append(managementTLSNames[:0], record.Configuration.TLSNames...)
		}
		missing, certificateErr := httpapi.ValidateCertificateNames(*tlsCert, managementTLSNames)
		if certificateErr != nil {
			fmt.Fprintln(os.Stderr, certificateErr)
			return 2
		}
		for _, name := range missing {
			fmt.Fprintf(os.Stderr, "WARNING: listener certificate is missing SAN %q; renew the configured certificate before using that URL\n", name)
		}
		managementAccess, err = network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
			Provider: provider, Store: repository, Firewall: nethelper.NewClient(*helperSocket, ""),
			Now: func() time.Time { return time.Now().UTC() }, Initial: initial,
			Reconfigure: *managementReconfigure, TLSMissing: missing,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if configured, ok := provider.(interface {
			UseManagementAccessPolicy(interface{ ConfiguredInterfaces() []string })
		}); ok {
			configured.UseManagementAccessPolicy(managementAccess)
		}
	}
	operations, err := network.NewOperationManager(network.OperationManagerOptions{
		Changer:  provider,
		Observer: provider,
		Now:      func() time.Time { return time.Now().UTC() },
		NewID:    func() string { return uuid.NewString() },
		Store:    application.NetworkOperationRepository(),
		Audit:    application.AuditWriter(),
		// An imported key goes straight to the privileged helper where one is
		// present. The simulation offers no stager, so it parses an import and
		// keeps no key at all: it manages no real tunnel, and a real key on a
		// developer's machine would be a liability with no purpose.
		Stager:   secretStagerFor(provider),
		Recovery: recoveryObserverFor(provider),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// A change that was in flight when the previous run ended is adopted before
	// anything is served, so its window is honoured by this run rather than
	// forgotten with the process that opened it, and then settled against the
	// helper's own record of what became of it.
	if err := operations.Adopt(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := operations.Reconcile(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// The startup selection is connected once the runtime is reconciled and
	// before anything is served, attributed to the system actor; a missing
	// uplink is retried from the maintenance loop rather than failing startup.
	var startup *network.Startup
	if connector, ok := provider.(network.StartupConnector); ok {
		startup = network.NewStartup(network.StartupOptions{Connector: connector, Audit: application.AuditWriter()})
		attempt, cancelAttempt := context.WithTimeout(ctx, 20*time.Second)
		startup.Attempt(attempt)
		cancelAttempt()
	}
	started := time.Now()
	sampler, err := network.NewSampler(network.SamplerOptions{Counters: provider, Store: application.NetworkRepository(), MeasuredBy: provider.Source(), Now: time.Now, Elapsed: func() time.Duration { return time.Since(started) }})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Device-network forwarding is re-planned on the sampling cadence, so a
	// tunnel or adapter that comes back brings its networks with it.
	var reconcileDeviceNetworks func(context.Context) error
	if reconciler, ok := provider.(interface{ ReconcileDeviceNetworks(context.Context) error }); ok {
		reconcileDeviceNetworks = reconciler.ReconcileDeviceNetworks
	}
	if managementAccess != nil {
		deviceReconcile := reconcileDeviceNetworks
		reconcileDeviceNetworks = func(ctx context.Context) error {
			var deviceErr error
			if deviceReconcile != nil {
				deviceErr = deviceReconcile(ctx)
			}
			return errors.Join(deviceErr, managementAccess.Reconcile(ctx))
		}
	}
	housekeeper, err := activity.NewHousekeeper(activity.HousekeeperOptions{
		Service: activityService, Audit: application.AuditWriter(), Pending: operations.PendingIDs,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	maintenanceCtx, stopMaintenance := context.WithCancel(ctx)
	maintenanceDone := make(chan struct{})
	go func() {
		defer close(maintenanceDone)
		network.RunMaintenanceWith(maintenanceCtx, network.MaintenanceOptions{
			Sampler: sampler, Operations: operations, Startup: startup, Reconcile: reconcileDeviceNetworks,
			Housekeeping: func(ctx context.Context) error {
				_, err := housekeeper.Sweep(ctx)
				return err
			},
			Report: func(err error) {
				if err != nil && maintenanceCtx.Err() == nil {
					fmt.Fprintln(os.Stderr, "network maintenance:", err)
				}
			},
		})
	}()
	defer func() { stopMaintenance(); <-maintenanceDone }()
	benchService, err := application.StartBench(*bundleDir, *backend, *profile == "simulation")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer benchService.Close()
	mcpHandler, err := mcp.NewHandler(mcp.HandlerOptions{
		Identity: application.IdentityService(), Network: networkService, Bench: benchService,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	handler, err := httpapi.NewHandler(httpapi.HandlerOptions{
		Identity: application.IdentityService(), Profile: *profile, Build: buildinfo.Current(),
		Catalog: catalog, Network: networkService, Operations: operations, Bench: benchService,
		MCP: mcpHandler, CookieSecurity: cookieSecurity,
		Activity: activityService, Audit: application.AuditWriter(), Power: powerController,
		ManagementAccess: managementAccess,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *guiDir != "" {
		handler, err = httpapi.StaticAssets(*guiDir, handler, *profile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	var managementGate httpapi.ManagementConnectionGate
	if managementAccess != nil {
		managementGate = managementAccess
	}
	err = httpapi.Serve(ctx, httpapi.ServeOptions{
		Certificate: *tlsCert, Key: *tlsKey,
		Address:         listenerAddress,
		Handler:         handler,
		ManagementGate:  managementGate,
		ShutdownTimeout: 5 * time.Second,
		OnListening: func(address net.Addr) {
			interfaces := []string{}
			addresses := []string{}
			if managementAccess != nil {
				status, statusErr := managementAccess.Status(ctx, netip.Addr{})
				if statusErr == nil {
					for _, item := range status.Interfaces {
						if item.Enabled {
							interfaces = append(interfaces, item.Name)
							if item.Usable {
								addresses = append(addresses, item.IPv4...)
								addresses = append(addresses, item.IPv6...)
							}
						}
					}
				}
			}
			_ = json.NewEncoder(os.Stdout).Encode(struct {
				Status     string   `json:"status"`
				Address    string   `json:"address"`
				Profile    string   `json:"profile"`
				Interfaces []string `json:"management_interfaces,omitempty"`
				Addresses  []string `json:"management_addresses,omitempty"`
			}{Status: "listening", Address: address.String(), Profile: *profile, Interfaces: interfaces, Addresses: addresses})
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// rootUsage names every command group, so an operator at the console can find
// them all from one place; each group prints its own usage when run alone.
const rootUsage = `usage: nilo <command> [options]

  nilo version                                 build information as JSON
  nilo serve --profile simulation --data-dir D --bind LOOPBACK
  nilo serve --profile appliance --data-dir D --bind PRIVATE_ADDR
  nilo serve --profile appliance --data-dir D --management-port PORT
             [--management-interface NAME ... --management-tls-name DNS_OR_IP ...]
             [--management-reconfigure]
             [--scenario healthy|unavailable] [--session-cookies development-localhost|https]
             [--gui-dir DIR --tls-cert PEM --tls-key PEM --nethelper-socket PATH]
  nilo serve-foundation-check --data-dir D     open and reconcile the data directory once
  nilo development issue-token --data-dir D    issue a development bearer token
  nilo admin ...                               identities and API tokens: locally with --data-dir,
                                               or remotely with --server URL --token-file PATH
  nilo network ...                             appliance network state and previewed changes
  nilo plugin ...                              manage installed integrations and discover devices
  nilo bench ...                               queue instrument jobs and retrieve replies/artifacts
  nilo activity ...                            audit and event history: list and export

Run a command group without arguments for its own usage. The remote groups
(admin, network, activity, plugin, bench) take --server URL and --token-file PATH;
the token is read from that file and never from an argument.`

func writeUsage() {
	fmt.Fprintln(os.Stderr, rootUsage)
}

type repeatedStringFlag []string

func (values *repeatedStringFlag) String() string { return strings.Join(*values, ",") }
func (values *repeatedStringFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("option value cannot be empty")
	}
	*values = append(*values, value)
	return nil
}
