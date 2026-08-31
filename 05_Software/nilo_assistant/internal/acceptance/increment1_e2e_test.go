package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/policy"
)

func TestIncrement1RealFakeIntegrationLifecycle(t *testing.T) {
	ctx := context.Background()
	fixture := newIncrementFixture(t)
	defer fixture.Close(t)

	unknownEcho := fixture.Command("echo", `{"value":"approved echo"}`)
	unknownEcho.AppKnown = false
	unknownEcho.AppMode = policy.Ask
	unknownEcho.DeviceKnown = false
	unknownEcho.DeviceMode = ""
	pending, err := fixture.Service.Submit(ctx, unknownEcho)
	if err != nil || pending.State != domain.PendingApproval || fixture.Runner.ExecutionCount() != 0 {
		t.Fatalf("unknown echo Submit() = (%+v, %v), executions = %d", pending, err, fixture.Runner.ExecutionCount())
	}
	fixture.Runner.ResetEvents()
	completed, err := fixture.Service.Approve(ctx, pending.ID, unknownEcho.Actor)
	if err != nil || completed.State != domain.Completed {
		t.Fatalf("echo Approve() = (%+v, %v)", completed, err)
	}
	fixture.AssertProgress(t, []float64{0, 1})

	artifactCommand := fixture.Command("artifact", fixture.ArtifactParameters(t))
	artifactCommand.AppKnown = true
	artifactCommand.AppMode = policy.Auto
	artifactCommand.DeviceKnown = true
	artifactCommand.DeviceMode = policy.Auto
	artifactSession, err := fixture.Service.Submit(ctx, artifactCommand)
	if err != nil || artifactSession.State != domain.Completed {
		t.Fatalf("artifact Submit() = (%+v, %v)", artifactSession, err)
	}
	fixture.AssertArtifact(t, artifactSession)

	sleepCommand := fixture.Command("sleep", `{"duration_ms":60000}`)
	sleepCommand.AppKnown = true
	sleepCommand.AppMode = policy.Ask
	sleepCommand.DeviceKnown = true
	sleepCommand.DeviceMode = policy.Full
	sleepPending, err := fixture.Service.Submit(ctx, sleepCommand)
	if err != nil || sleepPending.State != domain.PendingApproval {
		t.Fatalf("sleep Submit() = (%+v, %v)", sleepPending, err)
	}
	sleepResult := make(chan error, 1)
	go func() {
		_, approveErr := fixture.Service.Approve(ctx, sleepPending.ID, sleepCommand.Actor)
		sleepResult <- approveErr
	}()
	fixture.Runner.WaitForProgress(t, "sleep started")
	if err := fixture.Service.Cancel(ctx, sleepPending.ID, sleepCommand.Actor); err != nil {
		t.Fatalf("sleep Cancel() error = %v", err)
	}
	if err := <-sleepResult; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("sleep Approve() error = %v, want a clean cancelled result or context cancellation", err)
	}
	fixture.AssertSessionAndLease(t, sleepPending.ID, domain.Cancelled, 0)

	crashCommand := fixture.Command("crash", `{}`)
	crashCommand.AppKnown = true
	crashCommand.AppMode = policy.Full
	crashCommand.DeviceKnown = true
	crashCommand.DeviceMode = policy.Full
	crashPending, err := fixture.Service.Submit(ctx, crashCommand)
	if err != nil || crashPending.State != domain.PendingApproval {
		t.Fatalf("crash Submit() = (%+v, %v)", crashPending, err)
	}
	fixture.Runner.ResetEvents()
	crashed, err := fixture.Service.Approve(ctx, crashPending.ID, crashCommand.Actor)
	if !errors.Is(err, supervisor.ErrRunnerCrashed) || crashed.State != domain.Incomplete {
		t.Fatalf("crash Approve() = (%+v, %v)", crashed, err)
	}
	if diagnostics := supervisor.Diagnostics(err); strings.TrimSpace(diagnostics) != "fake crash requested" {
		t.Fatalf("crash diagnostics = %q", diagnostics)
	}
	fixture.AssertProgress(t, []float64{0.5})

	fixture.AssertAuditCorrelation(t, completed, unknownEcho)
	fixture.ProveRestartReconciliation(t, time.Now().UTC())
}
