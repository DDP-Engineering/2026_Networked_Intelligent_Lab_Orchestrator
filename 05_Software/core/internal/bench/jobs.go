package bench

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/operation"
	"nilo.local/nilo/internal/policy"
)

const jobColumns = `j.id,j.session_id,s.request_id,j.plugin_id,j.owner_id,s.state,j.submission_json,j.created_at,j.deadline,j.result_json,j.events_json,j.error,j.artifact_id`

type scanner interface{ Scan(...any) error }

func scanJob(row scanner) (Job, error) {
	var job Job
	var submission, events, result []byte
	var created, deadline string
	err := row.Scan(&job.ID, &job.SessionID, &job.RequestID, &job.PluginID, &job.OwnerID, &job.State, &submission, &created, &deadline, &result, &events, &job.Error, &job.ArtifactID)
	if err != nil {
		return job, normalizedError(err)
	}
	job.Result = append(json.RawMessage(nil), result...)
	if err = json.Unmarshal(submission, &job.Submission); err != nil {
		return job, err
	}
	if err = json.Unmarshal(events, &job.Replies); err != nil {
		return job, err
	}
	if job.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return job, err
	}
	job.Deadline, err = time.Parse(time.RFC3339Nano, deadline)
	return job, err
}
func (m *Manager) job(ctx context.Context, id string) (Job, error) {
	return scanJob(m.db().QueryRowContext(ctx, `SELECT `+jobColumns+` FROM bench_jobs j JOIN sessions s ON s.id=j.session_id WHERE j.id=?`, id))
}
func canRead(actor identity.Actor, job Job) bool {
	return actor.User.ID == job.OwnerID || actor.Has(auth.PermissionPluginsManage)
}
func (m *Manager) GetJob(ctx context.Context, actor identity.Actor, id string) (Job, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return Job{}, err
	}
	job, err := m.job(ctx, id)
	if err == nil && !canRead(actor, job) {
		return Job{}, ErrForbidden
	}
	return job, err
}
func (m *Manager) ListJobs(ctx context.Context, actor identity.Actor) ([]Job, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return nil, err
	}
	summaryColumns := strings.Replace(jobColumns, "j.result_json,j.events_json", "'{}','[]'", 1)
	query := `SELECT ` + summaryColumns + ` FROM bench_jobs j JOIN sessions s ON s.id=j.session_id`
	args := []any{}
	if !actor.Has(auth.PermissionPluginsManage) {
		query += ` WHERE j.owner_id=?`
		args = append(args, actor.User.ID)
	}
	query += ` ORDER BY j.created_at DESC,j.id DESC LIMIT 100`
	rows, err := m.db().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, job)
	}
	return result, rows.Err()
}

// Submit acknowledges only after both the immutable operation and its queue
// record are durable. A repeated owner/key pair returns the original transaction.
func (m *Manager) Submit(ctx context.Context, actor identity.Actor, input Submission) (Job, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return Job{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Job{}, ErrUnavailable
	}
	if !keyPattern.MatchString(input.IdempotencyKey) {
		return Job{}, ErrInvalid
	}
	existing, err := scanJob(m.db().QueryRowContext(ctx, `SELECT `+jobColumns+` FROM bench_jobs j JOIN sessions s ON s.id=j.session_id WHERE j.owner_id=? AND j.idempotency_key=?`, actor.User.ID, input.IdempotencyKey))
	if err == nil {
		// A retry retains the original normalized timeout even when the
		// administrator has since changed the default or maximum.
		retry, retryErr := canonicalSubmissionWithLimits(input, existing.Submission.TimeoutSeconds, 3600)
		if retryErr != nil {
			return Job{}, retryErr
		}
		previous, _ := json.Marshal(existing.Submission)
		canonical, _ := json.Marshal(retry)
		if !bytes.Equal(previous, canonical) {
			return Job{}, ErrConflict
		}
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Job{}, err
	}
	input, err = canonicalSubmissionWithLimits(input, m.settings.DefaultJobTimeoutSeconds, m.settings.MaxJobTimeoutSeconds)
	if err != nil {
		return Job{}, err
	}
	canonical, _ := json.Marshal(input)
	p, err := m.loadPlugin(ctx, input.PluginID)
	if err != nil {
		return Job{}, err
	}
	if !p.Installed || !p.Enabled {
		return Job{}, ErrUnavailable
	}
	var count int
	if err = m.db().QueryRowContext(ctx, `SELECT count(*) FROM bench_jobs j JOIN sessions s ON s.id=j.session_id WHERE s.state IN ('pending_approval','queued','running')`).Scan(&count); err != nil {
		return Job{}, err
	}
	if count >= m.settings.OutstandingJobs {
		return Job{}, ErrQueueFull
	}
	adapter, err := adapterFor(input.PluginID)
	if err != nil {
		return Job{}, err
	}
	if adapter.validateSubmission != nil {
		if err := adapter.validateSubmission(ctx, m, actor, p, input); err != nil {
			return Job{}, err
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Job{}, err
	}
	now := m.now()
	job := Job{ID: id.String(), PluginID: input.PluginID, OwnerID: actor.User.ID, Submission: input, CreatedAt: now, Deadline: now.Add(time.Duration(input.TimeoutSeconds) * time.Second)}
	service, _, err := m.operationService(job, p)
	if err != nil {
		return Job{}, err
	}
	snapshot, err := domain.NewSnapshotID()
	if err != nil {
		return Job{}, err
	}
	config, _ := json.Marshal(map[string]string{"plugin_id": p.ID, "digest": p.Digest, "mode": p.Mode})
	digest := sha256.Sum256(config)
	if _, err = m.db().ExecContext(ctx, `INSERT INTO config_versions(id,configuration_json,digest,created_at) VALUES(?,?,?,?)`, snapshot.String(), string(config), digest[:], now.Format(time.RFC3339Nano)); err != nil {
		return Job{}, err
	}
	actorID, err := domain.ParseActorID(actor.User.ID)
	if err != nil {
		return Job{}, ErrForbidden
	}
	integrationID, err := domain.ParseIntegrationID(p.InstanceID)
	if err != nil {
		return Job{}, err
	}
	deviceID := domain.DeviceID(uuid.NewSHA1(uuid.NameSpaceOID, []byte(p.ID+":"+input.DeviceID)))
	created, err := service.Prepare(ctx, operation.SubmitCommand{Actor: domain.Actor{ID: actorID}, IntegrationID: integrationID, OperationName: input.Operation, DeviceID: deviceID, CanonicalParameters: input.Parameters, ConfigurationSnapshot: snapshot, Deadline: job.Deadline, AppKnown: true, AppMode: policy.AppMode(p.Mode), DeviceKnown: true, DeviceMode: policy.AppMode(p.Mode)})
	if err != nil {
		return Job{}, err
	}
	_, err = m.db().ExecContext(ctx, `INSERT INTO bench_jobs(id,session_id,plugin_id,owner_id,credential_id,browser_session_id,idempotency_key,submission_json,deadline,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, job.ID, created.ID.String(), p.ID, actor.User.ID, actor.TokenID, actor.SessionID, input.IdempotencyKey, string(canonical), job.Deadline.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		_, cleanupErr := m.db().ExecContext(context.WithoutCancel(ctx), `UPDATE sessions SET state='incomplete',terminal_at=?,updated_at=? WHERE id=?`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), created.ID.String())
		return Job{}, errors.Join(err, cleanupErr)
	}
	m.signal()
	return m.job(ctx, job.ID)
}

func (m *Manager) Action(ctx context.Context, actor identity.Actor, id, action string) (Job, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return Job{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, err := m.job(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if !canRead(actor, job) {
		return Job{}, ErrForbidden
	}
	if action != "cancel" && action != "approve" {
		return Job{}, ErrInvalid
	}
	if action == "approve" && (actor.Credential != identity.CredentialSession || !actor.Has(auth.PermissionBenchApprove)) {
		return Job{}, ErrForbidden
	}
	if terminal(job.State) {
		return job, nil
	}
	if err = m.record(ctx, actor, "bench."+action, id, nil); err != nil {
		return job, err
	}
	if action == "cancel" {
		if run, ok := m.active[id]; ok {
			run.cancel()
		} else {
			err = m.settle(ctx, job, "cancelled", "cancelled_before_execution")
		}
	} else {
		p, loadErr := m.loadPlugin(ctx, job.PluginID)
		if loadErr != nil {
			return job, loadErr
		}
		if !p.Enabled {
			return job, ErrUnavailable
		}
		service, _, buildErr := m.operationService(job, p)
		if buildErr != nil {
			return job, buildErr
		}
		sessionID, _ := domain.ParseSessionID(job.SessionID)
		actorID, _ := domain.ParseActorID(actor.User.ID)
		_, err = service.ApproveQueued(ctx, sessionID, domain.Actor{ID: actorID})
	}
	if err != nil {
		return job, err
	}
	m.signal()
	return m.job(ctx, id)
}

func (m *Manager) OpenArtifact(ctx context.Context, actor identity.Actor, id string) (io.ReadCloser, domain.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, err := m.GetJob(ctx, actor, id)
	if err != nil {
		return nil, domain.Artifact{}, err
	}
	if job.State != "completed" || job.ArtifactID == "" {
		return nil, domain.Artifact{}, ErrNotFound
	}
	artifactID, err := domain.ParseArtifactID(job.ArtifactID)
	if err != nil {
		return nil, domain.Artifact{}, err
	}
	reader, metadata, err := m.artifacts.Open(ctx, artifactID)
	if err == nil && job.PluginID == "camera" {
		m.cameraReads[id]++
		return &cameraLegacyReader{ReadCloser: reader, manager: m, id: id}, metadata, nil
	}
	return reader, metadata, err
}
