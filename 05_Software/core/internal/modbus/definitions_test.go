package modbus

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestNetworkCanonicalizationAndUSBModelSeparation(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{" Gateway.Local. ", "gateway.local"}, {"2001:0db8:0:0::1", "2001:db8::1"}, {"::ffff:127.0.0.1", "127.0.0.1"}} {
		network := &Network{Host: tc.input, Port: 502, UnitMax: 255}
		e, err := (Endpoint{SchemaVersion: 1, ID: "peer", Revision: 1, Name: "Peer", Transport: "tcp", ResourceID: "gateway", Enabled: true, Presence: "configured", Readiness: "offline", Network: network}).Canonical()
		if err != nil || e.Network.Host != tc.want || network.Host != tc.input || e.SerialIdentity != nil {
			t.Fatalf("%s: %+v %v", tc.input, e, err)
		}
		raw, _ := json.Marshal(e)
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		if _, exists := fields["serial_identity"]; exists {
			t.Fatal("fabricated USB identity")
		}
	}
}

func TestRTUFramingAndStrictProfileSettings(t *testing.T) {
	e := Endpoint{SchemaVersion: 1, ID: "usb", Revision: 1, Name: "Adapter", Transport: "rtu", ResourceID: "usb-1", Enabled: true, Presence: "absent", Readiness: "offline", SerialIdentity: &USBIdentity{StableID: "usb-1", VendorID: "0403", ProductID: "6001", BusPath: "1-2:if00:port0"}}
	if _, err := e.Canonical(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		parity           string
		stop, data, baud int
		valid            bool
	}{
		{"even", 1, 8, 9600, true}, {"odd", 1, 8, 9600, true}, {"none", 2, 8, 9600, true}, {"none", 1, 8, 9600, true},
		{"even", 2, 8, 9600, false}, {"odd", 2, 8, 9600, false}, {"mark", 1, 8, 9600, false}, {"none", 1, 7, 9600, false}, {"none", 1, 8, 49, false}, {"none", 1, 8, 4000001, false},
	} {
		p := Profile{SchemaVersion: 1, ID: "profile", Revision: 1, OwnerID: "creator", Name: "Default", EndpointID: e.ID, Settings: json.RawMessage(fmt.Sprintf(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0,"baud_rate":%d,"data_bits":%d,"parity":%q,"stop_bits":%d,"local_echo":false}`, tc.baud, tc.data, tc.parity, tc.stop))}
		if _, err := p.Canonical(e); (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	for _, raw := range []string{
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0,"read_retries":0}`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0,"read_retries":0,"baud_rate":9600,"data_bits":8,"parity":"none","stop_bits":1}`,
		`{"response_timeout_ms":1,"connect_timeout_ms":1,"inter_request_ms":0,"read_retries":0} {}`,
	} {
		if _, err := (Profile{SchemaVersion: 1, ID: "profile", Revision: 1, OwnerID: "creator", Name: "Default", EndpointID: e.ID, Settings: json.RawMessage(raw)}).Canonical(e); err == nil {
			t.Fatal("incomplete framing accepted", raw)
		}
	}
}
