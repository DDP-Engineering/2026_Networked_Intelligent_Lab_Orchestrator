// The contract runner is a deterministic child process used only by cross-component tests.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const protocol = "nilo.integration/1"
const artifactName = "contract-artifact.bin"

var artifactBytes = []byte("nilo contract artifact v1\n")

type envelope struct {
	Protocol  string          `json:"protocol"`
	Kind      string          `json:"kind"`
	MessageID string          `json:"message_id"`
	RequestID string          `json:"request_id"`
	Method    string          `json:"method"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type server struct {
	mu      sync.Mutex
	writer  *bufio.Writer
	active  map[string]chan struct{}
	stopped bool
}

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--stdio" {
		fmt.Fprintln(os.Stderr, "usage: contract-runner --stdio")
		os.Exit(2)
	}
	s := &server{writer: bufio.NewWriter(os.Stdout), active: make(map[string]chan struct{})}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var request envelope
		if err := strict(scanner.Bytes(), &request); err != nil || request.Protocol != protocol || request.Kind != "request" || request.MessageID == "" {
			fmt.Fprintln(os.Stderr, "protocol error: invalid envelope")
			os.Exit(2)
		}
		if err := s.handle(request); err != nil {
			fmt.Fprintln(os.Stderr, "protocol error:", err)
			os.Exit(2)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "protocol error:", err)
		os.Exit(2)
	}
}

func strict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing data")
	}
	return nil
}

func messageID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

func (s *server) send(kind, method, requestID string, payload any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	encoded, _ := json.Marshal(payload)
	output, _ := json.Marshal(envelope{
		Protocol: protocol, Kind: kind, MessageID: messageID(), RequestID: requestID,
		Method: method, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Payload: encoded,
	})
	_, _ = s.writer.Write(output)
	_ = s.writer.WriteByte('\n')
	_ = s.writer.Flush()
}

func (s *server) handle(request envelope) error {
	switch request.Method {
	case "hello":
		var payload struct {
			Type   string `json:"type"`
			Client string `json:"client"`
		}
		if err := strict(request.Payload, &payload); err != nil || payload.Type != "hello" {
			return fmt.Errorf("invalid hello")
		}
		s.send("response", "hello", request.MessageID, map[string]any{"type": "hello", "integration": "contract-runner"})
	case "describe":
		s.send("response", "describe", request.MessageID, map[string]any{"type": "describe", "status": "ok", "integration": "contract-runner", "operations": []string{"echo", "artifact", "sleep", "fail", "crash"}})
	case "health":
		status := "ok"
		if s.stopped {
			status = "stopped"
		}
		s.send("response", "health", request.MessageID, map[string]any{"type": "health", "status": status})
	case "discover":
		s.send("response", "discover", request.MessageID, map[string]any{"type": "discover", "status": "ok"})
	case "configure":
		s.send("response", "configure", request.MessageID, map[string]any{"type": "configure", "status": "ok"})
	case "safe_shutdown":
		s.stopped = true
		s.send("response", "safe_shutdown", request.MessageID, map[string]any{"type": "safe_shutdown", "status": "ok"})
	case "cancel":
		var payload struct {
			Type            string `json:"type"`
			TargetRequestID string `json:"target_request_id"`
		}
		if err := strict(request.Payload, &payload); err != nil {
			return err
		}
		s.send("response", "cancel", request.MessageID, map[string]any{"type": "cancel", "status": "ok"})
		s.mu.Lock()
		cancel := s.active[payload.TargetRequestID]
		delete(s.active, payload.TargetRequestID)
		s.mu.Unlock()
		if cancel != nil {
			close(cancel)
		}
	case "execute":
		return s.execute(request)
	default:
		return fmt.Errorf("unsupported method")
	}
	return nil
}

func (s *server) progress(requestID string, fraction float64, message, stage string) {
	s.send("event", "execute", requestID, map[string]any{"type": "progress", "fraction": fraction, "message": message, "stage": stage})
}

func (s *server) result(requestID, status string, data any) {
	s.send("response", "execute", requestID, map[string]any{"type": "result", "status": status, "data": data})
}

func (s *server) execute(request envelope) error {
	var payload struct {
		Type       string          `json:"type"`
		Operation  string          `json:"operation"`
		Parameters json.RawMessage `json:"parameters"`
	}
	if err := strict(request.Payload, &payload); err != nil || payload.Type != "execute" {
		return fmt.Errorf("invalid execute")
	}
	switch payload.Operation {
	case "echo":
		var parameters any
		if err := json.Unmarshal(payload.Parameters, &parameters); err != nil {
			return err
		}
		s.progress(request.MessageID, 0, "echo started", "start")
		s.progress(request.MessageID, 1, "echo completed", "complete")
		s.result(request.MessageID, "completed", map[string]any{"canonical": parameters})
	case "fail":
		var parameters struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := strict(payload.Parameters, &parameters); err != nil {
			return err
		}
		s.send("response", "execute", request.MessageID, map[string]any{"type": "error", "code": parameters.Code, "message": parameters.Message, "retryable": false})
	case "crash":
		s.progress(request.MessageID, 0.5, "crash imminent", "crash")
		fmt.Fprintln(os.Stderr, "contract crash requested")
		os.Exit(70)
	case "artifact":
		var parameters struct {
			StagingDirectory string `json:"staging_directory"`
		}
		if err := strict(payload.Parameters, &parameters); err != nil {
			return err
		}
		path := filepath.Join(parameters.StagingDirectory, artifactName)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := file.Write(artifactBytes); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		digest := sha256.Sum256(artifactBytes)
		s.send("event", "execute", request.MessageID, map[string]any{"type": "artifact", "id": "contract_artifact", "media_type": "application/octet-stream", "size_bytes": len(artifactBytes), "digest": "sha256:" + hex.EncodeToString(digest[:])})
		s.result(request.MessageID, "completed", map[string]any{"artifact_id": "contract_artifact"})
	case "sleep":
		var parameters struct {
			DurationMS int `json:"duration_ms"`
		}
		if err := strict(payload.Parameters, &parameters); err != nil {
			return err
		}
		cancel := make(chan struct{})
		s.mu.Lock()
		s.active[request.MessageID] = cancel
		s.mu.Unlock()
		s.progress(request.MessageID, 0, "sleep started", "start")
		go func() {
			status := "completed"
			select {
			case <-time.After(time.Duration(parameters.DurationMS) * time.Millisecond):
			case <-cancel:
				status = "cancelled"
			}
			s.mu.Lock()
			delete(s.active, request.MessageID)
			s.mu.Unlock()
			s.progress(request.MessageID, 1, "sleep "+status, "complete")
			s.result(request.MessageID, status, map[string]any{"duration_ms": parameters.DurationMS})
		}()
	default:
		s.send("response", "execute", request.MessageID, map[string]any{"type": "error", "code": "unknown_operation", "message": "unsupported operation", "retryable": false})
	}
	return nil
}
