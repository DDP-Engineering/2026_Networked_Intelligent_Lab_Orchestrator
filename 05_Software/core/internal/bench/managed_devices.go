package bench

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/manifest"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/store/sqlite"
)

func (manager *Manager) runnerConfiguration(ctx context.Context, pluginID, deviceID string) (json.RawMessage, error) {
	plugin, err := sqlite.NewPluginConfigurationRepository(manager.options.Database).Get(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	var device any
	if deviceID != "" {
		record, err := managedRecord(ctx, manager, pluginID, deviceID)
		if err != nil || record.PluginID != pluginID {
			return nil, ErrNotFound
		}
		device = map[string]any{"id": record.ID, "revision": record.Revision, "settings": record.Settings}
	}
	return json.Marshal(map[string]any{"schema_version": 1, "plugin": map[string]any{"revision": plugin.Revision, "settings": plugin.Settings}, "device": device})
}

// ConfigurePlugin stores package settings only after validating the installed schema.
func (manager *Manager) ConfigurePlugin(ctx context.Context, actor identity.Actor, pluginID string, revision int64, settings json.RawMessage) (sqlite.PluginConfiguration, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return sqlite.PluginConfiguration{}, err
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.serialBusy(pluginID) || manager.cameraBusy(pluginID) {
		return sqlite.PluginConfiguration{}, ErrBusy
	}
	plugin, err := manager.loadPlugin(ctx, pluginID)
	if err != nil {
		return sqlite.PluginConfiguration{}, err
	}
	if !plugin.Installed {
		return sqlite.PluginConfiguration{}, ErrUnavailable
	}
	root, err := manager.packageRoot(pluginID)
	if err != nil {
		return sqlite.PluginConfiguration{}, err
	}
	descriptor, err := manifest.Load(root)
	if err != nil || descriptor.AppConfigurationSchema == "" {
		return sqlite.PluginConfiguration{}, ErrUnavailable
	}
	if err := manifest.ValidateConfiguration(root, descriptor.AppConfigurationSchema, settings); err != nil {
		return sqlite.PluginConfiguration{}, ErrInvalid
	}
	if err := manager.record(ctx, actor, "plugin.configure_requested", pluginID, map[string]any{"revision": revision}); err != nil {
		return sqlite.PluginConfiguration{}, err
	}
	updated, err := sqlite.NewPluginConfigurationRepository(manager.options.Database).Put(ctx, pluginID, revision, settings, manager.now())
	if errors.Is(err, sqlite.ErrManagedDeviceConflict) {
		return sqlite.PluginConfiguration{}, ErrConflict
	}
	if err != nil {
		return sqlite.PluginConfiguration{}, ErrInvalid
	}
	if err := manager.record(ctx, actor, "plugin.configured", pluginID, map[string]any{"revision": updated.Revision}); err != nil {
		return sqlite.PluginConfiguration{}, err
	}
	return updated, nil
}

func (manager *Manager) PluginConfiguration(ctx context.Context, actor identity.Actor, pluginID string) (sqlite.PluginConfiguration, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return sqlite.PluginConfiguration{}, err
	}
	plugin, err := manager.loadPlugin(ctx, pluginID)
	if err != nil {
		return sqlite.PluginConfiguration{}, err
	}
	if !plugin.Installed {
		return sqlite.PluginConfiguration{}, ErrUnavailable
	}
	return sqlite.NewPluginConfigurationRepository(manager.options.Database).Get(ctx, pluginID)
}

// ConfigureDevice accepts a revisioned camera configuration, then establishes
// its control baseline before allowing any user job to run on that device.
func (manager *Manager) ConfigureDevice(ctx context.Context, actor identity.Actor, pluginID, deviceID string, revision int64, name, purpose string, settings json.RawMessage) (Device, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return Device{}, err
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.serialBusy(deviceID) || manager.cameraBusy(deviceID) {
		return Device{}, ErrBusy
	}
	plugin, err := manager.loadPlugin(ctx, pluginID)
	if err != nil {
		return Device{}, err
	}
	if !plugin.Installed || !plugin.Enabled || (pluginID != "camera" && pluginID != "serial") {
		return Device{}, ErrUnavailable
	}
	root, err := manager.packageRoot(pluginID)
	if err != nil {
		return Device{}, err
	}
	descriptor, err := manifest.Load(root)
	if err != nil || descriptor.DeviceConfigurationSchema == "" {
		return Device{}, ErrUnavailable
	}
	if err := manifest.ValidateConfiguration(root, descriptor.DeviceConfigurationSchema, settings); err != nil {
		return Device{}, ErrInvalid
	}
	repository := sqlite.NewManagedDeviceRepository(manager.options.Database)
	if err := manager.record(ctx, actor, "device.configure_requested", pluginID+"/"+deviceID, map[string]any{"revision": revision}); err != nil {
		return Device{}, err
	}
	record, err := repository.Configure(ctx, pluginID, deviceID, revision, name, purpose, settings)
	if errors.Is(err, sqlite.ErrManagedDeviceConflict) {
		return Device{}, ErrConflict
	}
	if err != nil {
		return Device{}, ErrInvalid
	}
	if record.Presence != "present" || record.EndpointID == "" || !record.Enabled {
		return managedCameraDevices([]sqlite.ManagedDeviceRecord{record})[0], nil
	}
	if pluginID == "serial" {
		ready, err := repository.MarkReady(ctx, pluginID, deviceID, record.Revision)
		if err != nil {
			return Device{}, err
		}
		return managedCameraDevices([]sqlite.ManagedDeviceRecord{ready})[0], nil
	}
	runner, err := manager.processConfigured(plugin, deviceID)
	if err != nil {
		_ = repository.MarkFailed(ctx, pluginID, deviceID, record.Revision)
		return Device{}, err
	}
	defer runner.Shutdown(context.Background())
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(settings, &fields)
	parameters := map[string]any{
		"device_id":         record.ID,
		"endpoint_id":       record.EndpointID,
		"hardware_identity": map[string]string{"vendor": record.Vendor, "product": record.Product, "serial": record.Serial, "port_path": record.PortPath},
		"baseline_controls": json.RawMessage(`{}`),
	}
	if raw := fields["baseline_controls"]; len(raw) > 0 {
		parameters["baseline_controls"] = raw
	}
	raw, _ := json.Marshal(parameters)
	resetCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := runner.Execute(resetCtx, protocol.ExecuteRequest{Type: "execute", Operation: "reset_controls", Parameters: raw}, discardEvents{})
	if err != nil || result.Status != "completed" {
		_ = repository.MarkFailed(ctx, pluginID, deviceID, record.Revision)
		return Device{}, ErrUnavailable
	}
	ready, err := repository.MarkReady(ctx, pluginID, deviceID, record.Revision)
	if err != nil {
		return Device{}, err
	}
	if err := manager.record(ctx, actor, "device.configured", pluginID+"/"+deviceID, map[string]any{"revision": ready.Revision, "readiness": ready.Readiness}); err != nil {
		return Device{}, err
	}
	return managedCameraDevices([]sqlite.ManagedDeviceRecord{ready})[0], nil
}

// ListManagedDevices exposes operational records while retaining hardware binding details in the core.
func (manager *Manager) ListManagedDevices(ctx context.Context, actor identity.Actor, pluginID string) ([]Device, error) {
	if err := require(actor, auth.PermissionBenchRead); err != nil {
		return nil, err
	}
	if pluginID != "camera" && pluginID != "serial" {
		return nil, ErrNotFound
	}
	plugin, err := manager.loadPlugin(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	if !plugin.Installed {
		return nil, ErrUnavailable
	}
	return plugin.Devices, nil
}

// RebindDevice transfers one discovered provisional binding to its absent record.
func (manager *Manager) RebindDevice(ctx context.Context, actor identity.Actor, pluginID, deviceID string, revision int64, provisionalID string, provisionalRevision int64, confirmation string) (Device, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return Device{}, err
	}
	if (pluginID != "camera" && pluginID != "serial") || confirmation != deviceID {
		return Device{}, ErrInvalid
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.serialBusy(deviceID) || manager.cameraBusy(deviceID) {
		return Device{}, ErrBusy
	}
	plugin, err := manager.loadPlugin(ctx, pluginID)
	if err != nil {
		return Device{}, err
	}
	if !plugin.Installed || !plugin.Enabled {
		return Device{}, ErrUnavailable
	}
	if err := manager.record(ctx, actor, "device.rebind_requested", pluginID+"/"+deviceID, map[string]any{"revision": revision, "provisional_id": provisionalID}); err != nil {
		return Device{}, err
	}
	record, err := sqlite.NewManagedDeviceRepository(manager.options.Database).Rebind(ctx, pluginID, deviceID, revision, provisionalID, provisionalRevision)
	if errors.Is(err, sqlite.ErrManagedDeviceConflict) {
		return Device{}, ErrConflict
	}
	if err != nil {
		return Device{}, err
	}
	if err := manager.record(ctx, actor, "device.rebound", pluginID+"/"+deviceID, map[string]any{"old_revision": revision, "revision": record.Revision, "provisional_id": provisionalID}); err != nil {
		return Device{}, err
	}
	if record.Enabled && record.EndpointID != "" {
		if err := manager.initializeDevice(ctx, plugin, record); err != nil {
			manager.report(err)
			_ = sqlite.NewManagedDeviceRepository(manager.options.Database).MarkFailed(ctx, plugin.ID, record.ID, record.Revision)
		}
	}
	current, err := sqlite.NewManagedDeviceRepository(manager.options.Database).List(ctx, pluginID)
	if err != nil {
		return Device{}, err
	}
	for _, item := range current {
		if item.ID == deviceID {
			return managedCameraDevices([]sqlite.ManagedDeviceRecord{item})[0], nil
		}
	}
	return Device{}, ErrNotFound
}

// ForgetDevice tombstones a camera record after an exact-ID confirmation.
func (manager *Manager) ForgetDevice(ctx context.Context, actor identity.Actor, pluginID, deviceID string, revision int64, confirmation string) error {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return err
	}
	if (pluginID != "camera" && pluginID != "serial") || confirmation != deviceID {
		return ErrInvalid
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.serialBusy(deviceID) || manager.cameraBusy(deviceID) {
		return ErrBusy
	}
	if err := manager.record(ctx, actor, "device.forget_requested", pluginID+"/"+deviceID, map[string]any{"revision": revision}); err != nil {
		return err
	}
	err := sqlite.NewManagedDeviceRepository(manager.options.Database).Forget(ctx, pluginID, deviceID, revision)
	if errors.Is(err, sqlite.ErrManagedDeviceConflict) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if pluginID == "serial" {
		delete(manager.serials, deviceID)
	}
	return manager.record(ctx, actor, "device.forgotten", pluginID+"/"+deviceID, map[string]any{"old_revision": revision, "revision": revision + 1})
}

// SetDeviceEnabled changes admission and reapplies the baseline before re-enabling work.
func (manager *Manager) SetDeviceEnabled(ctx context.Context, actor identity.Actor, pluginID, deviceID string, revision int64, enabled bool) (Device, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return Device{}, err
	}
	if pluginID != "camera" && pluginID != "serial" {
		return Device{}, ErrNotFound
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if (manager.serialBusy(deviceID) && (pluginID != "serial" || enabled)) || manager.cameraBusy(deviceID) {
		return Device{}, ErrBusy
	}
	plugin, err := manager.loadPlugin(ctx, pluginID)
	if err != nil {
		return Device{}, err
	}
	if !plugin.Installed || !plugin.Enabled {
		return Device{}, ErrUnavailable
	}
	if err := manager.record(ctx, actor, "device.enable_requested", pluginID+"/"+deviceID, map[string]any{"revision": revision, "enabled": enabled}); err != nil {
		return Device{}, err
	}
	repository := sqlite.NewManagedDeviceRepository(manager.options.Database)
	record, err := repository.SetEnabled(ctx, pluginID, deviceID, revision, enabled)
	if errors.Is(err, sqlite.ErrManagedDeviceConflict) {
		return Device{}, ErrConflict
	}
	if err != nil {
		return Device{}, err
	}
	if pluginID == "serial" && !enabled {
		manager.stopSerialLocked(deviceID, "device_disabled")
	}
	if enabled && record.Presence == "present" && record.EndpointID != "" {
		if err := manager.initializeDevice(ctx, plugin, record); err != nil {
			manager.report(err)
			_ = repository.MarkFailed(ctx, pluginID, deviceID, record.Revision)
		}
	}
	if err := manager.record(ctx, actor, "device.enabled_changed", pluginID+"/"+deviceID, map[string]any{"old_revision": revision, "enabled": enabled}); err != nil {
		return Device{}, err
	}
	current, err := repository.List(ctx, pluginID)
	if err != nil {
		return Device{}, err
	}
	for _, item := range current {
		if item.ID == deviceID {
			return managedCameraDevices([]sqlite.ManagedDeviceRecord{item})[0], nil
		}
	}
	return Device{}, ErrNotFound
}

// RetryDevice repeats the configured reset for a failed present camera.
func (manager *Manager) RetryDevice(ctx context.Context, actor identity.Actor, pluginID, deviceID string, revision int64) (Device, error) {
	if err := require(actor, auth.PermissionPluginsManage); err != nil {
		return Device{}, err
	}
	if pluginID != "camera" && pluginID != "serial" {
		return Device{}, ErrNotFound
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.serialBusy(deviceID) || manager.cameraBusy(deviceID) {
		return Device{}, ErrBusy
	}
	plugin, err := manager.loadPlugin(ctx, pluginID)
	if err != nil {
		return Device{}, err
	}
	if !plugin.Installed || !plugin.Enabled {
		return Device{}, ErrUnavailable
	}
	if err := manager.record(ctx, actor, "device.retry_requested", pluginID+"/"+deviceID, map[string]any{"revision": revision}); err != nil {
		return Device{}, err
	}
	repository := sqlite.NewManagedDeviceRepository(manager.options.Database)
	record, err := repository.MarkInitializing(ctx, pluginID, deviceID, revision)
	if errors.Is(err, sqlite.ErrManagedDeviceConflict) {
		return Device{}, ErrConflict
	}
	if err != nil {
		return Device{}, err
	}
	if err := manager.initializeDevice(ctx, plugin, record); err != nil {
		_ = repository.MarkFailed(ctx, pluginID, deviceID, record.Revision)
		return Device{}, ErrUnavailable
	}
	current, err := repository.List(ctx, pluginID)
	if err != nil {
		return Device{}, err
	}
	for _, item := range current {
		if item.ID == deviceID {
			return managedCameraDevices([]sqlite.ManagedDeviceRecord{item})[0], nil
		}
	}
	return Device{}, ErrNotFound
}

func (manager *Manager) initializeDevice(ctx context.Context, plugin Plugin, record sqlite.ManagedDeviceRecord) error {
	if plugin.ID == "serial" {
		_, err := sqlite.NewManagedDeviceRepository(manager.options.Database).MarkReady(ctx, plugin.ID, record.ID, record.Revision)
		return err
	}
	return manager.resetCameraBaseline(ctx, plugin, record)
}
