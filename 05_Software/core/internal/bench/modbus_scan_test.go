package bench

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/auth"
)

func TestModbusScanExportPreservesFailedProbes(t *testing.T) {
	status := ModbusScanStatus{ID: "scan-1", State: "completed", Total: 2, Completed: 2, Plan: ModbusScanPlan{SessionID: "live", Function: 3, UnitFrom: 1, UnitTo: 1, AddressFrom: 10, AddressTo: 11, RatePerSecond: 2, DeadlineSeconds: 5}, Results: []ModbusScanProbe{
		{Function: 3, Unit: 1, Address: 10, Outcome: "acknowledged", Values: []json.RawMessage{json.RawMessage("42")}},
		{Function: 3, Unit: 1, Address: 11, Outcome: "timeout", Values: []json.RawMessage{}},
	}}
	csv, err := renderModbusScan(status, "csv")
	if err != nil {
		t.Fatal(err)
	}
	records, err := csvReader(csv)
	if err != nil || len(records) != 4 || records[1][0] != "plan" || !strings.Contains(records[1][len(records[1])-1], `"address_to":11`) || records[3][6] != "timeout" || records[3][7] != "" {
		t.Fatalf("CSV failed probe/plan: %q %v", csv, err)
	}
	jsonl, err := renderModbusScan(status, "jsonl")
	if err != nil || len(jsonl) == 0 || string(jsonl[len(jsonl)-1]) != "\n" || !bytes.Contains(jsonl, []byte(`"kind":"plan"`)) {
		t.Fatalf("JSONL export: %q %v", jsonl, err)
	}
}

func TestModbusScanCancelStopsFutureProbes(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-cancel", "mb-cancel-bus"))
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision, profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID, kind: string(actor.Credential), credential: actor.SessionID, expires: time.Now().Add(time.Hour), cancel: func() {}}
	m.modbusSessions[e.ID] = s
	started := make(chan struct{})
	var calls atomic.Int32
	dispatch := func(ctx context.Context, _ *modbusSession, _ ModbusRequest) (ModbusResult, error) {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		return ModbusResult{}, ctx.Err()
	}
	plan := ModbusScanPlan{SessionID: s.id, Function: 3, UnitFrom: 1, UnitTo: 1, AddressFrom: 0, AddressTo: 2, RatePerSecond: 10, DeadlineSeconds: 5}
	if _, err := m.startModbusScan(t.Context(), actor, e.ID, plan, dispatch); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first probe did not start")
	}
	status, err := m.CancelModbusScan(t.Context(), actor, e.ID, s.id)
	if err != nil || status.State != "cancelled" {
		t.Fatalf("cancel: %+v %v", status, err)
	}
	m.modbusWorkers.Wait()
	status, err = m.ModbusScan(t.Context(), actor, e.ID, s.id)
	if err != nil || calls.Load() != 1 || status.Completed != 0 {
		t.Fatalf("future probe after cancel: %+v %v calls=%d", status, err, calls.Load())
	}
}

func csvReader(raw []byte) ([][]string, error) { return csv.NewReader(bytes.NewReader(raw)).ReadAll() }

func TestModbusScanPlanBounds(t *testing.T) {
	valid := ModbusScanPlan{Function: 3, UnitFrom: 1, UnitTo: 2, AddressFrom: 10, AddressTo: 11, RatePerSecond: 5, DeadlineSeconds: 20}
	if probes, err := validateModbusScanPlan(valid); err != nil || probes != 4 {
		t.Fatalf("valid scan: probes=%d err=%v", probes, err)
	}
	for _, change := range []func(*ModbusScanPlan){
		func(p *ModbusScanPlan) { p.Function = 6 },
		func(p *ModbusScanPlan) { p.UnitTo = 256 },
		func(p *ModbusScanPlan) { p.AddressTo = 65536 },
		func(p *ModbusScanPlan) { p.RatePerSecond = 11 },
		func(p *ModbusScanPlan) { p.DeadlineSeconds = 61 },
		func(p *ModbusScanPlan) { p.UnitFrom = 3 },
	} {
		bad := valid
		change(&bad)
		if _, err := validateModbusScanPlan(bad); err == nil {
			t.Fatalf("accepted invalid plan: %+v", bad)
		}
	}
}

func TestModbusScanResultsStayWithOpeningCredential(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-private-scan", "mb-private-scan-bus"))
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision, profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID, kind: string(actor.Credential), credential: actor.SessionID, expires: time.Now().Add(time.Hour), cancel: func() {}}
	s.scan = &modbusScanState{status: ModbusScanStatus{ID: "private", SessionID: s.id, State: "completed", Results: []ModbusScanProbe{{Unit: 1, Address: 2, Function: 3, Outcome: "acknowledged"}}}}
	m.modbusSessions[e.ID] = s
	actor.Permissions = []auth.Permission{auth.PermissionBenchRead, auth.PermissionBenchExecute}
	otherCredential := actor
	otherCredential.SessionID = uuid.NewString()
	if _, err := m.ModbusScan(t.Context(), otherCredential, e.ID, s.id); !errors.Is(err, ErrForbidden) {
		t.Fatalf("scan status leaked to a different credential: %v", err)
	}
	if _, err := m.ExportModbusScan(t.Context(), otherCredential, e.ID, s.id, "jsonl"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("scan export leaked to a different credential: %v", err)
	}
	if _, err := m.ModbusScan(t.Context(), actor, e.ID, s.id); err != nil {
		t.Fatalf("opening credential lost scan access: %v", err)
	}
}

func TestModbusScanResultsPageKeepsFailedProbe(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-paged-scan", "mb-paged-scan-bus"))
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision, profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID, kind: string(actor.Credential), credential: actor.SessionID, expires: time.Now().Add(time.Hour), cancel: func() {}}
	results := make([]ModbusScanProbe, 101)
	for address := range results {
		results[address] = ModbusScanProbe{Unit: 1, Address: address, Function: 3, Outcome: "acknowledged"}
	}
	results[100].Outcome = "timeout"
	s.scan = &modbusScanState{status: ModbusScanStatus{ID: "paged", SessionID: s.id, State: "completed", Total: 101, Completed: 101, Results: results}}
	m.modbusSessions[e.ID] = s
	first, err := m.ModbusScanResults(t.Context(), actor, e.ID, s.id, 0)
	if err != nil || len(first.Results) != 100 || first.NextOffset != 100 || first.Total != 101 {
		t.Fatalf("first page: %+v %v", first, err)
	}
	last, err := m.ModbusScanResults(t.Context(), actor, e.ID, s.id, first.NextOffset)
	if err != nil || len(last.Results) != 1 || last.Results[0].Outcome != "timeout" || last.NextOffset != 101 {
		t.Fatalf("failed probe lost on next page: %+v %v", last, err)
	}
}

func TestModbusScanSettlesFiniteMixedProbes(t *testing.T) {
	m, actor, _ := serialFixture(t)
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-scan", "mb-scan-bus"))
	s := &modbusSession{id: uuid.NewString(), endpointID: e.ID, endpointRevision: e.Revision, profileID: p.ID, profileRevision: p.Revision, owner: actor.User.ID, kind: string(actor.Credential), credential: actor.SessionID, expires: time.Now().Add(time.Hour), cancel: func() {}}
	m.modbusSessions[e.ID] = s
	plan := ModbusScanPlan{SessionID: s.id, Function: 3, UnitFrom: 1, UnitTo: 1, AddressFrom: 10, AddressTo: 11, RatePerSecond: 10, DeadlineSeconds: 5}
	dispatch := func(_ context.Context, _ *modbusSession, request ModbusRequest) (ModbusResult, error) {
		if request.Address == 11 {
			return ModbusResult{}, context.DeadlineExceeded
		}
		return ModbusResult{RequestID: "probe", Outcome: "acknowledged", Attempts: 1, Values: []json.RawMessage{json.RawMessage("42")}, Frames: []ModbusTraceFrame{{RequestID: "probe", Direction: "tx", DataHex: "03000a0001"}, {RequestID: "probe", Direction: "rx", DataHex: "0302002a"}}}, nil
	}
	started, err := m.startModbusScan(t.Context(), actor, e.ID, plan, dispatch)
	if err != nil || started.Total != 2 {
		t.Fatalf("scan start: %+v %v", started, err)
	}
	deadline := time.After(2 * time.Second)
	for {
		status, err := m.ModbusScan(t.Context(), actor, e.ID, s.id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State != "running" {
			if status.State != "completed" || status.Completed != 2 || status.Results[0].Outcome != "acknowledged" || status.Results[1].Outcome != "timeout" || len(status.Results[0].Frames) != 2 {
				t.Fatalf("untruthful scan status: %+v", status)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("scan did not settle")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
