package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Envelope is the complete, versioned runner transport unit.
type Envelope struct {
	Protocol  string          `json:"protocol"`
	Kind      Kind            `json:"kind"`
	MessageID string          `json:"message_id"`
	RequestID string          `json:"request_id"`
	Method    Method          `json:"method"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

func (message Envelope) Validate() error {
	if message.Protocol != ProtocolV1 {
		return fmt.Errorf("%w: %q", ErrUnsupportedProtocol, message.Protocol)
	}
	if err := message.Kind.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(message.MessageID) == "" {
		return fmt.Errorf("%w: message_id is required", ErrInvalidMessage)
	}
	if err := message.Method.validate(); err != nil {
		return err
	}
	if err := validateUTCTimestamp(message.Timestamp); err != nil {
		return err
	}
	switch message.Kind {
	case Request:
		if message.RequestID != "" {
			return fmt.Errorf("%w: requests cannot have request_id", ErrInvalidMessage)
		}
	case Response, Event:
		if strings.TrimSpace(message.RequestID) == "" {
			return fmt.Errorf("%w: responses and events require request_id", ErrInvalidMessage)
		}
	}
	return validatePayload(message)
}

func validatePayload(message Envelope) error {
	if !validJSONObject(message.Payload) {
		return fmt.Errorf("%w: payload must be a JSON object", ErrInvalidMessage)
	}
	switch message.Kind {
	case Request:
		switch message.Method {
		case Hello:
			return decodePayload(message.Payload, func(value HelloRequest) error { return value.Validate() })
		case Configure:
			return decodePayload(message.Payload, func(value ConfigureRequest) error { return value.Validate() })
		case Execute:
			return decodePayload(message.Payload, func(value ExecuteRequest) error { return value.Validate() })
		case Cancel:
			return decodePayload(message.Payload, func(value CancelRequest) error { return value.Validate() })
		default:
			return decodePayload(message.Payload, func(value ControlPayload) error { return value.Validate(PayloadType(message.Method), false) })
		}
	case Response:
		if message.Method == Describe {
			return decodePayload(message.Payload, func(value DescribeResult) error { return value.Validate() })
		}
		if message.Method == Hello {
			return decodePayload(message.Payload, func(value HelloResponse) error { return value.Validate() })
		}
		if message.Method == Execute {
			return decodeExecuteTerminal(message.Payload)
		}
		return decodePayload(message.Payload, func(value ControlPayload) error { return value.Validate(PayloadType(message.Method), true) })
	case Event:
		if message.Method != Execute {
			return fmt.Errorf("%w: only execute can emit events", ErrInvalidMessage)
		}
		return decodeExecuteEvent(message.Payload)
	}
	return fmt.Errorf("%w: unsupported payload matrix", ErrInvalidMessage)
}

func decodePayload[T any](payload json.RawMessage, validate func(T) error) error {
	var value T
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("%w: decode payload: %v", ErrInvalidMessage, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: payload: %v", ErrInvalidMessage, err)
	}
	return validate(value)
}

func decodeExecuteTerminal(payload json.RawMessage) error {
	discriminator, err := payloadDiscriminator(payload)
	if err != nil {
		return err
	}
	switch discriminator.Type {
	case payloadResult:
		return decodePayload(payload, func(value Result) error { return value.Validate() })
	case payloadError:
		return decodePayload(payload, func(value Error) error { return value.Validate() })
	default:
		return fmt.Errorf("%w: invalid execute terminal type %q", ErrInvalidMessage, discriminator.Type)
	}
}

func decodeExecuteEvent(payload json.RawMessage) error {
	discriminator, err := payloadDiscriminator(payload)
	if err != nil {
		return err
	}
	switch discriminator.Type {
	case payloadProgress:
		return decodePayload(payload, func(value Progress) error { return value.Validate() })
	case payloadMeasurement:
		return decodePayload(payload, func(value Measurement) error { return value.Validate() })
	case payloadDomainEvent:
		return decodePayload(payload, func(value DomainEvent) error { return value.Validate() })
	case payloadWarning:
		return decodePayload(payload, func(value Warning) error { return value.Validate() })
	case payloadLogReference:
		return decodePayload(payload, func(value LogReference) error { return value.Validate() })
	case payloadArtifact:
		return decodePayload(payload, func(value ArtifactDeclaration) error { return value.Validate() })
	default:
		return fmt.Errorf("%w: invalid execute event type %q", ErrInvalidMessage, discriminator.Type)
	}
}

func payloadDiscriminator(payload json.RawMessage) (struct {
	Type PayloadType `json:"type"`
}, error) {
	var discriminator struct {
		Type PayloadType `json:"type"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	if err := decoder.Decode(&discriminator); err != nil {
		return discriminator, fmt.Errorf("%w: decode payload discriminator: %v", ErrInvalidMessage, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return discriminator, fmt.Errorf("%w: payload discriminator: %v", ErrInvalidMessage, err)
	}
	return discriminator, nil
}
