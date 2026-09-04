package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/network"
)

// TestAnInterfaceToggleKeepsAttributableHistoryAcrossARestart is the P1 exit
// criterion: an existing interface change is recorded durably, attributed to
// the administrator who asked for it, and still observable after the process
// that made it is gone.
//
// It composes the real application, the real SQLite database and the real
// operation lifecycle, because the point is the wiring rather than any one
// piece of it.
func TestAnInterfaceToggleKeepsAttributableHistoryAcrossARestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	actor := uuid.NewString()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	first := openApplication(t, ctx, dataDir, clock)
	firstIdentity := first.ApplianceRuntime()
	if firstIdentity.Identity.ID == "" || firstIdentity.BootID == "" {
		t.Fatalf("appliance runtime = %+v, want an appliance and a boot identity", firstIdentity)
	}

	simulated := network.NewSimulation(clock)
	operations := newOperations(t, first, simulated, clock)

	interfaces, err := simulated.Interfaces(ctx)
	if err != nil {
		t.Fatalf("read simulated interfaces: %v", err)
	}
	var target string
	for _, candidate := range interfaces {
		if candidate.Controllable {
			target = candidate.ID
			break
		}
	}
	if target == "" {
		t.Fatal("the simulation offers no controllable interface to toggle")
	}

	previewed, err := operations.Preview(ctx, actor, network.ActionInterfaceDisable, target)
	if err != nil {
		t.Fatalf("preview disable: %v", err)
	}
	if _, err := operations.Acknowledge(ctx, actor, previewed.ID, previewed.Revision); err != nil {
		t.Fatalf("acknowledge disable: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first application: %v", err)
	}

	// A second run over the same data directory is what a restart is.
	second := openApplication(t, ctx, dataDir, clock)
	if got := second.ApplianceRuntime(); got.Identity.ID != firstIdentity.Identity.ID {
		t.Fatalf("appliance identity after restart = %q, want %q", got.Identity.ID, firstIdentity.Identity.ID)
	} else if got.BootID == firstIdentity.BootID {
		t.Fatal("the restarted appliance reused the previous boot identity")
	}

	restarted := newOperations(t, second, network.NewSimulation(clock), clock)
	if err := restarted.Adopt(ctx); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	adopted, err := restarted.Get(previewed.ID)
	if err != nil {
		t.Fatalf("the change made before the restart is gone: %v", err)
	}
	if adopted.State != network.StateAwaitingConfirmation {
		t.Fatalf("adopted state = %q, want awaiting_confirmation", adopted.State)
	}
	if adopted.ActorID != actor {
		t.Fatalf("adopted actor = %q, want %q", adopted.ActorID, actor)
	}

	events, err := second.AuditRepository().List(ctx, audit.Filter{OperationID: previewed.ID})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("recorded %d events for one change, want at least the request and the application", len(events))
	}
	outcomes := map[audit.Outcome]bool{}
	for _, event := range events {
		outcomes[event.Outcome] = true
		if event.EnvelopeVersion != audit.EnvelopeVersion {
			t.Fatalf("event %s envelope version = %d, want %d", event.ID, event.EnvelopeVersion, audit.EnvelopeVersion)
		}
		if event.ApplianceID != firstIdentity.Identity.ID {
			t.Fatalf("event %s appliance = %q, want %q", event.ID, event.ApplianceID, firstIdentity.Identity.ID)
		}
		if event.BootID != firstIdentity.BootID {
			t.Fatalf("event %s boot = %q, want the boot that made the change", event.ID, event.BootID)
		}
		if event.ActorID.String() != actor {
			t.Fatalf("event %s actor = %q, want %q", event.ID, event.ActorID, actor)
		}
		if event.Component != "network.interfaces" {
			t.Fatalf("event %s component = %q, want network.interfaces", event.ID, event.Component)
		}
		if event.Category != audit.CategoryAudit {
			t.Fatalf("event %s category = %q, want audit", event.ID, event.Category)
		}
		// Nothing about a change may carry a secret, and an interface toggle has
		// none to carry, so the payload is checked for the shape of one anyway.
		if strings.Contains(strings.ToLower(string(event.Payload)), "key") {
			t.Fatalf("event %s payload mentions a key: %s", event.ID, event.Payload)
		}
	}
	if !outcomes[audit.OutcomeRequested] || !outcomes[audit.OutcomeApplied] {
		t.Fatalf("recorded outcomes = %v, want the request and the application recorded separately", outcomes)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second application: %v", err)
	}
}

func openApplication(t *testing.T, ctx context.Context, dataDir string, clock func() time.Time) *App {
	t.Helper()
	application, err := New(ctx, Options{DataDir: dataDir, BindAddress: "127.0.0.1:0", Now: clock})
	if err != nil {
		t.Fatalf("open application: %v", err)
	}
	return application
}

func newOperations(
	t *testing.T, application *App, simulated *network.Simulation, clock func() time.Time,
) *network.OperationManager {
	t.Helper()
	operations, err := network.NewOperationManager(network.OperationManagerOptions{
		Changer: simulated, Observer: simulated,
		Now:   func() time.Time { return clock().UTC() },
		NewID: func() string { return uuid.NewString() },
		Store: application.NetworkOperationRepository(),
		Audit: application.AuditWriter(),
	})
	if err != nil {
		t.Fatalf("build operation manager: %v", err)
	}
	return operations
}
