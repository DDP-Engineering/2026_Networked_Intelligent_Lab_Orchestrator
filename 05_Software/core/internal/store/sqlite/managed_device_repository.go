package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	sqliteDriver "modernc.org/sqlite"
)

var ErrManagedDeviceConflict = errors.New("managed_device_conflict")

// ManagedDeviceObservation is one validated physical device from an integration adapter.
// EndpointID is core-owned state and must not be accepted from a public request.
type ManagedDeviceObservation struct {
	Kind         string
	Vendor       string
	Product      string
	Serial       string
	PortPath     string
	EndpointID   string
	Endpoints    json.RawMessage
	Capabilities json.RawMessage
}

// ManagedDeviceRecord separates the durable record ID from its current binding.
type ManagedDeviceRecord struct {
	PluginID     string
	ID           string
	Sequence     int64
	Kind         string
	Vendor       string
	Product      string
	Serial       string
	PortPath     string
	IdentityRule string
	SerialWeak   bool
	EndpointID   string
	Endpoints    json.RawMessage
	Capabilities json.RawMessage
	DisplayName  string
	NameKey      string
	Purpose      string
	Settings     json.RawMessage
	Enabled      bool
	Configured   bool
	Presence     string
	Readiness    string
	Revision     int64
}

type ManagedDeviceRepository struct{ database *DB }

func NewManagedDeviceRepository(database *DB) *ManagedDeviceRepository {
	return &ManagedDeviceRepository{database: database}
}

func canonicalDeviceName(name string) (string, error) {
	if name != strings.TrimSpace(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 {
		return "", errors.New("invalid managed device name")
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return "", errors.New("invalid managed device name")
		}
	}
	// NFKC Casefold preserves Unicode equivalence independently of SQLite NOCASE.
	key := norm.NFKC.String(cases.Fold().String(norm.NFKC.String(name)))
	if key == "" {
		return "", errors.New("invalid managed device name")
	}
	return key, nil
}

// Configure accepts an administrator's name and settings only at the revision read.
func (repository *ManagedDeviceRepository) Configure(ctx context.Context, pluginID, recordID string, revision int64, name, purpose string, settings json.RawMessage) (ManagedDeviceRecord, error) {
	if repository == nil || repository.database == nil || pluginID == "" || recordID == "" || revision < 1 {
		return ManagedDeviceRecord{}, errors.New("invalid managed device update")
	}
	key, err := canonicalDeviceName(name)
	if err != nil || utf8.RuneCountInString(purpose) > 200 {
		return ManagedDeviceRecord{}, errors.New("invalid managed device metadata")
	}
	var fields map[string]json.RawMessage
	if len(settings) > 16<<10 || json.Unmarshal(settings, &fields) != nil || fields == nil {
		return ManagedDeviceRecord{}, errors.New("invalid managed device settings")
	}
	tx, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	defer tx.Rollback()
	active, err := activeManagedWork(ctx, tx, recordID)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	if active {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	result, err := tx.ExecContext(ctx, `UPDATE bench_devices SET display_name=?,name_key=?,purpose=?,settings_json=?,configured=1,readiness=CASE WHEN presence='present' AND enabled=1 THEN 'initializing' ELSE 'failed' END,revision=revision+1 WHERE plugin_id=? AND record_id=? AND revision=? AND tombstone_target IS NULL`, name, key, purpose, settings, pluginID, recordID, revision)
	if err != nil {
		var sqliteError *sqliteDriver.Error
		if errors.As(err, &sqliteError) && sqliteError.Code()&0xff == 19 {
			return ManagedDeviceRecord{}, ErrManagedDeviceConflict
		}
		return ManagedDeviceRecord{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	if changed == 0 {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	if err := tx.Commit(); err != nil {
		return ManagedDeviceRecord{}, err
	}
	records, err := repository.List(ctx, pluginID)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	for _, record := range records {
		if record.ID == recordID {
			return record, nil
		}
	}
	return ManagedDeviceRecord{}, ErrManagedDeviceConflict
}

// MarkReady completes a successful core-owned reset at the same device revision.
func (repository *ManagedDeviceRepository) MarkReady(ctx context.Context, pluginID, recordID string, revision int64) (ManagedDeviceRecord, error) {
	if repository == nil || repository.database == nil || pluginID == "" || recordID == "" || revision < 1 {
		return ManagedDeviceRecord{}, errors.New("invalid managed device readiness update")
	}
	result, err := repository.database.SQL().ExecContext(ctx, `UPDATE bench_devices SET readiness='ready',last_reset_status='completed',revision=revision+1 WHERE plugin_id=? AND record_id=? AND revision=? AND presence='present' AND configured=1 AND enabled=1 AND endpoint_id!='' AND tombstone_target IS NULL`, pluginID, recordID, revision)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	if changed == 0 {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	records, err := repository.List(ctx, pluginID)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	for _, record := range records {
		if record.ID == recordID {
			return record, nil
		}
	}
	return ManagedDeviceRecord{}, ErrManagedDeviceConflict
}

// MarkFailed keeps a configured device unavailable after an initialization failure.
func (repository *ManagedDeviceRepository) MarkFailed(ctx context.Context, pluginID, recordID string, revision int64) error {
	if repository == nil || repository.database == nil || pluginID == "" || recordID == "" || revision < 1 {
		return ErrManagedDeviceConflict
	}
	result, err := repository.database.SQL().ExecContext(ctx, `UPDATE bench_devices SET readiness='failed',last_reset_status='failed',revision=revision+1 WHERE plugin_id=? AND record_id=? AND revision=? AND configured=1 AND tombstone_target IS NULL`, pluginID, recordID, revision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrManagedDeviceConflict
	}
	return nil
}

// SetEnabled changes per-device admission at one observed revision.
func (repository *ManagedDeviceRepository) SetEnabled(ctx context.Context, pluginID, recordID string, revision int64, enabled bool) (ManagedDeviceRecord, error) {
	if repository == nil || repository.database == nil || pluginID == "" || recordID == "" || revision < 1 {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	tx, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	defer tx.Rollback()
	active, err := activeManagedWork(ctx, tx, recordID)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	if active {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	result, err := tx.ExecContext(ctx, `UPDATE bench_devices SET enabled=?,readiness=CASE WHEN ?=1 AND presence='present' AND endpoint_id!='' THEN 'initializing' ELSE 'failed' END,revision=revision+1 WHERE plugin_id=? AND record_id=? AND revision=? AND configured=1 AND tombstone_target IS NULL`, enabled, enabled, pluginID, recordID, revision)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	if changed != 1 {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	if err := tx.Commit(); err != nil {
		return ManagedDeviceRecord{}, err
	}
	return repository.recordByID(ctx, pluginID, recordID)
}

// MarkInitializing reserves a failed camera for a new core-owned baseline reset.
func (repository *ManagedDeviceRepository) MarkInitializing(ctx context.Context, pluginID, recordID string, revision int64) (ManagedDeviceRecord, error) {
	if repository == nil || repository.database == nil || pluginID == "" || recordID == "" || revision < 1 {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	result, err := repository.database.SQL().ExecContext(ctx, `UPDATE bench_devices SET readiness='initializing',revision=revision+1 WHERE plugin_id=? AND record_id=? AND revision=? AND configured=1 AND enabled=1 AND presence='present' AND endpoint_id!='' AND readiness='failed' AND tombstone_target IS NULL`, pluginID, recordID, revision)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	if changed != 1 {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	return repository.recordByID(ctx, pluginID, recordID)
}

func (repository *ManagedDeviceRepository) recordByID(ctx context.Context, pluginID, recordID string) (ManagedDeviceRecord, error) {
	records, err := repository.List(ctx, pluginID)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	for _, record := range records {
		if record.ID == recordID {
			return record, nil
		}
	}
	return ManagedDeviceRecord{}, ErrManagedDeviceConflict
}

// MarkUnknown prevents stale pre-startup presence from authorizing new work.
func (repository *ManagedDeviceRepository) MarkUnknown(ctx context.Context, pluginID string) error {
	if repository == nil || repository.database == nil || pluginID == "" {
		return errors.New("managed device repository is unavailable")
	}
	_, err := repository.database.SQL().ExecContext(ctx, `UPDATE bench_devices SET presence='unknown',revision=revision+1 WHERE plugin_id=? AND tombstone_target IS NULL AND presence!='unknown'`, pluginID)
	return err
}

// List returns all live records, including absent and unconfigured devices.
func (repository *ManagedDeviceRepository) List(ctx context.Context, pluginID string) ([]ManagedDeviceRecord, error) {
	if repository == nil || repository.database == nil || pluginID == "" {
		return nil, errors.New("managed device repository is unavailable")
	}
	tx, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	records, err := readManagedDevices(ctx, tx, pluginID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return records, nil
}

func activeManagedWork(ctx context.Context, tx *sql.Tx, deviceIDs ...string) (bool, error) {
	for _, id := range deviceIDs {
		var count int
		err := tx.QueryRowContext(ctx, `SELECT count(*) FROM operation_requests r JOIN sessions s ON s.request_id=r.id WHERE r.device_id=? AND s.state IN ('pending_approval','queued','running')`, id).Scan(&count)
		if err != nil {
			return false, err
		}
		if count != 0 {
			return true, nil
		}
	}
	return false, nil
}

// Rebind moves a serial-less camera's current hardware onto an absent configured record.
// The provisional record remains as a tombstone and its ID is never reused.
func (repository *ManagedDeviceRepository) Rebind(ctx context.Context, pluginID, recordID string, revision int64, provisionalID string, provisionalRevision int64) (_ ManagedDeviceRecord, err error) {
	if repository == nil || repository.database == nil || pluginID == "" || recordID == "" || provisionalID == "" || recordID == provisionalID || revision < 1 || provisionalRevision < 1 {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	tx, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	defer tx.Rollback()
	records, err := readManagedDevices(ctx, tx, pluginID)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	var source, provisional *ManagedDeviceRecord
	for index := range records {
		if records[index].ID == recordID {
			source = &records[index]
		}
		if records[index].ID == provisionalID {
			provisional = &records[index]
		}
	}
	if source == nil || provisional == nil || source.Revision != revision || provisional.Revision != provisionalRevision ||
		!source.Configured || source.Presence != "absent" || source.IdentityRule != "port" ||
		provisional.Configured || provisional.Presence != "present" || provisional.IdentityRule != "port" ||
		source.Kind != provisional.Kind || source.Vendor != provisional.Vendor || source.Product != provisional.Product {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	active, err := activeManagedWork(ctx, tx, recordID, provisionalID)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	if active {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	var prior int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM operation_requests WHERE device_id=?`, provisionalID).Scan(&prior); err != nil {
		return ManagedDeviceRecord{}, err
	}
	if prior != 0 {
		return ManagedDeviceRecord{}, ErrManagedDeviceConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE bench_devices SET tombstone_target=?,enabled=0,presence='absent',readiness='failed',endpoint_id='',revision=revision+1 WHERE plugin_id=? AND record_id=? AND revision=?`, recordID, pluginID, provisionalID, provisionalRevision); err != nil {
		return ManagedDeviceRecord{}, err
	}
	readiness := "initializing"
	if !source.Enabled || provisional.EndpointID == "" {
		readiness = "failed"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE bench_devices SET vendor=?,product=?,serial=?,port_path=?,identity_rule=?,serial_weak=?,endpoint_id=?,endpoints_json=?,capabilities_json=?,presence='present',readiness=?,last_seen_at=(SELECT last_seen_at FROM bench_devices WHERE plugin_id=? AND record_id=?),revision=revision+1 WHERE plugin_id=? AND record_id=? AND revision=?`,
		provisional.Vendor, provisional.Product, provisional.Serial, provisional.PortPath, provisional.IdentityRule, provisional.SerialWeak, provisional.EndpointID, provisional.Endpoints, provisional.Capabilities, readiness,
		pluginID, provisionalID, pluginID, recordID, revision); err != nil {
		return ManagedDeviceRecord{}, err
	}
	updated, err := readManagedDevices(ctx, tx, pluginID)
	if err != nil {
		return ManagedDeviceRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManagedDeviceRecord{}, err
	}
	for _, record := range updated {
		if record.ID == recordID {
			return record, nil
		}
	}
	return ManagedDeviceRecord{}, ErrManagedDeviceConflict
}

// Forget tombstones a record after its active jobs finish and clears its binding.
func (repository *ManagedDeviceRepository) Forget(ctx context.Context, pluginID, recordID string, revision int64) error {
	if repository == nil || repository.database == nil || pluginID == "" || recordID == "" || revision < 1 {
		return ErrManagedDeviceConflict
	}
	tx, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	active, err := activeManagedWork(ctx, tx, recordID)
	if err != nil {
		return err
	}
	if active {
		return ErrManagedDeviceConflict
	}
	result, err := tx.ExecContext(ctx, `UPDATE bench_devices SET vendor='',product='',serial='',port_path='',endpoint_id='',endpoints_json='[]',capabilities_json='{}',name_key=NULL,purpose='',settings_json='{}',enabled=0,configured=0,presence='absent',readiness='failed',tombstone_target='forgotten',revision=revision+1 WHERE plugin_id=? AND record_id=? AND revision=? AND tombstone_target IS NULL`, pluginID, recordID, revision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrManagedDeviceConflict
	}
	return tx.Commit()
}

func serialKey(vendor, product, serial string) string {
	return vendor + "\x00" + product + "\x00" + serial
}

func weakSerial(serial string) bool {
	switch strings.ToLower(strings.TrimSpace(serial)) {
	case "", "0", "00000000", "0123456789", "123456789", "unknown", "none":
		return true
	}
	return false
}

func recordPrefix(kind string) string {
	if kind == "usb-serial" {
		return "serial"
	}
	if kind == "camera" {
		return "cam"
	}
	return "la"
}

func suggestedName(kind, id string) string {
	if kind == "usb-serial" {
		return "USB Serial (" + id + ")"
	}
	if kind == "camera" {
		return "USB Camera (" + id + ")"
	}
	return "Logic Analyzer (" + id + ")"
}

func readManagedDevices(ctx context.Context, tx *sql.Tx, pluginID string) ([]ManagedDeviceRecord, error) {
	rows, err := tx.QueryContext(ctx, `SELECT plugin_id,record_id,sequence,kind,vendor,product,serial,port_path,identity_rule,serial_weak,endpoint_id,endpoints_json,capabilities_json,display_name,coalesce(name_key,''),purpose,settings_json,enabled,configured,presence,readiness,revision FROM bench_devices WHERE plugin_id=? AND tombstone_target IS NULL ORDER BY sequence`, pluginID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ManagedDeviceRecord{}
	for rows.Next() {
		var device ManagedDeviceRecord
		var settings, endpoints, capabilities string
		if err := rows.Scan(&device.PluginID, &device.ID, &device.Sequence, &device.Kind, &device.Vendor, &device.Product, &device.Serial, &device.PortPath, &device.IdentityRule, &device.SerialWeak, &device.EndpointID, &endpoints, &capabilities, &device.DisplayName, &device.NameKey, &device.Purpose, &settings, &device.Enabled, &device.Configured, &device.Presence, &device.Readiness, &device.Revision); err != nil {
			return nil, err
		}
		device.Settings = json.RawMessage(settings)
		device.Endpoints = json.RawMessage(endpoints)
		device.Capabilities = json.RawMessage(capabilities)
		result = append(result, device)
	}
	return result, rows.Err()
}

func nextManagedSequence(ctx context.Context, tx *sql.Tx, pluginID string) (int64, error) {
	var next int64
	err := tx.QueryRowContext(ctx, `SELECT next_sequence FROM bench_device_sequences WHERE plugin_id=?`, pluginID).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO bench_device_sequences(plugin_id,next_sequence) VALUES(?,2)`, pluginID)
		return 1, err
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE bench_device_sequences SET next_sequence=? WHERE plugin_id=?`, next+1, pluginID)
	return next, err
}

// Reconcile atomically updates physical bindings and presence without deleting history.
func (repository *ManagedDeviceRepository) Reconcile(ctx context.Context, pluginID string, observations []ManagedDeviceObservation, now time.Time) (_ []ManagedDeviceRecord, err error) {
	if repository == nil || repository.database == nil || pluginID == "" {
		return nil, errors.New("managed device repository is unavailable")
	}
	tx, err := repository.database.SQL().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	existing, err := readManagedDevices(ctx, tx, pluginID)
	if err != nil {
		return nil, err
	}
	weak := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT vendor,product,serial FROM bench_weak_serials WHERE plugin_id=?`, pluginID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var vendor, product, serial string
		if err := rows.Scan(&vendor, &product, &serial); err != nil {
			rows.Close()
			return nil, err
		}
		weak[serialKey(vendor, product, serial)] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	observedPorts := map[string]map[string]bool{}
	for index := range observations {
		observed := &observations[index]
		if observed.Kind == "" || observed.PortPath == "" {
			return nil, errors.New("invalid managed device observation")
		}
		if len(observed.Endpoints) == 0 {
			observed.Endpoints = json.RawMessage(`[]`)
		}
		if len(observed.Capabilities) == 0 {
			observed.Capabilities = json.RawMessage(`{}`)
		}
		if len(observed.Endpoints) > 16<<10 || len(observed.Capabilities) > 16<<10 || !json.Valid(observed.Endpoints) || !json.Valid(observed.Capabilities) {
			return nil, errors.New("invalid managed device capabilities")
		}
		key := serialKey(observed.Vendor, observed.Product, observed.Serial)
		if observedPorts[key] == nil {
			observedPorts[key] = map[string]bool{}
		}
		if observedPorts[key][observed.PortPath] {
			return nil, errors.New("duplicate physical device observation")
		}
		observedPorts[key][observed.PortPath] = true
		if weakSerial(observed.Serial) {
			weak[key] = true
		}
	}
	for key, seen := range observedPorts {
		if len(seen) > 1 {
			weak[key] = true
		}
	}
	historicalPorts := map[string]map[string]bool{}
	for _, record := range existing {
		if record.Serial == "" {
			continue
		}
		key := serialKey(record.Vendor, record.Product, record.Serial)
		if historicalPorts[key] == nil {
			historicalPorts[key] = map[string]bool{}
		}
		historicalPorts[key][record.PortPath] = true
	}
	for key, seen := range historicalPorts {
		if len(seen) > 1 {
			weak[key] = true
		}
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	for key, isWeak := range weak {
		if !isWeak {
			continue
		}
		parts := strings.Split(key, "\x00")
		if len(parts) != 3 || parts[2] == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bench_weak_serials(plugin_id,vendor,product,serial,classified_at) VALUES(?,?,?,?,?)`, pluginID, parts[0], parts[1], parts[2], stamp); err != nil {
			return nil, err
		}
	}
	used := map[string]bool{}
	for _, observed := range observations {
		key := serialKey(observed.Vendor, observed.Product, observed.Serial)
		serialWeak := weak[key] || weakSerial(observed.Serial)
		rule := "serial"
		if serialWeak {
			rule = "port"
		}
		var matched *ManagedDeviceRecord
		for index := range existing {
			candidate := &existing[index]
			if used[candidate.ID] || candidate.Vendor != observed.Vendor || candidate.Product != observed.Product {
				continue
			}
			if rule == "serial" && candidate.Serial == observed.Serial && candidate.IdentityRule == "serial" || rule == "port" && candidate.PortPath == observed.PortPath {
				matched = candidate
				break
			}
		}
		if matched != nil {
			used[matched.ID] = true
			readiness := matched.Readiness
			if !matched.Configured {
				readiness = "unconfigured"
			} else if !matched.Enabled {
				readiness = "failed"
			} else if matched.Presence != "present" {
				readiness = "initializing"
			}
			if observed.EndpointID == "" && matched.Configured {
				readiness = "failed"
			}
			// Polls must not invalidate configuration forms or approved sessions
			// when only the last-seen timestamp changed.
			if matched.Vendor == observed.Vendor && matched.Product == observed.Product && matched.Serial == observed.Serial && matched.PortPath == observed.PortPath && matched.IdentityRule == rule && matched.SerialWeak == serialWeak && matched.EndpointID == observed.EndpointID && bytes.Equal(matched.Endpoints, observed.Endpoints) && bytes.Equal(matched.Capabilities, observed.Capabilities) && matched.Presence == "present" && matched.Readiness == readiness {
				if _, err := tx.ExecContext(ctx, `UPDATE bench_devices SET last_seen_at=? WHERE plugin_id=? AND record_id=?`, stamp, pluginID, matched.ID); err != nil {
					return nil, err
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE bench_devices SET vendor=?,product=?,serial=?,port_path=?,identity_rule=?,serial_weak=?,endpoint_id=?,endpoints_json=?,capabilities_json=?,presence='present',readiness=?,last_seen_at=?,revision=revision+1 WHERE plugin_id=? AND record_id=?`, observed.Vendor, observed.Product, observed.Serial, observed.PortPath, rule, serialWeak, observed.EndpointID, observed.Endpoints, observed.Capabilities, readiness, stamp, pluginID, matched.ID); err != nil {
				return nil, err
			}
			continue
		}
		sequence, err := nextManagedSequence(ctx, tx, pluginID)
		if err != nil {
			return nil, err
		}
		id := fmt.Sprintf("%s-%d", recordPrefix(observed.Kind), sequence)
		if _, err := tx.ExecContext(ctx, `INSERT INTO bench_devices(plugin_id,record_id,sequence,kind,vendor,product,serial,port_path,identity_rule,serial_weak,endpoint_id,endpoints_json,capabilities_json,display_name,presence,readiness,last_seen_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, pluginID, id, sequence, observed.Kind, observed.Vendor, observed.Product, observed.Serial, observed.PortPath, rule, serialWeak, observed.EndpointID, observed.Endpoints, observed.Capabilities, suggestedName(observed.Kind, id), "present", "unconfigured", stamp, stamp); err != nil {
			return nil, err
		}
		used[id] = true
	}
	for _, record := range existing {
		if used[record.ID] || record.Presence == "absent" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE bench_devices SET presence='absent',readiness='failed',revision=revision+1 WHERE plugin_id=? AND record_id=?`, pluginID, record.ID); err != nil {
			return nil, err
		}
	}
	result, err := readManagedDevices(ctx, tx, pluginID)
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Sequence < result[j].Sequence })
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
