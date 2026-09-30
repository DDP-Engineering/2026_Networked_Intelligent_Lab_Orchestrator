package bench

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/modbus"
)

type modbusLifecycleRunner struct {
	controlledRunner
	started   chan struct{}
	startOnce sync.Once
	calls     atomic.Int32
	writes    atomic.Int32
	failUnit  atomic.Int32
}

func (r *modbusLifecycleRunner) Execute(ctx context.Context, request protocol.ExecuteRequest, _ protocol.EventSink) (protocol.ExecuteResult, error) {
	if request.Operation != "serve" {
		return protocol.ExecuteResult{}, ErrInvalid
	}
	var params struct {
		Directory string `json:"workspace_directory"`
	}
	if json.Unmarshal(request.Parameters, &params) != nil || params.Directory == "" {
		return protocol.ExecuteResult{}, ErrInvalid
	}
	listener, err := net.Listen("unix", filepath.Join(params.Directory, "control.sock"))
	if err != nil {
		return protocol.ExecuteResult{}, err
	}
	defer listener.Close()
	if err = os.Chmod(filepath.Join(params.Directory, "control.sock"), 0600); err != nil {
		return protocol.ExecuteResult{}, err
	}
	r.startOnce.Do(func() { close(r.started) })
	go func() { <-ctx.Done(); _ = listener.Close() }()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return protocol.ExecuteResult{}, ctx.Err()
		}
		go func() {
			defer conn.Close()
			line, readErr := bufio.NewReader(conn).ReadBytes('\n')
			if readErr != nil {
				return
			}
			var command struct {
				RequestID string            `json:"request_id"`
				Function  int               `json:"function"`
				Unit      int               `json:"unit"`
				Count     int               `json:"count"`
				Values    []json.RawMessage `json:"values"`
			}
			if json.Unmarshal(line, &command) != nil {
				return
			}
			r.calls.Add(1)
			if command.Function >= 5 {
				r.writes.Add(1)
			}
			values := command.Values
			if command.Unit == int(r.failUnit.Load()) {
				answer, _ := json.Marshal(map[string]any{"request_id": command.RequestID, "outcome": "timeout", "attempts": 1, "values": []any{}})
				_, _ = conn.Write(append(answer, '\n'))
				return
			}
			if command.Function <= 4 {
				values = []json.RawMessage{json.RawMessage("42")}
			}
			answer, _ := json.Marshal(map[string]any{"request_id": command.RequestID, "outcome": "acknowledged", "attempts": 1, "values": values})
			_, _ = conn.Write(append(answer, '\n'))
		}()
	}
}

func modbusLifecycleFixture(t *testing.T, mode string) (*Manager, identity.Actor, ModbusConnect, *modbusLifecycleRunner) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("private Unix runner is a Linux appliance contract")
	}
	m, actor, _ := serialFixture(t)
	shortRoot, err := os.MkdirTemp("", "nilo-modbus-")
	if err != nil {
		t.Fatal(err)
	}
	m.options.DataDir = shortRoot
	t.Cleanup(func() { _ = m.Close(); _ = os.RemoveAll(shortRoot) })
	installModbusFoundation(t, m)
	source := filepath.Join("..", "..", "..", "integrations", "modbus")
	target := filepath.Join(shortRoot, "integrations", "modbus")
	files := map[string]string{}
	for _, name := range []string{"manifest.json", "runner", "schemas/plugin.schema.json", "schemas/device.schema.json", "schemas/connect.input.schema.json", "schemas/serve.input.schema.json", "schemas/result.schema.json", "schemas/event.schema.json"} {
		content, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(target, name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, content, 0700); err != nil {
			t.Fatal(err)
		}
		files[name] = fmt.Sprintf("%x", sha256.Sum256(content))
	}
	index, _ := json.Marshal(files)
	if err = os.WriteFile(filepath.Join(target, "files.sha256.json"), index, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := verifyPackage(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.db().Exec(`UPDATE bench_plugins SET digest=?,mode=? WHERE id='modbus'`, digest, mode); err != nil {
		t.Fatal(err)
	}
	runner := &modbusLifecycleRunner{started: make(chan struct{})}
	m.options.runner = func(Plugin) (supervisor.Supervisor, error) { return runner, nil }
	e, p := saveConsumerDefinitions(t, m, actor, networkDefinition("mb-live", "mb-live-bus"))
	input := ModbusConnect{EndpointRevision: e.Revision, ProfileID: p.ID, ProfileRevision: p.Revision, IdempotencyKey: "modbus-live-connect", TimeoutSeconds: 10, Groups: []ModbusPollGroup{{ID: "holding", IntervalMS: 1000, Requests: []ModbusRequest{{Function: 3, Unit: 1, Address: 10, Count: 1}, {Function: 3, Unit: 2, Address: 10, Count: 1}}}}}
	return m, actor, input, runner
}

func TestModbusConnectionTestClosesTemporaryRunnerWithoutPDU(t *testing.T) {
	m, actor, input, runner := modbusLifecycleFixture(t, "auto")
	input.Groups = nil
	input.IdempotencyKey = "modbus-test-connection"
	job, err := m.TestModbusConnection(t.Context(), actor, "mb-live", input)
	if err != nil {
		t.Fatal(err)
	}
	finished := awaitJob(t, m, actor, job.ID, "completed")
	if !json.Valid(finished.Result) || runner.calls.Load() != 0 || runner.writes.Load() != 0 {
		t.Fatalf("test result %s; PDU calls %d/%d", finished.Result, runner.calls.Load(), runner.writes.Load())
	}
	status, err := m.ModbusSession(t.Context(), actor, "mb-live")
	if err != nil || status.Connected || status.State == "connected" {
		t.Fatalf("temporary session retained: %+v, %v", status, err)
	}
	m.mu.Lock()
	remaining := len(m.modbusSessions) + len(m.transports)
	m.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("temporary resources retained: %d", remaining)
	}
}

func TestModbusConnectionTestWaitsForAskApproval(t *testing.T) {
	m, actor, input, runner := modbusLifecycleFixture(t, "ask")
	input.Groups = nil
	input.IdempotencyKey = "modbus-test-ask"
	job, err := m.TestModbusConnection(t.Context(), actor, "mb-live", input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "pending_approval")
	select {
	case <-runner.started:
		t.Fatal("test runner opened before policy approval")
	default:
	}
	if _, err := m.Action(t.Context(), actor, job.ID, "approve"); err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "completed")
	if runner.calls.Load() != 0 {
		t.Fatal("test issued Modbus command")
	}
	status, err := m.ModbusSession(t.Context(), actor, "mb-live")
	if err != nil || status.Connected {
		t.Fatalf("test left session: %+v %v", status, err)
	}
}

func TestModbusSettingsChangeOnlyNewSessionAdmissions(t *testing.T) {
	m, actor, firstInput, _ := modbusLifecycleFixture(t, "auto")
	ctx := t.Context()
	other := networkDefinition("mb-other", "mb-other-bus")
	other.Network.Host = "other.local"
	other, err := m.SaveModbusEndpoint(ctx, actor, 0, other)
	if err != nil {
		t.Fatal(err)
	}
	otherProfile, err := m.SaveModbusProfile(ctx, actor, 0, profileDefinition("mb-other-profile", other.ID))
	if err != nil {
		t.Fatal(err)
	}
	settings, err := m.ModbusSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	short := settings.Settings
	short.MaxSessions, short.SessionLeaseSeconds, short.IdleSeconds = 1, 60, 30
	settings, err = m.ReplaceModbusSettings(ctx, actor, settings.Revision, short)
	if err != nil {
		t.Fatal(err)
	}
	firstInput.Groups = nil
	first, err := m.ConnectModbus(ctx, actor, "mb-live", firstInput)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, first.ID, "completed")
	m.mu.Lock()
	firstSession := m.modbusSessions["mb-live"]
	if firstSession == nil {
		m.mu.Unlock()
		t.Fatal("first session missing")
	}
	originalExpiry, originalIdle := firstSession.expires, firstSession.idleAfter
	m.mu.Unlock()
	if originalIdle != 30*time.Second || time.Until(originalExpiry) > 60*time.Second || time.Until(originalExpiry) < 50*time.Second {
		t.Fatalf("new-session lifetime = %v / %v", originalExpiry, originalIdle)
	}
	secondInput := ModbusConnect{EndpointRevision: other.Revision, ProfileID: otherProfile.ID, ProfileRevision: otherProfile.Revision, IdempotencyKey: "mb-other-first", TimeoutSeconds: 10}
	second, err := m.ConnectModbus(ctx, actor, other.ID, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, second.ID, "incomplete")
	short.MaxSessions, short.SessionLeaseSeconds, short.IdleSeconds = 2, 120, 60
	if _, err := m.ReplaceModbusSettings(ctx, actor, settings.Revision, short); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	if firstSession.expires != originalExpiry || firstSession.idleAfter != originalIdle || firstSession.stopped {
		m.mu.Unlock()
		t.Fatal("settings change altered active session")
	}
	m.mu.Unlock()
	secondInput.IdempotencyKey = "mb-other-second"
	second, err = m.ConnectModbus(ctx, actor, other.ID, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, second.ID, "completed")
	m.mu.Lock()
	secondSession := m.modbusSessions[other.ID]
	m.mu.Unlock()
	if secondSession == nil || secondSession.idleAfter != 60*time.Second || time.Until(secondSession.expires) < 110*time.Second {
		t.Fatalf("new admission ignored raised limits: %+v", secondSession)
	}
}

func TestModbusFiniteApprovalAndLiveRunner(t *testing.T) {
	m, actor, input, runner := modbusLifecycleFixture(t, "ask")
	job, err := m.ConnectModbus(t.Context(), actor, "mb-live", input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "pending_approval")
	select {
	case <-runner.started:
		t.Fatal("runner opened before human approval")
	default:
	}
	tokenActor := actor
	tokenActor.Credential = identity.CredentialToken
	tokenActor.TokenID = "token-cannot-approve"
	tokenActor.SessionID = ""
	if _, err = m.Action(t.Context(), tokenActor, job.ID, "approve"); err == nil {
		t.Fatal("token approved Ask admission")
	}
	if _, err = m.Action(t.Context(), actor, job.ID, "approve"); err != nil {
		t.Fatal(err)
	}
	finished := awaitJob(t, m, actor, job.ID, "completed")
	var admission struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(finished.Result, &admission) != nil || admission.SessionID == "" {
		t.Fatalf("admission result: %s", finished.Result)
	}
	status, err := m.ModbusSession(t.Context(), actor, "mb-live")
	if err != nil || !status.Connected || status.SessionID != admission.SessionID {
		t.Fatalf("status %+v %v", status, err)
	}
	result, err := m.ReadModbus(t.Context(), actor, "mb-live", admission.SessionID, ModbusRequest{Function: 3, Unit: 1, Address: 10, Count: 1})
	if err != nil || result.Outcome != "acknowledged" {
		t.Fatalf("read %+v %v", result, err)
	}
	if _, err = m.ReadModbus(t.Context(), actor, "mb-live", admission.SessionID, ModbusRequest{Function: 3, Unit: 1, Address: 11, Count: 1}); err == nil {
		t.Fatal("unapproved read plan change")
	}
	write := ModbusWrite{SessionID: admission.SessionID, IdempotencyKey: "live-write-one", Request: ModbusRequest{Function: 6, Unit: 1, Address: 20, Count: 1, Values: []int{42}}}
	command, err := m.WriteModbus(t.Context(), actor, "mb-live", write)
	if err != nil || command.Outcome != "acknowledged" {
		t.Fatalf("live write %+v %v", command, err)
	}
	beforeDuplicate := runner.writes.Load()
	repeated, err := m.WriteModbus(t.Context(), actor, "mb-live", write)
	if err != nil || repeated.ID != command.ID || runner.writes.Load() != beforeDuplicate {
		t.Fatalf("live duplicate %+v %v", repeated, err)
	}
	page, err := m.ListModbusCommands(t.Context(), actor, "mb-live", admission.SessionID, 0)
	if err != nil || len(page.Commands) != 1 || page.Commands[0].ID != command.ID {
		t.Fatalf("command page %+v %v", page, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var events ModbusEventPage
	for time.Now().Before(deadline) {
		events, err = m.ReadModbusEvents(t.Context(), actor, "mb-live", admission.SessionID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(events.Events) >= 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(events.Events) < 2 || events.Events[0].Request.Unit != 1 || events.Events[1].Request.Unit != 2 || events.NextCursor == 0 {
		t.Fatalf("poll events %+v", events)
	}
	status, err = m.SetModbusDisplayPaused(t.Context(), actor, "mb-live", admission.SessionID, true)
	if err != nil || !status.DisplayPaused || status.PollPaused {
		t.Fatalf("display pause %+v %v", status, err)
	}
	status, err = m.SetModbusPolling(t.Context(), actor, "mb-live", admission.SessionID, input.Groups, true)
	if err != nil || !status.PollPaused || !status.DisplayPaused {
		t.Fatalf("poll pause %+v %v", status, err)
	}
	m.mu.Lock()
	claim := m.modbusSessions["mb-live"].claim
	m.mu.Unlock()
	if err = m.DisconnectModbus(t.Context(), actor, "mb-live", admission.SessionID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-claim.done:
	case <-time.After(3 * time.Second):
		t.Fatal("runner cleanup did not release claim")
	}
	if claim.held {
		t.Fatal("claim still held after cleanup")
	}
}

func TestModbusPollGroupsPauseIndependently(t *testing.T) {
	m, actor, input, _ := modbusLifecycleFixture(t, "auto")
	input.Groups = []ModbusPollGroup{
		{ID: "first", IntervalMS: 100, Requests: []ModbusRequest{{Function: 3, Unit: 1, Address: 10, Count: 1}}},
		{ID: "second", IntervalMS: 100, Requests: []ModbusRequest{{Function: 3, Unit: 2, Address: 10, Count: 1}}},
	}
	job, err := m.ConnectModbus(t.Context(), actor, "mb-live", input)
	if err != nil {
		t.Fatal(err)
	}
	finished := awaitJob(t, m, actor, job.ID, "completed")
	var admission struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(finished.Result, &admission) != nil || admission.SessionID == "" {
		t.Fatalf("admission: %s", finished.Result)
	}
	status, err := m.SetModbusPollGroupPaused(t.Context(), actor, "mb-live", admission.SessionID, "first", true)
	if err != nil || !status.GroupStats["first"].Paused || status.GroupStats["second"].Paused {
		t.Fatalf("pause status: %+v %v", status, err)
	}
	if _, err := m.SetModbusPollGroupPaused(t.Context(), actor, "mb-live", admission.SessionID, "unknown", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown group: %v", err)
	}
	time.Sleep(350 * time.Millisecond)
	page, err := m.ReadModbusEvents(t.Context(), actor, "mb-live", admission.SessionID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var first, second int
	for _, event := range page.Events {
		if event.GroupID == "first" {
			first++
		}
		if event.GroupID == "second" {
			second++
		}
	}
	if first > 1 || second < 2 {
		t.Fatalf("independent polling: first=%d second=%d", first, second)
	}
	status, err = m.ModbusSession(t.Context(), actor, "mb-live")
	if err != nil || status.GroupStats["second"].Samples < 2 || status.GroupStats["second"].RequestedIntervalMS != 100 {
		t.Fatalf("group metrics: %+v %v", status, err)
	}
	if _, err = m.SetModbusPollGroupPaused(t.Context(), actor, "mb-live", admission.SessionID, "first", false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		page, err = m.ReadModbusEvents(t.Context(), actor, "mb-live", admission.SessionID, page.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range page.Events {
			if event.GroupID == "first" {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("resumed group produced no result")
}

func TestModbusConnectPinsSavedWorkspacePlan(t *testing.T) {
	m, actor, input, _ := modbusLifecycleFixture(t, "auto")
	input.Groups = []ModbusPollGroup{{ID: "holding", IntervalMS: 1000, Requests: []ModbusRequest{{Function: 3, Unit: 1, Address: 10, Count: 1}}}}
	mapped, err := m.SaveModbusMap(t.Context(), actor, 0, modbus.Map{SchemaVersion: 1, ID: "registers", Name: "Registers", Points: []modbus.Point{}})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := m.SaveModbusWorkspace(t.Context(), actor, 0, modbus.Workspace{SchemaVersion: 1, ID: "plant", Name: "Plant", EndpointID: "mb-live", ProfileRef: modbus.RevisionRef{ID: input.ProfileID, Revision: input.ProfileRevision}, MapRefs: []modbus.RevisionRef{{ID: mapped.ID, Revision: mapped.Revision}}, PollGroups: []modbus.PollGroup{{ID: "holding", Name: "Holding", Enabled: true, UnitID: 1, Function: 3, Address: 10, Quantity: 1, IntervalMS: 1000, MapRef: modbus.RevisionRef{ID: mapped.ID, Revision: mapped.Revision}}}, Layout: modbus.Layout{Columns: []string{}, AddressBase: "zero", Series: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	input.WorkspaceID, input.WorkspaceRevision = workspace.ID, workspace.Revision
	changed := input
	changed.Groups = []ModbusPollGroup{{ID: "holding", IntervalMS: 1000, Requests: []ModbusRequest{{Function: 3, Unit: 1, Address: 10, Count: 1}}}}
	changed.Groups[0].Requests[0].Address = 11
	changed.IdempotencyKey = "changed-plan"
	if _, err = m.ConnectModbus(t.Context(), actor, "mb-live", changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed saved plan: %v", err)
	}
	job, err := m.ConnectModbus(t.Context(), actor, "mb-live", input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "completed")
	status, err := m.ModbusSession(t.Context(), actor, "mb-live")
	if err != nil || status.WorkspaceID != "plant" || status.WorkspaceRevision != 1 {
		t.Fatalf("pinned workspace: %+v %v", status, err)
	}
}

func TestModbusPollGroupStopsOnErrorWithoutStoppingPeer(t *testing.T) {
	m, actor, input, runner := modbusLifecycleFixture(t, "auto")
	runner.failUnit.Store(1)
	input.Groups = []ModbusPollGroup{
		{ID: "failed", IntervalMS: 100, TimeoutMS: 200, StopOnError: true, Requests: []ModbusRequest{{Function: 3, Unit: 1, Address: 10, Count: 1}}},
		{ID: "healthy", IntervalMS: 100, Requests: []ModbusRequest{{Function: 3, Unit: 2, Address: 10, Count: 1}}},
	}
	job, err := m.ConnectModbus(t.Context(), actor, "mb-live", input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "completed")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, err := m.ModbusSession(t.Context(), actor, "mb-live")
		if err != nil {
			t.Fatal(err)
		}
		if status.GroupStats["failed"].Paused && status.GroupStats["healthy"].Samples > 1 {
			if status.GroupStats["failed"].Errors != 1 {
				t.Fatalf("failed group retried after stop: %+v", status.GroupStats["failed"])
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("failed group did not pause while peer advanced")
}

func TestModbusDisableStopsLiveLease(t *testing.T) {
	m, actor, input, _ := modbusLifecycleFixture(t, "auto")
	job, err := m.ConnectModbus(t.Context(), actor, "mb-live", input)
	if err != nil {
		t.Fatal(err)
	}
	awaitJob(t, m, actor, job.ID, "completed")
	m.mu.Lock()
	claim := m.modbusSessions["mb-live"].claim
	m.mu.Unlock()
	if _, err = m.db().Exec(`UPDATE bench_plugins SET enabled=0 WHERE id='modbus'`); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ModbusSession(t.Context(), actor, "mb-live"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled plugin retained live authority: %v", err)
	}
	select {
	case <-claim.done:
	case <-time.After(3 * time.Second):
		t.Fatal("disabled plugin retained transport claim")
	}
}
