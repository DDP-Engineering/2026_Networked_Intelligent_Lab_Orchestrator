package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"nilo.local/nilo/internal/integration/protocol"
)

func main() {
	mode := os.Args[1]
	reader := bufio.NewReader(os.Stdin)
	first, err := reader.ReadBytes('\n')
	if err != nil {
		os.Exit(2)
	}
	var hello protocol.Envelope
	if err := json.Unmarshal(first, &hello); err != nil || hello.Method != protocol.Hello {
		os.Exit(2)
	}
	if mode == "wrong-protocol" {
		fmt.Fprintf(os.Stdout, `{"protocol":"wrong","kind":"response","message_id":"hello-response","request_id":%q,"method":"hello","timestamp":"2026-08-30T12:00:00Z","payload":{"type":"hello","integration":"test"}}`+"\n", hello.MessageID)
		return
	}
	write(protocol.Response, "hello-response", hello.MessageID, protocol.Hello, map[string]any{"type": "hello", "integration": "test"})
	if mode == "malformed" {
		fmt.Fprint(os.Stdout, "not-json\n")
		return
	}
	if mode == "overflow" {
		fmt.Fprint(os.Stdout, strings.Repeat("x", protocol.MaxLineBytes)+"\n")
		return
	}
	if mode == "stderr" {
		fmt.Fprint(os.Stderr, strings.Repeat("x", 70<<10)+"END")
		return
	}
	if mode == "exit" {
		return
	}
	second, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}
	var execute protocol.Envelope
	if err := json.Unmarshal(second, &execute); err != nil {
		return
	}
	if mode == "describe" && execute.Method == protocol.Describe {
		write(protocol.Response, "describe-response", execute.MessageID, protocol.Describe, map[string]any{"type": "describe", "status": "ok"})
		return
	}
	if execute.Method != protocol.Execute {
		return
	}
	if mode == "hang" {
		time.Sleep(time.Hour)
	}
	write(protocol.Response, "execute-response", execute.MessageID, protocol.Execute, map[string]any{"type": "result", "status": "completed", "data": map[string]any{}})
}

func write(kind protocol.Kind, messageID, requestID string, method protocol.Method, payload any) {
	encoded, _ := json.Marshal(payload)
	message := protocol.Envelope{Protocol: protocol.ProtocolV1, Kind: kind, MessageID: messageID, RequestID: requestID, Method: method, Timestamp: time.Now().UTC(), Payload: encoded}
	encoded, _ = json.Marshal(message)
	fmt.Fprint(os.Stdout, string(encoded)+"\n")
}
