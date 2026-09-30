package bench

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/store/sqlite"
)

var modbusID = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

// ModbusConnect pins both shared definitions in a finite approval job.
type ModbusConnect struct {
	EndpointRevision  int64             `json:"endpoint_revision"`
	ProfileID         string            `json:"profile_id"`
	ProfileRevision   int64             `json:"profile_revision"`
	WorkspaceID       string            `json:"workspace_id,omitempty"`
	WorkspaceRevision int64             `json:"workspace_revision,omitempty"`
	Groups            []ModbusPollGroup `json:"groups"`
	IdempotencyKey    string            `json:"idempotency_key"`
	TimeoutSeconds    int               `json:"timeout_seconds,omitempty"`
}

type modbusConnectParameters struct {
	EndpointRevision  int64             `json:"endpoint_revision"`
	ProfileID         string            `json:"profile_id"`
	ProfileRevision   int64             `json:"profile_revision"`
	WorkspaceID       string            `json:"workspace_id,omitempty"`
	WorkspaceRevision int64             `json:"workspace_revision,omitempty"`
	Groups            []ModbusPollGroup `json:"groups"`
	TestOnly          bool              `json:"test_only,omitempty"`
}

func modbusCanonicalSubmission(input Submission) (Submission, error) {
	var p modbusConnectParameters
	if input.Operation != "connect" || !modbusID.MatchString(input.DeviceID) || strict(input.Parameters, &p) != nil || p.EndpointRevision < 1 || p.ProfileRevision < 1 || !modbusID.MatchString(p.ProfileID) {
		return input, ErrInvalid
	}
	var err error
	p.Groups, err = canonicalModbusPollGroups(p.Groups)
	if err != nil {
		return input, err
	}
	if p.TestOnly && (len(p.Groups) != 0 || p.WorkspaceID != "" || p.WorkspaceRevision != 0) {
		return input, ErrInvalid
	}
	if (p.WorkspaceID == "") != (p.WorkspaceRevision == 0) || p.WorkspaceID != "" && (!modbusID.MatchString(p.WorkspaceID) || p.WorkspaceRevision < 1) {
		return input, ErrInvalid
	}
	input.Parameters, _ = json.Marshal(p)
	return input, nil
}

func modbusValidateSubmission(ctx context.Context, m *Manager, actor identity.Actor, _ Plugin, input Submission) error {
	if actor.SessionID == "" && actor.TokenID == "" {
		return ErrForbidden
	}
	var p modbusConnectParameters
	_ = json.Unmarshal(input.Parameters, &p)
	if p.TestOnly && !actor.Has(auth.PermissionPluginsManage) {
		return ErrForbidden
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	e, err := r.Endpoint(ctx, input.DeviceID)
	if err != nil {
		return modbusDefinitionError(err)
	}
	profile, err := r.Profile(ctx, p.ProfileID)
	if err != nil {
		return modbusDefinitionError(err)
	}
	if e.Revision != p.EndpointRevision || profile.Revision != p.ProfileRevision || profile.EndpointID != e.ID {
		return ErrConflict
	}
	if !e.Enabled || e.Readiness == "fault" || e.Transport == "rtu" && (e.Presence != "present" || e.Readiness != "ready") {
		return ErrUnavailable
	}
	for _, group := range p.Groups {
		for _, request := range group.Requests {
			if validateModbusTarget(request, e) != nil {
				return ErrInvalid
			}
		}
	}
	if p.WorkspaceID != "" {
		workspace, err := m.ModbusWorkspaceRevision(ctx, actor, p.WorkspaceID, p.WorkspaceRevision)
		if err != nil {
			return err
		}
		if workspace.EndpointID != e.ID || workspace.ProfileRef.ID != profile.ID || workspace.ProfileRef.Revision != profile.Revision {
			return ErrConflict
		}
		groups := make([]ModbusPollGroup, 0, len(workspace.PollGroups))
		for _, saved := range workspace.PollGroups {
			if !saved.Enabled {
				continue
			}
			groups = append(groups, ModbusPollGroup{ID: saved.ID, IntervalMS: int(saved.IntervalMS), TimeoutMS: saved.TimeoutMS, StopOnError: saved.StopOnError, Requests: []ModbusRequest{{Function: saved.Function, Unit: saved.UnitID, Address: saved.Address, Count: saved.Quantity, Values: []int{}}}})
		}
		if !reflect.DeepEqual(groups, p.Groups) {
			return ErrConflict
		}
	}
	if m.transportBusyLocked(e.ResourceKeys()) {
		return ErrBusy
	}
	return m.serialCredentialValid(ctx, actor.User.ID, string(actor.Credential), serialCredentialID(actor))
}
