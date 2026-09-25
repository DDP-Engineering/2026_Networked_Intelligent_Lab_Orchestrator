package nethelper

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// fakeForwarding is the forwarding half of the fake driver: the owned table's
// state, what was last rendered into it and the forwarding switch.
type fakeForwarding struct {
	table       TableState
	installed   *ForwardingInventory
	switchValue string
	renders     int
	installErr  error
}

func (d *fakeDriver) ForwardingTable(context.Context) (TableState, error) {
	if d.forwarding.table == "" {
		return TableAbsent, nil
	}
	return d.forwarding.table, nil
}

func (d *fakeDriver) InstallForwarding(_ context.Context, inventory ForwardingInventory) error {
	if d.forwarding.installErr != nil {
		return d.forwarding.installErr
	}
	d.forwarding.renders++
	copied := inventory
	d.forwarding.installed = &copied
	if len(inventory.Plans) == 0 {
		d.forwarding.table = TableAbsent
	} else {
		d.forwarding.table = TableOwned
	}
	return nil
}

func (d *fakeDriver) ForwardingSwitch(context.Context) (string, error) {
	if d.forwarding.switchValue == "" {
		return "0", nil
	}
	return d.forwarding.switchValue, nil
}

func (d *fakeDriver) SetForwardingSwitch(_ context.Context, value string) error {
	d.forwarding.switchValue = value
	return nil
}

func (s *memoryStore) LoadForwarding() (*ForwardingInventory, error) { return s.forwarding, nil }
func (s *memoryStore) SaveForwarding(inventory ForwardingInventory) error {
	if s.fail {
		return errors.New("disk full")
	}
	s.forwarding = &inventory
	return nil
}

func linePlan(t *testing.T) network.ForwardingPlan {
	t.Helper()
	plan, err := network.PlanForwarding(network.DeviceNetwork{
		ID: "line3", Name: "Line 3", DeviceInterface: "02:00:00:00:00:02",
		ApplianceAddress: netip.MustParsePrefix("192.168.0.1/24"), Scope: network.ScopeHost,
		Target: netip.MustParseAddr("192.168.0.130"), Addressing: network.AddressingOriginal,
		Access: network.AccessLocal, LocalInterface: "02:00:00:00:00:01",
		LocalSources: []netip.Prefix{netip.MustParsePrefix("192.168.10.0/24")},
		Services:     network.ServicePolicy{TCPPorts: []int{502}},
	}, network.ForwardingContext{DeviceLink: "dev0", LocalLink: "lan0"})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

// TestForwardingApplyRemoveInventory covers REQ-REL-b6d0e358 and
// REQ-DATA-7b62c9f3 in the helper: an applied plan is rendered into the owned
// table and recorded durably, the forwarding switch is taken over and given
// back, and a removal leaves nothing owned.
func TestForwardingApplyRemoveInventory(t *testing.T) {
	e, d, s, _ := fixture(t)
	ctx := context.Background()
	plan := linePlan(t)
	if err := e.ApplyForwarding(ctx, plan); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if d.forwarding.renders != 1 || d.forwarding.table != TableOwned || d.forwarding.switchValue != "1" {
		t.Fatalf("render=%d table=%s switch=%s", d.forwarding.renders, d.forwarding.table, d.forwarding.switchValue)
	}
	if s.forwarding == nil || len(s.forwarding.Plans) != 1 || !s.forwarding.Switch.Owned || s.forwarding.Switch.Previous != "0" {
		t.Fatalf("durable inventory = %+v", s.forwarding)
	}
	applied, err := e.AppliedForwarding(ctx)
	if err != nil || len(applied) != 1 || applied[0].DeviceNetworkID != "line3" {
		t.Fatalf("applied = %+v %v", applied, err)
	}
	// Applying the same plan again changes nothing.
	if err := e.ApplyForwarding(ctx, plan); err != nil || d.forwarding.renders != 1 {
		t.Fatalf("idempotent apply: renders=%d %v", d.forwarding.renders, err)
	}
	if err := e.RemoveForwarding(ctx, "line3"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if d.forwarding.table != TableAbsent || d.forwarding.switchValue != "0" || len(s.forwarding.Plans) != 0 || s.forwarding.Switch.Owned {
		t.Fatalf("after removal: table=%s switch=%s inventory=%+v", d.forwarding.table, d.forwarding.switchValue, s.forwarding)
	}
	if err := e.RemoveForwarding(ctx, "line3"); err != nil {
		t.Fatalf("removing what is absent: %v", err)
	}
}

// TestForwardingLeavesAnOperatorsSwitchAlone: a switch that was already on is
// not NILO's to turn off.
func TestForwardingLeavesAnOperatorsSwitchAlone(t *testing.T) {
	e, d, s, _ := fixture(t)
	d.forwarding.switchValue = "1"
	ctx := context.Background()
	if err := e.ApplyForwarding(ctx, linePlan(t)); err != nil {
		t.Fatal(err)
	}
	if s.forwarding.Switch.Owned {
		t.Fatal("took over a switch that was already on")
	}
	if err := e.RemoveForwarding(ctx, "line3"); err != nil {
		t.Fatal(err)
	}
	if d.forwarding.switchValue != "1" {
		t.Fatal("turned off the operator's forwarding")
	}
}

// TestForwardingRefusesForeignTable: a table NILO did not create is never
// replaced.
func TestForwardingRefusesForeignTable(t *testing.T) {
	e, d, _, _ := fixture(t)
	d.forwarding.table = TableForeign
	if err := e.ApplyForwarding(context.Background(), linePlan(t)); !errors.Is(err, ErrForwardingForeign) {
		t.Fatalf("err = %v", err)
	}
	if d.forwarding.renders != 0 || d.forwarding.switchValue != "" {
		t.Fatal("touched the host despite the foreign table")
	}
}

// TestForwardingChangeIsRestoredByTimeout covers REQ-REL-b6d0e358: a
// device-network change is recovered from the helper's own snapshot when
// nothing confirms it, independently of the core.
func TestForwardingChangeIsRestoredByTimeout(t *testing.T) {
	e, d, s, now := fixture(t)
	ctx := context.Background()
	record, err := e.Prepare(ctx, "enable", network.Change{Action: network.ActionDeviceNetworkEnable, Target: "line3"}, network.ConfirmationWindow)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if record.Forwarding == nil || len(record.Forwarding.Plans) != 0 || record.Checkpoint != "" {
		t.Fatalf("record = %+v", record)
	}
	if len(d.events) != 0 {
		t.Fatalf("a forwarding change needs no NetworkManager checkpoint: %v", d.events)
	}
	if err := e.Apply(ctx, "enable"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := e.ApplyForwarding(ctx, linePlan(t)); err != nil {
		t.Fatalf("apply forwarding: %v", err)
	}
	if d.forwarding.table != TableOwned {
		t.Fatal("not installed")
	}
	*now = now.Add(network.ConfirmationWindow + time.Second)
	if err := e.Expire(ctx); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if d.forwarding.table != TableAbsent || d.forwarding.switchValue != "0" || len(s.forwarding.Plans) != 0 {
		t.Fatalf("not restored: table=%s switch=%s inventory=%+v", d.forwarding.table, d.forwarding.switchValue, s.forwarding)
	}
	if got, _ := e.Recovery(); got.Phase != "restored" {
		t.Fatalf("phase = %s", got.Phase)
	}
	// A confirmed change keeps what was applied.
	if _, err := e.Prepare(ctx, "again", network.Change{Action: network.ActionDeviceNetworkEnable, Target: "line3"}, network.ConfirmationWindow); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(ctx, "again"); err != nil {
		t.Fatal(err)
	}
	if err := e.ApplyForwarding(ctx, linePlan(t)); err != nil {
		t.Fatal(err)
	}
	if err := e.Confirm(ctx, "again"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if d.forwarding.table != TableOwned {
		t.Fatal("confirmation lost the forwarding")
	}
}

// TestForwardingOpsRefusedDuringOtherChange: while an adapter change is in
// flight, no forwarding is touched.
func TestForwardingOpsRefusedDuringOtherChange(t *testing.T) {
	e, _, _, _ := fixture(t)
	ctx := context.Background()
	if _, err := e.Prepare(ctx, "one", network.Change{Action: network.ActionInterfaceDisable, Target: "eth-a"}, network.ConfirmationWindow); err != nil {
		t.Fatal(err)
	}
	if err := e.ApplyForwarding(ctx, linePlan(t)); !errors.Is(err, network.ErrChangeInProgress) {
		t.Fatalf("err = %v", err)
	}
	if err := e.RemoveForwarding(ctx, "line3"); !errors.Is(err, network.ErrChangeInProgress) {
		t.Fatalf("err = %v", err)
	}
}

// TestForwardingIntegrityReinstalls: an owned table that vanished underneath
// the helper is put back by the helper's own timer, because a device network
// whose guards are gone is exposed, not closed.
func TestForwardingIntegrityReinstalls(t *testing.T) {
	e, d, _, _ := fixture(t)
	ctx := context.Background()
	if err := e.ApplyForwarding(ctx, linePlan(t)); err != nil {
		t.Fatal(err)
	}
	d.forwarding.table = TableAbsent
	if err := e.VerifyForwarding(ctx); err != nil {
		t.Fatal(err)
	}
	if d.forwarding.renders != 2 || d.forwarding.table != TableOwned {
		t.Fatalf("renders=%d table=%s", d.forwarding.renders, d.forwarding.table)
	}
	// A foreign table that appeared in its place is reported, not replaced.
	d.forwarding.table = TableForeign
	if err := e.VerifyForwarding(ctx); !errors.Is(err, ErrForwardingForeign) {
		t.Fatalf("err = %v", err)
	}
}

// TestForwardingSurvivesRestart: the durable inventory is what a restarted
// helper renders from.
func TestForwardingSurvivesRestart(t *testing.T) {
	e, d, s, now := fixture(t)
	ctx := context.Background()
	if err := e.ApplyForwarding(ctx, linePlan(t)); err != nil {
		t.Fatal(err)
	}
	d.forwarding.table = TableAbsent
	restarted, err := NewEngine(d, s, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	applied, err := restarted.AppliedForwarding(ctx)
	if err != nil || len(applied) != 1 {
		t.Fatalf("applied after restart = %+v %v", applied, err)
	}
	if err := restarted.VerifyForwarding(ctx); err != nil || d.forwarding.table != TableOwned {
		t.Fatalf("not reinstalled after restart: %v %s", err, d.forwarding.table)
	}
}
