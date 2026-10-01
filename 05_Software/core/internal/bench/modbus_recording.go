package bench

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

// ModbusRecordingOptions selects independently controlled retained evidence.
type ModbusRecordingOptions struct {
	Mode       string   `json:"mode"`
	Sampling   string   `json:"sampling"`
	IntervalMS int      `json:"interval_ms,omitempty"`
	Name       string   `json:"name,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

// ModbusRecording is the persistent logical identity across numbered parts.
type ModbusRecording struct {
	ID           string   `json:"id"`
	SessionID    string   `json:"session_id"`
	EndpointID   string   `json:"endpoint_id"`
	OwnerID      string   `json:"owner_id"`
	Name         string   `json:"name"`
	Tags         []string `json:"tags"`
	Mode         string   `json:"mode"`
	Sampling     string   `json:"sampling"`
	IntervalMS   int      `json:"interval_ms"`
	Status       string   `json:"status"`
	Reason       string   `json:"reason,omitempty"`
	StartedAt    string   `json:"started_at"`
	EndedAt      string   `json:"ended_at,omitempty"`
	LastSequence int64    `json:"last_sequence"`
	SizeBytes    int64    `json:"size_bytes"`
	PartCount    int      `json:"part_count"`
}

// ModbusRecordedEvent is one seekable immutable observation.
type ModbusRecordedEvent struct {
	Sequence   int64         `json:"sequence"`
	Timestamp  string        `json:"timestamp"`
	ElapsedNS  int64         `json:"elapsed_ns"`
	Kind       string        `json:"kind"`
	GroupID    string        `json:"group_id,omitempty"`
	Request    ModbusRequest `json:"request"`
	Result     ModbusResult  `json:"result"`
	PointID    string        `json:"point_id,omitempty"`
	Quality    string        `json:"quality,omitempty"`
	Raw        any           `json:"raw,omitempty"`
	Typed      any           `json:"typed,omitempty"`
	RequestHex string        `json:"request_hex,omitempty"`
	ReplyHex   string        `json:"reply_hex,omitempty"`
	CommandID  string        `json:"command_id,omitempty"`
	Digest     string        `json:"digest,omitempty"`
}

type modbusRecorder struct {
	meta     ModbusRecording
	snapshot []byte
	file     *os.File
	partNo   int
	partSize int64
	started  time.Time
	last     map[string]modbusSampleState
	points   map[string][]modbus.Point
}

type modbusSampleState struct {
	at      time.Time
	value   string
	quality string
}

func (r *modbusRecorder) shouldRecordSample(key string, at time.Time, value, quality string) bool {
	prior, found := r.last[key]
	keep := !found || prior.quality != quality
	if !keep {
		switch r.meta.Sampling {
		case "every":
			keep = true
		case "interval":
			keep = at.Sub(prior.at) >= time.Duration(r.meta.IntervalMS)*time.Millisecond
		case "change":
			keep = prior.value != value
		}
	}
	if keep {
		r.last[key] = modbusSampleState{at: at, value: value, quality: quality}
	}
	return keep
}

func validModbusRecordingOptions(o ModbusRecordingOptions) bool {
	if o.Mode != "values" && o.Mode != "traffic" && o.Mode != "both" {
		return false
	}
	if o.Sampling != "every" && o.Sampling != "interval" && o.Sampling != "change" {
		return false
	}
	if o.Sampling == "interval" && (o.IntervalMS < 100 || o.IntervalMS > 60000) || o.Sampling != "interval" && o.IntervalMS != 0 {
		return false
	}
	if !validModbusRecordingText(o.Name, 120) || len(o.Tags) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, tag := range o.Tags {
		key := strings.ToLower(tag)
		if tag == "" || !validModbusRecordingText(tag, 48) || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func validModbusRecordingText(value string, max int) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func formatModbusPart(number int) string { return fmt.Sprintf("%06d.jsonl", number) }

func modbusPartHeader(meta ModbusRecording, snapshot []byte, number int) ([]byte, error) {
	header, err := json.Marshal(struct {
		Schema      int             `json:"schema"`
		Kind        string          `json:"kind"`
		RecordingID string          `json:"recording_id"`
		PartNo      int             `json:"part_no"`
		SessionID   string          `json:"session_id"`
		EndpointID  string          `json:"endpoint_id"`
		OwnerID     string          `json:"owner_id"`
		Mode        string          `json:"mode"`
		Sampling    string          `json:"sampling"`
		StartedAt   string          `json:"started_at"`
		Snapshot    json.RawMessage `json:"snapshot"`
	}{1, "header", meta.ID, number, meta.SessionID, meta.EndpointID, meta.OwnerID, meta.Mode, meta.Sampling, meta.StartedAt, snapshot})
	if err != nil {
		return nil, err
	}
	return append(header, '\n'), nil
}

func (m *Manager) modbusRecordingRoot(id string) string {
	return filepath.Join(m.options.DataDir, "modbus-recordings", id)
}

// StartModbusRecording pins the current definitions and opens one logical recording.
func (m *Manager) StartModbusRecording(ctx context.Context, actor identity.Actor, endpointID, sessionID string, options ModbusRecordingOptions) (ModbusRecording, error) {
	if !validModbusRecordingOptions(options) {
		return ModbusRecording{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID {
		return ModbusRecording{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, true); err != nil {
		return ModbusRecording{}, err
	}
	if s.recording != nil {
		return ModbusRecording{}, ErrConflict
	}
	definitions, err := m.modbusRecordingSnapshot(ctx, s)
	if err != nil {
		return ModbusRecording{}, err
	}
	points, err := m.modbusRecordingPoints(ctx, s)
	if err != nil {
		return ModbusRecording{}, err
	}
	started := m.now()
	meta := ModbusRecording{ID: uuid.NewString(), SessionID: s.id, EndpointID: endpointID, OwnerID: s.owner,
		Name: options.Name, Tags: append([]string{}, options.Tags...), Mode: options.Mode, Sampling: options.Sampling,
		IntervalMS: options.IntervalMS, Status: "active", StartedAt: started.UTC().Format("2006-01-02T15:04:05.000000000Z")}
	header, err := modbusPartHeader(meta, definitions, 1)
	if err != nil {
		return ModbusRecording{}, err
	}
	if len(header) > 1<<20 || int64(len(header)) >= m.modbusSettings.PartBytes {
		return ModbusRecording{}, ErrInvalid
	}
	if err := m.reserveModbusRecordingLocked(ctx, int64(len(header)), 1); err != nil {
		return ModbusRecording{}, err
	}
	root := m.modbusRecordingRoot(meta.ID)
	if err := os.MkdirAll(root, 0700); err != nil {
		return ModbusRecording{}, err
	}
	file, err := os.OpenFile(filepath.Join(root, formatModbusPart(1)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		_ = os.Remove(root)
		return ModbusRecording{}, err
	}
	if _, err = file.Write(header); err == nil {
		err = file.Sync()
	}
	if err != nil {
		_ = file.Close()
		_ = os.RemoveAll(root)
		return ModbusRecording{}, err
	}
	tags, _ := json.Marshal(meta.Tags)
	tx, err := m.db().BeginTx(ctx, nil)
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO modbus_recordings(id,session_id,endpoint_id,owner_id,name,tags_json,mode,sampling,interval_ms,snapshot_json,status,started_at,size_bytes,part_count) VALUES(?,?,?,?,?,?,?,?,?,?,'active',?,?,1)`,
			meta.ID, meta.SessionID, meta.EndpointID, meta.OwnerID, meta.Name, string(tags), meta.Mode, meta.Sampling, meta.IntervalMS, string(definitions), meta.StartedAt, len(header))
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO modbus_recording_parts(recording_id,part_no,size_bytes) VALUES(?,1,?)`, meta.ID, len(header))
	}
	for _, tag := range meta.Tags {
		if err != nil {
			break
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO modbus_recording_tags(recording_id,tag_key) VALUES(?,?)`, meta.ID, strings.ToLower(tag))
	}
	if err == nil {
		err = tx.Commit()
	} else if tx != nil {
		_ = tx.Rollback()
	}
	if err != nil {
		_ = file.Close()
		_ = os.RemoveAll(root)
		return ModbusRecording{}, err
	}
	meta.SizeBytes = int64(len(header))
	meta.PartCount = 1
	s.recording = &modbusRecorder{meta: meta, snapshot: definitions, file: file, partNo: 1, partSize: meta.SizeBytes, started: started, last: map[string]modbusSampleState{}, points: points}
	s.lastRecordingID, s.recordingWarning = meta.ID, ""
	if err := m.record(ctx, actor, "modbus.recording_started", meta.ID, map[string]any{"mode": meta.Mode}); err != nil {
		m.report(err)
	}
	return meta, nil
}

func (m *Manager) modbusRecordingSnapshot(ctx context.Context, s *modbusSession) ([]byte, error) {
	repo := sqlite.NewModbusRepository(m.options.Database)
	e, err := repo.Endpoint(ctx, s.endpointID)
	if err != nil {
		return nil, err
	}
	p, err := repo.ProfileRevision(ctx, s.profileID, s.profileRevision)
	if err != nil {
		return nil, err
	}
	snapshot := map[string]any{"endpoint": e, "profile": p, "poll_groups": s.groups}
	if s.workspaceID != "" {
		workspace, err := repo.WorkspaceRevision(ctx, s.workspaceID, s.workspaceRevision)
		if err != nil {
			return nil, err
		}
		snapshot["workspace"] = workspace
		maps := make([]any, 0, len(workspace.MapRefs))
		for _, ref := range workspace.MapRefs {
			item, err := repo.MapRevision(ctx, ref.ID, ref.Revision)
			if err != nil {
				return nil, err
			}
			maps = append(maps, item)
		}
		snapshot["maps"] = maps
	}
	return json.Marshal(snapshot)
}

func (m *Manager) modbusRecordingPoints(ctx context.Context, s *modbusSession) (map[string][]modbus.Point, error) {
	points := map[string][]modbus.Point{}
	if s.workspaceID == "" {
		return points, nil
	}
	repo := sqlite.NewModbusRepository(m.options.Database)
	workspace, err := repo.WorkspaceRevision(ctx, s.workspaceID, s.workspaceRevision)
	if err != nil {
		return nil, err
	}
	for _, group := range workspace.PollGroups {
		item, err := repo.MapRevision(ctx, group.MapRef.ID, group.MapRef.Revision)
		if err != nil {
			return nil, err
		}
		points[group.ID] = item.Points
	}
	return points, nil
}

func modbusReadArea(function int) string {
	switch function {
	case 1:
		return "coil"
	case 2:
		return "discrete"
	case 3:
		return "holding"
	case 4:
		return "input"
	default:
		return ""
	}
}

func modbusSampleWords(values []json.RawMessage, start, width int) ([]uint16, bool) {
	if start < 0 || width < 1 || start+width > len(values) {
		return nil, false
	}
	words := make([]uint16, width)
	for i := range words {
		if string(values[start+i]) == "true" {
			words[i] = 1
			continue
		}
		if string(values[start+i]) == "false" {
			continue
		}
		var value int
		if json.Unmarshal(values[start+i], &value) != nil || value < 0 || value > 65535 {
			return nil, false
		}
		words[i] = uint16(value)
	}
	return words, true
}

// recordModbusObservationLocked retains traffic plus sampled point values; Manager.mu is held.
func (m *Manager) recordModbusObservationLocked(ctx context.Context, s *modbusSession, groupID string, request ModbusRequest, result ModbusResult) error {
	r := s.recording
	if r == nil {
		return nil
	}
	if r.meta.Mode == "traffic" || r.meta.Mode == "both" {
		trafficResult := result
		trafficResult.Values = nil
		traffic := ModbusRecordedEvent{Kind: "traffic", GroupID: groupID, Request: request, Result: trafficResult}
		for _, frame := range result.Frames {
			if frame.Direction == "tx" {
				traffic.RequestHex = frame.DataHex
			} else if frame.Direction == "rx" {
				traffic.ReplyHex = frame.DataHex
			}
		}
		if err := m.appendModbusRecordingLocked(ctx, s, traffic); err != nil {
			return err
		}
	}
	if r.meta.Mode == "traffic" {
		return nil
	}
	sampleResult := result
	sampleResult.Frames = nil
	quality := "good"
	if result.Outcome != "acknowledged" {
		quality = result.Outcome
	}
	points := r.points[groupID]
	kept := false
	for _, point := range points {
		if point.Area != modbusReadArea(request.Function) || point.Address < request.Address || point.Address+point.Width > request.Address+request.Count {
			continue
		}
		kept = true
		pointQuality := quality
		var raw any
		var typed any
		if pointQuality == "good" {
			words, ok := modbusSampleWords(result.Values, point.Address-request.Address, point.Width)
			if ok {
				raw = words
				decoded, err := modbus.DecodePoint(point, words)
				if err == nil {
					typed = decoded
				} else {
					pointQuality = "decode_error"
				}
			} else {
				pointQuality = "decode_error"
			}
		}
		value, _ := json.Marshal(raw)
		if !r.shouldRecordSample(groupID+":"+point.ID, m.now(), string(value), pointQuality) {
			continue
		}
		if err := m.appendModbusRecordingLocked(ctx, s, ModbusRecordedEvent{Kind: "sample", GroupID: groupID, Request: request, Result: sampleResult, PointID: point.ID, Quality: pointQuality, Raw: raw, Typed: typed}); err != nil {
			return err
		}
	}
	if kept {
		return nil
	}
	key := fmt.Sprintf("%s:%d:%d:%d", groupID, request.Unit, request.Function, request.Address)
	value, _ := json.Marshal(result.Values)
	if !r.shouldRecordSample(key, m.now(), string(value), quality) {
		return nil
	}
	return m.appendModbusRecordingLocked(ctx, s, ModbusRecordedEvent{Kind: "sample", GroupID: groupID, Request: request, Result: sampleResult, Quality: quality, Raw: result.Values})
}

func modbusWriteRecordedEvent(mode string, request ModbusRequest, command ModbusCommand) ModbusRecordedEvent {
	result := command.Result
	event := ModbusRecordedEvent{Kind: "write", Request: request, Result: result, CommandID: command.ID, Digest: command.Digest}
	if mode == "values" {
		event.Result.Frames = nil
		return event
	}
	for _, frame := range result.Frames {
		if frame.Direction == "tx" {
			event.RequestHex = frame.DataHex
		} else if frame.Direction == "rx" {
			event.ReplyHex = frame.DataHex
		}
	}
	return event
}

// StopModbusRecording completes the active recording without stopping polls.
func (m *Manager) StopModbusRecording(ctx context.Context, actor identity.Actor, endpointID, sessionID string) (ModbusRecording, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.modbusSessions[endpointID]
	if s == nil || s.id != sessionID {
		return ModbusRecording{}, ErrNotFound
	}
	if err := m.modbusAuthorized(ctx, actor, s, true); err != nil {
		return ModbusRecording{}, err
	}
	if s.recording == nil {
		return ModbusRecording{}, ErrNotFound
	}
	meta, err := m.stopModbusRecordingLocked(s, "")
	if err == nil {
		m.report(m.record(ctx, actor, "modbus.recording_stopped", meta.ID, nil))
	}
	return meta, err
}

func (m *Manager) stopModbusRecordingLocked(s *modbusSession, reason string) (ModbusRecording, error) {
	r := s.recording
	if r == nil {
		return ModbusRecording{}, ErrNotFound
	}
	s.recording = nil
	if err := m.closeModbusPartLocked(r); err != nil && reason == "" {
		reason = "part_close_failed"
	}
	if reason == "" {
		r.meta.Status = "completed"
	} else {
		r.meta.Status, r.meta.Reason = "incomplete", reason
	}
	if reason != "" {
		s.recordingWarning = reason
	}
	r.meta.EndedAt = m.now().Format(time.RFC3339Nano)
	_, err := m.db().Exec(`UPDATE modbus_recordings SET status=?,reason=?,ended_at=?,last_sequence=?,size_bytes=?,part_count=? WHERE id=?`, r.meta.Status, r.meta.Reason, r.meta.EndedAt, r.meta.LastSequence, r.meta.SizeBytes, r.meta.PartCount, r.meta.ID)
	return r.meta, err
}

func (m *Manager) closeModbusPartLocked(r *modbusRecorder) error {
	if r.file == nil {
		return nil
	}
	if err := r.file.Sync(); err != nil {
		_ = r.file.Close()
		r.file = nil
		return err
	}
	if err := r.file.Close(); err != nil {
		r.file = nil
		return err
	}
	r.file = nil
	file, err := os.Open(filepath.Join(m.modbusRecordingRoot(r.meta.ID), formatModbusPart(r.partNo)))
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	_, err = m.db().Exec(`UPDATE modbus_recording_parts SET closed=1,sha256=? WHERE recording_id=? AND part_no=?`, hex.EncodeToString(hash.Sum(nil)), r.meta.ID, r.partNo)
	return err
}

func (m *Manager) rotateModbusPartLocked(r *modbusRecorder, header []byte) error {
	if err := m.closeModbusPartLocked(r); err != nil {
		return err
	}
	number := r.partNo + 1
	path := filepath.Join(m.modbusRecordingRoot(r.meta.ID), formatModbusPart(number))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(header); err == nil {
		err = file.Sync()
	}
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if _, err = m.db().Exec(`INSERT INTO modbus_recording_parts(recording_id,part_no,size_bytes) VALUES(?,?,?)`, r.meta.ID, number, len(header)); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	r.file, r.partNo, r.partSize = file, number, int64(len(header))
	r.meta.PartCount++
	r.meta.SizeBytes += r.partSize
	return nil
}

// appendModbusRecordingLocked persists a line before exposing its index. Manager.mu is held.
func (m *Manager) appendModbusRecordingLocked(ctx context.Context, s *modbusSession, event ModbusRecordedEvent) error {
	r := s.recording
	if r == nil {
		return nil
	}
	if event.Kind != "traffic" && event.Kind != "sample" && event.Kind != "write" {
		return ErrInvalid
	}
	if event.Kind == "traffic" && r.meta.Mode == "values" || event.Kind == "sample" && r.meta.Mode == "traffic" {
		return nil
	}
	now := m.now()
	event.Sequence = r.meta.LastSequence + 1
	event.Timestamp = now.Format("2006-01-02T15:04:05.000000000Z")
	event.ElapsedNS = now.Sub(r.started).Nanoseconds()
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if len(line) > 60<<10 {
		return m.failModbusRecordingLocked(s, "event_oversized", ErrInvalid)
	}
	if int64(len(line))+128 > m.modbusSettings.PartBytes {
		return m.failModbusRecordingLocked(s, "event_oversized", ErrInvalid)
	}
	rotate := r.partSize+int64(len(line)) > m.modbusSettings.PartBytes
	additional := int64(len(line))
	var nextHeader []byte
	if rotate {
		nextHeader, err = modbusPartHeader(r.meta, r.snapshot, r.partNo+1)
		if err != nil {
			return m.failModbusRecordingLocked(s, "rotation_failed", err)
		}
		if int64(len(nextHeader))+int64(len(line)) > m.modbusSettings.PartBytes {
			return m.failModbusRecordingLocked(s, "event_oversized", ErrInvalid)
		}
		additional += int64(len(nextHeader))
	}
	if err := m.reserveModbusRecordingLocked(ctx, additional, 0); err != nil {
		return m.failModbusRecordingLocked(s, "quota_exhausted", err)
	}
	if rotate {
		if err := m.rotateModbusPartLocked(r, nextHeader); err != nil {
			return m.failModbusRecordingLocked(s, "rotation_failed", err)
		}
	}
	offset := r.partSize
	if _, err = r.file.Write(line); err == nil {
		err = r.file.Sync()
	}
	if err != nil {
		return m.failModbusRecordingLocked(s, "append_failed", err)
	}
	tx, err := m.db().BeginTx(ctx, nil)
	if err != nil {
		return m.failModbusRecordingLocked(s, "index_failed", err)
	}
	failIndex := func(cause error) error {
		_ = tx.Rollback()
		return m.failModbusRecordingLocked(s, "index_failed", cause)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO modbus_recording_events(recording_id,sequence,part_no,offset_bytes,length_bytes,kind,utc_at,elapsed_ns,unit,function,address,outcome,point_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.meta.ID, event.Sequence, r.partNo, offset, len(line), event.Kind, event.Timestamp, event.ElapsedNS,
		event.Request.Unit, event.Request.Function, event.Request.Address, event.Result.Outcome, event.PointID)
	if err != nil {
		return failIndex(err)
	}
	nextPartSize := r.partSize + int64(len(line))
	nextTotalSize := r.meta.SizeBytes + int64(len(line))
	_, err = tx.ExecContext(ctx, `UPDATE modbus_recording_parts SET size_bytes=?,first_sequence=CASE WHEN first_sequence=0 THEN ? ELSE first_sequence END,last_sequence=? WHERE recording_id=? AND part_no=?`, nextPartSize, event.Sequence, event.Sequence, r.meta.ID, r.partNo)
	if err != nil {
		return failIndex(err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE modbus_recordings SET last_sequence=?,size_bytes=?,part_count=? WHERE id=?`, event.Sequence, nextTotalSize, r.meta.PartCount, r.meta.ID)
	if err != nil {
		return failIndex(err)
	}
	if err := tx.Commit(); err != nil {
		return m.failModbusRecordingLocked(s, "index_failed", err)
	}
	r.partSize, r.meta.SizeBytes, r.meta.LastSequence = nextPartSize, nextTotalSize, event.Sequence
	return nil
}

func (m *Manager) failModbusRecordingLocked(s *modbusSession, reason string, cause error) error {
	_, stopErr := m.stopModbusRecordingLocked(s, reason)
	m.report(errors.Join(cause, stopErr))
	return cause
}

func (m *Manager) reserveModbusRecordingLocked(ctx context.Context, additionalBytes int64, additionalCount int) error {
	var count, bytes int64
	if err := m.db().QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(size_bytes),0) FROM modbus_recordings`).Scan(&count, &bytes); err != nil {
		return err
	}
	for count+int64(additionalCount) > int64(m.modbusSettings.RetentionCount) || bytes+additionalBytes > m.modbusSettings.RetentionBytes {
		rows, err := m.db().QueryContext(ctx, `SELECT id,size_bytes FROM modbus_recordings WHERE status='completed' ORDER BY started_at,id LIMIT 1000`)
		if err != nil {
			return err
		}
		var eligibleID string
		var eligibleBytes int64
		for rows.Next() {
			var id string
			var size int64
			if err := rows.Scan(&id, &size); err != nil {
				_ = rows.Close()
				return err
			}
			if m.modbusReads[id] == 0 {
				eligibleID, eligibleBytes = id, size
				break
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		if eligibleID == "" {
			m.report(m.options.Audit.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: audit.ActorKindSystem, Action: "modbus.retention_refused", SubjectType: "integration", SubjectID: "modbus", Outcome: audit.OutcomeDenied, Details: map[string]any{"reason": "no_eligible_recording"}}))
			return ErrBusy
		}
		if err := m.options.Audit.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: audit.ActorKindSystem, Action: "modbus.retention_delete_requested", SubjectType: "integration", SubjectID: eligibleID, Outcome: audit.OutcomeRequested}); err != nil {
			return err
		}
		if err := m.deleteModbusRecordingLocked(ctx, eligibleID); err != nil {
			return err
		}
		m.report(m.options.Audit.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: audit.ActorKindSystem, Action: "modbus.retention_deleted", SubjectType: "integration", SubjectID: eligibleID, Outcome: audit.OutcomeApplied}))
		count--
		bytes -= eligibleBytes
	}
	return nil
}

func (m *Manager) deleteModbusRecordingLocked(ctx context.Context, id string) error {
	if !validModbusRecordingID(id) {
		return ErrInvalid
	}
	root := m.modbusRecordingRoot(id)
	staged := root + ".deleting"
	if err := os.Rename(root, staged); err != nil {
		return err
	}
	_, err := m.db().ExecContext(ctx, `DELETE FROM modbus_recordings WHERE id=?`, id)
	if err != nil {
		_ = os.Rename(staged, root)
		return err
	}
	if err := os.RemoveAll(staged); err != nil {
		m.report(err)
	}
	return nil
}

// DeleteModbusRecording removes only a stopped, non-leased recording.
func (m *Manager) DeleteModbusRecording(ctx context.Context, actor identity.Actor, id string) error {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return err
	}
	meta, err := m.ModbusRecordingByID(ctx, actor, id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if meta.Status == "active" || m.modbusReads[id] != 0 {
		if err := m.auditModbusRecordingDeleteDenied(ctx, actor, id, "active_or_leased"); err != nil {
			return err
		}
		return ErrBusy
	}
	if err := m.record(ctx, actor, "modbus.recording_delete_requested", id, nil); err != nil {
		return err
	}
	if err := m.deleteModbusRecordingLocked(ctx, id); err != nil {
		return err
	}
	m.report(m.record(ctx, actor, "modbus.recording_deleted", id, nil))
	return nil
}

func (m *Manager) auditModbusRecordingDeleteDenied(ctx context.Context, actor identity.Actor, id, reason string) error {
	actorID, err := domain.ParseActorID(actor.User.ID)
	if err != nil {
		return ErrForbidden
	}
	kind := audit.ActorKindUser
	if actor.TokenID != "" {
		kind = audit.ActorKindToken
	}
	return m.options.Audit.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: kind, ActorID: actorID, Action: "modbus.recording_delete_denied", SubjectType: "integration", SubjectID: id, Outcome: audit.OutcomeDenied, Details: map[string]any{"reason": reason}})
}

func (m *Manager) readModbusRecording(ctx context.Context, id string) (ModbusRecording, error) {
	var result ModbusRecording
	var tags string
	err := m.db().QueryRowContext(ctx, `SELECT id,session_id,endpoint_id,owner_id,name,tags_json,mode,sampling,interval_ms,status,reason,started_at,ended_at,last_sequence,size_bytes,part_count FROM modbus_recordings WHERE id=?`, id).Scan(
		&result.ID, &result.SessionID, &result.EndpointID, &result.OwnerID, &result.Name, &tags,
		&result.Mode, &result.Sampling, &result.IntervalMS, &result.Status, &result.Reason, &result.StartedAt,
		&result.EndedAt, &result.LastSequence, &result.SizeBytes, &result.PartCount)
	if errors.Is(err, sql.ErrNoRows) {
		return ModbusRecording{}, ErrNotFound
	}
	if err != nil {
		return ModbusRecording{}, err
	}
	if json.Unmarshal([]byte(tags), &result.Tags) != nil {
		return ModbusRecording{}, ErrUnavailable
	}
	return result, nil
}

// ModbusRecordingByID returns retained metadata only to its owner or an administrator.
func (m *Manager) ModbusRecordingByID(ctx context.Context, actor identity.Actor, id string) (ModbusRecording, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return ModbusRecording{}, err
	}
	result, err := m.readModbusRecording(ctx, id)
	if err != nil {
		return ModbusRecording{}, err
	}
	if result.OwnerID != actor.User.ID && !actor.Has(auth.PermissionPluginsManage) {
		return ModbusRecording{}, ErrForbidden
	}
	return result, nil
}

// UpdateModbusRecordingMetadata changes only search labels, never evidence or snapshots.
func (m *Manager) UpdateModbusRecordingMetadata(ctx context.Context, actor identity.Actor, id, name string, tags []string) (ModbusRecording, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return ModbusRecording{}, err
	}
	if !validModbusRecordingOptions(ModbusRecordingOptions{Mode: "values", Sampling: "every", Name: name, Tags: tags}) {
		return ModbusRecording{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, err := m.ModbusRecordingByID(ctx, actor, id)
	if err != nil {
		return ModbusRecording{}, err
	}
	if err := m.record(ctx, actor, "modbus.recording_metadata_requested", id, map[string]any{"name": name, "tags": tags}); err != nil {
		return ModbusRecording{}, err
	}
	raw, _ := json.Marshal(tags)
	tx, err := m.db().BeginTx(ctx, nil)
	if err != nil {
		return ModbusRecording{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE modbus_recordings SET name=?,tags_json=? WHERE id=?`, name, string(raw), id); err != nil {
		return ModbusRecording{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM modbus_recording_tags WHERE recording_id=?`, id); err != nil {
		return ModbusRecording{}, err
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO modbus_recording_tags(recording_id,tag_key) VALUES(?,?)`, id, strings.ToLower(tag)); err != nil {
			return ModbusRecording{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ModbusRecording{}, err
	}
	meta.Name, meta.Tags = name, append([]string{}, tags...)
	m.report(m.record(ctx, actor, "modbus.recording_metadata_changed", id, nil))
	return meta, nil
}

func validModbusRecordingID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && strings.EqualFold(parsed.String(), id)
}
