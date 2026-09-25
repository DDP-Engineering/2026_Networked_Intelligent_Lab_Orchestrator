package nethelper

import "testing"

func TestDHCPIdentityIsLimitedToAutomaticOwnedClientFamilies(t *testing.T) {
	properties, err := dhcpIdentityProperties("nilo-p2rjmzdkf1", "auto", "auto")
	if err != nil {
		t.Fatal(err)
	}
	if properties.IPv4Hostname != "nilo-p2rjmzdkf1" || properties.IPv4Vendor != "NILO" ||
		properties.IPv6Hostname != "nilo-p2rjmzdkf1" {
		t.Fatalf("automatic identity = %+v", properties)
	}
	manual, err := dhcpIdentityProperties("nilo-p2rjmzdkf1", "manual", "link-local")
	if err != nil || manual != (dhcpIdentitySettings{}) {
		t.Fatalf("excluded identity = %+v, %v", manual, err)
	}
	if _, err := dhcpIdentityProperties("bad\nname", "auto", "auto"); err == nil {
		t.Fatal("unsafe serial name accepted")
	}
}
