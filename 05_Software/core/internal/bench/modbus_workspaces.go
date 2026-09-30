package bench

import (
	"context"
	"errors"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

func (m *Manager) ModbusWorkspaces(ctx context.Context, actor identity.Actor) ([]modbus.Workspace, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return nil, err
	}
	return sqlite.NewModbusRepository(m.options.Database).Workspaces(ctx)
}

func (m *Manager) ModbusWorkspace(ctx context.Context, actor identity.Actor, id string) (modbus.Workspace, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return modbus.Workspace{}, err
	}
	w, err := sqlite.NewModbusRepository(m.options.Database).Workspace(ctx, id)
	if err != nil {
		return w, modbusDefinitionError(err)
	}
	if err = m.validateModbusWorkspaceRefs(ctx, w); err != nil {
		return modbus.Workspace{}, err
	}
	return w, nil
}

func (m *Manager) ModbusWorkspaceRevision(ctx context.Context, actor identity.Actor, id string, revision int64) (modbus.Workspace, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return modbus.Workspace{}, err
	}
	if revision < 1 || revision > 2147483647 {
		return modbus.Workspace{}, ErrInvalid
	}
	item, err := sqlite.NewModbusRepository(m.options.Database).WorkspaceRevision(ctx, id, revision)
	if err != nil {
		return item, modbusDefinitionError(err)
	}
	if err = m.validateModbusWorkspaceRefs(ctx, item); err != nil {
		return modbus.Workspace{}, err
	}
	return item, nil
}

func (m *Manager) validateModbusWorkspaceRefs(ctx context.Context, w modbus.Workspace) error {
	r := sqlite.NewModbusRepository(m.options.Database)
	endpoint, err := r.Endpoint(ctx, w.EndpointID)
	if err != nil {
		return modbusDefinitionError(err)
	}
	profile, err := r.ProfileRevision(ctx, w.ProfileRef.ID, w.ProfileRef.Revision)
	if err != nil {
		return modbusDefinitionError(err)
	}
	if profile.Revision != w.ProfileRef.Revision || profile.EndpointID != endpoint.ID {
		return ErrConflict
	}
	for _, ref := range w.MapRefs {
		item, readErr := r.MapRevision(ctx, ref.ID, ref.Revision)
		if readErr != nil {
			return modbusDefinitionError(readErr)
		}
		if item.Revision != ref.Revision {
			return ErrConflict
		}
	}
	if endpoint.Network != nil {
		for _, group := range w.PollGroups {
			if group.UnitID < endpoint.Network.UnitMin || group.UnitID > endpoint.Network.UnitMax {
				return ErrInvalid
			}
		}
	}
	return nil
}

// SaveModbusWorkspace publishes validated metadata without opening a session.
func (m *Manager) SaveModbusWorkspace(ctx context.Context, actor identity.Actor, expected int64, w modbus.Workspace) (modbus.Workspace, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return modbus.Workspace{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return modbus.Workspace{}, ErrUnavailable
	}
	r := sqlite.NewModbusRepository(m.options.Database)
	old, err := r.Workspace(ctx, w.ID)
	if err != nil && !errors.Is(err, sqlite.ErrModbusNotFound) {
		return modbus.Workspace{}, err
	}
	if err == nil && !mayEditModbusDefinition(actor, old.OwnerID) {
		return modbus.Workspace{}, ErrForbidden
	}
	if expected < 0 || expected >= 2147483647 {
		return modbus.Workspace{}, ErrInvalid
	}
	if (err == nil && old.Revision != expected) || (errors.Is(err, sqlite.ErrModbusNotFound) && expected != 0) {
		return modbus.Workspace{}, ErrConflict
	}
	w.OwnerID = actor.User.ID
	if err == nil {
		w.OwnerID = old.OwnerID
	}
	w.Revision = expected + 1
	w, err = w.Canonical()
	if err != nil {
		return modbus.Workspace{}, ErrInvalid
	}
	if err = m.validateModbusWorkspaceRefs(ctx, w); err != nil {
		return modbus.Workspace{}, err
	}
	if err = m.record(ctx, actor, "modbus.workspace_save_requested", w.ID, map[string]any{"revision": expected}); err != nil {
		return modbus.Workspace{}, err
	}
	w, err = r.PutWorkspace(ctx, expected, w)
	if err != nil {
		return modbus.Workspace{}, modbusDefinitionError(err)
	}
	if err = m.record(ctx, actor, "modbus.workspace_saved", w.ID, map[string]any{"revision": w.Revision}); err != nil {
		return modbus.Workspace{}, err
	}
	return w, nil
}

func (m *Manager) DuplicateModbusWorkspace(ctx context.Context, actor identity.Actor, sourceID, newID string) (modbus.Workspace, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return modbus.Workspace{}, err
	}
	source, err := m.ModbusWorkspace(ctx, actor, sourceID)
	if err != nil {
		return modbus.Workspace{}, err
	}
	source.ID = newID
	source.OwnerID = actor.User.ID
	source.Revision = 1
	return m.SaveModbusWorkspace(ctx, actor, 0, source)
}
