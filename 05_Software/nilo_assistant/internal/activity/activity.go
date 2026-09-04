package activity

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
)

const (
	// SchemaVersion names the export and query shape.
	SchemaVersion = "nilo.activity/1"
	// DefaultPageSize and MaximumPageSize bound a query page.
	DefaultPageSize = 100
	MaximumPageSize = 500
	// DefaultExportRowLimit and DefaultExportByteLimit bound an export.
	DefaultExportRowLimit  = 20000
	DefaultExportByteLimit = 16 << 20
	exportBatch            = 500
	// Redaction names what an export never carries.
	Redaction = "envelope and safe details only; no secret, header, cookie, token or raw request reaches a record"
)

var (
	ErrInvalidFilter = errors.New("activity filter rejected")
)

// Format is an export format.
type Format string

const (
	FormatJSONL Format = "jsonl"
	FormatCSV   Format = "csv"
)

// Store is what the service reads and sweeps.
type Store interface {
	audit.Repository
	RetainedWindow(ctx context.Context) (map[audit.Category]Window, error)
	PurgeExpired(ctx context.Context, category audit.Category, cutoff time.Time, limit int, keep []string) (int, error)
	PurgeOldest(ctx context.Context, category audit.Category, limit int) (int, time.Time, error)
	LoadPolicy(ctx context.Context) (*Policy, error)
	SavePolicy(ctx context.Context, policy Policy, at time.Time) error
	Watermarks(ctx context.Context) (map[audit.Category]Watermark, error)
	SetWatermark(ctx context.Context, category audit.Category, watermark Watermark) error
}

// Options are the service's collaborators.
type Options struct {
	Store       Store
	ApplianceID string
	Now         func() time.Time
	// Monitor reports logging health; optional.
	Monitor *Monitor
	// ExportRowLimit and ExportByteLimit override the defaults; zero keeps them.
	ExportRowLimit  int
	ExportByteLimit int64
}

// Service answers activity queries and exports.
type Service struct{ options Options }

func NewService(options Options) (*Service, error) {
	if options.Store == nil || options.ApplianceID == "" || options.Now == nil {
		return nil, errors.New("activity service requires a store, the appliance identity and a clock")
	}
	if options.ExportRowLimit <= 0 {
		options.ExportRowLimit = DefaultExportRowLimit
	}
	if options.ExportByteLimit <= 0 {
		options.ExportByteLimit = DefaultExportByteLimit
	}
	return &Service{options: options}, nil
}

// Filter is a query as a caller states it, in wire values.
type Filter struct {
	Category  string
	Component string
	Target    string
	Actor     string
	Severity  string
	Outcome   string
	Operation string
	Action    string
	From      string
	To        string
	Cursor    string
	Limit     int
}

// RetainedWindow is what one category holds, together with what retention
// took away before it.
type RetainedWindow struct {
	Oldest       time.Time
	Newest       time.Time
	Count        int64
	Bytes        int64
	Truncated    bool
	PurgedBefore time.Time
	PurgeReason  string
}

// Page is one bounded answer.
type Page struct {
	Items          []audit.Event
	NextCursor     string
	RetainedWindow map[audit.Category]RetainedWindow
	LoggingHealth  Health
}

func (filter Filter) toAudit() (audit.Filter, error) {
	var parsed audit.Filter
	if filter.Category != "" {
		category := audit.Category(filter.Category)
		if category != audit.CategoryAudit && category != audit.CategoryEvent && category != audit.CategoryDebug {
			return parsed, fmt.Errorf("%w: unknown category %q", ErrInvalidFilter, filter.Category)
		}
		parsed.Category = category
	}
	if filter.Actor != "" {
		actor, err := domain.ParseActorID(filter.Actor)
		if err != nil {
			return parsed, fmt.Errorf("%w: actor must be an identity", ErrInvalidFilter)
		}
		parsed.ActorID = actor
	}
	if filter.Severity != "" {
		parsed.Severity = audit.Severity(filter.Severity)
	}
	if filter.Outcome != "" {
		parsed.Outcome = audit.Outcome(filter.Outcome)
	}
	parsed.Component, parsed.SubjectID, parsed.OperationID, parsed.Action = filter.Component, filter.Target, filter.Operation, filter.Action
	var err error
	if filter.From != "" {
		if parsed.From, err = time.Parse(time.RFC3339, filter.From); err != nil {
			return parsed, fmt.Errorf("%w: from must be RFC 3339", ErrInvalidFilter)
		}
	}
	if filter.To != "" {
		if parsed.To, err = time.Parse(time.RFC3339, filter.To); err != nil {
			return parsed, fmt.Errorf("%w: to must be RFC 3339", ErrInvalidFilter)
		}
	}
	if filter.Cursor != "" {
		after, err := strconv.ParseInt(filter.Cursor, 10, 64)
		if err != nil || after < 0 {
			return parsed, fmt.Errorf("%w: cursor is not one this appliance issued", ErrInvalidFilter)
		}
		parsed.AfterSequence = after
	}
	if filter.Limit < 0 || filter.Limit > MaximumPageSize {
		return parsed, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidFilter, MaximumPageSize)
	}
	parsed.Limit = filter.Limit
	if parsed.Limit == 0 {
		parsed.Limit = DefaultPageSize
	}
	if err := parsed.Validate(); err != nil {
		return parsed, fmt.Errorf("%w: %v", ErrInvalidFilter, err)
	}
	return parsed, nil
}

// Query answers one page.
func (service *Service) Query(ctx context.Context, filter Filter) (Page, error) {
	parsed, err := filter.toAudit()
	if err != nil {
		return Page{}, err
	}
	// One more than the page says whether there is a next page, without a
	// second count query.
	parsed.Limit++
	events, err := service.options.Store.List(ctx, parsed)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: events}
	if len(events) == parsed.Limit {
		page.Items = events[:parsed.Limit-1]
		page.NextCursor = strconv.FormatInt(page.Items[len(page.Items)-1].Sequence, 10)
	}
	if page.Items == nil {
		page.Items = []audit.Event{}
	}
	if page.RetainedWindow, err = service.retainedWindow(ctx); err != nil {
		return Page{}, err
	}
	page.LoggingHealth = service.health()
	return page, nil
}

func (service *Service) health() Health {
	if service.options.Monitor == nil {
		return Health{AuditWritable: true}
	}
	return service.options.Monitor.Health()
}

func (service *Service) retainedWindow(ctx context.Context) (map[audit.Category]RetainedWindow, error) {
	windows, err := service.options.Store.RetainedWindow(ctx)
	if err != nil {
		return nil, err
	}
	watermarks, err := service.options.Store.Watermarks(ctx)
	if err != nil {
		return nil, err
	}
	result := map[audit.Category]RetainedWindow{}
	for _, category := range categories {
		window := RetainedWindow{Oldest: windows[category].Oldest, Newest: windows[category].Newest, Count: windows[category].Count, Bytes: windows[category].Bytes}
		if watermark, ok := watermarks[category]; ok {
			window.Truncated, window.PurgedBefore, window.PurgeReason = true, watermark.PurgedBefore, watermark.Reason
		}
		result[category] = window
	}
	return result, nil
}

// Policy reads the current policy, the default when none was saved.
func (service *Service) Policy(ctx context.Context) (Policy, error) {
	saved, err := service.options.Store.LoadPolicy(ctx)
	if err != nil {
		return Policy{}, err
	}
	if saved == nil {
		return DefaultPolicy(), nil
	}
	if err := saved.Validate(); err != nil {
		return DefaultPolicy(), nil
	}
	return *saved, nil
}

// SavePolicy validates and stores a policy.
func (service *Service) SavePolicy(ctx context.Context, policy Policy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	return service.options.Store.SavePolicy(ctx, policy, service.options.Now().UTC())
}

// ExportSummary is what an export produced.
type ExportSummary struct {
	Rows      int
	Bytes     int64
	Truncated bool
}

// exportHeader is the first line of every export.
type exportHeader struct {
	SchemaVersion  string                        `json:"schema_version"`
	ApplianceID    string                        `json:"appliance_id"`
	ExportedAt     time.Time                     `json:"exported_at"`
	Filter         Filter                        `json:"filter"`
	TimeRange      map[string]string             `json:"time_range"`
	RetainedWindow map[string]exportWindow       `json:"retained_window"`
	Redaction      string                        `json:"redaction"`
	Limits         map[string]int64              `json:"limits"`
	Columns        []string                      `json:"columns,omitempty"`
	_              struct{ _ map[string]string } // keeps the shape closed to accidental extension
}

type exportWindow struct {
	Oldest       string `json:"oldest,omitempty"`
	Newest       string `json:"newest,omitempty"`
	Count        int64  `json:"count"`
	Truncated    bool   `json:"truncated"`
	PurgedBefore string `json:"purged_before,omitempty"`
	PurgeReason  string `json:"purge_reason,omitempty"`
}

var exportColumns = []string{"sequence", "id", "created_at", "category", "component", "action", "outcome", "severity", "actor_kind", "actor_id", "subject_type", "subject_id", "target_label", "operation_id", "appliance_id", "boot_id", "time_source", "details"}

type countingWriter struct {
	writer io.Writer
	bytes  int64
}

func (counter *countingWriter) Write(data []byte) (int, error) {
	written, err := counter.writer.Write(data)
	counter.bytes += int64(written)
	return written, err
}

// Export streams a bounded, filtered selection.
func (service *Service) Export(ctx context.Context, filter Filter, format Format, destination io.Writer) (ExportSummary, error) {
	if format != FormatJSONL && format != FormatCSV {
		return ExportSummary{}, fmt.Errorf("%w: format must be jsonl or csv", ErrInvalidFilter)
	}
	filter.Cursor, filter.Limit = "", 0
	parsed, err := filter.toAudit()
	if err != nil {
		return ExportSummary{}, err
	}
	windows, err := service.retainedWindow(ctx)
	if err != nil {
		return ExportSummary{}, err
	}
	now := service.options.Now().UTC()
	header := exportHeader{
		SchemaVersion: SchemaVersion, ApplianceID: service.options.ApplianceID, ExportedAt: now, Filter: filter,
		TimeRange:      map[string]string{"from": filter.From, "to": firstNonEmpty(filter.To, now.Format(time.RFC3339))},
		RetainedWindow: map[string]exportWindow{}, Redaction: Redaction,
		Limits: map[string]int64{"rows": int64(service.options.ExportRowLimit), "bytes": service.options.ExportByteLimit},
	}
	for category, window := range windows {
		encoded := exportWindow{Count: window.Count, Truncated: window.Truncated, PurgeReason: window.PurgeReason}
		if !window.Oldest.IsZero() {
			encoded.Oldest, encoded.Newest = window.Oldest.UTC().Format(time.RFC3339Nano), window.Newest.UTC().Format(time.RFC3339Nano)
		}
		if !window.PurgedBefore.IsZero() {
			encoded.PurgedBefore = window.PurgedBefore.UTC().Format(time.RFC3339Nano)
		}
		header.RetainedWindow[string(category)] = encoded
	}
	counter := &countingWriter{writer: destination}
	var emit func(audit.Event) error
	var finish func(summary ExportSummary) error
	switch format {
	case FormatJSONL:
		encoder := json.NewEncoder(counter)
		if err := encoder.Encode(header); err != nil {
			return ExportSummary{}, err
		}
		emit = func(event audit.Event) error { return encoder.Encode(exportRow(event)) }
		finish = func(summary ExportSummary) error {
			return encoder.Encode(map[string]any{"trailer": true, "rows": summary.Rows, "truncated": summary.Truncated})
		}
	case FormatCSV:
		if _, err := fmt.Fprintf(counter, "# schema_version=%s\n# appliance_id=%s\n# exported_at=%s\n# time_range=%s..%s\n# redaction=%s\n",
			SchemaVersion, service.options.ApplianceID, now.Format(time.RFC3339), header.TimeRange["from"], header.TimeRange["to"], Redaction); err != nil {
			return ExportSummary{}, err
		}
		retained, _ := json.Marshal(header.RetainedWindow)
		if _, err := fmt.Fprintf(counter, "# retained_window=%s\n", neutralizeCell(string(retained))); err != nil {
			return ExportSummary{}, err
		}
		writer := csv.NewWriter(counter)
		if err := writer.Write(exportColumns); err != nil {
			return ExportSummary{}, err
		}
		emit = func(event audit.Event) error {
			row := exportRow(event)
			cells := make([]string, 0, len(exportColumns))
			for _, column := range exportColumns {
				cells = append(cells, neutralizeCell(fmt.Sprint(row[column])))
			}
			if err := writer.Write(cells); err != nil {
				return err
			}
			writer.Flush()
			return writer.Error()
		}
		finish = func(summary ExportSummary) error {
			writer.Flush()
			_, err := fmt.Fprintf(counter, "# rows=%d truncated=%t\n", summary.Rows, summary.Truncated)
			return err
		}
	}
	var summary ExportSummary
	after := parsed.AfterSequence
	for {
		batch := parsed
		batch.AfterSequence, batch.Limit = after, exportBatch
		events, err := service.options.Store.List(ctx, batch)
		if err != nil {
			return summary, err
		}
		for _, event := range events {
			if summary.Rows >= service.options.ExportRowLimit || counter.bytes >= service.options.ExportByteLimit {
				summary.Truncated = true
				break
			}
			if err := emit(event); err != nil {
				return summary, err
			}
			summary.Rows++
			after = event.Sequence
		}
		if summary.Truncated || len(events) < exportBatch {
			break
		}
	}
	if err := finish(summary); err != nil {
		return summary, err
	}
	summary.Bytes = counter.bytes
	return summary, nil
}

// exportRow is the flat, safe shape of one event. The payload is passed on as
// the JSON it already is: it was written under the same redaction rule.
func exportRow(event audit.Event) map[string]any {
	details := "{}"
	if len(event.Payload) > 0 {
		details = string(event.Payload)
	}
	return map[string]any{
		"sequence": event.Sequence, "id": event.ID, "created_at": event.CreatedAt.UTC().Format(time.RFC3339Nano),
		"category": string(event.Category), "component": event.Component, "action": event.Action,
		"outcome": string(event.Outcome), "severity": string(event.Severity),
		"actor_kind": string(event.ActorKind), "actor_id": event.ActorID.String(),
		"subject_type": event.SubjectType, "subject_id": event.SubjectID, "target_label": event.TargetLabel,
		"operation_id": event.OperationID, "appliance_id": event.ApplianceID, "boot_id": event.BootID,
		"time_source": string(event.TimeSource), "details": json.RawMessage(details),
	}
}

// neutralizeCell keeps a spreadsheet from interpreting a cell as a formula.
func neutralizeCell(cell string) string {
	if cell == "" {
		return cell
	}
	switch cell[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + cell
	}
	return cell
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// sanitizeMessage bounds and flattens an error for a health answer.
func sanitizeMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, message)
	if len(message) > 200 {
		message = message[:200]
	}
	return message
}
