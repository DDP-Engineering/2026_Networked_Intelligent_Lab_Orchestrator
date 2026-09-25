package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"nilo.local/nilo/internal/activity"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
)

// The shared history, its export, and its retention policy. Reads answer from
// the activity service; every change the boundary decides on - an
// export, a policy change, a refusal - is itself recorded.

// accessGuard is authorize with a journal: a request that carries an identity
// but lacks the permission is refused and recorded, because an access denial
// is an accountable outcome. An unauthenticated request is refused without a
// record, so an unauthenticated flood cannot fill the store.
type accessGuard struct{ audit AuditJournal }

func (guard accessGuard) authorize(permission auth.Permission, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		actor, ok := actorFrom(request.Context())
		if !ok {
			writeUnauthorized(response)
			return
		}
		if !actor.Has(permission) {
			guard.denied(request, actor, permission)
			writeForbidden(response)
			return
		}
		next(response, request)
	})
}

func (guard accessGuard) denied(request *http.Request, actor identity.Actor, permission auth.Permission) {
	if guard.audit == nil {
		return
	}
	record := audit.Record{
		Category: audit.CategoryAudit, Component: "identity", Action: "identity.access.denied",
		SubjectType: "permission", SubjectID: string(permission), TargetLabel: request.Method + " " + request.URL.Path,
		Outcome: audit.OutcomeDenied, Severity: audit.SeverityWarning,
		Details: map[string]string{"method": request.Method, "path": request.URL.Path, "permission": string(permission), "credential": string(actor.Credential)},
	}
	record.ActorKind, record.ActorID = actorIdentity(actor)
	_ = guard.audit.Record(request.Context(), record)
}

// actorIdentity is the audit view of the request's actor.
func actorIdentity(actor identity.Actor) (audit.ActorKind, domain.ActorID) {
	kind := audit.ActorKindUser
	if actor.Credential == identity.CredentialToken {
		kind = audit.ActorKindToken
	}
	id, err := domain.ParseActorID(actor.User.ID)
	if err != nil {
		return audit.ActorKindAnonymous, domain.ActorID{}
	}
	return kind, id
}

type eventPayload struct {
	ID          string          `json:"id"`
	Sequence    int64           `json:"sequence"`
	CreatedAt   time.Time       `json:"created_at"`
	Category    string          `json:"category"`
	Component   string          `json:"component"`
	Action      string          `json:"action"`
	Outcome     string          `json:"outcome"`
	Severity    string          `json:"severity"`
	ActorKind   string          `json:"actor_kind"`
	ActorID     string          `json:"actor_id"`
	SubjectType string          `json:"subject_type"`
	SubjectID   string          `json:"subject_id"`
	TargetLabel string          `json:"target_label"`
	OperationID string          `json:"operation_id"`
	ApplianceID string          `json:"appliance_id"`
	BootID      string          `json:"boot_id"`
	TimeSource  string          `json:"time_source"`
	Details     json.RawMessage `json:"details"`
}

func eventPayloadOf(event audit.Event) eventPayload {
	details := json.RawMessage("{}")
	if len(event.Payload) > 0 {
		details = event.Payload
	}
	return eventPayload{
		ID: event.ID, Sequence: event.Sequence, CreatedAt: event.CreatedAt.UTC(), Category: string(event.Category),
		Component: event.Component, Action: event.Action, Outcome: string(event.Outcome), Severity: string(event.Severity),
		ActorKind: string(event.ActorKind), ActorID: event.ActorID.String(), SubjectType: event.SubjectType,
		SubjectID: event.SubjectID, TargetLabel: event.TargetLabel, OperationID: event.OperationID,
		ApplianceID: event.ApplianceID, BootID: event.BootID, TimeSource: string(event.TimeSource), Details: details,
	}
}

type retainedWindowPayload struct {
	Oldest       *time.Time `json:"oldest,omitempty"`
	Newest       *time.Time `json:"newest,omitempty"`
	Count        int64      `json:"count"`
	Bytes        int64      `json:"bytes"`
	Truncated    bool       `json:"truncated"`
	PurgedBefore *time.Time `json:"purged_before,omitempty"`
	PurgeReason  string     `json:"purge_reason,omitempty"`
}

func retainedWindowsOf(windows map[audit.Category]activity.RetainedWindow) map[string]retainedWindowPayload {
	payload := map[string]retainedWindowPayload{}
	for category, window := range windows {
		encoded := retainedWindowPayload{Count: window.Count, Bytes: window.Bytes, Truncated: window.Truncated, PurgeReason: window.PurgeReason}
		if !window.Oldest.IsZero() {
			oldest, newest := window.Oldest.UTC(), window.Newest.UTC()
			encoded.Oldest, encoded.Newest = &oldest, &newest
		}
		if !window.PurgedBefore.IsZero() {
			purged := window.PurgedBefore.UTC()
			encoded.PurgedBefore = &purged
		}
		payload[string(category)] = encoded
	}
	return payload
}

type loggingHealthPayload struct {
	AuditWritable bool       `json:"audit_writable"`
	Failures      int64      `json:"failures"`
	LastFailure   string     `json:"last_failure,omitempty"`
	LastFailureAt *time.Time `json:"last_failure_at,omitempty"`
}

func loggingHealthOf(health activity.Health) loggingHealthPayload {
	payload := loggingHealthPayload{AuditWritable: health.AuditWritable, Failures: health.Failures, LastFailure: health.LastFailure}
	if !health.LastFailureAt.IsZero() {
		at := health.LastFailureAt.UTC()
		payload.LastFailureAt = &at
	}
	return payload
}

func activityFilterOf(request *http.Request) (activity.Filter, error) {
	query := request.URL.Query()
	filter := activity.Filter{
		Category: query.Get("category"), Component: query.Get("component"), Target: query.Get("target"),
		Actor: query.Get("actor"), Severity: query.Get("severity"), Outcome: query.Get("outcome"),
		Operation: query.Get("operation"), Action: query.Get("action"), From: query.Get("from"), To: query.Get("to"),
		Cursor: query.Get("cursor"), Sort: query.Get("sort"), Order: query.Get("order"),
	}
	if text := query.Get("limit"); text == "all" {
		filter.All = true
	} else if text != "" {
		limit, err := strconv.Atoi(text)
		if err != nil {
			return filter, fmt.Errorf("%w: limit", activity.ErrInvalidFilter)
		}
		filter.Limit = limit
	}
	if text := query.Get("page"); text != "" {
		page, err := strconv.Atoi(text)
		if err != nil {
			return filter, fmt.Errorf("%w: page", activity.ErrInvalidFilter)
		}
		filter.Page = page
	}
	return filter, nil
}

func writeActivityFailure(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, activity.ErrInvalidFilter):
		writeRefusal(response, http.StatusBadRequest, "activity_filter_rejected", err.Error())
	case errors.Is(err, activity.ErrInvalidPolicy):
		writeRefusal(response, http.StatusBadRequest, "activity_policy_rejected", err.Error())
	default:
		writeError(response, http.StatusServiceUnavailable, "activity_unavailable", "")
	}
}

func handleActivityQuery(service *activity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		filter, err := activityFilterOf(request)
		if err != nil {
			writeActivityFailure(response, err)
			return
		}
		page, err := service.Query(request.Context(), filter)
		if err != nil {
			writeActivityFailure(response, err)
			return
		}
		items := make([]eventPayload, 0, len(page.Items))
		for _, event := range page.Items {
			items = append(items, eventPayloadOf(event))
		}
		writeJSON(response, http.StatusOK, struct {
			APIVersion     string                           `json:"api_version"`
			SchemaVersion  string                           `json:"schema_version"`
			Items          []eventPayload                   `json:"items"`
			NextCursor     string                           `json:"next_cursor,omitempty"`
			Total          int64                            `json:"total"`
			RetainedWindow map[string]retainedWindowPayload `json:"retained_window"`
			LoggingHealth  loggingHealthPayload             `json:"logging_health"`
		}{apiVersion, activity.SchemaVersion, items, page.NextCursor, page.Total, retainedWindowsOf(page.RetainedWindow), loggingHealthOf(page.LoggingHealth)})
	}
}

// handleActivityExport streams the selection and records the export once it
// is done, with what it produced.
func handleActivityExport(service *activity.Service, journal AuditJournal) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		filter, err := activityFilterOf(request)
		if err != nil {
			writeActivityFailure(response, err)
			return
		}
		format := activity.Format(request.URL.Query().Get("format"))
		if format == "" {
			format = activity.FormatJSONL
		}
		contentType, extension := "application/x-ndjson", "jsonl"
		if format == activity.FormatCSV {
			contentType, extension = "text/csv; charset=utf-8", "csv"
		} else if format != activity.FormatJSONL {
			writeActivityFailure(response, fmt.Errorf("%w: format must be jsonl or csv", activity.ErrInvalidFilter))
			return
		}
		response.Header().Set("Content-Type", contentType)
		response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "nilo-activity-"+time.Now().UTC().Format("20060102T150405Z")+"."+extension))
		response.Header().Set("Cache-Control", "no-store")
		summary, err := service.Export(request.Context(), filter, format, response)
		if journal != nil {
			record := audit.Record{
				Category: audit.CategoryAudit, Component: "system", Action: "activity.exported",
				SubjectType: "activity", SubjectID: "export", Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo,
				Details: map[string]any{"format": string(format), "rows": summary.Rows, "bytes": summary.Bytes, "truncated": summary.Truncated, "filter": filter},
			}
			if err != nil {
				record.Outcome, record.Severity = audit.OutcomeFailed, audit.SeverityError
				record.Details.(map[string]any)["failure"] = err.Error()
			}
			if actor, ok := actorFrom(request.Context()); ok {
				record.ActorKind, record.ActorID = actorIdentity(actor)
			}
			_ = journal.Record(request.Context(), record)
		}
		if err != nil && summary.Rows == 0 {
			// Nothing was streamed yet, so the failure can still be an answer.
			writeActivityFailure(response, err)
		}
	}
}

type policyPayload struct {
	APIVersion    string           `json:"api_version"`
	RetentionDays map[string]int   `json:"retention_days"`
	QuotaBytes    map[string]int64 `json:"quota_bytes"`
}

func policyPayloadOf(policy activity.Policy) policyPayload {
	payload := policyPayload{APIVersion: apiVersion, RetentionDays: map[string]int{}, QuotaBytes: map[string]int64{}}
	for category, days := range policy.RetentionDays {
		payload.RetentionDays[string(category)] = days
	}
	for category, quota := range policy.QuotaBytes {
		payload.QuotaBytes[string(category)] = quota
	}
	return payload
}

func handleReadPolicy(service *activity.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		policy, err := service.Policy(request.Context())
		if err != nil {
			writeActivityFailure(response, err)
			return
		}
		writeJSON(response, http.StatusOK, policyPayloadOf(policy))
	}
}

func handleChangePolicy(service *activity.Service, journal AuditJournal) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		var body struct {
			RetentionDays map[string]int   `json:"retention_days"`
			QuotaBytes    map[string]int64 `json:"quota_bytes"`
		}
		if err := decodeOperationRequest(request, &body); err != nil {
			writeError(response, http.StatusBadRequest, "network_request_invalid", "")
			return
		}
		proposed := activity.Policy{RetentionDays: map[audit.Category]int{}, QuotaBytes: map[audit.Category]int64{}}
		for category, days := range body.RetentionDays {
			proposed.RetentionDays[audit.Category(category)] = days
		}
		for category, quota := range body.QuotaBytes {
			proposed.QuotaBytes[audit.Category(category)] = quota
		}
		before, err := service.Policy(request.Context())
		if err != nil {
			writeActivityFailure(response, err)
			return
		}
		if err := service.SavePolicy(request.Context(), proposed); err != nil {
			writeActivityFailure(response, err)
			return
		}
		if journal != nil {
			record := audit.Record{
				Category: audit.CategoryAudit, Component: "system", Action: "activity.retention.changed",
				SubjectType: "activity", SubjectID: "policy", Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo,
				Details: map[string]any{"before": policyPayloadOf(before), "after": policyPayloadOf(proposed)},
			}
			if actor, ok := actorFrom(request.Context()); ok {
				record.ActorKind, record.ActorID = actorIdentity(actor)
			}
			_ = journal.Record(request.Context(), record)
		}
		writeJSON(response, http.StatusOK, policyPayloadOf(proposed))
	}
}
