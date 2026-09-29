package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"nilo.local/nilo/internal/modbus"
)

var (
	ErrModbusConflict = errors.New("modbus_revision_conflict")
	ErrModbusNotFound = errors.New("modbus_definition_not_found")
	ErrModbusInvalid  = errors.New("modbus_definition_invalid")
)

// ModbusRepository stores definitions with atomic expected-revision publication.
type ModbusRepository struct{ database *DB }

func NewModbusRepository(database *DB) *ModbusRepository {
	return &ModbusRepository{database: database}
}

type modbusQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readModbusEndpoint(ctx context.Context, q modbusQuery, id string) (modbus.Endpoint, error) {
	var e modbus.Endpoint
	var raw string
	var binding sql.NullString
	var bindingRevision sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT revision,managed_device_id,managed_revision,document FROM modbus_endpoints WHERE id=?`, id).Scan(&e.Revision, &binding, &bindingRevision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrModbusNotFound
	}
	if err != nil {
		return e, err
	}
	err = json.Unmarshal([]byte(raw), &e)
	e.Binding = binding.String
	e.BindingRevision = bindingRevision.Int64
	return e, err
}

func readModbusProfile(ctx context.Context, q modbusQuery, id string) (modbus.Profile, error) {
	var p modbus.Profile
	var raw string
	err := q.QueryRowContext(ctx, `SELECT document FROM modbus_profiles WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrModbusNotFound
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(raw), &p)
	return p, err
}

func (r *ModbusRepository) Endpoint(ctx context.Context, id string) (modbus.Endpoint, error) {
	return readModbusEndpoint(ctx, r.database.SQL(), id)
}

func (r *ModbusRepository) Profile(ctx context.Context, id string) (modbus.Profile, error) {
	return readModbusProfile(ctx, r.database.SQL(), id)
}

// Endpoints returns stable ID ordering, including disabled and absent definitions.
func (r *ModbusRepository) Endpoints(ctx context.Context) ([]modbus.Endpoint, error) {
	rows, err := r.database.SQL().QueryContext(ctx, `SELECT document,managed_device_id,managed_revision FROM modbus_endpoints ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []modbus.Endpoint{}
	for rows.Next() {
		var raw string
		var binding sql.NullString
		var bindingRevision sql.NullInt64
		var e modbus.Endpoint
		if err = rows.Scan(&raw, &binding, &bindingRevision); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, err
		}
		e.Binding = binding.String
		e.BindingRevision = bindingRevision.Int64
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *ModbusRepository) Profiles(ctx context.Context) ([]modbus.Profile, error) {
	rows, err := r.database.SQL().QueryContext(ctx, `SELECT document FROM modbus_profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []modbus.Profile{}
	for rows.Next() {
		var raw string
		var p modbus.Profile
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func nextModbusRevision(expected int64) (int64, error) {
	if expected < 0 || expected >= 2147483647 {
		return 0, ErrModbusInvalid
	}
	return expected + 1, nil
}

func saveModbusEndpoint(ctx context.Context, tx *sql.Tx, expected int64, e modbus.Endpoint) (modbus.Endpoint, error) {
	revision, err := nextModbusRevision(expected)
	if err != nil || e.ID == "" || e.SchemaVersion != 1 {
		return modbus.Endpoint{}, ErrModbusInvalid
	}
	old, err := readModbusEndpoint(ctx, tx, e.ID)
	if errors.Is(err, ErrModbusNotFound) {
		if expected != 0 {
			return modbus.Endpoint{}, ErrModbusConflict
		}
	} else if err != nil {
		return modbus.Endpoint{}, err
	} else {
		if expected != old.Revision {
			return modbus.Endpoint{}, ErrModbusConflict
		}
		if old.Transport != e.Transport {
			var count int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM modbus_profiles WHERE endpoint_id=?`, e.ID).Scan(&count); err != nil {
				return modbus.Endpoint{}, err
			}
			if count != 0 {
				return modbus.Endpoint{}, ErrModbusInvalid
			}
		}
	}
	e.Revision = revision
	e, err = e.Canonical()
	if err != nil {
		return modbus.Endpoint{}, ErrModbusInvalid
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return modbus.Endpoint{}, ErrModbusInvalid
	}
	var binding any
	if e.Binding != "" {
		binding = e.Binding
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO modbus_endpoints(id,revision,managed_device_id,managed_revision,document) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,managed_device_id=excluded.managed_device_id,managed_revision=excluded.managed_revision,document=excluded.document`, e.ID, e.Revision, binding, e.BindingRevision, string(raw))
	return e, err
}

// ReconcileUSB atomically projects trusted discovery into retained endpoint definitions.
// It runs independently of the installed/enabled state of the Serial integration.
func (r *ModbusRepository) ReconcileUSB(ctx context.Context, observations []ManagedDeviceObservation, now time.Time) ([]modbus.Endpoint, error) {
	for _, observation := range observations {
		if observation.Kind != "usb-serial" {
			return nil, ErrModbusInvalid
		}
	}
	tx, err := r.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	records, err := reconcileManagedDevices(ctx, tx, "modbus", observations, now)
	if err != nil {
		return nil, err
	}
	result := []modbus.Endpoint{}
	for _, record := range records {
		id := "usb-" + record.ID
		old, readErr := readModbusEndpoint(ctx, tx, id)
		if readErr != nil && !errors.Is(readErr, ErrModbusNotFound) {
			return nil, readErr
		}
		if readErr == nil && old.Binding != record.ID {
			return nil, ErrModbusConflict
		}
		h := modbus.USBIdentity{VendorID: record.Vendor, ProductID: record.Product, BusPath: record.PortPath}
		if !record.SerialWeak && record.IdentityRule == "serial" {
			serial := record.Serial
			h.SerialNumber = &serial
		}
		h.StableID = modbus.USBStableID(h)
		e := old
		if readErr != nil {
			e = modbus.Endpoint{SchemaVersion: 1, ID: id, Name: record.DisplayName, Transport: "rtu", Readiness: "unconfigured"}
		}
		e.ResourceID, e.SerialIdentity, e.Binding, e.BindingRevision = h.StableID, &h, record.ID, record.Revision
		e.Presence = record.Presence
		if e.Presence != "present" {
			e.Presence = "absent"
		}
		if !e.Enabled {
			e.Readiness = "unconfigured"
		} else if e.Presence == "absent" {
			e.Readiness = "offline"
		} else if e.BindingRevision != old.BindingRevision || e.Presence != old.Presence {
			e.Readiness = "ready"
		}
		if !reflect.DeepEqual(e, old) {
			e, err = saveModbusEndpoint(ctx, tx, old.Revision, e)
			if err != nil {
				return nil, err
			}
		}
		result = append(result, e)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// ResetReadiness requires fresh transport observations after Core startup.
func (r *ModbusRepository) ResetReadiness(ctx context.Context) error {
	endpoints, err := r.Endpoints(ctx)
	if err != nil {
		return err
	}
	tx, err := r.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range endpoints {
		old := e
		if e.Transport == "rtu" {
			e.Presence = "absent"
			if e.Enabled {
				e.Readiness = "offline"
			} else {
				e.Readiness = "unconfigured"
			}
		} else {
			e.Readiness = "offline"
		}
		if !reflect.DeepEqual(e, old) {
			if _, err = saveModbusEndpoint(ctx, tx, old.Revision, e); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (r *ModbusRepository) PutEndpoint(ctx context.Context, expected int64, e modbus.Endpoint) (modbus.Endpoint, error) {
	tx, err := r.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return modbus.Endpoint{}, err
	}
	defer tx.Rollback()
	e, err = saveModbusEndpoint(ctx, tx, expected, e)
	if err != nil {
		return modbus.Endpoint{}, err
	}
	return e, tx.Commit()
}

func (r *ModbusRepository) PutProfile(ctx context.Context, expected int64, p modbus.Profile) (modbus.Profile, error) {
	revision, err := nextModbusRevision(expected)
	if err != nil || p.ID == "" || p.OwnerID == "" || p.SchemaVersion != 1 {
		return modbus.Profile{}, ErrModbusInvalid
	}
	tx, err := r.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return modbus.Profile{}, err
	}
	defer tx.Rollback()
	old, err := readModbusProfile(ctx, tx, p.ID)
	if errors.Is(err, ErrModbusNotFound) {
		if expected != 0 {
			return modbus.Profile{}, ErrModbusConflict
		}
	} else if err != nil {
		return modbus.Profile{}, err
	} else {
		if expected != old.Revision {
			return modbus.Profile{}, ErrModbusConflict
		}
		p.OwnerID = old.OwnerID
	}
	e, err := readModbusEndpoint(ctx, tx, p.EndpointID)
	if err != nil {
		return modbus.Profile{}, err
	}
	p.Revision = revision
	p, err = p.Canonical(e)
	if err != nil {
		return modbus.Profile{}, ErrModbusInvalid
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return modbus.Profile{}, ErrModbusInvalid
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO modbus_profiles(id,revision,endpoint_id,owner_id,document) VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,endpoint_id=excluded.endpoint_id,owner_id=excluded.owner_id,document=excluded.document`, p.ID, p.Revision, p.EndpointID, p.OwnerID, string(raw))
	if err != nil {
		return modbus.Profile{}, err
	}
	return p, tx.Commit()
}
