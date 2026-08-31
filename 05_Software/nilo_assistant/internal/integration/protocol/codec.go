package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Registry owns message and request lifecycle state for one runner connection.
// Share one registry between a decoder and encoder with the WithRegistry constructors.
type Registry struct {
	mu         sync.Mutex
	messageIDs map[string]struct{}
	requests   map[string]requestState
}
type requestState struct {
	method   Method
	execute  bool
	terminal bool
}

func NewRegistry() *Registry {
	return &Registry{messageIDs: make(map[string]struct{}), requests: make(map[string]requestState)}
}
func (registry *Registry) accept(message Envelope) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.messageIDs[message.MessageID]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateMessageID, message.MessageID)
	}
	if message.Kind == Response || message.Kind == Event {
		request, exists := registry.requests[message.RequestID]
		if !exists {
			return fmt.Errorf("%w: %q", ErrUnknownRequest, message.RequestID)
		}
		if request.method != message.Method {
			return fmt.Errorf("%w: request %q", ErrRequestMethodMismatch, message.RequestID)
		}
		if request.execute {
			if message.Kind == Event && request.terminal {
				return ErrEventAfterTerminal
			}
			if message.Kind == Response {
				if request.terminal {
					return ErrMultipleTerminalResponse
				}
				request.terminal = true
				registry.requests[message.RequestID] = request
			}
		}
	}
	registry.messageIDs[message.MessageID] = struct{}{}
	if message.Kind == Request {
		registry.requests[message.MessageID] = requestState{method: message.Method, execute: message.Method == Execute}
	}
	return nil
}
func (registry *Registry) eof() error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, request := range registry.requests {
		if request.execute && !request.terminal {
			return ErrOutstandingExecute
		}
	}
	return io.EOF
}

type Decoder struct {
	reader   *bufio.Reader
	registry *Registry
	mu       sync.Mutex
	line     [MaxLineBytes - 1]byte
}

func NewDecoder(reader io.Reader) *Decoder { return NewDecoderWithRegistry(reader, NewRegistry()) }
func NewDecoderWithRegistry(reader io.Reader, registry *Registry) *Decoder {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Decoder{reader: bufio.NewReaderSize(reader, 32<<10), registry: registry}
}
func (decoder *Decoder) Decode() (Envelope, error) {
	decoder.mu.Lock()
	defer decoder.mu.Unlock()
	line, err := decoder.readLine()
	if err != nil {
		if err == io.EOF {
			return Envelope{}, decoder.registry.eof()
		}
		return Envelope{}, err
	}
	if err := preflightEnvelope(line); err != nil {
		return Envelope{}, err
	}
	var message Envelope
	jsonDecoder := json.NewDecoder(bytes.NewReader(line))
	jsonDecoder.DisallowUnknownFields()
	if err := jsonDecoder.Decode(&message); err != nil {
		return Envelope{}, fmt.Errorf("%w: decode envelope: %v", ErrInvalidMessage, err)
	}
	if err := ensureJSONEOF(jsonDecoder); err != nil {
		return Envelope{}, fmt.Errorf("%w: envelope: %v", ErrInvalidMessage, err)
	}
	if err := message.Validate(); err != nil {
		return Envelope{}, err
	}
	if err := decoder.registry.accept(message); err != nil {
		return Envelope{}, err
	}
	return message, nil
}
func (decoder *Decoder) readLine() ([]byte, error) {
	for index := range decoder.line {
		value, err := decoder.reader.ReadByte()
		if err != nil {
			if err == io.EOF && index == 0 {
				return nil, io.EOF
			}
			if err == io.EOF {
				return nil, ErrMissingNewline
			}
			return nil, err
		}
		if value == '\n' {
			return decoder.line[:index], nil
		}
		decoder.line[index] = value
	}
	value, err := decoder.reader.ReadByte()
	if err != nil {
		if err == io.EOF {
			return nil, ErrMissingNewline
		}
		return nil, err
	}
	if value == '\n' {
		return decoder.line[:], nil
	}
	return nil, ErrLineTooLong
}

func preflightEnvelope(line []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(line))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("%w: envelope must be an object", ErrInvalidMessage)
	}
	required := map[string]struct{}{"protocol": {}, "kind": {}, "message_id": {}, "request_id": {}, "method": {}, "timestamp": {}, "payload": {}}
	seen := make(map[string]struct{}, len(required))
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%w: envelope key: %v", ErrInvalidMessage, err)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("%w: envelope key is not a string", ErrInvalidMessage)
		}
		if _, ok := required[key]; !ok {
			return fmt.Errorf("%w: noncanonical envelope key %q", ErrInvalidMessage, key)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate envelope key %q", ErrInvalidMessage, key)
		}
		seen[key] = struct{}{}
		var discard json.RawMessage
		if err := decoder.Decode(&discard); err != nil {
			return fmt.Errorf("%w: envelope value: %v", ErrInvalidMessage, err)
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return fmt.Errorf("%w: envelope is not closed", ErrInvalidMessage)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: envelope: %v", ErrInvalidMessage, err)
	}
	if len(seen) != len(required) {
		return fmt.Errorf("%w: envelope must contain exactly seven canonical keys", ErrInvalidMessage)
	}
	return nil
}
func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

type Encoder struct {
	writer   io.Writer
	registry *Registry
	mu       sync.Mutex
}

func NewEncoder(writer io.Writer) *Encoder { return NewEncoderWithRegistry(writer, NewRegistry()) }
func NewEncoderWithRegistry(writer io.Writer, registry *Registry) *Encoder {
	if registry == nil {
		registry = NewRegistry()
	}
	return &Encoder{writer: writer, registry: registry}
}
func (encoder *Encoder) Encode(message Envelope) error {
	encoder.mu.Lock()
	defer encoder.mu.Unlock()
	if err := message.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode protocol message: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > MaxLineBytes {
		return ErrLineTooLong
	}
	if err := encoder.registry.accept(message); err != nil {
		return err
	}
	written, err := encoder.writer.Write(encoded)
	if err != nil {
		return err
	}
	if written != len(encoded) {
		return io.ErrShortWrite
	}
	return nil
}
