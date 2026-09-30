package bench

import (
	"context"
	"errors"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

func mayEditModbusDefinition(actor identity.Actor, owner string) bool {
	return actor.User.ID == owner || actor.Has(auth.PermissionPluginsManage)
}

// ModbusMaps returns shared metadata without activating any definition.
func (m *Manager) ModbusMaps(ctx context.Context, actor identity.Actor) ([]modbus.Map, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return nil, err
	}
	return sqlite.NewModbusRepository(m.options.Database).Maps(ctx)
}

func (m *Manager) ModbusMap(ctx context.Context, actor identity.Actor, id string) (modbus.Map, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return modbus.Map{}, err
	}
	out, err := sqlite.NewModbusRepository(m.options.Database).Map(ctx, id)
	return out, modbusDefinitionError(err)
}

func (m *Manager) ModbusMapRevision(ctx context.Context, actor identity.Actor, id string, revision int64) (modbus.Map, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return modbus.Map{}, err
	}
	if revision < 1 || revision > 2147483647 {
		return modbus.Map{}, ErrInvalid
	}
	item, err := sqlite.NewModbusRepository(m.options.Database).MapRevision(ctx, id, revision)
	return item, modbusDefinitionError(err)
}

// SaveModbusMap publishes one creator-owned revision independently of live plans.
func (m *Manager) SaveModbusMap(ctx context.Context, actor identity.Actor, expected int64, item modbus.Map) (modbus.Map, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return modbus.Map{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return modbus.Map{}, ErrUnavailable
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	old, err := r.Map(ctx, item.ID)
	if err != nil && !errors.Is(err, sqlite.ErrModbusNotFound) {
		return modbus.Map{}, err
	}
	if err == nil && !mayEditModbusDefinition(actor, old.OwnerID) {
		return modbus.Map{}, ErrForbidden
	}
	if expected < 0 || expected >= 2147483647 {
		return modbus.Map{}, ErrInvalid
	}
	if (err == nil && old.Revision != expected) || (errors.Is(err, sqlite.ErrModbusNotFound) && expected != 0) {
		return modbus.Map{}, ErrConflict
	}
	item.OwnerID = actor.User.ID
	if err == nil {
		item.OwnerID = old.OwnerID
	}
	item.Revision = expected + 1
	item, err = item.Canonical()
	if err != nil {
		return modbus.Map{}, ErrInvalid
	}
	if err = m.record(ctx, actor, "modbus.map_save_requested", item.ID, map[string]any{"revision": expected}); err != nil {
		return modbus.Map{}, err
	}
	item, err = r.PutMap(ctx, expected, item)
	if err != nil {
		return modbus.Map{}, modbusDefinitionError(err)
	}
	if err = m.record(ctx, actor, "modbus.map_saved", item.ID, map[string]any{"revision": item.Revision}); err != nil {
		return modbus.Map{}, err
	}
	return item, nil
}

func (m *Manager) DuplicateModbusMap(ctx context.Context, actor identity.Actor, sourceID, newID string) (modbus.Map, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return modbus.Map{}, err
	}
	source, err := m.ModbusMap(ctx, actor, sourceID)
	if err != nil {
		return modbus.Map{}, err
	}
	source.ID = newID
	source.OwnerID = actor.User.ID
	source.Revision = 1
	return m.SaveModbusMap(ctx, actor, 0, source)
}

// ModbusMapImport carries one bounded metadata document and publication intent.
type ModbusMapImport struct {
	Format           string `json:"format"`
	Mode             string `json:"mode"`
	TargetID         string `json:"target_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	AcceptConflicts  bool   `json:"accept_conflicts"`
	Document         string `json:"document"`
}

type ModbusMapImportPreview struct {
	Format    string            `json:"format"`
	Mode      string            `json:"mode"`
	TargetID  string            `json:"target_id"`
	CanApply  bool              `json:"can_apply"`
	RowErrors []modbus.RowError `json:"row_errors"`
	Conflicts []string          `json:"conflicts"`
	Candidate *modbus.Map       `json:"candidate,omitempty"`
}

// ExportModbusMap returns complete map metadata in JSON or safe CSV.
func (m *Manager) ExportModbusMap(ctx context.Context, actor identity.Actor, id, format string) (string, error) {
	item, err := m.ModbusMap(ctx, actor, id)
	if err != nil {
		return "", err
	}
	raw, err := modbus.ExportMap(item, format)
	if err != nil {
		return "", ErrInvalid
	}
	return string(raw), nil
}

// PreviewModbusMapImport checks all bounded errors without changing SQLite.
func (m *Manager) PreviewModbusMapImport(ctx context.Context, actor identity.Actor, input ModbusMapImport) (ModbusMapImportPreview, error) {
	out := ModbusMapImportPreview{Format: input.Format, Mode: input.Mode, TargetID: input.TargetID, RowErrors: []modbus.RowError{}, Conflicts: []string{}}
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return out, err
	}
	if input.Mode != "create" && input.Mode != "replace" && input.Mode != "merge" {
		return out, ErrInvalid
	}
	if input.ExpectedRevision < 0 || input.ExpectedRevision >= 2147483647 {
		return out, ErrInvalid
	}
	candidate, issues := modbus.ParseMapDocument(input.Format, []byte(input.Document))
	out.RowErrors = issues
	if len(issues) > 0 {
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
	existing, err := r.Map(ctx, input.TargetID)
	missing := errors.Is(err, sqlite.ErrModbusNotFound)
	if err != nil && !missing {
		return out, err
	}
	if input.Mode == "create" {
		if input.ExpectedRevision != 0 || !missing {
			out.Conflicts = append(out.Conflicts, "target_exists_or_revision")
		}
	} else {
		if missing {
			out.Conflicts = append(out.Conflicts, "target_missing")
		} else {
			if !mayEditModbusDefinition(actor, existing.OwnerID) {
				return out, ErrForbidden
			}
			if existing.Revision != input.ExpectedRevision {
				out.Conflicts = append(out.Conflicts, "revision")
			}
			candidate.OwnerID = existing.OwnerID
			if input.Mode == "merge" {
				byID := map[string]int{}
				for i, p := range existing.Points {
					byID[p.ID] = i
				}
				merged := append([]modbus.Point(nil), existing.Points...)
				for _, p := range candidate.Points {
					if index, found := byID[p.ID]; found {
						out.Conflicts = append(out.Conflicts, p.ID)
						merged[index] = p
					} else {
						merged = append(merged, p)
					}
				}
				candidate.Points = merged
			}
		}
	}
	canonical, canonErr := candidate.Canonical()
	if canonErr != nil {
		out.RowErrors = append(out.RowErrors, modbus.RowError{Row: 0, Field: "candidate", Code: "invalid"})
		return out, nil
	}
	out.Candidate = &canonical
	allowed := len(out.Conflicts) == 0
	if input.Mode == "merge" && input.AcceptConflicts && len(out.Conflicts) > 0 && !missing && existing.Revision == input.ExpectedRevision {
		allowed = true
	}
	out.CanApply = allowed
	return out, nil
}

func (m *Manager) ImportModbusMap(ctx context.Context, actor identity.Actor, input ModbusMapImport) (modbus.Map, error) {
	preview, err := m.PreviewModbusMapImport(ctx, actor, input)
	if err != nil {
		return modbus.Map{}, err
	}
	if !preview.CanApply || preview.Candidate == nil {
		if len(preview.Conflicts) > 0 {
			return modbus.Map{}, ErrConflict
		}
		return modbus.Map{}, ErrInvalid
	}
	return m.SaveModbusMap(ctx, actor, input.ExpectedRevision, *preview.Candidate)
}
