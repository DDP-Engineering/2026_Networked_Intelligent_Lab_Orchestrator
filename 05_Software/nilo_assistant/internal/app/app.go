// Package app composes the durable Increment 1 foundation services.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/activity"
	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/artifact"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/config"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/session"
	"nilo.local/nilo/internal/store/sqlite"
)

type Options struct {
	DataDir     string
	BindAddress string
	Now         func() time.Time
}

type App struct {
	configuration     config.Config
	database          *sqlite.DB
	sessionRepository *sqlite.SessionRepository
	sessionService    session.Service
	artifactStore     artifact.Store
	authRepository    *sqlite.AuthRepository
	identityService   *identity.Service
	applianceRuntime  appliance.Runtime
	auditWriter       *audit.Writer
	auditMonitor      *activity.Monitor
	now               func() time.Time
}

const (
	// browserSessionLifetime bounds how long one password login stays usable.
	browserSessionLifetime = 12 * time.Hour
	// loginFailureLimit and loginFailureWindow brake password guessing.
	loginFailureLimit  = 5
	loginFailureWindow = 15 * time.Minute
)

// DevelopmentToken is the one-time token result and its attributed user.
type DevelopmentToken struct {
	UserID string
	Token  auth.CreatedToken
}

// New starts the server's foundation: it opens the durable services and then
// reconciles the sessions an interrupted server left running. Only a starting
// server may do that, because reconciliation declares running work dead.
func New(ctx context.Context, options Options) (*App, error) {
	application, err := Open(ctx, options)
	if err != nil {
		return nil, err
	}
	if err := application.sessionService.ReconcileStartup(ctx); err != nil {
		_ = application.Close()
		return nil, fmt.Errorf("reconcile interrupted sessions: %w", err)
	}
	return application, nil
}

// Open composes the durable foundation services over a data directory without
// touching the operations recorded in it. Every command that is not the server
// itself opens the appliance this way, because a server may be running beside
// it and still own the sessions and leases stored there.
func Open(ctx context.Context, options Options) (*App, error) {
	if options.Now == nil {
		return nil, errors.New("application clock is required")
	}
	configuration, err := config.Load(config.Inputs{DataDir: options.DataDir, BindAddress: options.BindAddress})
	if err != nil {
		return nil, err
	}
	stagingRoot := filepath.Join(configuration.DataDir, "staging")
	for _, directory := range []string{configuration.DataDir, configuration.ArtifactRoot, configuration.IntegrationDir, stagingRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("create foundation directory %q: %w", directory, err)
		}
	}
	database, err := sqlite.Open(ctx, configuration.DatabasePath)
	if err != nil {
		return nil, err
	}
	sessionRepository := sqlite.NewSessionRepository(database)
	sessionService, err := session.NewService(sessionRepository, func() time.Time { return options.Now().UTC() })
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	artifactStore, err := artifact.NewStore(
		artifact.Config{ArtifactRoot: configuration.ArtifactRoot, StagingRoot: stagingRoot, MaximumBytes: 64 << 20},
		artifact.Dependencies{Repository: sqlite.NewArtifactRepository(database), NewArtifactID: domain.NewArtifactID, Now: func() time.Time { return options.Now().UTC() }},
	)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	// The appliance identity is established here, while the foundation is
	// opening, because it is the identity every recorded event is attributed to
	// and an event whose appliance is unknown cannot be correlated afterwards.
	applianceRuntime, err := appliance.Provision(ctx, sqlite.NewApplianceRepository(database), options.Now)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	// Every append is watched, so the Activity page can say when the
	// appliance can no longer record what it does.
	auditMonitor := activity.NewMonitor(sqlite.NewAuditRepository(database), func() time.Time { return options.Now().UTC() })
	auditWriter, err := audit.NewWriter(audit.WriterOptions{
		Events:      auditMonitor,
		ApplianceID: applianceRuntime.Identity.ID,
		BootID:      applianceRuntime.BootID,
		Now:         func() time.Time { return options.Now().UTC() },
	})
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	identityService, err := identity.NewService(identity.Options{
		Repository:         sqlite.NewIdentityRepository(database),
		Now:                func() time.Time { return options.Now().UTC() },
		Random:             rand.Reader,
		SessionLifetime:    browserSessionLifetime,
		LoginFailureLimit:  loginFailureLimit,
		LoginFailureWindow: loginFailureWindow,
		Audit:              auditWriter,
	})
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return &App{
		configuration: configuration, database: database,
		sessionRepository: sessionRepository, sessionService: sessionService,
		artifactStore: artifactStore, authRepository: sqlite.NewAuthRepository(database),
		identityService: identityService, applianceRuntime: applianceRuntime,
		auditWriter: auditWriter, auditMonitor: auditMonitor, now: options.Now,
	}, nil
}

// ApplianceRuntime is this appliance's stable identity together with the
// identity of this particular boot.
func (application *App) ApplianceRuntime() appliance.Runtime {
	if application == nil {
		return appliance.Runtime{}
	}
	return application.applianceRuntime
}

// AuditWriter records events with a complete envelope. Every service that
// changes something accountable writes through this one writer, so the Activity
// page reads one history rather than several that can disagree.
func (application *App) AuditWriter() *audit.Writer {
	if application == nil {
		return nil
	}
	return application.auditWriter
}

// AuditMonitor reports whether the audit store can still be written.
func (application *App) AuditMonitor() *activity.Monitor {
	if application == nil {
		return nil
	}
	return application.auditMonitor
}

// ActivityRepository returns the activity store: the audit events with the
// retained window, the policy and the retention operations.
func (application *App) ActivityRepository() *sqlite.ActivityRepository {
	if application == nil || application.database == nil {
		return nil
	}
	return sqlite.NewActivityRepository(application.database)
}

// DiagnosticRepository returns the timed debug sessions.
func (application *App) DiagnosticRepository() *sqlite.DiagnosticRepository {
	if application == nil || application.database == nil {
		return nil
	}
	return sqlite.NewDiagnosticRepository(application.database)
}

// AuditRepository returns the append-and-read event store, which the Activity
// page reads and retention sweeps. Writes go through AuditWriter, so that every
// recorded event carries a complete envelope.
func (application *App) AuditRepository() *sqlite.AuditRepository {
	if application == nil || application.database == nil {
		return nil
	}
	return sqlite.NewAuditRepository(application.database)
}

// VPNProfileRepository returns the saved VPN profile inventory. It holds public
// metadata only; key material lives in the privileged helper's keystore and is
// referenced from here by an opaque identity.
func (application *App) VPNProfileRepository() *sqlite.VPNProfileRepository {
	if application == nil || application.database == nil {
		return nil
	}
	return sqlite.NewVPNProfileRepository(application.database)
}

// InterfaceSettingsRepository returns the saved adapter configuration. It holds
// intent and references only; a wireless password never reaches it.
func (application *App) InterfaceSettingsRepository() *sqlite.InterfaceSettingsRepository {
	if application == nil || application.database == nil {
		return nil
	}
	return sqlite.NewInterfaceSettingsRepository(application.database)
}

// DeviceNetworkRepository returns the saved device networks.
func (application *App) DeviceNetworkRepository() *sqlite.DeviceNetworkRepository {
	if application == nil || application.database == nil {
		return nil
	}
	return sqlite.NewDeviceNetworkRepository(application.database)
}

// NetworkOperationRepository returns the durable record of managed network
// changes, which is what lets an interface reattach after a reload and a
// restarted appliance reconcile a change that was in flight.
func (application *App) NetworkOperationRepository() *sqlite.NetworkOperationRepository {
	if application == nil || application.database == nil {
		return nil
	}
	return sqlite.NewNetworkOperationRepository(application.database)
}

func (application *App) Configuration() config.Config { return application.configuration }

// IdentityService returns the authentication and authorization service shared
// by the HTTP boundary and the local administration command line.
func (application *App) IdentityService() *identity.Service { return application.identityService }

// NetworkRepository returns the traffic-history store, which the Network
// service reads and the sampler writes.
func (application *App) NetworkRepository() *sqlite.NetworkRepository {
	if application == nil || application.database == nil {
		return nil
	}
	return sqlite.NewNetworkRepository(application.database)
}

// IssueDevelopmentToken creates a unique non-login identity and bearer token.
func (application *App) IssueDevelopmentToken(ctx context.Context) (DevelopmentToken, error) {
	if application == nil || application.authRepository == nil || application.now == nil {
		return DevelopmentToken{}, errors.New("application authentication service is required")
	}
	userID, err := uuid.NewV7()
	if err != nil {
		return DevelopmentToken{}, fmt.Errorf("create development user ID: %w", err)
	}
	now := application.now().UTC()
	created, verifier, err := auth.NewToken(rand.Reader, now)
	if err != nil {
		return DevelopmentToken{}, err
	}
	user := auth.UserRecord{
		ID:               userID.String(),
		Username:         "development-" + userID.String(),
		PasswordVerifier: auth.DisabledDevelopmentPasswordVerifier,
		CreatedAt:        now,
	}
	if err := application.authRepository.CreateUserAndToken(ctx, user, created, verifier); err != nil {
		return DevelopmentToken{}, err
	}
	return DevelopmentToken{UserID: user.ID, Token: created}, nil
}

func (application *App) Close() error {
	if application == nil || application.database == nil {
		return nil
	}
	err := application.database.Close()
	application.database = nil
	return err
}
