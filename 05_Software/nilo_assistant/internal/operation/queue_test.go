package operation

import (
	"context"
	"testing"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/policy"
)

func TestPreparePersistsWithoutExecutingAndRunUsesSameTransaction(t *testing.T) {
	f := newFixture(t)
	f.command.AppMode = policy.Auto
	queued, ok := f.service.(interface {
		Prepare(context.Context, SubmitCommand) (domain.Session, error)
		Run(context.Context, domain.SessionID) (domain.Session, error)
	})
	if !ok {
		t.Fatal("operation service cannot acknowledge a transaction before execution")
	}
	value, err := queued.Prepare(context.Background(), f.command)
	if err != nil || value.State != domain.Queued || f.runner.executeCalls != 0 {
		t.Fatalf("prepare=%+v, %v; execute calls=%d", value, err, f.runner.executeCalls)
	}
	result, err := queued.Run(context.Background(), value.ID)
	if err != nil || result.ID != value.ID || result.RequestID != value.RequestID || result.State != domain.Completed {
		t.Fatalf("correlated execution=%+v, %v", result, err)
	}
	if _, err := queued.Run(context.Background(), value.ID); err == nil {
		t.Fatal("terminal transaction executed twice")
	}
}
