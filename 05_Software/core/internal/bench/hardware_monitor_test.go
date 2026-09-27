package bench

import (
	"context"
	"encoding/json"
	"testing"

	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
)

type inventoryRunner struct {
	controlledRunner
	raw json.RawMessage
	err error
}

func (r *inventoryRunner) Execute(context.Context, protocol.ExecuteRequest, protocol.EventSink) (protocol.ExecuteResult, error) {
	return protocol.ExecuteResult{Status: "completed", Data: r.raw}, r.err
}

func TestBackgroundHardwareRefreshRemovesDisconnectedPort(t *testing.T) {
	m, _, _, _ := serialLifecycleFixture(t, "auto")
	runner := &inventoryRunner{raw: json.RawMessage(`{"devices":[]}`)}
	m.options.runner = func(Plugin) (supervisor.Supervisor, error) { return runner, nil }
	m.refreshHardware(t.Context(), "serial")
	r, err := managedRecord(t.Context(), m, "serial", "serial-1")
	if err != nil || r.Presence != "absent" || r.Readiness != "failed" {
		t.Fatalf("disconnected inventory = %+v, %v", r, err)
	}
	runner.raw = json.RawMessage(`{"devices":[{"id":"ttyUSB0","kind":"usb-serial","display_name":"USB adapter","hardware_identity":{"vendor":"0403","product":"6001","serial":"abc/if00/port0","port_path":"1-2:if00:port0"},"capabilities":{"interface":"00"}}]}`)
	m.refreshHardware(t.Context(), "serial")
	r, err = managedRecord(t.Context(), m, "serial", "serial-1")
	if err != nil || r.Presence != "present" || r.Readiness != "ready" {
		t.Fatalf("reconnect = %+v %v", r, err)
	}
	revision := r.Revision
	m.refreshHardware(t.Context(), "serial")
	r, err = managedRecord(t.Context(), m, "serial", "serial-1")
	if err != nil || r.Revision != revision {
		t.Fatalf("poll invalidated active configuration: %+v %v", r, err)
	}
	runner.err = ErrUnavailable
	m.refreshHardware(t.Context(), "serial")
	r, err = managedRecord(t.Context(), m, "serial", "serial-1")
	if err != nil || r.Presence != "unknown" {
		t.Fatalf("failed refresh advertised readiness: %+v %v", r, err)
	}
}

func TestHardwareFailureAndNormalDisconnectDiffer(t *testing.T) {
	m, _, session := serialFixture(t)
	m.mu.Lock()
	m.stopSerialLocked(session.device, "disconnected")
	m.mu.Unlock()
	r, err := managedRecord(t.Context(), m, "serial", session.device)
	if err != nil || r.Readiness != "ready" {
		t.Fatalf("normal disconnect changed readiness: %+v %v", r, err)
	}
	m.mu.Lock()
	m.markHardwareFailed(t.Context(), "serial", session.device)
	r, err = managedRecord(t.Context(), m, "serial", session.device)
	m.mu.Unlock()
	if err != nil || r.Readiness != "failed" {
		t.Fatalf("device failure kept readiness: %+v %v", r, err)
	}
}
