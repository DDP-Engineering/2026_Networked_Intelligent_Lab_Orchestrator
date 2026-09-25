package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"nilo.local/nilo/internal/app"
	"nilo.local/nilo/internal/artifact"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/operation"
	"nilo.local/nilo/internal/policy"
	"nilo.local/nilo/internal/session"
	"nilo.local/nilo/internal/store/sqlite"
)

type incrementFixture struct {
	DataDir         string
	StagingRoot     string
	PackageRoot     string
	DB              *sqlite.DB
	Service         operation.Service
	Runner          *capturingSupervisor
	Requests        *sqlite.OperationRepository
	Sessions        *sqlite.SessionRepository
	SessionService  session.Service
	ArtifactStore   artifact.Store
	ConfigurationID domain.SnapshotID
	Actor           domain.Actor
	IntegrationID   domain.IntegrationID
	DeviceID        domain.DeviceID
}

func newIncrementFixture(t *testing.T) *incrementFixture {
	t.Helper()
	ctx := context.Background()
	_, sourceFile, _, _ := runtime.Caller(0)
	softwareRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	packageRoot := filepath.Join(t.TempDir(), "contract_runner")
	sourcePackage := filepath.Join(softwareRoot, "tests", "fixtures", "contract_runner")
	if err := os.CopyFS(packageRoot, os.DirFS(sourcePackage)); err != nil {
		t.Fatalf("copy contract runner package: %v", err)
	}
	executable := "runner"
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.Command("go", "build", "-o", filepath.Join(packageRoot, executable), filepath.Join(packageRoot, "main.go"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build contract runner: %v: %s", err, output)
	}
	loadedManifest, err := manifest.Load(packageRoot)
	if err != nil {
		t.Fatalf("manifest.Load() error = %v", err)
	}
	processSupervisor, err := supervisor.NewProcessSupervisor(supervisor.ProcessConfig{
		PackageID: loadedManifest.Package.ID, PackageRoot: packageRoot,
		Runner: loadedManifest.Runner, GracePeriod: 500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewProcessSupervisor() error = %v", err)
	}
	runner := newCapturingSupervisor(processSupervisor)
	dataDir := t.TempDir()
	stagingRoot := filepath.Join(dataDir, "staging")
	artifactRoot := filepath.Join(dataDir, "artifacts")
	for _, directory := range []string{stagingRoot, artifactRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", directory, err)
		}
	}
	database, err := sqlite.Open(ctx, filepath.Join(dataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	configurationID, _ := domain.NewSnapshotID()
	now := time.Now().UTC()
	if _, err := database.SQL().ExecContext(ctx,
		"INSERT INTO config_versions (id, configuration_json, digest, created_at) VALUES (?, '{}', ?, ?)",
		configurationID.String(), make([]byte, sha256.Size), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed configuration snapshot: %v", err)
	}
	actorID, _ := domain.NewActorID()
	integrationID, _ := domain.NewIntegrationID()
	deviceID, _ := domain.NewDeviceID()
	sessionRepository := sqlite.NewSessionRepository(database)
	sessionService, err := session.NewService(sessionRepository, func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatalf("session.NewService() error = %v", err)
	}
	artifactStore, err := artifact.NewStore(
		artifact.Config{ArtifactRoot: artifactRoot, StagingRoot: stagingRoot, MaximumBytes: 1 << 20},
		artifact.Dependencies{Repository: sqlite.NewArtifactRepository(database), NewArtifactID: domain.NewArtifactID, Now: func() time.Time { return time.Now().UTC() }},
	)
	if err != nil {
		t.Fatalf("artifact.NewStore() error = %v", err)
	}
	validator := &manifestValidator{
		manifest: loadedManifest, integrationID: integrationID, deviceID: deviceID, devicePresent: true,
	}
	requestRepository := sqlite.NewOperationRepository(database)
	serviceValue, err := operation.NewService(operation.Dependencies{
		Validator: validator, Requests: requestRepository, Approvals: sqlite.NewPolicyRepository(database),
		Sessions: sessionService, SessionRepository: sessionRepository,
		Audit: sqlite.NewAuditRepository(database), Supervisor: runner,
		Evidence: &contractArtifactEvidence{store: artifactStore},
		Now:      func() time.Time { return time.Now().UTC() }, NewID: func() (string, error) { return uuid.NewString(), nil },
		NewSnapshotID: domain.NewSnapshotID,
	})
	if err != nil {
		t.Fatalf("operation.NewService() error = %v", err)
	}
	return &incrementFixture{
		DataDir: dataDir, StagingRoot: stagingRoot, PackageRoot: packageRoot,
		DB: database, Service: serviceValue, Runner: runner, Requests: requestRepository,
		Sessions: sessionRepository, SessionService: sessionService, ArtifactStore: artifactStore,
		ConfigurationID: configurationID, Actor: domain.Actor{ID: actorID},
		IntegrationID: integrationID, DeviceID: deviceID,
	}
}

func (fixture *incrementFixture) Command(operationName, parameters string) operation.SubmitCommand {
	return operation.SubmitCommand{
		Actor: fixture.Actor, IntegrationID: fixture.IntegrationID, OperationName: operationName,
		DeviceID: fixture.DeviceID, CanonicalParameters: json.RawMessage(parameters),
		ConfigurationSnapshot: fixture.ConfigurationID, Deadline: time.Now().UTC().Add(2 * time.Minute),
		AppKnown: true, AppMode: policy.Ask, DeviceKnown: true, DeviceMode: policy.Ask,
	}
}

func (fixture *incrementFixture) ArtifactParameters(t *testing.T) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]string{"staging_directory": fixture.StagingRoot})
	if err != nil {
		t.Fatalf("marshal artifact parameters: %v", err)
	}
	return string(encoded)
}

func (fixture *incrementFixture) AssertProgress(t *testing.T, want []float64) {
	t.Helper()
	events := fixture.Runner.Events()
	got := make([]float64, 0, len(events))
	for _, event := range events {
		var progress protocol.Progress
		if json.Unmarshal(event.Payload, &progress) == nil && progress.Type == "progress" {
			got = append(got, progress.Fraction)
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("progress fractions = %v, want %v", got, want)
	}
}

func (fixture *incrementFixture) AssertArtifact(t *testing.T, sessionValue domain.Session) {
	t.Helper()
	var artifactValue string
	if err := fixture.DB.SQL().QueryRowContext(context.Background(), "SELECT id FROM artifacts WHERE session_id = ?", sessionValue.ID.String()).Scan(&artifactValue); err != nil {
		t.Fatalf("query artifact metadata: %v", err)
	}
	artifactID, err := domain.ParseArtifactID(artifactValue)
	if err != nil {
		t.Fatalf("ParseArtifactID() error = %v", err)
	}
	reader, metadata, err := fixture.ArtifactStore.Open(context.Background(), artifactID)
	if err != nil {
		t.Fatalf("ArtifactStore.Open() error = %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(artifact) error = %v", err)
	}
	want := []byte("nilo contract artifact v1\n")
	if string(content) != string(want) || metadata.Digest != sha256.Sum256(want) || metadata.SizeBytes != int64(len(want)) {
		t.Fatalf("artifact content or metadata mismatch: content=%q metadata=%+v", content, metadata)
	}
}

func (fixture *incrementFixture) AssertSessionAndLease(t *testing.T, sessionID domain.SessionID, state domain.SessionState, activeLeases int) {
	t.Helper()
	stored, err := fixture.Sessions.Get(context.Background(), sessionID)
	if err != nil || stored.State != state {
		t.Fatalf("stored session = %+v, error = %v, want %s", stored, err, state)
	}
	var count int
	if err := fixture.DB.SQL().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM resource_leases WHERE session_id = ? AND released_at IS NULL", sessionID.String()).Scan(&count); err != nil {
		t.Fatalf("count active leases: %v", err)
	}
	if count != activeLeases {
		t.Fatalf("active leases = %d, want %d", count, activeLeases)
	}
}

func (fixture *incrementFixture) AssertAuditCorrelation(t *testing.T, sessionValue domain.Session, command operation.SubmitCommand) {
	t.Helper()
	var payload []byte
	if err := fixture.DB.SQL().QueryRowContext(context.Background(), `
		SELECT payload_json FROM audit_events WHERE action = 'operation.terminal' AND subject_id = ?
	`, sessionValue.ID.String()).Scan(&payload); err != nil {
		t.Fatalf("query terminal audit: %v", err)
	}
	var correlation map[string]any
	if err := json.Unmarshal(payload, &correlation); err != nil {
		t.Fatalf("decode terminal audit: %v", err)
	}
	requestID := sessionValue.RequestID.String()
	for key, want := range map[string]string{
		"request_id": requestID, "session_id": sessionValue.ID.String(), "actor_id": command.Actor.ID.String(),
		"integration_id": command.IntegrationID.String(), "device_id": command.DeviceID.String(),
	} {
		if got := correlation[key]; got != want {
			t.Fatalf("audit correlation %s = %v, want %s", key, got, want)
		}
	}
}

func (fixture *incrementFixture) ProveRestartReconciliation(t *testing.T, now time.Time) {
	t.Helper()
	snapshotID, _ := domain.NewSnapshotID()
	snapshot, err := policy.Resolve(policy.ResolutionInput{
		SnapshotID: snapshotID, IntegrationID: fixture.IntegrationID, OperationName: "sleep", DeviceID: fixture.DeviceID,
		AppKnown: true, AppMode: policy.Full, ManifestPolicy: policy.FullOnly,
		DeviceKnown: true, DeviceMode: policy.Full, ResolvedAt: now,
	})
	if err != nil {
		t.Fatalf("Resolve(restart) error = %v", err)
	}
	request, err := domain.NewOperationRequest(domain.OperationRequestInput{
		Actor: fixture.Actor, IntegrationID: fixture.IntegrationID, OperationName: "sleep", DeviceID: fixture.DeviceID,
		CanonicalParameters: json.RawMessage(`{"duration_ms":60000}`), ConfigurationSnapshot: fixture.ConfigurationID,
		PolicySnapshot: snapshotID, Deadline: now.Add(time.Minute),
	}, now)
	if err != nil {
		t.Fatalf("NewOperationRequest(restart) error = %v", err)
	}
	if err := fixture.Requests.Create(context.Background(), request, snapshot); err != nil {
		t.Fatalf("Requests.Create(restart) error = %v", err)
	}
	queued, err := fixture.SessionService.Create(context.Background(), request, policy.Allow)
	if err != nil {
		t.Fatalf("Sessions.Create(restart) error = %v", err)
	}
	resource := fixture.IntegrationID.String() + "/device/stable-001"
	if err := fixture.Sessions.AcquireLeases(context.Background(), queued.ID, []string{resource}, now); err != nil {
		t.Fatalf("AcquireLeases(restart) error = %v", err)
	}
	running, err := fixture.SessionService.Transition(context.Background(), queued.ID, domain.Queued, domain.Running)
	if err != nil {
		t.Fatalf("Transition(restart) error = %v", err)
	}
	if err := fixture.DB.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}
	fixture.DB = nil
	application, err := app.New(context.Background(), app.Options{
		DataDir: fixture.DataDir, BindAddress: "127.0.0.1:0", Now: func() time.Time { return now.Add(time.Second) },
	})
	if err != nil {
		t.Fatalf("app.New(restart) error = %v", err)
	}
	if err := application.Close(); err != nil {
		t.Fatalf("app.Close(restart) error = %v", err)
	}
	reopened, err := sqlite.Open(context.Background(), filepath.Join(fixture.DataDir, "nilo.db"))
	if err != nil {
		t.Fatalf("sqlite.Open(after restart) error = %v", err)
	}
	fixture.DB = reopened
	stored, err := sqlite.NewSessionRepository(reopened).Get(context.Background(), running.ID)
	if err != nil || stored.State != domain.Incomplete {
		t.Fatalf("restart session = %+v, error = %v", stored, err)
	}
	var leaseCount, auditCount int
	if err := reopened.SQL().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM resource_leases WHERE session_id = ? AND released_at IS NULL", running.ID.String()).Scan(&leaseCount); err != nil {
		t.Fatalf("count restart leases: %v", err)
	}
	if err := reopened.SQL().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM audit_events WHERE action = 'session.reconciled' AND subject_id = ?", running.ID.String()).Scan(&auditCount); err != nil {
		t.Fatalf("count restart audits: %v", err)
	}
	if leaseCount != 0 || auditCount != 1 {
		t.Fatalf("restart evidence: leases=%d audits=%d", leaseCount, auditCount)
	}
}

func (fixture *incrementFixture) Close(t *testing.T) {
	t.Helper()
	if fixture.Runner != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := fixture.Runner.Shutdown(ctx); err != nil && !errors.Is(err, supervisor.ErrSupervisorClosed) {
			t.Errorf("Supervisor.Shutdown() error = %v", err)
		}
	}
	if fixture.DB != nil {
		if err := fixture.DB.Close(); err != nil {
			t.Errorf("DB.Close() error = %v", err)
		}
	}
}

type manifestValidator struct {
	manifest      manifest.Manifest
	integrationID domain.IntegrationID
	deviceID      domain.DeviceID
	devicePresent bool
}

func (validator *manifestValidator) Validate(_ context.Context, command operation.SubmitCommand) (operation.Validation, error) {
	return validator.validate(command.IntegrationID, command.DeviceID, command.OperationName, command.CanonicalParameters)
}

func (validator *manifestValidator) Revalidate(_ context.Context, request domain.OperationRequest) (operation.Validation, error) {
	if !validator.devicePresent {
		return operation.Validation{}, errors.New("device is not present")
	}
	return validator.validate(request.IntegrationID(), request.DeviceID(), request.OperationName(), request.CanonicalParameters())
}

func (validator *manifestValidator) validate(integrationID domain.IntegrationID, deviceID domain.DeviceID, operationName string, parameters json.RawMessage) (operation.Validation, error) {
	if integrationID != validator.integrationID || deviceID != validator.deviceID {
		return operation.Validation{}, errors.New("unknown integration or device")
	}
	var descriptor *manifest.Operation
	for index := range validator.manifest.Operations {
		if validator.manifest.Operations[index].ID == operationName {
			descriptor = &validator.manifest.Operations[index]
			break
		}
	}
	if descriptor == nil {
		return operation.Validation{}, errors.New("unknown operation")
	}
	if err := validateParameters(descriptor.InputSchema, parameters); err != nil {
		return operation.Validation{}, err
	}
	resources := make([]string, 0, len(descriptor.Resources))
	for _, claim := range descriptor.Resources {
		parts := strings.Split(string(claim), "/")
		resources = append(resources, integrationID.String()+"/"+parts[1]+"/"+parts[2])
	}
	validation := operation.Validation{
		PackageID: validator.manifest.Package.ID, ManifestPolicy: descriptor.Policy, Resources: resources,
	}
	if operationName == "artifact" {
		for _, declaration := range validator.manifest.Artifacts {
			validation.RequiredArtifacts = append(validation.RequiredArtifacts, declaration.ID)
		}
	}
	return validation, nil
}

func validateParameters(schemaPath string, parameters json.RawMessage) error {
	document, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	var schemaValue any
	if err := json.Unmarshal(document, &schemaValue); err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const resource = "urn:nilo:acceptance-input"
	if err := compiler.AddResource(resource, schemaValue); err != nil {
		return err
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		return err
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(parameters)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return compiled.Validate(value)
}

type capturingSupervisor struct {
	delegate supervisor.Supervisor
	mu       sync.Mutex
	events   []protocol.Envelope
	eventCh  chan protocol.Envelope
	executed int
}

func newCapturingSupervisor(delegate supervisor.Supervisor) *capturingSupervisor {
	return &capturingSupervisor{delegate: delegate, eventCh: make(chan protocol.Envelope, 128)}
}

func (runner *capturingSupervisor) Describe(ctx context.Context, packageID string) (protocol.DescribeResult, error) {
	return runner.delegate.Describe(ctx, packageID)
}

func (runner *capturingSupervisor) Execute(ctx context.Context, request protocol.ExecuteRequest, sink protocol.EventSink) (protocol.ExecuteResult, error) {
	runner.mu.Lock()
	runner.executed++
	runner.mu.Unlock()
	return runner.delegate.Execute(ctx, request, protocol.EventSinkFunc(func(eventContext context.Context, event protocol.Envelope) error {
		copyValue := event
		copyValue.Payload = append(json.RawMessage(nil), event.Payload...)
		runner.mu.Lock()
		runner.events = append(runner.events, copyValue)
		runner.mu.Unlock()
		select {
		case runner.eventCh <- copyValue:
		default:
		}
		return sink.Emit(eventContext, event)
	}))
}

func (runner *capturingSupervisor) Cancel(ctx context.Context, packageID string, requestID domain.RequestID) error {
	return runner.delegate.Cancel(ctx, packageID, requestID)
}

func (runner *capturingSupervisor) Shutdown(ctx context.Context) error {
	return runner.delegate.Shutdown(ctx)
}

func (runner *capturingSupervisor) ExecutionCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.executed
}

func (runner *capturingSupervisor) ResetEvents() {
	runner.mu.Lock()
	runner.events = nil
	runner.mu.Unlock()
	for {
		select {
		case <-runner.eventCh:
		default:
			return
		}
	}
}

func (runner *capturingSupervisor) Events() []protocol.Envelope {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]protocol.Envelope(nil), runner.events...)
}

func (runner *capturingSupervisor) WaitForProgress(t *testing.T, message string) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-runner.eventCh:
			var progress protocol.Progress
			if json.Unmarshal(event.Payload, &progress) == nil && progress.Message == message {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for progress %q", message)
		}
	}
}

type contractArtifactEvidence struct {
	store artifact.Store
}

func (evidence *contractArtifactEvidence) Persist(ctx context.Context, value operation.Evidence) error {
	var declaration protocol.ArtifactDeclaration
	if err := json.Unmarshal(value.Event.Payload, &declaration); err != nil {
		return err
	}
	var parameters struct {
		StagingDirectory string `json:"staging_directory"`
	}
	if err := json.Unmarshal(value.Request.CanonicalParameters(), &parameters); err != nil {
		return err
	}
	digestText := strings.TrimPrefix(declaration.Digest, "sha256:")
	digestBytes, err := hex.DecodeString(digestText)
	if err != nil || len(digestBytes) != sha256.Size {
		return errors.New("invalid declared artifact digest")
	}
	var digest [sha256.Size]byte
	copy(digest[:], digestBytes)
	_, err = evidence.store.Register(ctx, artifact.RegisterInput{
		StagedPath: filepath.Join(parameters.StagingDirectory, "contract-artifact.bin"),
		RequestID:  value.Request.ID(), SessionID: value.Session.ID, DeviceID: value.Request.DeviceID(),
		IntegrationID: value.Request.IntegrationID(), MediaType: declaration.MediaType,
		DeclaredDigest: digest, DeclaredSize: declaration.SizeBytes,
		TerminalState: domain.Completed, PresentationHints: json.RawMessage(`{}`),
	})
	return err
}

var _ operation.Validator = (*manifestValidator)(nil)
var _ supervisor.Supervisor = (*capturingSupervisor)(nil)
var _ operation.EvidenceStore = (*contractArtifactEvidence)(nil)
