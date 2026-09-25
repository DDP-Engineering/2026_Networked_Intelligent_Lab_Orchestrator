package nethelper

import (
	"context"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// TestRecoverOncePutsAnInterruptedChangeBack covers REQ-REL-8fa3d260 for the
// independent timer: with the helper gone, a change left applied is restored
// from the saved record, an owned table that vanished is reinstalled from the
// inventory, and a pass with nothing to do changes nothing.
func TestRecoverOncePutsAnInterruptedChangeBack(t *testing.T) {
	ctx := context.Background()
	e, d, s, now := fixture(t)
	// Forwarding installed earlier, then an adapter change left applied.
	if err := e.ApplyForwarding(ctx, linePlan(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Prepare(ctx, "one", network.Change{Action: network.ActionInterfaceDisable, Target: "eth-a"}, network.ConfirmationWindow); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	// The helper dies; its table is lost with a reboot.
	d.forwarding.table = TableAbsent
	renders := d.forwarding.renders

	outcome, err := RecoverOnce(ctx, d, s, func() time.Time { return *now })
	if err != nil {
		t.Fatalf("recover: %v (%+v)", err, outcome)
	}
	if outcome.Restored != "one" || outcome.Phase != "restored" || !outcome.ForwardingVerified {
		t.Fatalf("outcome = %+v", outcome)
	}
	if d.device.Interface.State != network.StateConnected {
		t.Fatal("the interface was not put back")
	}
	if d.forwarding.table != TableOwned || d.forwarding.renders != renders+1 {
		t.Fatalf("forwarding not reinstalled: table=%s renders=%d", d.forwarding.table, d.forwarding.renders)
	}

	// Nothing pending: a pass leaves everything as it is.
	again, err := RecoverOnce(ctx, d, s, func() time.Time { return *now })
	if err != nil || again.Restored != "" || again.Phase != "restored" || !again.ForwardingVerified {
		t.Fatalf("quiet pass = %+v %v", again, err)
	}
	if d.forwarding.renders != renders+1 {
		t.Fatal("a quiet pass rendered again")
	}

	// A foreign table is reported, never replaced.
	d.forwarding.table = TableForeign
	foreign, err := RecoverOnce(ctx, d, s, func() time.Time { return *now })
	if err == nil || foreign.ForwardingVerified {
		t.Fatalf("foreign table: %+v %v", foreign, err)
	}
}
