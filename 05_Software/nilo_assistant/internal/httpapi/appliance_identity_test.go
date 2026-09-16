package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/appliance"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

type cancellingIdentifyService struct{ cancel context.CancelFunc }

func (service cancellingIdentifyService) Identify(context.Context) (network.IdentifyResult, error) {
	service.cancel()
	return network.IdentifyResult{Status: network.IdentifyAccepted, ActiveUntil: time.Now().UTC().Add(10 * time.Second)}, nil
}

type contextCheckingIdentifyOperations struct {
	markContextErr  error
	flushContextErr error
}

func (*contextCheckingIdentifyOperations) Begin(context.Context, string, string, time.Time) (bool, error) {
	return true, nil
}
func (operations *contextCheckingIdentifyOperations) MarkActed(ctx context.Context, _ string, _ bool, _ string, _ audit.Event, _ time.Time) error {
	operations.markContextErr = ctx.Err()
	return operations.markContextErr
}
func (operations *contextCheckingIdentifyOperations) Flush(ctx context.Context, _ string) error {
	operations.flushContextErr = ctx.Err()
	return operations.flushContextErr
}

type identifyAuditComposerStub struct{}

func (identifyAuditComposerStub) Record(context.Context, audit.Record) error { return nil }
func (identifyAuditComposerStub) Compose(audit.Record) (audit.Event, error)   { return audit.Event{}, nil }

func TestIdentifyPersistsPostActionStateOutsideCancelledRequest(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	requestContext = context.WithValue(requestContext, actorContextKey{}, identity.Actor{
		User: auth.User{ID: "0199a2d0-1234-7abc-8def-0123456789ab"}, Credential: identity.CredentialSession,
	})
	operations := &contextCheckingIdentifyOperations{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appliance/identify", nil).WithContext(requestContext)
	response := httptest.NewRecorder()
	handleIdentify(cancellingIdentifyService{cancel: cancel}, identifyAuditComposerStub{}, operations).ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || operations.markContextErr != nil || operations.flushContextErr != nil {
		t.Fatalf("status=%d mark=%v flush=%v body=%s", response.Code, operations.markContextErr, operations.flushContextErr, response.Body.String())
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
