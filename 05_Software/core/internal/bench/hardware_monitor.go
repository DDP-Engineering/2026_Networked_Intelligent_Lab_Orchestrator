package bench

import (
	"context"
	"encoding/json"
	"time"

	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/store/sqlite"
)

// monitorHardware refreshes physical inventory independently of browser sessions.
// Discovery only reads hardware metadata; acquisition still requires approval.
func (m *Manager) monitorHardware(ctx context.Context) {
	defer close(m.hardwareDone)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			for _, id := range []string{"camera", "serial"} {
				m.refreshHardware(ctx, id)
			}
		}
	}
}

func (m *Manager) refreshHardware(ctx context.Context, id string) {
	m.mu.Lock()
	p, err := m.loadPlugin(ctx, id)
	if m.closed || err != nil || !p.Installed || !p.Enabled || p.Status != "ready" {
		m.mu.Unlock()
		return
	}
	runner, err := m.process(p)
	m.mu.Unlock()
	var result protocol.ExecuteResult
	if err == nil {
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		result, err = runner.Execute(bounded, protocol.ExecuteRequest{Type: "execute", Operation: "discover_devices", Parameters: json.RawMessage(`{}`)}, discardEvents{})
		cancel()
		shutdown, finish := context.WithTimeout(context.Background(), 3*time.Second)
		_ = runner.Shutdown(shutdown)
		finish()
		if err == nil && result.Status != "completed" {
			err = ErrUnavailable
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || ctx.Err() != nil {
		return
	}
	current, loadErr := m.loadPlugin(ctx, id)
	// Discard observations from an instance disabled or replaced during the read.
	if loadErr != nil || !current.Enabled || !current.Installed || current.InstanceID != p.InstanceID || current.Digest != p.Digest {
		return
	}
	if err == nil {
		if id == "camera" {
			_, err = m.reconcileCameraDiscovery(ctx, result.Data)
			if err == nil {
				bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
				_, err = m.initializeDiscoveredCameraRecords(bounded, current)
				cancel()
			}
		} else {
			_, err = m.reconcileSerialDiscovery(ctx, result.Data)
		}
	}
	if err != nil {
		m.report(err)
		m.report(sqlite.NewManagedDeviceRepository(m.options.Database).MarkUnknown(ctx, id))
	}
}

// markHardwareFailed is called with the manager lock held only for device or
// transport failures. Normal stops and invalid user parameters keep readiness.
func (m *Manager) markHardwareFailed(ctx context.Context, plugin, id string) {
	record, err := managedRecord(ctx, m, plugin, id)
	if err != nil {
		m.report(err)
		return
	}
	if record.Configured && record.Readiness != "failed" {
		m.report(sqlite.NewManagedDeviceRepository(m.options.Database).MarkFailed(ctx, plugin, id, record.Revision))
	}
}
