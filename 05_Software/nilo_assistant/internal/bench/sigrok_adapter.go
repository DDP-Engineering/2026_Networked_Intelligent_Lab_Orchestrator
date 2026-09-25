package bench

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"nilo.local/nilo/internal/domain"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/integration/protocol"
)

func sigrokValidateSubmission(ctx context.Context, manager *Manager, actor identity.Actor, plugin Plugin, input Submission) error {
	switch input.Operation {
	case "capture":
		for _, device := range plugin.Devices {
			if device.ID == input.DeviceID {
				return nil
			}
		}
		return ErrNotFound
	case "decode_uart":
		var parameters decodeInput
		if err := json.Unmarshal(input.Parameters, &parameters); err != nil {
			return ErrInvalid
		}
		source, err := manager.job(ctx, parameters.CaptureID)
		if err != nil {
			return err
		}
		if !canRead(actor, source) {
			return ErrForbidden
		}
		if source.State != "completed" || source.ArtifactID == "" || source.Submission.Operation != "capture" {
			return ErrInvalid
		}
		var captured captureInput
		if err := json.Unmarshal(source.Submission.Parameters, &captured); err != nil {
			return ErrInvalid
		}
		for _, channel := range captured.Channels {
			if channel == parameters.Channel {
				return nil
			}
		}
		return ErrInvalid
	default:
		return ErrInvalid
	}
}

// sigrokPrepareExecution supplies only core-owned paths and a discovered
// connection. Clients cannot choose either value in their submission.
func sigrokPrepareExecution(ctx context.Context, r *jobRunner, request protocol.ExecuteRequest) (protocol.ExecuteRequest, error) {
	var parameters map[string]any
	if err := json.Unmarshal(request.Parameters, &parameters); err != nil {
		return request, err
	}
	switch request.Operation {
	case "capture":
		conn := ""
		for _, device := range r.plugin.Devices {
			if device.ID == r.job.Submission.DeviceID {
				conn = device.Connection
			}
		}
		if !connectionPattern.MatchString(conn) {
			return request, ErrNotFound
		}
		parameters["conn"] = conn
		parameters["staging_directory"] = r.stage
	case "decode_uart":
		var input decodeInput
		if strict(request.Parameters, &input) != nil {
			return request, ErrInvalid
		}
		source, err := r.manager.job(ctx, input.CaptureID)
		if err != nil {
			return request, err
		}
		artifactID, err := domain.ParseArtifactID(source.ArtifactID)
		if err != nil {
			return request, err
		}
		content, _, err := r.manager.artifacts.Open(ctx, artifactID)
		if err != nil {
			return request, err
		}
		path := filepath.Join(r.stage, "input.sr")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			content.Close()
			return request, err
		}
		_, copyErr := io.Copy(file, content)
		err = errors.Join(copyErr, file.Close(), content.Close())
		if err != nil {
			return request, err
		}
		parameters = map[string]any{"input_path": path, "channel": input.Channel, "baudrate": input.Baudrate}
	default:
		return request, ErrInvalid
	}
	request.Parameters, _ = json.Marshal(parameters)
	return request, nil
}
