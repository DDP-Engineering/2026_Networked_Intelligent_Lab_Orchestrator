package bench

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/store/sqlite"
)

// modbusSession fields are guarded by Manager.mu except the command gate.
type modbusSession struct {
	id, endpointID, profileID, owner, kind, credential string
	workspaceID                                        string
	workspaceRevision                                  int64
	endpointRevision, profileRevision                  int64
	expires, lastAccess                                time.Time
	idleAfter                                          time.Duration
	socket                                             string
	cancel                                             context.CancelFunc
	done                                               chan struct{}
	claim                                              *transportLease
	stopped                                            bool
	reason                                             string
	commandGate                                        sync.Mutex
	groups                                             []ModbusPollGroup
	groupStats                                         map[string]*ModbusGroupStats
	pollPaused, displayPaused                          bool
	events                                             []ModbusEvent
	cursor                                             int64
	recording                                          *modbusRecorder
	lastRecordingID, recordingWarning                  string
}

// ModbusStatus describes one credential-bound, pinned live endpoint.
type ModbusStatus struct {
	SessionID         string                      `json:"session_id"`
	State             string                      `json:"state"`
	EndpointID        string                      `json:"endpoint_id"`
	ProfileID         string                      `json:"profile_id"`
	OwnerID           string                      `json:"owner_id"`
	EndpointRevision  int64                       `json:"endpoint_revision"`
	ProfileRevision   int64                       `json:"profile_revision"`
	WorkspaceID       string                      `json:"workspace_id,omitempty"`
	WorkspaceRevision int64                       `json:"workspace_revision,omitempty"`
	Connected         bool                        `json:"connected"`
	ExpiresAt         string                      `json:"expires_at"`
	Reason            string                      `json:"reason,omitempty"`
	PollPaused        bool                        `json:"poll_paused"`
	DisplayPaused     bool                        `json:"display_paused"`
	CanControl        bool                        `json:"can_control"`
	GroupStats        map[string]ModbusGroupStats `json:"group_stats"`
	RecordingID       string                      `json:"recording_id,omitempty"`
	Logging           bool                        `json:"logging"`
	LoggingMode       string                      `json:"logging_mode,omitempty"`
	LoggingSampling   string                      `json:"logging_sampling,omitempty"`
	RecordingWarning  string                      `json:"recording_warning,omitempty"`
}

// ModbusGroupStats reports bounded live scheduling observations for one admitted group.
type ModbusGroupStats struct {
	Paused              bool   `json:"paused"`
	RequestedIntervalMS int    `json:"requested_interval_ms"`
	ActualIntervalMS    int64  `json:"actual_interval_ms"`
	LatencyMS           int64  `json:"latency_ms"`
	MissedPolls         int64  `json:"missed_polls"`
	Errors              int64  `json:"errors"`
	Samples             int64  `json:"samples"`
	LastSampleAt        string `json:"last_sample_at,omitempty"`
	lastCompleted       time.Time
}

func (m *Manager) modbusStatus(s *modbusSession, actor identity.Actor) ModbusStatus {
	state := "connected"
	if s.stopped {
		state = "closing"
	}
	stats := make(map[string]ModbusGroupStats, len(s.groupStats))
	for id, value := range s.groupStats {
		stats[id] = *value
	}
	status := ModbusStatus{SessionID: s.id, State: state, EndpointID: s.endpointID, ProfileID: s.profileID, OwnerID: s.owner, EndpointRevision: s.endpointRevision, ProfileRevision: s.profileRevision, WorkspaceID: s.workspaceID, WorkspaceRevision: s.workspaceRevision, Connected: !s.stopped, ExpiresAt: s.expires.Format(time.RFC3339Nano), Reason: s.reason, PollPaused: s.pollPaused, DisplayPaused: s.displayPaused, CanControl: !s.stopped && actor.User.ID == s.owner && string(actor.Credential) == s.kind && serialCredentialID(actor) == s.credential && actor.Has(auth.PermissionBenchExecute), GroupStats: stats, RecordingID: s.lastRecordingID, RecordingWarning: s.recordingWarning}
	if s.recording != nil {
		status.RecordingID, status.Logging = s.recording.meta.ID, true
		status.LoggingMode, status.LoggingSampling = s.recording.meta.Mode, s.recording.meta.Sampling
	}
	return status
}

func (m *Manager) modbusLeaseValid(ctx context.Context, s *modbusSession) error {
	if !m.now().Before(s.expires) {
		return ErrForbidden
	}
	if err := m.serialCredentialValid(ctx, s.owner, s.kind, s.credential); err != nil {
		return err
	}
	var installed, enabled bool
	if err := m.db().QueryRowContext(ctx, `SELECT installed,enabled FROM bench_plugins WHERE id='modbus'`).Scan(&installed, &enabled); err != nil || !installed || !enabled {
		return ErrUnavailable
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	e, err := r.Endpoint(ctx, s.endpointID)
	if err != nil || e.Revision != s.endpointRevision || !e.Enabled || e.Readiness == "fault" || e.Transport == "rtu" && (e.Presence != "present" || e.Readiness != "ready") {
		return ErrUnavailable
	}
	p, err := r.Profile(ctx, s.profileID)
	if err != nil || p.Revision != s.profileRevision || p.EndpointID != e.ID {
		return ErrUnavailable
	}
	return nil
}

func (m *Manager) modbusAuthorized(ctx context.Context, actor identity.Actor, s *modbusSession, control bool) error {
	if !actor.Has(auth.PermissionBenchRead) || actor.User.ID != s.owner && !actor.Has(auth.PermissionPluginsManage) {
		return ErrForbidden
	}
	if control && (!actor.Has(auth.PermissionBenchExecute) || actor.User.ID != s.owner || string(actor.Credential) != s.kind || serialCredentialID(actor) != s.credential) {
		return ErrForbidden
	}
	if s.stopped {
		return ErrUnavailable
	}
	if err := m.modbusLeaseValid(ctx, s); err != nil {
		m.stopModbusLocked(s, "authorization_expired")
		return err
	}
	s.lastAccess = m.now()
	return nil
}

// ConnectModbus creates only the finite policy job. Its completion opens the live lease.
func (m *Manager) ConnectModbus(ctx context.Context, actor identity.Actor, endpointID string, input ModbusConnect) (Job, error) {
	raw, _ := json.Marshal(modbusConnectParameters{EndpointRevision: input.EndpointRevision, ProfileID: input.ProfileID, ProfileRevision: input.ProfileRevision, WorkspaceID: input.WorkspaceID, WorkspaceRevision: input.WorkspaceRevision, Groups: input.Groups})
	return m.Submit(ctx, actor, Submission{PluginID: "modbus", DeviceID: endpointID, Operation: "connect", Parameters: raw, IdempotencyKey: input.IdempotencyKey, TimeoutSeconds: input.TimeoutSeconds})
}

// TestModbusConnection submits a finite, read-free connect job through the normal policy.
func (m *Manager) TestModbusConnection(ctx context.Context, actor identity.Actor, endpointID string, input ModbusConnect) (Job, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return Job{}, err
	}
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return Job{}, err
	}
	if len(input.Groups) != 0 || input.TimeoutSeconds > 60 || input.TimeoutSeconds < 0 {
		return Job{}, ErrInvalid
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = 30
	}
	raw, _ := json.Marshal(modbusConnectParameters{EndpointRevision: input.EndpointRevision, ProfileID: input.ProfileID, ProfileRevision: input.ProfileRevision, TestOnly: true})
	return m.Submit(ctx, actor, Submission{PluginID: "modbus", DeviceID: endpointID, Operation: "connect", Parameters: raw, IdempotencyKey: input.IdempotencyKey, TimeoutSeconds: input.TimeoutSeconds})
}

func (m *Manager) executeModbusConnect(ctx context.Context, job Job, p Plugin) (protocol.ExecuteResult, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, ErrUnavailable
	}
	var parameters modbusConnectParameters
	if strict(job.Submission.Parameters, &parameters) != nil || validateModbusPollGroups(parameters.Groups) != nil {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, ErrInvalid
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	e, err := r.Endpoint(ctx, job.Submission.DeviceID)
	if err != nil {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, modbusDefinitionError(err)
	}
	profile, err := r.Profile(ctx, parameters.ProfileID)
	if err != nil {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, modbusDefinitionError(err)
	}
	if e.Revision != parameters.EndpointRevision || profile.Revision != parameters.ProfileRevision || profile.EndpointID != e.ID {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, ErrConflict
	}
	if !e.Enabled || e.Readiness == "fault" || e.Transport == "rtu" && (e.Presence != "present" || e.Readiness != "ready") {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, ErrUnavailable
	}
	var tty string
	if e.Transport == "rtu" {
		record, recordErr := managedRecord(ctx, m, "modbus", e.Binding)
		if recordErr != nil || record.EndpointID == "" {
			m.mu.Unlock()
			return protocol.ExecuteResult{}, ErrUnavailable
		}
		tty = record.EndpointID
	}
	if len(m.modbusSessions) >= m.modbusSettings.MaxSessions {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, ErrBusy
	}
	var token, browser string
	if err = m.db().QueryRowContext(ctx, `SELECT credential_id,browser_session_id FROM bench_jobs WHERE id=?`, job.ID).Scan(&token, &browser); err != nil {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, err
	}
	kind, credential := string(identity.CredentialSession), browser
	if token != "" {
		kind, credential = string(identity.CredentialToken), token
	}
	if err = m.serialCredentialValid(ctx, job.OwnerID, kind, credential); err != nil {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, err
	}
	claim, err := m.claimTransportLocked("modbus", e.ID, profile.ID, e.ResourceKeys())
	if err != nil {
		m.mu.Unlock()
		return protocol.ExecuteResult{}, err
	}
	claim.endpointRevision = e.Revision
	runner, err := m.processConfigured(p, "", 3*time.Second)
	if err != nil {
		m.finishTransportLocked(claim, nil)
		m.mu.Unlock()
		return protocol.ExecuteResult{}, err
	}
	id := uuid.NewString()
	root := filepath.Join(m.options.DataDir, "modbus-workspaces", id)
	if err = os.MkdirAll(root, 0700); err != nil {
		m.finishTransportLocked(claim, nil)
		m.mu.Unlock()
		_ = runner.Shutdown(context.Background())
		return protocol.ExecuteResult{}, err
	}
	expires := m.now().Add(time.Duration(m.modbusSettings.SessionLeaseSeconds) * time.Second)
	runCtx, cancel := context.WithDeadline(context.Background(), expires)
	claim.cancel = cancel
	s := &modbusSession{id: id, endpointID: e.ID, profileID: profile.ID, workspaceID: parameters.WorkspaceID, workspaceRevision: parameters.WorkspaceRevision, owner: job.OwnerID, kind: kind, credential: credential, endpointRevision: e.Revision, profileRevision: profile.Revision, expires: expires, idleAfter: time.Duration(m.modbusSettings.IdleSeconds) * time.Second, lastAccess: m.now(), socket: filepath.Join(root, "control.sock"), cancel: cancel, done: make(chan struct{}), claim: claim, events: []ModbusEvent{}, groups: parameters.Groups, groupStats: map[string]*ModbusGroupStats{}}
	for _, group := range parameters.Groups {
		s.groupStats[group.ID] = &ModbusGroupStats{RequestedIntervalMS: group.IntervalMS}
	}
	_, err = m.db().ExecContext(ctx, `INSERT INTO modbus_sessions(id,endpoint_id,endpoint_revision,profile_id,profile_revision,owner_id,credential_kind,credential_id,started_at,expires_at,state) VALUES(?,?,?,?,?,?,?,?,?,?,'active')`, s.id, e.ID, e.Revision, profile.ID, profile.Revision, s.owner, kind, credential, m.now().Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano))
	if err != nil {
		m.finishTransportLocked(claim, nil)
		m.mu.Unlock()
		cancel()
		_ = runner.Shutdown(context.Background())
		_ = os.RemoveAll(root)
		return protocol.ExecuteResult{}, err
	}
	m.modbusSessions[e.ID] = s
	params := map[string]any{"workspace_directory": root, "endpoint": e, "profile": profile}
	if e.Transport == "rtu" {
		params["tty"] = tty
	}
	raw, _ := json.Marshal(params)
	m.modbusWorkers.Add(1)
	go func() {
		defer m.modbusWorkers.Done()
		defer close(s.done)
		defer cancel()
		_, runErr := runner.Execute(runCtx, protocol.ExecuteRequest{Type: "execute", Operation: "serve", Parameters: raw}, discardEvents{})
		shutdown, finish := context.WithTimeout(context.Background(), 3*time.Second)
		cleanupErr := runner.Shutdown(shutdown)
		finish()
		cleanupErr = errors.Join(cleanupErr, os.RemoveAll(root))
		m.finishTransport(claim, cleanupErr)
		m.mu.Lock()
		m.stopModbusLocked(s, "runner_stopped")
		if m.modbusSessions[s.endpointID] == s {
			delete(m.modbusSessions, s.endpointID)
		}
		m.mu.Unlock()
		if runErr != nil && runCtx.Err() == nil {
			m.report(runErr)
		}
	}()
	m.mu.Unlock()
	readyErr := waitModbusReady(ctx, s)
	m.mu.Lock()
	if readyErr != nil || s.stopped || m.closed {
		m.stopModbusLocked(s, "connect_failed")
		m.mu.Unlock()
		if readyErr != nil {
			return protocol.ExecuteResult{}, readyErr
		}
		return protocol.ExecuteResult{}, ErrUnavailable
	}
	if parameters.TestOnly {
		m.stopModbusLocked(s, "test_completed")
		m.mu.Unlock()
		select {
		case <-s.done:
			value, _ := json.Marshal(map[string]any{"connection_test": "ready"})
			return protocol.ExecuteResult{Status: "completed", Data: value}, nil
		case <-ctx.Done():
			return protocol.ExecuteResult{}, ctx.Err()
		}
	}
	m.modbusWorkers.Add(1)
	go m.monitorModbus(s)
	m.modbusWorkers.Add(1)
	go m.pollModbus(s)
	value, _ := json.Marshal(map[string]any{"session_id": s.id, "expires_at": s.expires.Format(time.RFC3339Nano)})
	_, err = m.db().ExecContext(ctx, `UPDATE bench_jobs SET result_json=? WHERE id=?`, string(value), job.ID)
	m.mu.Unlock()
	return protocol.ExecuteResult{Status: "completed", Data: value}, err
}

func waitModbusReady(ctx context.Context, s *modbusSession) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := net.DialTimeout("unix", s.socket, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return ErrUnavailable
		case <-ticker.C:
		}
	}
}

func (m *Manager) stopModbusLocked(s *modbusSession, reason string) {
	if s.stopped {
		return
	}
	s.stopped, s.reason = true, reason
	if s.recording != nil {
		recordingReason := ""
		if reason != "disconnected" {
			recordingReason = "session_" + reason
		}
		_, recordingErr := m.stopModbusRecordingLocked(s, recordingReason)
		m.report(recordingErr)
	}
	s.cancel()
	_, err := m.db().ExecContext(context.Background(), `UPDATE modbus_sessions SET state='closed',reason=? WHERE id=?`, reason, s.id)
	m.report(err)
}

func (m *Manager) stopAllModbusLocked(reason string) {
	for _, s := range m.modbusSessions {
		m.stopModbusLocked(s, reason)
	}
}

func (m *Manager) monitorModbus(s *modbusSession) {
	defer m.modbusWorkers.Done()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.done:
			m.mu.Lock()
			m.stopModbusLocked(s, "runner_stopped")
			m.mu.Unlock()
			return
		case <-tick.C:
		}
		m.mu.Lock()
		if s.stopped {
			m.mu.Unlock()
			return
		}
		if err := m.modbusLeaseValid(context.Background(), s); err != nil {
			m.stopModbusLocked(s, "authorization_expired")
			m.mu.Unlock()
			return
		}
		if m.now().Sub(s.lastAccess) >= s.idleAfter {
			m.stopModbusLocked(s, "idle_timeout")
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()
	}
}

// ModbusSession returns an owner/admin view without creating another controller.
func (m *Manager) ModbusSession(ctx context.Context, actor identity.Actor, endpointID string) (ModbusStatus, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return ModbusStatus{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil {
		var latest ModbusStatus
		var expires string
		err := m.db().QueryRowContext(ctx, `SELECT id,endpoint_id,profile_id,owner_id,endpoint_revision,profile_revision,expires_at,state,reason FROM modbus_sessions WHERE endpoint_id=? ORDER BY started_at DESC,id DESC LIMIT 1`, endpointID).Scan(&latest.SessionID, &latest.EndpointID, &latest.ProfileID, &latest.OwnerID, &latest.EndpointRevision, &latest.ProfileRevision, &expires, &latest.State, &latest.Reason)
		if errors.Is(err, sql.ErrNoRows) {
			return ModbusStatus{EndpointID: endpointID, State: "disconnected"}, nil
		}
		if err != nil {
			return ModbusStatus{}, err
		}
		if actor.User.ID != latest.OwnerID && !actor.Has(auth.PermissionPluginsManage) {
			return ModbusStatus{}, ErrForbidden
		}
		latest.ExpiresAt = expires
		return latest, nil
	}
	if s.stopped {
		if actor.User.ID != s.owner && !actor.Has(auth.PermissionPluginsManage) {
			return ModbusStatus{}, ErrForbidden
		}
		return m.modbusStatus(s, actor), nil
	}
	if err := m.modbusAuthorized(ctx, actor, s, false); err != nil {
		return ModbusStatus{}, err
	}
	return m.modbusStatus(s, actor), nil
}

// DisconnectModbus stops an owned session; administrators may stop another user's session.
func (m *Manager) DisconnectModbus(ctx context.Context, actor identity.Actor, endpointID, sessionID string) error {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID {
		return ErrNotFound
	}
	if actor.User.ID != s.owner && !actor.Has(auth.PermissionPluginsManage) {
		return ErrForbidden
	}
	if err := m.record(ctx, actor, "modbus.disconnect", s.id, nil); err != nil {
		return err
	}
	m.stopModbusLocked(s, "disconnected")
	return nil
}

func modbusPrivateCall(ctx context.Context, s *modbusSession, request ModbusRequest, pollKey string) (ModbusResult, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(60 * time.Second)
	}
	deadlineMS := int(time.Until(deadline).Milliseconds())
	if deadlineMS < 1 {
		return ModbusResult{}, context.DeadlineExceeded
	}
	if deadlineMS > 60000 {
		deadlineMS = 60000
	}
	requestID := uuid.NewString()
	values := make([]any, len(request.Values))
	for i, value := range request.Values {
		if request.Function == 5 || request.Function == 15 {
			values[i] = value == 1
		} else {
			values[i] = value
		}
	}
	message := map[string]any{"version": 1, "kind": "transaction", "request_id": requestID, "function": request.Function, "unit": request.Unit, "address": request.Address, "count": request.Count, "values": values, "deadline_ms": deadlineMS}
	if pollKey != "" {
		message["poll_key"] = pollKey
	}
	raw, _ := json.Marshal(message)
	if len(raw) > 8191 {
		return ModbusResult{}, ErrInvalid
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", s.socket)
	if err != nil {
		return ModbusResult{}, modbusCallError(ctx, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)
	if _, err = io.Copy(conn, bytes.NewReader(append(raw, '\n'))); err != nil {
		return ModbusResult{}, modbusCallError(ctx, err)
	}
	line, err := bufio.NewReader(io.LimitReader(conn, 65537)).ReadBytes('\n')
	if err != nil || len(line) > 65536 {
		return ModbusResult{}, modbusCallError(ctx, err)
	}
	var result ModbusResult
	if strict(line, &result) != nil || result.RequestID != requestID || !validModbusResult(request, result) {
		return ModbusResult{}, ErrUnavailable
	}
	return result, nil
}

func modbusCallError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return context.DeadlineExceeded
	}
	return ErrUnavailable
}

// ModbusResult preserves raw booleans/register values and the runner's conservative outcome.
type ModbusResult struct {
	RequestID     string             `json:"request_id"`
	Outcome       string             `json:"outcome"`
	Values        []json.RawMessage  `json:"values"`
	Attempts      int                `json:"attempts"`
	ExceptionCode int                `json:"exception_code,omitempty"`
	Frames        []ModbusTraceFrame `json:"frames,omitempty"`
}

// ModbusTraceFrame is a bounded byte buffer captured by the runner's transport hook.
type ModbusTraceFrame struct {
	RequestID string  `json:"request_id"`
	Direction string  `json:"direction"`
	Sequence  int     `json:"sequence"`
	Kind      string  `json:"kind"`
	DataHex   string  `json:"data_hex"`
	ElapsedMS float64 `json:"elapsed_ms"`
}

func validModbusOutcome(outcome string) bool {
	switch outcome {
	case "acknowledged", "exception", "timeout", "cancelled", "protocol_error", "rejected", "queue_full", "disconnected", "delivery_unknown", "coalesced":
		return true
	default:
		return false
	}
}

// validModbusResult prevents a compromised or incompatible runner from
// reporting success for a wrong quantity, type or write echo.
func validModbusResult(request ModbusRequest, result ModbusResult) bool {
	if !validModbusOutcome(result.Outcome) || result.Attempts < 0 || result.Attempts > 4 || request.Function >= 5 && result.Attempts > 1 {
		return false
	}
	if len(result.Frames) > 64*4 {
		return false
	}
	traceBytes := 0
	for _, frame := range result.Frames {
		if frame.RequestID != result.RequestID || frame.Direction != "tx" && frame.Direction != "rx" || len(frame.DataHex)%2 != 0 || len(frame.DataHex) > 49152 {
			return false
		}
		if _, err := hex.DecodeString(frame.DataHex); err != nil {
			return false
		}
		traceBytes += len(frame.DataHex) / 2
		if traceBytes > 4*6000 {
			return false
		}
	}
	if result.Outcome != "acknowledged" {
		return len(result.Values) == 0 && (result.Outcome != "exception" || result.ExceptionCode >= 1 && result.ExceptionCode <= 255)
	}
	if result.Attempts < 1 || len(result.Values) != request.Count || result.ExceptionCode != 0 {
		return false
	}
	for i, raw := range result.Values {
		if request.Function == 1 || request.Function == 2 || request.Function == 5 || request.Function == 15 {
			var value bool
			if json.Unmarshal(raw, &value) != nil || request.Function >= 5 && value != (request.Values[i] == 1) {
				return false
			}
		} else {
			var value int
			if json.Unmarshal(raw, &value) != nil || value < 0 || value > 65535 || request.Function >= 5 && value != request.Values[i] {
				return false
			}
		}
	}
	return true
}

// ReadModbus executes one validated read against the pinned live endpoint.
func (m *Manager) ReadModbus(ctx context.Context, actor identity.Actor, endpointID, sessionID string, request ModbusRequest) (ModbusResult, error) {
	request, err := canonicalModbusRequest(request)
	if err != nil || request.Function > 4 {
		return ModbusResult{}, ErrInvalid
	}
	m.mu.Lock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID {
		m.mu.Unlock()
		return ModbusResult{}, ErrNotFound
	}
	if err = m.modbusAuthorized(ctx, actor, s, true); err == nil {
		e, eerr := sqlite.NewModbusRepository(m.options.Database).Endpoint(ctx, endpointID)
		if eerr != nil || validateModbusTarget(request, e) != nil || !modbusRequestInPlan(s.groups, request) {
			err = ErrInvalid
		}
	}
	m.mu.Unlock()
	if err != nil {
		return ModbusResult{}, err
	}
	result, callErr := modbusPrivateCall(ctx, s, request, "")
	if callErr == nil {
		m.mu.Lock()
		if m.modbusSessions[endpointID] == s && !s.stopped {
			m.report(m.recordModbusObservationLocked(context.Background(), s, "manual", request, result))
		}
		m.mu.Unlock()
	}
	return result, callErr
}
