package protocol

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecoderReadsValidHelloExchange(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "tests", "contract", "fixtures", "protocol-v1", "hello.ndjson"))
	if err != nil {
		t.Fatal(err)
	}

	decoder := NewDecoder(bytes.NewReader(input))
	request, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() request error = %v", err)
	}
	if request.Kind != Request || request.Method != Hello || request.MessageID != "request-hello-1" {
		t.Fatalf("request = %+v, want hello request", request)
	}
	if request.Timestamp.Location() != time.UTC {
		t.Fatalf("request timestamp location = %v, want UTC", request.Timestamp.Location())
	}

	response, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() response error = %v", err)
	}
	if response.Kind != Response || response.RequestID != request.MessageID || response.Method != Hello {
		t.Fatalf("response = %+v, want hello response tied to request", response)
	}
	if _, err := decoder.Decode(); !errors.Is(err, io.EOF) {
		t.Fatalf("Decode() after exchange error = %v, want io.EOF", err)
	}

	var output bytes.Buffer
	encoder := NewEncoder(&output)
	if err := encoder.Encode(request); err != nil {
		t.Fatalf("Encode() request error = %v", err)
	}
	if err := encoder.Encode(response); err != nil {
		t.Fatalf("Encode() response error = %v", err)
	}
	if got := output.String(); got != string(input) {
		t.Fatalf("golden round trip = %q, want %q", got, input)
	}
}

func TestDecoderRejectsUnknownEnvelopeField(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(validRequest(`,"unexpected":true`) + "\n"))
	if _, err := decoder.Decode(); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("Decode() error = %v, want ErrInvalidMessage", err)
	}
}

func TestDecoderRejectsUnknownMessageKind(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(strings.Replace(validRequest(""), `"request"`, `"notice"`, 1) + "\n"))
	if _, err := decoder.Decode(); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("Decode() error = %v, want ErrInvalidKind", err)
	}
}

func TestDecoderRejectsDuplicateRequestMessageID(t *testing.T) {
	line := validRequest("") + "\n"
	decoder := NewDecoder(strings.NewReader(line + line))
	if _, err := decoder.Decode(); err != nil {
		t.Fatalf("first Decode() error = %v", err)
	}
	if _, err := decoder.Decode(); !errors.Is(err, ErrDuplicateRequestMessageID) {
		t.Fatalf("second Decode() error = %v, want ErrDuplicateRequestMessageID", err)
	}
}

func TestDecoderRejectsMalformedJSON(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(`{"protocol":` + "\n"))
	if _, err := decoder.Decode(); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("Decode() error = %v, want ErrInvalidMessage", err)
	}
}

func TestDecoderRejectsMissingNewlineAtEOF(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(validRequest("")))
	if _, err := decoder.Decode(); !errors.Is(err, ErrMissingNewline) {
		t.Fatalf("Decode() error = %v, want ErrMissingNewline", err)
	}
}

func TestDecoderRejectsLineLargerThanOneMiB(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(strings.Repeat("x", MaxLineBytes+1) + "\n"))
	if _, err := decoder.Decode(); !errors.Is(err, ErrLineTooLong) {
		t.Fatalf("Decode() error = %v, want ErrLineTooLong", err)
	}
}

func TestEncoderWritesOneMessageAndNewlineAtomically(t *testing.T) {
	var output bytes.Buffer
	encoder := NewEncoder(&output)
	message := Envelope{
		Protocol: ProtocolV1, Kind: Request, MessageID: "request-health-1", Method: Health,
		Timestamp: time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC), Payload: []byte(`{}`),
	}
	if err := encoder.Encode(message); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if got := output.String(); !strings.HasSuffix(got, "\n") || strings.Count(got, "\n") != 1 {
		t.Fatalf("encoded output = %q, want one newline-delimited object", got)
	}
}

func TestEnvelopeAndPayloadValidationRejectUnsafeValues(t *testing.T) {
	message := Envelope{
		Protocol: ProtocolV1, Kind: Response, MessageID: "response-1", Method: Execute,
		Timestamp: time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC), Payload: []byte(`{}`),
	}
	if err := message.Validate(); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("response without request ID Validate() error = %v, want ErrInvalidMessage", err)
	}
	if err := (Progress{Fraction: 1.01}).Validate(); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("out-of-range Progress.Validate() error = %v, want ErrInvalidMessage", err)
	}
	if err := (LogReference{ArtifactID: "log-1", Offset: 0, Length: MaxReferencedLogBytes + 1}).Validate(); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("oversized LogReference.Validate() error = %v, want ErrInvalidMessage", err)
	}
	if err := (Measurement{Name: "voltage", Value: 3.3, Timestamp: time.Date(2026, 8, 30, 12, 0, 0, 0, time.FixedZone("offset", 2*60*60))}).Validate(); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("non-UTC Measurement.Validate() error = %v, want ErrInvalidMessage", err)
	}
}

func validRequest(suffix string) string {
	return `{"protocol":"nilo.integration/1","kind":"request","message_id":"request-1","request_id":"","method":"health","timestamp":"2026-08-30T10:00:00Z","payload":{}}`[:len(`{"protocol":"nilo.integration/1","kind":"request","message_id":"request-1","request_id":"","method":"health","timestamp":"2026-08-30T10:00:00Z","payload":{}}`)-1] + suffix + `}`
}
