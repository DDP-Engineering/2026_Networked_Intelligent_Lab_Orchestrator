package nethelper

import (
	"net/netip"
	"testing"
)

func TestCommissioningSubnetOverlapRefusesOnlyConflictingForeignRoutes(t *testing.T) {
	subnet := netip.MustParsePrefix("192.168.247.0/24")
	if commissioningSubnetAvailable(subnet, "wlan1", []hostRoute{
		{Interface: "eth0", Prefix: netip.MustParsePrefix("10.0.0.0/8")},
		{Interface: "wlan1", Prefix: subnet},
	}) == false {
		t.Fatal("non-overlapping routes and the owned AP route were refused")
	}
	if commissioningSubnetAvailable(subnet, "wlan1", []hostRoute{{Interface: "eth0", Prefix: netip.MustParsePrefix("192.168.247.128/25")}}) {
		t.Fatal("overlapping foreign route was accepted")
	}
}
