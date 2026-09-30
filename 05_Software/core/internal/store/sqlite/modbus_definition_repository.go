package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"nilo.local/nilo/internal/modbus"
)

func definitionDocument(ctx context.Context, q modbusQuery, table, id string) ([]byte, error) {
	var raw string
	err := q.QueryRowContext(ctx, "SELECT document FROM "+table+" WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrModbusNotFound
	}
	return []byte(raw), err
}

func (r *ModbusRepository) Map(ctx context.Context, id string) (modbus.Map, error) {
	var m modbus.Map
	raw, err := definitionDocument(ctx, r.database.SQL(), "modbus_maps", id)
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(raw, &m)
	return m, err
}

// MapRevision reads an immutable published revision used by saved workspaces.
func (r *ModbusRepository) MapRevision(ctx context.Context, id string, revision int64) (modbus.Map, error) {
	var item modbus.Map
	var raw string
	err := r.database.SQL().QueryRowContext(ctx, "SELECT document FROM modbus_map_revisions WHERE id=? AND revision=?", id, revision).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrModbusNotFound
	}
	if err != nil {
		return item, err
	}
	err = json.Unmarshal([]byte(raw), &item)
	return item, err
}

func (r *ModbusRepository) Maps(ctx context.Context) ([]modbus.Map, error) {
	rows, err := r.database.SQL().QueryContext(ctx, "SELECT document FROM modbus_maps ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []modbus.Map{}
	for rows.Next() {
		var raw string
		var m modbus.Map
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *ModbusRepository) Workspace(ctx context.Context, id string) (modbus.Workspace, error) {
	var w modbus.Workspace
	raw, err := definitionDocument(ctx, r.database.SQL(), "modbus_workspaces", id)
	if err != nil {
		return w, err
	}
	err = json.Unmarshal(raw, &w)
	return w, err
}

func (r *ModbusRepository) WorkspaceRevision(ctx context.Context, id string, revision int64) (modbus.Workspace, error) {
	var item modbus.Workspace
	var raw string
	err := r.database.SQL().QueryRowContext(ctx, "SELECT document FROM modbus_workspace_revisions WHERE id=? AND revision=?", id, revision).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrModbusNotFound
	}
	if err != nil {
		return item, err
	}
	err = json.Unmarshal([]byte(raw), &item)
	return item, err
}

func (r *ModbusRepository) Workspaces(ctx context.Context) ([]modbus.Workspace, error) {
	rows, err := r.database.SQL().QueryContext(ctx, "SELECT document FROM modbus_workspaces ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []modbus.Workspace{}
	for rows.Next() {
		var raw string
		var w modbus.Workspace
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func saveDefinition(ctx context.Context, tx *sql.Tx, table, id, owner, endpoint string, expected int64, document any) error {
	raw, err := json.Marshal(document)
	if err != nil {
		return ErrModbusInvalid
	}
	if expected == 0 {
		query := "INSERT INTO " + table + "(id,revision,owner_id,document) VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING"
		args := []any{id, 1, owner, string(raw)}
		if table == "modbus_workspaces" {
			query = "INSERT INTO modbus_workspaces(id,revision,owner_id,endpoint_id,document) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING"
			args = []any{id, 1, owner, endpoint, string(raw)}
		}
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrModbusConflict
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO "+table[:len(table)-1]+"_revisions(id,revision,document) VALUES(?,?,?)", id, 1, string(raw))
		return err
	}
	query := "UPDATE " + table + " SET revision=?,document=? WHERE id=? AND revision=?"
	args := []any{expected + 1, string(raw), id, expected}
	if table == "modbus_workspaces" {
		query = "UPDATE modbus_workspaces SET revision=?,endpoint_id=?,document=? WHERE id=? AND revision=?"
		args = []any{expected + 1, endpoint, string(raw), id, expected}
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrModbusConflict
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO "+table[:len(table)-1]+"_revisions(id,revision,document) VALUES(?,?,?)", id, expected+1, string(raw))
	return err
}

func (r *ModbusRepository) PutMap(ctx context.Context, expected int64, m modbus.Map) (modbus.Map, error) {
	rev, err := nextModbusRevision(expected)
	if err != nil {
		return modbus.Map{}, err
	}
	tx, err := r.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return modbus.Map{}, err
	}
	defer tx.Rollback()
	if expected > 0 {
		raw, readErr := definitionDocument(ctx, tx, "modbus_maps", m.ID)
		if readErr != nil {
			if errors.Is(readErr, ErrModbusNotFound) {
				return modbus.Map{}, ErrModbusConflict
			}
			return modbus.Map{}, readErr
		}
		var old modbus.Map
		if json.Unmarshal(raw, &old) != nil {
			return modbus.Map{}, ErrModbusInvalid
		}
		m.OwnerID = old.OwnerID
	}
	m.Revision = rev
	m, err = m.Canonical()
	if err != nil {
		return modbus.Map{}, ErrModbusInvalid
	}
	if err = saveDefinition(ctx, tx, "modbus_maps", m.ID, m.OwnerID, "", expected, m); err != nil {
		return modbus.Map{}, err
	}
	return m, tx.Commit()
}

func (r *ModbusRepository) PutWorkspace(ctx context.Context, expected int64, w modbus.Workspace) (modbus.Workspace, error) {
	rev, err := nextModbusRevision(expected)
	if err != nil {
		return modbus.Workspace{}, err
	}
	tx, err := r.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return modbus.Workspace{}, err
	}
	defer tx.Rollback()
	if expected > 0 {
		raw, readErr := definitionDocument(ctx, tx, "modbus_workspaces", w.ID)
		if readErr != nil {
			if errors.Is(readErr, ErrModbusNotFound) {
				return modbus.Workspace{}, ErrModbusConflict
			}
			return modbus.Workspace{}, readErr
		}
		var old modbus.Workspace
		if json.Unmarshal(raw, &old) != nil {
			return modbus.Workspace{}, ErrModbusInvalid
		}
		w.OwnerID = old.OwnerID
	}
	w.Revision = rev
	w, err = w.Canonical()
	if err != nil {
		return modbus.Workspace{}, ErrModbusInvalid
	}
	if err = saveDefinition(ctx, tx, "modbus_workspaces", w.ID, w.OwnerID, w.EndpointID, expected, w); err != nil {
		return modbus.Workspace{}, err
	}
	return w, tx.Commit()
}
