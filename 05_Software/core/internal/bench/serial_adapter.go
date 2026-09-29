package bench

import (
	"context"
	"encoding/json"
	"regexp"

	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/store/sqlite"
)

var serialIDPattern = regexp.MustCompile(`^serial-[1-9][0-9]*$`)
var serialEndpointPattern = regexp.MustCompile(`^tty(USB|ACM)[0-9]+$`)

func serialCanonicalSubmission(input Submission) (Submission, error) {
	var p serialConnectParameters
	if input.Operation != "connect" || !serialIDPattern.MatchString(input.DeviceID) || strict(input.Parameters, &p) != nil || p.Revision < 1 || !validSerialSettings(p.Settings) {
		return input, ErrInvalid
	}
	input.Parameters, _ = json.Marshal(p)
	return input, nil
}
func managedRecord(ctx context.Context, m *Manager, pluginID, id string) (sqlite.ManagedDeviceRecord, error) {
	records, err := sqlite.NewManagedDeviceRepository(m.options.Database).List(ctx, pluginID)
	if err != nil {
		return sqlite.ManagedDeviceRecord{}, err
	}
	for _, r := range records {
		if r.ID == id {
			return r, nil
		}
	}
	return sqlite.ManagedDeviceRecord{}, ErrNotFound
}
func serialValidateSubmission(ctx context.Context, m *Manager, actor identity.Actor, _ Plugin, input Submission) error {
	if actor.SessionID == "" && actor.TokenID == "" {
		return ErrForbidden
	}
	if m.serialBusy(input.DeviceID) {
		return ErrBusy
	}
	r, err := managedRecord(ctx, m, "serial", input.DeviceID)
	if err != nil {
		return err
	}
	if m.sharedDeviceBusyLocked(ctx, "serial", input.DeviceID) {
		return ErrBusy
	}
	var p serialConnectParameters
	_ = json.Unmarshal(input.Parameters, &p)
	if r.Revision != p.Revision {
		return ErrConflict
	}
	if !r.Enabled || !r.Configured || r.Presence != "present" || r.Readiness != "ready" {
		return ErrUnavailable
	}
	return m.serialCredentialValid(ctx, actor.User.ID, string(actor.Credential), serialCredentialID(actor))
}
func serialDiscoveryObservations(raw json.RawMessage) ([]sqlite.ManagedDeviceObservation, error) {
	var data struct {
		Devices []struct {
			ID               string `json:"id"`
			Kind             string `json:"kind"`
			DisplayName      string `json:"display_name"`
			HardwareIdentity struct {
				Vendor   string `json:"vendor"`
				Product  string `json:"product"`
				Serial   string `json:"serial"`
				PortPath string `json:"port_path"`
			} `json:"hardware_identity"`
			Capabilities json.RawMessage `json:"capabilities"`
		} `json:"devices"`
	}
	if strict(raw, &data) != nil || len(data.Devices) > 64 {
		return nil, ErrInvalid
	}
	observations := make([]sqlite.ManagedDeviceObservation, 0, len(data.Devices))
	for _, d := range data.Devices {
		h := d.HardwareIdentity
		if d.Kind != "usb-serial" || !serialEndpointPattern.MatchString(d.ID) || !cameraUSBIDPattern.MatchString(h.Vendor) || !cameraUSBIDPattern.MatchString(h.Product) || !cameraPortPattern.MatchString(h.PortPath) || len(h.Serial) > 128 {
			return nil, ErrInvalid
		}
		endpoints, _ := json.Marshal([]string{d.ID})
		if len(d.Capabilities) == 0 {
			d.Capabilities = json.RawMessage(`{}`)
		}
		observations = append(observations, sqlite.ManagedDeviceObservation{Kind: "usb-serial", Vendor: h.Vendor, Product: h.Product, Serial: h.Serial, PortPath: h.PortPath, EndpointID: d.ID, Endpoints: endpoints, Capabilities: d.Capabilities})
	}
	return observations, nil
}

func (m *Manager) reconcileSerialDiscovery(ctx context.Context, raw json.RawMessage) ([]Device, error) {
	observations, err := serialDiscoveryObservations(raw)
	if err != nil {
		return nil, err
	}
	repo := sqlite.NewManagedDeviceRepository(m.options.Database)
	records, err := repo.Reconcile(ctx, "serial", observations, m.now())
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.Enabled && r.Configured && r.Presence == "present" && r.Readiness == "initializing" {
			if _, err = repo.MarkReady(ctx, "serial", r.ID, r.Revision); err != nil {
				return nil, err
			}
		}
	}
	records, err = repo.List(ctx, "serial")
	return managedCameraDevices(records), err
}
func (m *Manager) discoverSerial(ctx context.Context, p Plugin) ([]Device, error) {
	runner, err := m.process(p)
	if err != nil {
		return nil, err
	}
	defer runner.Shutdown(context.Background())
	result, err := runner.Execute(ctx, protocol.ExecuteRequest{Type: "execute", Operation: "discover_devices", Parameters: json.RawMessage(`{}`)}, discardEvents{})
	if err != nil {
		return nil, err
	}
	if result.Status != "completed" {
		return nil, ErrUnavailable
	}
	return m.reconcileSerialDiscovery(ctx, result.Data)
}
