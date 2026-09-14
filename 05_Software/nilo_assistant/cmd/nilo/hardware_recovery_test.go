package main

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/nethelper"
	"nilo.local/nilo/internal/network"
)

type runtimeRecoveryProvider struct{ interfaces []network.Interface }

func (provider *runtimeRecoveryProvider) Interfaces(context.Context) ([]network.Interface, error) {
	return append([]network.Interface(nil), provider.interfaces...), nil
}

func (*runtimeRecoveryProvider) Source() network.Source { return "test" }

type runtimeAccessStore struct {
	record  network.ManagementAccessRecord
	present bool
}

func (store *runtimeAccessStore) LoadManagementAccess(context.Context) (network.ManagementAccessRecord, bool, error) {
	return store.record, store.present, nil
}

func (store *runtimeAccessStore) SaveManagementAccess(_ context.Context, record network.ManagementAccessRecord) error {
	store.record, store.present = record, true
	return nil
}

type runtimeFirewall struct {
	applied []network.ManagementAccessConfiguration
}

func (firewall *runtimeFirewall) ApplyManagementAccess(_ context.Context, configuration network.ManagementAccessConfiguration) error {
	firewall.applied = append(firewall.applied, configuration)
	return nil
}

type runtimeRecoveryLinkDriver struct{}

func (*runtimeRecoveryLinkDriver) ActivateRecoveryLink(context.Context, string, string, time.Time) (string, error) {
	return "link", nil
}
func (*runtimeRecoveryLinkDriver) ExtendRecoveryLink(context.Context, string, time.Time) error {
	return nil
}
func (*runtimeRecoveryLinkDriver) RestoreRecoveryLink(context.Context, string) error { return nil }

type trackingRuntimeRecoveryLinkDriver struct{ restored []string }

func (*trackingRuntimeRecoveryLinkDriver) ActivateRecoveryLink(context.Context, string, string, time.Time) (string, error) {
	return "unused", nil
}
func (*trackingRuntimeRecoveryLinkDriver) ExtendRecoveryLink(context.Context, string, time.Time) error {
	return nil
}
func (driver *trackingRuntimeRecoveryLinkDriver) RestoreRecoveryLink(_ context.Context, recordID string) error {
	driver.restored = append(driver.restored, recordID)
	return nil
}

type runtimeRecoveryStore struct {
	record    network.RecoveryRecord
	failSaves int
}

func (store *runtimeRecoveryStore) LoadHardwareRecovery(context.Context) (network.RecoveryRecord, bool, error) {
	return store.record, true, nil
}
func (store *runtimeRecoveryStore) SaveHardwareRecovery(_ context.Context, record network.RecoveryRecord) error {
	if store.failSaves > 0 {
		store.failSaves--
		return errors.New("injected recovery persistence failure")
	}
	store.record = record
	return nil
}

type failingRuntimeRecoveryHelper struct {
	recordID string
	err      error
}

func (helper *failingRuntimeRecoveryHelper) ActivateRecoveryLink(context.Context, string, string, time.Time) (string, error) {
	return helper.recordID, helper.err
}
func (*failingRuntimeRecoveryHelper) ExtendRecoveryLink(context.Context, string, time.Time) error {
	return nil
}
func (*failingRuntimeRecoveryHelper) RestoreRecoveryLink(context.Context, string) error { return nil }

func TestRuntimeRecoveryDriverPreservesPartialHelperIdentity(t *testing.T) {
	driver := &runtimeRecoveryDriver{helper: &failingRuntimeRecoveryHelper{
		recordID: "partial-helper-record", err: errors.New("partial activation"),
	}}
	recordID, err := driver.ActivateRecoveryLink(context.Background(), "adapter", "eth1", time.Now().Add(time.Minute))
	if err == nil || recordID != "partial-helper-record" {
		t.Fatalf("recordID=%q err=%v, want partial helper identity and failure", recordID, err)
	}
}

func TestRuntimeStartupRestoresAnActiveRecoveryLinkInsteadOfReadoptingIt(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{interfaces: []network.Interface{{
		Name: "eth1", AdapterID: "adapter", Kind: network.KindEthernet, Physical: true, State: network.StateConnected,
	}}}
	configuration := network.ManagementAccessConfiguration{Interfaces: []string{"eth0"}, Port: 8443, TLSNames: []string{"bench.local"}}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{present: true, record: network.ManagementAccessRecord{
			Configuration: configuration, LastKnownGood: configuration, State: network.ManagementAccessApplied, UpdatedAt: now,
		}}, Firewall: &runtimeFirewall{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	linkDriver := &trackingRuntimeRecoveryLinkDriver{}
	store := &runtimeRecoveryStore{record: network.RecoveryRecord{
		Version: 1, State: network.RecoveryActive, ArmedAt: now.Add(-time.Minute), ExpiresAt: now.Add(9 * time.Minute),
		AdapterID: "adapter", Interface: "eth1", HelperRecordID: "helper-record", UpdatedAt: now.Add(-time.Minute),
	}}
	service, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: linkDriver, Store: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &runtimePublisher{}
	announcer, err := network.NewCommissionedMDNSAnnouncer(publisher, "bench", "0199a2d0-1234-7abc-8def-0123456789ab", 8443)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &hardwareRecoveryRuntime{
		helper: &runtimeHardware{}, service: service, management: management,
		driver: &runtimeRecoveryDriver{management: management, announcer: announcer, interfaces: func(context.Context) ([]string, error) { return []string{"eth0"}, nil }},
	}
	if err := runtime.Adopt(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(linkDriver.restored, []string{"helper-record"}) || store.record.State != network.RecoveryIdle {
		t.Fatalf("startup retained recovery: restored=%v record=%+v", linkDriver.restored, store.record)
	}
}

type runtimeHardware struct {
	event       nethelper.HardwareRecoveryEvent
	acked       string
	ackFailures int
	indicators  []nethelper.HardwareIndicatorState
}

func (hardware *runtimeHardware) ReadRecoveryEvent(context.Context) (nethelper.HardwareRecoveryEvent, bool, error) {
	return hardware.event, hardware.event.ID != "" && hardware.acked == "", nil
}
func (hardware *runtimeHardware) AcknowledgeRecoveryEvent(_ context.Context, id string) error {
	if hardware.ackFailures > 0 {
		hardware.ackFailures--
		return errors.New("ack unavailable")
	}
	hardware.acked = id
	return nil
}
func (hardware *runtimeHardware) SetRecoveryIndicator(_ context.Context, state nethelper.HardwareIndicatorState) error {
	hardware.indicators = append(hardware.indicators, state)
	return nil
}

type runtimeResetter struct {
	called bool
	access network.ManagementAccessRecord
}

func (resetter *runtimeResetter) ResetNetworkCommissioningAudited(
	_ context.Context, access network.ManagementAccessRecord, _ audit.Event, _ uint64,
) error {
	resetter.called, resetter.access = true, access
	return nil
}

type runtimeAudit struct {
	records    []audit.Record
	failAction string
}

func (*runtimeAudit) Compose(audit.Record) (audit.Event, error) { return audit.Event{}, nil }
func (writer *runtimeAudit) Record(_ context.Context, record audit.Record) error {
	if writer.failAction == record.Action {
		return errors.New("injected audit failure")
	}
	writer.records = append(writer.records, record)
	return nil
}

type runtimePublisher struct {
	advertisements []network.MDNSAdvertisement
	withdrawals    int
	publishErr     error
}

func (publisher *runtimePublisher) PublishMDNS(_ context.Context, advertisement network.MDNSAdvertisement) error {
	publisher.advertisements = append(publisher.advertisements, advertisement)
	return publisher.publishErr
}
func (publisher *runtimePublisher) WithdrawMDNS(context.Context) error {
	publisher.withdrawals++
	return nil
}

type runtimeCommissioner struct{}

func (runtimeCommissioner) CompleteCommissioning(context.Context) error { return nil }

func TestLiveCommissioningReplacesNiloAliasWithPermanentService(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{interfaces: []network.Interface{{
		Name: "eth0", AdapterID: "management", Kind: network.KindEthernet, Physical: true,
		State: network.StateConnected, IPv4: []string{"192.0.2.10/24"},
	}}}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"bench.local", "nilo.local"}, Commissioner: runtimeCommissioner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &runtimePublisher{}
	provisioning := network.NewProvisioningMDNSAnnouncer(publisher)
	commissioned, err := network.NewCommissionedMDNSAnnouncer(publisher, "bench", "0199a2d0-1234-7abc-8def-0123456789ab", 8443)
	if err != nil {
		t.Fatal(err)
	}
	var current *network.MDNSAnnouncer
	if err := reconcileApplianceDiscovery(ctx, management, &current, provisioning, commissioned); err != nil {
		t.Fatal(err)
	}
	if _, err := management.Commission(ctx, []string{"eth0"}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if err := reconcileApplianceDiscovery(ctx, management, &current, provisioning, commissioned); err != nil {
		t.Fatal(err)
	}
	if len(publisher.advertisements) != 2 || publisher.advertisements[0].Service != nil || publisher.advertisements[1].Service == nil || publisher.withdrawals != 1 {
		t.Fatalf("lifecycle advertisements = %+v", publisher.advertisements)
	}
}

func TestLiveCommissioningWithdrawsSharedAliasBeforeAFailedPermanentReplacement(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{interfaces: []network.Interface{{
		Name: "eth0", AdapterID: "management", Kind: network.KindEthernet, Physical: true,
		State: network.StateConnected, IPv4: []string{"192.0.2.10/24"},
	}}}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"bench.local", "nilo.local"}, Commissioner: runtimeCommissioner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	publisher := &runtimePublisher{}
	provisioning := network.NewProvisioningMDNSAnnouncer(publisher)
	commissioned, err := network.NewCommissionedMDNSAnnouncer(publisher, "bench", "0199a2d0-1234-7abc-8def-0123456789ab", 8443)
	if err != nil {
		t.Fatal(err)
	}
	var current *network.MDNSAnnouncer
	if err := reconcileApplianceDiscovery(ctx, management, &current, provisioning, commissioned); err != nil {
		t.Fatal(err)
	}
	if _, err := management.Commission(ctx, []string{"eth0"}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	publisher.publishErr = errors.New("hostname collision")
	if err := reconcileApplianceDiscovery(ctx, management, &current, provisioning, commissioned); err == nil {
		t.Fatal("permanent replacement unexpectedly succeeded")
	}
	if publisher.withdrawals != 1 {
		t.Fatalf("withdrawals=%d, want shared alias removed before replacement", publisher.withdrawals)
	}
}

func TestLegacyPrivateListenerPermanentDiscoveryFollowsItsBoundInterface(t *testing.T) {
	provider := &runtimeRecoveryProvider{interfaces: []network.Interface{
		{Name: "eth0", Kind: network.KindEthernet, State: network.StateConnected, IPv4: []string{"192.168.1.22/24"}},
		{Name: "wlan0", Kind: network.KindWiFi, State: network.StateConnected, IPv4: []string{"10.44.118.42/24"}},
	}}
	interfaces, err := boundDiscoveryInterfaces(context.Background(), provider, "10.44.118.42:8443")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(interfaces, []string{"wlan0"}) {
		t.Fatalf("bound discovery interfaces = %v, want wlan0 only", interfaces)
	}
	interfaces, err = boundDiscoveryInterfaces(context.Background(), provider, "127.0.0.1:8443")
	if err != nil || len(interfaces) != 0 {
		t.Fatalf("loopback discovery interfaces = %v err=%v, want none", interfaces, err)
	}
}

func TestDiscoveryCertificateNamesAddTheSharedAliasOnlyForPiRecovery(t *testing.T) {
	if got := requiredDiscoveryTLSNames("bench-west", false); !slices.Equal(got, []string{"bench-west.local"}) {
		t.Fatalf("permanent discovery TLS names = %v", got)
	}
	if got := requiredDiscoveryTLSNames("bench-west", true); !slices.Equal(got, []string{"bench-west.local", "nilo.local"}) {
		t.Fatalf("Pi recovery discovery TLS names = %v", got)
	}
}

func TestLongPressTransitionsTheLiveRuntimeToSafeProvisioningWithoutARebootGap(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{interfaces: []network.Interface{
		{Name: "eth0", AdapterID: "management", Kind: network.KindEthernet, Physical: true, State: network.StateConnected, IPv4: []string{"192.0.2.10/24"}},
		{Name: "eth1", AdapterID: "free", Kind: network.KindEthernet, Physical: true, State: network.StateConnected, IPv4: []string{"169.254.10.20/16"}},
		{Name: "eth2", AdapterID: "plc", Kind: network.KindEthernet, Physical: true, State: network.StateConnected, IPv4: []string{"192.168.0.1/24"}},
	}}
	coordinator := network.NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []network.AdapterRoleClaim {
		return []network.AdapterRoleClaim{{AdapterID: "plc", Interface: "eth2", Role: network.AdapterRoleDevice}}
	})
	configuration := network.ManagementAccessConfiguration{Interfaces: []string{"eth0"}, Port: 8443, TLSNames: []string{"bench.local", "nilo.local"}}
	store := &runtimeAccessStore{present: true, record: network.ManagementAccessRecord{
		Configuration: configuration, LastKnownGood: configuration, State: network.ManagementAccessApplied, UpdatedAt: now,
	}}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: &runtimeFirewall{}, Coordinator: coordinator,
		Now: func() time.Time { return now }, ProvisioningTLSNames: []string{"bench.local", "nilo.local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Coordinator: coordinator, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000007", Generation: 7, Action: nethelper.HardwareRecoveryResetCommissioning,
		ReleasedAt: now, SnapshotValid: true,
	}}
	resetter := &runtimeResetter{}
	runtime := &hardwareRecoveryRuntime{
		helper: hardware, service: recovery, commission: resetter, management: management,
		audit: &runtimeAudit{}, applianceID: "appliance",
	}
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if !management.Provisioning() || !resetter.called || hardware.acked != hardware.event.ID {
		t.Fatalf("provisioning=%t reset=%t ack=%q", management.Provisioning(), resetter.called, hardware.acked)
	}
	for _, name := range resetter.access.Configuration.Interfaces {
		if name == "eth2" {
			t.Fatal("protected PLC interface entered derived provisioning admission")
		}
	}
}

func TestHardwareEventReplayAfterAcknowledgementFailureDoesNotRearmOrReset(t *testing.T) {
	ctx := context.Background()
	releasedAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	now := releasedAt
	provider := &runtimeRecoveryProvider{}
	configuration := network.ManagementAccessConfiguration{Interfaces: []string{"eth0"}, Port: 8443, TLSNames: []string{"bench.local"}}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{present: true, record: network.ManagementAccessRecord{
			Configuration: configuration, LastKnownGood: configuration, State: network.ManagementAccessApplied, UpdatedAt: now,
		}}, Firewall: &runtimeFirewall{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &runtimeRecoveryStore{record: network.RecoveryRecord{Version: 1, State: network.RecoveryIdle, UpdatedAt: now}}
	service, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Store: store, Now: func() time.Time { return now }, Window: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000009", Generation: 9, Action: nethelper.HardwareRecoveryArm,
		ReleasedAt: releasedAt, SnapshotValid: true,
	}, ackFailures: 1}
	runtime := &hardwareRecoveryRuntime{helper: hardware, service: service, management: management}
	if err := runtime.Reconcile(ctx); err == nil {
		t.Fatal("injected acknowledgement failure was ignored")
	}
	now = now.Add(2 * time.Minute)
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if service.Current().State != network.RecoveryIdle || service.Current().HardwareGeneration != 9 || hardware.acked != hardware.event.ID {
		t.Fatalf("arm replay was not consumed from idle tombstone: record=%+v ack=%q", service.Current(), hardware.acked)
	}

	resetStore := &runtimeRecoveryStore{record: network.RecoveryRecord{
		Version: 1, State: network.RecoveryIdle, HardwareGeneration: 12, UpdatedAt: now,
	}}
	resetService, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Store: resetStore, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	resetter := &runtimeResetter{}
	resetHardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000012", Generation: 12, Action: nethelper.HardwareRecoveryResetCommissioning,
		ReleasedAt: now, SnapshotValid: true,
	}}
	resetRuntime := &hardwareRecoveryRuntime{
		helper: resetHardware, service: resetService, management: management, commission: resetter,
	}
	if err := resetRuntime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if resetter.called || resetHardware.acked != resetHardware.event.ID {
		t.Fatalf("handled reset replay executed=%t ack=%q", resetter.called, resetHardware.acked)
	}
}

func TestProvisioningRefusalConsumesEventBeforeAcknowledgementFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{interfaces: []network.Interface{{
		Name: "eth0", AdapterID: "management", Kind: network.KindEthernet, Physical: true,
		State: network.StateConnected, IPv4: []string{"192.0.2.10/24"},
	}}}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"bench.local", "nilo.local"}, Commissioner: runtimeCommissioner{},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &runtimeRecoveryStore{record: network.RecoveryRecord{Version: 1, State: network.RecoveryIdle, UpdatedAt: now}}
	service, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Store: store,
		Now: func() time.Time { return now }, Audit: &runtimeAudit{}, SubjectID: "appliance",
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000021", Generation: 21, Action: nethelper.HardwareRecoveryArm,
		ReleasedAt: now, SnapshotValid: true,
	}, ackFailures: 1}
	runtime := &hardwareRecoveryRuntime{helper: hardware, service: service, management: management, audit: &runtimeAudit{}, applianceID: "appliance"}
	if err := runtime.Reconcile(ctx); err == nil {
		t.Fatal("injected acknowledgement failure was ignored")
	}
	if store.record.State != network.RecoveryIdle || store.record.HardwareGeneration != 21 {
		t.Fatalf("provisioning refusal was not durably consumed: %+v", store.record)
	}
	if _, err := management.Commission(ctx, []string{"eth0"}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if service.Current().State != network.RecoveryIdle || hardware.acked != hardware.event.ID {
		t.Fatalf("refused event replayed after commissioning: record=%+v ack=%q", service.Current(), hardware.acked)
	}
}

func TestArmPersistenceFailureCannotPublishHandledStateBeforeRetryAndRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{interfaces: []network.Interface{{
		Name: "eth0", AdapterID: "management", Kind: network.KindEthernet, Physical: true,
		State: network.StateConnected, IPv4: []string{"192.0.2.10/24"},
	}}}
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"eth0"}, Port: 8443, TLSNames: []string{"bench.local"},
	}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{present: true, record: network.ManagementAccessRecord{
			Configuration: configuration, LastKnownGood: configuration,
			State: network.ManagementAccessApplied, UpdatedAt: now,
		}}, Firewall: &runtimeFirewall{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &runtimeRecoveryStore{
		record:    network.RecoveryRecord{Version: 1, State: network.RecoveryIdle, UpdatedAt: now},
		failSaves: 1,
	}
	service, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Store: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000031", Generation: 31, Action: nethelper.HardwareRecoveryArm,
		ReleasedAt: now, ConnectedAdapterIDs: []string{"management"}, SnapshotValid: true,
	}}
	runtime := &hardwareRecoveryRuntime{helper: hardware, service: service, management: management}
	if err := runtime.Reconcile(ctx); err == nil {
		t.Fatal("injected arm persistence failure was ignored")
	}
	if hardware.acked != "" || service.Current().State != network.RecoveryIdle || service.Current().HardwareGeneration != 0 {
		t.Fatalf("failed arm was published or acknowledged: record=%+v ack=%q", service.Current(), hardware.acked)
	}
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if hardware.acked != hardware.event.ID || store.record.State != network.RecoveryArmed || store.record.HardwareGeneration != 31 {
		t.Fatalf("retried arm was not durably handled before ack: durable=%+v ack=%q", store.record, hardware.acked)
	}
	restarted, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Store: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Current().State != network.RecoveryArmed || restarted.Current().HardwareGeneration != 31 {
		t.Fatalf("restart lost the acknowledged arm: %+v", restarted.Current())
	}
}

func TestPhysicalArmRefusalAuditPrecedesHandledGenerationAndAcknowledgement(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 22, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{interfaces: []network.Interface{{
		Name: "eth0", AdapterID: "management", Kind: network.KindEthernet, Physical: true,
		State: network.StateConnected, IPv4: []string{"192.0.2.10/24"},
	}}}
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"eth0"}, Port: 8443, TLSNames: []string{"bench.local"},
	}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{present: true, record: network.ManagementAccessRecord{
			Configuration: configuration, LastKnownGood: configuration,
			State: network.ManagementAccessApplied, UpdatedAt: now,
		}}, Firewall: &runtimeFirewall{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &runtimeRecoveryStore{record: network.RecoveryRecord{Version: 1, State: network.RecoveryIdle, UpdatedAt: now}}
	journal := &runtimeAudit{failAction: "network.recovery.refused"}
	service, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Store: store,
		Now: func() time.Time { return now }, Audit: journal, SubjectID: "appliance",
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000041", Generation: 41, Action: nethelper.HardwareRecoveryArm,
		ReleasedAt: now, ConnectedAdapterIDs: []string{"management"}, SnapshotValid: true,
	}}
	runtime := &hardwareRecoveryRuntime{helper: hardware, service: service, management: management}
	if err := runtime.Reconcile(ctx); err == nil {
		t.Fatal("injected already-connected refusal audit failure was ignored")
	}
	if hardware.acked != "" || store.record.State != network.RecoveryIdle || store.record.HardwareGeneration != 0 {
		t.Fatalf("unaudited physical arm was published or acknowledged: durable=%+v ack=%q", store.record, hardware.acked)
	}

	journal.failAction = ""
	restarted, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Store: store,
		Now: func() time.Time { return now }, Audit: journal, SubjectID: "appliance",
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.service = restarted
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if hardware.acked != hardware.event.ID || store.record.State != network.RecoveryArmed || store.record.HardwareGeneration != 41 {
		t.Fatalf("retried physical arm was not audited and persisted before acknowledgement: durable=%+v ack=%q", store.record, hardware.acked)
	}
	found := false
	for _, record := range journal.records {
		if record.Action != "network.recovery.refused" {
			continue
		}
		details, _ := record.Details.(map[string]string)
		if details["reason"] == "already_connected_when_armed" && details["adapter_id"] == "management" {
			found = true
		}
	}
	if !found {
		t.Fatalf("successful retry did not record the physical snapshot refusal: %+v", journal.records)
	}
}
