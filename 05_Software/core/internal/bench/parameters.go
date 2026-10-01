package bench

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
)

var connectionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
var channelPattern = regexp.MustCompile(`^D([0-9]|1[0-5])$`)
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

type captureInput struct {
	Channels   []string `json:"channels"`
	Samplerate int      `json:"samplerate"`
	Samples    int      `json:"samples"`
}
type decodeInput struct {
	CaptureID string `json:"capture_id"`
	Channel   string `json:"channel"`
	Baudrate  int    `json:"baudrate"`
}

// canonicalSubmission rejects unexposed capabilities before policy, queuing or
// subprocess creation. Only the core can supply paths and USB connections.
func canonicalSubmission(input Submission) (Submission, error) {
	return canonicalSubmissionWithLimits(input, 60, 300)
}

func canonicalSubmissionWithLimits(input Submission, defaultSeconds, maximumSeconds int) (Submission, error) {
	if !keyPattern.MatchString(input.IdempotencyKey) {
		return input, ErrInvalid
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = defaultSeconds
	}
	if input.TimeoutSeconds < 1 || input.TimeoutSeconds > maximumSeconds {
		return input, ErrInvalid
	}
	adapter, err := adapterFor(input.PluginID)
	if err != nil || adapter.canonicalize == nil {
		return input, ErrInvalid
	}
	return adapter.canonicalize(input)
}

func sigrokCanonicalSubmission(input Submission) (Submission, error) {
	var value any
	switch input.Operation {
	case "capture":
		var p captureInput
		if strict(input.Parameters, &p) != nil || len(p.Channels) == 0 || len(p.Channels) > 16 || p.Samples < 1 || p.Samples > 1000000 || !slices.Contains([]int{1000000, 2000000, 4000000, 5000000, 10000000}, p.Samplerate) || input.DeviceID == "" {
			return input, ErrInvalid
		}
		seen := map[string]bool{}
		for _, ch := range p.Channels {
			if !channelPattern.MatchString(ch) || seen[ch] {
				return input, ErrInvalid
			}
			seen[ch] = true
		}
		value = p
	case "decode_uart":
		var p decodeInput
		if strict(input.Parameters, &p) != nil || p.CaptureID == "" || !channelPattern.MatchString(p.Channel) || p.Baudrate < 1 || p.Baudrate > 4000000 || input.DeviceID != "" {
			return input, ErrInvalid
		}
		value = p
	default:
		return input, ErrInvalid
	}
	input.Parameters, _ = json.Marshal(value)
	return input, nil
}

func resourceFor(job Job) string {
	adapter, err := adapterFor(job.PluginID)
	if err != nil || adapter.resource == nil {
		return ""
	}
	return adapter.resource(job)
}

func sigrokResource(job Job) string {
	if job.Submission.Operation == "capture" {
		return fmt.Sprintf("%s/device/%s", job.PluginID, job.Submission.DeviceID)
	}
	return ""
}
