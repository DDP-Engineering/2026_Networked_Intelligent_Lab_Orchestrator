package nethelper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

type memoryIdentifyActionStore struct {
	*memoryStore
	ledger     IdentifyActionLedger
	saves      int
	failSaveAt int
}

type identifyActionDriver struct {
	fakeDriver
	mdnsCalls       int
	reannounceCalls int
	mdnsOperations  map[string]bool
}

func (driver *identifyActionDriver) ReplaceMDNS(context.Context, network.MDNSAdvertisement) error {
	driver.mdnsCalls++
	return nil
}

func (driver *identifyActionDriver) ReannounceMDNS(
	_ context.Context, operationID string, _ network.MDNSAdvertisement,
) error {
	if driver.mdnsOperations == nil {
		driver.mdnsOperations = make(map[string]bool)
	}
	if driver.mdnsOperations[operationID] {
		return nil
	}
	driver.mdnsOperations[operationID] = true
	driver.mdnsCalls++
	driver.reannounceCalls++
	return nil
}

func (driver *identifyActionDriver) AcknowledgeMDNSReannouncement(operationID string) {
	delete(driver.mdnsOperations, operationID)
}

func (*identifyActionDriver) WithdrawMDNS(context.Context) error { return nil }

type identifyActionSSDP struct {
	publishCalls    int
	reannounceCalls int
	operations      map[string]bool
}

type durableIdentifyHardware struct {
	queue          *recoveryEventQueue
	indicatorCalls int
}

func (hardware *durableIdentifyHardware) ReadRecoveryEvent(context.Context) (HardwareRecoveryEvent, bool, error) {
	return HardwareRecoveryEvent{}, false, nil
}

func (hardware *durableIdentifyHardware) AcknowledgeRecoveryEvent(context.Context, string) error {
	return nil
}

func (hardware *durableIdentifyHardware) SetRecoveryIndicator(context.Context, HardwareIndicatorState) error {
	return nil
}

func (hardware *durableIdentifyHardware) SetIdentifyIndicator(
	_ context.Context, operationID string, activeUntil time.Time, _ HardwareIndicatorState,
) (time.Time, error) {
	actual, created, err := hardware.queue.SetIdentifyOperation(operationID, activeUntil)
	if created {
		hardware.indicatorCalls++
	}
	return actual, err
}

func (hardware *durableIdentifyHardware) AcknowledgeIdentifyIndicator(_ context.Context, operationID string) error {
	return hardware.queue.AcknowledgeIdentifyOperation(operationID, time.Now().UTC())
}

func (responder *identifyActionSSDP) Publish(context.Context, network.SSDPPresence) error {
	responder.publishCalls++
	return nil
}
func (responder *identifyActionSSDP) Reannounce(
	_ context.Context, operationID string, _ network.SSDPPresence,
) error {
	if responder.operations == nil {
		responder.operations = make(map[string]bool)
	}
	if responder.operations[operationID] {
		return nil
	}
	responder.operations[operationID] = true
	responder.publishCalls++
	responder.reannounceCalls++
	return nil
}
func (responder *identifyActionSSDP) AcknowledgeReannouncement(operationID string) {
	delete(responder.operations, operationID)
}
func (*identifyActionSSDP) Withdraw(context.Context) error { return nil }
func (*identifyActionSSDP) Current() bool                  { return true }

func TestIdentifyActionExactReceiptPersistsInPrivateFileStore(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	hardware := &fakeRecoveryHardware{}
	driver := &fakeDriver{}
	first, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.ConfigureRecoveryHardware(hardware); err != nil {
		t.Fatal(err)
	}
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789ad", Source: network.IdentifySourceInterface,
		ActiveUntil: time.Now().UTC().Add(10 * time.Second),
	}
	want, err := first.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEngine(driver, reopened, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.ConfigureRecoveryHardware(hardware); err != nil {
		t.Fatal(err)
	}
	got, err := second.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if hardware.indicatorCalls != 1 || !reflect.DeepEqual(got, want) {
		t.Fatalf("calls=%d got=%+v want=%+v", hardware.indicatorCalls, got, want)
	}
	if err := second.AcknowledgeIdentifyAction(context.Background(), request.OperationID); err != nil {
		t.Fatal(err)
	}
	ledger, err := reopened.LoadIdentifyActions()
	if err != nil || len(ledger.Operations) != 0 {
		t.Fatalf("acknowledged private receipt=%+v err=%v", ledger, err)
	}
}

func TestIdentifyActionReceiptPreservesCompleteReannouncementWithoutReplay(t *testing.T) {
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}}
	driver := &identifyActionDriver{}
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	hardware := &fakeRecoveryHardware{}
	if err := engine.ConfigureRecoveryHardware(hardware); err != nil {
		t.Fatal(err)
	}
	responder := &identifyActionSSDP{}
	if err := engine.ConfigureSSDP(responder); err != nil {
		t.Fatal(err)
	}
	advertisement := network.MDNSAdvertisement{HostName: "nilo-p2rjmzdkf1", Interfaces: []string{"eth0"}}
	if err := engine.PublishMDNS(context.Background(), advertisement); err != nil {
		t.Fatal(err)
	}
	presence, err := network.NewSSDPPresence(network.SSDPPresence{
		ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", Version: "0.8.0", Port: 8443,
		Interfaces: []string{"eth0"}, BootID: 1, ConfigID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.PublishSSDP(context.Background(), presence); err != nil {
		t.Fatal(err)
	}
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789ae", Source: network.IdentifySourceButton,
		ActiveUntil: time.Now().UTC().Add(10 * time.Second),
	}
	first, err := engine.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Reannouncement != network.IdentifyReannouncementComplete || !reflect.DeepEqual(second, first) {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	if hardware.indicatorCalls != 1 || driver.mdnsCalls != 2 || responder.publishCalls != 2 {
		t.Fatalf("indicator=%d mdns=%d ssdp=%d", hardware.indicatorCalls, driver.mdnsCalls, responder.publishCalls)
	}
}

func TestIdentifyActionProtocolCarriesTypedOperationAndExactReceipt(t *testing.T) {
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}}
	hardware := &fakeRecoveryHardware{}
	engine := newIdentifyActionEngine(t, store, hardware)
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789af", Source: network.IdentifySourceInterface,
		ActiveUntil: time.Now().UTC().Add(10 * time.Second),
	}
	response := dispatch(context.Background(), engine, Request{Op: "identify.execute", IdentifyAction: &request})
	if response.Error != "" || response.IdentifyAction == nil || response.IdentifyAction.Status != network.IdentifyAccepted {
		t.Fatalf("response = %+v", response)
	}
	repeated := dispatch(context.Background(), engine, Request{Op: "identify.execute", IdentifyAction: &request})
	if repeated.Error != "" || repeated.IdentifyAction == nil || !reflect.DeepEqual(*repeated.IdentifyAction, *response.IdentifyAction) || hardware.indicatorCalls != 1 {
		t.Fatalf("repeated=%+v calls=%d", repeated, hardware.indicatorCalls)
	}
	if acknowledged := dispatch(context.Background(), engine, Request{Op: "identify.ack", ID: request.OperationID}); acknowledged.Error != "" {
		t.Fatalf("acknowledgement = %+v", acknowledged)
	}
	if len(store.ledger.Operations) != 0 {
		t.Fatalf("acknowledged receipt retained: %+v", store.ledger)
	}
	if refused := dispatch(context.Background(), engine, Request{Op: "identify.execute", ID: "unexpected", IdentifyAction: &request}); refused.Code != "invalid_request" {
		t.Fatalf("unsafe envelope accepted: %+v", refused)
	}
	if refused := dispatch(context.Background(), engine, Request{Op: "identify.ack"}); refused.Code != "invalid_request" {
		t.Fatalf("empty acknowledgement accepted: %+v", refused)
	}
}

func (store *memoryIdentifyActionStore) LoadIdentifyActions() (IdentifyActionLedger, error) {
	return store.ledger, nil
}

func (store *memoryIdentifyActionStore) SaveIdentifyActions(ledger IdentifyActionLedger) error {
	store.saves++
	if store.failSaveAt != 0 && store.saves == store.failSaveAt {
		return errors.New("injected identify action save failure")
	}
	store.ledger = ledger
	return nil
}

func newIdentifyActionEngine(t *testing.T, store *memoryIdentifyActionStore, hardware *fakeRecoveryHardware) *Engine {
	t.Helper()
	driver := &fakeDriver{device: Device{Interface: network.Interface{
		ID: "eth-a", Name: "eth0", Kind: network.KindEthernet, State: network.StateConnected, Controllable: true,
	}}}
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ConfigureRecoveryHardware(hardware); err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestIdentifyActionExecutesPendingIntentOnceAfterCrashBeforeInvocation(t *testing.T) {
	now := time.Now().UTC()
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789ab", Source: network.IdentifySourceInterface,
		ActiveUntil: now.Add(10 * time.Second),
	}
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}, ledger: IdentifyActionLedger{
		Version: 1, Operations: []IdentifyActionRecord{{Version: 1, Request: request, State: IdentifyActionPending, UpdatedAt: now}},
	}}
	hardware := &fakeRecoveryHardware{}
	engine := newIdentifyActionEngine(t, store, hardware)

	result, err := engine.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if hardware.indicatorCalls != 1 || result.Status != network.IdentifyAccepted || !result.Supported {
		t.Fatalf("calls=%d result=%+v", hardware.indicatorCalls, result)
	}
}

func TestIdentifyActionReturnsDurableExactReceiptAfterCrashBeforeCorePersistence(t *testing.T) {
	now := time.Now().UTC()
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789ac", Source: network.IdentifySourceButton,
		ActiveUntil: now.Add(10 * time.Second),
	}
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}}
	hardware := &fakeRecoveryHardware{}
	first := newIdentifyActionEngine(t, store, hardware)
	want, err := first.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if hardware.indicatorCalls != 1 {
		t.Fatalf("first action calls = %d", hardware.indicatorCalls)
	}

	// Simulate Core and helper reconstruction after the external action returned
	// but before Core persisted its audit outcome. The helper-owned receipt must
	// return the exact result without another physical action.
	second := newIdentifyActionEngine(t, store, hardware)
	got, err := second.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if hardware.indicatorCalls != 1 {
		t.Fatalf("replayed physical action: calls=%d", hardware.indicatorCalls)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("receipt=%+v want=%+v", got, want)
	}
	if got.Status != network.IdentifyAccepted || !got.Supported || got.Reannouncement != network.IdentifyReannouncementUnavailable {
		t.Fatalf("unexpected exact receipt: %+v", got)
	}
}

func TestIdentifyActionPostEffectSaveFailureRetriesIdempotently(t *testing.T) {
	now := time.Date(2026, 9, 16, 18, 0, 0, 0, time.UTC)
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}, failSaveAt: 2}
	hardware := &fakeRecoveryHardware{}
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789b0", Source: network.IdentifySourceInterface,
		ActiveUntil: now.Add(10 * time.Second),
	}
	first := newIdentifyActionEngine(t, store, hardware)
	if _, err := first.ExecuteIdentifyAction(context.Background(), request); err == nil {
		t.Fatal("post-effect receipt save unexpectedly succeeded")
	}
	store.failSaveAt = 0
	second := newIdentifyActionEngine(t, store, hardware)
	result, err := second.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if hardware.indicatorCalls != 1 || result.Status != network.IdentifyAccepted || !result.Supported {
		t.Fatalf("indicator calls=%d result=%+v", hardware.indicatorCalls, result)
	}
}

func TestIdentifyActionExpiredRetryUsesDurableIndicatorDeadline(t *testing.T) {
	started := time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)
	now := started
	eventStore := &memoryHardwareEventStore{}
	queue, err := newRecoveryEventQueue(eventStore)
	if err != nil {
		t.Fatal(err)
	}
	hardware := &durableIdentifyHardware{queue: queue}
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}, failSaveAt: 2}
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789b5", Source: network.IdentifySourceInterface,
		ActiveUntil: started.Add(10 * time.Second),
	}
	newEngine := func() *Engine {
		engine, engineErr := NewEngine(&fakeDriver{}, store, func() time.Time { return now })
		if engineErr != nil {
			t.Fatal(engineErr)
		}
		if engineErr = engine.ConfigureRecoveryHardware(hardware); engineErr != nil {
			t.Fatal(engineErr)
		}
		return engine
	}
	if _, err = newEngine().ExecuteIdentifyAction(context.Background(), request); err == nil {
		t.Fatal("post-effect receipt save unexpectedly succeeded")
	}
	now = request.ActiveUntil.Add(time.Second)
	store.failSaveAt = 0
	result, err := newEngine().ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if hardware.indicatorCalls != 1 || !result.ActiveUntil.Equal(request.ActiveUntil) {
		t.Fatalf("indicator calls=%d active_until=%s want durable %s", hardware.indicatorCalls, result.ActiveUntil, request.ActiveUntil)
	}
}

func TestIdentifyActionPostReannouncementSaveFailureDoesNotReplayEffects(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}, failSaveAt: 6}
	hardware := &fakeRecoveryHardware{}
	driver := &identifyActionDriver{}
	responder := &identifyActionSSDP{}
	newEngine := func() *Engine {
		engine, err := NewEngine(driver, store, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		if err := engine.ConfigureRecoveryHardware(hardware); err != nil {
			t.Fatal(err)
		}
		if err := engine.ConfigureSSDP(responder); err != nil {
			t.Fatal(err)
		}
		if err := engine.PublishMDNS(context.Background(), network.MDNSAdvertisement{
			HostName: "nilo-p2rjmzdkf1", Interfaces: []string{"eth0"},
		}); err != nil {
			t.Fatal(err)
		}
		presence, err := network.NewSSDPPresence(network.SSDPPresence{
			ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", Version: "0.8.0", Port: 8443,
			Interfaces: []string{"eth0"}, BootID: 1, ConfigID: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := engine.PublishSSDP(context.Background(), presence); err != nil {
			t.Fatal(err)
		}
		return engine
	}
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789b3", Source: network.IdentifySourceButton,
		ActiveUntil: now.Add(10 * time.Second),
	}
	if _, err := newEngine().ExecuteIdentifyAction(context.Background(), request); err == nil {
		t.Fatal("post-reannouncement receipt save unexpectedly succeeded")
	}
	store.failSaveAt = 0
	result, err := newEngine().ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reannouncement != network.IdentifyReannouncementComplete || hardware.indicatorCalls != 1 ||
		driver.reannounceCalls != 1 || responder.reannounceCalls != 1 {
		t.Fatalf("result=%+v indicator=%d mdns=%d ssdp=%d", result, hardware.indicatorCalls,
			driver.reannounceCalls, responder.reannounceCalls)
	}
}

func TestIdentifyActionRestartPublicationFulfillsStartedReannouncement(t *testing.T) {
	now := time.Now().UTC()
	request := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789b4", Source: network.IdentifySourceButton,
		ActiveUntil: now.Add(10 * time.Second),
	}
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}, ledger: IdentifyActionLedger{
		Version: 1, Operations: []IdentifyActionRecord{{
			Version: 1, Request: request, State: IdentifyActionPending, IndicatorDone: true,
			MDNSResult: identifyEffectStarted, SSDPResult: identifyEffectStarted,
			Result: network.IdentifyActionResult{
				Supported: true, Status: network.IdentifyAccepted, ActiveUntil: request.ActiveUntil,
				Reannouncement: network.IdentifyReannouncementNotRequested,
			},
			UpdatedAt: now,
		}},
	}}
	driver := &identifyActionDriver{}
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ConfigureRecoveryHardware(&fakeRecoveryHardware{}); err != nil {
		t.Fatal(err)
	}
	responder := &identifyActionSSDP{}
	if err := engine.ConfigureSSDP(responder); err != nil {
		t.Fatal(err)
	}
	if err := engine.PublishMDNS(context.Background(), network.MDNSAdvertisement{
		HostName: "nilo-p2rjmzdkf1", Interfaces: []string{"eth0"},
	}); err != nil {
		t.Fatal(err)
	}
	presence, err := network.NewSSDPPresence(network.SSDPPresence{
		ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", Version: "0.8.0", Port: 8443,
		Interfaces: []string{"eth0"}, BootID: 1, ConfigID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.PublishSSDP(context.Background(), presence); err != nil {
		t.Fatal(err)
	}
	result, err := engine.ExecuteIdentifyAction(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reannouncement != network.IdentifyReannouncementComplete ||
		driver.reannounceCalls != 0 || responder.reannounceCalls != 0 {
		t.Fatalf("result=%+v explicit mdns=%d ssdp=%d", result, driver.reannounceCalls, responder.reannounceCalls)
	}
}

func TestIdentifyActionReceiptIsRetainedUntilCoreAcknowledges(t *testing.T) {
	now := time.Date(2026, 9, 16, 18, 1, 0, 0, time.UTC)
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}}
	hardware := &fakeRecoveryHardware{}
	engine := newIdentifyActionEngine(t, store, hardware)
	first := network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789b1", Source: network.IdentifySourceInterface,
		ActiveUntil: now.Add(10 * time.Second),
	}
	if _, err := engine.ExecuteIdentifyAction(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	for index := 1; index < identifyActionLimit; index++ {
		request := network.IdentifyActionRequest{
			OperationID: fmt.Sprintf("receipt-%03d", index), Source: network.IdentifySourceInterface,
			ActiveUntil: now.Add(10 * time.Second),
		}
		if _, err := engine.ExecuteIdentifyAction(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	overflow := network.IdentifyActionRequest{
		OperationID: "receipt-overflow", Source: network.IdentifySourceInterface,
		ActiveUntil: now.Add(10 * time.Second),
	}
	if _, err := engine.ExecuteIdentifyAction(context.Background(), overflow); err == nil {
		t.Fatal("unacknowledged receipt was evicted under ledger pressure")
	}
	if err := engine.AcknowledgeIdentifyAction(context.Background(), first.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ExecuteIdentifyAction(context.Background(), overflow); err != nil {
		t.Fatalf("acknowledged receipt did not release capacity: %v", err)
	}
}

func TestIdentifyActionRejectsUnboundedFutureDeadline(t *testing.T) {
	now := time.Date(2026, 9, 16, 18, 2, 0, 0, time.UTC)
	store := &memoryIdentifyActionStore{memoryStore: &memoryStore{}}
	hardware := &fakeRecoveryHardware{}
	engine, err := NewEngine(&fakeDriver{}, store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ConfigureRecoveryHardware(hardware); err != nil {
		t.Fatal(err)
	}
	_, err = engine.ExecuteIdentifyAction(context.Background(), network.IdentifyActionRequest{
		OperationID: "0199a2d0-1234-7abc-8def-0123456789b2", Source: network.IdentifySourceInterface,
		ActiveUntil: now.Add(31 * time.Second),
	})
	if err == nil || hardware.indicatorCalls != 0 || len(store.ledger.Operations) != 0 {
		t.Fatalf("unbounded request err=%v calls=%d ledger=%+v", err, hardware.indicatorCalls, store.ledger)
	}
}
