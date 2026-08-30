package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Decoder reads one bounded, newline-delimited protocol message at a time.
// Its request ID set is deliberately scoped to one connection.
type Decoder struct {
	reader            *bufio.Reader
	requestMessageIDs map[string]struct{}
	line              [MaxLineBytes]byte
}

func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{reader: bufio.NewReaderSize(reader, 32<<10), requestMessageIDs: make(map[string]struct{})}
}

// Decode reads and strictly validates exactly one NDJSON envelope.
func (decoder *Decoder) Decode() (Envelope, error) {
	line, err := decoder.readLine()
	if err != nil {
		return Envelope{}, err
	}
	var message Envelope
	jsonDecoder := json.NewDecoder(bytes.NewReader(line))
	jsonDecoder.DisallowUnknownFields()
	if err := jsonDecoder.Decode(&message); err != nil {
		return Envelope{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidMessage, err)
	}
	if err := ensureJSONEOF(jsonDecoder); err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	if err := message.Validate(); err != nil {
		return Envelope{}, err
	}
	if message.Kind == Request {
		if _, exists := decoder.requestMessageIDs[message.MessageID]; exists {
			return Envelope{}, fmt.Errorf("%w: %q", ErrDuplicateRequestMessageID, message.MessageID)
		}
		decoder.requestMessageIDs[message.MessageID] = struct{}{}
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

// Encoder serializes whole messages under one mutex so concurrent callers
// cannot interleave their JSON objects on the same connection.
type Encoder struct {
	writer io.Writer
	mu     sync.Mutex
}

func NewEncoder(writer io.Writer) *Encoder { return &Encoder{writer: writer} }

// Encode validates and writes exactly one JSON object followed by one newline.
func (encoder *Encoder) Encode(message Envelope) error {
	if err := message.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode protocol message: %w", err)
	}
	encoded = append(encoded, '\n')
	encoder.mu.Lock()
	defer encoder.mu.Unlock()
	written, err := encoder.writer.Write(encoded)
	if err != nil {
		return err
	}
	if written != len(encoded) {
		return io.ErrShortWrite
	}
	return nil
}
