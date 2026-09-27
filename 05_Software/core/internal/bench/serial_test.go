package bench

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

func TestSerialApprovedConnectionContract(t *testing.T) {
	input := Submission{PluginID: "serial", Operation: "connect", DeviceID: "serial-1", IdempotencyKey: "serial-test-key", Parameters: json.RawMessage(`{"revision":1,"settings":{"baud_rate":115200,"data_bits":8,"parity":"none","stop_bits":1,"flow_control":"none","rts":true,"dtr":true}}`)}
	if _, err := canonicalSubmission(input); err != nil {
		t.Fatalf("serial connection must be admitted for policy evaluation: %v", err)
	}
	input.Operation = "stream"
	if _, err := canonicalSubmission(input); err == nil {
		t.Fatal("private stream accepted")
	}
	input.Operation = "connect"
	input.DeviceID = "/dev/ttyUSB0"
	if _, err := canonicalSubmission(input); err == nil {
		t.Fatal("raw device path accepted")
	}
}

func TestSerialPayloadAndBoundedPages(t *testing.T) {
	var defaults SerialSettings
	if err := json.Unmarshal([]byte(`{}`), &defaults); err != nil || defaults.BaudRate != 115200 || !defaults.RTS || !defaults.DTR {
		t.Fatalf("serial defaults %+v %v", defaults, err)
	}
	if err := json.Unmarshal([]byte(`{"rts":false,"dtr":false}`), &defaults); err != nil || defaults.RTS || defaults.DTR {
		t.Fatal("explicit false line states lost")
	}
	if err := json.Unmarshal([]byte(`{"rts":null}`), &defaults); err == nil {
		t.Fatal("null line state accepted")
	}
	for _, test := range []struct {
		encoding, data, ending string
		want                   []byte
		valid                  bool
	}{
		{"hex", "00 ff 0a", "crlf", []byte{0, 255, 10, 13, 10}, true},
		{"hex", "0 0", "none", nil, false},
		{"hex", "0", "none", nil, false}, {"hex", "0z", "none", nil, false},
		{"ascii", "é", "none", nil, false}, {"utf8", "é", "lf", []byte("é\n"), true},
		{"ascii", strings.Repeat("a", 4096), "lf", nil, false},
	} {
		got, err := serialPayload(SerialSend{IdempotencyKey: "payload-test", Encoding: test.encoding, Data: test.data, Ending: test.ending})
		if (err == nil) != test.valid || test.valid && !bytes.Equal(got, test.want) {
			t.Errorf("%+v got %q %v", test, got, err)
		}
	}
	events := []SerialEvent{}
	for i := int64(50); i < 100; i++ {
		events = append(events, SerialEvent{Sequence: i, Direction: "rx", DataBase64: base64.StdEncoding.EncodeToString(make([]byte, 4096)), ByteCount: 4096})
	}
	page := serialPage(events, 1)
	raw, _ := json.Marshal(page)
	if !page.Gap || len(raw) > 64<<10 || len(page.Events) == 0 || page.NextCursor != page.Events[len(page.Events)-1].Sequence {
		t.Fatalf("bad bounded page %d %+v", len(raw), page)
	}
}

func serialFixture(t *testing.T) (*Manager, identity.Actor, *serialSession) {
	t.Helper()
	m, actor, _ := testManager(t, "auto")
	actor.SessionID = uuid.NewString()
	now := time.Now().UTC()
	stamp := now.Format(time.RFC3339Nano)
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users(id,username,password_hash,created_at,updated_at,role) VALUES(?,?,?,?,?,'administrator')`, []any{actor.User.ID, "serial-owner", "test", stamp, stamp}},
		{`INSERT INTO user_sessions(id,user_id,verifier,csrf_verifier,created_at,expires_at) VALUES(?,?,x'01',x'02',?,?)`, []any{actor.SessionID, actor.User.ID, stamp, now.Add(time.Hour).Format(time.RFC3339Nano)}},
		{`INSERT INTO bench_plugins(id,instance_id,installed,enabled,mode) VALUES('serial',?,1,1,'auto')`, []any{uuid.NewString()}},
	}
	for _, s := range statements {
		if _, err := m.db().Exec(s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	raw := json.RawMessage(`{"devices":[{"id":"ttyUSB0","kind":"usb-serial","display_name":"USB adapter","hardware_identity":{"vendor":"0403","product":"6001","serial":"abc/if00/port0","port_path":"1-2:if00:port0"},"capabilities":{"interface":"00"}}]}`)
	records, err := m.reconcileSerialDiscovery(t.Context(), raw)
	if err != nil || len(records) != 1 || records[0].ID != "serial-1" {
		t.Fatalf("discovery %+v %v", records, err)
	}
	repo := sqlite.NewManagedDeviceRepository(m.options.Database)
	r, err := repo.Configure(t.Context(), "serial", "serial-1", records[0].Revision, "Console", "", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	r, err = repo.MarkReady(t.Context(), "serial", r.ID, r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	s := &serialSession{id: uuid.NewString(), device: r.ID, owner: actor.User.ID, credential: actor.SessionID, kind: "session", revision: r.Revision, settings: SerialSettings{115200, 8, "none", 1, "none", true, true}, expires: now.Add(time.Hour), lastAccess: now, signals: map[string]*bool{}, transport: &cameraSession{cancel: func() {}}}
	m.serials[s.device] = s
	m.cameraSpace = func(string) (int64, int64, error) { return 100 << 30, 200 << 30, nil }
	settings, _ := json.Marshal(s.settings)
	if _, err = m.db().Exec(`INSERT INTO serial_connections VALUES(?,?,?,?,?,?,?,?,?,?)`, s.id, s.device, s.owner, s.kind, s.credential, s.revision, string(settings), stamp, s.expires.Format(time.RFC3339Nano), "active"); err != nil {
		t.Fatal(err)
	}
	return m, actor, s
}

func TestSerialCredentialBindingAndRevocation(t *testing.T) {
	m, a, s := serialFixture(t)
	if err := m.serialAuthorized(t.Context(), a, s, true); err != nil {
		t.Fatal(err)
	}
	other := a
	other.SessionID = uuid.NewString()
	if err := m.serialAuthorized(t.Context(), other, s, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("different credential: %v", err)
	}
	other.User.ID = uuid.NewString()
	if err := m.serialAuthorized(t.Context(), other, s, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin cannot inherit TX: %v", err)
	}
	if _, err := m.db().Exec(`UPDATE user_sessions SET revoked_at=? WHERE id=?`, time.Now().Format(time.RFC3339Nano), a.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := m.serialAuthorized(t.Context(), a, s, true); !errors.Is(err, ErrForbidden) || !s.stopped {
		t.Fatalf("revocation failed %v stopped=%v", err, s.stopped)
	}
}

func TestSerialSendIdempotencyAndUnknownResult(t *testing.T) {
	m, a, s := serialFixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if calls.Load() == 2 {
			w.WriteHeader(500)
			return
		}
		io.WriteString(w, `{"status":"sent","bytes_written":3}`)
	}))
	defer server.Close()
	s.transport.client = &http.Client{Transport: serialTestTransport{server.URL}, Timeout: time.Second}
	input := SerialSend{ConnectionID: s.id, IdempotencyKey: "serial-send-one", Encoding: "hex", Data: "00 01 ff", Ending: "none"}
	for i := 0; i < 2; i++ {
		result, err := m.SendSerial(t.Context(), a, s.device, input)
		if err != nil || result.Status != "sent" || result.BytesWritten != 3 {
			t.Fatalf("send %+v %v", result, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("idempotent send repeated hardware write")
	}
	input.Data = "02"
	if _, err := m.SendSerial(t.Context(), a, s.device, input); !errors.Is(err, ErrConflict) {
		t.Fatal("same key with different payload accepted")
	}
	input.IdempotencyKey = "serial-send-two"
	for i := 0; i < 2; i++ {
		result, err := m.SendSerial(t.Context(), a, s.device, input)
		if err != nil || result.Status != "unknown" {
			t.Fatalf("uncertain send %+v %v", result, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("unknown send was replayed")
	}
	// A restart discards the live connection but retains its send reservations.
	delete(m.serials, s.device)
	if err := m.recoverSerialLogs(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := m.SendSerial(t.Context(), a, s.device, input)
	if err != nil || result.Status != "unknown" || calls.Load() != 2 {
		t.Fatalf("restart replay/result %+v %v calls=%d", result, err, calls.Load())
	}
}

type serialTestTransport struct{ url string }

func (t serialTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.URL.Scheme = "http"
	r.URL.Host = strings.TrimPrefix(t.url, "http://")
	return http.DefaultTransport.RoundTrip(r)
}

func TestSerialLogRotationOwnershipExportAndRetention(t *testing.T) {
	m, a, s := serialFixture(t)
	ctx := t.Context()
	state, err := m.serialRetention(ctx, s.device)
	if err != nil {
		t.Fatal(err)
	}
	s.stopped = true
	_, err = m.UpdateSerialRetention(ctx, a, s.device, state.Revision, SerialRetention{2, 1 << 20, 64 << 10})
	s.stopped = false
	if err != nil {
		t.Fatal(err)
	}
	if err = m.startSerialLog(ctx, s); err != nil {
		t.Fatal(err)
	}
	expected := bytes.Repeat([]byte{0, 255, 13, 10}, 1024)
	for i := int64(1); i <= 25; i++ {
		event := SerialEvent{Sequence: i, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Direction: "rx", DataBase64: base64.StdEncoding.EncodeToString(expected), ByteCount: len(expected)}
		if err = m.appendSerialLog(s, event); err != nil {
			t.Fatal(err)
		}
	}
	m.finishSerialLog(s, true)
	page, err := m.ListSerialLogs(ctx, a, s.device, 0)
	if err != nil || page.Total != 2 {
		t.Fatalf("rotation/count: %+v %v", page, err)
	}
	id := page.Logs[0].ID
	other := identity.Actor{User: auth.User{ID: uuid.NewString()}, Permissions: []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}}
	if _, err = m.ReadSerialLog(ctx, other, id, 0); !errors.Is(err, ErrForbidden) {
		t.Fatal("cross-owner log readable")
	}
	reader, _, _, err := m.ExportSerialLog(ctx, a, id, "rx")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.DeleteSerialLog(ctx, a, id); !errors.Is(err, ErrBusy) {
		t.Fatal("active download deletable")
	}
	content, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || len(content) == 0 || len(content)%len(expected) != 0 || !bytes.Equal(content[:len(expected)], expected) {
		t.Fatal("binary export changed bytes")
	}
	if err = m.DeleteSerialLog(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	m.cameraSpace = func(string) (int64, int64, error) { return 1 << 20, 200 << 30, nil }
	if err = m.startSerialLog(ctx, s); err == nil {
		t.Fatal("disk reserve ignored")
	}
}

func TestSerialCrashRecoveryPreservesValidPrefix(t *testing.T) {
	m, a, s := serialFixture(t)
	if err := m.startSerialLog(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	event := SerialEvent{Sequence: 1, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Direction: "tx", DataBase64: "AP8=", ByteCount: 2}
	if err := m.appendSerialLog(s, event); err != nil {
		t.Fatal(err)
	}
	id := s.log.id
	size := s.log.size
	if _, err := s.log.file.WriteString(`{"sequence":2`); err != nil {
		t.Fatal(err)
	}
	s.log.file.Close()
	s.log = nil
	if err := m.recoverSerialLogs(context.Background()); err != nil {
		t.Fatal(err)
	}
	logs, err := m.ListSerialLogs(t.Context(), a, s.device, 0)
	if err != nil || logs.Logs[0].Status != "incomplete" || logs.Logs[0].TXBytes != 2 {
		t.Fatalf("recovery %+v %v", logs, err)
	}
	path, _ := m.serialLogPath(id)
	stat, _ := os.Stat(path)
	if stat.Size() != size {
		t.Fatalf("invalid tail retained: %d != %d", stat.Size(), size)
	}
}

func TestSerialRotationRetainsBufferedEventTimestamps(t *testing.T) {
	m, a, s := serialFixture(t)
	state, err := m.serialRetention(t.Context(), s.device)
	if err != nil {
		t.Fatal(err)
	}
	s.stopped = true
	_, err = m.UpdateSerialRetention(t.Context(), a, s.device, state.Revision, SerialRetention{10, 1 << 20, 64 << 10})
	s.stopped = false
	if err != nil {
		t.Fatal(err)
	}
	if err = m.startSerialLog(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	observed := time.Now().UTC().Format(time.RFC3339Nano)
	for i := int64(1); i <= 25; i++ {
		e := SerialEvent{Sequence: i, Timestamp: observed, Direction: "rx", DataBase64: base64.StdEncoding.EncodeToString(make([]byte, 4096)), ByteCount: 4096}
		if err = m.appendSerialLog(s, e); err != nil {
			t.Fatal(err)
		}
	}
	m.finishSerialLog(s, true)
	page, err := m.ListSerialLogs(t.Context(), a, s.device, 0)
	if err != nil {
		t.Fatal(err)
	}
	var bytes int64
	for _, l := range page.Logs {
		bytes += l.RXBytes
	}
	if bytes != 25*4096 {
		t.Fatalf("buffered bytes lost across rotation: %d", bytes)
	}
}

func TestSerialInteriorGapIsVisibleAndLogBecomesIncomplete(t *testing.T) {
	m, a, s := serialFixture(t)
	if err := m.startSerialLog(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	for _, sequence := range []int64{1, 3} {
		event := SerialEvent{Sequence: sequence, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Direction: "rx", DataBase64: "AA==", ByteCount: 1}
		raw, _ := json.Marshal(SerialRead{Events: []SerialEvent{event}, NextCursor: sequence, Gap: sequence == 3})
		if err := m.ingestSerial(s, raw); err != nil {
			t.Fatal(err)
		}
	}
	page := serialPage(s.events, 1)
	if !page.Gap || s.log != nil {
		t.Fatal("gap hidden or logging still active")
	}
	logs, err := m.ListSerialLogs(t.Context(), a, s.device, 0)
	if err != nil || logs.Logs[0].Status != "incomplete" {
		t.Fatalf("gap recording %+v %v", logs, err)
	}
}

func TestSerialStopLoggingDrainsRunnerTail(t *testing.T) {
	m, a, s := serialFixture(t)
	if err := m.startSerialLog(t.Context(), s); err != nil {
		t.Fatal(err)
	}
	event := SerialEvent{Sequence: 1, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Direction: "tx", DataBase64: "AP8=", ByteCount: 2}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			io.WriteString(w, `{"connected":true,"last_sequence":1,"signals":{}}`)
			return
		}
		json.NewEncoder(w).Encode(SerialRead{Events: []SerialEvent{event}, NextCursor: 1})
	}))
	defer server.Close()
	s.transport.client = &http.Client{Transport: serialTestTransport{server.URL}, Timeout: time.Second}
	if _, err := m.SerialLogging(t.Context(), a, s.device, SerialAction{ConnectionID: s.id, Action: "stop"}); err != nil {
		t.Fatal(err)
	}
	logs, err := m.ListSerialLogs(t.Context(), a, s.device, 0)
	if err != nil || logs.Logs[0].TXBytes != 2 || logs.Logs[0].Status != "completed" {
		t.Fatalf("undrained tail %+v %v", logs, err)
	}
}

func TestSerialQueuedSendHonorsCancellation(t *testing.T) {
	m, a, s := serialFixture(t)
	s.txGate = make(chan struct{}, 1)
	s.txGate <- struct{}{}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := m.SendSerial(ctx, a, s.device, SerialSend{ConnectionID: s.id, IdempotencyKey: "cancelled-send", Encoding: "ascii", Data: "a", Ending: "none"})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatalf("queued cancellation %v", err)
	}
	var count int
	if err = m.db().QueryRow(`SELECT count(*) FROM serial_sends`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unadmitted queued send reserved or written")
	}
}

func TestSerialRejectedDisableDoesNotStopConnection(t *testing.T) {
	m, a, s := serialFixture(t)
	for _, plugin := range []string{"camera", "serial"} {
		if _, err := m.SetDeviceEnabled(t.Context(), a, plugin, s.device, s.revision-1, false); err == nil || s.stopped {
			t.Fatalf("rejected disable %s stopped=%v err=%v", plugin, s.stopped, err)
		}
	}
}
