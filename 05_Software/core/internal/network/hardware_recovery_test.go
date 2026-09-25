package network

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
)

type recoveryProvider struct {
	interfaces []Interface
}

func (provider *recoveryProvider) Interfaces(context.Context) ([]Interface, error) {
	return cloneInterfaces(provider.interfaces), nil
}

func (*recoveryProvider) Source() Source { return "test" }

type recoveryDriver struct {
	activated   []string
	extended    []time.Time
	restored    []string
	activateErr error
	extendErr   error
	failedID    string
	restoreErr  error
}

func (driver *recoveryDriver) ActivateRecoveryLink(_ context.Context, adapterID, interfaceName string, _ time.Time) (string, error) {
	driver.activated = append(driver.activated, adapterID+":"+interfaceName)
	if driver.activateErr != nil {
		return driver.failedID, driver.activateErr
	}
	return "recovery-link-1", nil
}

func TestRecoveryActivationFailureRetainsHelperIdentityUntilRestoreIsProved(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{activateErr: errors.New("alias failed"), failedID: "recovery-link-partial"}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{Provider: provider, Driver: driver, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	record, err := service.Reconcile(context.Background())
	if err == nil || record.State != RecoveryFailed || record.HelperRecordID != "recovery-link-partial" {
		t.Fatalf("record=%+v err=%v, want retained helper identity", record, err)
	}
	driver.activateErr = nil
	record, err = service.Reconcile(context.Background())
	if err != nil || record.State != RecoveryIdle || !slices.Equal(driver.restored, []string{"recovery-link-partial"}) {
		t.Fatalf("record=%+v err=%v restored=%v", record, err, driver.restored)
	}
}

type failingRecoveryAudit struct{ calls int }

func (journal *failingRecoveryAudit) Record(context.Context, audit.Record) error {
	journal.calls++
	return errors.New("audit unavailable")
}

func TestRecoveryExpiryRestoresEvenWhenAuditIsUnavailable(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected}}}
	driver := &recoveryDriver{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{Provider: provider, Driver: driver, Now: func() time.Time { return now }, Window: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	service.audit = &failingRecoveryAudit{}
	now = now.Add(2 * time.Minute)
	record, err := service.Reconcile(context.Background())
	if record.State != RecoveryRestoring || !slices.Contains(record.PendingAudits, pendingRestoredAudit) || !slices.Equal(driver.restored, []string{"recovery-link-1"}) {
		t.Fatalf("cleanup was blocked by audit: record=%+v err=%v restored=%v", record, err, driver.restored)
	}
}

type recoveryAudit struct {
	records []audit.Record
}

func recoveryAuditReason(record audit.Record) string {
	details, _ := record.Details.(map[string]string)
	return details["reason"]
}

type switchableRecoveryAudit struct {
	fail       bool
	failAction string
	records    []audit.Record
}

func (journal *switchableRecoveryAudit) Record(_ context.Context, record audit.Record) error {
	if journal.fail || journal.failAction == record.Action {
		return errors.New("audit unavailable")
	}
	journal.records = append(journal.records, record)
	return nil
}

type memoryRecoveryStore struct {
	record  RecoveryRecord
	found   bool
	saves   int
	failAt  int
	history []RecoveryRecord
}

func (store *memoryRecoveryStore) LoadHardwareRecovery(context.Context) (RecoveryRecord, bool, error) {
	return store.record, store.found, nil
}

func (store *memoryRecoveryStore) SaveHardwareRecovery(_ context.Context, record RecoveryRecord) error {
	store.saves++
	if store.saves == store.failAt {
		return errors.New("injected recovery persistence failure")
	}
	record.PendingAudits = append([]string(nil), record.PendingAudits...)
	store.history = append(store.history, record)
	store.record, store.found = record, true
	return nil
}

func TestRecoveryPersistsPendingAuditWithTheFirstAdvancedFailureOrCancelState(t *testing.T) {
	now := time.Date(2026, 9, 14, 17, 15, 0, 0, time.UTC)

	t.Run("activation failure", func(t *testing.T) {
		provider := &recoveryProvider{interfaces: []Interface{{
			Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
		}}}
		store := &memoryRecoveryStore{}
		service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
			Provider: provider, Driver: &recoveryDriver{activateErr: errors.New("partial activation")},
			Store: store, Audit: &switchableRecoveryAudit{failAction: "network.recovery.failed"},
			Now: func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.Arm(context.Background()); err != nil {
			t.Fatal(err)
		}
		provider.interfaces[0].State = StateConnected
		if _, err = service.Reconcile(context.Background()); err == nil {
			t.Fatal("activation failure unexpectedly succeeded")
		}
		if len(store.history) < 2 || store.history[1].State != RecoveryFailed ||
			!hasPendingAudit(store.history[1], pendingActivateFailed) {
			t.Fatalf("first failed durable record lacked its audit marker: history=%+v", store.history)
		}
	})

	t.Run("armed cancellation", func(t *testing.T) {
		store := &memoryRecoveryStore{}
		service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
			Provider: &recoveryProvider{}, Driver: &recoveryDriver{}, Store: store,
			Audit: &switchableRecoveryAudit{failAction: "network.recovery.restored"},
			Now:   func() time.Time { return now },
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.Arm(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err = service.Cancel(context.Background()); err == nil {
			t.Fatal("injected cancellation audit failure was ignored")
		}
		if len(store.history) < 2 || store.history[1].State != RecoveryIdle ||
			!hasPendingAudit(store.history[1], pendingCancelAudit) {
			t.Fatalf("first cancelled durable record lacked its audit marker: history=%+v", store.history)
		}
	})
}

func TestRecoveryRestoresTheHelperEffectWhenActiveStateCannotBePersisted(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{}
	store := &memoryRecoveryStore{failAt: 2}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Store: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	record, err := service.Reconcile(context.Background())
	if err == nil {
		t.Fatal("activation succeeded despite losing its durable active record")
	}
	if !slices.Equal(driver.restored, []string{"recovery-link-1"}) {
		t.Fatalf("helper effect was stranded after persistence failure: restored=%v", driver.restored)
	}
	if record.State != RecoveryIdle || store.record.State != RecoveryIdle {
		t.Fatalf("cleanup did not settle idle: record=%+v durable=%+v", record, store.record)
	}
}

func (journal *recoveryAudit) Record(_ context.Context, record audit.Record) error {
	journal.records = append(journal.records, record)
	return nil
}

func TestRecoveryRetriesArmAndArmedExpiryWhenTheirAuditIsUnavailable(t *testing.T) {
	now := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	store := &memoryRecoveryStore{}
	journal := &switchableRecoveryAudit{fail: true}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: &recoveryProvider{}, Driver: &recoveryDriver{}, Store: store,
		Now: func() time.Time { return now }, Window: time.Minute, Audit: journal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ArmHardwareEvent(context.Background(), RecoveryArmEvent{
		Generation: 7, ReleasedAt: now, SnapshotValid: true,
	}); err == nil || store.found {
		t.Fatalf("unaudited arm persisted: found=%t err=%v", store.found, err)
	}
	journal.fail = false
	if _, err := service.ArmHardwareEvent(context.Background(), RecoveryArmEvent{
		Generation: 7, ReleasedAt: now, SnapshotValid: true,
	}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	journal.failAction = "network.recovery.expired"
	record, err := service.Reconcile(context.Background())
	if err == nil || record.State != RecoveryArmed {
		t.Fatalf("unaudited armed expiry was discarded: record=%+v err=%v", record, err)
	}
	journal.failAction = ""
	record, err = service.Reconcile(context.Background())
	if err != nil || record.State != RecoveryIdle || record.HardwareGeneration != 7 {
		t.Fatalf("retried armed expiry=%+v err=%v", record, err)
	}
	if len(journal.records) != 2 || journal.records[0].Action != "network.recovery.armed" || journal.records[1].Action != "network.recovery.expired" {
		t.Fatalf("audit records=%+v", journal.records)
	}
}

func (driver *recoveryDriver) ExtendRecoveryLink(_ context.Context, _ string, expiresAt time.Time) error {
	driver.extended = append(driver.extended, expiresAt)
	return driver.extendErr
}

func (driver *recoveryDriver) RestoreRecoveryLink(_ context.Context, recordID string) error {
	driver.restored = append(driver.restored, recordID)
	return driver.restoreErr
}

func TestRecoveryRequiresAPostArmSafeLinkTransitionAndChoosesOneAdapter(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{
		{ID: "eth0", Name: "eth0", AdapterID: "b", Kind: KindEthernet, Physical: true, State: StateConnected},
		{ID: "eth1", Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected},
		{ID: "eth2", Name: "eth2", AdapterID: "c", Kind: KindEthernet, Physical: true, State: StateDisconnected},
	}}
	driver := &recoveryDriver{}
	coordinator := NewAdapterRoleCoordinator()
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Coordinator: coordinator,
		Now: func() time.Time { return now }, Window: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(driver.activated) != 0 {
		t.Fatalf("already-connected adapter activated: %v", driver.activated)
	}

	provider.interfaces[1].State = StateConnected
	provider.interfaces[2].State = StateConnected
	record, err := service.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(driver.activated, []string{"a:eth1"}) {
		t.Fatalf("activated = %v, want stable first eligible adapter", driver.activated)
	}
	if record.State != RecoveryActive || record.AdapterID != "a" || record.Interface != "eth1" {
		t.Fatalf("active record = %+v", record)
	}
	if err := coordinator.Check([]AdapterRoleClaim{{AdapterID: "a", Role: AdapterRoleDevice}}); err == nil {
		t.Fatal("the active recovery adapter was not protected from another role")
	}
}

func TestRecoveryRefusesProtectedSavedAndConfiguredAdapters(t *testing.T) {
	coordinator := NewAdapterRoleCoordinator()
	coordinator.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{AdapterID: "device", Interface: "eth1", Role: AdapterRoleDevice}}
	})
	for _, candidate := range []Interface{
		{Name: "eth0", AdapterID: "device", Kind: KindEthernet, Physical: true, State: StateConnected},
		{Name: "eth0", AdapterID: "saved", Kind: KindEthernet, Physical: true, State: StateConnected, Settings: &SettingsState{Saved: true, Method: MethodAutomatic}},
		{Name: "eth0", AdapterID: "static", Kind: KindEthernet, Physical: true, State: StateConnected, Settings: &SettingsState{Saved: true, Method: MethodStatic}},
		{Name: "eth0", AdapterID: "addressed", Kind: KindEthernet, Physical: true, State: StateConnected, IPv4: []string{"192.0.2.10/24"}},
		{Name: "eth0", AdapterID: "gateway", Kind: KindEthernet, Physical: true, State: StateConnected, Gateway4: "192.0.2.1"},
	} {
		if err := RecoveryInterfaceEligible(candidate, coordinator); err == nil {
			t.Errorf("candidate %+v was accepted", candidate)
		}
	}
	if err := RecoveryInterfaceEligible(Interface{
		Name: "eth9", AdapterID: "free", Kind: KindEthernet, Physical: true, State: StateConnected,
		IPv6: []string{"fe80::1/64"},
	}, coordinator); err != nil {
		t.Fatalf("safe unassigned Ethernet with ordinary IPv6 link-local was refused: %v", err)
	}
	if err := RecoveryInterfaceEligible(Interface{
		Name: "eth9", AdapterID: "global-v6", Kind: KindEthernet, Physical: true, State: StateConnected,
		IPv6: []string{"2001:db8::10/64"},
	}, coordinator); err == nil {
		t.Fatal("globally addressed IPv6 interface was accepted")
	}
}

func TestRecoveryExpiryRestoresOwnedLinkAndRepeatArmExtendsWithoutSwitching(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Coordinator: NewAdapterRoleCoordinator(),
		Now: func() time.Time { return now }, Window: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	if _, err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(9 * time.Minute)
	record, err := service.Arm(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if record.ExpiresAt != now.Add(10*time.Minute) || len(driver.activated) != 1 ||
		!slices.Equal(driver.extended, []time.Time{record.ExpiresAt}) {
		t.Fatalf("extended record = %+v; activations=%v", record, driver.activated)
	}
	now = now.Add(11 * time.Minute)
	record, err = service.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if record.State != RecoveryIdle || !slices.Equal(driver.restored, []string{"recovery-link-1"}) {
		t.Fatalf("expiry record=%+v restored=%v", record, driver.restored)
	}
}

func TestRecoveryArmEventIsIdempotentAcrossCoreRestartBeforeAcknowledgement(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected}}}
	store := &memoryRecoveryStore{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{Provider: provider, Driver: &recoveryDriver{}, Store: store, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.ArmEvent(context.Background(), 17)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	restarted, err := NewHardwareRecoveryService(HardwareRecoveryOptions{Provider: provider, Driver: &recoveryDriver{}, Store: store, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.ArmEvent(context.Background(), 17)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ExpiresAt != first.ExpiresAt || replayed.HardwareGeneration != 17 {
		t.Fatalf("replayed arm changed deadline: first=%+v replayed=%+v", first, replayed)
	}
}

func TestRecoveryArmUsesTheDurableButtonReleaseTimeAndLinkSnapshot(t *testing.T) {
	releasedAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	now := releasedAt.Add(5 * time.Minute)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth0", AdapterID: "already-connected", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: &recoveryDriver{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.ArmHardwareEvent(context.Background(), RecoveryArmEvent{
		Generation: 41, ReleasedAt: releasedAt, ConnectedAtRelease: []string{"already-connected"}, SnapshotValid: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.ArmedAt != releasedAt || record.ExpiresAt != releasedAt.Add(RecoveryWindow) {
		t.Fatalf("arm timing=%+v, want release-based window", record)
	}
	provider.interfaces[0].State = StateConnected
	if _, err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.Current().State != RecoveryArmed {
		t.Fatal("a link connected at button release became a later recovery candidate")
	}
}

func TestRecoveryEventGenerationSurvivesIdleAndClockRollbackCannotExtendTheWindow(t *testing.T) {
	releasedAt := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	now := releasedAt
	store := &memoryRecoveryStore{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: &recoveryProvider{}, Driver: &recoveryDriver{}, Store: store, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	event := RecoveryArmEvent{Generation: 52, ReleasedAt: releasedAt, SnapshotValid: true}
	if _, err := service.ArmHardwareEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	now = releasedAt.Add(RecoveryWindow + time.Second)
	record, err := service.Reconcile(context.Background())
	if err != nil || record.State != RecoveryIdle || record.HardwareGeneration != 52 {
		t.Fatalf("expired record=%+v err=%v", record, err)
	}
	replayed, err := service.ArmHardwareEvent(context.Background(), event)
	if err != nil || replayed.State != RecoveryIdle || replayed.HardwareGeneration != 52 {
		t.Fatalf("replayed event changed idle tombstone: record=%+v err=%v", replayed, err)
	}

	rolledBack, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: &recoveryProvider{}, Driver: &recoveryDriver{}, Now: func() time.Time { return releasedAt.Add(-time.Second) },
	})
	if err != nil {
		t.Fatal(err)
	}
	refused, err := rolledBack.ArmHardwareEvent(context.Background(), RecoveryArmEvent{
		Generation: 1, ReleasedAt: releasedAt, SnapshotValid: true,
	})
	if err != nil || refused.State != RecoveryIdle || refused.HardwareGeneration != 1 {
		t.Fatalf("clock rollback event was not durably refused: record=%+v err=%v", refused, err)
	}
}

func TestFutureDatedHardwareEventIsConsumedBeforeWallClockCatchesUp(t *testing.T) {
	releasedAt := time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)
	now := releasedAt.Add(-time.Minute)
	store := &memoryRecoveryStore{}
	journal := &recoveryAudit{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: &recoveryProvider{}, Driver: &recoveryDriver{}, Store: store,
		Now: func() time.Time { return now }, Audit: journal, SubjectID: "appliance",
	})
	if err != nil {
		t.Fatal(err)
	}
	event := RecoveryArmEvent{Generation: 61, ReleasedAt: releasedAt, SnapshotValid: true}
	record, err := service.ArmHardwareEvent(context.Background(), event)
	if err != nil || record.State != RecoveryIdle || record.HardwareGeneration != 61 {
		t.Fatalf("future release was not durably refused: record=%+v err=%v", record, err)
	}
	now = releasedAt.Add(time.Minute)
	replayed, err := service.ArmHardwareEvent(context.Background(), event)
	if err != nil || replayed.State != RecoveryIdle || replayed.HardwareGeneration != 61 {
		t.Fatalf("future release replayed after clock catch-up: record=%+v err=%v", replayed, err)
	}
}

func TestRecoveryRetriesPostMutationAuditWithoutRepeatingTheMutation(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{}
	store := &memoryRecoveryStore{}
	journal := &switchableRecoveryAudit{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Store: store, Now: func() time.Time { return now },
		Window: time.Minute, Audit: journal, SubjectID: "appliance",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	journal.failAction = "network.recovery.activated"
	record, err := service.Reconcile(context.Background())
	if err == nil || record.State != RecoveryActive || !slices.Contains(record.PendingAudits, pendingActivationAudit) {
		t.Fatalf("activation audit was not left durable: record=%+v err=%v", record, err)
	}
	journal.failAction = ""
	record, err = service.Reconcile(context.Background())
	if err != nil || record.State != RecoveryActive || len(record.PendingAudits) != 0 || len(driver.activated) != 1 {
		t.Fatalf("activation audit retry repeated mutation: record=%+v err=%v activations=%v", record, err, driver.activated)
	}
	journal.failAction = "network.recovery.restored"
	now = now.Add(2 * time.Minute)
	record, err = service.Reconcile(context.Background())
	if err == nil || record.State != RecoveryRestoring || !slices.Contains(record.PendingAudits, pendingRestoredAudit) || len(driver.restored) != 1 {
		t.Fatalf("restored audit was not left durable: record=%+v err=%v restored=%v", record, err, driver.restored)
	}
	journal.failAction = ""
	record, err = service.Reconcile(context.Background())
	if err != nil || record.State != RecoveryIdle || len(driver.restored) != 1 {
		t.Fatalf("restored audit retry repeated mutation: record=%+v err=%v restored=%v", record, err, driver.restored)
	}
}

func TestPendingActivationAuditCannotBlockExpiryCleanup(t *testing.T) {
	now := time.Date(2026, 9, 14, 14, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{}
	store := &memoryRecoveryStore{}
	journal := &switchableRecoveryAudit{failAction: "network.recovery.activated"}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Store: store, Now: func() time.Time { return now },
		Window: time.Minute, Audit: journal, SubjectID: "appliance",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	if _, err = service.Reconcile(context.Background()); err == nil {
		t.Fatal("injected activation audit failure was ignored")
	}
	now = now.Add(2 * time.Minute)
	record, err := service.Reconcile(context.Background())
	if err == nil || len(driver.restored) != 1 || record.State != RecoveryRestoring {
		t.Fatalf("pending audit blocked physical expiry cleanup: record=%+v err=%v restored=%v", record, err, driver.restored)
	}
	journal.failAction = ""
	record, err = service.Reconcile(context.Background())
	if err != nil || record.State != RecoveryIdle || len(driver.restored) != 1 {
		t.Fatalf("pending audits did not settle without repeating cleanup: record=%+v err=%v restored=%v", record, err, driver.restored)
	}
}

func TestRecoveryQueuesExpiryAndRestoreRequestAuditsWithoutRepeatingCleanup(t *testing.T) {
	for _, failedAction := range []string{"network.recovery.expired", "network.recovery.restore_requested"} {
		t.Run(failedAction, func(t *testing.T) {
			now := time.Date(2026, 9, 14, 16, 0, 0, 0, time.UTC)
			provider := &recoveryProvider{interfaces: []Interface{{
				Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
			}}}
			driver := &recoveryDriver{}
			journal := &switchableRecoveryAudit{}
			service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
				Provider: provider, Driver: driver, Store: &memoryRecoveryStore{}, Now: func() time.Time { return now },
				Window: time.Minute, Audit: journal, SubjectID: "appliance",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.Arm(context.Background()); err != nil {
				t.Fatal(err)
			}
			provider.interfaces[0].State = StateConnected
			if _, err = service.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			journal.failAction = failedAction
			now = now.Add(2 * time.Minute)
			record, reconcileErr := service.Reconcile(context.Background())
			if reconcileErr == nil || record.State != RecoveryRestoring || len(driver.restored) != 1 || len(record.PendingAudits) < 2 {
				t.Fatalf("required audit was not queued through cleanup: record=%+v err=%v restored=%v", record, reconcileErr, driver.restored)
			}
			journal.failAction = ""
			record, reconcileErr = service.Reconcile(context.Background())
			if reconcileErr != nil || record.State != RecoveryIdle || len(driver.restored) != 1 {
				t.Fatalf("audit retry repeated cleanup: record=%+v err=%v restored=%v", record, reconcileErr, driver.restored)
			}
			if !slices.ContainsFunc(journal.records, func(record audit.Record) bool { return record.Action == failedAction }) {
				t.Fatalf("retried audit %q was not recorded: %+v", failedAction, journal.records)
			}
		})
	}
}

func TestRecoveryQueuesFailureAuditsUntilTheyCanBeRecorded(t *testing.T) {
	now := time.Date(2026, 9, 14, 17, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{activateErr: errors.New("partial activation"), failedID: "partial-record"}
	journal := &switchableRecoveryAudit{failAction: "network.recovery.failed"}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Store: &memoryRecoveryStore{}, Now: func() time.Time { return now },
		Window: time.Minute, Audit: journal, SubjectID: "appliance",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	record, reconcileErr := service.Reconcile(context.Background())
	if reconcileErr == nil || record.State != RecoveryFailed || len(record.PendingAudits) == 0 {
		t.Fatalf("activation failure audit was not queued: record=%+v err=%v", record, reconcileErr)
	}
	driver.activateErr = nil
	journal.failAction = ""
	record, reconcileErr = service.Reconcile(context.Background())
	if reconcileErr != nil || record.State != RecoveryIdle || !slices.Equal(driver.restored, []string{"partial-record"}) {
		t.Fatalf("failure audit retry did not settle exact cleanup: record=%+v err=%v restored=%v", record, reconcileErr, driver.restored)
	}
	if !slices.ContainsFunc(journal.records, func(record audit.Record) bool {
		return record.Action == "network.recovery.failed" && recoveryAuditReason(record) == "helper_activation_failed"
	}) {
		t.Fatalf("activation failure audit was lost: %+v", journal.records)
	}
}

func TestRecoveryQueuesExtensionAndRestorationFailureAudits(t *testing.T) {
	now := time.Date(2026, 9, 14, 17, 30, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{}
	journal := &switchableRecoveryAudit{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Store: &memoryRecoveryStore{}, Now: func() time.Time { return now },
		Window: time.Minute, Audit: journal, SubjectID: "appliance",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}

	driver.extendErr = errors.New("extension failed")
	journal.failAction = "network.recovery.failed"
	record, operationErr := service.Arm(context.Background())
	if operationErr == nil || record.State != RecoveryActive || len(record.PendingAudits) == 0 {
		t.Fatalf("extension failure audit was not queued: record=%+v err=%v", record, operationErr)
	}
	driver.extendErr = nil
	journal.failAction = ""
	if record, operationErr = service.Reconcile(context.Background()); operationErr != nil || len(record.PendingAudits) != 0 {
		t.Fatalf("extension failure audit was not retried: record=%+v err=%v", record, operationErr)
	}

	driver.restoreErr = errors.New("restoration failed")
	journal.failAction = "network.recovery.failed"
	now = now.Add(2 * time.Minute)
	record, operationErr = service.Reconcile(context.Background())
	if operationErr == nil || record.State != RecoveryFailed || len(record.PendingAudits) == 0 {
		t.Fatalf("restoration failure audit was not queued: record=%+v err=%v", record, operationErr)
	}
	driver.restoreErr = nil
	journal.failAction = ""
	record, operationErr = service.Reconcile(context.Background())
	if operationErr != nil || record.State != RecoveryIdle || len(driver.restored) != 2 {
		t.Fatalf("restoration failure audit retry did not settle: record=%+v err=%v restored=%v", record, operationErr, driver.restored)
	}
	for _, reason := range []string{"helper_extension_failed", "helper_restoration_failed"} {
		if !slices.ContainsFunc(journal.records, func(record audit.Record) bool {
			return record.Action == "network.recovery.failed" && recoveryAuditReason(record) == reason
		}) {
			t.Fatalf("failure audit %q was lost: %+v", reason, journal.records)
		}
	}
}

func TestRecoveryExpiresByElapsedTimeAfterWallClockRollback(t *testing.T) {
	wall := time.Date(2026, 9, 14, 18, 0, 0, 0, time.UTC)
	steady := wall
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Now: func() time.Time { return wall },
		SteadyNow: func() time.Time { return steady }, Window: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	steady = steady.Add(2 * time.Minute)
	wall = wall.Add(-time.Hour)
	record, err := service.Reconcile(context.Background())
	if err != nil || record.State != RecoveryIdle || !slices.Equal(driver.restored, []string{"recovery-link-1"}) {
		t.Fatalf("elapsed deadline did not restore through wall rollback: record=%+v err=%v restored=%v", record, err, driver.restored)
	}
}

func TestRecoveryRestartFailsClosedAfterTheLastPersistedClockObservation(t *testing.T) {
	wall := time.Date(2026, 9, 14, 18, 30, 0, 0, time.UTC)
	steady := wall
	store := &memoryRecoveryStore{}
	provider := &recoveryProvider{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: &recoveryDriver{}, Store: store,
		Now: func() time.Time { return wall }, SteadyNow: func() time.Time { return steady },
		Window: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	wall = wall.Add(time.Minute)
	steady = steady.Add(time.Minute)
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.record.UpdatedAt != wall {
		t.Fatalf("last observed wall time was not persisted: record=%+v", store.record)
	}

	wall = wall.Add(-30 * time.Second)
	steady = steady.Add(time.Hour)
	restarted, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: &recoveryDriver{}, Store: store,
		Now: func() time.Time { return wall }, SteadyNow: func() time.Time { return steady },
		Window: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := restarted.Reconcile(context.Background())
	if err != nil || record.State != RecoveryIdle {
		t.Fatalf("restart did not fail closed behind the last wall observation: record=%+v err=%v", record, err)
	}
}

func TestRecoveryRestoreFailureRetainsInterlock(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{restoreErr: errors.New("profile still active")}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Now: func() time.Time { return now }, Window: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	record, err := service.Reconcile(context.Background())
	if err == nil || record.State != RecoveryFailed || record.HelperRecordID == "" {
		t.Fatalf("record=%+v err=%v, want retained failed interlock", record, err)
	}
}

func TestRecoveryCancelRestoresActiveLinkBeforeCommissioningReset(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "a", Kind: KindEthernet, Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	record, err := service.Cancel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if record.State != RecoveryIdle || !slices.Equal(driver.restored, []string{"recovery-link-1"}) {
		t.Fatalf("record=%+v restored=%v", record, driver.restored)
	}
}

func TestRecoveryAuditsBoundedLifecycleAndRefusal(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "aa:bb:cc:dd:ee:ff", Kind: KindEthernet,
		Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{}
	journal := &recoveryAudit{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Audit: journal, SubjectID: "appliance-1",
		Now: func() time.Time { return now }, Window: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	provider.interfaces[0].Settings = &SettingsState{Saved: true, Method: MethodStatic}
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces = append(provider.interfaces, Interface{
		Name: "eth2", AdapterID: "bb:cc:dd:ee:ff:00", Kind: KindEthernet,
		Physical: true, State: StateConnected,
	})
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err = service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	actions := make([]string, 0, len(journal.records))
	for _, record := range journal.records {
		if record.ActorKind != audit.ActorKindSystem || record.SubjectID != "appliance-1" {
			t.Fatalf("unattributed audit record: %+v", record)
		}
		actions = append(actions, record.Action)
	}
	want := []string{
		"network.recovery.armed", "network.recovery.refused", "network.recovery.activation_requested",
		"network.recovery.activated", "network.recovery.extended", "network.recovery.expired",
		"network.recovery.restore_requested", "network.recovery.restored",
	}
	if !slices.Equal(actions, want) {
		t.Fatalf("audit actions = %v, want %v", actions, want)
	}
}

func TestRecoveryFailureAuditAndReturnedErrorOmitHelperDetail(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	provider := &recoveryProvider{interfaces: []Interface{{
		Name: "eth1", AdapterID: "aa:bb:cc:dd:ee:ff", Kind: KindEthernet,
		Physical: true, State: StateDisconnected,
	}}}
	driver := &recoveryDriver{activateErr: errors.New("sentinel-password-token-private-key")}
	journal := &recoveryAudit{}
	service, err := NewHardwareRecoveryService(HardwareRecoveryOptions{
		Provider: provider, Driver: driver, Audit: journal, SubjectID: "appliance-1", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	_, err = service.Reconcile(context.Background())
	serialized, marshalErr := json.Marshal(journal.records)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if err == nil || strings.Contains(err.Error(), "sentinel") || strings.Contains(string(serialized), "sentinel") {
		t.Fatalf("err=%v audit=%s", err, serialized)
	}
}
