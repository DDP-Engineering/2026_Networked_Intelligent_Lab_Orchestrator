package network

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func staticSettings(adapter string) InterfaceSettings {
	return InterfaceSettings{
		AdapterID: adapter, Method: MethodStatic,
		Address: netip.MustParsePrefix("192.168.0.1/24"),
		DNS:     []netip.Addr{netip.MustParseAddr("192.168.0.53")},
	}
}

// TestInterfaceSettingsValidateEveryField covers the input half of
// REQ-FUNC-5e2b91d7 and REQ-FUNC-7c40aa18: every refusal names what is wrong,
// and a valid static, automatic and Wi-Fi configuration is accepted.
func TestInterfaceSettingsValidateEveryField(t *testing.T) {
	t.Parallel()
	for _, valid := range []InterfaceSettings{
		staticSettings("00:11:22:33:44:55"),
		{AdapterID: "00:11:22:33:44:55", Method: MethodAutomatic, DNSAutomatic: true},
		{AdapterID: "00:11:22:33:44:55", Method: MethodAutomatic, DNS: []netip.Addr{netip.MustParseAddr("9.9.9.9")}},
		{AdapterID: "00:11:22:33:44:55", Method: MethodStatic, Address: netip.MustParsePrefix("10.0.0.2/8"), Gateway: netip.MustParseAddr("10.0.0.1"), NeverDefault: true},
		{AdapterID: "00:11:22:33:44:56", Method: MethodAutomatic, DNSAutomatic: true,
			WiFi: &WiFiSettings{SSID: "lab-hotspot", Security: WiFiWPA2Personal, SecretRef: "psk-1", ConnectAtStartup: true}},
		{AdapterID: "00:11:22:33:44:56", Method: MethodAutomatic, DNSAutomatic: true,
			WiFi: &WiFiSettings{SSID: "open-cafe", Security: WiFiOpen}},
	} {
		if err := valid.Validate(); err != nil {
			t.Fatalf("valid settings %+v refused: %v", valid, err)
		}
	}
	for _, refused := range []struct {
		name string
		edit func(*InterfaceSettings)
		want string
	}{
		{"no adapter", func(s *InterfaceSettings) { s.AdapterID = "" }, "adapter"},
		{"unknown method", func(s *InterfaceSettings) { s.Method = "bootp" }, "method"},
		{"static without address", func(s *InterfaceSettings) { s.Address = netip.Prefix{} }, "address"},
		{"ipv6 address", func(s *InterfaceSettings) { s.Address = netip.MustParsePrefix("fd00::1/64") }, "IPv4"},
		{"point to point mask", func(s *InterfaceSettings) { s.Address = netip.MustParsePrefix("192.168.0.1/31") }, "/30"},
		{"network address", func(s *InterfaceSettings) { s.Address = netip.MustParsePrefix("192.168.0.0/24") }, "network address"},
		{"broadcast address", func(s *InterfaceSettings) { s.Address = netip.MustParsePrefix("192.168.0.255/24") }, "broadcast"},
		{"gateway outside", func(s *InterfaceSettings) { s.Gateway = netip.MustParseAddr("10.9.9.1") }, "gateway"},
		{"gateway equal to address", func(s *InterfaceSettings) { s.Gateway = netip.MustParseAddr("192.168.0.1") }, "gateway"},
		{"automatic with address", func(s *InterfaceSettings) { s.Method = MethodAutomatic }, "automatic"},
		{"automatic dns with servers", func(s *InterfaceSettings) { s.DNSAutomatic = true }, "DNS"},
		{"too many dns", func(s *InterfaceSettings) {
			for range 8 {
				s.DNS = append(s.DNS, netip.MustParseAddr("9.9.9.9"))
			}
		}, "DNS"},
		{"wifi without ssid", func(s *InterfaceSettings) { s.WiFi = &WiFiSettings{Security: WiFiOpen} }, "SSID"},
		{"wifi ssid too long", func(s *InterfaceSettings) { s.WiFi = &WiFiSettings{SSID: strings.Repeat("x", 33), Security: WiFiOpen} }, "SSID"},
		{"wifi unknown security", func(s *InterfaceSettings) { s.WiFi = &WiFiSettings{SSID: "lab", Security: "wep"} }, "security"},
		{"wifi personal without secret", func(s *InterfaceSettings) { s.WiFi = &WiFiSettings{SSID: "lab", Security: WiFiWPA3Personal} }, "password"},
		{"wifi open with secret", func(s *InterfaceSettings) { s.WiFi = &WiFiSettings{SSID: "lab", Security: WiFiOpen, SecretRef: "psk"} }, "password"},
		{"wifi secret that looks like a value", func(s *InterfaceSettings) {
			s.WiFi = &WiFiSettings{SSID: "lab", Security: WiFiWPA2Personal, SecretRef: "hunter two"}
		}, "reference"},
	} {
		t.Run(refused.name, func(t *testing.T) {
			settings := staticSettings("00:11:22:33:44:55")
			refused.edit(&settings)
			err := settings.Validate()
			if !errors.Is(err, ErrInterfaceSettingsRejected) {
				t.Fatalf("%s: %v, want ErrInterfaceSettingsRejected", refused.name, err)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(refused.want)) {
				t.Fatalf("%s: %q does not name %q", refused.name, err.Error(), refused.want)
			}
		})
	}
}

// TestSavedSettingsFollowTheAdapterNotTheKernelName covers REQ-FUNC-3a71c204:
// a configuration binds to the permanent hardware identity, survives a kernel
// rename, and is reported as saved but not applied while the adapter is away.
func TestSavedSettingsFollowTheAdapterNotTheKernelName(t *testing.T) {
	t.Parallel()
	saved := []InterfaceSettings{staticSettings("00:11:22:33:44:55")}
	interfaces := []Interface{
		{ID: "nm-eth0-00:11:22:33:44:55", Name: "eth0", AdapterID: "00:11:22:33:44:55", Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.0.1/24"}, Controllable: true},
		{ID: "nm-wlan0-00:11:22:33:44:56", Name: "wlan0", AdapterID: "00:11:22:33:44:56", Kind: KindWiFi, State: StateDisconnected, Controllable: true},
	}
	bound := BindSettings(interfaces, saved)
	if bound.Interfaces[0].Settings == nil || !bound.Interfaces[0].Settings.Saved || !bound.Interfaces[0].Settings.Applied {
		t.Fatalf("eth0 binding = %+v, want saved and applied", bound.Interfaces[0].Settings)
	}
	if bound.Interfaces[1].Settings != nil {
		t.Fatalf("wlan0 has settings %+v, want none", bound.Interfaces[1].Settings)
	}
	if len(bound.Unattached) != 0 {
		t.Fatalf("unattached = %+v, want none while the adapter is present", bound.Unattached)
	}

	// The kernel renames the device; the adapter is the same.
	interfaces[0].ID, interfaces[0].Name = "nm-enp3s0-00:11:22:33:44:55", "enp3s0"
	bound = BindSettings(interfaces, saved)
	if bound.Interfaces[0].Settings == nil || !bound.Interfaces[0].Settings.Saved {
		t.Fatal("a kernel rename lost the binding")
	}

	// The address is not on the device: saved, present, not applied.
	interfaces[0].IPv4 = []string{"192.168.0.7/24"}
	bound = BindSettings(interfaces, saved)
	if bound.Interfaces[0].Settings == nil || !bound.Interfaces[0].Settings.Saved || bound.Interfaces[0].Settings.Applied {
		t.Fatalf("binding with a different address = %+v, want saved and not applied", bound.Interfaces[0].Settings)
	}

	// The adapter is unplugged: the configuration is neither applied nor gone.
	bound = BindSettings(interfaces[1:], saved)
	if len(bound.Unattached) != 1 || bound.Unattached[0].AdapterID != "00:11:22:33:44:55" {
		t.Fatalf("unattached = %+v, want the saved configuration for the missing adapter", bound.Unattached)
	}
	for _, candidate := range bound.Interfaces {
		if candidate.Settings != nil {
			t.Fatalf("%s carries settings it does not own", candidate.Name)
		}
	}
}

// TestSettingsRevisionChangesOnlyWhenTheContentDoes covers the revision half of
// REQ-DATA-7b62c9f3 for interface settings.
func TestSettingsRevisionChangesOnlyWhenTheContentDoes(t *testing.T) {
	t.Parallel()
	first := staticSettings("00:11:22:33:44:55")
	at := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	same := first
	if first.ContentRevision() != same.ContentRevision() {
		t.Fatal("identical settings produced different revisions")
	}
	same.UpdatedAt = at.Add(time.Hour)
	same.Revision = "stale"
	if first.ContentRevision() != same.ContentRevision() {
		t.Fatal("bookkeeping fields changed the content revision")
	}
	changed := first
	changed.Gateway = netip.MustParseAddr("192.168.0.254")
	if first.ContentRevision() == changed.ContentRevision() {
		t.Fatal("an edit did not change the revision")
	}
	protectedOnly := first
	protectedOnly.ManagementProtected = true
	if first.ContentRevision() == protectedOnly.ContentRevision() {
		t.Fatal("changing management protection did not change the revision")
	}
}
