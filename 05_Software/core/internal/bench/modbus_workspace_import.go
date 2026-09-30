package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

// ModbusWorkspaceImport is an inert JSON definition publication request.
type ModbusWorkspaceImport struct {
	Mode             string `json:"mode"`
	TargetID         string `json:"target_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Document         string `json:"document"`
}

type ModbusWorkspaceImportPreview struct {
	Mode      string            `json:"mode"`
	TargetID  string            `json:"target_id"`
	CanApply  bool              `json:"can_apply"`
	RowErrors []modbus.RowError `json:"row_errors"`
	Conflicts []string          `json:"conflicts"`
	Candidate *modbus.Workspace `json:"candidate,omitempty"`
}

func (m *Manager) PreviewModbusWorkspaceImport(ctx context.Context, actor identity.Actor, input ModbusWorkspaceImport) (ModbusWorkspaceImportPreview, error) {
	out := ModbusWorkspaceImportPreview{Mode: input.Mode, TargetID: input.TargetID, RowErrors: []modbus.RowError{}, Conflicts: []string{}}
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return out, err
	}
	if input.Mode != "create" && input.Mode != "replace" {
		return out, ErrInvalid
	}
	if input.ExpectedRevision < 0 || input.ExpectedRevision >= 2147483647 {
		return out, ErrInvalid
	}
	if len(input.Document) > modbus.MaxMapDocumentBytes {
		out.RowErrors = append(out.RowErrors, modbus.RowError{Row: 0, Field: "document", Code: "limit"})
		return out, nil
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(input.Document)))
	decoder.DisallowUnknownFields()
	var candidate modbus.Workspace
	if decoder.Decode(&candidate) != nil || decoder.Decode(new(any)) != io.EOF {
		out.RowErrors = append(out.RowErrors, modbus.RowError{Row: 0, Field: "document", Code: "invalid"})
		return out, nil
	}
	if input.TargetID == "" {
		input.TargetID = candidate.ID
		out.TargetID = candidate.ID
	}
	candidate.ID = input.TargetID
	candidate.OwnerID = actor.User.ID
	candidate.Revision = input.ExpectedRevision + 1
	r := sqlite.NewModbusRepository(m.options.Database)
	old, err := r.Workspace(ctx, candidate.ID)
	missing := errors.Is(err, sqlite.ErrModbusNotFound)
	if err != nil && !missing {
		return out, err
	}
	if input.Mode == "create" {
		if !missing || input.ExpectedRevision != 0 {
			out.Conflicts = append(out.Conflicts, "target_exists_or_revision")
		}
	} else {
		if missing {
			out.Conflicts = append(out.Conflicts, "target_missing")
		} else {
			if !mayEditModbusDefinition(actor, old.OwnerID) {
				return out, ErrForbidden
			}
			if old.Revision != input.ExpectedRevision {
				out.Conflicts = append(out.Conflicts, "revision")
			}
			candidate.OwnerID = old.OwnerID
		}
	}
	candidate, err = candidate.Canonical()
	if err != nil {
		out.RowErrors = append(out.RowErrors, modbus.RowError{Row: 0, Field: "workspace", Code: "invalid"})
		return out, nil
	}
	if err = m.validateModbusWorkspaceRefs(ctx, candidate); err != nil {
		out.RowErrors = append(out.RowErrors, modbus.RowError{Row: 0, Field: "references", Code: "invalid_or_missing"})
		return out, nil
	}
	out.Candidate = &candidate
	out.CanApply = len(out.Conflicts) == 0
	return out, nil
}

func (m *Manager) ImportModbusWorkspace(ctx context.Context, actor identity.Actor, input ModbusWorkspaceImport) (modbus.Workspace, error) {
	preview, err := m.PreviewModbusWorkspaceImport(ctx, actor, input)
	if err != nil {
		return modbus.Workspace{}, err
	}
	if !preview.CanApply || preview.Candidate == nil {
		if len(preview.Conflicts) > 0 {
			return modbus.Workspace{}, ErrConflict
		}
		return modbus.Workspace{}, ErrInvalid
	}
	return m.SaveModbusWorkspace(ctx, actor, input.ExpectedRevision, *preview.Candidate)
}
