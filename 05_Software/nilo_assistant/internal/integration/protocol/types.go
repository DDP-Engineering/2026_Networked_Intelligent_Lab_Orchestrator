// Package protocol implements NILO's versioned runner wire contract.
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
	ErrDuplicateMessageID        = errors.New("duplicate integration message ID")
	ErrDuplicateRequestMessageID = ErrDuplicateMessageID
	ErrUnknownRequest            = errors.New("unknown integration request")
	ErrRequestMethodMismatch     = errors.New("integration response or event method does not match request")
	ErrMultipleTerminalResponse  = errors.New("multiple terminal execute responses")
	ErrEventAfterTerminal        = errors.New("execute event after terminal response")
	ErrOutstandingExecute        = errors.New("execute request has no terminal response at end of stream")
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

type PayloadType string

const (
	payloadHello        PayloadType = "hello"
	payloadDescribe     PayloadType = "describe"
	payloadDiscover     PayloadType = "discover"
	payloadHealth       PayloadType = "health"
	payloadConfigure    PayloadType = "configure"
	payloadExecute      PayloadType = "execute"
	payloadCancel       PayloadType = "cancel"
	payloadSafeShutdown PayloadType = "safe_shutdown"
	payloadProgress     PayloadType = "progress"
	payloadMeasurement  PayloadType = "measurement"
	payloadDomainEvent  PayloadType = "domain_event"
	payloadWarning      PayloadType = "warning"
	payloadLogReference PayloadType = "log_reference"
	payloadArtifact     PayloadType = "artifact"
	payloadResult       PayloadType = "result"
	payloadError        PayloadType = "error"
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

type HelloRequest struct {
	Type   PayloadType `json:"type"`
	Client string      `json:"client"`
}

func (value HelloRequest) Validate() error {
	if value.Type != payloadHello || strings.TrimSpace(value.Client) == "" {
		return fmt.Errorf("%w: invalid hello request", ErrInvalidMessage)
	}
	return nil
}

type HelloResponse struct {
	Type        PayloadType `json:"type"`
	Integration string      `json:"integration"`
}

func (value HelloResponse) Validate() error {
	if value.Type != payloadHello || strings.TrimSpace(value.Integration) == "" {
		return fmt.Errorf("%w: invalid hello response", ErrInvalidMessage)
	}
	return nil
}

type ControlPayload struct {
	Type   PayloadType `json:"type"`
	Status string      `json:"status,omitempty"`
}

func (value ControlPayload) Validate(expected PayloadType, response bool) error {
	if value.Type != expected || (response && strings.TrimSpace(value.Status) == "") {
		return fmt.Errorf("%w: invalid control payload", ErrInvalidMessage)
	}
	return nil
}

type ConfigureRequest struct {
	Type          PayloadType     `json:"type"`
	Configuration json.RawMessage `json:"configuration"`
}

func (value ConfigureRequest) Validate() error {
	if value.Type != payloadConfigure || !validJSONObject(value.Configuration) {
		return fmt.Errorf("%w: invalid configure request", ErrInvalidMessage)
	}
	return nil
}

type ExecuteRequest struct {
	Type       PayloadType     `json:"type"`
	Operation  string          `json:"operation"`
	Parameters json.RawMessage `json:"parameters"`
}

func (value ExecuteRequest) Validate() error {
	if value.Type != payloadExecute || strings.TrimSpace(value.Operation) == "" || !validJSONValue(value.Parameters) {
		return fmt.Errorf("%w: invalid execute request", ErrInvalidMessage)
	}
	return nil
}

type CancelRequest struct {
	Type            PayloadType `json:"type"`
	TargetRequestID string      `json:"target_request_id"`
}

func (value CancelRequest) Validate() error {
	if value.Type != payloadCancel || strings.TrimSpace(value.TargetRequestID) == "" {
		return fmt.Errorf("%w: invalid cancel request", ErrInvalidMessage)
	}
	return nil
}

type Progress struct {
	Type     PayloadType `json:"type"`
	Fraction float64     `json:"fraction"`
	Message  string      `json:"message"`
	Stage    string      `json:"stage,omitempty"`
}

func (value Progress) Validate() error {
	if value.Type != payloadProgress || math.IsNaN(value.Fraction) || math.IsInf(value.Fraction, 0) || value.Fraction < 0 || value.Fraction > 1 || strings.TrimSpace(value.Message) == "" {
		return fmt.Errorf("%w: invalid progress", ErrInvalidMessage)
	}
	return nil
}

type Measurement struct {
	Type      PayloadType `json:"type"`
	Name      string      `json:"name"`
	Value     float64     `json:"value"`
	Unit      string      `json:"unit"`
	Timestamp time.Time   `json:"timestamp"`
}

func (value Measurement) Validate() error {
	if value.Type != payloadMeasurement || strings.TrimSpace(value.Name) == "" || strings.TrimSpace(value.Unit) == "" || math.IsNaN(value.Value) || math.IsInf(value.Value, 0) {
		return fmt.Errorf("%w: invalid measurement", ErrInvalidMessage)
	}
	return validateUTCTimestamp(value.Timestamp)
}

type DomainEvent struct {
	Type      PayloadType     `json:"type"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

func (value DomainEvent) Validate() error {
	if value.Type != payloadDomainEvent || strings.TrimSpace(value.Name) == "" || !validJSONValue(value.Data) {
		return fmt.Errorf("%w: invalid domain event", ErrInvalidMessage)
	}
	return validateUTCTimestamp(value.Timestamp)
}

type Warning struct {
	Type    PayloadType `json:"type"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
}

func (value Warning) Validate() error {
	if value.Type != payloadWarning || strings.TrimSpace(value.Code) == "" || strings.TrimSpace(value.Message) == "" {
		return fmt.Errorf("%w: invalid warning", ErrInvalidMessage)
	}
	return nil
}

type LogReference struct {
	Type       PayloadType `json:"type"`
	ArtifactID string      `json:"artifact_id"`
	Offset     int64       `json:"offset"`
	Length     int64       `json:"length"`
}

func (value LogReference) Validate() error {
	if value.Type != payloadLogReference || strings.TrimSpace(value.ArtifactID) == "" || value.Offset < 0 || value.Length < 0 || value.Length > MaxReferencedLogBytes {
		return fmt.Errorf("%w: invalid bounded log reference", ErrInvalidMessage)
	}
	return nil
}

type ArtifactDeclaration struct {
	Type      PayloadType `json:"type"`
	ID        string      `json:"id"`
	MediaType string      `json:"media_type"`
	SizeBytes int64       `json:"size_bytes"`
	Digest    string      `json:"digest"`
}

func (value ArtifactDeclaration) Validate() error {
	if value.Type != payloadArtifact || strings.TrimSpace(value.ID) == "" || strings.TrimSpace(value.MediaType) == "" || value.SizeBytes < 0 || strings.TrimSpace(value.Digest) == "" {
		return fmt.Errorf("%w: invalid artifact declaration", ErrInvalidMessage)
	}
	return nil
}

type Result struct {
	Type   PayloadType     `json:"type"`
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
}

func (value Result) Validate() error {
	if value.Type != payloadResult || strings.TrimSpace(value.Status) == "" || !validJSONValue(value.Data) {
		return fmt.Errorf("%w: invalid result", ErrInvalidMessage)
	}
	return nil
}

type Error struct {
	Type      PayloadType `json:"type"`
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Retryable bool        `json:"retryable"`
}

func (value Error) Validate() error {
	if value.Type != payloadError || strings.TrimSpace(value.Code) == "" || strings.TrimSpace(value.Message) == "" {
		return fmt.Errorf("%w: invalid error", ErrInvalidMessage)
	}
	return nil
}

func validateUTCTimestamp(timestamp time.Time) error {
	if timestamp.IsZero() || timestamp.Location() != time.UTC {
		return fmt.Errorf("%w: timestamp must be UTC", ErrInvalidMessage)
	}
	return nil
}
func validJSONObject(value json.RawMessage) bool {
	var object map[string]json.RawMessage
	return len(value) > 0 && json.Unmarshal(value, &object) == nil && object != nil
}
func validJSONValue(value json.RawMessage) bool { return len(value) > 0 && json.Valid(value) }
