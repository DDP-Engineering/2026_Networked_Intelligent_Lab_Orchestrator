package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/power"
	"nilo.local/nilo/internal/simulation"
)

type capturingPowerController struct {
	events *[]string
	err    error
}

func (controller *capturingPowerController) Restart(context.Context) error {
	*controller.events = append(*controller.events, "power:restart")
	return controller.err
}

func (controller *capturingPowerController) PowerOff(context.Context) error {
	*controller.events = append(*controller.events, "power:power_off")
	return controller.err
}

type capturingPowerAudit struct {
	events  *[]string
	records []audit.Record
	err     error
}

func (journal *capturingPowerAudit) Record(_ context.Context, record audit.Record) error {
	journal.records = append(journal.records, record)
	*journal.events = append(*journal.events, "audit:"+record.TargetLabel)
	return journal.err
}

func powerRequest(f apiFixture, session loginResult, body, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/power", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session.sessionCookie)
	if csrf != "" {
		request.Header.Set(auth.CSRFHeaderName, csrf)
	}
	return f.do(request)
}

func TestOperatorAndAdministratorSessionsDispatchExactPowerActionsAfterAudit(t *testing.T) {
	for _, test := range []struct {
		name       string
		username   string
		credential string
		role       auth.Role
		action     string
	}{
		{name: "operator restarts", username: "operator", credential: operatorPassword, role: auth.RoleOperator, action: "restart"},
		{name: "administrator powers off", username: "admin", credential: administratorPassword, role: auth.RoleAdministrator, action: "power_off"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			controller := &capturingPowerController{events: &events}
			journal := &capturingPowerAudit{events: &events}
			f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
			if test.username != "admin" {
				f.createUser(t, test.username, test.role, test.credential)
			}
			f = f.withHandlerOptions(t, func(options *HandlerOptions) {
				options.Power = controller
				options.Audit = journal
			})
			session := f.login(t, test.username, test.credential)

			response := powerRequest(f, session, `{"action":"`+test.action+`"}`, session.csrfToken)

			if response.Code != http.StatusAccepted {
				t.Fatalf("power request = %d, want 202; body=%s", response.Code, response.Body.String())
			}
			wantEvents := []string{"audit:" + test.action, "power:" + test.action}
			if !reflect.DeepEqual(events, wantEvents) {
				t.Fatalf("event order = %v, want %v", events, wantEvents)
			}
			if len(journal.records) != 1 {
				t.Fatalf("audit records = %d, want 1", len(journal.records))
			}
			record := journal.records[0]
			if record.Action != "system.power.requested" || record.Component != "system" || record.SubjectType != "appliance" || record.SubjectID == "" || record.Outcome != audit.OutcomeRequested || record.ActorKind != audit.ActorKindUser {
				t.Fatalf("power audit = %+v", record)
			}
			if !strings.Contains(response.Body.String(), `"action":"`+test.action+`"`) || !strings.Contains(response.Body.String(), `"status":"accepted"`) {
				t.Fatalf("power response = %s", response.Body.String())
			}
		})
	}
}

func TestPowerRouteRefusesViewerBearerAndInvalidCSRFBeforeDispatch(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)
	administratorToken := f.issueToken(t, "admin", "admin-power", nil, nil)
	events := []string{}
	f = f.withHandlerOptions(t, func(options *HandlerOptions) {
		options.Power = &capturingPowerController{events: &events}
		options.Audit = &capturingPowerAudit{events: &events}
	})

	viewer := f.login(t, "viewer", viewerPassword)
	if response := powerRequest(f, viewer, `{"action":"restart"}`, viewer.csrfToken); response.Code != http.StatusForbidden {
		t.Fatalf("Viewer power request = %d, want 403", response.Code)
	}
	operator := f.login(t, "operator", operatorPassword)
	if response := powerRequest(f, operator, `{"action":"restart"}`, "wrong"); response.Code != http.StatusForbidden {
		t.Fatalf("bad-CSRF power request = %d, want 403", response.Code)
	}
	tokenRequest := httptest.NewRequest(http.MethodPost, "/api/v1/system/power", strings.NewReader(`{"action":"restart"}`))
	tokenRequest.Header.Set("Authorization", "Bearer "+administratorToken)
	if response := f.do(tokenRequest); response.Code != http.StatusForbidden {
		t.Fatalf("bearer power request = %d, want 403", response.Code)
	}
	for _, event := range events {
		if strings.HasPrefix(event, "power:") {
			t.Fatalf("refused request reached power controller: %v", events)
		}
	}
}

func TestPowerRouteRejectsMalformedActionsBeforeAuditOrDispatch(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	events := []string{}
	f = f.withHandlerOptions(t, func(options *HandlerOptions) {
		options.Power = &capturingPowerController{events: &events}
		options.Audit = &capturingPowerAudit{events: &events}
	})
	session := f.login(t, "admin", administratorPassword)

	for _, body := range []string{`{}`, `{"action":""}`, `{"action":"halt"}`, `{"action":"restart","delay":1}`, `not-json`} {
		if response := powerRequest(f, session, body, session.csrfToken); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_power_action") {
			t.Fatalf("body %q = %d %s, want invalid 400", body, response.Code, response.Body.String())
		}
	}
	if len(events) != 0 {
		t.Fatalf("malformed request reached audit or power: %v", events)
	}
}

func TestPowerRouteFailsClosedWhenAuditOrControllerFails(t *testing.T) {
	for _, test := range []struct {
		name       string
		auditError error
		powerError error
		wantEvents []string
		wantCode   string
	}{
		{name: "audit", auditError: errors.New("disk full"), wantEvents: []string{"audit:restart"}, wantCode: "power_audit_failed"},
		{name: "controller", powerError: errors.New("login1 unavailable"), wantEvents: []string{"audit:restart", "power:restart"}, wantCode: "power_action_failed"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
			f = f.withHandlerOptions(t, func(options *HandlerOptions) {
				options.Power = &capturingPowerController{events: &events, err: test.powerError}
				options.Audit = &capturingPowerAudit{events: &events, err: test.auditError}
			})
			session := f.login(t, "admin", administratorPassword)

			response := powerRequest(f, session, `{"action":"restart"}`, session.csrfToken)

			if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), test.wantCode) {
				t.Fatalf("failure = %d %s, want %s", response.Code, response.Body.String(), test.wantCode)
			}
			if !reflect.DeepEqual(events, test.wantEvents) {
				t.Fatalf("events = %v, want %v", events, test.wantEvents)
			}
		})
	}
}

func TestPowerRouteReportsUnavailableController(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f = f.withHandlerOptions(t, func(options *HandlerOptions) {
		options.Power = power.Unavailable{}
	})
	session := f.login(t, "admin", administratorPassword)
	response := powerRequest(f, session, `{"action":"restart"}`, session.csrfToken)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "power_control_unavailable") {
		t.Fatalf("simulation power = %d %s, want unavailable 503", response.Code, response.Body.String())
	}
}
