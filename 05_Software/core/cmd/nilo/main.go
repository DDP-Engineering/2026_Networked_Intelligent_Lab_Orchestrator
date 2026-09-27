package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
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
	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/buildinfo"
	"nilo.local/nilo/internal/discovery"
	"nilo.local/nilo/internal/httpapi"
	"nilo.local/nilo/internal/mcp"
	"nilo.local/nilo/internal/nethelper"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/power"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/systeminfo"
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
	if len(arguments) > 0 && arguments[0] == "discover" {
		return runDiscoverCommand(ctx, arguments[1:], commandIO{stdout: os.Stdout, stderr: os.Stderr}, discovery.Discover)
	}
	if len(arguments) > 0 && arguments[0] == "open" {
		return runOpenCommand(ctx, arguments[1:], commandIO{stdout: os.Stdout, stderr: os.Stderr}, discovery.Discover, defaultBrowserLauncher)
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
	if len(arguments) > 0 && arguments[0] == "seed-admin" {
		return runSeedAdmin(ctx, arguments[1:], adminIO{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr})
	}
	if len(arguments) > 0 && arguments[0] == "maintenance" {
		return runMaintenanceCommand(ctx, arguments[1:], adminIO{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr})
	}
	if len(arguments) > 0 && arguments[0] == "network" {
		return runNetworkCommand(ctx, arguments[1:], networkIO{stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin, interactive: networkTerminal()})
	}
	if len(arguments) > 0 && (arguments[0] == "plugin" || arguments[0] == "bench" || arguments[0] == "camera" || arguments[0] == "serial") {
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
	localStatusBind := flags.String("local-status-bind", "", "optional literal loopback listener for appliance-local status")
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
	if *localStatusBind != "" {
		if *profile != "appliance" {
			fmt.Fprintln(os.Stderr, "--local-status-bind is available only for the appliance profile")
			return 2
		}
		if err := httpapi.ValidateLocalStatusBind(*localStatusBind); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
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
	mode, listenerAddress, err := resolveListenerMode(*profile, *bindAddress, managementMode, *managementPort)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
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
		if *guiDir == "" || (*tlsCert == "") != (*tlsKey == "") {
			fmt.Fprintln(os.Stderr, "appliance requires --gui-dir and either both or neither of --tls-cert and --tls-key")
			return 2
		}
		cookieSecurity = httpapi.CookieHTTPSDeployment
	}
	if mode == listenerBind && (*profile != "appliance" || *tlsCert != "") {
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
	var applianceHelper *nethelper.Client
	if *profile == "appliance" {
		applianceHelper = nethelper.NewClient(*helperSocket, "")
		powerController = applianceHelper
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

	// The command line cannot know whether first boot is legitimate; the
	// appliance's own recorded state can, and it is readable only now.
	if err := checkProvisioningPermitted(mode, application.Commissioning()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

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
	var mdnsAnnouncer *network.MDNSAnnouncer
	var provisioningAnnouncer *network.MDNSAnnouncer
	var commissionedAnnouncer *network.MDNSAnnouncer
	var ssdpAnnouncer *network.SSDPAnnouncer
	var mdnsInterfaces func(context.Context) ([]string, error)
	systemHostname := ""
	var helperIdentity nethelper.ApplianceIdentity
	var runtimeTLSCertificateProvider func(*tls.ClientHelloInfo) (*tls.Certificate, error)
	var applianceMetadata *httpapi.ApplianceMetadata
	hardwareRecoveryEnabled := false
	if *profile == "appliance" {
		helperIdentity, err = applianceHelper.ApplianceIdentity(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read permanent appliance identity:", err)
			return 1
		}
		systemHostname = helperIdentity.SerialName
		tlsNames := []string{systemHostname, "nilo"}
		if friendly := application.ApplianceRuntime().Identity.FriendlyName; friendly != "" {
			tlsNames = append(tlsNames, friendly)
		}
		if err = applianceHelper.ActivateNames(ctx, tlsNames); err != nil {
			fmt.Fprintln(os.Stderr, "activate appliance certificate names:", err)
			return 1
		}
		if *tlsCert == "" {
			_, certificateErr := nethelper.NewRemoteTLSCertificate(ctx, applianceHelper)
			if certificateErr != nil {
				fmt.Fprintln(os.Stderr, "open helper-backed appliance certificate:", certificateErr)
				return 1
			}
			runtimeTLSCertificateProvider = func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				certificate, err := nethelper.NewRemoteTLSCertificate(context.Background(), applianceHelper)
				return &certificate, err
			}
		}
		_, _, hardwareErr := applianceHelper.ReadRecoveryEvent(ctx)
		switch {
		case hardwareErr == nil:
			hardwareRecoveryEnabled = true
		case errors.Is(hardwareErr, nethelper.ErrDirectRecoveryUnsupported):
		default:
			fmt.Fprintln(os.Stderr, "probe Raspberry Pi recovery hardware:", hardwareErr)
			return 1
		}
	}
	var roleCoordinator *network.AdapterRoleCoordinator
	if roles, ok := provider.(interface {
		AdapterRoleCoordinator() *network.AdapterRoleCoordinator
	}); ok {
		roleCoordinator = roles.AdapterRoleCoordinator()
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
	// Rehydrate and settle in-flight device-network state before management
	// composition. This restores rollback snapshots and reserves the complete
	// old/new adapter union before any listener, firewall or forwarding plan is
	// derived from durable state.
	if err := operations.Adopt(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := operations.Reconcile(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var commissioningLinks *network.CommissioningLinkService
	var commissioningAP *network.CommissioningAccessPointService
	if *profile == "appliance" {
		commissioningLinks, err = network.NewCommissioningLinkService(
			provider, nethelper.NewClient(*helperSocket, ""), roleCoordinator,
		)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := commissioningLinks.Reconcile(ctx, mode == listenerProvisioning); err != nil {
			fmt.Fprintln(os.Stderr, "prepare commissioning Ethernet links:", err)
			return 1
		}
		accessPointPort := *managementPort
		if accessPointPort == 0 {
			accessPointPort = network.ProvisioningPort
		}
		commissioningAP, err = network.NewCommissioningAccessPointService(network.CommissioningAccessPointOptions{
			Provider: provider, Driver: applianceHelper, Roles: roleCoordinator, SerialName: systemHostname, Port: uint16(accessPointPort),
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := commissioningAP.Reconcile(ctx, application.Commissioning() == "provisioning"); err != nil {
			fmt.Fprintln(os.Stderr, "commissioning Wi-Fi access point unavailable:", err)
		}
	}
	if mode == listenerProvisioning {
		provisioningTLSNames := requiredDiscoveryTLSNames(systemHostname, true)
		provisioningCommissioner, provisioningCredential := commissioningDependencies(application)
		// A missing first-contact name is a misprepared image, not a reason to
		// leave the appliance unreachable: first boot serves on what it has and
		// carries the gap into the interface, where an administrator can see it.
		var missing []string
		if *tlsCert != "" {
			var certificateErr error
			missing, certificateErr = missingDiscoveryNames(*tlsCert, systemHostname, true)
			if certificateErr != nil {
				fmt.Fprintln(os.Stderr, certificateErr)
				return 1
			}
		}
		for _, name := range missing {
			fmt.Fprintf(os.Stderr,
				"WARNING: the provisioning certificate is missing required discovery SAN %q; "+
					"first contact at that name reports a name mismatch until it is corrected\n", name)
		}
		managementAccess, err = network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
			Provider: provider, Store: application.ManagementAccessRepository(),
			Firewall:   nethelper.NewClient(*helperSocket, ""),
			Now:        func() time.Time { return time.Now().UTC() },
			TLSMissing: missing, Coordinator: roleCoordinator,
			Provisioning: true, ProvisioningTLSNames: provisioningTLSNames,
			Commissioner: provisioningCommissioner, PublishedCredential: provisioningCredential,
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
		mdnsInterfaces = func(context.Context) ([]string, error) {
			return managementAccess.ConfiguredInterfaces(), nil
		}
	}
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
		var missing []string
		if *tlsCert != "" {
			var certificateErr error
			missing, certificateErr = httpapi.ValidateCertificateNames(*tlsCert, managementTLSNames)
			if certificateErr != nil {
				fmt.Fprintln(os.Stderr, certificateErr)
				return 2
			}
			discoveryMissing, discoveryErr := missingDiscoveryNames(*tlsCert, systemHostname, hardwareRecoveryEnabled)
			if discoveryErr != nil {
				fmt.Fprintln(os.Stderr, discoveryErr)
				return 2
			}
			missing = mergeMissingNames(missing, discoveryMissing)
		}
		// The same reasoning as first boot: a commissioned appliance that will
		// not start cannot be administered back into shape either. The two lists
		// overlap by design - the appliance's own host name is normally both a
		// configured management name and an advertised discovery name - so they
		// are merged rather than concatenated.
		for _, name := range missing {
			fmt.Fprintf(os.Stderr, "WARNING: listener certificate is missing SAN %q; renew the configured certificate before using that URL\n", name)
		}
		managementCommissioner, managementCredential := commissioningDependencies(application)
		managementAccess, err = network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
			Provider: provider, Store: repository, Firewall: nethelper.NewClient(*helperSocket, ""),
			Now: func() time.Time { return time.Now().UTC() }, Initial: initial,
			Reconfigure: *managementReconfigure, TLSMissing: missing, Coordinator: roleCoordinator,
			ProvisioningTLSNames: requiredDiscoveryTLSNames(systemHostname, true),
			// A hardware recovery reset returns this running service to
			// provisioning, so it needs everything completing commissioning
			// needs, not just the gate.
			Commissioner: managementCommissioner, PublishedCredential: managementCredential,
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
		mdnsInterfaces = func(context.Context) ([]string, error) {
			return managementAccess.ConfiguredInterfaces(), nil
		}
	}
	discoveryPort := *managementPort
	if mode == listenerBind {
		hostText, portText, splitErr := net.SplitHostPort(listenerAddress)
		if splitErr != nil {
			fmt.Fprintln(os.Stderr, splitErr)
			return 2
		}
		discoveryPort, err = strconv.Atoi(portText)
		if err != nil || discoveryPort < 0 || discoveryPort > 65535 {
			fmt.Fprintln(os.Stderr, "invalid discovery listener port")
			return 2
		}
		if address, addressErr := netip.ParseAddr(hostText); discoveryPort == 0 &&
			(addressErr != nil || !address.IsLoopback()) {
			fmt.Fprintln(os.Stderr, "a discoverable private-address listener requires a fixed port")
			return 2
		}
	}
	if discoveryPort == 0 {
		discoveryPort = network.ProvisioningPort
	}
	if *profile == "appliance" {
		version := buildinfo.Current().Version
		storedIdentity := application.ApplianceRuntime().Identity
		provisioningAnnouncer, err = network.NewApplianceMDNSAnnouncer(applianceHelper, network.MDNSApplianceIdentity{
			ApplianceID: storedIdentity.ID, SerialName: systemHostname, FriendlyName: storedIdentity.FriendlyName,
			Port: uint16(discoveryPort), State: "provisioning", Version: version,
		})
		if err == nil {
			commissionedAnnouncer, err = network.NewApplianceMDNSAnnouncer(applianceHelper, network.MDNSApplianceIdentity{
				ApplianceID: storedIdentity.ID, SerialName: systemHostname, FriendlyName: storedIdentity.FriendlyName,
				Port: uint16(discoveryPort), State: "commissioned", Version: version,
			})
		}
		bootID := crc32.ChecksumIEEE([]byte(application.ApplianceRuntime().BootID))
		if bootID == 0 {
			bootID = 1
		}
		if err == nil {
			ssdpAnnouncer, err = network.NewSSDPAnnouncer(applianceHelper, network.SSDPPresence{
				ApplianceID: storedIdentity.ID, Version: version, Port: uint16(discoveryPort), BootID: bootID, ConfigID: 1,
			})
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if managementAccess != nil {
		if provisioningAnnouncer == nil {
			provisioningAnnouncer = network.NewProvisioningMDNSAnnouncer(nethelper.NewClient(*helperSocket, ""))
		}
		if err := reconcileApplianceDiscovery(ctx, managementAccess, &mdnsAnnouncer, provisioningAnnouncer, commissionedAnnouncer); err != nil {
			fmt.Fprintln(os.Stderr, "appliance discovery is not being answered yet:", err)
		}
		if err := ssdpAnnouncer.Announce(ctx, managementAccess.ConfiguredInterfaces()); err != nil {
			fmt.Fprintln(os.Stderr, "appliance SSDP presence is not being answered yet:", err)
		}
	} else if *profile == "appliance" && mode == listenerBind {
		host, _, _ := net.SplitHostPort(listenerAddress)
		address, _ := netip.ParseAddr(host)
		if !address.IsLoopback() {
			// A legacy bound appliance listener is a supported mode, so it
			// reports a certificate gap exactly as the other two do rather than
			// refusing to start: an appliance nobody can reach cannot be
			// corrected either.
			var legacyMissing []string
			if *tlsCert != "" {
				var certificateErr error
				legacyMissing, certificateErr = missingDiscoveryNames(*tlsCert, systemHostname, false)
				if certificateErr != nil {
					fmt.Fprintln(os.Stderr, certificateErr)
					return 2
				}
			}
			for _, name := range legacyMissing {
				fmt.Fprintf(os.Stderr,
					"WARNING: the appliance certificate is missing required discovery SAN %q; "+
						"first contact at that name reports a name mismatch until it is corrected\n", name)
			}
			mdnsInterfaces = func(ctx context.Context) ([]string, error) {
				return boundDiscoveryInterfaces(ctx, provider, listenerAddress)
			}
			interfaces, discoveryErr := mdnsInterfaces(ctx)
			if discoveryErr == nil {
				discoveryErr = commissionedAnnouncer.Announce(ctx, interfaces)
			}
			if discoveryErr == nil {
				discoveryErr = ssdpAnnouncer.Announce(ctx, interfaces)
			}
			if discoveryErr != nil {
				fmt.Fprintln(os.Stderr, "appliance discovery is not being answered yet:", discoveryErr)
			}
		}
	}
	var identifyService *network.IdentifyService
	var identifyAction network.IdentifyActionExecutor
	var identifyIndicator *runtimeIdentifyIndicator
	identifyOperations := application.IdentifyOperationRepository()
	if *profile == "appliance" {
		var indicator network.IdentifyIndicator
		if hardwareRecoveryEnabled {
			identifyIndicator = newRuntimeIndicatorArbiter(applianceHelper)
			indicator = identifyIndicator
		}
		identifyService, err = network.NewIdentifyService(network.IdentifyOptions{Indicator: indicator})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		identifyAction = identifyService
		if applianceHelper != nil {
			identifyAction = applianceHelper
		}
		pendingIdentify, pendingErr := identifyOperations.Pending(ctx)
		if pendingErr != nil {
			fmt.Fprintln(os.Stderr, "list pending Identify actions:", pendingErr)
			return 1
		}
		for _, pending := range pendingIdentify {
			if pending.State == "reserved" {
				result, executeErr := identifyAction.ExecuteIdentifyAction(ctx, network.IdentifyActionRequest{
					OperationID: pending.ID, Source: network.IdentifySource(pending.Source), ActiveUntil: pending.ActiveUntil,
				})
				if executeErr != nil {
					fmt.Fprintln(os.Stderr, "resume pending Identify action:", executeErr)
					return 1
				}
				outcome := audit.OutcomeApplied
				status := string(result.Status)
				if result.Status != network.IdentifyAccepted && result.Status != network.IdentifyUnsupported {
					outcome = audit.OutcomeFailed
					status = "failed"
				}
				if err := identifyOperations.RecordOutcome(ctx, pending.ID, result.Supported,
					result.Reannouncement, status, outcome, time.Now().UTC()); err != nil {
					fmt.Fprintln(os.Stderr, "record resumed Identify result:", err)
					return 1
				}
			}
			if err := identifyAction.AcknowledgeIdentifyAction(ctx, pending.ID); err != nil {
				fmt.Fprintln(os.Stderr, "acknowledge resumed Identify result:", err)
				return 1
			}
			if pending.State != "acted" {
				if err := identifyOperations.MarkActed(ctx, pending.ID, time.Now().UTC()); err != nil {
					fmt.Fprintln(os.Stderr, "finalize resumed Identify audit:", err)
					return 1
				}
			}
			if err := identifyOperations.Flush(ctx, pending.ID); err != nil {
				fmt.Fprintln(os.Stderr, "flush resumed Identify audit:", err)
				return 1
			}
		}
	} else if err := identifyOperations.FlushPending(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "replay pending Identify audit:", err)
		return 1
	}
	var recoveryRuntime *hardwareRecoveryRuntime
	if hardwareRecoveryEnabled && managementAccess != nil && commissionedAnnouncer != nil {
		helper := nethelper.NewClient(*helperSocket, "")
		driver := &runtimeRecoveryDriver{
			helper: helper, management: managementAccess, announcer: commissionedAnnouncer,
			interfaces: mdnsInterfaces,
		}
		recoveryService, recoveryErr := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
			Provider: provider, Driver: driver, Store: application.HardwareRecoveryRepository(),
			Coordinator: roleCoordinator, Now: func() time.Time { return time.Now().UTC() },
			Audit: application.AuditWriter(), SubjectID: application.ApplianceRuntime().Identity.ID,
		})
		if recoveryErr != nil {
			fmt.Fprintln(os.Stderr, recoveryErr)
			return 1
		}
		recoveryRuntime = &hardwareRecoveryRuntime{
			helper: helper, service: recoveryService, driver: driver,
			commission: application.CommissioningRepository(), management: managementAccess, audit: application.AuditWriter(),
			identifyOps: identifyOperations,
			applianceID: application.ApplianceRuntime().Identity.ID, identify: identifyService, identifyAction: identifyAction,
			indicator: identifyIndicator,
			reannounce: func(ctx context.Context) error {
				if mdnsAnnouncer == nil {
					return nil
				}
				interfaces := managementAccess.ConfiguredInterfaces()
				mdnsAnnouncer.Invalidate()
				mdnsErr := mdnsAnnouncer.Announce(ctx, interfaces)
				ssdpAnnouncer.Invalidate()
				ssdpErr := ssdpAnnouncer.Announce(ctx, interfaces)
				return errors.Join(mdnsErr, ssdpErr)
			},
		}
		if err := recoveryRuntime.Adopt(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	// Appliance forwarding is intentionally deferred until management role
	// reconciliation and its firewall have completed.
	if reconciler, ok := provider.(interface{ ReconcileDeviceNetworks(context.Context) error }); ok {
		if err := reconciler.ReconcileDeviceNetworks(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
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
	var reconcileManagementAccess func(context.Context) error
	var refreshApplianceMetadata func(context.Context) error
	if managementAccess != nil {
		// The published local name follows whatever is actually carrying
		// management access, so it is renewed as part of reconciling the
		// allowlist rather than beside it. The announcer republishes only on a
		// change.
		reconcileManagementAccess = func(ctx context.Context) error {
			var accessPointStep func(context.Context) error
			if commissioningAP != nil {
				accessPointStep = func(ctx context.Context) error {
					return commissioningAP.Reconcile(ctx, managementAccess.Provisioning())
				}
			}
			var commissioningLinkStep func(context.Context) error
			if commissioningLinks != nil {
				commissioningLinkStep = func(ctx context.Context) error {
					return commissioningLinks.Reconcile(ctx, managementAccess.Provisioning())
				}
			}
			var discoveryStep func(context.Context) error
			if mdnsInterfaces != nil {
				discoveryStep = func(ctx context.Context) error {
					return reconcileApplianceDiscovery(ctx, managementAccess, &mdnsAnnouncer, provisioningAnnouncer, commissionedAnnouncer)
				}
			}
			var recoveryStep func(context.Context) error
			if recoveryRuntime != nil {
				recoveryStep = recoveryRuntime.Reconcile
			}
			return reconcileApplianceManagementRuntime(
				ctx, accessPointStep, commissioningLinkStep, managementAccess.Reconcile,
				refreshApplianceMetadata, discoveryStep,
				func(ctx context.Context) error {
					return ssdpAnnouncer.Announce(ctx, managementAccess.ConfiguredInterfaces())
				},
				recoveryStep,
			)
		}
	} else if commissionedAnnouncer != nil && mdnsInterfaces != nil {
		reconcileManagementAccess = func(ctx context.Context) error {
			interfaces, err := mdnsInterfaces(ctx)
			if err != nil {
				return err
			}
			if err := commissionedAnnouncer.Announce(ctx, interfaces); err != nil {
				return err
			}
			if refreshApplianceMetadata != nil {
				if err := refreshApplianceMetadata(ctx); err != nil {
					return err
				}
			}
			return ssdpAnnouncer.Announce(ctx, interfaces)
		}
	}
	reconcileRuntime := func(ctx context.Context) error {
		return reconcileNetworkRuntime(
			ctx, operations.Reconcile, reconcileManagementAccess, reconcileDeviceNetworks,
		)
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
			Sampler: sampler, Operations: operations, Startup: startup, Reconcile: reconcileRuntime,
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
	if ssdpAnnouncer != nil {
		defer func() { _ = ssdpAnnouncer.Withdraw(context.Background()) }()
	}
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
	if *profile == "appliance" {
		observed, metadataErr := provider.Interfaces(ctx)
		if metadataErr != nil {
			fmt.Fprintln(os.Stderr, "read appliance discovery addresses:", metadataErr)
			return 1
		}
		interfaces := []string{}
		if managementAccess != nil {
			interfaces = managementAccess.ConfiguredInterfaces()
		} else if mdnsInterfaces != nil {
			interfaces, metadataErr = mdnsInterfaces(ctx)
			if metadataErr != nil {
				fmt.Fprintln(os.Stderr, "read appliance discovery interfaces:", metadataErr)
				return 1
			}
		}
		state := string(application.Commissioning())
		applianceMetadata, _, metadataErr = applianceDiscoveryMetadata(
			helperIdentity, application.ApplianceRuntime().Identity, state,
			buildinfo.Current().Version, discoveryPort, observed, interfaces,
		)
		if metadataErr != nil {
			fmt.Fprintln(os.Stderr, "compose appliance discovery metadata:", metadataErr)
			return 1
		}
		refreshApplianceMetadata = func(ctx context.Context) error {
			observed, err := provider.Interfaces(ctx)
			if err != nil {
				return fmt.Errorf("refresh appliance discovery addresses: %w", err)
			}
			interfaces := []string{}
			if managementAccess != nil {
				interfaces = managementAccess.ConfiguredInterfaces()
			} else if mdnsInterfaces != nil {
				interfaces, err = mdnsInterfaces(ctx)
				if err != nil {
					return err
				}
			}
			next, _, err := applianceDiscoveryMetadata(
				helperIdentity, application.ApplianceRuntime().Identity, string(application.Commissioning()),
				buildinfo.Current().Version, discoveryPort, observed, interfaces,
			)
			if err != nil {
				return err
			}
			applianceMetadata.UpdateRuntime(next.State, next.ManagementAddresses, next.ManagementMACs)
			return nil
		}
	}
	var namingService *appliance.NamingService
	if *profile == "appliance" {
		friendlyInterfaces := func(ctx context.Context) ([]string, error) {
			if managementAccess != nil {
				return managementAccess.ConfiguredInterfaces(), nil
			}
			if mdnsInterfaces != nil {
				return mdnsInterfaces(ctx)
			}
			return nil, errors.New("management discovery interfaces are unavailable")
		}
		publisher := runtimeFriendlyNamePublisher{
			provisioning: provisioningAnnouncer, commissioned: commissionedAnnouncer,
			interfaces: friendlyInterfaces,
			selected: func() *network.MDNSAnnouncer {
				if managementAccess != nil && managementAccess.Provisioning() {
					return provisioningAnnouncer
				}
				return commissionedAnnouncer
			},
		}
		namingService, err = appliance.NewNamingService(
			helperIdentity.SerialName, application.ApplianceRuntime().Identity.FriendlyName,
			application.ApplianceRepository(), applianceHelper, publisher,
		)
		if err != nil {
			fmt.Fprintln(os.Stderr, "compose appliance naming:", err)
			return 1
		}
	}
	systemProbe := systeminfo.New(*dataDir)
	handler, err := httpapi.NewHandler(httpapi.HandlerOptions{
		Identity: application.IdentityService(), Profile: *profile, Build: buildinfo.Current(),
		SystemInformation: systemProbe.Snapshot,
		Catalog:           catalog, Network: networkService, Operations: operations, Bench: benchService,
		MCP: mcpHandler, CookieSecurity: cookieSecurity,
		Activity: activityService, Audit: application.AuditWriter(), Power: powerController,
		ManagementAccess: managementAccess,
		Appliance:        applianceMetadata, Identify: identifyAction,
		IdentifyOperations: identifyOperations, Naming: namingService,
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
	var localStatusDone <-chan error
	var stopLocalStatus context.CancelFunc
	if *localStatusBind != "" {
		localHandler, localErr := httpapi.NewLocalStatusHandler(applianceMetadata)
		if localErr != nil {
			fmt.Fprintln(os.Stderr, "compose local appliance status:", localErr)
			return 1
		}
		result := make(chan error, 1)
		listening := make(chan struct{})
		localCtx, cancelLocal := context.WithCancel(ctx)
		stopLocalStatus = cancelLocal
		go func() {
			result <- httpapi.Serve(localCtx, httpapi.ServeOptions{
				Address: *localStatusBind, Handler: localHandler, ShutdownTimeout: 5 * time.Second,
				OnListening: func(net.Addr) { close(listening) },
			})
		}()
		select {
		case <-listening:
			localStatusDone = result
		case localErr := <-result:
			fmt.Fprintln(os.Stderr, "serve local appliance status:", localErr)
			return 1
		}
	}
	var managementGate httpapi.ManagementConnectionGate
	if managementAccess != nil {
		managementGate = managementAccess
	}
	err = httpapi.Serve(ctx, httpapi.ServeOptions{
		Certificate: *tlsCert, Key: *tlsKey,
		TLSCertificateProvider: runtimeTLSCertificateProvider,
		Address:                listenerAddress,
		Handler:                handler,
		ManagementGate:         managementGate,
		ShutdownTimeout:        5 * time.Second,
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
	if localStatusDone != nil {
		stopLocalStatus()
		if localErr := <-localStatusDone; localErr != nil {
			fmt.Fprintln(os.Stderr, "serve local appliance status:", localErr)
			return 1
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// reconcileNetworkRuntime preserves the startup safety order during periodic
// maintenance. Forwarding must not converge on device intent while operation
// recovery or the management allowlist is still unsafe.
func reconcileNetworkRuntime(
	ctx context.Context,
	reconcileOperations, reconcileManagement, reconcileDevices func(context.Context) error,
) error {
	for _, reconcile := range []func(context.Context) error{
		reconcileOperations,
		reconcileManagement,
		reconcileDevices,
	} {
		if reconcile == nil {
			continue
		}
		if err := reconcile(ctx); err != nil {
			return err
		}
	}
	return nil
}

// reconcileApplianceManagementRuntime restores current discovery publication
// before replaying pending button Identify work. A helper-only restart can
// therefore settle write-ahead reannouncement markers from the real startup
// publication instead of recording a false unavailable result.
func reconcileApplianceManagementRuntime(
	ctx context.Context,
	accessPoint func(context.Context) error,
	commissioningLinks func(context.Context) error,
	management func(context.Context) error,
	metadata func(context.Context) error,
	mdns func(context.Context) error,
	ssdp func(context.Context) error,
	pendingIdentify func(context.Context) error,
) error {
	var accessPointErr error
	if accessPoint != nil {
		accessPointErr = accessPoint(ctx)
	}
	for _, step := range []func(context.Context) error{commissioningLinks, management, metadata, mdns, ssdp} {
		if step != nil {
			if err := step(ctx); err != nil {
				return errors.Join(accessPointErr, err)
			}
		}
	}
	if pendingIdentify == nil {
		return accessPointErr
	}
	return errors.Join(accessPointErr, pendingIdentify(ctx))
}

// rootUsage names every command group, so an operator at the console can find
// them all from one place; each group prints its own usage when run alone.
const rootUsage = `usage: nilo <command> [options]

  nilo version                                 build information as JSON
  nilo discover [--timeout DURATION] [--json]  find local appliances
  nilo discover --label NAME                   write one printable SVG label
  nilo open NAME                               open one uniquely discovered appliance
  nilo serve --profile simulation --data-dir D --bind LOOPBACK
  nilo serve --profile appliance --data-dir D --bind PRIVATE_ADDR
  nilo serve --profile appliance --data-dir D --management-port PORT
             [--management-interface NAME ... --management-tls-name DNS_OR_IP ...]
             [--management-reconfigure]
             [--scenario healthy|unavailable] [--session-cookies development-localhost|https]
             [--gui-dir DIR --tls-cert PEM --tls-key PEM --nethelper-socket PATH]
             [--local-status-bind LOOPBACK]
  nilo serve-foundation-check --data-dir D     open and reconcile the data directory once
  nilo development issue-token --data-dir D    issue a development bearer token
  nilo seed-admin --data-dir D --username U --password-file P
                                               seed the first administrator from a protected file
  nilo maintenance ...                         inventory, snapshot, restore, remove or purge local state
  nilo admin ...                               identities and API tokens: locally with --data-dir,
                                               or remotely with --server URL --token-file PATH
  nilo network ...                             appliance network state and previewed changes
  nilo plugin ...                              manage installed integrations and discover devices
  nilo bench ...                               queue instrument jobs and retrieve replies/artifacts
  nilo serial status|connect|read|send|disconnect|signals|logging|logs|log-read|export|delete|retention [remote options]
  nilo activity ...                            audit and event history: list and export

Run a command group without arguments for its own usage. The remote groups
(admin, network, activity, plugin, bench, camera, serial) take --server URL_OR_NAME and --token-file PATH;
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
