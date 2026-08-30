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
	marker := ""
	if len(os.Args) > 2 {
		marker = os.Args[2]
	}
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
	if mode == "stderr-control" {
		fmt.Fprint(os.Stderr, "\x1b[31msecret\r\x00\x7f\u0085END")
		return
	}
	if mode == "exit" {
		return
	}
	if mode == "stall-stdin" {
		time.Sleep(time.Hour)
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
	if mode == "describe-hang" && execute.Method == protocol.Describe {
		mark(marker, "describe-ready")
		waitForSafeShutdown(reader, marker)
		return
	}
	if mode == "describe-unread" && execute.Method == protocol.Describe {
		mark(marker, "describe-ready")
		time.Sleep(time.Hour)
		return
	}
	if execute.Method != protocol.Execute {
		return
	}
	if mode == "mismatched-event" {
		write(protocol.Event, "bad-event", "00000000-0000-0000-0000-000000000000", protocol.Execute, map[string]any{"type": "progress", "fraction": 0.5, "message": "wrong request"})
		return
	}
	if mode == "remote-error" {
		write(protocol.Response, "execute-error", execute.MessageID, protocol.Execute, map[string]any{"type": "error", "code": "device_busy", "message": "device busy", "retryable": true})
		return
	}
	if mode == "event-hang" {
		write(protocol.Event, "progress-event", execute.MessageID, protocol.Execute, map[string]any{"type": "progress", "fraction": 0.5, "message": "blocked sink"})
		time.Sleep(time.Hour)
	}
	if mode == "event-then-result" {
		write(protocol.Event, "progress-event", execute.MessageID, protocol.Execute, map[string]any{"type": "progress", "fraction": 0.5, "message": "sink fails"})
		write(protocol.Response, "execute-response", execute.MessageID, protocol.Execute, map[string]any{"type": "result", "status": "completed", "data": map[string]any{}})
		return
	}
	if mode == "cooperative-cancel" {
		mark(marker, execute.MessageID)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var cancel protocol.Envelope
		if err := json.Unmarshal(line, &cancel); err != nil || cancel.Method != protocol.Cancel {
			mark(marker, "wrong-cancel")
			return
		}
		var payload protocol.CancelRequest
		if err := json.Unmarshal(cancel.Payload, &payload); err != nil || payload.TargetRequestID != execute.MessageID {
			mark(marker, "wrong-cancel")
			return
		}
		mark(marker, "correct-cancel")
		waitForSafeShutdown(reader, marker)
		return
	}
	if mode == "hang" {
		time.Sleep(time.Hour)
	}
	write(protocol.Response, "execute-response", execute.MessageID, protocol.Execute, map[string]any{"type": "result", "status": "completed", "data": map[string]any{}})
}

func waitForSafeShutdown(reader *bufio.Reader, marker string) {
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return
	}
	var shutdown protocol.Envelope
	if err := json.Unmarshal(line, &shutdown); err == nil && shutdown.Method == protocol.SafeShutdown {
		mark(marker, "safe_shutdown")
	}
}

func mark(path, value string) {
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintln(file, value)
}

func write(kind protocol.Kind, messageID, requestID string, method protocol.Method, payload any) {
	encoded, _ := json.Marshal(payload)
	message := protocol.Envelope{Protocol: protocol.ProtocolV1, Kind: kind, MessageID: messageID, RequestID: requestID, Method: method, Timestamp: time.Now().UTC(), Payload: encoded}
	encoded, _ = json.Marshal(message)
	fmt.Fprint(os.Stdout, string(encoded)+"\n")
}
