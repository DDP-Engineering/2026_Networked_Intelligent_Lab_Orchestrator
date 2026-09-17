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

type failingRestoreRuntimeRecoveryLinkDriver struct{ runtimeRecoveryLinkDriver }

func (*failingRestoreRuntimeRecoveryLinkDriver) RestoreRecoveryLink(context.Context, string) error {
	return errors.New("injected restore failure")
}

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

func TestRuntimeIndicatorArbiterPreservesIdentifyAndRecoveryPrecedence(t *testing.T) {
	hardware := &runtimeHardware{}
	arbiter := newRuntimeIndicatorArbiter(hardware)
	ctx := context.Background()
	if err := arbiter.SetIdentifyIndicator(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := arbiter.SetRecoveryIndicator(ctx, nethelper.HardwareIndicatorIdle); err != nil {
		t.Fatal(err)
	}
	if got := hardware.indicators[len(hardware.indicators)-1]; got != nethelper.HardwareIndicatorIdentify {
		t.Fatalf("maintenance cleared Identify: %v", hardware.indicators)
	}
	if err := arbiter.SetRecoveryIndicator(ctx, nethelper.HardwareIndicatorArmed); err != nil {
		t.Fatal(err)
	}
	if got := hardware.indicators[len(hardware.indicators)-1]; got != nethelper.HardwareIndicatorArmed {
		t.Fatalf("recovery did not take precedence: %v", hardware.indicators)
	}
	if err := arbiter.SetRecoveryIndicator(ctx, nethelper.HardwareIndicatorIdle); err != nil {
		t.Fatal(err)
	}
	if got := hardware.indicators[len(hardware.indicators)-1]; got != nethelper.HardwareIndicatorIdentify {
		t.Fatalf("Identify did not resume inside its deadline: %v", hardware.indicators)
	}
	if err := arbiter.SetIdentifyIndicator(ctx, false); err != nil {
		t.Fatal(err)
	}
	if got := hardware.indicators[len(hardware.indicators)-1]; got != nethelper.HardwareIndicatorIdle {
		t.Fatalf("expiry did not restore recovery state: %v", hardware.indicators)
	}
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
	records     []audit.Record
	failAction  string
	failCompose bool
}

func (writer *runtimeAudit) Compose(audit.Record) (audit.Event, error) {
	if writer.failCompose {
		writer.failCompose = false
		return audit.Event{}, errors.New("injected compose failure")
	}
	return audit.Event{}, nil
}
func (writer *runtimeAudit) Record(_ context.Context, record audit.Record) error {
	if writer.failAction == record.Action {
		return errors.New("injected audit failure")
	}
	writer.records = append(writer.records, record)
	return nil
}

type runtimeIdentifyOperations struct {
	state             string
	beginCalls        int
	reserveCalls      int
	markCalls         int
	outcomeCalls      int
	failOutcome       bool
	failOutcomeAlways bool
	flushCalls        int
	failMark          bool
	failFlush         bool
	supported         bool
	reannouncement    string
	status            string
	outcome           audit.Outcome
}

func (operations *runtimeIdentifyOperations) Begin(context.Context, string, string, time.Time, time.Time) (bool, error) {
	operations.beginCalls++
	if operations.state != "" {
		return false, nil
	}
	operations.state = "pending"
	return true, nil
}

func (operations *runtimeIdentifyOperations) State(context.Context, string) (string, error) {
	return operations.state, nil
}

func (operations *runtimeIdentifyOperations) Reserve(context.Context, string, audit.Event, time.Time) error {
	operations.reserveCalls++
	if operations.state != "pending" {
		return errors.New("identify operation is not pending")
	}
	operations.state = "reserved"
	return nil
}

func (operations *runtimeIdentifyOperations) MarkActed(
	context.Context, string, time.Time,
) error {
	operations.markCalls++
	if operations.failMark {
		operations.failMark = false
		return errors.New("injected identify acted-state failure")
	}
	if operations.state != "performed" && operations.state != "acted" {
		return errors.New("identify operation outcome is not recorded")
	}
	operations.state = "acted"
	return nil
}

func (operations *runtimeIdentifyOperations) RecordOutcome(
	_ context.Context, _ string, supported bool, reannouncement, status string, outcome audit.Outcome, _ time.Time,
) error {
	operations.outcomeCalls++
	if operations.failOutcomeAlways {
		return errors.New("injected persistent identify outcome persistence failure")
	}
	if operations.failOutcome {
		operations.failOutcome = false
		return errors.New("injected identify outcome persistence failure")
	}
	if operations.state != "reserved" && operations.state != "performed" {
		return errors.New("identify operation is not reserved")
	}
	operations.supported = supported
	operations.reannouncement = reannouncement
	operations.status = status
	operations.outcome = outcome
	operations.state = "performed"
	return nil
}

type runtimeIdentifyActionExecutor struct {
	calls    int
	acks     int
	receipts map[string]network.IdentifyActionResult
	result   *network.IdentifyActionResult
}

func (executor *runtimeIdentifyActionExecutor) ExecuteIdentifyAction(
	_ context.Context, request network.IdentifyActionRequest,
) (network.IdentifyActionResult, error) {
	if result, ok := executor.receipts[request.OperationID]; ok {
		return result, nil
	}
	executor.calls++
	result := network.IdentifyActionResult{
		Supported: true, Status: network.IdentifyAccepted, ActiveUntil: request.ActiveUntil,
		Reannouncement: network.IdentifyReannouncementComplete,
	}
	if executor.result != nil {
		result = *executor.result
	}
	if executor.receipts == nil {
		executor.receipts = make(map[string]network.IdentifyActionResult)
	}
	executor.receipts[request.OperationID] = result
	return result, nil
}

func (executor *runtimeIdentifyActionExecutor) AcknowledgeIdentifyAction(_ context.Context, operationID string) error {
	executor.acks++
	delete(executor.receipts, operationID)
	return nil
}

func (operations *runtimeIdentifyOperations) Flush(context.Context, string) error {
	operations.flushCalls++
	if operations.failFlush {
		operations.failFlush = false
		return errors.New("injected identify audit flush failure")
	}
	operations.state = "settled"
	return nil
}

type runtimePublisher struct {
	advertisements []network.MDNSAdvertisement
	withdrawals    int
	publishErr     error
}

func TestShortButtonPressIdentifiesDuringProvisioningWithoutNetworkMutation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"nilo-p2rjmzdkf1.local", "nilo.local"},
		Commissioner:         runtimeCommissioner{}, PublishedCredential: changedPublishedCredential{},
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000001", Generation: 1, Action: nethelper.HardwareRecoveryIdentify,
		ReleasedAt: now, SnapshotValid: true,
	}}
	identify, err := network.NewIdentifyService(network.IdentifyOptions{Indicator: newRuntimeIndicatorArbiter(hardware)})
	if err != nil {
		t.Fatal(err)
	}
	reannounced := 0
	auditWriter := &runtimeAudit{}
	runtime := &hardwareRecoveryRuntime{
		helper: hardware, service: recovery, management: management, identify: identify,
		reannounce: func(context.Context) error { reannounced++; return nil },
		audit:      auditWriter, applianceID: "appliance",
	}
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if hardware.acked != hardware.event.ID || reannounced != 1 || !management.Provisioning() {
		t.Fatalf("ack=%q reannounced=%d provisioning=%t", hardware.acked, reannounced, management.Provisioning())
	}
	if len(hardware.indicators) == 0 || hardware.indicators[0] != nethelper.HardwareIndicatorIdentify {
		t.Fatalf("indicator states = %v", hardware.indicators)
	}
	if len(auditWriter.records) != 1 || auditWriter.records[0].Action != "appliance.identify" {
		t.Fatalf("audit records = %+v", auditWriter.records)
	}
}

func TestPendingIdentifyCannotMaskRestoredRecoveryIndicator(t *testing.T) {
	now := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name      string
		record    network.RecoveryRecord
		driver    network.HardwareRecoveryDriver
		want      nethelper.HardwareIndicatorState
		wantError bool
	}{
		{
			name: "armed", want: nethelper.HardwareIndicatorArmed,
			record: network.RecoveryRecord{Version: 1, State: network.RecoveryArmed, ArmedAt: now.Add(-time.Minute), ExpiresAt: now.Add(9 * time.Minute), UpdatedAt: now.Add(-time.Minute)},
			driver: &runtimeRecoveryLinkDriver{},
		},
		{
			name: "active", want: nethelper.HardwareIndicatorActive,
			record: network.RecoveryRecord{Version: 1, State: network.RecoveryActive, ArmedAt: now.Add(-time.Minute), ExpiresAt: now.Add(9 * time.Minute), UpdatedAt: now.Add(-time.Minute), AdapterID: "adapter", Interface: "eth1", HelperRecordID: "helper-record"},
			driver: &runtimeRecoveryLinkDriver{},
		},
		{
			name: "failed", want: nethelper.HardwareIndicatorFailed, wantError: true,
			record: network.RecoveryRecord{Version: 1, State: network.RecoveryFailed, ArmedAt: now.Add(-time.Minute), ExpiresAt: now.Add(9 * time.Minute), UpdatedAt: now.Add(-time.Minute), AdapterID: "adapter", Interface: "eth1", HelperRecordID: "helper-record"},
			driver: &failingRestoreRuntimeRecoveryLinkDriver{},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
				Provider: &runtimeRecoveryProvider{}, Driver: testCase.driver,
				Store: &runtimeRecoveryStore{record: testCase.record}, Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
				ID: "gpio-00000000000000000051", Generation: 51, Action: nethelper.HardwareRecoveryIdentify,
				ReleasedAt: now, SnapshotValid: true,
			}}
			indicator := newRuntimeIndicatorArbiter(hardware)
			identify, err := network.NewIdentifyService(network.IdentifyOptions{Indicator: indicator})
			if err != nil {
				t.Fatal(err)
			}
			runtime := &hardwareRecoveryRuntime{helper: hardware, service: service, identify: identify, indicator: indicator}
			err = runtime.Reconcile(context.Background())
			if testCase.wantError {
				if err == nil {
					t.Fatal("failed recovery restoration unexpectedly permitted pending Identify")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(hardware.indicators) == 0 || hardware.indicators[len(hardware.indicators)-1] != testCase.want {
				t.Fatalf("indicator states=%v want final %q", hardware.indicators, testCase.want)
			}
			for _, state := range hardware.indicators {
				if state == nethelper.HardwareIndicatorIdentify {
					t.Fatalf("pending Identify masked %q recovery state: %v", testCase.want, hardware.indicators)
				}
			}
		})
	}
}

func TestShortButtonIdentifyRetriesAuditWithoutRepeatingPhysicalAction(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"nilo-p2rjmzdkf1.local", "nilo.local"},
		Commissioner:         runtimeCommissioner{}, PublishedCredential: changedPublishedCredential{},
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000002", Generation: 2, Action: nethelper.HardwareRecoveryIdentify,
		ReleasedAt: now, SnapshotValid: true,
	}}
	identify, err := network.NewIdentifyService(network.IdentifyOptions{Indicator: newRuntimeIndicatorArbiter(hardware)})
	if err != nil {
		t.Fatal(err)
	}
	operations := &runtimeIdentifyOperations{failFlush: true}
	reannounced := 0
	runtime := &hardwareRecoveryRuntime{
		helper: hardware, service: recovery, management: management, identify: identify,
		reannounce:  func(context.Context) error { reannounced++; return nil },
		audit:       &runtimeAudit{},
		identifyOps: operations,
		applianceID: "appliance",
	}
	if err := runtime.Reconcile(ctx); err == nil {
		t.Fatal("first audit flush unexpectedly succeeded")
	}
	if operations.state != "acted" || hardware.acked != "" || reannounced != 1 || len(hardware.indicators) != 1 {
		t.Fatalf("after failed flush state=%q ack=%q reannounced=%d indicators=%v", operations.state, hardware.acked, reannounced, hardware.indicators)
	}
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if operations.state != "settled" || operations.markCalls != 1 || operations.flushCalls != 2 ||
		hardware.acked != hardware.event.ID || reannounced != 1 || len(hardware.indicators) != 1 {
		t.Fatalf("after retry state=%q mark=%d flush=%d ack=%q reannounced=%d indicators=%v",
			operations.state, operations.markCalls, operations.flushCalls, hardware.acked, reannounced, hardware.indicators)
	}
}

func TestShortButtonIdentifyPersistentOutcomeFailureRecoversWithoutRepeatingPhysicalAction(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 16, 14, 0, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"nilo-p2rjmzdkf1.local", "nilo.local"},
		Commissioner:         runtimeCommissioner{}, PublishedCredential: changedPublishedCredential{},
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000004", Generation: 4, Action: nethelper.HardwareRecoveryIdentify,
		ReleasedAt: now, SnapshotValid: true,
	}}
	identify, err := network.NewIdentifyService(network.IdentifyOptions{Indicator: newRuntimeIndicatorArbiter(hardware)})
	if err != nil {
		t.Fatal(err)
	}
	operations := &runtimeIdentifyOperations{failOutcomeAlways: true}
	executor := &runtimeIdentifyActionExecutor{}
	runtime := &hardwareRecoveryRuntime{
		helper: hardware, service: recovery, management: management, identify: identify,
		audit: &runtimeAudit{}, identifyOps: operations, identifyAction: executor, applianceID: "appliance",
	}
	if err := runtime.Reconcile(ctx); err == nil {
		t.Fatal("persistent outcome failure unexpectedly succeeded")
	}
	if operations.state != "reserved" || operations.outcomeCalls != 3 || executor.calls != 1 {
		t.Fatalf("after persistent failure state=%q outcomes=%d action calls=%d",
			operations.state, operations.outcomeCalls, executor.calls)
	}
	operations.failOutcomeAlways = false
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if operations.state != "settled" || hardware.acked != hardware.event.ID || executor.calls != 1 {
		t.Fatalf("after recovery state=%q ack=%q action calls=%d",
			operations.state, hardware.acked, executor.calls)
	}
}

func TestShortButtonIdentifyCrashBeforeExternalInvocationExecutesOnceWithExactAuditResult(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 16, 15, 2, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"nilo-p2rjmzdkf1.local", "nilo.local"},
		Commissioner:         runtimeCommissioner{}, PublishedCredential: changedPublishedCredential{},
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000005", Generation: 5, Action: nethelper.HardwareRecoveryIdentify,
		ReleasedAt: now, SnapshotValid: true,
	}}
	operations := &runtimeIdentifyOperations{state: "reserved"}
	executor := &runtimeIdentifyActionExecutor{}
	runtime := &hardwareRecoveryRuntime{
		helper: hardware, service: recovery, management: management,
		audit: &runtimeAudit{}, identifyOps: operations, identifyAction: executor, applianceID: "appliance",
	}
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if executor.calls != 1 || operations.state != "settled" || !operations.supported ||
		operations.reannouncement != "complete" || operations.status != "accepted" || operations.outcome != audit.OutcomeApplied {
		t.Fatalf("calls=%d state=%q result=%t/%s/%s/%s", executor.calls, operations.state,
			operations.supported, operations.reannouncement, operations.status, operations.outcome)
	}
}

func TestShortButtonIdentifyCrashAfterExternalSuccessReturnsExactReceiptWithoutReplay(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 16, 15, 3, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"nilo-p2rjmzdkf1.local", "nilo.local"},
		Commissioner:         runtimeCommissioner{}, PublishedCredential: changedPublishedCredential{},
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000006", Generation: 6, Action: nethelper.HardwareRecoveryIdentify,
		ReleasedAt: now, SnapshotValid: true,
	}}
	operations := &runtimeIdentifyOperations{failOutcomeAlways: true}
	executor := &runtimeIdentifyActionExecutor{}
	runtime := &hardwareRecoveryRuntime{
		helper: hardware, service: recovery, management: management,
		audit: &runtimeAudit{}, identifyOps: operations, identifyAction: executor, applianceID: "appliance",
	}
	if err := runtime.Reconcile(ctx); err == nil {
		t.Fatal("persistent Core outcome failure unexpectedly succeeded")
	}
	operations.failOutcomeAlways = false
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if executor.calls != 1 || operations.state != "settled" || !operations.supported ||
		operations.reannouncement != "complete" || operations.status != "accepted" || operations.outcome != audit.OutcomeApplied {
		t.Fatalf("calls=%d state=%q result=%t/%s/%s/%s", executor.calls, operations.state,
			operations.supported, operations.reannouncement, operations.status, operations.outcome)
	}
}

func TestShortButtonIdentifyPersistsExactFailedReceipt(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 16, 18, 3, 0, 0, time.UTC)
	provider := &runtimeRecoveryProvider{}
	management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
		Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
		Now: func() time.Time { return now }, Provisioning: true,
		ProvisioningTLSNames: []string{"nilo-p2rjmzdkf1.local", "nilo.local"},
		Commissioner:         runtimeCommissioner{}, PublishedCredential: changedPublishedCredential{},
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
		Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
		ID: "gpio-00000000000000000007", Generation: 7, Action: nethelper.HardwareRecoveryIdentify,
		ReleasedAt: now, SnapshotValid: true,
	}}
	operations := &runtimeIdentifyOperations{}
	executor := &runtimeIdentifyActionExecutor{result: &network.IdentifyActionResult{
		Supported: false, Status: network.IdentifyFailed,
		Reannouncement: network.IdentifyReannouncementPartial,
	}}
	runtime := &hardwareRecoveryRuntime{
		helper: hardware, service: recovery, management: management,
		audit: &runtimeAudit{}, identifyOps: operations, identifyAction: executor, applianceID: "appliance",
	}
	if err := runtime.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if operations.supported || operations.status != "failed" || operations.outcome != audit.OutcomeFailed ||
		operations.reannouncement != network.IdentifyReannouncementPartial {
		t.Fatalf("stored result=%t/%s/%s/%s", operations.supported, operations.status,
			operations.outcome, operations.reannouncement)
	}
}

func TestShortButtonIdentifyPreOutboxFailuresDoNotRepeatPhysicalAction(t *testing.T) {
	tests := []struct {
		name             string
		audit            *runtimeAudit
		operations       *runtimeIdentifyOperations
		firstShouldError bool
	}{
		{name: "compose", audit: &runtimeAudit{failCompose: true}, operations: &runtimeIdentifyOperations{}, firstShouldError: true},
		{name: "record outcome", audit: &runtimeAudit{}, operations: &runtimeIdentifyOperations{failOutcome: true}},
		{name: "mark acted", audit: &runtimeAudit{}, operations: &runtimeIdentifyOperations{failMark: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
			provider := &runtimeRecoveryProvider{}
			management, err := network.NewManagementAccessService(ctx, network.ManagementAccessOptions{
				Provider: provider, Store: &runtimeAccessStore{}, Firewall: &runtimeFirewall{},
				Now: func() time.Time { return now }, Provisioning: true,
				ProvisioningTLSNames: []string{"nilo-p2rjmzdkf1.local", "nilo.local"},
				Commissioner:         runtimeCommissioner{}, PublishedCredential: changedPublishedCredential{},
			})
			if err != nil {
				t.Fatal(err)
			}
			recovery, err := network.NewHardwareRecoveryService(network.HardwareRecoveryOptions{
				Provider: provider, Driver: &runtimeRecoveryLinkDriver{}, Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			hardware := &runtimeHardware{event: nethelper.HardwareRecoveryEvent{
				ID: "gpio-00000000000000000003", Generation: 3, Action: nethelper.HardwareRecoveryIdentify,
				ReleasedAt: now, SnapshotValid: true,
			}}
			identify, err := network.NewIdentifyService(network.IdentifyOptions{Indicator: newRuntimeIndicatorArbiter(hardware)})
			if err != nil {
				t.Fatal(err)
			}
			reannounced := 0
			runtime := &hardwareRecoveryRuntime{
				helper: hardware, service: recovery, management: management, identify: identify,
				reannounce: func(context.Context) error { reannounced++; return nil },
				audit:      test.audit, identifyOps: test.operations, applianceID: "appliance",
			}
			firstErr := runtime.Reconcile(ctx)
			if test.firstShouldError {
				if firstErr == nil {
					t.Fatal("first outbox failure unexpectedly succeeded")
				}
				if err := runtime.Reconcile(ctx); err != nil {
					t.Fatal(err)
				}
			} else if firstErr != nil {
				t.Fatal(firstErr)
			}
			if reannounced != 1 || len(hardware.indicators) != 1 || hardware.acked != hardware.event.ID {
				t.Fatalf("reannounced=%d indicators=%v ack=%q state=%q", reannounced, hardware.indicators, hardware.acked, test.operations.state)
			}
			if test.name == "mark acted" && test.operations.markCalls != 2 {
				t.Fatalf("transient outcome persistence attempts = %d, want 2", test.operations.markCalls)
			}
			if test.name == "record outcome" && test.operations.outcomeCalls != 2 {
				t.Fatalf("transient physical outcome writes = %d, want 2", test.operations.outcomeCalls)
			}
		})
	}
}

func (publisher *runtimePublisher) PublishMDNS(_ context.Context, advertisement network.MDNSAdvertisement) error {
	publisher.advertisements = append(publisher.advertisements, advertisement)
	return publisher.publishErr
}
func (publisher *runtimePublisher) WithdrawMDNS(context.Context) error {
	publisher.withdrawals++
	return nil
}

// changedPublishedCredential is an appliance whose published administrator
// password has already been replaced, so the commissioning gate lets it past.
type changedPublishedCredential struct{}

func (changedPublishedCredential) PublishedVerifierRemains(context.Context) (bool, error) {
	return false, nil
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
		// These appliances have had the published password replaced already,
		// which is what lets them commission at all.
		PublishedCredential: changedPublishedCredential{},
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
		// These appliances have had the published password replaced already,
		// which is what lets them commission at all.
		PublishedCredential: changedPublishedCredential{},
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
		// These appliances have had the published password replaced already,
		// which is what lets them commission at all.
		PublishedCredential: changedPublishedCredential{},
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
