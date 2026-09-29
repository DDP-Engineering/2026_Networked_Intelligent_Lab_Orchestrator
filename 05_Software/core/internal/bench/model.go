// Package bench owns installed integrations and durable, queued instrument jobs.
// Hardware and vendor-specific execution remain in supervised external runners.
package bench

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrUnavailable = errors.New("integration_unavailable")
	ErrBusy        = errors.New("integration_busy")
	ErrInvalid     = errors.New("invalid_parameters")
	ErrNotFound    = errors.New("not_found")
	ErrForbidden   = errors.New("forbidden")
	ErrQueueFull   = errors.New("queue_full")
	ErrConflict    = errors.New("transaction_conflict")
)

type Device struct {
	ID             string          `json:"id"`
	Connection     string          `json:"connection"`
	DisplayName    string          `json:"display_name"`
	Kind           string          `json:"kind,omitempty"`
	Purpose        string          `json:"purpose,omitempty"`
	Presence       string          `json:"presence"`
	Readiness      string          `json:"readiness"`
	Configured     bool            `json:"configured"`
	Enabled        bool            `json:"enabled"`
	Revision       int64           `json:"revision"`
	Capabilities   json.RawMessage `json:"capabilities,omitempty"`
	Settings       json.RawMessage `json:"settings,omitempty"`
	Driver         string          `json:"driver"`
	Channels       []string        `json:"channels"`
	SampleRates    []int           `json:"samplerates"`
	MaximumSamples int             `json:"maximum_samples"`
}

type Plugin struct {
	AvailableVersion string   `json:"available_version"`
	UpdateAvailable  bool     `json:"update_available"`
	ID               string   `json:"id"`
	InstanceID       string   `json:"-"`
	DisplayName      string   `json:"display_name"`
	Version          string   `json:"version"`
	Installed        bool     `json:"installed"`
	Enabled          bool     `json:"enabled"`
	Mode             string   `json:"mode"`
	Status           string   `json:"status"`
	Simulation       bool     `json:"simulation"`
	Operations       []string `json:"operations"`
	Devices          []Device `json:"devices"`
	Error            string   `json:"error"`
	Digest           string   `json:"digest"`
}

// Submission never carries executable, filesystem, driver or arbitrary config values.
type Submission struct {
	PluginID       string          `json:"plugin_id"`
	Operation      string          `json:"operation"`
	DeviceID       string          `json:"device_id"`
	Parameters     json.RawMessage `json:"parameters"`
	IdempotencyKey string          `json:"idempotency_key"`
	TimeoutSeconds int             `json:"timeout_seconds"`
}

type Reply struct {
	Sequence      int             `json:"sequence"`
	TransactionID string          `json:"transaction_id"`
	Time          time.Time       `json:"time"`
	Payload       json.RawMessage `json:"payload"`
}

type Job struct {
	ID         string          `json:"transaction_id"`
	SessionID  string          `json:"session_id"`
	RequestID  string          `json:"request_id"`
	PluginID   string          `json:"plugin_id"`
	OwnerID    string          `json:"owner_id"`
	State      string          `json:"state"`
	Submission Submission      `json:"submission"`
	CreatedAt  time.Time       `json:"created_at"`
	Deadline   time.Time       `json:"deadline"`
	Result     json.RawMessage `json:"result"`
	Replies    []Reply         `json:"replies"`
	Error      string          `json:"error"`
	ArtifactID string          `json:"artifact_id"`
}

func terminal(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled" || state == "incomplete"
}
