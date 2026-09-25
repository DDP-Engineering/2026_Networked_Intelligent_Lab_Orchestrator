package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/store/sqlite"
)

// discoverCamera reconciles physical presence and resets a baseline on reconnect.
// The caller holds the manager lock so user submissions cannot overtake a reset.
func (manager *Manager) discoverCamera(ctx context.Context, plugin Plugin) ([]Device, error) {
	runner, err := manager.process(plugin)
	if err != nil {
		return nil, err
	}
	defer runner.Shutdown(context.Background())
	result, err := runner.Execute(ctx, protocol.ExecuteRequest{Type: "execute", Operation: "discover_devices", Parameters: json.RawMessage(`{}`)}, discardEvents{})
	if err != nil || result.Status != "completed" {
		if err != nil {
			return nil, err
		}
		return nil, ErrUnavailable
	}
	if _, err := manager.reconcileCameraDiscovery(ctx, result.Data); err != nil {
		return nil, err
	}
	return manager.initializeDiscoveredCameraRecords(ctx, plugin)
}

func (manager *Manager) initializeDiscoveredCameraRecords(ctx context.Context, plugin Plugin) ([]Device, error) {
	records, err := sqlite.NewManagedDeviceRepository(manager.options.Database).List(ctx, "camera")
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if !record.Configured || !record.Enabled || record.Presence != "present" || record.Readiness != "initializing" || record.EndpointID == "" {
			continue
		}
		if err := manager.resetCameraBaseline(ctx, plugin, record); err != nil {
			manager.report(fmt.Errorf("initialize camera %s: %w", record.ID, err))
			if markErr := sqlite.NewManagedDeviceRepository(manager.options.Database).MarkFailed(ctx, plugin.ID, record.ID, record.Revision); markErr != nil {
				manager.report(fmt.Errorf("mark camera %s failed: %w", record.ID, markErr))
			}
		}
	}
	records, err = sqlite.NewManagedDeviceRepository(manager.options.Database).List(ctx, "camera")
	if err != nil {
		return nil, err
	}
	return managedCameraDevices(records), nil
}

func (manager *Manager) resetCameraBaseline(ctx context.Context, plugin Plugin, record sqlite.ManagedDeviceRecord) error {
	subject := plugin.ID + "/" + record.ID
	if err := manager.options.Audit.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: audit.ActorKindSystem, Action: "device.initialize_requested", SubjectType: "integration", SubjectID: subject, Outcome: audit.OutcomeRequested, Details: map[string]any{"revision": record.Revision}}); err != nil {
		return err
	}
	runner, err := manager.processConfigured(plugin, record.ID)
	if err != nil {
		return err
	}
	defer runner.Shutdown(context.Background())
	var settings map[string]json.RawMessage
	if json.Unmarshal(record.Settings, &settings) != nil {
		return ErrInvalid
	}
	parameters := map[string]any{
		"device_id": record.ID, "endpoint_id": record.EndpointID,
		"hardware_identity": map[string]string{"vendor": record.Vendor, "product": record.Product, "serial": record.Serial, "port_path": record.PortPath},
		"baseline_controls": json.RawMessage(`{}`),
	}
	if baseline := settings["baseline_controls"]; len(baseline) > 0 {
		parameters["baseline_controls"] = baseline
	}
	raw, _ := json.Marshal(parameters)
	resetCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := runner.Execute(resetCtx, protocol.ExecuteRequest{Type: "execute", Operation: "reset_controls", Parameters: raw}, discardEvents{})
	if err != nil || result.Status != "completed" {
		if err != nil {
			return err
		}
		return ErrUnavailable
	}
	if _, err := sqlite.NewManagedDeviceRepository(manager.options.Database).MarkReady(ctx, plugin.ID, record.ID, record.Revision); err != nil {
		return err
	}
	return manager.options.Audit.Record(ctx, audit.Record{Category: audit.CategoryAudit, Component: "bench", ActorKind: audit.ActorKindSystem, Action: "device.initialized", SubjectType: "integration", SubjectID: subject, Outcome: audit.OutcomeApplied, Details: map[string]any{"revision": record.Revision + 1}})
}
