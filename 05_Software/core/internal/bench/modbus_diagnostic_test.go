package bench

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestModbusDiagnosticTransportAndTypedResult(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-diag", "mb-diag-bus"))
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision, profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID, kind: string(actor.Credential), credential: actor.SessionID, expires: time.Now().Add(time.Hour), cancel: func() {}}
	m.modbusSessions[e.ID] = s
	calls := 0
	dispatch := func(_ context.Context, _ *modbusSession, request ModbusRequest) (ModbusResult, error) {
		calls++
		if request.Function != 43 || request.Unit != 1 {
			t.Fatalf("unexpected diagnostic request: %+v", request)
		}
		return ModbusResult{Outcome: "acknowledged", Attempts: 1, Diagnostic: &ModbusDiagnosticData{Kind: "device_information", Objects: []ModbusIdentityObject{{ID: 0, ValueHex: "41636d65", Text: "Acme"}}}}, nil
	}
	if _, err := m.readModbusDiagnostic(t.Context(), actor, e.ID, ModbusDiagnosticRequest{SessionID: s.id, Kind: "comm_event_counter", Unit: 1}, dispatch); err == nil || calls != 0 {
		t.Fatalf("native TCP serial diagnostic dispatched: %v calls=%d", err, calls)
	}
	result, err := m.readModbusDiagnostic(t.Context(), actor, e.ID, ModbusDiagnosticRequest{SessionID: s.id, Kind: "device_information", Unit: 1}, dispatch)
	if err != nil || result.Diagnostic == nil || result.Diagnostic.Objects[0].Text != "Acme" || calls != 1 {
		t.Fatalf("identity: %+v %v calls=%d", result, err, calls)
	}
}
