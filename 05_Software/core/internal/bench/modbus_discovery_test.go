package bench

import (
	"encoding/json"
	"testing"

	"nilo.local/nilo/internal/store/sqlite"
)

const modbusUSBObservation = `{"devices":[{"id":"ttyUSB0","kind":"usb-serial","display_name":"USB adapter","hardware_identity":{"vendor":"0403","product":"6001","serial":"abc/if00/port0","port_path":"1-2:if00:port0"},"capabilities":{"interface":"00"}}]}`

func TestModbusTrustedDiscoveryAndDisabledAbsentDefinitions(t *testing.T) {
	m, admin, runner := testManager(t, "auto")
	endpoints, err := m.reconcileModbusDiscovery(t.Context(), json.RawMessage(modbusUSBObservation))
	if err != nil || len(endpoints) != 1 {
		t.Fatal(endpoints, err)
	}
	e := endpoints[0]
	e.Name = "Adapter"
	e.Enabled = true
	e, err = m.SaveModbusEndpoint(t.Context(), admin, e.Revision, e)
	if err != nil || e.Readiness != "ready" {
		t.Fatal(e, err)
	}
	p := profileDefinition("serial-profile", e.ID)
	p.Settings = json.RawMessage(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0,"baud_rate":9600,"data_bits":8,"parity":"even","stop_bits":1,"local_echo":false}`)
	if _, err = m.SaveModbusProfile(t.Context(), admin, 0, p); err != nil {
		t.Fatal(err)
	}
	e.Enabled = false
	e, err = m.SaveModbusEndpoint(t.Context(), admin, e.Revision, e)
	if err != nil || e.Readiness != "unconfigured" {
		t.Fatal(e, err)
	}
	endpoints, err = m.reconcileModbusDiscovery(t.Context(), json.RawMessage(`{"devices":[]}`))
	if err != nil || endpoints[0].Enabled || endpoints[0].Name != "Adapter" || endpoints[0].Presence != "absent" {
		t.Fatal(endpoints, err)
	}
	profiles, err := m.ModbusProfiles(t.Context(), admin)
	if err != nil || len(profiles) != 1 {
		t.Fatal(profiles, err)
	}
	for _, raw := range []string{`{"devices":[{"id":"/dev/ttyUSB0","kind":"usb-serial"}]}`, `{"devices":[],"command":"open"}`} {
		if _, err = m.reconcileModbusDiscovery(t.Context(), json.RawMessage(raw)); err == nil {
			t.Fatal("untrusted discovery admitted", raw)
		}
	}
	runner.mu.Lock()
	calls := runner.calls
	runner.mu.Unlock()
	if calls != 0 {
		t.Fatal("metadata invoked transport", calls)
	}
}

func TestModbusStartupRequiresFreshPresenceAndReadiness(t *testing.T) {
	m, admin, _ := testManager(t, "auto")
	endpoints, err := m.reconcileModbusDiscovery(t.Context(), json.RawMessage(modbusUSBObservation))
	if err != nil {
		t.Fatal(err)
	}
	e := endpoints[0]
	e.Enabled = true
	if _, err = m.SaveModbusEndpoint(t.Context(), admin, e.Revision, e); err != nil {
		t.Fatal(err)
	}
	network, err := m.SaveModbusEndpoint(t.Context(), admin, 0, networkDefinition("network", "bus"))
	if err != nil {
		t.Fatal(err)
	}
	network.Readiness = "ready"
	if _, err = sqlite.NewModbusRepository(m.options.Database).PutEndpoint(t.Context(), network.Revision, network); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(m.options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	endpoints, err = restarted.ModbusEndpoints(t.Context(), admin)
	if err != nil || len(endpoints) != 2 {
		t.Fatal(endpoints, err)
	}
	for _, endpoint := range endpoints {
		if endpoint.Readiness != "offline" || (endpoint.Transport == "rtu" && endpoint.Presence != "absent") || (endpoint.Transport == "tcp" && endpoint.Presence != "configured") {
			t.Fatal("stale startup availability", endpoint)
		}
	}
}
