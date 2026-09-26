package bench

import (
	"context"
	"errors"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/store/sqlite"
)

var publicTerminalCodes = map[string]struct{}{
	"device_not_ready": {}, "device_initializing": {}, "device_absent": {},
	"device_busy": {}, "unsupported_endpoint": {}, "unsupported_format": {},
	"control_out_of_range": {}, "control_unsupported": {}, "frame_timeout": {},
	"invalid_parameters": {}, "runtime_unavailable": {}, "device_not_found": {},
	"image_too_large": {}, "capture_failed": {},
}

func terminalErrorCode(err error) string {
	var cameraError cameraOperationError
	if errors.As(err, &cameraError) {
		return string(cameraError)
	}
	if errors.Is(err, errCameraStorage) {
		return "camera_storage_limit"
	}
	if errors.Is(err, ErrConflict) {
		return "configuration_changed"
	}
	if errors.Is(err, ErrBusy) {
		return "device_busy"
	}
	var remote *supervisor.RemoteExecutionError
	if errors.Is(err, supervisor.ErrRunnerRemote) && errors.As(err, &remote) {
		if _, allowed := publicTerminalCodes[remote.Code]; allowed {
			return remote.Code
		}
	}
	return "execution_failed"
}

func (m *Manager) schedule(ctx context.Context) {
	defer close(m.done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
		m.mu.Lock()
		if !m.closed {
			m.dispatch(ctx)
		}
		m.mu.Unlock()
	}
}

// dispatch holds admission's mutex. Slots remain occupied until Run has killed
// its child process and released leases, including after timeout/cancellation.
func (m *Manager) dispatch(ctx context.Context) {
	rows, err := m.db().QueryContext(ctx, `SELECT `+jobColumns+` FROM bench_jobs j JOIN sessions s ON s.id=j.session_id WHERE s.state IN ('pending_approval','queued') ORDER BY j.created_at,j.id`)
	if err != nil {
		m.report(err)
		return
	}
	jobs := []Job{}
	for rows.Next() {
		job, scanErr := scanJob(rows)
		if scanErr != nil {
			rows.Close()
			m.report(scanErr)
			return
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		m.report(err)
		return
	}
	for _, job := range jobs {
		if _, exists := m.active[job.ID]; exists {
			continue
		}
		if !m.now().Before(job.Deadline) {
			m.report(m.settle(ctx, job, "incomplete", "deadline_exceeded"))
			continue
		}
		if job.State != "queued" || len(m.active) >= 2 {
			continue
		}
		resource := resourceFor(job)
		if job.PluginID == "camera" && job.Submission.Operation != "workspace" && m.cameraBusy(job.Submission.DeviceID) {
			m.report(m.settle(ctx, job, "failed", "device_busy"))
			continue
		}
		busy := false
		for _, run := range m.active {
			if resource != "" && resource == run.resource {
				busy = true
			}
		}
		if busy {
			continue
		}
		p, err := m.loadPlugin(ctx, job.PluginID)
		if err != nil || !p.Enabled || !p.Installed {
			m.report(m.settle(ctx, job, "failed", "integration_unavailable"))
			continue
		}
		service, runner, err := m.operationService(job, p)
		if err != nil {
			m.report(m.settle(ctx, job, "failed", "integration_unavailable"))
			continue
		}
		ctx, cancel := context.WithDeadline(ctx, job.Deadline)
		m.active[job.ID] = execution{service: service, cancel: cancel, resource: resource}
		m.workers.Add(1)
		go func() {
			defer m.workers.Done()
			sessionID, _ := domain.ParseSessionID(job.SessionID)
			_, runErr := service.Run(ctx, sessionID)
			timeout := errors.Is(ctx.Err(), context.DeadlineExceeded)
			cancelled := errors.Is(ctx.Err(), context.Canceled)
			cancel()
			runner.cleanup()
			m.mu.Lock()
			defer m.mu.Unlock()
			// Use a bounded finalization context independent of the expired command.
			final, finish := context.WithTimeout(context.Background(), 5*time.Second)
			defer finish()
			current, readErr := m.job(final, job.ID)
			m.report(readErr)
			if readErr == nil {
				message := ""
				if runErr != nil {
					message = terminalErrorCode(runErr)
				}
				if timeout {
					message = "deadline_exceeded"
				}
				if cancelled {
					message = "cancelled"
				}
				if !terminal(current.State) {
					state := "incomplete"
					if cancelled {
						state = "cancelled"
					}
					m.report(m.settle(final, current, state, message))
				}
				if message != "" {
					_, saveErr := m.db().ExecContext(final, `UPDATE bench_jobs SET error=? WHERE id=?`, message, job.ID)
					m.report(saveErr)
				}
			}
			delete(m.active, job.ID)
			m.signal()
		}()
	}
}

// settle is reserved for work that has never acquired a hardware lease. Running
// operations always terminate through operation.Service's atomic lease release.
func (m *Manager) settle(ctx context.Context, job Job, state, reason string) error {
	tx, err := m.db().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := m.now().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET state=?,updated_at=?,terminal_at=? WHERE id=? AND state IN ('pending_approval','queued')`, state, now, now, job.SessionID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bench_jobs SET error=? WHERE id=?`, reason, job.ID); err != nil {
		return err
	}
	event, err := m.options.Audit.Compose(audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: audit.ActorKindSystem, Action: "bench.settled", SubjectType: "transaction", SubjectID: job.ID, Outcome: audit.OutcomeApplied, Details: map[string]string{"state": state, "reason": reason}})
	if err != nil {
		return err
	}
	if err = sqlite.NewAuditRepository(m.options.Database).AppendInTransaction(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Manager) settleQueued(ctx context.Context, pluginID, state, reason string) error {
	rows, err := m.db().QueryContext(ctx, `SELECT `+jobColumns+` FROM bench_jobs j JOIN sessions s ON s.id=j.session_id WHERE j.plugin_id=? AND s.state IN ('pending_approval','queued')`, pluginID)
	if err != nil {
		return err
	}
	jobs := []Job{}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if _, running := m.active[job.ID]; running {
			continue
		}
		if err = m.settle(ctx, job, state, reason); err != nil {
			return err
		}
	}
	return nil
}
