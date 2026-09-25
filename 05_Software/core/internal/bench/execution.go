package bench

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/artifact"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/operation"
	"nilo.local/nilo/internal/policy"
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
	runner, err := m.process(p)
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
	validation := operation.Validation{PackageID: r.plugin.ID, ManifestPolicy: policy.AutoEligible}
	if resource := resourceFor(r.job); resource != "" {
		validation.Resources = []string{r.plugin.InstanceID + strings.TrimPrefix(resource, r.plugin.ID)}
		validation.RequiredArtifacts = []string{"capture"}
	}
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
	if err := os.Mkdir(r.stage, 0700); err != nil {
		return protocol.ExecuteResult{}, err
	}
	var parameters map[string]any
	if err := json.Unmarshal(request.Parameters, &parameters); err != nil {
		return protocol.ExecuteResult{}, err
	}
	if request.Operation == "capture" {
		conn := ""
		for _, device := range r.plugin.Devices {
			if device.ID == r.job.Submission.DeviceID {
				conn = device.Connection
			}
		}
		if !connectionPattern.MatchString(conn) {
			return protocol.ExecuteResult{}, ErrNotFound
		}
		parameters["conn"] = conn
		parameters["staging_directory"] = r.stage
	} else {
		var input decodeInput
		if strict(request.Parameters, &input) != nil {
			return protocol.ExecuteResult{}, ErrInvalid
		}
		source, err := r.manager.job(ctx, input.CaptureID)
		if err != nil {
			return protocol.ExecuteResult{}, err
		}
		artifactID, err := domain.ParseArtifactID(source.ArtifactID)
		if err != nil {
			return protocol.ExecuteResult{}, err
		}
		content, _, err := r.manager.artifacts.Open(ctx, artifactID)
		if err != nil {
			return protocol.ExecuteResult{}, err
		}
		path := filepath.Join(r.stage, "input.sr")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			content.Close()
			return protocol.ExecuteResult{}, err
		}
		_, copyErr := io.Copy(file, content)
		err = errors.Join(copyErr, file.Close(), content.Close())
		if err != nil {
			return protocol.ExecuteResult{}, err
		}
		parameters = map[string]any{"input_path": path, "channel": input.Channel, "baudrate": input.Baudrate}
	}
	request.Parameters, _ = json.Marshal(parameters)
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
	if json.Unmarshal(evidence.Event.Payload, &declaration) != nil || declaration.ID != "capture" || r.job.Submission.Operation != "capture" {
		return ErrInvalid
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(declaration.Digest, "sha256:"))
	if err != nil || len(digest) != 32 {
		return ErrInvalid
	}
	var hash [32]byte
	copy(hash[:], digest)
	saved, err := r.manager.artifacts.Register(ctx, artifact.RegisterInput{StagedPath: filepath.Join(r.stage, "capture.sr"), RequestID: evidence.Request.ID(), SessionID: evidence.Session.ID, DeviceID: evidence.Request.DeviceID(), IntegrationID: evidence.Request.IntegrationID(), MediaType: declaration.MediaType, DeclaredDigest: hash, DeclaredSize: declaration.SizeBytes, TerminalState: domain.Completed, PresentationHints: json.RawMessage(`{"filename":"capture.sr"}`)})
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
