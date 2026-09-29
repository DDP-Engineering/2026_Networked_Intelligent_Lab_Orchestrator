package bench

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

// serialSession fields are protected by Manager.mu except the TX queue/lock.
type serialSession struct {
	id, device, owner, credential, kind string
	revision                            int64
	settings                            SerialSettings
	expires, lastAccess                 time.Time
	idleTimeout                         time.Duration
	transport                           *cameraSession
	runnerDone                          chan struct{}
	stopped                             bool
	events                              []SerialEvent
	ringBytes                           int
	cursor, rx, tx                      int64
	warning                             string
	signals                             map[string]*bool
	log                                 *serialLogWriter
	txGate                              chan struct{}
	queued                              atomic.Int64
	claim                               *transportLease
}

func serialCredentialID(actor identity.Actor) string {
	if actor.Credential == identity.CredentialToken {
		return actor.TokenID
	}
	return actor.SessionID
}
func (m *Manager) serialCredentialValid(ctx context.Context, owner, kind, id string) error {
	repo := sqlite.NewIdentityRepository(m.options.Database)
	user, err := repo.UserByID(ctx, owner)
	if err != nil || !user.Enabled() || (!user.Role.Has(auth.PermissionBenchExecute) || !user.Role.Has(auth.PermissionBenchRead)) {
		return ErrForbidden
	}
	if id == "" {
		return ErrForbidden
	}
	switch kind {
	case string(identity.CredentialToken):
		token, err := repo.APITokenByID(ctx, id)
		if err != nil || token.UserID != owner || !token.Usable(m.now()) {
			return ErrForbidden
		}
		permissions := auth.EffectivePermissions(user.Role, token.Scopes)
		read, execute := false, false
		for _, permission := range permissions {
			read = read || permission == auth.PermissionBenchRead
			execute = execute || permission == auth.PermissionBenchExecute
		}
		if read && execute {
			return nil
		}
		return ErrForbidden
	case string(identity.CredentialSession):
		var expires string
		var revoked sql.NullString
		err = m.db().QueryRowContext(ctx, `SELECT expires_at,revoked_at FROM user_sessions WHERE id=? AND user_id=?`, id, owner).Scan(&expires, &revoked)
		end, e := time.Parse(time.RFC3339Nano, expires)
		if err != nil || e != nil || revoked.Valid || !m.now().Before(end) {
			return ErrForbidden
		}
		return nil
	default:
		return ErrForbidden
	}
}
func (m *Manager) serialBusy(id string) bool {
	for device, s := range m.serials {
		if (!s.stopped || (s.claim != nil && s.claim.held)) && (id == "serial" || id == device) {
			return true
		}
	}
	return false
}
func (m *Manager) serialAuthorized(ctx context.Context, actor identity.Actor, s *serialSession, control bool) error {
	if !actor.Has(auth.PermissionBenchRead) {
		return ErrForbidden
	}
	if actor.User.ID != s.owner && !actor.Has(auth.PermissionPluginsManage) {
		return ErrForbidden
	}
	if control && (!actor.Has(auth.PermissionBenchExecute) || actor.User.ID != s.owner || string(actor.Credential) != s.kind || serialCredentialID(actor) != s.credential) {
		return ErrForbidden
	}
	if s.stopped {
		return ErrUnavailable
	}
	if err := m.serialLeaseValid(ctx, s); err != nil {
		m.stopSerialLocked(s.device, "authorization_expired")
		return err
	}
	s.lastAccess = m.now()
	return nil
}
func (m *Manager) serialLeaseValid(ctx context.Context, s *serialSession) error {
	if !m.now().Before(s.expires) {
		return ErrForbidden
	}
	if err := m.serialCredentialValid(ctx, s.owner, s.kind, s.credential); err != nil {
		return err
	}
	var installed, enabled bool
	if err := m.db().QueryRowContext(ctx, `SELECT installed,enabled FROM bench_plugins WHERE id='serial'`).Scan(&installed, &enabled); err != nil || !installed || !enabled {
		return ErrUnavailable
	}
	r, err := managedRecord(ctx, m, "serial", s.device)
	if err != nil || !r.Enabled || !r.Configured || r.Revision != s.revision || r.Presence != "present" || r.Readiness != "ready" {
		return ErrUnavailable
	}
	return nil
}
func (m *Manager) serialStatus(s *serialSession, actor identity.Actor) SerialStatus {
	return SerialStatus{DeviceID: s.device, Connected: !s.stopped, ConnectionID: s.id, OwnerID: s.owner, ExpiresAt: s.expires.Format(time.RFC3339Nano), Settings: s.settings, Logging: s.log != nil, LogID: serialLogID(s), RXBytes: s.rx, TXBytes: s.tx, Warning: s.warning, Signals: s.signals, CanControl: !s.stopped && actor.Has(auth.PermissionBenchExecute) && actor.User.ID == s.owner && string(actor.Credential) == s.kind && serialCredentialID(actor) == s.credential}
}
func serialLogID(s *serialSession) string {
	if s.log != nil {
		return s.log.id
	}
	return ""
}

// ConnectSerial enters the ordinary Bench approval and durable idempotency path.
func (m *Manager) ConnectSerial(ctx context.Context, actor identity.Actor, id string, input SerialConnect) (Job, error) {
	raw, _ := json.Marshal(serialConnectParameters{input.Revision, input.Settings})
	return m.Submit(ctx, actor, Submission{PluginID: "serial", DeviceID: id, Operation: "connect", Parameters: raw, IdempotencyKey: input.IdempotencyKey, TimeoutSeconds: input.TimeoutSeconds})
}
func (m *Manager) executeSerialConnect(ctx context.Context, job Job, p Plugin) (protocol.ExecuteResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.serialBusy(job.Submission.DeviceID) {
		return protocol.ExecuteResult{}, ErrBusy
	}
	count := 0
	for _, s := range m.serials {
		if !s.stopped {
			count++
		}
	}
	if count >= m.settings.SerialConnections {
		return protocol.ExecuteResult{}, ErrBusy
	}
	var params serialConnectParameters
	if strict(job.Submission.Parameters, &params) != nil {
		return protocol.ExecuteResult{}, ErrInvalid
	}
	r, err := managedRecord(ctx, m, "serial", job.Submission.DeviceID)
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	if r.Revision != params.Revision || !r.Enabled || !r.Configured || r.Readiness != "ready" || r.Presence != "present" {
		return protocol.ExecuteResult{}, ErrConflict
	}
	var token, browser string
	if err = m.db().QueryRowContext(ctx, `SELECT credential_id,browser_session_id FROM bench_jobs WHERE id=?`, job.ID).Scan(&token, &browser); err != nil {
		return protocol.ExecuteResult{}, err
	}
	kind, credential := string(identity.CredentialSession), browser
	if token != "" {
		kind, credential = string(identity.CredentialToken), token
	}
	if err = m.serialCredentialValid(ctx, job.OwnerID, kind, credential); err != nil {
		return protocol.ExecuteResult{}, err
	}
	claim, err := m.claimTransportLocked("serial", r.ID, "", modbus.USBResourceKeys(managedUSBIdentity(r)))
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	runner, err := m.processConfigured(p, r.ID, 3*time.Second)
	if err != nil {
		m.finishTransportLocked(claim, nil)
		return protocol.ExecuteResult{}, err
	}
	id := uuid.NewString()
	root := filepath.Join(m.options.DataDir, "serial-workspaces", id)
	if err = os.MkdirAll(root, 0700); err != nil {
		shutdown, done := context.WithTimeout(context.Background(), 3*time.Second)
		cleanupErr := runner.Shutdown(shutdown)
		done()
		m.finishTransportLocked(claim, cleanupErr)
		m.report(cleanupErr)
		return protocol.ExecuteResult{}, err
	}
	lifetime := time.Duration(m.settings.SerialLifetimeMinutes) * time.Minute
	runCtx, cancel := context.WithDeadline(context.Background(), m.now().Add(lifetime))
	claim.cancel = cancel
	transport := &http.Transport{DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(c, "unix", filepath.Join(root, "control.sock"))
	}, MaxConnsPerHost: 3}
	s := &serialSession{id: id, device: r.ID, owner: job.OwnerID, kind: kind, credential: credential, revision: r.Revision, settings: params.Settings, expires: m.now().Add(lifetime), idleTimeout: time.Duration(m.settings.SerialIdleMinutes) * time.Minute, lastAccess: m.now(), runnerDone: make(chan struct{}), signals: map[string]*bool{}, events: []SerialEvent{}, transport: &cameraSession{cancel: cancel, client: &http.Client{Transport: transport, Timeout: 3 * time.Second}}}
	s.claim = claim
	rawSettings, _ := json.Marshal(params.Settings)
	_, err = m.db().ExecContext(ctx, `INSERT INTO serial_connections(id,device_id,owner_id,credential_kind,credential_id,revision,settings_json,started_at,expires_at,state) VALUES(?,?,?,?,?,?,?,?,?,'active')`, id, r.ID, s.owner, kind, credential, r.Revision, string(rawSettings), m.now().Format(time.RFC3339Nano), s.expires.Format(time.RFC3339Nano))
	if err != nil {
		cancel()
		shutdown, done := context.WithTimeout(context.Background(), 3*time.Second)
		cleanupErr := runner.Shutdown(shutdown)
		done()
		transport.CloseIdleConnections()
		cleanupErr = errors.Join(cleanupErr, os.RemoveAll(root))
		m.finishTransportLocked(claim, cleanupErr)
		m.report(cleanupErr)
		return protocol.ExecuteResult{}, err
	}
	m.serials[r.ID] = s
	raw, _ := json.Marshal(map[string]any{"workspace_directory": root, "hardware_identity": map[string]string{"vendor": r.Vendor, "product": r.Product, "serial": r.Serial, "port_path": r.PortPath}, "endpoint_id": r.EndpointID, "device_id": r.ID, "settings": params.Settings})
	m.serialWorkers.Add(1)
	go func() {
		defer m.serialWorkers.Done()
		defer close(s.runnerDone)
		defer cancel()
		_, runErr := runner.Execute(runCtx, protocol.ExecuteRequest{Type: "execute", Operation: "stream", Parameters: raw}, discardEvents{})
		shutdown, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		cleanupErr := runner.Shutdown(shutdown)
		transport.CloseIdleConnections()
		cleanupErr = errors.Join(cleanupErr, os.RemoveAll(root))
		m.finishTransport(claim, cleanupErr)
		if runErr != nil && runCtx.Err() == nil {
			m.report(runErr)
		}
	}()
	m.mu.Unlock()
	readyErr := m.waitSerialReady(ctx, s)
	m.mu.Lock()
	if readyErr != nil || s.stopped || m.closed || m.serials[r.ID] != s {
		if m.serials[r.ID] == s {
			m.stopSerialLocked(r.ID, "connect_failed")
		}
		if readyErr != nil {
			return protocol.ExecuteResult{}, readyErr
		}
		return protocol.ExecuteResult{}, ErrUnavailable
	}
	m.serialWorkers.Add(1)
	go m.monitorSerial(s)
	value, _ := json.Marshal(map[string]any{"connection_id": id, "expires_at": s.expires.Format(time.RFC3339Nano)})
	_, err = m.db().ExecContext(ctx, `UPDATE bench_jobs SET result_json=? WHERE id=?`, string(value), job.ID)
	return protocol.ExecuteResult{Status: "completed", Data: value}, err
}
func (m *Manager) stopSerialLocked(id, reason string) {
	for device, s := range m.serials {
		if s.stopped || (id != "serial" && id != device) {
			continue
		}
		completed := reason == "disconnected" || reason == "plugin_disabled" || reason == "device_disabled" || reason == "shutdown" || reason == "lease_expired" || reason == "idle_timeout"
		if s.log != nil && m.drainSerial(s) != nil {
			completed = false
		}
		s.stopped = true
		s.events = nil
		s.ringBytes = 0
		s.warning = reason
		m.finishSerialLog(s, completed)
		s.transport.cancel()
		_, err := m.db().ExecContext(context.Background(), `UPDATE serial_connections SET state='closed' WHERE id=?`, s.id)
		m.report(err)
	}
}
func (m *Manager) monitorSerial(s *serialSession) {
	defer m.serialWorkers.Done()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	lastCheck := time.Time{}
	for {
		select {
		case <-s.runnerDone:
			m.mu.Lock()
			if m.serials[s.device] == s && !s.stopped {
				m.markHardwareFailed(context.Background(), "serial", s.device)
				m.stopSerialLocked(s.device, "device_unavailable")
			}
			m.mu.Unlock()
			return
		case <-tick.C:
		}
		m.mu.Lock()
		if s.stopped || m.serials[s.device] != s {
			m.mu.Unlock()
			return
		}
		cursor := s.cursor
		refreshSignals := time.Since(lastCheck) >= time.Second
		if time.Since(lastCheck) >= time.Second {
			lastCheck = time.Now()
			reason := ""
			if !m.now().Before(s.expires) {
				reason = "lease_expired"
			} else if m.serialLeaseValid(context.Background(), s) != nil {
				reason = "authorization_expired"
			} else if s.log == nil && m.now().Sub(s.lastAccess) >= s.idleTimeout {
				reason = "idle_timeout"
			}
			if reason != "" {
				m.stopSerialLocked(s.device, reason)
				m.mu.Unlock()
				return
			}
		}
		m.mu.Unlock()
		readCtx, readCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		raw, err := cameraCall(readCtx, s.transport, "GET", "/read?after="+strconv.FormatInt(cursor, 10)+"&wait_ms=0", nil)
		readCancel()
		var status serialRunnerStatus
		statusValid := false
		if refreshSignals && err == nil {
			statusCtx, statusCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			statusRaw, statusErr := cameraCall(statusCtx, s.transport, "GET", "/status", nil)
			statusCancel()
			statusValid = statusErr == nil && json.Unmarshal(statusRaw, &status) == nil
		}
		m.mu.Lock()
		if statusValid {
			s.signals = status.Signals
			if status.Warning != "" {
				s.warning = status.Warning
			}
		}
		if s.stopped || m.serials[s.device] != s {
			m.mu.Unlock()
			return
		}
		if err != nil {
			m.markHardwareFailed(context.Background(), "serial", s.device)
			m.stopSerialLocked(s.device, "device_unavailable")
			m.mu.Unlock()
			return
		}
		if err = m.ingestSerial(s, raw); err != nil {
			m.stopSerialLocked(s.device, "invalid_stream")
		}

		m.mu.Unlock()
	}
}

// SerialWorkspace reads an owner/admin view without attaching another controller.
func (m *Manager) SerialWorkspace(ctx context.Context, actor identity.Actor, id string) (SerialStatus, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return SerialStatus{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := managedRecord(ctx, m, "serial", id); err != nil {
		return SerialStatus{}, err
	}
	s := m.serials[id]
	if s == nil {
		return SerialStatus{DeviceID: id, Signals: map[string]*bool{}}, nil
	}
	if actor.User.ID != s.owner && !actor.Has(auth.PermissionPluginsManage) {
		return SerialStatus{}, ErrForbidden
	}
	return m.serialStatus(s, actor), nil
}

// ReadSerial waits for a bounded page; authenticated reads renew idle attachment.
func (m *Manager) ReadSerial(ctx context.Context, actor identity.Actor, id, connection string, after int64, waitMS int) (SerialRead, error) {
	if after < 0 || waitMS < 0 || waitMS > 10000 {
		return SerialRead{}, ErrInvalid
	}
	deadline := time.Now().Add(time.Duration(waitMS) * time.Millisecond)
	for {
		m.mu.Lock()
		s := m.serials[id]
		if s == nil || s.id != connection {
			m.mu.Unlock()
			return SerialRead{}, ErrNotFound
		}
		if err := m.serialAuthorized(ctx, actor, s, false); err != nil {
			m.mu.Unlock()
			return SerialRead{}, err
		}
		page := serialPage(s.events, after)
		m.mu.Unlock()
		if len(page.Events) > 0 || page.Gap || !time.Now().Before(deadline) {
			return page, nil
		}
		select {
		case <-ctx.Done():
			return SerialRead{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func serialPage(events []SerialEvent, after int64) SerialRead {
	p := SerialRead{Events: []SerialEvent{}, NextCursor: after}
	if len(events) > 0 && after < events[0].Sequence-1 {
		p.Gap = true
	}
	size := 128
	for _, e := range events {
		if e.Sequence <= after {
			continue
		}
		raw, _ := json.Marshal(e)
		if size+len(raw) > 64<<10 {
			break
		}
		if e.Sequence > p.NextCursor+1 {
			p.Gap = true
		}
		p.Events = append(p.Events, e)
		p.NextCursor = e.Sequence
		size += len(raw) + 1
	}
	return p
}

// DisconnectSerial is an unconditional owner/admin stop, independent of operation policy.
func (m *Manager) DisconnectSerial(ctx context.Context, actor identity.Actor, id, connection string) error {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.serials[id]
	if s == nil || s.id != connection {
		return ErrNotFound
	}
	if actor.User.ID != s.owner && !actor.Has(auth.PermissionPluginsManage) {
		return ErrForbidden
	}
	if err := m.record(ctx, actor, "serial.disconnect", connection, nil); err != nil {
		return err
	}
	m.stopSerialLocked(id, "disconnected")
	return nil
}
