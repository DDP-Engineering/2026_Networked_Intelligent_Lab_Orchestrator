package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"nilo.local/nilo/internal/modbus"
)

type modbusUSBReconciler interface {
	ReconcileUSB(context.Context, []ManagedDeviceObservation, time.Time) ([]modbus.Endpoint, error)
}

func TestModbusUSBReconciliationRetainsIdentityConfigurationAndAbsence(t *testing.T) {
	r := NewModbusRepository(openTemporaryDatabase(t))
	reconciler, ok := any(r).(modbusUSBReconciler)
	if !ok {
		t.Fatal("Modbus USB identity reconciliation unavailable without Serial plugin")
	}
	observation := ManagedDeviceObservation{Kind: "usb-serial", Vendor: "0403", Product: "6001", Serial: "abc/if00/port0", PortPath: "1-2:if00:port0", EndpointID: "ttyUSB0", Endpoints: json.RawMessage(`["ttyUSB0"]`), Capabilities: json.RawMessage(`{}`)}
	read := func(observations []ManagedDeviceObservation) []modbus.Endpoint {
		t.Helper()
		endpoints, err := reconciler.ReconcileUSB(t.Context(), observations, time.Now())
		if err != nil || len(endpoints) != 1 {
			t.Fatalf("reconcile: %+v %v", endpoints, err)
		}
		return endpoints
	}
	e := read([]ManagedDeviceObservation{observation})[0]
	if e.Transport != "rtu" || e.Network != nil || e.Presence != "present" || e.Enabled || e.SerialIdentity.SerialNumber == nil {
		t.Fatal(e)
	}
	e.Name = "Configured adapter"
	e.Enabled = true
	e.Readiness = "ready"
	e, err := r.PutEndpoint(t.Context(), e.Revision, e)
	if err != nil {
		t.Fatal(err)
	}
	repeated := read([]ManagedDeviceObservation{observation})[0]
	if repeated.Revision != e.Revision || repeated.Name != e.Name {
		t.Fatal("unchanged discovery revised configuration", repeated)
	}
	observation.EndpointID = "ttyUSB4"
	observation.Endpoints = json.RawMessage(`["ttyUSB4"]`)
	alias := read([]ManagedDeviceObservation{observation})[0]
	if alias.ID != e.ID || alias.ResourceID != e.ResourceID || alias.Revision <= e.Revision {
		t.Fatal("tty alias changed identity or missed revision", alias)
	}
	absent := read(nil)[0]
	if absent.ID != e.ID || absent.Presence != "absent" || absent.Readiness != "offline" || !absent.Enabled || absent.Name != e.Name {
		t.Fatal("unplug removed configuration", absent)
	}
	observation.PortPath = "1-4:if00:port0"
	replug := read([]ManagedDeviceObservation{observation})[0]
	if replug.ID != e.ID || replug.ResourceID != e.ResourceID || replug.SerialIdentity.BusPath != observation.PortPath || replug.Readiness != "ready" {
		t.Fatal("strong identity move", replug)
	}
}

func TestModbusWeakDuplicateAdaptersStayPortBoundAndProjectionIsAtomic(t *testing.T) {
	r := NewModbusRepository(openTemporaryDatabase(t))
	reconciler, ok := any(r).(modbusUSBReconciler)
	if !ok {
		t.Fatal("Modbus USB identity reconciliation unavailable")
	}
	first := ManagedDeviceObservation{Kind: "usb-serial", Vendor: "0403", Product: "6001", Serial: "duplicate", PortPath: "1-2:if00:port0", EndpointID: "ttyUSB0", Endpoints: json.RawMessage(`["ttyUSB0"]`), Capabilities: json.RawMessage(`{}`)}
	second := first
	second.PortPath = "1-3:if00:port0"
	second.EndpointID = "ttyUSB1"
	second.Endpoints = json.RawMessage(`["ttyUSB1"]`)
	endpoints, err := reconciler.ReconcileUSB(t.Context(), []ManagedDeviceObservation{first, second}, time.Now())
	if err != nil || len(endpoints) != 2 {
		t.Fatal(endpoints, err)
	}
	if endpoints[0].SerialIdentity.SerialNumber != nil || endpoints[1].SerialIdentity.SerialNumber != nil || endpoints[0].ResourceID == endpoints[1].ResourceID {
		t.Fatal("duplicate serials merged", endpoints)
	}
	// A duplicate port is rejected by the managed reconciliation transaction.
	second.PortPath = first.PortPath
	if _, err = reconciler.ReconcileUSB(t.Context(), []ManagedDeviceObservation{first, second}, time.Now()); err == nil {
		t.Fatal("duplicate physical observation accepted")
	}
	retained, err := r.Endpoints(t.Context())
	if err != nil || len(retained) != 2 || retained[0].Revision != endpoints[0].Revision || retained[1].Revision != endpoints[1].Revision {
		t.Fatal("partial projection", retained, err)
	}
}
