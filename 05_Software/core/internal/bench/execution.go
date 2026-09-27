package bench

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/artifact"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/operation"
	"nilo.local/nilo/internal/session"
	"nilo.local/nilo/internal/store/sqlite"
)

type jobRunner struct {
	supervisor.Supervisor
	manager *Manager
	job     Job
	plugin  Plugin
	stage   string
	replyMu sync.Mutex
}

func (m *Manager) operationService(job Job, p Plugin) (operation.Service, *jobRunner, error) {
	deviceID := ""
	if job.PluginID == "camera" || job.PluginID == "serial" {
		deviceID = job.Submission.DeviceID
	}
	runner, err := m.processConfigured(p, deviceID)
	if err != nil {
		return nil, nil, err
	}
	adapter := &jobRunner{Supervisor: runner, manager: m, job: job, plugin: p, stage: filepath.Join(m.options.DataDir, "staging", job.ID)}
	repository := sqlite.NewSessionRepository(m.options.Database)
	sessions, err := session.NewService(repository, m.now)
	if err != nil {
		return nil, nil, err
	}
	service, err := operation.NewService(operation.Dependencies{Validator: adapter, Requests: sqlite.NewOperationRepository(m.options.Database), Approvals: sqlite.NewPolicyRepository(m.options.Database), Sessions: sessions, SessionRepository: repository, Audit: sqlite.NewAuditRepository(m.options.Database), Supervisor: adapter, Evidence: adapter, Now: m.now, NewID: func() (string, error) { id, err := uuid.NewV7(); return id.String(), err }, NewSnapshotID: domain.NewSnapshotID})
	return service, adapter, err
}

func (r *jobRunner) validation() (operation.Validation, error) {
	root, err := r.manager.packageRoot(r.plugin.ID)
	if err != nil {
		return operation.Validation{}, err
	}
	descriptor, err := manifest.Load(root)
	if err != nil {
		return operation.Validation{}, err
	}
	adapter, err := adapterFor(r.plugin.ID)
	if err != nil {
		return operation.Validation{}, err
	}
	validation := operation.Validation{PackageID: r.plugin.ID, RequiredArtifacts: append([]string(nil), adapter.requiredArtifacts[r.job.Submission.Operation]...)}
	found := false
	for _, declared := range descriptor.Operations {
		if declared.ID != r.job.Submission.Operation {
			continue
		}
		found = true
		validation.ManifestPolicy = declared.Policy
		for _, resource := range declared.Resources {
			validation.Resources = append(validation.Resources, r.plugin.InstanceID+strings.TrimPrefix(string(resource), r.plugin.ID))
		}
	}
	if !found {
		return operation.Validation{}, ErrInvalid
	}
	if resource := resourceFor(r.job); resource != "" {
		validation.Resources = append(validation.Resources, r.plugin.InstanceID+strings.TrimPrefix(resource, r.plugin.ID))
	}
	sort.Strings(validation.Resources)
	return validation, nil
}
func (r *jobRunner) Validate(context.Context, operation.SubmitCommand) (operation.Validation, error) {
	return r.validation()
}
func (r *jobRunner) Revalidate(ctx context.Context, request domain.OperationRequest) (operation.Validation, error) {
	p, err := r.manager.loadPlugin(ctx, r.plugin.ID)
	if err != nil {
		return operation.Validation{}, err
	}
	if !p.Enabled || !p.Installed || p.Digest != r.plugin.Digest {
		return operation.Validation{}, ErrUnavailable
	}
	if request.OperationName() != r.job.Submission.Operation {
		return operation.Validation{}, ErrInvalid
	}
	return r.validation()
}

func (r *jobRunner) Execute(ctx context.Context, request protocol.ExecuteRequest, sink protocol.EventSink) (protocol.ExecuteResult, error) {
	if r.plugin.ID == "serial" && request.Operation == "connect" {
		return r.manager.executeSerialConnect(ctx, r.job, r.plugin)
	}
	if r.plugin.ID == "camera" && request.Operation == "workspace" {
		result, err := r.manager.executeCameraCommand(ctx, r.job, r.plugin)
		if err == nil {
			_, err = r.manager.db().ExecContext(ctx, `UPDATE bench_jobs SET result_json=? WHERE id=?`, string(result.Data), r.job.ID)
		}
		return result, err
	}
	if err := os.Mkdir(r.stage, 0700); err != nil {
		return protocol.ExecuteResult{}, err
	}
	adapter, err := adapterFor(r.plugin.ID)
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	if adapter.prepareExecution != nil {
		request, err = adapter.prepareExecution(ctx, r, request)
		if err != nil {
			return protocol.ExecuteResult{}, err
		}
	}
	result, err := r.Supervisor.Execute(ctx, request, protocol.EventSinkFunc(func(ctx context.Context, event protocol.Envelope) error {
		if err := r.reply(ctx, event); err != nil {
			return err
		}
		return sink.Emit(ctx, event)
	}))
	if err != nil {
		return result, err
	}
	if len(result.Data) > 1<<20 {
		return result, ErrInvalid
	}
	_, err = r.manager.db().ExecContext(ctx, `UPDATE bench_jobs SET result_json=? WHERE id=? AND session_id IN (SELECT id FROM sessions WHERE state='running')`, string(result.Data), r.job.ID)
	return result, err
}

// reply retains a bounded ordered queue, correlated at both the wire request
// and public transaction boundaries. Late replies cannot modify terminal jobs.
func (r *jobRunner) reply(ctx context.Context, event protocol.Envelope) error {
	r.replyMu.Lock()
	defer r.replyMu.Unlock()
	if len(event.Payload) > 4096 {
		return ErrInvalid
	}
	job, err := r.manager.job(ctx, r.job.ID)
	if err != nil {
		return err
	}
	if job.State != "running" {
		return ErrConflict
	}
	if len(job.Replies) >= 100 {
		return errors.New("reply_queue_full")
	}
	job.Replies = append(job.Replies, Reply{Sequence: len(job.Replies) + 1, TransactionID: job.ID, Time: r.manager.now(), Payload: append(json.RawMessage(nil), event.Payload...)})
	raw, _ := json.Marshal(job.Replies)
	_, err = r.manager.db().ExecContext(ctx, `UPDATE bench_jobs SET events_json=? WHERE id=? AND session_id IN (SELECT id FROM sessions WHERE state='running')`, string(raw), job.ID)
	return err
}

func (r *jobRunner) Persist(ctx context.Context, evidence operation.Evidence) error {
	var declaration protocol.ArtifactDeclaration
	if json.Unmarshal(evidence.Event.Payload, &declaration) != nil {
		return ErrInvalid
	}
	adapter, err := adapterFor(r.plugin.ID)
	if err != nil {
		return err
	}
	if _, ok := adapter.artifacts[declaration.ID]; !ok {
		return ErrInvalid
	}
	required := false
	for _, id := range adapter.requiredArtifacts[r.job.Submission.Operation] {
		if id == declaration.ID {
			required = true
		}
	}
	if !required || adapter.artifactFile == nil {
		return ErrInvalid
	}
	root, err := r.manager.packageRoot(r.plugin.ID)
	if err != nil {
		return err
	}
	descriptor, err := manifest.Load(root)
	if err != nil {
		return err
	}
	declared := false
	for _, artifact := range descriptor.Artifacts {
		if artifact.ID != declaration.ID {
			continue
		}
		for _, mediaType := range artifact.MediaTypes {
			if mediaType == declaration.MediaType {
				declared = true
			}
		}
	}
	if !declared {
		return ErrInvalid
	}
	stagedName, presentationName, err := adapter.artifactFile(declaration.ID, declaration.MediaType)
	if err != nil || !artifactBasename.MatchString(stagedName) || !artifactBasename.MatchString(presentationName) || strings.Contains(stagedName, "..") || strings.Contains(presentationName, "..") {
		return ErrInvalid
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(declaration.Digest, "sha256:"))
	if err != nil || len(digest) != 32 {
		return ErrInvalid
	}
	var hash [32]byte
	copy(hash[:], digest)
	hints, _ := json.Marshal(map[string]string{"filename": presentationName})
	saved, err := r.manager.artifacts.Register(ctx, artifact.RegisterInput{StagedPath: filepath.Join(r.stage, stagedName), RequestID: evidence.Request.ID(), SessionID: evidence.Session.ID, DeviceID: evidence.Request.DeviceID(), IntegrationID: evidence.Request.IntegrationID(), MediaType: declaration.MediaType, DeclaredDigest: hash, DeclaredSize: declaration.SizeBytes, TerminalState: domain.Completed, PresentationHints: hints})
	if err != nil {
		return err
	}
	_, err = r.manager.db().ExecContext(ctx, `UPDATE bench_jobs SET artifact_id=? WHERE id=?`, saved.ID.String(), r.job.ID)
	return err
}

func (r *jobRunner) cleanup() {
	// stage is created solely from a generated UUID below the configured root.
	r.manager.report(r.Supervisor.Shutdown(context.Background()))
	r.manager.report(os.RemoveAll(r.stage))
}
