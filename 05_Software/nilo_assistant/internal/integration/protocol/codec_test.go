package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedRegistryValidatesGoldenLifecycleAndRoundTrips(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "tests", "contract", "fixtures", "protocol-v1", "hello.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	decoder := NewDecoderWithRegistry(bytes.NewReader(input), registry)
	var messages []Envelope
	for {
		message, err := decoder.Decode()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		messages = append(messages, message)
	}
	if len(messages) != 10 {
		t.Fatalf("decoded messages = %d, want 10", len(messages))
	}
	if messages[0].Method != Hello || messages[2].Method != Execute || messages[len(messages)-1].Kind != Response {
		t.Fatalf("unexpected lifecycle: %#v", messages)
	}
	var output bytes.Buffer
	encoder := NewEncoderWithRegistry(&output, NewRegistry())
	for _, message := range messages {
		if err := encoder.Encode(message); err != nil {
			t.Fatalf("Encode(%s/%s) error = %v", message.Kind, message.Method, err)
		}
	}
	if got := output.String(); got != string(input) {
		t.Fatalf("golden round trip = %q, want %q", got, input)
	}
}

func TestRegistryRejectsOrphanMismatchedAndDuplicateMessages(t *testing.T) {
	tests := []struct {
		name, input string
		want        error
	}{
		{name: "orphan response", input: wire(Response, "response-1", "request-missing", Hello, `{"type":"hello","integration":"fake"}`), want: ErrUnknownRequest},
		{name: "mismatched response method", input: wire(Request, "request-1", "", Hello, `{"type":"hello","client":"core"}`) + wire(Response, "response-1", "request-1", Health, `{"type":"health","status":"ready"}`), want: ErrRequestMethodMismatch},
		{name: "duplicate ID across kinds", input: wire(Request, "same-id", "", Hello, `{"type":"hello","client":"core"}`) + wire(Response, "same-id", "same-id", Hello, `{"type":"hello","integration":"fake"}`), want: ErrDuplicateMessageID},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(test.input))
			if test.name == "orphan response" {
				if _, err := decoder.Decode(); !errors.Is(err, test.want) {
					t.Fatalf("Decode() error = %v, want %v", err, test.want)
				}
				return
			}
			if _, err := decoder.Decode(); err != nil {
				t.Fatalf("first Decode() error = %v", err)
			}
			if _, err := decoder.Decode(); !errors.Is(err, test.want) {
				t.Fatalf("second Decode() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRegistryRejectsMultipleTerminalResponsesAndEventsAfterTerminal(t *testing.T) {
	request := wire(Request, "execute-1", "", Execute, `{"type":"execute","operation":"echo","parameters":{}}`)
	result := wire(Response, "result-1", "execute-1", Execute, `{"type":"result","status":"completed","data":{}}`)
	tests := []struct {
		name, input string
		want        error
	}{
		{name: "second terminal response", input: request + result + wire(Response, "result-2", "execute-1", Execute, `{"type":"result","status":"completed","data":{}}`), want: ErrMultipleTerminalResponse},
		{name: "event after terminal", input: request + result + wire(Event, "event-1", "execute-1", Execute, `{"type":"progress","fraction":1,"message":"late"}`), want: ErrEventAfterTerminal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(test.input))
			for index := 0; index < 2; index++ {
				if _, err := decoder.Decode(); err != nil {
					t.Fatalf("Decode(%d) error = %v", index, err)
				}
			}
			if _, err := decoder.Decode(); !errors.Is(err, test.want) {
				t.Fatalf("terminal Decode() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestDecoderReportsOutstandingExecuteAtEOF(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(wire(Request, "execute-1", "", Execute, `{"type":"execute","operation":"echo","parameters":{}}`)))
	if _, err := decoder.Decode(); err != nil {
		t.Fatalf("Decode() execute request error = %v", err)
	}
	if _, err := decoder.Decode(); !errors.Is(err, ErrOutstandingExecute) {
		t.Fatalf("Decode() EOF error = %v, want ErrOutstandingExecute", err)
	}
}

func TestDecoderRejectsNoncanonicalEnvelopeKeys(t *testing.T) {
	tests := []struct{ name, line string }{
		{name: "case variant", line: strings.Replace(wire(Request, "request-1", "", Hello, `{"type":"hello","client":"core"}`), `"protocol"`, `"Protocol"`, 1)},
		{name: "duplicate key", line: strings.Replace(wire(Request, "request-1", "", Hello, `{"type":"hello","client":"core"}`), `"kind":"request"`, `"kind":"request","kind":"request"`, 1)},
		{name: "omitted key", line: strings.Replace(wire(Request, "request-1", "", Hello, `{"type":"hello","client":"core"}`), `,"request_id":""`, "", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewDecoder(strings.NewReader(test.line)).Decode(); !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("Decode() error = %v, want ErrInvalidMessage", err)
			}
		})
	}
}

func TestDecoderRejectsUnknownEnvelopeFieldAndPayloadShapes(t *testing.T) {
	tests := []struct {
		name, line string
		prelude    bool
	}{
		{name: "unknown envelope field", line: strings.TrimSuffix(wire(Request, "request-1", "", Hello, `{"type":"hello","client":"core"}`), "\n") + `,"unexpected":true}` + "\n"},
		{name: "scalar payload", line: wire(Request, "request-1", "", Hello, `"hello"`)},
		{name: "null payload", line: wire(Request, "request-1", "", Hello, `null`)},
		{name: "unknown event type", line: wire(Request, "execute-1", "", Execute, `{"type":"execute","operation":"echo","parameters":{}}`) + wire(Event, "event-1", "execute-1", Execute, `{"type":"unknown"}`), prelude: true},
		{name: "event type missing required field", line: wire(Request, "execute-1", "", Execute, `{"type":"execute","operation":"echo","parameters":{}}`) + wire(Event, "event-1", "execute-1", Execute, `{"type":"progress","fraction":0.5}`), prelude: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(test.line))
			if test.prelude {
				if _, err := decoder.Decode(); err != nil {
					t.Fatalf("request Decode() error = %v", err)
				}
			}
			if _, err := decoder.Decode(); !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("payload Decode() error = %v, want ErrInvalidMessage", err)
			}
		})
	}
}

func TestDecoderRejectsMalformedJSONMissingNewlineAndOversizedLine(t *testing.T) {
	for _, test := range []struct {
		name, input string
		want        error
	}{
		{name: "malformed", input: `{"protocol":` + "\n", want: ErrInvalidMessage},
		{name: "missing newline", input: strings.TrimSuffix(wire(Request, "request-1", "", Hello, `{"type":"hello","client":"core"}`), "\n"), want: ErrMissingNewline},
		{name: "oversized", input: strings.Repeat("x", MaxLineBytes) + "\n", want: ErrLineTooLong},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewDecoder(strings.NewReader(test.input)).Decode(); !errors.Is(err, test.want) {
				t.Fatalf("Decode() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestEncoderRejectsFrameOverOneMiBAndAcceptsExactBoundary(t *testing.T) {
	base := Envelope{Protocol: ProtocolV1, Kind: Request, MessageID: "request-boundary", Method: Configure, Timestamp: time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)}
	base.Payload = mustPayload(t, `{"type":"configure","configuration":{"padding":""}}`)
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	fill := MaxLineBytes - (len(encoded) + 1)
	base.Payload = mustPayload(t, `{"type":"configure","configuration":{"padding":"`+strings.Repeat("x", fill)+`"}}`)
	var exact bytes.Buffer
	if err := NewEncoder(&exact).Encode(base); err != nil {
		t.Fatalf("Encode() exact boundary error = %v", err)
	}
	if got := exact.Len(); got != MaxLineBytes {
		t.Fatalf("exact frame size = %d, want %d", got, MaxLineBytes)
	}
	base.Payload = mustPayload(t, `{"type":"configure","configuration":{"padding":"`+strings.Repeat("x", fill+1)+`"}}`)
	if err := NewEncoder(io.Discard).Encode(base); !errors.Is(err, ErrLineTooLong) {
		t.Fatalf("Encode() plus one error = %v, want ErrLineTooLong", err)
	}
}

func TestEncoderSerializesConcurrentWrites(t *testing.T) {
	writer := &concurrencyDetectingWriter{}
	encoder := NewEncoder(writer)
	const count = 32
	var group sync.WaitGroup
	errors := make(chan error, count)
	for index := 0; index < count; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			errors <- encoder.Encode(Envelope{Protocol: ProtocolV1, Kind: Request, MessageID: "request-" + string(rune('a'+index)), Method: Health, Timestamp: time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC), Payload: mustPayload(t, `{"type":"health"}`)})
		}(index)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent Encode() error = %v", err)
		}
	}
	if writer.concurrent.Load() != 0 || strings.Count(writer.String(), "\n") != count {
		t.Fatalf("writer concurrency=%d lines=%d, want 0 and %d", writer.concurrent.Load(), strings.Count(writer.String(), "\n"), count)
	}
}

func wire(kind Kind, messageID, requestID string, method Method, payload string) string {
	return `{"protocol":"nilo.integration/1","kind":"` + string(kind) + `","message_id":"` + messageID + `","request_id":"` + requestID + `","method":"` + string(method) + `","timestamp":"2026-08-30T10:00:00Z","payload":` + payload + "}\n"
}

func mustPayload(t *testing.T, value string) json.RawMessage {
	t.Helper()
	if !json.Valid([]byte(value)) {
		t.Fatalf("invalid test payload %q", value)
	}
	return json.RawMessage(value)
}

type concurrencyDetectingWriter struct {
	bytes.Buffer
	active     atomic.Int32
	concurrent atomic.Int32
}

func (writer *concurrencyDetectingWriter) Write(value []byte) (int, error) {
	if writer.active.Add(1) != 1 {
		writer.concurrent.Store(1)
	}
	time.Sleep(time.Millisecond)
	written, err := writer.Buffer.Write(value)
	writer.active.Add(-1)
	return written, err
}
