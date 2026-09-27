// Package bench owns managed plugins, durable instrument transactions and bounded execution.
package bench

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"nilo.local/nilo/internal/artifact"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/config"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/operation"
	"nilo.local/nilo/internal/store/sqlite"
)

// Options supplies server-owned storage, curated runtime paths and diagnostics.
type Options struct {
	Database   *sqlite.DB
	DataDir    string
	BundleDir  string
	Backend    string
	Simulation bool
	Audit      *audit.Writer
	Now        func() time.Time
	Report     func(error)
	runner     func(Plugin) (supervisor.Supervisor, error)
}

type execution struct {
	service  operation.Service
	cancel   context.CancelFunc
	resource string
}

// Manager serializes admission and lifecycle changes while scheduling independent jobs.
type Manager struct {
	options       Options
	mu            sync.Mutex
	active        map[string]execution
	cancel        context.CancelFunc
	wake          chan struct{}
	done          chan struct{}
	hardwareDone  chan struct{}
	workers       sync.WaitGroup
	artifacts     artifact.Store
	closed        bool
	cameras       map[string]*cameraSession
	cameraWorkers sync.WaitGroup
	serials       map[string]*serialSession
	serialWorkers sync.WaitGroup
	serialReads   map[string]int
	cameraReads   map[string]int
	cameraSpace   func(string) (int64, int64, error)
}

// New starts one manager after app startup reconciliation; never share its data directory with another server.
func New(options Options) (*Manager, error) {
	if options.Database == nil || options.DataDir == "" || options.Audit == nil {
		return nil, ErrInvalid
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Backend == "" {
		options.Backend = "/usr/bin/sigrok-cli"
	}
	if options.Simulation && !filepath.IsAbs(options.Backend) {
		absolute, err := filepath.Abs(options.Backend)
		if err != nil {
			return nil, err
		}
		options.Backend = absolute
	}
	if !filepath.IsAbs(options.DataDir) || !filepath.IsAbs(options.Backend) {
		return nil, ErrInvalid
	}
	for _, name := range []string{"integrations", "staging", "artifacts"} {
		if err := os.MkdirAll(filepath.Join(options.DataDir, name), 0700); err != nil {
			return nil, err
		}
	}
	if err := recoverPackageUpgrades(options); err != nil {
		return nil, err
	}
	storage, err := artifact.NewStore(artifact.Config{ArtifactRoot: filepath.Join(options.DataDir, "artifacts"), StagingRoot: filepath.Join(options.DataDir, "staging"), MaximumBytes: 8 << 20}, artifact.Dependencies{Repository: sqlite.NewArtifactRepository(options.Database), NewArtifactID: domain.NewArtifactID, Now: func() time.Time { return options.Now().UTC() }})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{options: options, active: make(map[string]execution), cancel: cancel, wake: make(chan struct{}, 1), done: make(chan struct{}), artifacts: storage}
	m.cameras = make(map[string]*cameraSession)
	m.cameraReads = make(map[string]int)
	m.cameraSpace = cameraDiskSpace
	m.serials = make(map[string]*serialSession)
	m.serialReads = make(map[string]int)
	if err := m.recoverSerialLogs(ctx); err != nil {
		cancel()
		return nil, err
	}
	if err := m.recoverCameraMedia(ctx); err != nil {
		cancel()
		return nil, err
	}
	// app.New reconciles sessions first; also handle previously pending/queued
	// jobs because a restarted server must not replay instrument actions.
	_, err = options.Database.SQL().ExecContext(ctx, `UPDATE sessions SET state='incomplete',updated_at=?,terminal_at=? WHERE id IN (SELECT session_id FROM bench_jobs) AND state IN ('pending_approval','queued','running')`, m.now().Format(time.RFC3339Nano), m.now().Format(time.RFC3339Nano))
	if err != nil {
		cancel()
		return nil, err
	}
	if err := sqlite.NewManagedDeviceRepository(options.Database).MarkUnknown(ctx, "camera"); err != nil {
		cancel()
		return nil, err
	}
	if camera, err := m.loadPlugin(ctx, "camera"); err == nil && camera.Installed && camera.Enabled {
		startupCtx, finish := context.WithTimeout(ctx, 15*time.Second)
		_, discoveryErr := m.discoverCamera(startupCtx, camera)
		finish()
		m.report(discoveryErr)
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		m.report(err)
	}
	if err := sqlite.NewManagedDeviceRepository(options.Database).MarkUnknown(ctx, "serial"); err != nil {
		cancel()
		return nil, err
	}
	if p, err := m.loadPlugin(ctx, "serial"); err == nil && p.Installed && p.Enabled {
		c, done := context.WithTimeout(ctx, 10*time.Second)
		_, e := m.discoverSerial(c, p)
		done()
		m.report(e)
	}
	m.hardwareDone = make(chan struct{})
	if !options.Simulation {
		go m.monitorHardware(ctx)
	} else {
		close(m.hardwareDone)
	}
	go m.schedule(ctx)
	return m, nil
}

func (m *Manager) now() time.Time { return m.options.Now().UTC() }
func (m *Manager) db() *sql.DB    { return m.options.Database.SQL() }
func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Close cancels work and waits for all runner cleanup.
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	m.stopSerialLocked("serial", "shutdown")
	for _, camera := range m.cameras {
		shutdown, finish := context.WithTimeout(context.Background(), 12*time.Second)
		_, _ = cameraCall(shutdown, camera, "POST", "/command", map[string]string{"action": "stop"})
		finish()
		camera.cancel()
	}
	m.cancel()
	m.mu.Unlock()
	<-m.done
	<-m.hardwareDone
	m.workers.Wait()
	m.cameraWorkers.Wait()
	m.serialWorkers.Wait()
	return nil
}

func (m *Manager) record(ctx context.Context, actor identity.Actor, action, id string, payload any) error {
	actorID, err := domain.ParseActorID(actor.User.ID)
	if err != nil {
		return ErrForbidden
	}
	kind := audit.ActorKindUser
	if actor.TokenID != "" {
		kind = audit.ActorKindToken
	}
	outcome := audit.OutcomeApplied
	if strings.HasSuffix(action, "_requested") {
		outcome = audit.OutcomeRequested
	}
	return m.options.Audit.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: kind, ActorID: actorID, Action: action, SubjectType: "integration", SubjectID: id, Outcome: outcome, Details: payload})
}

func require(actor identity.Actor, permission auth.Permission) error {
	if !actor.Has(permission) {
		return ErrForbidden
	}
	return nil
}

func strict(raw json.RawMessage, value any) error {
	return decodeStrict(raw, value)
}

func (m *Manager) packageRoot(id string) (string, error) {
	if !packageIDPattern.MatchString(id) {
		return "", ErrInvalid
	}
	return config.ResolveWithin(filepath.Join(m.options.DataDir, "integrations"), id)
}

func normalizedError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// report exposes background storage/cleanup failures to the server diagnostics.
func (m *Manager) report(err error) {
	if err != nil && m.options.Report != nil {
		m.options.Report(err)
	}
}
