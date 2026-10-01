package bench

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

// ModbusWrite requests one exact raw FC05/06/15/16 action inside a live lease.
type ModbusWrite struct {
	SessionID      string        `json:"session_id"`
	IdempotencyKey string        `json:"idempotency_key"`
	Request        ModbusRequest `json:"request"`
}

// ModbusCommand is a durable, idempotent write outcome. Unknown delivery is terminal.
type ModbusCommand struct {
	ID             string       `json:"command_id"`
	SessionID      string       `json:"session_id"`
	IdempotencyKey string       `json:"idempotency_key"`
	Digest         string       `json:"digest"`
	Outcome        string       `json:"outcome"`
	Result         ModbusResult `json:"result"`
}

// ModbusCommandPage is a bounded durable history page for one session.
type ModbusCommandPage struct {
	Commands   []ModbusCommand `json:"commands"`
	NextOffset int             `json:"next_offset"`
}

// ModbusCommand reads one durable write result for its owner or administrator.
func (m *Manager) ModbusCommand(ctx context.Context, actor identity.Actor, id string) (ModbusCommand, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return ModbusCommand{}, err
	}
	var raw, owner string
	err := m.db().QueryRowContext(ctx, `SELECT c.result_json,s.owner_id FROM modbus_commands c JOIN modbus_sessions s ON s.id=c.session_id WHERE c.id=?`, id).Scan(&raw, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ModbusCommand{}, ErrNotFound
	}
	if err != nil {
		return ModbusCommand{}, err
	}
	if actor.User.ID != owner && !actor.Has(auth.PermissionPluginsManage) {
		return ModbusCommand{}, ErrForbidden
	}
	var result ModbusCommand
	if json.Unmarshal([]byte(raw), &result) != nil {
		return ModbusCommand{}, ErrUnavailable
	}
	return result, nil
}

// ListModbusCommands returns one finite history page without touching a runner.
func (m *Manager) ListModbusCommands(ctx context.Context, actor identity.Actor, endpointID, sessionID string, offset int) (ModbusCommandPage, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return ModbusCommandPage{}, err
	}
	if offset < 0 || offset > 100000 {
		return ModbusCommandPage{}, ErrInvalid
	}
	var owner string
	err := m.db().QueryRowContext(ctx, `SELECT owner_id FROM modbus_sessions WHERE id=? AND endpoint_id=?`, sessionID, endpointID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ModbusCommandPage{}, ErrNotFound
	}
	if err != nil {
		return ModbusCommandPage{}, err
	}
	if actor.User.ID != owner && !actor.Has(auth.PermissionPluginsManage) {
		return ModbusCommandPage{}, ErrForbidden
	}
	rows, err := m.db().QueryContext(ctx, `SELECT result_json FROM modbus_commands WHERE session_id=? ORDER BY created_at DESC,id DESC LIMIT 100 OFFSET ?`, sessionID, offset)
	if err != nil {
		return ModbusCommandPage{}, err
	}
	defer rows.Close()
	page := ModbusCommandPage{Commands: []ModbusCommand{}, NextOffset: offset}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return ModbusCommandPage{}, err
		}
		var command ModbusCommand
		if json.Unmarshal([]byte(raw), &command) != nil {
			return ModbusCommandPage{}, ErrUnavailable
		}
		page.Commands = append(page.Commands, command)
		page.NextOffset++
	}
	return page, rows.Err()
}

// ModbusWritePreview exposes the exact PDU and immutable digest before admission.
type ModbusWritePreview struct {
	SessionID        string        `json:"session_id"`
	EndpointRevision int64         `json:"endpoint_revision"`
	ProfileRevision  int64         `json:"profile_revision"`
	Request          ModbusRequest `json:"request"`
	PDUHex           string        `json:"pdu_hex"`
	Digest           string        `json:"digest"`
}

func modbusPDU(request ModbusRequest) ([]byte, error) {
	request, err := canonicalModbusRequest(request)
	if err != nil {
		return nil, err
	}
	pdu := make([]byte, 5)
	pdu[0] = byte(request.Function)
	binary.BigEndian.PutUint16(pdu[1:3], uint16(request.Address))
	if request.Function == 5 {
		if request.Values[0] == 1 {
			binary.BigEndian.PutUint16(pdu[3:5], 0xff00)
		}
		return pdu, nil
	}
	if request.Function == 6 {
		binary.BigEndian.PutUint16(pdu[3:5], uint16(request.Values[0]))
		return pdu, nil
	}
	if request.Function == 22 {
		pdu = append(pdu, 0, 0)
		binary.BigEndian.PutUint16(pdu[3:5], uint16(*request.AndMask))
		binary.BigEndian.PutUint16(pdu[5:7], uint16(*request.OrMask))
		return pdu, nil
	}
	if request.Function == 23 {
		pdu = make([]byte, 10, 10+request.Count*2)
		pdu[0] = 23
		binary.BigEndian.PutUint16(pdu[1:3], uint16(*request.ReadAddress))
		binary.BigEndian.PutUint16(pdu[3:5], uint16(request.ReadCount))
		binary.BigEndian.PutUint16(pdu[5:7], uint16(request.Address))
		binary.BigEndian.PutUint16(pdu[7:9], uint16(request.Count))
		pdu[9] = byte(request.Count * 2)
		for _, value := range request.Values {
			pdu = binary.BigEndian.AppendUint16(pdu, uint16(value))
		}
		return pdu, nil
	}
	binary.BigEndian.PutUint16(pdu[3:5], uint16(request.Count))
	if request.Function == 15 {
		payload := make([]byte, (request.Count+7)/8)
		for i, value := range request.Values {
			payload[i/8] |= byte(value << (i % 8))
		}
		return append(append(pdu, byte(len(payload))), payload...), nil
	}
	if request.Function == 16 {
		payload := make([]byte, request.Count*2)
		for i, value := range request.Values {
			binary.BigEndian.PutUint16(payload[i*2:], uint16(value))
		}
		return append(append(pdu, byte(len(payload))), payload...), nil
	}
	return pdu, nil
}

// PreviewModbusWrite checks current authority and returns exact raw bytes without I/O.
func (m *Manager) PreviewModbusWrite(ctx context.Context, actor identity.Actor, endpointID string, input ModbusWrite) (ModbusWritePreview, error) {
	request, err := canonicalModbusRequest(input.Request)
	if err != nil || request.Function < 5 || !keyPattern.MatchString(input.IdempotencyKey) {
		return ModbusWritePreview{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != input.SessionID {
		return ModbusWritePreview{}, ErrNotFound
	}
	if err = m.modbusAuthorized(ctx, actor, s, true); err != nil {
		return ModbusWritePreview{}, err
	}
	e, err := sqlite.NewModbusRepository(m.options.Database).Endpoint(ctx, endpointID)
	if err != nil || validateModbusTarget(request, e) != nil {
		return ModbusWritePreview{}, ErrInvalid
	}
	pdu, _ := modbusPDU(request)
	digest, _ := modbusWriteDigest(s.id, s.endpointRevision, s.profileRevision, request)
	return ModbusWritePreview{SessionID: s.id, EndpointRevision: s.endpointRevision, ProfileRevision: s.profileRevision, Request: request, PDUHex: hex.EncodeToString(pdu), Digest: digest}, nil
}

type modbusDispatch func(context.Context, *modbusSession, ModbusRequest) (ModbusResult, error)

func modbusWriteDigest(sessionID string, endpointRevision, profileRevision int64, request ModbusRequest) (string, string) {
	canonical, _ := json.Marshal(struct {
		SessionID        string        `json:"session_id"`
		EndpointRevision int64         `json:"endpoint_revision"`
		ProfileRevision  int64         `json:"profile_revision"`
		Request          ModbusRequest `json:"request"`
	}{sessionID, endpointRevision, profileRevision, request})
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:]), string(canonical)
}

func (m *Manager) previousModbusWrite(ctx context.Context, actor identity.Actor, endpointID string, input ModbusWrite, digest string) (ModbusCommand, bool, error) {
	var result ModbusCommand
	var stored, raw, owner, kind, credential string
	err := m.db().QueryRowContext(ctx, `SELECT c.id,c.digest,c.result_json,s.owner_id,s.credential_kind,s.credential_id FROM modbus_commands c JOIN modbus_sessions s ON s.id=c.session_id WHERE s.id=? AND s.endpoint_id=? AND c.request_key=?`, input.SessionID, endpointID, input.IdempotencyKey).Scan(&result.ID, &stored, &raw, &owner, &kind, &credential)
	if errors.Is(err, sql.ErrNoRows) {
		return ModbusCommand{}, false, nil
	}
	if err != nil {
		return ModbusCommand{}, false, err
	}
	if !actor.Has(auth.PermissionBenchRead) || !actor.Has(auth.PermissionBenchExecute) || actor.User.ID != owner || string(actor.Credential) != kind || serialCredentialID(actor) != credential {
		return ModbusCommand{}, true, ErrForbidden
	}
	if err := m.serialCredentialValid(ctx, owner, kind, credential); err != nil {
		return ModbusCommand{}, true, err
	}
	if digest != stored {
		return ModbusCommand{}, true, ErrConflict
	}
	if json.Unmarshal([]byte(raw), &result) != nil {
		return ModbusCommand{}, true, ErrUnavailable
	}
	return result, true, nil
}

// WriteModbus reserves one immutable command before transport work.
func (m *Manager) WriteModbus(ctx context.Context, actor identity.Actor, endpointID string, input ModbusWrite) (ModbusCommand, error) {
	return m.writeModbus(ctx, actor, endpointID, input, func(ctx context.Context, s *modbusSession, request ModbusRequest) (ModbusResult, error) {
		return modbusPrivateCall(ctx, s, request, "")
	})
}

func (m *Manager) writeModbus(ctx context.Context, actor identity.Actor, endpointID string, input ModbusWrite, dispatch modbusDispatch) (ModbusCommand, error) {
	request, err := canonicalModbusRequest(input.Request)
	if err != nil || request.Function < 5 || !keyPattern.MatchString(input.IdempotencyKey) || !modbusID.MatchString(endpointID) || input.SessionID == "" {
		return ModbusCommand{}, ErrInvalid
	}
	input.Request = request
	var endpointRevision, profileRevision int64
	err = m.db().QueryRowContext(ctx, `SELECT endpoint_revision,profile_revision FROM modbus_sessions WHERE id=? AND endpoint_id=?`, input.SessionID, endpointID).Scan(&endpointRevision, &profileRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return ModbusCommand{}, ErrNotFound
	}
	if err != nil {
		return ModbusCommand{}, err
	}
	digest, canonical := modbusWriteDigest(input.SessionID, endpointRevision, profileRevision, request)
	if prior, found, err := m.previousModbusWrite(ctx, actor, endpointID, input, digest); found || err != nil {
		return prior, err
	}
	m.mu.Lock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != input.SessionID {
		m.mu.Unlock()
		return ModbusCommand{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, true); err != nil {
		m.mu.Unlock()
		return ModbusCommand{}, err
	}
	m.mu.Unlock()
	s.commandGate.Lock()
	defer s.commandGate.Unlock()
	if prior, found, err := m.previousModbusWrite(ctx, actor, endpointID, input, digest); found || err != nil {
		return prior, err
	}
	m.mu.Lock()
	if err := m.modbusAuthorized(ctx, actor, s, true); err == nil {
		e, lookupErr := sqlite.NewModbusRepository(m.options.Database).Endpoint(ctx, endpointID)
		if lookupErr != nil || validateModbusTarget(request, e) != nil {
			err = ErrInvalid
		}
	}
	if err != nil {
		m.mu.Unlock()
		return ModbusCommand{}, err
	}
	command := ModbusCommand{ID: uuid.NewString(), SessionID: s.id, IdempotencyKey: input.IdempotencyKey, Digest: digest, Outcome: "delivery_unknown", Result: ModbusResult{Outcome: "delivery_unknown", Values: []json.RawMessage{}}}
	raw, _ := json.Marshal(command)
	_, err = m.db().ExecContext(ctx, `INSERT INTO modbus_commands(id,session_id,request_key,digest,request_json,result_json,created_at) VALUES(?,?,?,?,?,?,?)`, command.ID, s.id, input.IdempotencyKey, digest, canonical, string(raw), m.now().Format(time.RFC3339Nano))
	if err == nil {
		err = m.record(ctx, actor, "modbus.write_requested", s.id, map[string]any{"command_id": command.ID, "digest": digest})
	}
	m.mu.Unlock()
	if err != nil {
		return command, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	result, callErr := dispatch(callCtx, s, request)
	cancel()
	if callErr == nil && validModbusResult(request, result) {
		command.Result, command.Outcome = result, result.Outcome
	}
	raw, _ = json.Marshal(command)
	_, err = m.db().ExecContext(context.WithoutCancel(ctx), `UPDATE modbus_commands SET result_json=? WHERE id=?`, string(raw), command.ID)
	if err != nil {
		return ModbusCommand{ID: command.ID, SessionID: s.id, IdempotencyKey: input.IdempotencyKey, Digest: digest, Outcome: "delivery_unknown"}, err
	}
	_ = m.record(context.WithoutCancel(ctx), actor, "modbus.write_finished", s.id, map[string]any{"command_id": command.ID, "outcome": command.Outcome})
	m.mu.Lock()
	if m.modbusSessions[endpointID] == s && !s.stopped {
		if s.recording != nil {
			m.report(m.appendModbusRecordingLocked(context.Background(), s, modbusWriteRecordedEvent(s.recording.meta.Mode, request, command)))
		}
	}
	m.mu.Unlock()
	return command, nil
}
