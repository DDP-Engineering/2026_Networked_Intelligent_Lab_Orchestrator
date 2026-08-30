// Package protocol implements the versioned, newline-delimited integration
// runner wire protocol.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	ProtocolV1                  = "nilo.integration/1"
	MaxLineBytes                = 1 << 20
	MaxReferencedLogBytes int64 = 64 << 10
)

var (
	ErrInvalidMessage            = errors.New("invalid integration protocol message")
	ErrUnsupportedProtocol       = errors.New("unsupported integration protocol")
	ErrInvalidKind               = errors.New("invalid integration message kind")
	ErrInvalidMethod             = errors.New("invalid integration method")
	ErrDuplicateRequestMessageID = errors.New("duplicate integration request message ID")
	ErrMissingNewline            = errors.New("integration message is missing newline delimiter")
	ErrLineTooLong               = errors.New("integration message exceeds one MiB line limit")
)

type Kind string

const (
	Request  Kind = "request"
	Response Kind = "response"
	Event    Kind = "event"
)

type Method string

const (
	Hello        Method = "hello"
	Describe     Method = "describe"
	Discover     Method = "discover"
	Health       Method = "health"
	Configure    Method = "configure"
	Execute      Method = "execute"
	Cancel       Method = "cancel"
	SafeShutdown Method = "safe_shutdown"
)

func (kind Kind) validate() error {
	switch kind {
	case Request, Response, Event:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidKind, kind)
	}
}

func (method Method) validate() error {
	switch method {
	case Hello, Describe, Discover, Health, Configure, Execute, Cancel, SafeShutdown:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidMethod, method)
	}
}

// Progress is a fractional execute update.
type Progress struct {
	Fraction float64 `json:"fraction"`
	Message  string  `json:"message"`
}

func (progress Progress) Validate() error {
	if math.IsNaN(progress.Fraction) || math.IsInf(progress.Fraction, 0) || progress.Fraction < 0 || progress.Fraction > 1 {
		return fmt.Errorf("%w: progress fraction must be in [0,1]", ErrInvalidMessage)
	}
	return nil
}

// Measurement records a timestamped numeric observation.
type Measurement struct {
	Name      string    `json:"name"`
	Value     float64   `json:"value"`
	Unit      string    `json:"unit"`
	Timestamp time.Time `json:"timestamp"`
}

func (measurement Measurement) Validate() error {
	if strings.TrimSpace(measurement.Name) == "" || math.IsNaN(measurement.Value) || math.IsInf(measurement.Value, 0) {
		return fmt.Errorf("%w: measurement name and finite value are required", ErrInvalidMessage)
	}
	return validateUTCTimestamp(measurement.Timestamp)
}

// DomainEvent carries integration-specific event data without changing the
// envelope contract.
type DomainEvent struct {
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

func (event DomainEvent) Validate() error {
	if strings.TrimSpace(event.Name) == "" || !validJSONValue(event.Data) {
		return fmt.Errorf("%w: event name and data are required", ErrInvalidMessage)
	}
	return validateUTCTimestamp(event.Timestamp)
}

// Warning communicates a non-terminal warning.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (warning Warning) Validate() error {
	if strings.TrimSpace(warning.Code) == "" || strings.TrimSpace(warning.Message) == "" {
		return fmt.Errorf("%w: warning code and message are required", ErrInvalidMessage)
	}
	return nil
}

// LogReference points to bounded stored log data. Raw log data is never put
// in a wire message.
type LogReference struct {
	ArtifactID string `json:"artifact_id"`
	Offset     int64  `json:"offset"`
	Length     int64  `json:"length"`
}

func (reference LogReference) Validate() error {
	if strings.TrimSpace(reference.ArtifactID) == "" || reference.Offset < 0 || reference.Length < 0 || reference.Length > MaxReferencedLogBytes {
		return fmt.Errorf("%w: invalid bounded log reference", ErrInvalidMessage)
	}
	return nil
}

// ArtifactDeclaration describes an artifact retained outside the wire stream.
type ArtifactDeclaration struct {
	ID        string `json:"id"`
	MediaType string `json:"media_type"`
	SizeBytes int64  `json:"size_bytes"`
	Digest    string `json:"digest"`
}

func (artifact ArtifactDeclaration) Validate() error {
	if strings.TrimSpace(artifact.ID) == "" || strings.TrimSpace(artifact.MediaType) == "" || artifact.SizeBytes < 0 || strings.TrimSpace(artifact.Digest) == "" {
		return fmt.Errorf("%w: invalid artifact declaration", ErrInvalidMessage)
	}
	return nil
}

// Result is a terminal operation result.
type Result struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

func (result Result) Validate() error {
	if strings.TrimSpace(result.Status) == "" || !validJSONValue(result.Data) {
		return fmt.Errorf("%w: result status and data are required", ErrInvalidMessage)
	}
	return nil
}

// Error is a terminal operation failure payload.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (failure Error) Validate() error {
	if strings.TrimSpace(failure.Code) == "" || strings.TrimSpace(failure.Message) == "" {
		return fmt.Errorf("%w: error code and message are required", ErrInvalidMessage)
	}
	return nil
}

func validateUTCTimestamp(timestamp time.Time) error {
	if timestamp.IsZero() || timestamp.Location() != time.UTC {
		return fmt.Errorf("%w: timestamp must be UTC", ErrInvalidMessage)
	}
	return nil
}

func validJSONValue(value json.RawMessage) bool {
	return len(value) != 0 && json.Valid(value)
}
