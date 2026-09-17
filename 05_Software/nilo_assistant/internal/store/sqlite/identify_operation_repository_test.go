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
	created, err := repository.Begin(ctx, "button-1", "button", now.Add(10*time.Second), now)
	if err != nil || !created {
		t.Fatalf("begin = %t, %v", created, err)
	}
	if created, err := repository.Begin(ctx, "button-1", "button", now.Add(10*time.Second), now); err != nil || created {
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
	reserved := event
	reserved.Outcome = audit.OutcomeRequested
	reserved.Payload = json.RawMessage(`{"source":"button","status":"requested"}`)
	if err := repository.Reserve(ctx, "button-1", reserved, now); err != nil {
		t.Fatal(err)
	}
	state, err := repository.State(ctx, "button-1")
	if err != nil {
		t.Fatal(err)
	}
	if state != "reserved" {
		t.Fatalf("state after reservation = %q, want reserved", state)
	}
	pending, err := repository.Pending(ctx)
	if err != nil || len(pending) != 1 || pending[0].ID != "button-1" || pending[0].Source != "button" ||
		pending[0].State != "reserved" || !pending[0].ActiveUntil.Equal(now.Add(10*time.Second)) {
		t.Fatalf("pending identify actions = %+v, %v", pending, err)
	}
	if err := repository.Flush(ctx, "button-1"); err == nil {
		t.Fatal("pre-action reservation was flushable as an acted operation")
	}
	if err := repository.RecordOutcome(ctx, "button-1", true, "complete", "accepted", audit.OutcomeApplied, now); err != nil {
		t.Fatal(err)
	}
	pending, err = repository.Pending(ctx)
	if err != nil || len(pending) != 1 || pending[0].State != "performed" {
		t.Fatalf("performed identify action is not awaiting receipt acknowledgement: %+v, %v", pending, err)
	}
	state, err = repository.State(ctx, "button-1")
	if err != nil || state != "performed" {
		t.Fatalf("state after physical outcome = %q, %v", state, err)
	}
	if err := repository.Flush(ctx, "button-1"); err == nil {
		t.Fatal("stored physical outcome was flushable before audit finalization")
	}
	if err := repository.RecordOutcome(ctx, "button-1", true, "complete", "accepted", audit.OutcomeApplied, now); err != nil {
		t.Fatalf("idempotent physical outcome: %v", err)
	}
	if err := repository.RecordOutcome(ctx, "button-1", true, "complete", "failed", audit.OutcomeFailed, now); err == nil {
		t.Fatal("conflicting acted outcome was accepted")
	}
	if err := repository.MarkActed(ctx, "button-1", now); err != nil {
		t.Fatal(err)
	}
	pending, err = repository.Pending(ctx)
	if err != nil || len(pending) != 1 || pending[0].State != "acted" {
		t.Fatalf("acted identify action is not awaiting receipt acknowledgement: %+v, %v", pending, err)
	}
	if err := repository.MarkActed(ctx, "button-1", now); err != nil {
		t.Fatalf("idempotent audit finalization: %v", err)
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
	stored, err := NewAuditRepository(database).List(ctx, audit.Filter{OperationID: "button-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Outcome != audit.OutcomeApplied ||
		string(stored[0].Payload) != `{"reannouncement":"applied","source":"button","status":"accepted"}` {
		t.Fatalf("stored identify activity = %+v", stored)
	}
}

func TestIdentifyOperationStartupFinalizesPersistedPhysicalOutcome(t *testing.T) {
	ctx := context.Background()
	database := openTemporaryDatabase(t)
	repository := NewIdentifyOperationRepository(database)
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	if created, err := repository.Begin(ctx, "button-2", "button", now.Add(10*time.Second), now); err != nil || !created {
		t.Fatalf("begin = %t, %v", created, err)
	}
	actor, _ := domain.ParseActorID(audit.SystemActorID)
	reserved := audit.Event{
		ID: "01994f4a-3c00-7000-8000-000000000002", ActorID: actor,
		Action: "appliance.identify", SubjectType: "appliance", SubjectID: "local",
		Payload: json.RawMessage(`{"source":"button","status":"requested"}`), CreatedAt: now,
		EnvelopeVersion: audit.EnvelopeVersion, ApplianceID: "appliance", BootID: "boot",
		Category: audit.CategoryAudit, Component: "appliance.identity", ActorKind: audit.ActorKindSystem,
		Outcome: audit.OutcomeRequested, Severity: audit.SeverityInfo, OperationID: "button-2",
		TimeSource: audit.TimeUnsynchronized,
	}
	if err := repository.Reserve(ctx, "button-2", reserved, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordOutcome(ctx, "button-2", false, "partial", "unsupported", audit.OutcomeApplied, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}
	state, err := repository.State(ctx, "button-2")
	if err != nil || state != "settled" {
		t.Fatalf("startup-replayed state = %q, %v", state, err)
	}
	events, err := NewAuditRepository(database).List(ctx, audit.Filter{OperationID: "button-2", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Outcome != audit.OutcomeApplied ||
		string(events[0].Payload) != `{"reannouncement":"failed","source":"button","status":"unsupported"}` {
		t.Fatalf("startup-finalized identify activity = %+v", events)
	}
}
