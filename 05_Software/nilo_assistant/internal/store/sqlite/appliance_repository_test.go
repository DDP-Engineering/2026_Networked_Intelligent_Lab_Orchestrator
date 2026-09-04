package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/network"
)

// TestApplianceIdentityIsCreatedOnceAndKeptAcrossRestarts covers
// REQ-DATA-58f3c012: an appliance gets its identity once and keeps it, and two
// appliances never share one.
func TestApplianceIdentityIsCreatedOnceAndKeptAcrossRestarts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	commissioned := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)

	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	stored, err := NewApplianceRepository(first).Ensure(ctx, appliance.Identity{
		ID: "0b6e0d3a-1f2c-4a5b-9c8d-000000000001", Name: "bench", CommissionedAt: commissioned,
	})
	if err != nil {
		t.Fatalf("ensure first identity: %v", err)
	}
	if stored.ID != "0b6e0d3a-1f2c-4a5b-9c8d-000000000001" {
		t.Fatalf("stored ID = %q, want the candidate", stored.ID)
	}

	// A second Ensure is a start, not a re-provisioning: whatever candidate it
	// offers, the appliance keeps the identity it already has.
	again, err := NewApplianceRepository(first).Ensure(ctx, appliance.Identity{
		ID: "0b6e0d3a-1f2c-4a5b-9c8d-000000000002", Name: "other", CommissionedAt: commissioned.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ensure second time: %v", err)
	}
	if again.ID != stored.ID || again.Name != "bench" {
		t.Fatalf("second ensure = %+v, want the first identity unchanged", again)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	survived, err := NewApplianceRepository(reopened).Ensure(ctx, appliance.Identity{
		ID: "0b6e0d3a-1f2c-4a5b-9c8d-000000000003", Name: "third", CommissionedAt: commissioned,
	})
	if err != nil {
		t.Fatalf("ensure after restart: %v", err)
	}
	if survived.ID != stored.ID {
		t.Fatalf("identity after restart = %q, want %q", survived.ID, stored.ID)
	}
	if !survived.CommissionedAt.Equal(commissioned) {
		t.Fatalf("commissioned at = %v, want %v", survived.CommissionedAt, commissioned)
	}

	// A second data directory is a second appliance, so it receives its own.
	separate := openTemporaryDatabase(t)
	other, err := NewApplianceRepository(separate).Ensure(ctx, appliance.Identity{
		ID: "0b6e0d3a-1f2c-4a5b-9c8d-000000000009", Name: "second appliance", CommissionedAt: commissioned,
	})
	if err != nil {
		t.Fatalf("ensure separate appliance: %v", err)
	}
	if other.ID == stored.ID {
		t.Fatal("a separate database shares the first appliance's identity")
	}
}

// TestAnExistingDatabaseUpgradesWithoutDisturbingWhatItHolds covers the
// migration half of REQ-DATA-91e7ab46 and REQ-DATA-2c8b5f79 against the case
// that actually matters: an appliance that already has history.
//
// It builds a database at the previous schema, puts an audit row in it, and
// then opens it the way the running service does.
func TestAnExistingDatabaseUpgradesWithoutDisturbingWhatItHolds(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	actor := mustActor(t, "5f6d2e10-0000-4000-8000-00000000000e")
	recorded := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

	earlier, err := sql.Open(driverName, sqliteDSN(path))
	if err != nil {
		t.Fatalf("open at the earlier schema: %v", err)
	}
	earlier.SetMaxOpenConns(1)
	if err := configureConnection(ctx, earlier); err != nil {
		t.Fatalf("configure: %v", err)
	}
	// Everything up to and including 005_network_traffic, which is what an
	// appliance running the previous release has.
	if err := applyMigrationSet(ctx, earlier, migrations[:5]); err != nil {
		t.Fatalf("apply the earlier schema: %v", err)
	}
	if _, err := earlier.ExecContext(ctx, `
		INSERT INTO audit_events (id, sequence, actor_id, action, subject_type, subject_id, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "44444444-0000-4000-8000-000000000001", 7, actor.String(), "identity.role.changed",
		"user", "u-9", []byte(`{"from":"viewer","to":"operator"}`), recorded.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert an event at the earlier schema: %v", err)
	}
	if err := earlier.Close(); err != nil {
		t.Fatalf("close the earlier database: %v", err)
	}

	// Opening applies 006 and re-verifies the digests of 001 to 005, so a
	// successful open is also proof that none of them was edited.
	upgraded, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade in place: %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })

	repository := NewAuditRepository(upgraded)
	events, err := repository.List(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("list after upgrade: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("listed %d events after the upgrade, want the one that was already there", len(events))
	}
	if events[0].Action != "identity.role.changed" || events[0].Sequence != 7 {
		t.Fatalf("the stored event changed on upgrade: %+v", events[0])
	}
	if string(events[0].Payload) != `{"from":"viewer","to":"operator"}` {
		t.Fatalf("the stored payload changed on upgrade: %s", events[0].Payload)
	}
	if events[0].EnvelopeVersion != 1 || events[0].ApplianceID != "" {
		t.Fatalf("the upgrade invented envelope values: %+v", events[0])
	}

	// The allocator was seeded past the history that was already there, so the
	// next event cannot collide with it.
	if err := repository.Append(ctx, enveloped(actor, 5, recorded.Add(time.Hour))); err != nil {
		t.Fatalf("append after upgrade: %v", err)
	}
	after, err := repository.List(ctx, audit.Filter{AfterSequence: 7})
	if err != nil {
		t.Fatalf("list the new event: %v", err)
	}
	if len(after) != 1 || after[0].Sequence != 8 {
		t.Fatalf("the first event after the upgrade = %+v, want sequence 8", after)
	}
}

// TestApplianceLabelsChangeWithoutChangingIdentity covers the mutable part of
// REQ-DATA-58f3c012: a rename is a label change, not a new appliance.
func TestApplianceLabelsChangeWithoutChangingIdentity(t *testing.T) {
	ctx := context.Background()
	repository := NewApplianceRepository(openTemporaryDatabase(t))
	stored, err := repository.Ensure(ctx, appliance.Identity{
		ID: "3c1d5f70-0000-4000-8000-000000000001", Name: "unnamed",
		CommissionedAt: time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	relabelled, err := repository.SetLabels(ctx, "line 4 gateway", "hall B")
	if err != nil {
		t.Fatalf("set labels: %v", err)
	}
	if relabelled.ID != stored.ID {
		t.Fatalf("identity changed on rename: %q then %q", stored.ID, relabelled.ID)
	}
	if relabelled.Name != "line 4 gateway" || relabelled.Location != "hall B" {
		t.Fatalf("labels = %q / %q, want the new values", relabelled.Name, relabelled.Location)
	}
}

// TestAuditEnvelopeRoundTripsAndLeavesOlderRowsReadable covers
// REQ-DATA-91e7ab46: the envelope is stored and read back whole, and a row
// written before the envelope existed keeps its original meaning.
func TestAuditEnvelopeRoundTripsAndLeavesOlderRowsReadable(t *testing.T) {
	ctx := context.Background()
	db := openTemporaryDatabase(t)
	repository := NewAuditRepository(db)
	actor := mustActor(t, "5f6d2e10-0000-4000-8000-00000000000a")
	recorded := time.Date(2026, 9, 3, 9, 30, 0, 0, time.UTC)

	// A row in the shape the previous writer produced, inserted directly so the
	// test proves the reader tolerates it rather than the writer producing it.
	if _, err := db.SQL().ExecContext(ctx, `
		INSERT INTO audit_events (id, sequence, actor_id, action, subject_type, subject_id, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "11111111-0000-4000-8000-000000000001", 1, actor.String(), "identity.created",
		"user", "u-1", []byte(`{"username":"admin"}`), recorded.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	full := audit.Event{
		ID: "22222222-0000-4000-8000-000000000002", ActorID: actor,
		Action: "network.device_network.enable", SubjectType: "device_network", SubjectID: "dn-7",
		Payload:   json.RawMessage(`{"before":{"enabled":false},"after":{"enabled":true}}`),
		CreatedAt: recorded.Add(time.Minute),

		EnvelopeVersion: audit.EnvelopeVersion,
		ApplianceID:     "0b6e0d3a-1f2c-4a5b-9c8d-000000000001",
		BootID:          "boot-1",
		Category:        audit.CategoryAudit,
		Component:       "network.device_networks",
		ActorKind:       audit.ActorKindUser,
		Outcome:         audit.OutcomeApplied,
		Severity:        audit.SeverityInfo,
		OperationID:     "op-42",
		TargetLabel:     "line 4 devices",
		TimeSource:      audit.TimeSynchronized,
	}
	if err := repository.Append(ctx, full); err != nil {
		t.Fatalf("append enveloped event: %v", err)
	}

	events, err := repository.List(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("listed %d events, want 2", len(events))
	}

	legacy := events[0]
	if legacy.EnvelopeVersion != 1 {
		t.Fatalf("legacy envelope version = %d, want 1", legacy.EnvelopeVersion)
	}
	if legacy.Action != "identity.created" || legacy.SubjectID != "u-1" {
		t.Fatalf("legacy row changed meaning: %+v", legacy)
	}
	for name, value := range map[string]string{
		"appliance": legacy.ApplianceID, "boot": legacy.BootID,
		"component": legacy.Component, "operation": legacy.OperationID,
	} {
		if value != "" {
			t.Fatalf("legacy %s = %q, want empty rather than an invented value", name, value)
		}
	}

	stored := events[1]
	stored.Sequence = 0
	if stored.EnvelopeVersion != audit.EnvelopeVersion {
		t.Fatalf("envelope version = %d, want %d", stored.EnvelopeVersion, audit.EnvelopeVersion)
	}
	if stored.ApplianceID != full.ApplianceID || stored.BootID != full.BootID {
		t.Fatalf("appliance/boot = %q/%q, want %q/%q", stored.ApplianceID, stored.BootID, full.ApplianceID, full.BootID)
	}
	if stored.Category != audit.CategoryAudit || stored.Component != "network.device_networks" {
		t.Fatalf("category/component = %q/%q", stored.Category, stored.Component)
	}
	if stored.ActorKind != audit.ActorKindUser || stored.Outcome != audit.OutcomeApplied {
		t.Fatalf("actor kind/outcome = %q/%q", stored.ActorKind, stored.Outcome)
	}
	if stored.Severity != audit.SeverityInfo || stored.TimeSource != audit.TimeSynchronized {
		t.Fatalf("severity/time source = %q/%q", stored.Severity, stored.TimeSource)
	}
	if stored.OperationID != "op-42" || stored.TargetLabel != "line 4 devices" {
		t.Fatalf("correlation/label = %q/%q", stored.OperationID, stored.TargetLabel)
	}
}

// TestAuditSequenceNeverRepeatsAfterRetentionEmptiesTheTable covers
// REQ-DATA-2c8b5f79. MAX(sequence)+1 is correct only while nothing is ever
// deleted, and this milestone deletes.
func TestAuditSequenceNeverRepeatsAfterRetentionEmptiesTheTable(t *testing.T) {
	ctx := context.Background()
	repository := NewAuditRepository(openTemporaryDatabase(t))
	actor := mustActor(t, "5f6d2e10-0000-4000-8000-00000000000b")
	base := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)

	for index := range 3 {
		if err := repository.Append(ctx, enveloped(actor, index, base.Add(time.Duration(index)*time.Minute))); err != nil {
			t.Fatalf("append %d: %v", index, err)
		}
	}
	before, err := repository.List(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("list before purge: %v", err)
	}
	highest := before[len(before)-1].Sequence
	if highest != 3 {
		t.Fatalf("highest sequence = %d, want 3", highest)
	}

	purged, err := repository.PurgeBefore(ctx, audit.CategoryAudit, base.Add(time.Hour), 100)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if purged != 3 {
		t.Fatalf("purged %d rows, want 3", purged)
	}
	remaining, err := repository.List(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("list after purge: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("%d rows survived the purge, want none", len(remaining))
	}

	if err := repository.Append(ctx, enveloped(actor, 9, base.Add(2*time.Hour))); err != nil {
		t.Fatalf("append after purge: %v", err)
	}
	after, err := repository.List(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("list after append: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("listed %d events, want 1", len(after))
	}
	if after[0].Sequence <= highest {
		t.Fatalf("sequence after retention = %d, want greater than the deleted maximum %d", after[0].Sequence, highest)
	}
}

// TestNetworkOperationsSurviveARestartAndListPending covers REQ-FUNC-97c1e5d8
// and the persistence half of REQ-REL-2e91cb74.
func TestNetworkOperationsSurviveARestartAndListPending(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nilo.sqlite")
	created := time.Date(2026, 9, 3, 11, 0, 0, 0, time.UTC)

	first, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repository := NewNetworkOperationRepository(first)
	pending := network.OperationRecord{
		ID: "op-1", Action: string(network.ActionInterfaceDisable), Target: "eth1",
		ActorID: "5f6d2e10-0000-4000-8000-00000000000c", State: string(network.StateAwaitingConfirmation),
		Revision: "abc123", CheckpointID: "cp-1",
		CreatedAt: created, UpdatedAt: created.Add(time.Second),
		ConfirmDeadline: created.Add(120 * time.Second),
	}
	settled := network.OperationRecord{
		ID: "op-2", Action: string(network.ActionProfileImport), Target: "office",
		ActorID: pending.ActorID, State: string(network.StateConfirmed), Revision: "def456",
		CreatedAt: created.Add(-time.Hour), UpdatedAt: created.Add(-time.Hour),
	}
	for _, record := range []network.OperationRecord{pending, settled} {
		if err := repository.Save(ctx, record); err != nil {
			t.Fatalf("save %s: %v", record.ID, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := NewNetworkOperationRepository(reopened)

	unfinished, err := restarted.Unfinished(ctx)
	if err != nil {
		t.Fatalf("list unfinished: %v", err)
	}
	if len(unfinished) != 1 || unfinished[0].ID != "op-1" {
		t.Fatalf("unfinished = %+v, want only op-1", unfinished)
	}
	if unfinished[0].State != string(network.StateAwaitingConfirmation) {
		t.Fatalf("state = %q, want awaiting_confirmation", unfinished[0].State)
	}
	if !unfinished[0].ConfirmDeadline.Equal(pending.ConfirmDeadline) {
		t.Fatalf("deadline = %v, want %v", unfinished[0].ConfirmDeadline, pending.ConfirmDeadline)
	}
	if unfinished[0].CheckpointID != "cp-1" {
		t.Fatalf("checkpoint = %q, want cp-1", unfinished[0].CheckpointID)
	}

	recent, err := restarted.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent) != 2 || recent[0].ID != "op-1" {
		t.Fatalf("recent = %+v, want op-1 newest first", recent)
	}

	// A later save of the same operation is the same operation moving on, not a
	// second row: reattachment must not show one change twice.
	moved := pending
	moved.State = string(network.StateConfirmed)
	moved.UpdatedAt = created.Add(time.Minute)
	moved.ConfirmDeadline = time.Time{}
	if err := restarted.Save(ctx, moved); err != nil {
		t.Fatalf("save moved: %v", err)
	}
	stillUnfinished, err := restarted.Unfinished(ctx)
	if err != nil {
		t.Fatalf("list unfinished after confirm: %v", err)
	}
	if len(stillUnfinished) != 0 {
		t.Fatalf("unfinished after confirmation = %+v, want none", stillUnfinished)
	}
	afterMove, err := restarted.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("list recent after confirm: %v", err)
	}
	if len(afterMove) != 2 {
		t.Fatalf("recent = %d rows, want the same 2 operations", len(afterMove))
	}
}

// TestNetworkOperationParametersRejectSecretText covers the storage half of
// REQ-SEC-c62481fe: the operations table is public metadata, so a record
// carrying profile text has no place to put it.
func TestNetworkOperationParametersRejectSecretText(t *testing.T) {
	ctx := context.Background()
	repository := NewNetworkOperationRepository(openTemporaryDatabase(t))
	err := repository.Save(ctx, network.OperationRecord{
		ID: "op-3", Action: string(network.ActionProfileImport), Target: "office",
		ActorID: "5f6d2e10-0000-4000-8000-00000000000d", State: string(network.StatePreviewed),
		Revision:   "aaa",
		Parameters: json.RawMessage(`{"private_key":"4Gv0Q2s="}`),
		CreatedAt:  time.Date(2026, 9, 3, 11, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 9, 3, 11, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("saved an operation whose parameters name a private key, want a refusal")
	}
}

func enveloped(actor domain.ActorID, index int, at time.Time) audit.Event {
	return audit.Event{
		ID:              uuidForIndex(index),
		ActorID:         actor,
		Action:          "network.interface.disable",
		SubjectType:     "interface",
		SubjectID:       "eth1",
		Payload:         json.RawMessage(`{}`),
		CreatedAt:       at,
		EnvelopeVersion: audit.EnvelopeVersion,
		ApplianceID:     "0b6e0d3a-1f2c-4a5b-9c8d-000000000001",
		BootID:          "boot-1",
		Category:        audit.CategoryAudit,
		Component:       "network.interfaces",
		ActorKind:       audit.ActorKindUser,
		Outcome:         audit.OutcomeApplied,
		Severity:        audit.SeverityInfo,
		TimeSource:      audit.TimeSynchronized,
	}
}

func uuidForIndex(index int) string {
	return "33333333-0000-4000-8000-00000000000" + string(rune('0'+index))
}

func mustActor(t *testing.T, value string) domain.ActorID {
	t.Helper()
	actor, err := domain.ParseActorID(value)
	if err != nil {
		t.Fatalf("parse actor %q: %v", value, err)
	}
	return actor
}
