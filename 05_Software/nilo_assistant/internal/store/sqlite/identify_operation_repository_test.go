package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
)

func TestIdentifyOperationAuditRetryDoesNotRepeatTheAction(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewIdentifyOperationRepository(database)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	created, err := repository.Begin(ctx, "button-1", "button", now)
	if err != nil || !created {
		t.Fatalf("begin = %t, %v", created, err)
	}
	if created, err := repository.Begin(ctx, "button-1", "button", now); err != nil || created {
		t.Fatalf("duplicate begin = %t, %v", created, err)
	}
	actor, _ := domain.ParseActorID(audit.SystemActorID)
	event := audit.Event{
		ID: "01994f4a-3c00-7000-8000-000000000001", Sequence: 0, ActorID: actor,
		Action: "appliance.identify", SubjectType: "appliance", SubjectID: "local",
		Payload: json.RawMessage(`{"source":"button","status":"accepted"}`), CreatedAt: now,
		EnvelopeVersion: audit.EnvelopeVersion, ApplianceID: "appliance", BootID: "boot",
		Category: audit.CategoryAudit, Component: "appliance.identity", ActorKind: audit.ActorKindSystem,
		Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo, OperationID: "button-1",
		TimeSource: audit.TimeUnsynchronized,
	}
	if err := repository.MarkActed(ctx, "button-1", true, "complete", event, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.Flush(ctx, "button-1"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Flush(ctx, "button-1"); err != nil && !errors.Is(err, audit.ErrEventAlreadyAppended) {
		t.Fatalf("idempotent flush: %v", err)
	}
	var events, settled int
	if err := database.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE operation_id = ?`, "button-1").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM identify_operations WHERE id = ? AND state = 'settled'`, "button-1").Scan(&settled); err != nil {
		t.Fatal(err)
	}
	if events != 1 || settled != 1 {
		t.Fatalf("events=%d settled=%d", events, settled)
	}
}
