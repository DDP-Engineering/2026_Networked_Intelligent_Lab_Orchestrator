package network

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"
)

type accessProvider struct {
	mu         sync.Mutex
	interfaces []Interface
}

func (provider *accessProvider) Interfaces(context.Context) ([]Interface, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return cloneInterfaces(provider.interfaces), nil
}

func (*accessProvider) Source() Source { return "test" }

func (provider *accessProvider) set(interfaces []Interface) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.interfaces = cloneInterfaces(interfaces)
}

type accessStore struct {
	record  ManagementAccessRecord
	present bool
}

func (store *accessStore) LoadManagementAccess(context.Context) (ManagementAccessRecord, bool, error) {
	return store.record, store.present, nil
}

func (store *accessStore) SaveManagementAccess(_ context.Context, record ManagementAccessRecord) error {
	store.record, store.present = record, true
	return nil
}

type accessFirewall struct {
	applied []ManagementAccessConfiguration
	fail    error
}

func (firewall *accessFirewall) ApplyManagementAccess(_ context.Context, configuration ManagementAccessConfiguration) error {
	if firewall.fail != nil {
		return firewall.fail
	}
	firewall.applied = append(firewall.applied, canonicalManagementConfiguration(configuration))
	return nil
}

func accessInterfaces() []Interface {
	return []Interface{
		{Name: "wlan0", Kind: KindWiFi, State: StateConnected, IPv4: []string{"192.168.1.103/24"}, SSID: "DDP"},
		{Name: "nilo0", Kind: KindWireGuard, State: StateConnected, IPv4: []string{"10.205.148.11/32"}},
		{Name: "eth0", Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.0.1/24"}},
	}
}

func accessConfiguration() ManagementAccessConfiguration {
	return ManagementAccessConfiguration{
		Interfaces: []string{"wlan0", "nilo0"}, Port: 8443,
		TLSNames: []string{"ddp-remote.local", "10.205.148.11"},
	}
}

func newAccessService(t *testing.T, now *time.Time, provider *accessProvider, store *accessStore, firewall *accessFirewall, initial bool) *ManagementAccessService {
	t.Helper()
	var configuration *ManagementAccessConfiguration
	if initial {
		value := accessConfiguration()
		configuration = &value
	}
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Now: func() time.Time { return *now },
		Initial: configuration, ConfirmWindow: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewManagementAccessService: %v", err)
	}
	return service
}

func TestManagementAccessConfigurationRejectsUnsafeIdentifiers(t *testing.T) {
	valid := accessConfiguration()
	for _, test := range []struct {
		name   string
		mutate func(*ManagementAccessConfiguration)
	}{
		{name: "loopback interface", mutate: func(value *ManagementAccessConfiguration) { value.Interfaces = []string{"lo"} }},
		{name: "shell-like interface", mutate: func(value *ManagementAccessConfiguration) { value.Interfaces = []string{"eth0;drop"} }},
		{name: "empty TLS identity", mutate: func(value *ManagementAccessConfiguration) { value.TLSNames = nil }},
		{name: "unsafe TLS identity", mutate: func(value *ManagementAccessConfiguration) { value.TLSNames = []string{"ddp-remote.local/path"} }},
		{name: "duplicate TLS identity", mutate: func(value *ManagementAccessConfiguration) {
			value.TLSNames = []string{"DDP-REMOTE.LOCAL", "ddp-remote.local"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			configuration := valid
			configuration.Interfaces = append([]string(nil), valid.Interfaces...)
			configuration.TLSNames = append([]string(nil), valid.TLSNames...)
			test.mutate(&configuration)
			if err := configuration.Validate(); err == nil {
				t.Fatalf("unsafe configuration accepted: %+v", configuration)
			}
		})
	}
}

func TestManagementAccessTracksAllowedInterfaceAddressesAcrossDHCPAndReconnect(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	service := newAccessService(t, &now, provider, &accessStore{}, &accessFirewall{}, true)

	for _, address := range []string{"192.168.1.103", "10.205.148.11"} {
		if !service.Allows(netip.MustParseAddr(address)) {
			t.Fatalf("allowed address %s was rejected", address)
		}
	}
	if service.Allows(netip.MustParseAddr("192.168.0.1")) {
		t.Fatal("PLC-facing eth0 address was allowed")
	}

	changed := accessInterfaces()
	changed[0].IPv4 = []string{"192.168.1.207/24"}
	changed[1].State = StateDisconnected
	changed[1].IPv4 = nil
	provider.set(changed)
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.Allows(netip.MustParseAddr("192.168.1.103")) || !service.Allows(netip.MustParseAddr("192.168.1.207")) {
		t.Fatal("Wi-Fi DHCP renewal did not replace the accepted destination address")
	}
	if service.Allows(netip.MustParseAddr("10.205.148.11")) {
		t.Fatal("disconnected nilo0 remained accepted")
	}

	changed[1].State = StateConnected
	changed[1].IPv4 = []string{"10.205.148.11/32"}
	provider.set(changed)
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !service.Allows(netip.MustParseAddr("10.205.148.11")) || !service.Allows(netip.MustParseAddr("192.168.1.207")) {
		t.Fatal("returning VPN and continuing Wi-Fi paths were not both accepted")
	}
}

func TestManagementAccessRequiresCurrentPathConfirmationAndSurvivesRestart(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	firewall := &accessFirewall{}
	service := newAccessService(t, &now, provider, store, firewall, true)
	wifi := netip.MustParseAddr("192.168.1.103")
	vpn := netip.MustParseAddr("10.205.148.11")

	if _, err := service.Update(context.Background(), []string{"nilo0"}, false, wifi); !errors.Is(err, ErrCurrentAccessConfirmation) {
		t.Fatalf("removing the current interface: %v, want confirmation refusal", err)
	}
	status, err := service.Update(context.Background(), []string{"nilo0"}, true, wifi)
	if err != nil || status.State != ManagementAccessPending {
		t.Fatalf("confirmed update = %+v, %v", status, err)
	}
	if service.Allows(wifi) || !service.Allows(vpn) {
		t.Fatal("pending allowlist was not enforced")
	}
	if _, err := service.Confirm(context.Background(), wifi); err == nil {
		t.Fatal("a disabled path confirmed the new configuration")
	}
	status, err = service.Confirm(context.Background(), vpn)
	if err != nil || status.State != ManagementAccessApplied {
		t.Fatalf("VPN confirmation = %+v, %v", status, err)
	}

	restarted := newAccessService(t, &now, provider, store, firewall, false)
	if restarted.Allows(wifi) || !restarted.Allows(vpn) {
		t.Fatal("restart did not retain the confirmed management configuration")
	}
	if !slices.Equal(store.record.Configuration.Interfaces, []string{"nilo0"}) {
		t.Fatalf("stored interfaces = %v", store.record.Configuration.Interfaces)
	}
}

func TestManagementAccessRejectsEmptyOrUnusableAndRecoversUnconfirmedChange(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	firewall := &accessFirewall{}
	service := newAccessService(t, &now, provider, store, firewall, true)

	if _, err := service.Update(context.Background(), nil, true, netip.MustParseAddr("192.168.1.103")); !errors.Is(err, ErrManagementAccessRequired) {
		t.Fatalf("empty update: %v", err)
	}
	if _, err := service.Update(context.Background(), []string{"missing0"}, true, netip.MustParseAddr("192.168.1.103")); !errors.Is(err, ErrManagementAccessUnsafe) {
		t.Fatalf("unusable update: %v", err)
	}
	if _, err := service.Update(context.Background(), []string{"nilo0"}, true, netip.MustParseAddr("192.168.1.103")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.record.State != ManagementAccessRecovered || !service.Allows(netip.MustParseAddr("192.168.1.103")) {
		t.Fatalf("expired configuration was not recovered: %+v", store.record)
	}
}

func TestManagementAccessConsoleReconfigurationPreservesRecoveryPath(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	firewall := &accessFirewall{}
	_ = newAccessService(t, &now, provider, store, firewall, true)
	reconfigured := accessConfiguration()
	reconfigured.Interfaces = []string{"nilo0"}

	if _, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Now: func() time.Time { return now },
		Initial: &reconfigured,
	}); !errors.Is(err, ErrManagementReconfigure) {
		t.Fatalf("implicit console overwrite error=%v", err)
	}
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Now: func() time.Time { return now },
		Initial: &reconfigured, Reconfigure: true, ConfirmWindow: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.record.State != ManagementAccessPending || !slices.Equal(store.record.LastKnownGood.Interfaces, []string{"nilo0", "wlan0"}) {
		t.Fatalf("recoverable console record=%+v", store.record)
	}
	now = now.Add(2 * time.Minute)
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.record.State != ManagementAccessRecovered || !service.Allows(netip.MustParseAddr("192.168.1.103")) {
		t.Fatalf("console change did not recover: %+v", store.record)
	}
}

func TestManagementAccessInitializationRequiresAUsablePath(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	interfaces := accessInterfaces()
	for index := range interfaces {
		interfaces[index].State = StateDisconnected
		interfaces[index].IPv4 = nil
		interfaces[index].IPv6 = nil
	}
	configuration := accessConfiguration()
	_, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: &accessProvider{interfaces: interfaces}, Store: &accessStore{}, Firewall: &accessFirewall{},
		Now: func() time.Time { return now }, Initial: &configuration,
	})
	if !errors.Is(err, ErrManagementAccessUnsafe) {
		t.Fatalf("unusable initialization error=%v", err)
	}
}

func TestPersistedManagementAccessMayStartOfflineAndServeWhenAnInterfaceReturns(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	firewall := &accessFirewall{}
	_ = newAccessService(t, &now, provider, store, firewall, true)
	offline := accessInterfaces()
	for index := range offline {
		offline[index].State = StateDisconnected
		offline[index].IPv4 = nil
		offline[index].IPv6 = nil
	}
	provider.set(offline)
	service := newAccessService(t, &now, provider, store, firewall, false)
	if service.Allows(netip.MustParseAddr("10.205.148.11")) {
		t.Fatal("offline interface was accepted")
	}
	offline[1].State = StateConnected
	offline[1].IPv4 = []string{"10.205.148.11/32"}
	provider.set(offline)
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !service.Allows(netip.MustParseAddr("10.205.148.11")) {
		t.Fatal("returning persisted interface did not begin serving")
	}
}

func TestManagementAccessRecoversPendingConfigurationOnSimulatedReboot(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	firewall := &accessFirewall{}
	service := newAccessService(t, &now, provider, store, firewall, true)
	if _, err := service.Update(context.Background(), []string{"nilo0"}, true, netip.MustParseAddr("192.168.1.103")); err != nil {
		t.Fatal(err)
	}

	rebooted := newAccessService(t, &now, provider, store, firewall, false)
	if store.record.State != ManagementAccessRecovered || !rebooted.Allows(netip.MustParseAddr("192.168.1.103")) {
		t.Fatalf("reboot did not restore last known-good configuration: %+v", store.record)
	}
}
