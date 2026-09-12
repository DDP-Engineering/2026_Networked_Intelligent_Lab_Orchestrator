package network

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

type failingAccessStore struct {
	delegate *accessStore
	mu       sync.Mutex
	failNext bool
}

type blockingAccessStore struct {
	delegate *accessStore
	mu       sync.Mutex
	block    bool
	started  chan struct{}
	release  chan struct{}
}

type cancellingAccessStore struct {
	delegate *accessStore
	mu       sync.Mutex
	blocked  bool
	started  chan struct{}
}

func (store *cancellingAccessStore) LoadManagementAccess(ctx context.Context) (ManagementAccessRecord, bool, error) {
	return store.delegate.LoadManagementAccess(ctx)
}

func (store *cancellingAccessStore) SaveManagementAccess(ctx context.Context, record ManagementAccessRecord) error {
	store.mu.Lock()
	block := !store.blocked
	store.blocked = true
	store.mu.Unlock()
	if block {
		close(store.started)
		<-ctx.Done()
		return ctx.Err()
	}
	return store.delegate.SaveManagementAccess(ctx, record)
}

func (store *blockingAccessStore) LoadManagementAccess(ctx context.Context) (ManagementAccessRecord, bool, error) {
	return store.delegate.LoadManagementAccess(ctx)
}

func (store *blockingAccessStore) SaveManagementAccess(ctx context.Context, record ManagementAccessRecord) error {
	store.mu.Lock()
	block := store.block
	if block {
		store.block = false
	}
	store.mu.Unlock()
	if block {
		close(store.started)
		<-store.release
	}
	return store.delegate.SaveManagementAccess(ctx, record)
}

func (store *failingAccessStore) LoadManagementAccess(ctx context.Context) (ManagementAccessRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.delegate.LoadManagementAccess(ctx)
}

func (store *failingAccessStore) SaveManagementAccess(ctx context.Context, record ManagementAccessRecord) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.failNext {
		store.failNext = false
		return errors.New("simulated management persistence failure")
	}
	return store.delegate.SaveManagementAccess(ctx, record)
}

func (store *failingAccessStore) failOneSave() {
	store.mu.Lock()
	store.failNext = true
	store.mu.Unlock()
}

type recoveryBlockingFirewall struct {
	mu             sync.Mutex
	calls          int
	restoreStarted chan struct{}
	restoreRelease chan struct{}
	failCall       int
}

func (firewall *recoveryBlockingFirewall) ApplyManagementAccess(_ context.Context, _ ManagementAccessConfiguration) error {
	firewall.mu.Lock()
	firewall.calls++
	call := firewall.calls
	firewall.mu.Unlock()
	if call == 3 {
		if firewall.restoreStarted != nil {
			close(firewall.restoreStarted)
			<-firewall.restoreRelease
		}
	}
	if call == firewall.failCall {
		return errors.New("simulated management firewall failure")
	}
	return nil
}

func (firewall *recoveryBlockingFirewall) callCount() int {
	firewall.mu.Lock()
	defer firewall.mu.Unlock()
	return firewall.calls
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
		{Name: "wlan0", AdapterID: "wifi-adapter", Kind: KindWiFi, State: StateConnected, IPv4: []string{"192.168.1.103/24"}, SSID: "DDP"},
		{Name: "nilo0", Kind: KindWireGuard, State: StateConnected, IPv4: []string{"10.205.148.11/32"}},
		{Name: "eth0", AdapterID: "ethernet-adapter", Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.0.1/24"}},
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

func TestManagementAccessStartupDoesNotMutateFirewallWhenInitialPersistenceFails(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	store := &failingAccessStore{delegate: &accessStore{}, failNext: true}
	firewall := &recoveryBlockingFirewall{}
	configuration := accessConfiguration()
	_, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: &accessProvider{interfaces: accessInterfaces()}, Store: store,
		Firewall: firewall, Now: func() time.Time { return now }, Initial: &configuration,
	})
	if err == nil {
		t.Fatal("startup accepted a management record that could not be persisted")
	}
	if calls := firewall.callCount(); calls != 0 {
		t.Fatalf("startup mutated the firewall %d time(s) before persistence succeeded", calls)
	}
}

func TestManagementAccessSaveFailureKeepsRecoveryInsideRoleCoordinator(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	coordinator := NewAdapterRoleCoordinator()
	store := &failingAccessStore{delegate: &accessStore{}}
	firewall := &recoveryBlockingFirewall{
		restoreStarted: make(chan struct{}), restoreRelease: make(chan struct{}),
	}
	simulation := NewSimulation(func() time.Time { return now })
	simulation.interfaces = append(simulation.interfaces, Interface{
		ID: "sim-enp3s0", Name: "enp3s0", AdapterID: "02:00:00:00:00:03",
		Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.30.1/24"},
	})
	configuration := ManagementAccessConfiguration{
		Interfaces: []string{"enp1s0"}, Port: 8443, TLSNames: []string{"nilo.local"},
	}
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: &accessProvider{interfaces: cloneInterfaces(simulation.interfaces)}, Store: store,
		Firewall: firewall, Now: func() time.Time { return now }, Initial: &configuration,
		Coordinator: coordinator,
	})
	if err != nil {
		t.Fatal(err)
	}
	deviceStore := newMemoryDeviceNetworkStore()
	forwarder := newMemoryForwarder()
	manager, err := NewDeviceNetworkManager(DeviceNetworkManagerOptions{
		Forwarder: forwarder, Environment: simulationEnvironment{s: simulation},
		Coordinator: coordinator, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.UseStore(context.Background(), deviceStore); err != nil {
		t.Fatal(err)
	}
	store.failOneSave()
	updateResult := make(chan error, 1)
	go func() {
		_, updateErr := service.Update(
			context.Background(), []string{"enp1s0", "enp3s0"}, false,
			netip.MustParseAddr("192.168.10.24"),
		)
		updateResult <- updateErr
	}()
	<-firewall.restoreStarted

	competitorResult := make(chan error, 1)
	deviceNetwork := proposedNetwork()
	deviceNetwork.DeviceInterface = "02:00:00:00:00:03"
	deviceNetwork.ApplianceAddress = netip.MustParsePrefix("192.168.30.1/24")
	deviceNetwork.Target = netip.MustParseAddr("192.168.30.130")
	go func() {
		competitorResult <- manager.Apply(context.Background(), Change{
			Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: deviceNetwork,
		})
	}()
	earlyMutation := false
	select {
	case <-competitorResult:
		earlyMutation = true
	case <-time.After(100 * time.Millisecond):
	}
	if len(deviceStore.records) != 0 || len(forwarder.snapshot()) != 0 {
		t.Fatalf("device operation mutated during recovery: records=%+v forwarding=%+v", deviceStore.records, forwarder.snapshot())
	}
	close(firewall.restoreRelease)
	if err := <-updateResult; err == nil {
		t.Fatal("update unexpectedly succeeded after persistence failure")
	}
	if !earlyMutation {
		if err := <-competitorResult; err != nil {
			t.Fatalf("device role was not available after successful recovery: %v", err)
		}
	}
	if earlyMutation {
		t.Fatal("device mutation overlapped management firewall recovery")
	}
	if len(deviceStore.records) != 1 {
		t.Fatalf("device operation did not persist after recovery: %+v", deviceStore.records)
	}
}

func TestManagementAccessPublishesClaimsBeforeAWaitingDeviceMutation(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	coordinator := NewAdapterRoleCoordinator()
	store := &blockingAccessStore{
		delegate: &accessStore{}, started: make(chan struct{}), release: make(chan struct{}),
	}
	simulation := NewSimulation(func() time.Time { return now })
	simulation.interfaces = append(simulation.interfaces, Interface{
		ID: "sim-enp3s0", Name: "enp3s0", AdapterID: "02:00:00:00:00:03",
		Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.30.1/24"},
	})
	configuration := ManagementAccessConfiguration{
		Interfaces: []string{"enp1s0"}, Port: 8443, TLSNames: []string{"nilo.local"},
	}
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: &accessProvider{interfaces: cloneInterfaces(simulation.interfaces)}, Store: store,
		Firewall: &accessFirewall{}, Now: func() time.Time { return now }, Initial: &configuration,
		Coordinator: coordinator,
	})
	if err != nil {
		t.Fatal(err)
	}
	deviceStore := newMemoryDeviceNetworkStore()
	forwarder := newMemoryForwarder()
	manager, err := NewDeviceNetworkManager(DeviceNetworkManagerOptions{
		Forwarder: forwarder, Environment: simulationEnvironment{s: simulation},
		Coordinator: coordinator, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.UseStore(context.Background(), deviceStore); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.block = true
	store.mu.Unlock()
	managementUnlocked := make(chan struct{})
	releaseManagementReturn := make(chan struct{})
	var afterUnlockFirst atomic.Bool
	coordinator.afterCheckAndRun = func() {
		if afterUnlockFirst.CompareAndSwap(false, true) {
			close(managementUnlocked)
			<-releaseManagementReturn
		}
	}
	managementResult := make(chan error, 1)
	go func() {
		_, updateErr := service.Update(
			context.Background(), []string{"enp1s0", "enp3s0"}, false,
			netip.MustParseAddr("192.168.10.24"),
		)
		managementResult <- updateErr
	}()
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("management update did not reach its durable write")
	}

	deviceNetwork := proposedNetwork()
	deviceNetwork.DeviceInterface = "02:00:00:00:00:03"
	deviceNetwork.ApplianceAddress = netip.MustParsePrefix("192.168.30.1/24")
	deviceNetwork.Target = netip.MustParseAddr("192.168.30.130")
	deviceResult := make(chan error, 1)
	go func() {
		deviceResult <- manager.Apply(context.Background(), Change{
			Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: deviceNetwork,
		})
	}()
	close(store.release)
	select {
	case <-managementUnlocked:
	case <-time.After(time.Second):
		t.Fatal("management update did not unlock the coordinator")
	}
	var conflict *AdapterRoleConflictError
	select {
	case err := <-deviceResult:
		if !errors.As(err, &conflict) {
			t.Fatalf("waiting device mutation was not refused after publication: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting device mutation did not finish while management was paused after unlock")
	}
	close(releaseManagementReturn)
	if err := <-managementResult; err != nil {
		t.Fatalf("management update lost the serialized race: %v", err)
	}
	coordinator.afterCheckAndRun = nil
	if len(deviceStore.records) != 0 || len(forwarder.snapshot()) != 0 {
		t.Fatalf("losing device operation mutated state: records=%+v forwarding=%+v", deviceStore.records, forwarder.snapshot())
	}
}

func TestCancelledManagementPublicationReleasesRoleForWaitingDeviceMutation(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	coordinator := NewAdapterRoleCoordinator()
	baseStore := &accessStore{}
	simulation := NewSimulation(func() time.Time { return now })
	simulation.interfaces = append(simulation.interfaces, Interface{
		ID: "sim-enp3s0", Name: "enp3s0", AdapterID: "02:00:00:00:00:03",
		Kind: KindEthernet, State: StateConnected, IPv4: []string{"192.168.30.1/24"},
	})
	configuration := ManagementAccessConfiguration{
		Interfaces: []string{"enp1s0"}, Port: 8443, TLSNames: []string{"nilo.local"},
	}
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: &accessProvider{interfaces: cloneInterfaces(simulation.interfaces)}, Store: baseStore,
		Firewall: &accessFirewall{}, Now: func() time.Time { return now }, Initial: &configuration,
		Coordinator: coordinator,
	})
	if err != nil {
		t.Fatal(err)
	}
	cancellingStore := &cancellingAccessStore{delegate: baseStore, started: make(chan struct{})}
	service.store = cancellingStore

	deviceStore := newMemoryDeviceNetworkStore()
	manager, err := NewDeviceNetworkManager(DeviceNetworkManagerOptions{
		Forwarder: newMemoryForwarder(), Environment: simulationEnvironment{s: simulation},
		Coordinator: coordinator, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.UseStore(context.Background(), deviceStore); err != nil {
		t.Fatal(err)
	}

	updateCtx, cancelUpdate := context.WithCancel(context.Background())
	managementUnlocked := make(chan struct{})
	releaseManagementReturn := make(chan struct{})
	var afterUnlockFirst atomic.Bool
	coordinator.afterCheckAndRun = func() {
		if afterUnlockFirst.CompareAndSwap(false, true) {
			close(managementUnlocked)
			<-releaseManagementReturn
		}
	}
	managementResult := make(chan error, 1)
	go func() {
		_, updateErr := service.Update(
			updateCtx, []string{"enp1s0", "enp3s0"}, false,
			netip.MustParseAddr("192.168.10.24"),
		)
		managementResult <- updateErr
	}()
	select {
	case <-cancellingStore.started:
	case <-time.After(time.Second):
		t.Fatal("management update did not reach its durable write")
	}

	deviceNetwork := proposedNetwork()
	deviceNetwork.DeviceInterface = "02:00:00:00:00:03"
	deviceNetwork.ApplianceAddress = netip.MustParsePrefix("192.168.30.1/24")
	deviceNetwork.Target = netip.MustParseAddr("192.168.30.130")
	deviceResult := make(chan error, 1)
	go func() {
		deviceResult <- manager.Apply(context.Background(), Change{
			Action: ActionDeviceNetworkCreate, Target: "new", DeviceNetwork: deviceNetwork,
		})
	}()
	cancelUpdate()
	select {
	case <-managementUnlocked:
	case <-time.After(time.Second):
		t.Fatal("cancelled management update did not unlock the coordinator")
	}
	select {
	case err := <-deviceResult:
		if err != nil {
			t.Fatalf("waiting device mutation did not proceed after recovery: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting device mutation did not finish while cancelled management was paused after unlock")
	}
	close(releaseManagementReturn)
	if err := <-managementResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled management update = %v, want context cancellation", err)
	}
	coordinator.afterCheckAndRun = nil
	if len(deviceStore.records) != 1 {
		t.Fatalf("device mutation count = %d, want one post-recovery mutation", len(deviceStore.records))
	}
}

func TestManagementAccessFailedRecoveryRetainsTheCandidateRoleClaim(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	coordinator := NewAdapterRoleCoordinator()
	store := &failingAccessStore{delegate: &accessStore{}}
	firewall := &recoveryBlockingFirewall{failCall: 3}
	configuration := accessConfiguration()
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: &accessProvider{interfaces: accessInterfaces()}, Store: store,
		Firewall: firewall, Now: func() time.Time { return now }, Initial: &configuration,
		Coordinator: coordinator,
	})
	if err != nil {
		t.Fatal(err)
	}
	store.failOneSave()
	status, err := service.Update(
		context.Background(), []string{"wlan0", "nilo0", "eth0"}, false,
		netip.MustParseAddr("192.168.1.103"),
	)
	if err == nil || status.State != ManagementAccessFailed {
		t.Fatalf("failed recovery status=%+v error=%v", status, err)
	}
	if err := coordinator.Check([]AdapterRoleClaim{{
		AdapterID: "ethernet-adapter", Interface: "eth0", Role: AdapterRoleDevice,
	}}); err == nil {
		t.Fatal("uncertain candidate firewall did not retain the management role claim")
	}
	var failedCandidate *ManagementAccessInterface
	for index := range status.Interfaces {
		if status.Interfaces[index].AdapterID == "ethernet-adapter" {
			failedCandidate = &status.Interfaces[index]
			break
		}
	}
	if failedCandidate == nil || failedCandidate.Role != AdapterRoleManagement ||
		!strings.Contains(failedCandidate.RoleDetail, "recovery") {
		t.Fatalf("failed recovery candidate is not represented in status: %+v", failedCandidate)
	}
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

func TestManagementAccessFollowsAStableAdapterAcrossRename(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	service := newAccessService(t, &now, provider, store, &accessFirewall{}, true)
	if len(store.record.Configuration.Bindings) != 2 || store.record.Configuration.Bindings[1].AdapterID != "wifi-adapter" {
		t.Fatalf("initial bindings=%+v", store.record.Configuration.Bindings)
	}

	renamed := accessInterfaces()
	renamed[0].Name = "wlp1s0"
	renamed[0].IPv4 = []string{"192.168.1.207/24"}
	provider.set(renamed)
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.Allows(netip.MustParseAddr("192.168.1.103")) || !service.Allows(netip.MustParseAddr("192.168.1.207")) {
		t.Fatal("stable management binding did not follow the renamed adapter")
	}
	status, err := service.Status(context.Background(), netip.MustParseAddr("192.168.1.207"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range status.Interfaces {
		if candidate.Name == "wlp1s0" {
			found = candidate.Enabled && candidate.AdapterID == "wifi-adapter"
		}
	}
	if !found {
		t.Fatalf("renamed status=%+v", status.Interfaces)
	}
}

func TestManagementAccessRejectsADeviceClaimBeforeFirewallOrStoreMutation(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	firewall := &accessFirewall{}
	coordinator := NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{
			AdapterID: "ethernet-adapter", Interface: "eth0", Role: AdapterRoleDevice,
			ResourceID: "device-network-1", ResourceName: "PLC network",
		}}
	})
	configuration := accessConfiguration()
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Coordinator: coordinator,
		Now: func() time.Time { return now }, Initial: &configuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Status(context.Background(), netip.MustParseAddr("192.168.1.103"))
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range status.Interfaces {
		if candidate.Name == "eth0" && (candidate.Role != AdapterRoleDevice || candidate.RoleAvailable || candidate.ConflictingResourceName != "PLC network") {
			t.Fatalf("device-claimed status=%+v", candidate)
		}
		if candidate.Name == "wlan0" && (candidate.Role != AdapterRoleManagement || !candidate.RoleAvailable) {
			t.Fatalf("management status=%+v", candidate)
		}
	}
	before := store.record
	beforeFirewallCalls := len(firewall.applied)
	_, err = service.Update(context.Background(), []string{"eth0", "nilo0"}, true, netip.MustParseAddr("192.168.1.103"))
	var conflict *AdapterRoleConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("update error=%v, want adapter role conflict", err)
	}
	if conflict.ConflictingResourceID != "device-network-1" || conflict.RequestedRole != AdapterRoleManagement {
		t.Fatalf("conflict=%+v", conflict)
	}
	if store.record.UpdatedAt != before.UpdatedAt || len(firewall.applied) != beforeFirewallCalls {
		t.Fatalf("conflict mutated store or firewall: record=%+v calls=%d", store.record, len(firewall.applied))
	}
}

func TestManagementAccessStatusMarksPendingRollbackAdaptersAsManagementOwned(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	firewall := &accessFirewall{}
	service := newAccessService(t, &now, provider, store, firewall, true)

	status, err := service.Update(
		context.Background(),
		[]string{"eth0", "nilo0"},
		false,
		netip.MustParseAddr("10.205.148.11"),
	)
	if err != nil || status.State != ManagementAccessPending {
		t.Fatalf("pending update = %+v, %v", status, err)
	}
	for _, candidate := range status.Interfaces {
		if candidate.Name != "wlan0" {
			continue
		}
		if candidate.Enabled || candidate.Role != AdapterRoleManagement || !candidate.RoleAvailable {
			t.Fatalf("rollback adapter status=%+v", candidate)
		}
		if !strings.Contains(candidate.RoleDetail, "recovery") {
			t.Fatalf("rollback adapter detail=%q, want recovery explanation", candidate.RoleDetail)
		}
		return
	}
	t.Fatalf("rollback adapter missing from status: %+v", status.Interfaces)
}

func TestManagementAccessStartupRecoversAConflictThroughTheRetainedPath(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	configuration := ManagementAccessConfiguration{
		Interfaces: []string{"eth0", "nilo0"},
		Bindings: []ManagementInterfaceBinding{
			{AdapterID: "ethernet-adapter", LastKnownName: "eth0"},
			{LastKnownName: "nilo0"},
		},
		Port: 8443, TLSNames: []string{"ddp-remote.local"},
	}
	store := &accessStore{record: ManagementAccessRecord{
		Configuration: configuration, LastKnownGood: configuration,
		State: ManagementAccessApplied, UpdatedAt: now,
	}, present: true}
	firewall := &accessFirewall{}
	coordinator := NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{
			AdapterID: "ethernet-adapter", Interface: "eth0", Role: AdapterRoleDevice,
			ResourceID: "device-network-1", ResourceName: "PLC network",
		}}
	})
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: &accessProvider{interfaces: accessInterfaces()}, Store: store, Firewall: firewall,
		Coordinator: coordinator, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.record.State != ManagementAccessRecovered || !slices.Equal(store.record.Configuration.Interfaces, []string{"nilo0"}) {
		t.Fatalf("recovered record=%+v", store.record)
	}
	if !slices.Equal(store.record.LastKnownGood.Interfaces, []string{"nilo0"}) || !strings.Contains(store.record.Detail, "PLC network") {
		t.Fatalf("recovered rollback/detail=%+v %q", store.record.LastKnownGood, store.record.Detail)
	}
	if service.Allows(netip.MustParseAddr("192.168.0.1")) || !service.Allows(netip.MustParseAddr("10.205.148.11")) {
		t.Fatal("destination gate did not retain only the non-conflicting path")
	}
	if len(firewall.applied) != 1 || !slices.Equal(firewall.applied[0].Interfaces, []string{"nilo0"}) {
		t.Fatalf("firewall=%+v", firewall.applied)
	}
}

func TestManagementAccessStartupFailsClosedWhenEveryUsablePathConflicts(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	configuration := ManagementAccessConfiguration{
		Interfaces: []string{"eth0"},
		Bindings:   []ManagementInterfaceBinding{{AdapterID: "ethernet-adapter", LastKnownName: "eth0"}},
		Port:       8443, TLSNames: []string{"ddp-remote.local"},
	}
	store := &accessStore{record: ManagementAccessRecord{
		Configuration: configuration, LastKnownGood: configuration,
		State: ManagementAccessApplied, UpdatedAt: now,
	}, present: true}
	firewall := &accessFirewall{}
	coordinator := NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{AdapterID: "ethernet-adapter", Interface: "eth0", Role: AdapterRoleDevice}}
	})
	_, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: &accessProvider{interfaces: accessInterfaces()}, Store: store, Firewall: firewall,
		Coordinator: coordinator, Now: func() time.Time { return now },
	})
	var conflict *AdapterRoleConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("startup error=%v, want role conflict", err)
	}
	if len(firewall.applied) != 0 {
		t.Fatalf("all-conflict startup touched firewall: %+v", firewall.applied)
	}
}

func TestManagementAccessMaintenanceReconcilesRolesBeforeOpeningARenamedPath(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	provider := &accessProvider{interfaces: accessInterfaces()}
	store := &accessStore{}
	firewall := &accessFirewall{}
	coordinator := NewAdapterRoleCoordinator()
	var deviceClaims []AdapterRoleClaim
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim { return deviceClaims })
	configuration := accessConfiguration()
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Coordinator: coordinator,
		Now: func() time.Time { return now }, Initial: &configuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	renamed := accessInterfaces()
	renamed[0].Name = "wlp1s0"
	renamed[0].IPv4 = []string{"192.168.1.207/24"}
	provider.set(renamed)
	deviceClaims = []AdapterRoleClaim{{
		AdapterID: "wifi-adapter", Interface: "wlp1s0", Role: AdapterRoleDevice,
		ResourceID: "device-network-1", ResourceName: "PLC network",
	}}
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.record.State != ManagementAccessRecovered || !slices.Equal(store.record.Configuration.Interfaces, []string{"nilo0"}) {
		t.Fatalf("maintenance record=%+v", store.record)
	}
	if service.Allows(netip.MustParseAddr("192.168.1.207")) || !service.Allows(netip.MustParseAddr("10.205.148.11")) {
		t.Fatal("conflicting renamed address entered the live destination gate")
	}
	if len(firewall.applied) != 2 || !slices.Equal(firewall.applied[1].Interfaces, []string{"nilo0"}) {
		t.Fatalf("maintenance firewall=%+v", firewall.applied)
	}
}
