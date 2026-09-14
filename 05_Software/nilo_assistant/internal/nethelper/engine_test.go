package nethelper

import (
	"context"
	"errors"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

type fakeDriver struct {
	device      Device
	events      []string
	restoreErr  error
	tunnels     fakeTunnels
	settings    fakeSettings
	forwarding  fakeForwarding
	recovery    []string
	recoveryErr error
}

func (d *fakeDriver) Devices(context.Context) ([]Device, error) { return []Device{d.device}, nil }
func (d *fakeDriver) Checkpoint(_ context.Context, target Device, _ time.Duration) (string, error) {
	// An empty target is the whole system, which is what a tunnel change
	// checkpoints because routes and DNS change everywhere.
	if target.Interface.ID == "" {
		d.events = append(d.events, "checkpoint-all")
	} else {
		d.events = append(d.events, "checkpoint")
	}
	return "/checkpoint/1", nil
}
func (d *fakeDriver) Apply(_ context.Context, c network.Change, _ Device) error {
	d.events = append(d.events, "apply")
	d.device.Interface.State = network.StateDisconnected
	return nil
}
func (d *fakeDriver) Restore(_ context.Context, r Record) error {
	d.events = append(d.events, "restore")
	if d.restoreErr != nil {
		return d.restoreErr
	}
	if r.System != nil {
		d.tunnels.active = r.System.ActiveTunnel
		return nil
	}
	d.device = r.Before
	return nil
}
func (d *fakeDriver) Destroy(context.Context, string) error {
	d.events = append(d.events, "destroy")
	return nil
}

func (d *fakeDriver) ActivateRecoveryLink(_ context.Context, _ string, adapterID, interfaceName string) error {
	d.recovery = append(d.recovery, "activate:"+adapterID+":"+interfaceName)
	return d.recoveryErr
}

func (d *fakeDriver) RestoreRecoveryLink(_ context.Context, record DirectRecoveryRecord) error {
	d.recovery = append(d.recovery, "restore:"+record.ID)
	return nil
}

type memoryStore struct {
	record         *Record
	directRecovery *DirectRecoveryRecord
	fail           bool
	forwarding     *ForwardingInventory
}

func (s *memoryStore) LoadDirectRecovery() (*DirectRecoveryRecord, error) {
	if s.directRecovery == nil {
		return nil, nil
	}
	record := *s.directRecovery
	return &record, nil
}

func (s *memoryStore) SaveDirectRecovery(record DirectRecoveryRecord) error {
	if s.fail {
		return errors.New("disk full")
	}
	s.directRecovery = &record
	return nil
}

func (s *memoryStore) ClearDirectRecovery() error {
	s.directRecovery = nil
	return nil
}

func (s *memoryStore) Load() (*Record, error) { return s.record, nil }
func (s *memoryStore) Save(r Record) error {
	if s.fail {
		return errors.New("disk full")
	}
	s.record = &r
	return nil
}
func fixture(t *testing.T) (*Engine, *fakeDriver, *memoryStore, *time.Time) {
	t.Helper()
	now := time.Now()
	d := &fakeDriver{device: Device{Interface: network.Interface{ID: "eth-a", Name: "eth0", Kind: network.KindEthernet, State: network.StateConnected, Controllable: true}, Connection: "connection-a", AutoConnect: true}}
	s := &memoryStore{}
	e, err := NewEngine(d, s, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return e, d, s, &now
}
func TestRecoveryMustBeDurableBeforeMutation(t *testing.T) {
	e, d, s, _ := fixture(t)
	s.fail = true
	if _, err := e.Prepare(context.Background(), "one", network.Change{Action: network.ActionInterfaceDisable, Target: "eth-a"}, network.ConfirmationWindow); err == nil {
		t.Fatal("accepted lost recovery")
	}
	if len(d.events) != 0 {
		t.Fatal(d.events)
	}
}
func TestHelperBindsOneChangeAndRejectsReplay(t *testing.T) {
	e, d, _, _ := fixture(t)
	ctx := context.Background()
	if _, err := e.Prepare(ctx, "one", network.Change{Action: network.ActionInterfaceDisable, Target: "eth-a"}, network.ConfirmationWindow); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(ctx, "different"); err == nil {
		t.Fatal("accepted different operation")
	}
	if err := e.Apply(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(ctx, "one"); err == nil {
		t.Fatal("replayed")
	}
	if _, err := e.Prepare(ctx, "two", network.Change{Action: network.ActionInterfaceEnable, Target: "eth-a"}, network.ConfirmationWindow); err == nil {
		t.Fatal("concurrent change")
	}
	if len(d.events) != 2 {
		t.Fatal(d.events)
	}
}

func TestDirectRecoveryProtocolCarriesOnlyTypedAdapterIdentity(t *testing.T) {
	e, driver, _, now := fixture(t)
	response := dispatch(context.Background(), e, Request{
		Op: "recovery.link.activate", AdapterID: "aa:bb:cc:dd:ee:ff", InterfaceName: "eth1",
		RecoveryExpiresAt: now.Add(10 * time.Minute),
	})
	if response.Error != "" || response.RecoveryRecordID == "" {
		t.Fatalf("activate response = %+v", response)
	}
	if got := driver.recovery; len(got) != 1 || got[0] != "activate:aa:bb:cc:dd:ee:ff:eth1" {
		t.Fatalf("driver calls = %v", got)
	}
	if refused := dispatch(context.Background(), e, Request{
		Op: "interface.list", AdapterID: "aa:bb:cc:dd:ee:ff",
	}); refused.Code != "invalid_request" {
		t.Fatalf("unrelated operation accepted adapter identity: %+v", refused)
	}
	if restored := dispatch(context.Background(), e, Request{
		Op: "recovery.link.restore", RecoveryRecordID: response.RecoveryRecordID,
	}); restored.Error != "" {
		t.Fatalf("restore response = %+v", restored)
	}
}

type fakeRecoveryHardware struct {
	event     HardwareRecoveryEvent
	indicator HardwareIndicatorState
	acked     string
}

func (hardware *fakeRecoveryHardware) ReadRecoveryEvent(context.Context) (HardwareRecoveryEvent, bool, error) {
	return hardware.event, hardware.event.ID != "" && hardware.acked != hardware.event.ID, nil
}

func (hardware *fakeRecoveryHardware) AcknowledgeRecoveryEvent(_ context.Context, eventID string) error {
	hardware.acked = eventID
	return nil
}

func (hardware *fakeRecoveryHardware) SetRecoveryIndicator(_ context.Context, state HardwareIndicatorState) error {
	hardware.indicator = state
	return nil
}

func TestHardwareRecoveryProtocolReadsAcknowledgesAndIndicates(t *testing.T) {
	e, _, _, _ := fixture(t)
	hardware := &fakeRecoveryHardware{event: HardwareRecoveryEvent{ID: "event-1", Action: HardwareRecoveryArm}}
	if err := e.ConfigureRecoveryHardware(hardware); err != nil {
		t.Fatal(err)
	}
	read := dispatch(context.Background(), e, Request{Op: "hardware.recovery.read"})
	if read.Error != "" || read.HardwareEvent == nil || read.HardwareEvent.ID != "event-1" {
		t.Fatalf("read response = %+v", read)
	}
	if ack := dispatch(context.Background(), e, Request{Op: "hardware.recovery.ack", HardwareEventID: "event-1"}); ack.Error != "" {
		t.Fatalf("ack response = %+v", ack)
	}
	state := HardwareIndicatorArmed
	if indicated := dispatch(context.Background(), e, Request{Op: "hardware.indicator.set", Indicator: &state}); indicated.Error != "" {
		t.Fatalf("indicator response = %+v", indicated)
	}
	if hardware.acked != "event-1" || hardware.indicator != HardwareIndicatorArmed {
		t.Fatalf("hardware state = %+v", hardware)
	}
	if refused := dispatch(context.Background(), e, Request{Op: "interface.list", HardwareEventID: "event-1"}); refused.Code != "invalid_request" {
		t.Fatalf("unrelated operation accepted hardware event: %+v", refused)
	}
}

func TestHardwareRecoveryIsUnavailableUntilExplicitlyConfigured(t *testing.T) {
	engine, _, _, _ := fixture(t)
	response := dispatch(context.Background(), engine, Request{Op: "hardware.recovery.read"})
	if response.Code != "direct_recovery_unavailable" {
		t.Fatalf("unconfigured hardware response = %+v", response)
	}
	if err := engine.ConfigureRecoveryHardware(nil); err == nil {
		t.Fatal("partial hardware configuration was accepted")
	}
}
func TestExpiryAndStartupVerifyRestoration(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "restart"}[restart], func(t *testing.T) {
			e, d, s, now := fixture(t)
			ctx := context.Background()
			_, err := e.Prepare(ctx, "one", network.Change{Action: network.ActionInterfaceDisable, Target: "eth-a"}, network.ConfirmationWindow)
			if err != nil {
				t.Fatal(err)
			}
			if err = e.Apply(ctx, "one"); err != nil {
				t.Fatal(err)
			}
			if restart {
				e, err = NewEngine(d, s, func() time.Time { return *now })
				if err != nil {
					t.Fatal(err)
				}
				err = e.Reconcile(ctx)
			} else {
				*now = now.Add(network.ConfirmationWindow)
				err = e.Expire(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			r, _ := e.Recovery()
			if r.Phase != "restored" || d.device.Interface.State != network.StateConnected {
				t.Fatalf("not restored: %+v", r)
			}
		})
	}
}
func TestFailedRecoveryBlocksNewChanges(t *testing.T) {
	e, d, _, _ := fixture(t)
	ctx := context.Background()
	_, _ = e.Prepare(ctx, "one", network.Change{Action: network.ActionInterfaceDisable, Target: "eth-a"}, network.ConfirmationWindow)
	_ = e.Apply(ctx, "one")
	d.restoreErr = errors.New("no route")
	if err := e.Restore(ctx, "one"); err == nil {
		t.Fatal("claimed restoration")
	}
	r, _ := e.Recovery()
	if r.Phase != "recovery_failed" {
		t.Fatal(r.Phase)
	}
	if _, err := e.Prepare(ctx, "two", network.Change{Action: network.ActionInterfaceEnable, Target: "eth-a"}, network.ConfirmationWindow); err == nil {
		t.Fatal("unblocked")
	}
}

func TestHelperTreatsShellFragmentsAsIdentifiersAndRejectsUnknownOperations(t *testing.T) {
	e, d, _, _ := fixture(t)
	d.device.Interface.ID = "eth0; touch unexpected"
	response := dispatch(context.Background(), e, Request{Op: "change.inspect", Change: network.Change{Action: network.ActionInterfaceDisable, Target: d.device.Interface.ID}})
	if response.Error != "" || response.Device == nil || response.Device.Interface.ID != d.device.Interface.ID {
		t.Fatalf("identifier was interpreted: %+v", response)
	}
	if len(d.events) != 0 {
		t.Fatal("inspection mutated network")
	}
	response = dispatch(context.Background(), e, Request{Op: "shell.run"})
	if response.Error == "" || len(d.events) != 0 {
		t.Fatal("unknown operation was executed")
	}
	for _, kind := range []network.Kind{network.KindLoopback, network.KindWireGuard, network.KindOther} {
		d.device.Interface.Kind = kind
		for _, action := range []network.Action{network.ActionInterfaceEnable, network.ActionInterfaceDisable} {
			_, _, err := e.Inspect(context.Background(), network.Change{Action: action, Target: d.device.Interface.ID})
			if !errors.Is(err, network.ErrNotControllable) {
				t.Fatalf("managed infrastructure: %v", err)
			}
		}
	}
	d.device.Interface.Kind = network.KindEthernet
	d.device.Interface.Controllable = false
	if _, _, err := e.Inspect(context.Background(), network.Change{Action: network.ActionInterfaceDisable, Target: d.device.Interface.ID}); !errors.Is(err, network.ErrNotControllable) {
		t.Fatal("managed an unmanaged interface")
	}
}

func TestHelperRefusesRecoverySnapshotDuringIPActivation(t *testing.T) {
	e, d, _, _ := fixture(t)
	d.device.Transitioning = true
	_, err := e.Prepare(context.Background(), "one", network.Change{Action: network.ActionInterfaceDisable, Target: d.device.Interface.ID}, network.ConfirmationWindow)
	if !errors.Is(err, network.ErrPreviewStale) || len(d.events) != 0 {
		t.Fatalf("captured partial IP state: %v %v", err, d.events)
	}
}

// An advertisement is accepted only by the operation that publishes one, and
// that operation accepts nothing else. The request type carries no field able
// to hold responder configuration text, so the unprivileged process can ask for
// a name on named interfaces and nothing more.
func TestHelperAcceptsAnAdvertisementOnlyForLocalNamePublication(t *testing.T) {
	e, d, _, _ := fixture(t)
	wanted := network.MDNSAdvertisement{HostName: "ddp-remote", Interfaces: []string{"enp1s0"}}

	for _, op := range []string{"interface.list", "management.access.apply", "forwarding.inventory"} {
		response := dispatch(context.Background(), e, Request{Op: op, MDNS: &wanted})
		if response.Error == "" {
			t.Fatalf("operation %q accepted an mDNS advertisement", op)
		}
		if response.Code != "invalid_request" {
			t.Fatalf("operation %q rejected the advertisement as %q, want invalid_request", op, response.Code)
		}
	}

	response := dispatch(context.Background(), e, Request{Op: "mdns.publish"})
	if response.Error == "" || response.Code != "invalid_request" {
		t.Fatalf("publication without an advertisement was accepted: %+v", response)
	}
	response = dispatch(context.Background(), e, Request{Op: "mdns.publish", ID: "op-1", MDNS: &wanted})
	if response.Error == "" || response.Code != "invalid_request" {
		t.Fatalf("publication carrying an operation identity was accepted: %+v", response)
	}
	if len(d.events) != 0 {
		t.Fatal("a refused advertisement reached the network")
	}
}
