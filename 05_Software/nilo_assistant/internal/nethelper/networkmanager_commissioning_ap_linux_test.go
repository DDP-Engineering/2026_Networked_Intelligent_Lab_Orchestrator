//go:build linux

package nethelper

import (
	"testing"

	"nilo.local/nilo/internal/network"
)

func TestCommissioningAccessPointProfileCannotAutoconnectBeforeEligibility(t *testing.T) {
	record := CommissioningAccessPointProfileRecord{
		AdapterID: "02:00:00:00:00:03", Interface: "wlan1",
		UUID: "0199a2d0-1234-7abc-8def-0123456789ab", SSID: "nilo-p2rjmzdkf1",
	}
	plan := network.CommissioningAccessPoint{
		Enabled: true, AdapterID: record.AdapterID, Interface: record.Interface,
		SerialName: record.SSID, Address: "192.168.247.1/24", Port: 8443,
	}
	settings, err := commissioningAPConnectionSettings(record.UUID, plan)
	if err != nil {
		t.Fatal(err)
	}
	if value[bool](settings["connection"], "autoconnect") {
		t.Fatal("new commissioning AP profile could activate before helper preflight")
	}
	if !commissioningAPProfileOwned(settings, record) {
		t.Fatal("safe non-autoconnect AP profile was not recognized as exact helper ownership")
	}
}
