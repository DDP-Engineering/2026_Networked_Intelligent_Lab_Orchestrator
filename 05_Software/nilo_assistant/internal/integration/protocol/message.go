package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Envelope is the complete, versioned transport unit. Payload is decoded by
// a method handler into one of the typed payload values.
type Envelope struct {
	Protocol  string          `json:"protocol"`
	Kind      Kind            `json:"kind"`
	MessageID string          `json:"message_id"`
	RequestID string          `json:"request_id"`
	Method    Method          `json:"method"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// Validate enforces the invariants shared by all runner messages.
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
	if !validJSONValue(message.Payload) {
		return fmt.Errorf("%w: payload must be valid JSON", ErrInvalidMessage)
	}
	switch message.Kind {
	case Request:
		if message.RequestID != "" {
			return fmt.Errorf("%w: request_id is not allowed on requests", ErrInvalidMessage)
		}
	case Response, Event:
		if strings.TrimSpace(message.RequestID) == "" {
			return fmt.Errorf("%w: request_id is required on %s messages", ErrInvalidMessage, message.Kind)
		}
	}
	return nil
}
