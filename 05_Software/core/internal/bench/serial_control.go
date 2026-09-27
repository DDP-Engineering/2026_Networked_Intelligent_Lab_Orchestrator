package bench

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
)

// SendSerial reserves a durable result before any byte can be transmitted.
// A pending reservation is deliberately returned as unknown after interruption.
func (m *Manager) SendSerial(ctx context.Context, actor identity.Actor, id string, input SerialSend) (SerialSendResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	payload, err := serialPayload(input)
	if err != nil {
		return SerialSendResult{}, err
	}
	hash := sha256.Sum256(payload)
	digest := hex.EncodeToString(hash[:])
	if prior, found, err := m.serialPreviousSend(ctx, actor, id, input, digest); found || err != nil {
		return prior, err
	}
	m.mu.Lock()
	s := m.serials[id]
	if s == nil || s.id != input.ConnectionID {
		m.mu.Unlock()
		return SerialSendResult{}, ErrNotFound
	}
	err = m.serialAuthorized(ctx, actor, s, true)
	m.mu.Unlock()
	if err != nil {
		return SerialSendResult{}, err
	}
	queued := s.queued.Add(int64(len(payload)))
	defer s.queued.Add(-int64(len(payload)))
	if queued > 64<<10 {
		return SerialSendResult{}, ErrQueueFull
	}
	release, err := m.serialTX(ctx, s)
	if err != nil {
		return SerialSendResult{}, err
	}
	defer release()
	m.mu.Lock()
	if err = m.serialAuthorized(ctx, actor, s, true); err != nil {
		m.mu.Unlock()
		return SerialSendResult{}, err
	}
	var oldDigest, oldJSON string
	err = m.db().QueryRowContext(ctx, `SELECT digest,result_json FROM serial_sends WHERE connection_id=? AND request_key=?`, s.id, input.IdempotencyKey).Scan(&oldDigest, &oldJSON)
	result := SerialSendResult{IdempotencyKey: input.IdempotencyKey, Status: "unknown"}
	if err == nil {
		m.mu.Unlock()
		if oldDigest != digest {
			return result, ErrConflict
		}
		err = json.Unmarshal([]byte(oldJSON), &result)
		return result, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		m.mu.Unlock()
		return result, err
	}
	raw, _ := json.Marshal(result)
	_, err = m.db().ExecContext(ctx, `INSERT INTO serial_sends(connection_id,request_key,digest,result_json) VALUES(?,?,?,?)`, s.id, input.IdempotencyKey, digest, string(raw))
	if err == nil {
		err = m.record(ctx, actor, "serial.send_requested", s.id, map[string]any{"bytes": len(payload), "request_id": input.IdempotencyKey})
	}
	m.mu.Unlock()
	if err != nil {
		return result, err
	}
	response, callErr := cameraCall(ctx, s.transport, "POST", "/send", map[string]string{"data_base64": base64.StdEncoding.EncodeToString(payload)})
	if callErr == nil {
		var reply struct {
			Status       string `json:"status"`
			BytesWritten int    `json:"bytes_written"`
			Error        string `json:"error,omitempty"`
		}
		if strict(response, &reply) == nil && reply.BytesWritten >= 0 && reply.BytesWritten <= len(payload) && (reply.Status == "sent" || reply.Status == "partial" || reply.Status == "failed" || reply.Status == "unknown") {
			result.Status = reply.Status
			result.BytesWritten = reply.BytesWritten
			if reply.Status != "sent" {
				result.Error = "serial_write_incomplete"
			}
		}
	}
	raw, _ = json.Marshal(result)
	_, err = m.db().ExecContext(context.WithoutCancel(ctx), `UPDATE serial_sends SET result_json=? WHERE connection_id=? AND request_key=?`, string(raw), s.id, input.IdempotencyKey)
	if err != nil {
		return SerialSendResult{IdempotencyKey: input.IdempotencyKey, Status: "unknown"}, err
	}
	_ = m.record(context.WithoutCancel(ctx), actor, "serial.send_finished", s.id, map[string]any{"bytes_written": result.BytesWritten, "status": result.Status, "request_id": input.IdempotencyKey})
	return result, nil
}

// SetSerialSignals changes only supported manual lines inside the same lease.
func (m *Manager) SetSerialSignals(ctx context.Context, actor identity.Actor, id string, input SerialSignals) (SerialStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	m.mu.Lock()
	s := m.serials[id]
	if s == nil || s.id != input.ConnectionID {
		m.mu.Unlock()
		return SerialStatus{}, ErrNotFound
	}
	m.mu.Unlock()
	release, err := m.serialTX(ctx, s)
	if err != nil {
		return SerialStatus{}, err
	}
	defer release()
	m.mu.Lock()
	if err := m.serialAuthorized(ctx, actor, s, true); err != nil {
		m.mu.Unlock()
		return SerialStatus{}, err
	}
	if input.RTS == nil && input.DTR == nil || input.RTS != nil && s.settings.FlowControl == "rtscts" {
		m.mu.Unlock()
		return SerialStatus{}, ErrInvalid
	}
	body := map[string]bool{}
	if input.RTS != nil {
		if s.signals["rts"] == nil {
			m.mu.Unlock()
			return SerialStatus{}, ErrUnavailable
		}
		body["rts"] = *input.RTS
	}
	if input.DTR != nil {
		if s.signals["dtr"] == nil {
			m.mu.Unlock()
			return SerialStatus{}, ErrUnavailable
		}
		body["dtr"] = *input.DTR
	}
	err = m.record(ctx, actor, "serial.signals", s.id, body)
	m.mu.Unlock()
	if err != nil {
		return SerialStatus{}, err
	}
	raw, err := cameraCall(ctx, s.transport, "POST", "/signals", body)
	if err != nil {
		return SerialStatus{}, err
	}
	var reply serialRunnerStatus
	if json.Unmarshal(raw, &reply) != nil {
		return SerialStatus{}, ErrUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.stopped || m.serials[id] != s {
		return SerialStatus{}, ErrUnavailable
	}
	s.signals = reply.Signals
	return m.serialStatus(s, actor), nil
}

// serialPreviousSend allows a retry to resolve an old reservation after restart.
// It never resumes a closed connection or grants its authority to another credential.
func (m *Manager) serialPreviousSend(ctx context.Context, actor identity.Actor, id string, input SerialSend, digest string) (SerialSendResult, bool, error) {
	var stored, raw, owner, kind, credential string
	err := m.db().QueryRowContext(ctx, `SELECT s.digest,s.result_json,c.owner_id,c.credential_kind,c.credential_id FROM serial_sends s JOIN serial_connections c ON c.id=s.connection_id WHERE c.id=? AND c.device_id=? AND s.request_key=?`, input.ConnectionID, id, input.IdempotencyKey).Scan(&stored, &raw, &owner, &kind, &credential)
	if errors.Is(err, sql.ErrNoRows) {
		return SerialSendResult{}, false, nil
	}
	if err != nil {
		return SerialSendResult{}, false, err
	}
	if !actor.Has(auth.PermissionBenchRead) || !actor.Has(auth.PermissionBenchExecute) || actor.User.ID != owner || string(actor.Credential) != kind || serialCredentialID(actor) != credential {
		return SerialSendResult{}, true, ErrForbidden
	}
	if err = m.serialCredentialValid(ctx, owner, kind, credential); err != nil {
		return SerialSendResult{}, true, err
	}
	if digest != stored {
		return SerialSendResult{}, true, ErrConflict
	}
	var result SerialSendResult
	err = json.Unmarshal([]byte(raw), &result)
	return result, true, err
}

// SerialLogging admits a future-only log or closes the current segment.
func (m *Manager) SerialLogging(ctx context.Context, actor identity.Actor, id string, input SerialAction) (SerialStatus, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return SerialStatus{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.serials[id]
	if s == nil || s.id != input.ConnectionID {
		return SerialStatus{}, ErrNotFound
	}
	if input.Action == "stop" {
		if actor.User.ID != s.owner && !actor.Has(auth.PermissionPluginsManage) {
			return SerialStatus{}, ErrForbidden
		}
		m.finishSerialLog(s, m.drainSerial(s) == nil)
		return m.serialStatus(s, actor), nil
	}
	if input.Action != "start" {
		return SerialStatus{}, ErrInvalid
	}
	if err := m.serialAuthorized(ctx, actor, s, true); err != nil {
		return SerialStatus{}, err
	}
	if s.log != nil {
		return m.serialStatus(s, actor), nil
	}
	if err := m.record(ctx, actor, "serial.logging_started", s.id, nil); err != nil {
		return SerialStatus{}, err
	}
	if err := m.startSerialLog(ctx, s); err != nil {
		s.warning = "logging_stopped"
		return SerialStatus{}, err
	}
	s.warning = ""
	return m.serialStatus(s, actor), nil
}

// serialTX permits cancellation while waiting for the single physical writer.
func (m *Manager) serialTX(ctx context.Context, s *serialSession) (func(), error) {
	m.mu.Lock()
	if s.txGate == nil {
		s.txGate = make(chan struct{}, 1)
	}
	gate := s.txGate
	m.mu.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
