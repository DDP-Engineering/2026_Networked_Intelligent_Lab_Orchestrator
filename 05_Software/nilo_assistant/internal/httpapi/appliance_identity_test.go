package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

type cancellingIdentifyService struct{ cancel context.CancelFunc }

func (service cancellingIdentifyService) ExecuteIdentifyAction(context.Context, network.IdentifyActionRequest) (network.IdentifyActionResult, error) {
	service.cancel()
	return network.IdentifyActionResult{Supported: true, Status: network.IdentifyAccepted, ActiveUntil: time.Now().UTC().Add(10 * time.Second), Reannouncement: network.IdentifyReannouncementNotRequested}, nil
}
func (cancellingIdentifyService) AcknowledgeIdentifyAction(context.Context, string) error { return nil }

type contextCheckingIdentifyOperations struct {
	reserveContextErr error
	markContextErr    error
	flushContextErr   error
	markCalls         int
	failMark          bool
}

func (*contextCheckingIdentifyOperations) Begin(context.Context, string, string, time.Time, time.Time) (bool, error) {
	return true, nil
}
func (operations *contextCheckingIdentifyOperations) Reserve(ctx context.Context, _ string, _ audit.Event, _ time.Time) error {
	operations.reserveContextErr = ctx.Err()
	return operations.reserveContextErr
}
func (operations *contextCheckingIdentifyOperations) RecordOutcome(ctx context.Context, _ string, _ bool, _, _ string, _ audit.Outcome, _ time.Time) error {
	return ctx.Err()
}
func (operations *contextCheckingIdentifyOperations) MarkActed(ctx context.Context, _ string, _ time.Time) error {
	operations.markCalls++
	operations.markContextErr = ctx.Err()
	if operations.markContextErr == nil && operations.failMark {
		operations.failMark = false
		return errors.New("injected identify outcome persistence failure")
	}
	return operations.markContextErr
}
func (operations *contextCheckingIdentifyOperations) Flush(ctx context.Context, _ string) error {
	operations.flushContextErr = ctx.Err()
	return operations.flushContextErr
}

type identifyAuditComposerStub struct{}

func (identifyAuditComposerStub) Record(context.Context, audit.Record) error { return nil }
func (identifyAuditComposerStub) Compose(audit.Record) (audit.Event, error) {
	return audit.Event{}, nil
}

type failingIdentifyAuditComposer struct{}

func (failingIdentifyAuditComposer) Record(context.Context, audit.Record) error { return nil }
func (failingIdentifyAuditComposer) Compose(audit.Record) (audit.Event, error) {
	return audit.Event{}, errors.New("injected compose failure")
}

type countingIdentifyService struct{ calls int }

func (service *countingIdentifyService) ExecuteIdentifyAction(_ context.Context, request network.IdentifyActionRequest) (network.IdentifyActionResult, error) {
	service.calls++
	return network.IdentifyActionResult{Supported: true, Status: network.IdentifyAccepted, ActiveUntil: request.ActiveUntil, Reannouncement: network.IdentifyReannouncementNotRequested}, nil
}
func (*countingIdentifyService) AcknowledgeIdentifyAction(context.Context, string) error { return nil }

type receiptIdentifyService struct {
	calls   int
	acks    int
	ackErr  error
	request network.IdentifyActionRequest
	receipt network.IdentifyActionResult
}

func (service *receiptIdentifyService) ExecuteIdentifyAction(
	_ context.Context, request network.IdentifyActionRequest,
) (network.IdentifyActionResult, error) {
	if service.request.OperationID != "" {
		if service.request != request {
			return network.IdentifyActionResult{}, errors.New("conflicting identify action request")
		}
		return service.receipt, nil
	}
	service.calls++
	service.request = request
	service.receipt = network.IdentifyActionResult{
		Supported: true, Status: network.IdentifyAccepted, ActiveUntil: request.ActiveUntil,
		Reannouncement: network.IdentifyReannouncementNotRequested,
	}
	return service.receipt, nil
}

func (service *receiptIdentifyService) AcknowledgeIdentifyAction(_ context.Context, operationID string) error {
	if service.request.OperationID != operationID {
		return errors.New("acknowledged unexpected identify operation")
	}
	service.acks++
	return service.ackErr
}

type failingOnceIdentifyOperations struct {
	inner     *sqlite.IdentifyOperationRepository
	markCalls int
}

type persistentlyFailingOutcomeOperations struct {
	inner        *sqlite.IdentifyOperationRepository
	operationID  string
	outcomeCalls int
}

func (operations *persistentlyFailingOutcomeOperations) Begin(ctx context.Context, id, source string, activeUntil, now time.Time) (bool, error) {
	operations.operationID = id
	return operations.inner.Begin(ctx, id, source, activeUntil, now)
}
func (operations *persistentlyFailingOutcomeOperations) Reserve(ctx context.Context, id string, event audit.Event, now time.Time) error {
	return operations.inner.Reserve(ctx, id, event, now)
}
func (operations *persistentlyFailingOutcomeOperations) RecordOutcome(
	context.Context, string, bool, string, string, audit.Outcome, time.Time,
) error {
	operations.outcomeCalls++
	return errors.New("injected persistent identify outcome persistence failure")
}
func (operations *persistentlyFailingOutcomeOperations) MarkActed(ctx context.Context, id string, now time.Time) error {
	return operations.inner.MarkActed(ctx, id, now)
}
func (operations *persistentlyFailingOutcomeOperations) Flush(ctx context.Context, id string) error {
	return operations.inner.Flush(ctx, id)
}

func (operations *failingOnceIdentifyOperations) Begin(ctx context.Context, id, source string, activeUntil, now time.Time) (bool, error) {
	return operations.inner.Begin(ctx, id, source, activeUntil, now)
}
func (operations *failingOnceIdentifyOperations) Reserve(ctx context.Context, id string, event audit.Event, now time.Time) error {
	return operations.inner.Reserve(ctx, id, event, now)
}
func (operations *failingOnceIdentifyOperations) MarkActed(
	ctx context.Context, id string, now time.Time,
) error {
	operations.markCalls++
	if operations.markCalls == 1 {
		return errors.New("injected identify outcome persistence failure")
	}
	return operations.inner.MarkActed(ctx, id, now)
}
func (operations *failingOnceIdentifyOperations) RecordOutcome(
	ctx context.Context, id string, supported bool, reannouncement, status string, outcome audit.Outcome, now time.Time,
) error {
	return operations.inner.RecordOutcome(ctx, id, supported, reannouncement, status, outcome, now)
}
func (operations *failingOnceIdentifyOperations) Flush(ctx context.Context, id string) error {
	return operations.inner.Flush(ctx, id)
}

func TestIdentifyComposeFailurePreventsPhysicalAction(t *testing.T) {
	service := &receiptIdentifyService{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appliance/identify", nil)
	request = request.WithContext(context.WithValue(request.Context(), actorContextKey{}, identity.Actor{
		User: auth.User{ID: "0199a2d0-1234-7abc-8def-0123456789ab"}, Credential: identity.CredentialSession,
	}))
	response := httptest.NewRecorder()
	handleIdentify(service, failingIdentifyAuditComposer{}, &contextCheckingIdentifyOperations{}).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || service.calls != 0 {
		t.Fatalf("status=%d identify calls=%d body=%s", response.Code, service.calls, response.Body.String())
	}
}

func TestIdentifyPersistsPostActionStateOutsideCancelledRequest(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	requestContext = context.WithValue(requestContext, actorContextKey{}, identity.Actor{
		User: auth.User{ID: "0199a2d0-1234-7abc-8def-0123456789ab"}, Credential: identity.CredentialSession,
	})
	operations := &contextCheckingIdentifyOperations{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appliance/identify", nil).WithContext(requestContext)
	response := httptest.NewRecorder()
	handleIdentify(cancellingIdentifyService{cancel: cancel}, identifyAuditComposerStub{}, operations).ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || operations.reserveContextErr != nil || operations.markContextErr != nil || operations.flushContextErr != nil {
		t.Fatalf("status=%d reserve=%v mark=%v flush=%v body=%s", response.Code, operations.reserveContextErr, operations.markContextErr, operations.flushContextErr, response.Body.String())
	}
}

func TestIdentifyRetriesOutcomePersistenceWithoutRepeatingPhysicalAction(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	service := &receiptIdentifyService{}
	operations := &failingOnceIdentifyOperations{inner: sqlite.NewIdentifyOperationRepository(fixture.database)}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appliance/identify", nil)
	request = request.WithContext(context.WithValue(request.Context(), actorContextKey{}, identity.Actor{
		User: auth.User{ID: "0199a2d0-1234-7abc-8def-0123456789ab"}, Credential: identity.CredentialSession,
	}))
	response := httptest.NewRecorder()
	handleIdentify(service, fixture.options.Audit, operations).ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || service.calls != 1 || service.acks != 1 || operations.markCalls != 2 {
		t.Fatalf("status=%d identify calls=%d acks=%d mark calls=%d body=%s",
			response.Code, service.calls, service.acks, operations.markCalls, response.Body.String())
	}
	events, err := sqlite.NewAuditRepository(fixture.database).List(context.Background(), audit.Filter{
		Action: "appliance.identify", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Outcome != audit.OutcomeApplied ||
		string(events[0].Payload) != `{"reannouncement":"not_requested","source":"interface","status":"accepted"}` {
		t.Fatalf("persisted identify activity = %+v", events)
	}
}

func TestIdentifyPersistentOutcomeFailureIsRecoveredAfterRestartWithoutRepeatingPhysicalAction(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	service := &receiptIdentifyService{}
	repository := sqlite.NewIdentifyOperationRepository(fixture.database)
	operations := &persistentlyFailingOutcomeOperations{inner: repository}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appliance/identify", nil)
	request = request.WithContext(context.WithValue(request.Context(), actorContextKey{}, identity.Actor{
		User: auth.User{ID: "0199a2d0-1234-7abc-8def-0123456789ab"}, Credential: identity.CredentialSession,
	}))
	response := httptest.NewRecorder()
	handleIdentify(service, fixture.options.Audit, operations).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || service.calls != 1 || operations.outcomeCalls != 3 {
		t.Fatalf("status=%d identify calls=%d outcome calls=%d body=%s",
			response.Code, service.calls, operations.outcomeCalls, response.Body.String())
	}
	if service.acks != 0 {
		t.Fatalf("receipt acknowledged before durable Core outcome: %d", service.acks)
	}
	state, err := repository.State(context.Background(), operations.operationID)
	if err != nil || state != "reserved" {
		t.Fatalf("state after persistent outcome failure = %q, %v", state, err)
	}
	replayed, err := service.ExecuteIdentifyAction(context.Background(), service.request)
	if err != nil || service.calls != 1 || replayed != service.receipt {
		t.Fatalf("durable executor retry calls=%d result=%+v err=%v", service.calls, replayed, err)
	}
	if err := repository.RecordOutcome(context.Background(), operations.operationID, replayed.Supported,
		replayed.Reannouncement, string(replayed.Status), audit.OutcomeApplied, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := service.AcknowledgeIdentifyAction(context.Background(), operations.operationID); err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkActed(context.Background(), operations.operationID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := repository.Flush(context.Background(), operations.operationID); err != nil {
		t.Fatal(err)
	}
	events, err := sqlite.NewAuditRepository(fixture.database).List(context.Background(), audit.Filter{
		OperationID: operations.operationID, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Outcome != audit.OutcomeApplied ||
		string(events[0].Payload) != `{"reannouncement":"not_requested","source":"interface","status":"accepted"}` {
		t.Fatalf("recovered identify activity = %+v", events)
	}
}

func TestIdentifyAcknowledgementFailureLeavesDurableCoreOutcomeWithoutReplay(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	service := &receiptIdentifyService{ackErr: errors.New("injected helper acknowledgement failure")}
	repository := sqlite.NewIdentifyOperationRepository(fixture.database)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appliance/identify", nil)
	request = request.WithContext(context.WithValue(request.Context(), actorContextKey{}, identity.Actor{
		User: auth.User{ID: "0199a2d0-1234-7abc-8def-0123456789ab"}, Credential: identity.CredentialSession,
	}))
	response := httptest.NewRecorder()
	handleIdentify(service, fixture.options.Audit, repository).ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || service.calls != 1 || service.acks != 1 {
		t.Fatalf("status=%d calls=%d acks=%d body=%s", response.Code, service.calls, service.acks, response.Body.String())
	}
	state, err := repository.State(context.Background(), service.request.OperationID)
	if err != nil || state != "performed" {
		t.Fatalf("state after acknowledgement failure=%q err=%v", state, err)
	}
	service.ackErr = nil
	if err := service.AcknowledgeIdentifyAction(context.Background(), service.request.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkActed(context.Background(), service.request.OperationID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := repository.Flush(context.Background(), service.request.OperationID); err != nil {
		t.Fatal(err)
	}
	if service.calls != 1 {
		t.Fatalf("acknowledgement recovery replayed action: %d", service.calls)
	}
}

type apiIdentifyIndicator struct{ enabled []bool }

func (indicator *apiIdentifyIndicator) SetIdentifyIndicator(_ context.Context, enabled bool) error {
	indicator.enabled = append(indicator.enabled, enabled)
	return nil
}

type apiFriendlyNameService struct{ names []string }

func (service *apiFriendlyNameService) SetFriendlyName(_ context.Context, name string) (appliance.Identity, error) {
	service.names = append(service.names, name)
	return appliance.Identity{FriendlyName: name}, nil
}

type collidingFriendlyNameService struct{}

func (collidingFriendlyNameService) SetFriendlyName(_ context.Context, name string) (appliance.Identity, error) {
	return appliance.Identity{}, &appliance.FriendlyNameConflictError{
		Candidate: "untrusted", Interfaces: []string{"wlan0", "invalid/interface", "eth0", "wlan0", "lo"},
	}
}

func TestFriendlyNameCollisionResponseNamesOnlyCandidateAndInterfaces(t *testing.T) {
	request := httptest.NewRequest(http.MethodPut, "/api/v1/appliance/friendly-name", strings.NewReader(`{"friendly_name":"bench-west"}`))
	response := httptest.NewRecorder()
	handleFriendlyName(collidingFriendlyNameService{}, &ApplianceMetadata{}).ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Error   string                              `json:"error"`
		Details appliance.FriendlyNameConflictError `json:"details"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "friendly_name_conflict" || payload.Details.Candidate != "bench-west" ||
		!reflect.DeepEqual(payload.Details.Interfaces, []string{"eth0", "wlan0"}) {
		t.Fatalf("payload = %+v", payload)
	}
	for _, forbidden := range []string{"publish candidate", "Avahi", "helper"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response leaked internal collision detail %q: %s", forbidden, response.Body.String())
		}
	}
}

func TestApplianceIdentityLabelAndIdentifyRoutesAreProtectedAndSecretFree(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	indicator := &apiIdentifyIndicator{}
	naming := &apiFriendlyNameService{}
	identify, err := network.NewIdentifyService(network.IdentifyOptions{Indicator: indicator})
	if err != nil {
		t.Fatal(err)
	}
	metadata := &ApplianceMetadata{
		ID: "0199a2d0-1234-7abc-8def-0123456789ab", SerialName: "nilo-p2rjmzdkf1",
		FriendlyName: "bench-west", State: "commissioned", Version: "0.8.0",
		ManagementURL:       "https://nilo-p2rjmzdkf1.local:8443/",
		ManagementAddresses: []string{"192.168.1.42"}, ManagementMACs: []string{"02:00:00:00:00:02"},
	}
	fixture = fixture.withHandlerOptions(t, func(options *HandlerOptions) {
		options.Appliance = metadata
		options.Identify = identify
		options.IdentifyOperations = sqlite.NewIdentifyOperationRepository(fixture.database)
		options.Naming = naming
	})
	administrator := fixture.issueToken(t, "admin", "appliance", nil, nil)
	readOnly := fixture.issueToken(t, "admin", "appliance-read", []auth.Permission{auth.PermissionStatusRead}, nil)

	if response := fixture.get("/api/v1/appliance", administrator); response.Code != http.StatusOK ||
		strings.Contains(response.Body.String(), "secret") || !strings.Contains(response.Body.String(), metadata.SerialName) {
		t.Fatalf("appliance metadata response = %d %s", response.Code, response.Body.String())
	}
	if response := fixture.get("/api/v1/appliance/label.svg", administrator); response.Code != http.StatusOK ||
		response.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(response.Body.String(), "<svg") {
		t.Fatalf("label response = %d %s", response.Code, response.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appliance/identify", nil)
	request.Header.Set("Authorization", "Bearer "+readOnly)
	if response := fixture.do(request); response.Code != http.StatusForbidden {
		t.Fatalf("read-only identify status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/appliance/identify", nil)
	request.Header.Set("Authorization", "Bearer "+administrator)
	if response := fixture.do(request); response.Code != http.StatusForbidden {
		t.Fatalf("bearer Identify status = %d body=%s", response.Code, response.Body.String())
	}
	session := fixture.administratorSession(t)
	response := fixture.sessionSend(t, http.MethodPost, "/api/v1/appliance/identify", nil, session)
	if response.Code != http.StatusAccepted {
		t.Fatalf("administrator session Identify status = %d body=%s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result["supported"] != true || result["active_until"] == "" {
		t.Fatalf("identify response = %v, %v", result, err)
	}
	if len(indicator.enabled) != 1 || !indicator.enabled[0] {
		t.Fatalf("indicator calls = %v", indicator.enabled)
	}
	var operationState, outcome string
	if err := fixture.database.SQL().QueryRow(`
		SELECT operations.state, events.outcome
		FROM identify_operations operations
		JOIN audit_events events ON events.operation_id = operations.id
		WHERE operations.source = 'interface'
	`).Scan(&operationState, &outcome); err != nil || operationState != "settled" || outcome != "applied" {
		t.Fatalf("identify audit state=%q outcome=%q error=%v", operationState, outcome, err)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/v1/appliance/friendly-name", strings.NewReader(`{"friendly_name":"bench-east"}`))
	request.Header.Set("Authorization", "Bearer "+administrator)
	response = fixture.do(request)
	if response.Code != http.StatusOK || len(naming.names) != 1 || naming.names[0] != "bench-east" || metadata.FriendlyName != "bench-east" {
		t.Fatalf("friendly-name response = %d %s; calls=%v metadata=%+v", response.Code, response.Body.String(), naming.names, metadata)
	}
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/appliance/friendly-name", nil)
	request.Header.Set("Authorization", "Bearer "+administrator)
	response = fixture.do(request)
	if response.Code != http.StatusNoContent || len(naming.names) != 2 || naming.names[1] != "" {
		t.Fatalf("friendly-name DELETE = %d %s; calls=%v", response.Code, response.Body.String(), naming.names)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/v1/appliance/friendly-name", strings.NewReader(`{"friendly_name":"INVALID"}`))
	request.Header.Set("Authorization", "Bearer "+administrator)
	response = fixture.do(request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_friendly_name") {
		t.Fatalf("invalid friendly-name response = %d %s", response.Code, response.Body.String())
	}
}

func TestDiscoveryDescriptionIsMountedOutsideAuthentication(t *testing.T) {
	fixture := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	metadata := &ApplianceMetadata{
		ID: "0199a2d0-1234-7abc-8def-0123456789ab", SerialName: "nilo-p2rjmzdkf1",
		State: "provisioning", Version: "0.8.0", ManagementURL: "https://nilo-p2rjmzdkf1.local:8443/",
	}
	fixture = fixture.withHandlerOptions(t, func(options *HandlerOptions) {
		options.Appliance = metadata
	})
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/.well-known/nilo-device.xml", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "urn:schemas-upnp-org:device:Basic:1") {
		t.Fatalf("public description = %d %s", response.Code, response.Body.String())
	}
	metadata.replaceFriendlyName("bench-east")
	response = httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/.well-known/nilo-device.xml", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "bench-east") {
		t.Fatalf("public description retained startup metadata = %d %s", response.Code, response.Body.String())
	}
}
