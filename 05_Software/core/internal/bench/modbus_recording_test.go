package bench

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
)

func TestModbusRecordingRetainsRotatedEvidence(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-record", "mb-record-bus"))
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision,
		profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID,
		kind: string(actor.Credential), credential: actor.SessionID,
		expires: time.Now().Add(time.Hour), cancel: func() {}, groups: []ModbusPollGroup{}, groupStats: map[string]*ModbusGroupStats{}}
	m.modbusSessions[e.ID] = s
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := m.db().Exec(`INSERT INTO modbus_sessions(id,endpoint_id,endpoint_revision,profile_id,profile_revision,owner_id,credential_kind,credential_id,started_at,expires_at,state) VALUES(?,?,?,?,?,?,?,?,?,?,'active')`, s.id, e.ID, e.Revision, p.ID, p.Revision, s.owner, s.kind, s.credential, stamp, s.expires.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	m.modbusSettings.PartBytes = 3000
	started, err := m.StartModbusRecording(t.Context(), actor, e.ID, s.id, ModbusRecordingOptions{Mode: "both", Sampling: "every", Name: "run one"})
	if err != nil || started.Status != "active" {
		t.Fatalf("start: %+v %v", started, err)
	}
	foreign := actor
	foreign.User.ID = uuid.NewString()
	foreign.Permissions = []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}
	if page, err := m.FindModbusRecordings(t.Context(), foreign, ModbusRecordingSearch{}); err != nil || len(page.Recordings) != 0 || !page.NoMatch {
		t.Fatalf("foreign listing exposed recording: %+v %v", page, err)
	}
	if _, err := m.FindModbusRecordings(t.Context(), foreign, ModbusRecordingSearch{Before: started.ID}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign cursor accepted: %v", err)
	}
	if _, err := m.ModbusRecordingByID(t.Context(), foreign, started.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign metadata: %v", err)
	}
	if _, err := m.FindModbusRecordingEvents(t.Context(), foreign, started.ID, ModbusRecordingFind{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign events: %v", err)
	}
	if _, _, err := m.ExportModbusRecording(t.Context(), foreign, started.ID, "jsonl", 0, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign export: %v", err)
	}
	if err := m.DeleteModbusRecording(t.Context(), foreign, started.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign delete: %v", err)
	}
	status := m.modbusStatus(s, actor)
	if !status.Logging || status.RecordingID != started.ID || status.LoggingMode != "both" {
		t.Fatalf("live recording status: %+v", status)
	}
	if err := m.DeleteModbusRecording(t.Context(), actor, started.ID); !errors.Is(err, ErrBusy) {
		t.Fatalf("active recording deletion: %v", err)
	}
	var denied int
	if err := m.db().QueryRow(`SELECT COUNT(*) FROM audit_events WHERE action='modbus.recording_delete_denied' AND subject_id=?`, started.ID).Scan(&denied); err != nil || denied != 1 {
		t.Fatalf("active delete refusal audit: count=%d err=%v", denied, err)
	}
	request := ModbusRequest{Function: 3, Unit: 1, Address: 10, Count: 1}
	for i := 0; i < 4; i++ {
		result := ModbusResult{Outcome: "acknowledged", Attempts: 1, Values: []json.RawMessage{json.RawMessage("42")}}
		if i == 0 {
			result.Frames = []ModbusTraceFrame{{RequestID: "read-1", Direction: "tx", Sequence: 1, Kind: "tcp", DataHex: "0102037f"}}
		}
		m.mu.Lock()
		err = m.recordModbusObservationLocked(context.Background(), s, strings.Repeat("g", 180), request, result)
		m.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	stopped, err := m.StopModbusRecording(t.Context(), actor, e.ID, s.id)
	if err != nil || stopped.Status != "completed" || stopped.LastSequence != 8 || stopped.PartCount < 2 {
		t.Fatalf("stop: %+v %v", stopped, err)
	}
	leasedMeta, release, err := m.leaseAuthorizedModbusRecording(t.Context(), actor, started.ID)
	if err != nil || leasedMeta.ID != started.ID {
		t.Fatalf("authorized lease: %+v %v", leasedMeta, err)
	}
	if err := m.DeleteModbusRecording(t.Context(), actor, started.ID); !errors.Is(err, ErrBusy) {
		t.Fatalf("atomic lease deletion: %v", err)
	}
	release()
	for part := 1; part <= stopped.PartCount; part++ {
		path := filepath.Join(m.options.DataDir, "modbus-recordings", started.ID, formatModbusPart(part))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("part %d: %v", part, err)
		}
	}
	first, err := os.ReadFile(filepath.Join(m.options.DataDir, "modbus-recordings", started.ID, formatModbusPart(1)))
	if err != nil || !strings.Contains(string(first), `"kind":"header"`) || !strings.Contains(string(first), `"snapshot":`) {
		t.Fatalf("missing first part header: %s %v", first, err)
	}
	foundSample := false
	for part := 1; part <= stopped.PartCount; part++ {
		body, err := os.ReadFile(filepath.Join(m.options.DataDir, "modbus-recordings", started.ID, formatModbusPart(part)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body[:bytes.IndexByte(body, '\n')], []byte(`"snapshot":`)) {
			t.Fatalf("part %d lacks pinned definitions", part)
		}
		foundSample = foundSample || strings.Contains(string(body), `"kind":"sample"`) && strings.Contains(string(body), `"raw":[42]`)
	}
	if !foundSample {
		t.Fatal("rotated parts lost raw sample")
	}
	page, err := m.FindModbusRecordingEvents(t.Context(), actor, started.ID, ModbusRecordingFind{Kind: "sample", Unit: 1, Function: 3})
	if err != nil || len(page.Events) != 4 || page.NextCursor == 0 {
		t.Fatalf("indexed sample search: %+v %v", page, err)
	}
	future, err := m.FindModbusRecordingEvents(t.Context(), actor, started.ID, ModbusRecordingFind{UTCFrom: "2100-01-01T00:00:00Z"})
	if err != nil || !future.NoMatch || len(future.Events) != 0 {
		t.Fatalf("UTC Find bound: %+v %v", future, err)
	}
	hexPage, err := m.FindModbusRecordingEvents(t.Context(), actor, started.ID, ModbusRecordingFind{Hex: "03 7F"})
	if err != nil || len(hexPage.Events) != 1 || hexPage.Events[0].Kind != "traffic" {
		t.Fatalf("captured frame hex Find: %+v %v", hexPage, err)
	}
	firstPage, err := m.FindModbusRecordingEvents(t.Context(), actor, started.ID, ModbusRecordingFind{Limit: 3})
	if err != nil || firstPage.Count != 3 || !firstPage.Truncated || firstPage.NextCursor != 3 {
		t.Fatalf("bounded first Find page: %+v %v", firstPage, err)
	}
	nextPage, err := m.FindModbusRecordingEvents(t.Context(), actor, started.ID, ModbusRecordingFind{After: firstPage.NextCursor, Limit: 3})
	if err != nil || nextPage.Count != 3 || nextPage.Events[0].Sequence != 4 {
		t.Fatalf("resumed Find page: %+v %v", nextPage, err)
	}
	if _, err := m.FindModbusRecordingEvents(t.Context(), actor, started.ID, ModbusRecordingFind{Hex: "037"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("odd hex accepted: %v", err)
	}
	updated, err := m.UpdateModbusRecordingMetadata(t.Context(), actor, started.ID, "=SUM(1,1)", []string{"commissioning"})
	if err != nil || updated.Name != "=SUM(1,1)" || len(updated.Tags) != 1 {
		t.Fatalf("metadata update: %+v %v", updated, err)
	}
	listed, err := m.FindModbusRecordings(t.Context(), actor, ModbusRecordingSearch{Name: "SUM", Tag: "commissioning"})
	if err != nil || len(listed.Recordings) != 1 || listed.Recordings[0].ID != started.ID {
		t.Fatalf("metadata find: %+v %v", listed, err)
	}
	canonical, _, err := m.ExportModbusRecording(t.Context(), actor, started.ID, "jsonl", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	canonicalData, err := io.ReadAll(canonical)
	_ = canonical.Close()
	if err != nil || !bytes.Contains(canonicalData, []byte(`"snapshot":`)) || !bytes.Contains(canonicalData, []byte(`"mb-record"`)) {
		t.Fatalf("JSONL missing pinned snapshot: %v", err)
	}
	reader, mediaType, err := m.ExportModbusRecording(t.Context(), actor, started.ID, "csv", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(mediaType, "text/csv") || !strings.Contains(string(data), "'=SUM(1,1)") || !strings.Contains(string(data), "42") || !strings.Contains(string(data), "recording_status") {
		t.Fatalf("CSV export: %s %s %v", mediaType, data, err)
	}
	if err := m.DeleteModbusRecording(t.Context(), actor, started.ID); !errors.Is(err, ErrBusy) {
		t.Fatalf("leased recording deletion: %v", err)
	}
	m.modbusSettings.RetentionCount = 1
	if _, err := m.StartModbusRecording(t.Context(), actor, e.ID, s.id, ModbusRecordingOptions{Mode: "values", Sampling: "change"}); !errors.Is(err, ErrBusy) {
		t.Fatalf("leased recording reclaimed: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	bundle, media, err := m.ExportModbusRecording(t.Context(), actor, started.ID, "bundle", 0, 0)
	if err != nil || media != "application/zip" {
		t.Fatalf("bundle open: %s %v", media, err)
	}
	archiveBytes, err := io.ReadAll(bundle)
	_ = bundle.Close()
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		t.Fatal(err)
	}
	var hasDefinitions, hasManifest, hasPart bool
	for _, entry := range archive.File {
		switch entry.Name {
		case "definitions.json":
			hasDefinitions = true
		case "manifest.json":
			hasManifest = true
			file, openErr := entry.Open()
			if openErr != nil {
				t.Fatal(openErr)
			}
			manifest, readErr := io.ReadAll(file)
			_ = file.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			checksum := sha256.Sum256(first)
			if !bytes.Contains(manifest, []byte(hex.EncodeToString(checksum[:]))) {
				t.Fatal("bundle manifest missing first-part digest")
			}
		case "parts/000001.jsonl":
			hasPart = true
		}
	}
	if !hasDefinitions || !hasManifest || !hasPart {
		t.Fatalf("incomplete bundle: %+v", archive.File)
	}
	firstPath := filepath.Join(m.options.DataDir, "modbus-recordings", started.ID, formatModbusPart(1))
	corrupt, err := os.OpenFile(firstPath, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = corrupt.WriteAt([]byte{'X'}, 0)
	if err == nil {
		err = corrupt.Close()
	} else {
		_ = corrupt.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.ExportModbusRecording(t.Context(), actor, started.ID, "bundle", 0, 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("tampered bundle accepted: %v", err)
	}
	root := m.modbusRecordingRoot(started.ID)
	if err := os.Rename(root, root+".deleting"); err != nil {
		t.Fatal(err)
	}
	if err := m.reconcileModbusDeletes(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("interrupted delete not restored: %v", err)
	}
	second, err := m.StartModbusRecording(t.Context(), actor, e.ID, s.id, ModbusRecordingOptions{Mode: "values", Sampling: "change"})
	if err != nil || second.ID == started.ID {
		t.Fatalf("retention did not admit new recording: %+v %v", second, err)
	}
	if _, err := m.ModbusRecordingByID(t.Context(), actor, started.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old completed recording not reclaimed: %v", err)
	}
	orphan := filepath.Join(m.options.DataDir, "modbus-recordings", uuid.NewString()+".deleting")
	if err := os.MkdirAll(orphan, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.reconcileModbusDeletes(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan staged delete retained: %v", err)
	}
	m.mu.Lock()
	err = m.recordModbusObservationLocked(context.Background(), s, "holding", request, ModbusResult{Outcome: "acknowledged", Attempts: 1, Values: []json.RawMessage{json.RawMessage("42")}, Frames: []ModbusTraceFrame{{Direction: "tx", DataHex: "deadbeef"}}})
	if err == nil {
		body, readErr := os.ReadFile(filepath.Join(m.modbusRecordingRoot(second.ID), formatModbusPart(1)))
		if readErr != nil {
			err = readErr
		} else if bytes.Contains(body, []byte(`"data_hex"`)) {
			t.Error("values-only mode retained raw traffic frames")
		}
	}
	if err == nil {
		_, err = s.recording.file.Write([]byte(`{"torn"`))
	}
	if err == nil {
		err = s.recording.file.Sync()
	}
	if err == nil {
		err = s.recording.file.Close()
	}
	s.recording = nil // model an unclean process exit before the new Manager starts
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.recoverModbusRecordings(t.Context()); err != nil {
		t.Fatal(err)
	}
	recovered, err := m.ModbusRecordingByID(t.Context(), actor, second.ID)
	if err != nil || recovered.Status != "incomplete" || recovered.LastSequence != 1 {
		t.Fatalf("crash recovery: %+v %v", recovered, err)
	}
}

func TestModbusRecordingSamplingKeepsQualityTransitions(t *testing.T) {
	start := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	r := &modbusRecorder{meta: ModbusRecording{Mode: "values", Sampling: "interval", IntervalMS: 1000}, last: map[string]modbusSampleState{}}
	key := "holding:10"
	if !r.shouldRecordSample(key, start, "42", "good") {
		t.Fatal("first sample suppressed")
	}
	if r.shouldRecordSample(key, start.Add(400*time.Millisecond), "42", "good") {
		t.Fatal("interval sample retained too early")
	}
	if !r.shouldRecordSample(key, start.Add(500*time.Millisecond), "42", "timeout") {
		t.Fatal("quality transition suppressed")
	}
	if !r.shouldRecordSample(key, start.Add(1100*time.Millisecond), "42", "good") {
		t.Fatal("recovery transition suppressed")
	}
	r.meta.Sampling = "change"
	if r.shouldRecordSample(key, start.Add(1200*time.Millisecond), "42", "good") {
		t.Fatal("unchanged value retained")
	}
	if !r.shouldRecordSample(key, start.Add(1300*time.Millisecond), "43", "good") {
		t.Fatal("changed value suppressed")
	}
}

func TestModbusRecordingWriteEvidenceHonorsMode(t *testing.T) {
	request := ModbusRequest{Function: 6, Unit: 1, Address: 4, Count: 1, Values: []int{42}}
	command := ModbusCommand{ID: uuid.NewString(), Digest: "digest", Result: ModbusResult{Outcome: "acknowledged", Frames: []ModbusTraceFrame{{Direction: "tx", DataHex: "01020304"}, {Direction: "rx", DataHex: "05060708"}}}}
	values := modbusWriteRecordedEvent("values", request, command)
	if values.Kind != "write" || values.CommandID != command.ID || len(values.Result.Frames) != 0 || values.RequestHex != "" {
		t.Fatalf("values-only write leaked frames: %+v", values)
	}
	traffic := modbusWriteRecordedEvent("traffic", request, command)
	if traffic.Kind != "write" || traffic.RequestHex != "01020304" || traffic.ReplyHex != "05060708" || len(traffic.Result.Frames) != 2 {
		t.Fatalf("traffic write lost frames: %+v", traffic)
	}
}

func TestModbusRecordingRecoveryIndexesSyncedUnindexedLine(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-recovery", "mb-recovery-bus"))
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision,
		profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID,
		kind: string(actor.Credential), credential: actor.SessionID,
		expires: time.Now().Add(time.Hour), cancel: func() {}, groups: []ModbusPollGroup{}, groupStats: map[string]*ModbusGroupStats{}}
	m.modbusSessions[e.ID] = s
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := m.db().Exec(`INSERT INTO modbus_sessions(id,endpoint_id,endpoint_revision,profile_id,profile_revision,owner_id,credential_kind,credential_id,started_at,expires_at,state) VALUES(?,?,?,?,?,?,?,?,?,?,'active')`, s.id, e.ID, e.Revision, p.ID, p.Revision, s.owner, s.kind, s.credential, stamp, s.expires.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	started, err := m.StartModbusRecording(t.Context(), actor, e.ID, s.id, ModbusRecordingOptions{Mode: "traffic", Sampling: "every"})
	if err != nil {
		t.Fatal(err)
	}
	event := ModbusRecordedEvent{Sequence: 1, Timestamp: stamp, Kind: "traffic", Request: ModbusRequest{Function: 3, Unit: 1, Address: 4, Count: 1}, Result: ModbusResult{Outcome: "acknowledged"}}
	line, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	_, err = s.recording.file.Write(append(line, '\n'))
	if err == nil {
		err = s.recording.file.Sync()
	}
	if err == nil {
		_, err = m.stopModbusRecordingLocked(s, "index_failed")
	}
	m.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.recoverModbusRecordings(t.Context()); err != nil {
		t.Fatal(err)
	}
	recovered, err := m.ModbusRecordingByID(t.Context(), actor, started.ID)
	if err != nil || recovered.Status != "incomplete" || recovered.Reason != "index_failed" || recovered.LastSequence != 1 {
		t.Fatalf("recovered synced line: %+v %v", recovered, err)
	}
	for _, format := range []string{"jsonl", "csv", "text"} {
		reader, _, err := m.ExportModbusRecording(t.Context(), actor, started.ID, format, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil || !bytes.Contains(body, []byte("index_failed")) {
			t.Fatalf("%s lost incomplete reason: %v", format, readErr)
		}
	}
	page, err := m.FindModbusRecordingEvents(t.Context(), actor, started.ID, ModbusRecordingFind{Kind: "traffic"})
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("recovered index: %+v %v", page, err)
	}
	next, err := m.StartModbusRecording(t.Context(), actor, e.ID, s.id, ModbusRecordingOptions{Mode: "traffic", Sampling: "every"})
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	err = s.recording.file.Close()
	if err == nil {
		err = m.appendModbusRecordingLocked(t.Context(), s, ModbusRecordedEvent{Kind: "traffic", Request: event.Request, Result: event.Result})
	}
	warning := s.recordingWarning
	m.mu.Unlock()
	if err == nil || warning != "append_failed" {
		t.Fatalf("append failure not visible: %s %v", warning, err)
	}
	failed, err := m.ModbusRecordingByID(t.Context(), actor, next.ID)
	if err != nil || failed.Status != "incomplete" || failed.Reason != "append_failed" {
		t.Fatalf("append failure metadata: %+v %v", failed, err)
	}
}

func TestModbusRecordingRotationReservesNewHeader(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-quota", "mb-quota-bus"))
	fixed := time.Now().UTC()
	m.options.Now = func() time.Time { return fixed }
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision,
		profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID,
		kind: string(actor.Credential), credential: actor.SessionID,
		expires: fixed.Add(time.Hour), cancel: func() {}, groups: []ModbusPollGroup{}, groupStats: map[string]*ModbusGroupStats{}}
	m.modbusSessions[e.ID] = s
	stamp := fixed.Format(time.RFC3339Nano)
	if _, err := m.db().Exec(`INSERT INTO modbus_sessions(id,endpoint_id,endpoint_revision,profile_id,profile_revision,owner_id,credential_kind,credential_id,started_at,expires_at,state) VALUES(?,?,?,?,?,?,?,?,?,?,'active')`, s.id, e.ID, e.Revision, p.ID, p.Revision, s.owner, s.kind, s.credential, stamp, s.expires.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	started, err := m.StartModbusRecording(t.Context(), actor, e.ID, s.id, ModbusRecordingOptions{Mode: "traffic", Sampling: "every"})
	if err != nil {
		t.Fatal(err)
	}
	event := ModbusRecordedEvent{Sequence: 1, Timestamp: fixed.Format("2006-01-02T15:04:05.000000000Z"), Kind: "traffic", Request: ModbusRequest{Function: 3, Unit: 1, Address: 4, Count: 1}, Result: ModbusResult{Outcome: "acknowledged"}}
	line, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	lineBytes := int64(len(line) + 1)
	m.modbusSettings.PartBytes = started.SizeBytes + 2*lineBytes - 1
	m.modbusSettings.RetentionBytes = started.SizeBytes + 2*lineBytes
	m.mu.Lock()
	err = m.appendModbusRecordingLocked(t.Context(), s, ModbusRecordedEvent{Kind: "traffic", Request: event.Request, Result: event.Result})
	if err == nil {
		err = m.appendModbusRecordingLocked(t.Context(), s, ModbusRecordedEvent{Kind: "traffic", Request: event.Request, Result: event.Result})
	}
	m.mu.Unlock()
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second part header bypassed quota: %v", err)
	}
	meta, err := m.ModbusRecordingByID(t.Context(), actor, started.ID)
	if err != nil || meta.Status != "incomplete" || meta.Reason != "quota_exhausted" || meta.SizeBytes > m.modbusSettings.RetentionBytes {
		t.Fatalf("quota failure metadata: %+v %v", meta, err)
	}
}
