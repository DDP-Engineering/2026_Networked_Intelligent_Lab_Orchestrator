package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDescribeAcceptsSDKMetadataButRejectsUnknownFields(t *testing.T) {
	envelope := Envelope{Protocol: ProtocolV1, Kind: Response, Method: Describe, MessageID: "reply", RequestID: "request", Timestamp: time.Now().UTC()}
	envelope.Payload = json.RawMessage(`{"type":"describe","status":"ok","integration":"sigrok","operations":["capture"]}`)
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	envelope.Payload = json.RawMessage(`{"type":"describe","status":"ok","integration":"sigrok","operations":["capture"],"shell":"injected"}`)
	if err := envelope.Validate(); err == nil {
		t.Fatal("unknown metadata was accepted")
	}
}
