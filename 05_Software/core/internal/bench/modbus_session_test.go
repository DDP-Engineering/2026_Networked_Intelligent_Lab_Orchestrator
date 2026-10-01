package bench

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestModbusCanonicalRequestBounds(t *testing.T) {
	for _, test := range []struct {
		request ModbusRequest
		valid   bool
	}{
		{ModbusRequest{Function: 3, Unit: 1, Address: 0, Count: 125}, true},
		{ModbusRequest{Function: 3, Unit: 1, Address: 0, Count: 126}, false},
		{ModbusRequest{Function: 16, Unit: 1, Address: 0, Count: 2, Values: []int{7, 8}}, true},
		{ModbusRequest{Function: 16, Unit: 1, Address: 0, Count: 2, Values: []int{7}}, false},
		{ModbusRequest{Function: 6, Unit: 1, Address: 65535, Count: 1, Values: []int{65535}}, true},
		{ModbusRequest{Function: 6, Unit: 1, Address: 65535, Count: 2, Values: []int{1}}, false},
		{ModbusRequest{Function: 5, Unit: 1, Address: 1, Count: 1, Values: []int{1}}, true},
		{ModbusRequest{Function: 5, Unit: 1, Address: 1, Count: 1, Values: []int{2}}, false},
		{ModbusRequest{Function: 23, Unit: 1, Address: 1, Count: 1}, false},
	} {
		_, err := canonicalModbusRequest(test.request)
		if (err == nil) != test.valid {
			t.Errorf("request %+v: valid=%v error=%v", test.request, test.valid, err)
		}
	}
}

func TestModbusConnectValidation(t *testing.T) {
	input := Submission{PluginID: "modbus", Operation: "connect", DeviceID: "gateway", IdempotencyKey: "modbus-connect-one", Parameters: json.RawMessage(`{"endpoint_revision":1,"profile_id":"profile-gateway","profile_revision":2}`)}
	if _, err := modbusCanonicalSubmission(input); err != nil {
		t.Fatal(err)
	}
	input.Operation = "serve"
	if _, err := modbusCanonicalSubmission(input); err == nil {
		t.Fatal("private runner operation publicly admitted")
	}
	input.Operation = "connect"
	input.DeviceID = "../ttyUSB0"
	if _, err := modbusCanonicalSubmission(input); err == nil {
		t.Fatal("path as endpoint ID admitted")
	}
}

func TestModbusConnectionTestUsesCanonicalReadFreeIntent(t *testing.T) {
	input := Submission{PluginID: "modbus", Operation: "connect", DeviceID: "gateway", IdempotencyKey: "modbus-test-one", Parameters: json.RawMessage(`{"endpoint_revision":1,"profile_id":"profile-gateway","profile_revision":2,"test_only":true}`)}
	canonical, err := modbusCanonicalSubmission(input)
	if err != nil || !json.Valid(canonical.Parameters) {
		t.Fatalf("test intent: %s %v", canonical.Parameters, err)
	}
	input.Parameters = json.RawMessage(`{"endpoint_revision":1,"profile_id":"profile-gateway","profile_revision":2,"test_only":true,"groups":[{"id":"holding","interval_ms":1000,"requests":[{"function":3,"unit":1,"address":10,"count":1}]}]}`)
	if _, err := modbusCanonicalSubmission(input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("test with polling admitted: %v", err)
	}
}

func TestModbusConnectPinsReadPlanForApproval(t *testing.T) {
	input := Submission{PluginID: "modbus", Operation: "connect", DeviceID: "gateway", IdempotencyKey: "modbus-plan-key", Parameters: json.RawMessage(`{"endpoint_revision":1,"profile_id":"profile-gateway","profile_revision":2,"groups":[{"id":"holding","interval_ms":1000,"requests":[{"function":3,"unit":1,"address":10,"count":2,"values":[]}]}]}`)}
	canonical, err := modbusCanonicalSubmission(input)
	if err != nil || len(canonical.Parameters) == 0 {
		t.Fatalf("plan admission: %s %v", canonical.Parameters, err)
	}
	input.Parameters = json.RawMessage(`{"endpoint_revision":1,"profile_id":"profile-gateway","profile_revision":2,"groups":[{"id":"bad","interval_ms":1,"requests":[{"function":3,"unit":1,"address":10,"count":2,"values":[]}]}]}`)
	if _, err := modbusCanonicalSubmission(input); err == nil {
		t.Fatal("unbounded plan admitted")
	}
}

func TestModbusReadPlanMatchesExactCanonicalRequest(t *testing.T) {
	groups, err := canonicalModbusPollGroups([]ModbusPollGroup{{ID: "holding", IntervalMS: 1000, Requests: []ModbusRequest{{Function: 3, Unit: 1, Address: 10, Count: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	request, err := canonicalModbusRequest(ModbusRequest{Function: 3, Unit: 1, Address: 10, Count: 2})
	if err != nil || !modbusRequestInPlan(groups, request) {
		t.Fatal("approved read missing")
	}
	request.Address = 11
	if modbusRequestInPlan(groups, request) {
		t.Fatal("unapproved address admitted")
	}
}

func TestModbusPollPlanBounds(t *testing.T) {
	valid := []ModbusPollGroup{{ID: "holding", IntervalMS: 100, Requests: []ModbusRequest{{Function: 3, Unit: 1, Address: 0, Count: 2}}}}
	if err := validateModbusPollGroups(valid); err != nil {
		t.Fatal(err)
	}
	valid[0].IntervalMS = 99
	if err := validateModbusPollGroups(valid); err == nil {
		t.Fatal("poll interval below minimum accepted")
	}
	valid[0].IntervalMS = 100
	valid[0].Requests[0].Function = 6
	valid[0].Requests[0].Values = []int{1}
	if err := validateModbusPollGroups(valid); err == nil {
		t.Fatal("poll plan accepted a write")
	}
}

func TestModbusWriteReservationNoReplay(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-write", "mb-write-bus"))
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision, profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID, kind: string(actor.Credential), credential: actor.SessionID, expires: time.Now().Add(time.Hour), cancel: func() {}}
	m.modbusSessions[e.ID] = s
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := m.db().Exec(`INSERT INTO modbus_sessions(id,endpoint_id,endpoint_revision,profile_id,profile_revision,owner_id,credential_kind,credential_id,started_at,expires_at,state) VALUES(?,?,?,?,?,?,?,?,?,?,'active')`, s.id, e.ID, e.Revision, p.ID, p.Revision, s.owner, s.kind, s.credential, stamp, s.expires.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	dispatch := func(context.Context, *modbusSession, ModbusRequest) (ModbusResult, error) {
		calls.Add(1)
		return ModbusResult{Outcome: "acknowledged", Attempts: 1, Values: []json.RawMessage{json.RawMessage("42")}}, nil
	}
	input := ModbusWrite{SessionID: s.id, IdempotencyKey: "modbus-write-one", Request: ModbusRequest{Function: 6, Unit: 1, Address: 10, Count: 1, Values: []int{42}}}
	first, err := m.writeModbus(t.Context(), actor, e.ID, input, dispatch)
	if err != nil || first.Outcome != "acknowledged" {
		t.Fatalf("first write: %+v %v", first, err)
	}
	readBack, err := m.ModbusCommand(t.Context(), actor, first.ID)
	if err != nil || readBack.ID != first.ID || readBack.Digest != first.Digest {
		t.Fatalf("durable command lookup: %+v %v", readBack, err)
	}
	second, err := m.writeModbus(t.Context(), actor, e.ID, input, dispatch)
	if err != nil || second.ID != first.ID || calls.Load() != 1 {
		t.Fatalf("duplicate replay: %+v %v calls=%d", second, err, calls.Load())
	}
	input.Request.Values[0] = 43
	if _, err = m.writeModbus(t.Context(), actor, e.ID, input, dispatch); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry: %v", err)
	}
	input.IdempotencyKey = "modbus-write-two"
	unknown := func(context.Context, *modbusSession, ModbusRequest) (ModbusResult, error) {
		calls.Add(1)
		return ModbusResult{}, ErrUnavailable
	}
	result, err := m.writeModbus(t.Context(), actor, e.ID, input, unknown)
	if err != nil || result.Outcome != "delivery_unknown" {
		t.Fatalf("lost reply: %+v %v", result, err)
	}
	result, err = m.writeModbus(t.Context(), actor, e.ID, input, unknown)
	if err != nil || result.Outcome != "delivery_unknown" || calls.Load() != 2 {
		t.Fatalf("unknown replay: %+v %v calls=%d", result, err, calls.Load())
	}
	delete(m.modbusSessions, e.ID)
	result, err = m.writeModbus(t.Context(), actor, e.ID, input, unknown)
	if err != nil || result.Outcome != "delivery_unknown" || calls.Load() != 2 {
		t.Fatalf("restart replay: %+v %v calls=%d", result, err, calls.Load())
	}
	m.modbusSessions[e.ID] = s
	other := actor
	other.SessionID = uuid.NewString()
	if _, err = m.writeModbus(t.Context(), other, e.ID, input, unknown); !errors.Is(err, ErrForbidden) {
		t.Fatalf("other credential reused command: %v", err)
	}
	if _, err = m.db().Exec(`UPDATE user_sessions SET revoked_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), actor.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ModbusSession(t.Context(), actor, e.ID); !errors.Is(err, ErrForbidden) || !s.stopped {
		t.Fatalf("revoked session still active: %v stopped=%v", err, s.stopped)
	}
}

func TestModbusPrivateSocketUsesCanonicalCoilValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modbus.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("Unix socket unavailable: %v", err)
	}
	defer listener.Close()
	defer os.Remove(path)
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		var request struct {
			RequestID string `json:"request_id"`
			Values    []bool `json:"values"`
			Function  int    `json:"function"`
		}
		if err = json.NewDecoder(conn).Decode(&request); err != nil {
			done <- err
			return
		}
		if request.Function != 5 || len(request.Values) != 1 || !request.Values[0] {
			done <- ErrInvalid
			return
		}
		response, _ := json.Marshal(map[string]any{"request_id": request.RequestID, "outcome": "acknowledged", "values": []bool{true}, "attempts": 1})
		_, err = conn.Write(append(response, '\n'))
		done <- err
	}()
	s := &modbusSession{socket: path}
	result, err := modbusPrivateCall(t.Context(), s, ModbusRequest{Function: 5, Unit: 1, Address: 0, Count: 1, Values: []int{1}}, "")
	if err != nil || result.Outcome != "acknowledged" || len(result.Values) != 1 || string(result.Values[0]) != "true" {
		t.Fatalf("result %+v %v", result, err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestModbusResultRejectsWrongWriteEcho(t *testing.T) {
	request := ModbusRequest{Function: 6, Unit: 1, Address: 10, Count: 1, Values: []int{42}}
	if validModbusResult(request, ModbusResult{Outcome: "acknowledged", Attempts: 1, Values: []json.RawMessage{json.RawMessage("43")}}) {
		t.Fatal("wrong write echo accepted")
	}
	if !validModbusResult(request, ModbusResult{Outcome: "delivery_unknown", Attempts: 1, Values: []json.RawMessage{}}) {
		t.Fatal("unknown delivery was lost")
	}
}

func TestModbusResultAcceptsBoundedCapturedFrame(t *testing.T) {
	request := ModbusRequest{Function: 3, Unit: 1, Address: 10, Count: 1}
	result := ModbusResult{RequestID: "one", Outcome: "acknowledged", Attempts: 1, Values: []json.RawMessage{json.RawMessage("42")}, Frames: []ModbusTraceFrame{{RequestID: "one", Direction: "tx", DataHex: "000102"}}}
	if !validModbusResult(request, result) {
		t.Fatal("bounded actual frame rejected")
	}
	result.Frames[0].DataHex = "not-hex"
	if validModbusResult(request, result) {
		t.Fatal("invalid frame accepted")
	}
}

func TestModbusWritePreviewPDUAndDigest(t *testing.T) {
	request := ModbusRequest{Function: 6, Unit: 1, Address: 10, Count: 1, Values: []int{42}}
	pdu, err := modbusPDU(request)
	if err != nil || string(pdu) != string([]byte{6, 0, 10, 0, 42}) {
		t.Fatalf("PDU %x %v", pdu, err)
	}
	digest, _ := modbusWriteDigest("session", 1, 2, request)
	changed, _ := modbusWriteDigest("session", 1, 2, ModbusRequest{Function: 6, Unit: 1, Address: 10, Count: 1, Values: []int{43}})
	if digest == changed {
		t.Fatal("write value absent from digest")
	}
}
