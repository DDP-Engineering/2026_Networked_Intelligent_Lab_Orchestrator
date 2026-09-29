package bench

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/config"
	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/integration/supervisor"
	"nilo.local/nilo/internal/store/sqlite"
)

var packageIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func decodeStrict(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return ErrInvalid
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

// verifyPackage checks the complete regular-file inventory, including the runner
// and schemas. A catalog directory is an administrator-controlled trust source.
func verifyPackage(root string) (string, error) {
	index, err := os.ReadFile(filepath.Join(root, "files.sha256.json"))
	if err != nil {
		return "", err
	}
	if len(index) > 1<<20 {
		return "", ErrInvalid
	}
	files := map[string]string{}
	if json.Unmarshal(index, &files) != nil || len(files) == 0 || len(files) > 4096 {
		return "", ErrInvalid
	}
	if files["manifest.json"] == "" || files["runner"] == "" {
		return "", ErrInvalid
	}
	total := int64(0)
	runtimeBytes := int64(0)
	for name, want := range files {
		path, err := config.ResolveWithin(root, name)
		if err != nil {
			return "", err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return "", ErrInvalid
		}
		total += info.Size()
		if strings.HasPrefix(name, "runtime/") {
			runtimeBytes += info.Size()
		}
		if total > 1<<30 {
			return "", ErrInvalid
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		hash := sha256.New()
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil || hex.EncodeToString(hash.Sum(nil)) != want {
			return "", fmt.Errorf("package_integrity: %s", name)
		}
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name != "files.sha256.json" && files[name] == "" {
			return fmt.Errorf("unlisted_package_file: %s", name)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if runtimeBytes == 0 {
		if total > 128<<20 {
			return "", ErrInvalid
		}
	} else {
		var limits struct {
			Architecture   string `json:"architecture"`
			Backend        string `json:"backend"`
			MeasuredBytes  int64  `json:"measured_bytes"`
			MaximumBytes   int64  `json:"maximum_bytes"`
			ProjectVersion string `json:"project_version"`
			SelfContained  bool   `json:"self_contained"`
		}
		raw, readErr := os.ReadFile(filepath.Join(root, "runtime.json"))
		if readErr != nil || decodeStrict(raw, &limits) != nil || !limits.SelfContained || limits.MeasuredBytes != runtimeBytes || limits.MaximumBytes < (runtimeBytes*120+99)/100 || limits.MaximumBytes > 1<<30 {
			return "", ErrInvalid
		}
	}
	hash := sha256.Sum256(index)
	return hex.EncodeToString(hash[:]), nil
}

func (m *Manager) loadPlugin(ctx context.Context, id string) (Plugin, error) {
	p := Plugin{ID: id, DisplayName: id, Simulation: m.options.Simulation && id == "sigrok", Devices: []Device{}, Operations: []string{}, Status: "available"}
	if adapter, err := adapterFor(id); err == nil {
		p.DisplayName = adapter.displayName
	}
	var devices []byte
	err := m.db().QueryRowContext(ctx, `SELECT instance_id,installed,enabled,mode,digest,devices_json,last_error FROM bench_plugins WHERE id=?`, id).Scan(&p.InstanceID, &p.Installed, &p.Enabled, &p.Mode, &p.Digest, &devices, &p.Error)
	if err != nil {
		return p, normalizedError(err)
	}
	if err = json.Unmarshal(devices, &p.Devices); err != nil {
		return p, err
	}
	if id == "camera" || id == "serial" {
		records, recordErr := sqlite.NewManagedDeviceRepository(m.options.Database).List(ctx, id)
		if recordErr != nil {
			return p, recordErr
		}
		p.Devices = managedCameraDevices(records)
	}
	if p.Installed {
		p.Status = "disabled"
		root, err := m.packageRoot(id)
		if err != nil {
			return p, err
		}
		descriptor, err := manifest.Load(root)
		if err == nil {
			err = verifyAdapterContract(descriptor)
		}
		if err != nil {
			p.Status = "unavailable"
			p.Error = "Installed package is unavailable"
			return p, nil
		}
		p.Version = descriptor.Package.Version
		for _, op := range descriptor.Operations {
			p.Operations = append(p.Operations, op.ID)
		}
		if p.Enabled {
			p.Status = "ready"
		}
		if p.Error != "" {
			p.Status = "unavailable"
		}
	}
	return p, nil
}

func (m *Manager) ListPlugins(ctx context.Context, actor identity.Actor) ([]Plugin, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	rows, err := m.db().QueryContext(ctx, `SELECT id FROM bench_plugins ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if m.options.BundleDir != "" {
		entries, _ := os.ReadDir(m.options.BundleDir)
		for _, entry := range entries {
			if entry.IsDir() && packageIDPattern.MatchString(entry.Name()) {
				ids[entry.Name()] = true
			}
		}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	result := make([]Plugin, 0, len(ids))
	for _, id := range ordered {
		p, err := m.loadPlugin(ctx, id)
		if errors.Is(err, ErrNotFound) {
			p.Mode = "ask"
		} else if err != nil {
			return nil, err
		}
		m.describeUpdate(&p)
		result = append(result, p)
	}
	return result, nil
}

func (m *Manager) process(p Plugin) (supervisor.Supervisor, error) {
	return m.processConfigured(p, "")
}

func (m *Manager) processConfigured(p Plugin, deviceID string, shutdownGrace ...time.Duration) (supervisor.Supervisor, error) {
	root, err := m.packageRoot(p.ID)
	if err != nil {
		return nil, err
	}
	digest, err := verifyPackage(root)
	if err != nil || digest != p.Digest {
		return nil, ErrUnavailable
	}
	if m.options.runner != nil {
		return m.options.runner(p)
	}
	descriptor, err := manifest.Load(root)
	if err != nil {
		return nil, err
	}
	if err := verifyAdapterContract(descriptor); err != nil {
		return nil, err
	}
	configuration, err := m.runnerConfiguration(context.Background(), p.ID, deviceID)
	if err != nil {
		return nil, err
	}
	args, err := runnerArgumentsFor(p.ID, root, m.options, descriptor.Runner.Arguments)
	if err != nil {
		return nil, err
	}
	descriptor.Runner.Arguments = args
	grace := time.Second
	if len(shutdownGrace) > 0 {
		grace = shutdownGrace[0]
	}
	return supervisor.NewProcessSupervisor(supervisor.ProcessConfig{PackageID: p.ID, PackageRoot: root, Runner: descriptor.Runner, Configuration: configuration, GracePeriod: grace, Environment: map[string]string{"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8"}})
}

func (m *Manager) health(ctx context.Context, p Plugin) error {
	runner, err := m.process(p)
	if err != nil {
		return err
	}
	defer runner.Shutdown(context.Background())
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = runner.Describe(ctx, p.ID)
	return err
}

func (m *Manager) waitPluginIdle(ctx context.Context, id string, maximum time.Duration) error {
	deadline := time.NewTimer(maximum)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		err := m.db().QueryRowContext(ctx, `SELECT count(*) FROM bench_jobs j JOIN sessions s ON s.id=j.session_id WHERE j.plugin_id=? AND s.state IN ('queued','running','pending_approval')`, id).Scan(&count)
		if err != nil {
			if ctx.Err() != nil {
				return ErrBusy
			}
			return err
		}
		if count == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrBusy
		case <-deadline.C:
			return ErrBusy
		case <-ticker.C:
		}
	}
}

func rollbackPackageSwitch(target, backupRoot string) error {
	integrationRoot := filepath.Dir(target)
	failed := filepath.Join(backupRoot, "failed-upgrade")
	previous := filepath.Join(backupRoot, "previous")
	if err := os.Rename(target, failed); err != nil {
		return err
	}
	if err := os.Rename(previous, target); err != nil {
		_ = os.Rename(failed, target)
		return err
	}
	if err := syncPackageDirectory(integrationRoot); err != nil {
		return err
	}
	return os.RemoveAll(failed)
}

func migratePackageState(currentSchema int, candidate manifest.Manifest, state []byte) ([]byte, error) {
	if currentSchema < 1 || candidate.StateSchema < currentSchema {
		return nil, ErrInvalid
	}
	var document any
	if err := json.Unmarshal(state, &document); err != nil {
		return nil, ErrInvalid
	}
	for currentSchema < candidate.StateSchema {
		found := false
		for _, migration := range candidate.StateMigrations {
			if migration.From == currentSchema && migration.Kind == "preserve-json" {
				currentSchema = migration.To
				found = true
				break
			}
		}
		if !found || currentSchema > candidate.StateSchema {
			return nil, ErrInvalid
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, ErrInvalid
	}
	return encoded, nil
}

func (m *Manager) recordUpgradeRollback(ctx context.Context, actor identity.Actor, current Plugin, targetVersion, reason string) error {
	return m.record(ctx, actor, "plugin.upgrade_rolled_back", current.ID, map[string]string{
		"from": current.Version, "reason": reason, "to": targetVersion,
	})
}

func (m *Manager) upgradePlugin(ctx context.Context, actor identity.Actor, current Plugin) error {
	if m.options.BundleDir == "" {
		return ErrUnavailable
	}
	if err := m.waitPluginIdle(ctx, current.ID, time.Minute); err != nil {
		return err
	}
	source, err := config.ResolveWithin(m.options.BundleDir, current.ID)
	if err != nil {
		return err
	}
	digest, err := verifyPackage(source)
	if err != nil {
		return err
	}
	descriptor, err := manifest.Load(source)
	if err != nil || descriptor.Package.ID != current.ID {
		return errors.Join(ErrInvalid, err)
	}
	if digest == current.Digest {
		return nil
	}
	target, err := m.packageRoot(current.ID)
	if err != nil {
		return err
	}
	currentDescriptor, err := manifest.Load(target)
	if err != nil {
		return errors.Join(ErrInvalid, err)
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".upgrade-"+current.ID+"-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.CopyFS(stage, os.DirFS(source)); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(stage, "runner"), 0o700); err != nil {
		return err
	}
	if stagedDigest, err := verifyPackage(stage); err != nil || stagedDigest != digest {
		return errors.Join(ErrInvalid, err)
	}
	if err := m.record(ctx, actor, "plugin.upgrade_requested", current.ID, map[string]string{"from": current.Version, "to": descriptor.Package.Version}); err != nil {
		return err
	}
	var priorDigest string
	var priorStateSchema int
	var priorState []byte
	if err := m.db().QueryRowContext(ctx,
		`SELECT digest,state_schema,devices_json FROM bench_plugins WHERE id=?`, current.ID,
	).Scan(&priorDigest, &priorStateSchema, &priorState); err != nil {
		_ = m.recordUpgradeRollback(ctx, actor, current, descriptor.Package.Version, "state-snapshot")
		return err
	}
	if priorStateSchema != currentDescriptor.StateSchema {
		rollbackErr := m.recordUpgradeRollback(ctx, actor, current, descriptor.Package.Version, "state-schema-mismatch")
		return errors.Join(ErrInvalid, rollbackErr)
	}
	migratedState, err := migratePackageState(priorStateSchema, descriptor, priorState)
	if err != nil {
		rollbackErr := m.recordUpgradeRollback(ctx, actor, current, descriptor.Package.Version, "migration")
		return errors.Join(err, rollbackErr)
	}
	backupRoot, err := os.MkdirTemp(filepath.Dir(target), ".upgrade-"+current.ID+"-backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(backupRoot)
	integrationRoot := filepath.Dir(target)
	if err := syncPackageTree(stage); err != nil {
		return err
	}
	journal := upgradeJournal{
		Backup: filepath.Base(backupRoot), CandidateDigest: digest, ID: current.ID,
		Phase: "prepared", PriorDigest: priorDigest, Schema: upgradeJournalSchema,
		Stage: filepath.Base(stage), Target: current.ID,
	}
	if err := writeUpgradeJournal(integrationRoot, journal); err != nil {
		return err
	}
	if err := os.Rename(target, filepath.Join(backupRoot, "previous")); err != nil {
		_ = removeUpgradeJournal(integrationRoot, current.ID)
		rollbackErr := m.recordUpgradeRollback(ctx, actor, current, descriptor.Package.Version, "activate-backup")
		return errors.Join(err, rollbackErr)
	}
	if err := syncPackageDirectory(integrationRoot); err != nil {
		return err
	}
	journal.Phase = "backup_moved"
	if err := writeUpgradeJournal(integrationRoot, journal); err != nil {
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		restoreErr := os.Rename(filepath.Join(backupRoot, "previous"), target)
		if restoreErr == nil {
			restoreErr = errors.Join(syncPackageDirectory(integrationRoot), removeUpgradeJournal(integrationRoot, current.ID))
		}
		rollbackErr := m.recordUpgradeRollback(ctx, actor, current, descriptor.Package.Version, "activate-candidate")
		return errors.Join(err, restoreErr, rollbackErr)
	}
	if err := syncPackageDirectory(integrationRoot); err != nil {
		return err
	}
	journal.Phase = "candidate_active"
	if err := writeUpgradeJournal(integrationRoot, journal); err != nil {
		return err
	}
	candidate := current
	candidate.Digest = digest
	if err := m.health(ctx, candidate); err != nil {
		m.report(err)
		restoreErr := rollbackPackageSwitch(target, backupRoot)
		if restoreErr == nil {
			restoreErr = removeUpgradeJournal(integrationRoot, current.ID)
		}
		rollbackErr := m.recordUpgradeRollback(ctx, actor, current, descriptor.Package.Version, "health")
		return errors.Join(ErrUnavailable, restoreErr, rollbackErr)
	}
	if _, err := m.db().ExecContext(ctx,
		`UPDATE bench_plugins SET digest=?,state_schema=?,devices_json=?,last_error='' WHERE id=?`,
		digest, descriptor.StateSchema, migratedState, current.ID,
	); err != nil {
		restoreErr := rollbackPackageSwitch(target, backupRoot)
		if restoreErr == nil {
			restoreErr = removeUpgradeJournal(integrationRoot, current.ID)
		}
		rollbackErr := m.recordUpgradeRollback(ctx, actor, current, descriptor.Package.Version, "state-commit")
		return errors.Join(err, restoreErr, rollbackErr)
	}
	journal.Phase = "state_committed"
	if err := writeUpgradeJournal(integrationRoot, journal); err != nil {
		return err
	}
	if err := m.record(ctx, actor, "plugin.upgrade_succeeded", current.ID, map[string]string{"from": current.Version, "to": descriptor.Package.Version}); err != nil {
		_, stateErr := m.db().ExecContext(ctx,
			`UPDATE bench_plugins SET digest=?,state_schema=?,devices_json=? WHERE id=?`,
			priorDigest, priorStateSchema, priorState, current.ID,
		)
		restoreErr := rollbackPackageSwitch(target, backupRoot)
		if stateErr == nil && restoreErr == nil {
			restoreErr = removeUpgradeJournal(integrationRoot, current.ID)
		}
		rollbackErr := m.recordUpgradeRollback(ctx, actor, current, descriptor.Package.Version, "success-audit")
		return errors.Join(err, stateErr, restoreErr, rollbackErr)
	}
	if err := os.RemoveAll(backupRoot); err != nil {
		return err
	}
	return removeUpgradeJournal(integrationRoot, current.ID)
}

// Lifecycle serializes admission against installation and enablement changes.
// Busy restart/uninstall refuses; disable lets existing work finish and settles
// queued jobs so it cannot secretly start hardware after disable was requested.
func (m *Manager) Lifecycle(ctx context.Context, actor identity.Actor, id, action, mode string) (Plugin, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return Plugin{}, err
	}
	if !packageIDPattern.MatchString(id) || (action != "enable" && mode != "") {
		return Plugin{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.serialBusy(id) {
		if action != "disable" {
			return Plugin{}, ErrBusy
		}
		m.stopSerialLocked(id, "plugin_disabled")
	}
	if m.transportFamilyBusyLocked(id) {
		if action != "disable" {
			return Plugin{}, ErrBusy
		}
		m.cancelTransportsLocked(id, "")
	}
	if m.cameraBusy(id) {
		return Plugin{}, ErrBusy
	}
	if m.closed {
		return Plugin{}, ErrUnavailable
	}
	p, err := m.loadPlugin(ctx, id)
	if action == "install" {
		if err == nil && p.Installed {
			return p, nil
		}
		if m.options.BundleDir == "" {
			return p, ErrUnavailable
		}
		source, err := config.ResolveWithin(m.options.BundleDir, id)
		if err != nil {
			return p, err
		}
		digest, err := verifyPackage(source)
		if err != nil {
			return p, err
		}
		descriptor, err := manifest.Load(source)
		if err != nil {
			return p, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if descriptor.Package.ID != id {
			return p, ErrInvalid
		}
		target, err := m.packageRoot(id)
		if err != nil {
			return p, err
		}
		if _, err := os.Lstat(target); err == nil {
			return p, ErrConflict
		}
		stage, err := os.MkdirTemp(filepath.Dir(target), ".install-")
		if err != nil {
			return p, err
		}
		defer os.RemoveAll(stage)
		if err = os.CopyFS(stage, os.DirFS(source)); err != nil {
			return p, err
		}
		if err = os.Chmod(filepath.Join(stage, "runner"), 0700); err != nil {
			return p, err
		}
		if _, err = verifyPackage(stage); err != nil {
			return p, err
		}
		if err = m.record(ctx, actor, "plugin.install_requested", id, nil); err != nil {
			return p, err
		}
		if err = os.Rename(stage, target); err != nil {
			return p, err
		}
		instance, err := domain.NewIntegrationID()
		if err != nil {
			return p, err
		}
		_, err = m.db().ExecContext(ctx, `INSERT INTO bench_plugins(id,instance_id,installed,enabled,mode,digest,state_schema) VALUES(?,?,1,0,'ask',?,?) ON CONFLICT(id) DO UPDATE SET installed=1,enabled=0,digest=excluded.digest,state_schema=excluded.state_schema,last_error=''`, id, instance.String(), digest, descriptor.StateSchema)
		if err != nil {
			os.RemoveAll(target)
			return p, err
		}
	} else {
		if err != nil {
			return p, err
		}
		if !p.Installed {
			return p, ErrUnavailable
		}
		switch action {
		case "enable":
			if mode == "" {
				mode = p.Mode
			}
			if mode != "ask" && mode != "auto" {
				return p, ErrInvalid
			}
			if err = m.health(ctx, p); err != nil {
				m.report(err)
				_, storeErr := m.db().ExecContext(ctx, `UPDATE bench_plugins SET last_error='Runtime health check failed' WHERE id=?`, id)
				return p, errors.Join(ErrUnavailable, storeErr)
			}
			if err = m.record(ctx, actor, "plugin.enable_requested", id, map[string]string{"mode": mode}); err != nil {
				return p, err
			}
			_, err = m.db().ExecContext(ctx, `UPDATE bench_plugins SET enabled=1,mode=?,last_error='' WHERE id=?`, mode, id)
		case "disable":
			if err = m.record(ctx, actor, "plugin.disable_requested", id, nil); err != nil {
				return p, err
			}
			_, err = m.db().ExecContext(ctx, `UPDATE bench_plugins SET enabled=0 WHERE id=?`, id)
			if err == nil {
				err = m.settleQueued(ctx, id, "cancelled", "integration_disabled")
			}
		case "upgrade":
			err = m.upgradePlugin(ctx, actor, p)
		case "restart", "uninstall":
			var count int
			err = m.db().QueryRowContext(ctx, `SELECT count(*) FROM bench_jobs j JOIN sessions s ON s.id=j.session_id WHERE j.plugin_id=? AND s.state IN ('queued','running','pending_approval')`, id).Scan(&count)
			if err != nil {
				return p, err
			}
			if count > 0 {
				return p, ErrBusy
			}
			if action == "restart" {
				if !p.Enabled {
					return p, ErrUnavailable
				}
				err = m.health(ctx, p)
				if err != nil {
					m.report(err)
					_, storeErr := m.db().ExecContext(ctx, `UPDATE bench_plugins SET last_error='Runtime health check failed' WHERE id=?`, id)
					return p, errors.Join(ErrUnavailable, storeErr)
				}
				_, err = m.db().ExecContext(ctx, `UPDATE bench_plugins SET last_error='' WHERE id=?`, id)
			} else {
				root, pathErr := m.packageRoot(id)
				if pathErr != nil {
					return p, pathErr
				}
				if err = m.record(ctx, actor, "plugin.uninstall_requested", id, nil); err != nil {
					return p, err
				}
				if err = os.RemoveAll(root); err != nil {
					return p, err
				}
				_, err = m.db().ExecContext(ctx, `UPDATE bench_plugins SET installed=0,enabled=0,devices_json='[]',last_error='' WHERE id=?`, id)
			}
		default:
			return p, ErrInvalid
		}
		if err != nil {
			return p, err
		}
	}
	if err = m.record(ctx, actor, "plugin."+action, id, map[string]string{"outcome": "applied"}); err != nil {
		return p, err
	}
	m.signal()
	return m.loadPlugin(ctx, id)
}

func (m *Manager) Discover(ctx context.Context, actor identity.Actor, id string) ([]Device, error) {
	if err := require(actor, auth.PermissionBenchExecute); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cameraBusy(id) {
		return nil, ErrBusy
	}
	p, err := m.loadPlugin(ctx, id)
	if err != nil {
		return nil, err
	}
	if !p.Installed || !p.Enabled {
		return nil, ErrUnavailable
	}
	for _, run := range m.active {
		if strings.HasPrefix(run.resource, id+"/") {
			return nil, ErrBusy
		}
	}
	runner, err := m.process(p)
	if err != nil {
		return nil, err
	}
	defer runner.Shutdown(context.Background())
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = m.record(ctx, actor, "plugin.discover_requested", id, nil); err != nil {
		return nil, err
	}
	result, err := runner.Execute(ctx, protocol.ExecuteRequest{Type: "execute", Operation: "discover_devices", Parameters: json.RawMessage(`{}`)}, discardEvents{})
	if err != nil {
		return nil, err
	}
	if result.Status != "completed" {
		return nil, ErrUnavailable
	}
	if id == "serial" {
		return m.reconcileSerialDiscovery(ctx, result.Data)
	}
	if id == "camera" {
		if _, err := m.reconcileCameraDiscovery(ctx, result.Data); err != nil {
			return nil, err
		}
		return m.initializeDiscoveredCameraRecords(ctx, p)
	}
	var data struct {
		Devices    []Device `json:"devices"`
		Simulation bool     `json:"simulation"`
	}
	if strict(result.Data, &data) != nil {
		return nil, ErrInvalid
	}
	for _, device := range data.Devices {
		if !connectionPattern.MatchString(device.Connection) || device.ID != "atk-dl16-"+device.Connection || device.Driver != "atk-dl16" || len(device.Channels) != 16 {
			return nil, ErrInvalid
		}
		for i, ch := range device.Channels {
			if ch != fmt.Sprintf("D%d", i) {
				return nil, ErrInvalid
			}
		}
	}
	if data.Devices == nil {
		data.Devices = []Device{}
	}
	raw, _ := json.Marshal(data.Devices)
	_, err = m.db().ExecContext(ctx, `UPDATE bench_plugins SET devices_json=? WHERE id=?`, raw, id)
	return data.Devices, err
}

type discardEvents struct{}

func (discardEvents) Emit(context.Context, protocol.Envelope) error { return nil }
