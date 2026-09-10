package nethelper

import (
	"context"
	"errors"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

type fakeDriver struct {
	device     Device
	events     []string
	restoreErr error
	tunnels    fakeTunnels
	settings   fakeSettings
	forwarding fakeForwarding
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

type memoryStore struct {
	record     *Record
	fail       bool
	forwarding *ForwardingInventory
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
