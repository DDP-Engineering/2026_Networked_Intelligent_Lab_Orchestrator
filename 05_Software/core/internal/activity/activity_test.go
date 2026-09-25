package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
)

// memoryStore is the activity store in memory: the audit repository plus the
// policy, watermarks and the retention operations the sweeper needs.
type memoryStore struct {
	mu         sync.Mutex
	events     []audit.Event
	next       int64
	policy     *Policy
	watermarks map[audit.Category]Watermark
	appendErr  error
}

func newMemoryStore() *memoryStore {
	return &memoryStore{watermarks: map[audit.Category]Watermark{}}
}

func (store *memoryStore) Append(_ context.Context, event audit.Event) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.appendErr != nil {
		return store.appendErr
	}
	store.next++
	event.Sequence = store.next
	store.events = append(store.events, event)
	return nil
}

func (store *memoryStore) List(_ context.Context, filter audit.Filter) ([]audit.Event, error) {
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	matches := make([]audit.Event, 0)
	for _, event := range store.events {
		if filter.ActorID != (domain.ActorID{}) && event.ActorID != filter.ActorID {
			continue
		}
		if filter.SubjectID != "" && event.SubjectID != filter.SubjectID {
			continue
		}
		if filter.Category != "" && event.Category != filter.Category {
			continue
		}
		if strings.HasSuffix(filter.Component, ".*") {
			if !strings.HasPrefix(event.Component, strings.TrimSuffix(filter.Component, "*")) {
				continue
			}
		} else if filter.Component != "" && event.Component != filter.Component {
			continue
		}
		if filter.Outcome != "" && event.Outcome != filter.Outcome {
			continue
		}
		if filter.Severity != "" && event.Severity != filter.Severity {
			continue
		}
		if filter.OperationID != "" && event.OperationID != filter.OperationID {
			continue
		}
		if filter.Action != "" && event.Action != filter.Action {
			continue
		}
		if filter.Sort == "" && event.Sequence <= filter.AfterSequence {
			continue
		}
		if !filter.From.IsZero() && event.CreatedAt.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && !event.CreatedAt.Before(filter.To) {
			continue
		}
		matches = append(matches, event)
	}
	if filter.Sort != "" {
		sort.SliceStable(matches, func(left, right int) bool {
			leftValue, rightValue := sortValue(matches[left], filter.Sort), sortValue(matches[right], filter.Sort)
			less := leftValue < rightValue || (leftValue == rightValue && matches[left].Sequence < matches[right].Sequence)
			if filter.Order == audit.SortDescending {
				return !less && (leftValue != rightValue || matches[left].Sequence != matches[right].Sequence)
			}
			return less
		})
		if filter.AfterSequence > 0 {
			remaining := matches[:0]
			for _, event := range matches {
				value := sortValue(event, filter.Sort)
				after := value > filter.CursorValue || (value == filter.CursorValue && event.Sequence > filter.AfterSequence)
				if filter.Order == audit.SortDescending {
					after = value < filter.CursorValue || (value == filter.CursorValue && event.Sequence < filter.AfterSequence)
				}
				if after {
					remaining = append(remaining, event)
				}
			}
			matches = remaining
		}
	}
	if filter.Offset >= len(matches) {
		return []audit.Event{}, nil
	}
	if filter.Offset > 0 {
		matches = matches[filter.Offset:]
	}
	if filter.Limit > 0 && len(matches) > filter.Limit {
		matches = matches[:filter.Limit]
	}
	return matches, nil
}

func (store *memoryStore) Count(ctx context.Context, filter audit.Filter) (int64, error) {
	filter.AfterSequence, filter.CursorValue = 0, ""
	filter.Limit, filter.Offset = 0, 0
	events, err := store.List(ctx, filter)
	return int64(len(events)), err
}

func (store *memoryStore) PurgeBefore(ctx context.Context, category audit.Category, cutoff time.Time, limit int) (int, error) {
	return store.PurgeExpired(ctx, category, cutoff, limit, nil)
}

func (store *memoryStore) PurgeExpired(_ context.Context, category audit.Category, cutoff time.Time, limit int, keep []string) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	kept := map[string]bool{}
	for _, id := range keep {
		kept[id] = true
	}
	removed := 0
	remaining := store.events[:0:0]
	for _, event := range store.events {
		if removed < limit && event.Category == category && event.CreatedAt.Before(cutoff) && !(event.OperationID != "" && kept[event.OperationID]) {
			removed++
			continue
		}
		remaining = append(remaining, event)
	}
	store.events = remaining
	return removed, nil
}

func (store *memoryStore) PurgeOldest(_ context.Context, category audit.Category, limit int) (int, time.Time, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	removed := 0
	var newest time.Time
	remaining := store.events[:0:0]
	for _, event := range store.events {
		if removed < limit && event.Category == category {
			removed++
			if event.CreatedAt.After(newest) {
				newest = event.CreatedAt
			}
			continue
		}
		remaining = append(remaining, event)
	}
	store.events = remaining
	return removed, newest, nil
}

func (store *memoryStore) RetainedWindow(context.Context) (map[audit.Category]Window, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	windows := map[audit.Category]Window{}
	for _, event := range store.events {
		window := windows[event.Category]
		if window.Count == 0 || event.CreatedAt.Before(window.Oldest) {
			window.Oldest = event.CreatedAt
		}
		if event.CreatedAt.After(window.Newest) {
			window.Newest = event.CreatedAt
		}
		window.Count++
		window.Bytes += int64(len(event.Payload)) + RowOverheadBytes
		windows[event.Category] = window
	}
	return windows, nil
}

func (store *memoryStore) LoadPolicy(context.Context) (*Policy, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.policy, nil
}

func (store *memoryStore) SavePolicy(_ context.Context, policy Policy, _ time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.policy = &policy
	return nil
}

func (store *memoryStore) Watermarks(context.Context) (map[audit.Category]Watermark, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	copied := map[audit.Category]Watermark{}
	for category, watermark := range store.watermarks {
		copied[category] = watermark
	}
	return copied, nil
}

func (store *memoryStore) SetWatermark(_ context.Context, category audit.Category, watermark Watermark) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.watermarks[category] = watermark
	return nil
}

type fixture struct {
	store   *memoryStore
	writer  *audit.Writer
	service *Service
	now     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{store: newMemoryStore(), now: time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)}
	writer, err := audit.NewWriter(audit.WriterOptions{
		Events: f.store, ApplianceID: "appliance-1", BootID: "boot-1",
		Now: func() time.Time { return f.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	f.writer = writer
	service, err := NewService(Options{Store: f.store, ApplianceID: "appliance-1", Now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	f.service = service
	return f
}

func (f *fixture) record(t *testing.T, record audit.Record) {
	t.Helper()
	if record.Category == "" {
		record.Category = audit.CategoryAudit
	}
	if record.Component == "" {
		record.Component = "network.interfaces"
	}
	if record.ActorKind == "" {
		record.ActorKind = audit.ActorKindSystem
	}
	if record.Outcome == "" {
		record.Outcome = audit.OutcomeApplied
	}
	if record.Severity == "" {
		record.Severity = audit.SeverityInfo
	}
	if record.SubjectType == "" {
		record.SubjectType = "interface"
	}
	if record.SubjectID == "" {
		record.SubjectID = "eth0"
	}
	if err := f.writer.Record(context.Background(), record); err != nil {
		t.Fatal(err)
	}
}

func actorID(t *testing.T, seed string) domain.ActorID {
	t.Helper()
	parsed, err := domain.ParseActorID(uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed)).String())
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// TestQueryFiltersEveryFieldAndPages covers REQ-FUNC-d4f81a26: each filter
// narrows, pages are bounded, the cursor tiles a full result set, and the
// retained window comes back with the answer.
func TestQueryFiltersEveryFieldAndPages(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	alice := actorID(t, "alice")
	f.record(t, audit.Record{Action: "interface.disable", ActorKind: audit.ActorKindUser, ActorID: alice, OperationID: "op-1", Outcome: audit.OutcomeRequested})
	f.now = f.now.Add(time.Minute)
	f.record(t, audit.Record{Action: "interface.disable", ActorKind: audit.ActorKindUser, ActorID: alice, OperationID: "op-1", Outcome: audit.OutcomeConfirmed})
	f.now = f.now.Add(time.Minute)
	f.record(t, audit.Record{Category: audit.CategoryEvent, Component: "network.wireguard", Action: "vpn.startup", SubjectType: "profile", SubjectID: "office", Severity: audit.SeverityWarning, Outcome: audit.OutcomeFailed})
	f.now = f.now.Add(time.Minute)
	f.record(t, audit.Record{Component: "network.device_networks", Action: "device_network.plan", SubjectType: "device_network", SubjectID: "line3"})

	for name, filter := range map[string]Filter{
		"category":  {Category: "event"},
		"component": {Component: "network.device_networks"},
		"target":    {Target: "office"},
		"actor":     {Actor: alice.String(), Outcome: "confirmed"},
		"severity":  {Severity: "warning"},
		"outcome":   {Outcome: "requested"},
		"operation": {Operation: "op-1", Outcome: "requested"},
		"window":    {From: f.now.Add(-90 * time.Second).Format(time.RFC3339), To: f.now.Add(-30 * time.Second).Format(time.RFC3339)},
	} {
		page, err := f.service.Query(ctx, filter)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(page.Items) != 1 {
			t.Fatalf("%s: %d items, want 1: %+v", name, len(page.Items), page.Items)
		}
	}
	// A bounded page with a cursor that tiles the full set.
	first, err := f.service.Query(ctx, Filter{Limit: 3})
	if err != nil || len(first.Items) != 3 || first.NextCursor == "" {
		t.Fatalf("first page = %d items cursor %q err %v", len(first.Items), first.NextCursor, err)
	}
	second, err := f.service.Query(ctx, Filter{Limit: 3, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %d items cursor %q err %v", len(second.Items), second.NextCursor, err)
	}
	if second.Items[0].Sequence <= first.Items[2].Sequence {
		t.Fatal("pages overlap")
	}
	if _, err := f.service.Query(ctx, Filter{Limit: MaximumPageSize + 1}); err == nil {
		t.Fatal("an oversized page was accepted")
	}
	if _, err := f.service.Query(ctx, Filter{Category: "secrets"}); err == nil {
		t.Fatal("an unknown category was accepted")
	}
	if _, err := f.service.Query(ctx, Filter{Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("a malformed cursor was accepted")
	}
	// The retained window says what the answer was drawn from.
	window := first.RetainedWindow[audit.CategoryAudit]
	if window.Count != 3 || window.Oldest.IsZero() || window.Newest.Before(window.Oldest) {
		t.Fatalf("retained audit window = %+v", window)
	}
	if first.LoggingHealth.AuditWritable != true {
		t.Fatalf("logging health = %+v", first.LoggingHealth)
	}
}

func TestQuerySortsAndPagesWithAnOpaqueCursor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, action := range []string{"zebra", "alpha", "middle"} {
		f.record(t, audit.Record{Action: action})
		f.now = f.now.Add(time.Minute)
	}

	first, err := f.service.Query(ctx, Filter{Sort: "activity", Order: "asc", Limit: 2})
	if err != nil {
		t.Fatalf("first sorted page: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].Action != "alpha" || first.Items[1].Action != "middle" {
		t.Fatalf("first sorted page = %+v", first.Items)
	}
	if first.NextCursor == "" || first.NextCursor == "2" {
		t.Fatalf("sorted cursor = %q, want an opaque cursor", first.NextCursor)
	}
	second, err := f.service.Query(ctx, Filter{Sort: "activity", Order: "asc", Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("second sorted page: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Action != "zebra" {
		t.Fatalf("second sorted page = %+v", second.Items)
	}
	if _, err := f.service.Query(ctx, Filter{Sort: "time", Order: "desc", Cursor: first.NextCursor}); err == nil {
		t.Fatal("a cursor issued for another sort was accepted")
	}

	numbered, err := f.service.Query(ctx, Filter{
		Sort: "activity", Order: "asc", Limit: 2, Page: 2,
	})
	if err != nil {
		t.Fatalf("numbered page: %v", err)
	}
	if numbered.Total != 3 || len(numbered.Items) != 1 || numbered.Items[0].Action != "zebra" {
		t.Fatalf("numbered page = total %d, items %+v", numbered.Total, numbered.Items)
	}

	all, err := f.service.Query(ctx, Filter{Sort: "time", Order: "desc", All: true})
	if err != nil {
		t.Fatalf("all records: %v", err)
	}
	if all.Total != 3 || len(all.Items) != 3 || all.Items[0].Action != "middle" {
		t.Fatalf("all records = total %d, items %+v", all.Total, all.Items)
	}
	if _, err := f.service.Query(ctx, Filter{All: true, Page: 2}); err == nil {
		t.Fatal("a second page was accepted for the whole selection")
	}
}

// TestExportCarriesMetadataAndStaysBounded covers REQ-FUNC-6ba25f39: both
// formats carry the schema version, appliance identity, time range and
// retained window; the export is bounded; a CSV field starting with a
// formula character is neutralized.
func TestExportCarriesMetadataAndStaysBounded(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.record(t, audit.Record{Action: "interface.disable", TargetLabel: "=HYPERLINK(\"x\")", OperationID: "op-1", Outcome: audit.OutcomeRequested})
	for index := 0; index < 5; index++ {
		f.now = f.now.Add(time.Second)
		f.record(t, audit.Record{Action: fmt.Sprintf("action.%d", index), Details: map[string]string{"note": "+not a formula"}})
	}

	var jsonl bytes.Buffer
	summary, err := f.service.Export(ctx, Filter{}, FormatJSONL, &jsonl)
	if err != nil {
		t.Fatalf("jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(jsonl.String()), "\n")
	if len(lines) != 8 || summary.Rows != 6 || summary.Truncated {
		t.Fatalf("jsonl lines=%d summary=%+v", len(lines), summary)
	}
	var header struct {
		SchemaVersion  string            `json:"schema_version"`
		ApplianceID    string            `json:"appliance_id"`
		TimeRange      map[string]string `json:"time_range"`
		RetainedWindow map[string]any    `json:"retained_window"`
		Redaction      string            `json:"redaction"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header.SchemaVersion != SchemaVersion || header.ApplianceID != "appliance-1" || header.TimeRange["to"] == "" || len(header.RetainedWindow) == 0 || header.Redaction == "" {
		t.Fatalf("header = %+v", header)
	}
	var trailer struct {
		Trailer   bool `json:"trailer"`
		Rows      int  `json:"rows"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &trailer); err != nil || !trailer.Trailer || trailer.Rows != 6 {
		t.Fatalf("trailer = %+v %v", trailer, err)
	}

	var csv bytes.Buffer
	if _, err := f.service.Export(ctx, Filter{}, FormatCSV, &csv); err != nil {
		t.Fatalf("csv: %v", err)
	}
	text := csv.String()
	if !strings.HasPrefix(text, "# schema_version="+SchemaVersion) || !strings.Contains(text, "# appliance_id=appliance-1") || !strings.Contains(text, "# retained_window=") {
		t.Fatalf("csv header:\n%s", text)
	}
	if !strings.Contains(text, `"'=HYPERLINK(""x"")"`) || strings.Contains(text, "\n=HYPERLINK") {
		t.Fatalf("formula not neutralized:\n%s", text)
	}
	// No data cell begins with a formula character; a value nested inside
	// the JSON details is a string in a JSON cell, not a cell of its own.
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "=") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "@") {
			t.Fatalf("a line begins with a formula character:\n%s", text)
		}
	}

	// The bound is enforced and disclosed.
	bounded, err := NewService(Options{Store: f.store, ApplianceID: "appliance-1", Now: func() time.Time { return f.now }, ExportRowLimit: 4})
	if err != nil {
		t.Fatal(err)
	}
	var limited bytes.Buffer
	summary, err = bounded.Export(ctx, Filter{}, FormatJSONL, &limited)
	if err != nil || summary.Rows != 4 || !summary.Truncated || !strings.Contains(limited.String(), `"truncated":true`) {
		t.Fatalf("bounded export summary=%+v err=%v", summary, err)
	}
	if _, err := f.service.Export(ctx, Filter{}, "xml", &limited); !errors.Is(err, ErrInvalidFilter) {
		t.Fatalf("unknown format err = %v", err)
	}
}

// TestHousekeepingPurgesByAgeAndQuotaAndDisclosesTheGap covers AU-07: expired
// records go in bounded batches, quota pressure removes the oldest, the
// watermark shows the retained window actually available, pending operation
// evidence is kept, and the sweep is itself recorded.
func TestHousekeepingPurgesByAgeAndQuotaAndDisclosesTheGap(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	start := f.now
	// Old audit rows, one belonging to a still-pending operation.
	f.now = start.Add(-100 * 24 * time.Hour)
	f.record(t, audit.Record{Action: "interface.disable", OperationID: "pending-op", Outcome: audit.OutcomeApplied})
	f.record(t, audit.Record{Action: "interface.enable", OperationID: "old-op", Outcome: audit.OutcomeConfirmed})
	// Event rows within age but over a tiny quota.
	f.now = start.Add(-time.Hour)
	for index := 0; index < 20; index++ {
		f.now = f.now.Add(time.Second)
		f.record(t, audit.Record{Category: audit.CategoryEvent, Action: "runtime.step", Details: map[string]string{"padding": strings.Repeat("x", 60000)}})
	}
	f.now = start
	policy := DefaultPolicy()
	policy.QuotaBytes[audit.CategoryEvent] = 1 << 20
	if err := f.service.SavePolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	keeper, err := NewHousekeeper(HousekeeperOptions{
		Service: f.service, Audit: f.writer,
		Pending: func(context.Context) ([]string, error) { return []string{"pending-op"}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := keeper.Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if report.Removed[audit.CategoryAudit] != 1 || report.Removed[audit.CategoryEvent] == 0 {
		t.Fatalf("report = %+v", report)
	}
	remaining, _ := f.store.List(ctx, audit.Filter{Category: audit.CategoryAudit, Action: "interface.disable"})
	if len(remaining) != 1 {
		t.Fatal("pending operation evidence was purged")
	}
	windows, _ := f.store.RetainedWindow(ctx)
	if windows[audit.CategoryEvent].Bytes > policy.QuotaBytes[audit.CategoryEvent] {
		t.Fatalf("events still over quota: %+v", windows[audit.CategoryEvent])
	}
	page, err := f.service.Query(ctx, Filter{Category: "event"})
	if err != nil {
		t.Fatal(err)
	}
	window := page.RetainedWindow[audit.CategoryEvent]
	if !window.Truncated || window.PurgedBefore.IsZero() || window.PurgeReason != "quota" {
		t.Fatalf("event window does not disclose the gap: %+v", window)
	}
	if page.RetainedWindow[audit.CategoryAudit].PurgeReason != "age" {
		t.Fatalf("audit window does not disclose the age purge: %+v", page.RetainedWindow[audit.CategoryAudit])
	}
	sweeps, _ := f.store.List(ctx, audit.Filter{Action: "activity.retention.purged"})
	if len(sweeps) != 1 || sweeps[0].Category != audit.CategoryEvent || sweeps[0].ActorKind != audit.ActorKindSystem {
		t.Fatalf("sweep events = %+v", sweeps)
	}
	// A quiet sweep records nothing.
	if _, err := keeper.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if sweeps, _ = f.store.List(ctx, audit.Filter{Action: "activity.retention.purged"}); len(sweeps) != 1 {
		t.Fatal("a sweep that removed nothing was recorded")
	}
}

// TestPolicyIsValidatedAndDefaulted: an absent policy is the shipped default;
// a nonsensical one is refused.
func TestPolicyIsValidatedAndDefaulted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	policy, err := f.service.Policy(ctx)
	if err != nil || policy.RetentionDays[audit.CategoryAudit] != 90 || policy.RetentionDays[audit.CategoryEvent] != 14 {
		t.Fatalf("default policy = %+v %v", policy, err)
	}
	bad := DefaultPolicy()
	bad.RetentionDays[audit.CategoryEvent] = 0
	if err := f.service.SavePolicy(ctx, bad); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("err = %v", err)
	}
	bad = DefaultPolicy()
	delete(bad.QuotaBytes, audit.CategoryEvent)
	if err := f.service.SavePolicy(ctx, bad); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("err = %v", err)
	}
}

// TestLoggingHealthReportsAuditFailures: a monitored repository that cannot
// append is reported as degraded, and recovers when appends succeed again.
func TestLoggingHealthReportsAuditFailures(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	monitor := NewMonitor(f.store, func() time.Time { return f.now })
	writer, err := audit.NewWriter(audit.WriterOptions{Events: monitor, ApplianceID: "appliance-1", BootID: "boot-1", Now: func() time.Time { return f.now }})
	if err != nil {
		t.Fatal(err)
	}
	monitored, err := NewService(Options{Store: f.store, ApplianceID: "appliance-1", Now: func() time.Time { return f.now }, Monitor: monitor})
	if err != nil {
		t.Fatal(err)
	}
	f.store.appendErr = errors.New("disk full")
	if err := writer.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "system", Action: "x", SubjectType: "s", SubjectID: "1", ActorKind: audit.ActorKindSystem, Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo}); err == nil {
		t.Fatal("append succeeded")
	}
	page, _ := monitored.Query(ctx, Filter{})
	if page.LoggingHealth.AuditWritable || page.LoggingHealth.LastFailure == "" || page.LoggingHealth.Failures != 1 {
		t.Fatalf("health = %+v", page.LoggingHealth)
	}
	f.store.appendErr = nil
	if err := writer.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "system", Action: "x", SubjectType: "s", SubjectID: "1", ActorKind: audit.ActorKindSystem, Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo}); err != nil {
		t.Fatal(err)
	}
	page, _ = monitored.Query(ctx, Filter{})
	if !page.LoggingHealth.AuditWritable {
		t.Fatalf("health after recovery = %+v", page.LoggingHealth)
	}
	// The health answer never carries the failure text verbatim beyond a
	// bounded, sanitized message.
	if strings.ContainsAny(page.LoggingHealth.LastFailure, "\n\r") {
		t.Fatal("unsanitized failure text")
	}
}
