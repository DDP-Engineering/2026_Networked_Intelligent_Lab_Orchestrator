package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"

	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
	"nilo.local/nilo/internal/store/sqlite"
)

var cameraIDPattern = regexp.MustCompile(`^cam-[1-9][0-9]*$`)
var cameraEndpointPattern = regexp.MustCompile(`^video[0-9]+$`)
var cameraPortPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var cameraUSBIDPattern = regexp.MustCompile(`^[A-Fa-f0-9]{4}$`)

type cameraDiscovery struct {
	Devices []struct {
		ID               string `json:"id"`
		Vendor           string `json:"vendor"`
		Product          string `json:"product"`
		Serial           string `json:"serial"`
		PortPath         string `json:"port_path"`
		DisplayName      string `json:"display_name"`
		SelectedEndpoint string `json:"selected_endpoint"`
		UnusableReason   string `json:"unusable_reason"`
		Endpoints        []struct {
			ID             string             `json:"id"`
			Interface      string             `json:"interface"`
			Capture        bool               `json:"capture"`
			Formats        []string           `json:"formats"`
			Sizes          map[string][][]int `json:"sizes"`
			FrameIntervals []struct {
				Format      string `json:"format"`
				Width       int    `json:"width"`
				Height      int    `json:"height"`
				Numerator   int    `json:"numerator"`
				Denominator int    `json:"denominator"`
			} `json:"frame_intervals"`
		} `json:"endpoints"`
		Controls []json.RawMessage `json:"controls"`
	} `json:"devices"`
}

func managedCameraDevices(records []sqlite.ManagedDeviceRecord) []Device {
	devices := make([]Device, 0, len(records))
	for _, record := range records {
		devices = append(devices, Device{ID: record.ID, DisplayName: record.DisplayName, Kind: "camera", Purpose: record.Purpose, Presence: record.Presence, Readiness: record.Readiness, Configured: record.Configured, Enabled: record.Enabled, Revision: record.Revision, Capabilities: record.Capabilities, Settings: record.Settings, Channels: []string{}, SampleRates: []int{}})
	}
	return devices
}

func (manager *Manager) reconcileCameraDiscovery(ctx context.Context, raw json.RawMessage) ([]Device, error) {
	var data cameraDiscovery
	if strict(raw, &data) != nil || len(data.Devices) > 32 {
		return nil, ErrInvalid
	}
	observations := make([]sqlite.ManagedDeviceObservation, 0, len(data.Devices))
	for _, item := range data.Devices {
		if !cameraUSBIDPattern.MatchString(item.Vendor) || !cameraUSBIDPattern.MatchString(item.Product) || !cameraPortPattern.MatchString(item.PortPath) || len(item.Serial) > 128 || len(item.Endpoints) > 16 {
			return nil, ErrInvalid
		}
		selected := ""
		for _, endpoint := range item.Endpoints {
			if !cameraEndpointPattern.MatchString(endpoint.ID) {
				return nil, ErrInvalid
			}
			if len(endpoint.FrameIntervals) > 1024 {
				return nil, ErrInvalid
			}
			for _, interval := range endpoint.FrameIntervals {
				if !slices.Contains(endpoint.Formats, interval.Format) || interval.Width < 1 || interval.Width > 4096 || interval.Height < 1 || interval.Height > 4096 || interval.Width*interval.Height > 2_000_000 || interval.Numerator < 1 || interval.Numerator > 10000 || interval.Denominator < 1 || interval.Denominator > 10000 {
					return nil, ErrInvalid
				}
			}
			if endpoint.ID == item.SelectedEndpoint && endpoint.Capture && (slices.Contains(endpoint.Formats, "MJPG") || slices.Contains(endpoint.Formats, "YUYV")) {
				selected = endpoint.ID
			}
		}
		if item.SelectedEndpoint != "" && selected == "" {
			return nil, ErrInvalid
		}
		endpoints, _ := json.Marshal(item.Endpoints)
		formats := []string{}
		sizes := map[string][][]int{}
		var frameIntervals any = []any{}
		for _, endpoint := range item.Endpoints {
			if endpoint.ID == selected {
				formats = endpoint.Formats
				sizes = endpoint.Sizes
				frameIntervals = endpoint.FrameIntervals
			}
		}
		capabilities, _ := json.Marshal(map[string]any{"formats": formats, "sizes": sizes, "frame_intervals": frameIntervals, "controls": item.Controls, "unusable_reason": item.UnusableReason})
		observations = append(observations, sqlite.ManagedDeviceObservation{Kind: "camera", Vendor: item.Vendor, Product: item.Product, Serial: item.Serial, PortPath: item.PortPath, EndpointID: selected, Endpoints: endpoints, Capabilities: capabilities})
	}
	records, err := sqlite.NewManagedDeviceRepository(manager.options.Database).Reconcile(ctx, "camera", observations, manager.now())
	if err != nil {
		return nil, err
	}
	return managedCameraDevices(records), nil
}

func cameraCanonicalSubmission(input Submission) (Submission, error) {
	if !cameraIDPattern.MatchString(input.DeviceID) {
		return input, ErrInvalid
	}
	var value map[string]json.RawMessage
	if strict(input.Parameters, &value) != nil || value == nil {
		return input, ErrInvalid
	}
	allowed := map[string]bool{"width": true, "height": true, "format": true, "frame_interval": true, "controls": true, "processing": true, "encoding": true, "jpeg_quality": true, "maximum_dimension": true}
	switch input.Operation {
	case "snapshot":
		for key := range value {
			if !allowed[key] {
				return input, ErrInvalid
			}
		}
	case "get_controls":
		if len(value) != 0 {
			return input, ErrInvalid
		}
	case "set_controls":
		if len(value) != 1 || value["controls"] == nil {
			return input, ErrInvalid
		}
		var controls map[string]json.RawMessage
		if json.Unmarshal(value["controls"], &controls) != nil || len(controls) == 0 || len(controls) > 64 {
			return input, ErrInvalid
		}
	case "reset_controls":
		if len(value) > 1 {
			return input, ErrInvalid
		}
		if raw := value["factory"]; raw != nil {
			var flag bool
			if json.Unmarshal(raw, &flag) != nil || string(raw) != "true" && string(raw) != "false" {
				return input, ErrInvalid
			}
		}
	default:
		return input, ErrInvalid
	}
	input.Parameters, _ = json.Marshal(value)
	return input, nil
}

func cameraResource(job Job) string {
	return fmt.Sprintf("camera/device/%s", job.Submission.DeviceID)
}

func cameraRunnerArguments(_ string, _ Options, arguments []string) []string {
	return arguments
}

func cameraArtifactFile(id, mediaType string) (string, string, error) {
	if id != "image" {
		return "", "", ErrInvalid
	}
	switch mediaType {
	case "image/jpeg":
		return "image.jpg", "snapshot.jpg", nil
	case "image/png":
		return "image.png", "snapshot.png", nil
	default:
		return "", "", ErrInvalid
	}
}

func cameraRecord(ctx context.Context, manager *Manager, id string) (sqlite.ManagedDeviceRecord, error) {
	records, err := sqlite.NewManagedDeviceRepository(manager.options.Database).List(ctx, "camera")
	if err != nil {
		return sqlite.ManagedDeviceRecord{}, err
	}
	for _, record := range records {
		if record.ID == id {
			return record, nil
		}
	}
	return sqlite.ManagedDeviceRecord{}, ErrNotFound
}

func cameraValidateSubmission(ctx context.Context, manager *Manager, _ identity.Actor, _ Plugin, input Submission) error {
	record, err := cameraRecord(ctx, manager, input.DeviceID)
	if err != nil {
		return err
	}
	if !record.Configured || !record.Enabled || record.Presence != "present" || record.Readiness != "ready" {
		return ErrUnavailable
	}
	if !cameraEndpointPattern.MatchString(record.EndpointID) {
		return ErrUnavailable
	}
	return nil
}

func cameraPrepareExecution(ctx context.Context, runner *jobRunner, request protocol.ExecuteRequest) (protocol.ExecuteRequest, error) {
	record, err := cameraRecord(ctx, runner.manager, runner.job.Submission.DeviceID)
	if err != nil {
		return request, err
	}
	if !record.Configured || !record.Enabled || record.Presence != "present" || record.Readiness != "ready" || !cameraEndpointPattern.MatchString(record.EndpointID) {
		return request, ErrUnavailable
	}
	var parameters map[string]json.RawMessage
	if json.Unmarshal(request.Parameters, &parameters) != nil || parameters == nil {
		return request, ErrInvalid
	}
	parameters["device_id"], _ = json.Marshal(record.ID)
	parameters["endpoint_id"], _ = json.Marshal(record.EndpointID)
	parameters["hardware_identity"], _ = json.Marshal(map[string]string{"vendor": record.Vendor, "product": record.Product, "serial": record.Serial, "port_path": record.PortPath})
	if request.Operation == "reset_controls" {
		var settings map[string]json.RawMessage
		if json.Unmarshal(record.Settings, &settings) != nil {
			return request, ErrInvalid
		}
		if raw := settings["baseline_controls"]; len(raw) > 0 {
			parameters["baseline_controls"] = raw
		}
	}
	if request.Operation == "snapshot" {
		parameters["staging_directory"], _ = json.Marshal(runner.stage)
	}
	request.Parameters, _ = json.Marshal(parameters)
	return request, nil
}
