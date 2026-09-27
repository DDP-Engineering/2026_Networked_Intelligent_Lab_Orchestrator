package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSerialCLIStatusUsesAuthenticatedHTTP(t *testing.T) {
	server, calls := networkStub(t, 200, map[string]any{"connected": false})
	stderr := new(bytes.Buffer)
	code := runBenchCommand(context.Background(), "serial", []string{"status", "--device", "serial-1", "--server", server.URL, "--token-file", tokenFile(t, "serial-token"), "--json"}, networkIO{stdout: new(bytes.Buffer), stderr: stderr})
	if code != 0 || len(*calls) != 1 {
		t.Fatalf("serial status missing: code=%d %s", code, stderr)
	}
	if c := (*calls)[0]; c.method != "GET" || c.path != "/api/v1/serial-devices/serial-1/workspace" || c.authorized != "Bearer serial-token" {
		t.Fatal(c)
	}
}

func TestSerialCLICommandsKeepSessionAndPayload(t *testing.T) {
	for _, tc := range []struct {
		args         []string
		method, path string
	}{
		{[]string{"connect", "--revision", "7", "--key", "connect-once"}, "POST", "/connect"},
		{[]string{"send", "--connection", "connection-one", "--key", "send-once", "--ending", "crlf"}, "POST", "/send"},
		{[]string{"signals", "--connection", "connection-one", "--rts=false"}, "POST", "/signals"},
		{[]string{"logging", "--connection", "connection-one", "--action", "start"}, "POST", "/logging"},
		{[]string{"disconnect", "--connection", "connection-one"}, "POST", "/disconnect"},
		{[]string{"logs", "--offset", "100"}, "GET", "/logs?offset=100"},
		{[]string{"retention"}, "GET", "/retention"},
	} {
		t.Run(tc.args[0], func(t *testing.T) {
			var method, path string
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.RequestURI()
				if r.Body != nil {
					_ = json.NewDecoder(r.Body).Decode(&body)
				}
				_, _ = io.WriteString(w, `{"ok":true}`)
			}))
			defer server.Close()
			args := append(tc.args, "--device", "serial-1", "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--json")
			errOut := new(bytes.Buffer)
			code := runSerialCommand(context.Background(), args, networkIO{stdin: strings.NewReader("  hello\n"), stdout: new(bytes.Buffer), stderr: errOut})
			if code != 0 || method != tc.method || path != "/api/v1/serial-devices/serial-1"+tc.path {
				t.Fatalf("code=%d method=%s path=%s err=%s", code, method, path, errOut)
			}
			if tc.args[0] == "send" && (body["data"] != "  hello\n" || body["connection_id"] != "connection-one" || body["ending"] != "crlf" || body["idempotency_key"] != "send-once") {
				t.Fatal(body)
			}
			if tc.args[0] == "signals" {
				if body["rts"] != false || len(body) != 2 {
					t.Fatal(body)
				}
			}
			if tc.args[0] == "connect" {
				settings, ok := body["settings"].(map[string]any)
				if !ok || body["revision"] != float64(7) || settings["baud_rate"] != float64(115200) || settings["dtr"] != true {
					t.Fatal(body)
				}
			}
		})
	}
}

func TestSerialCLIRejectsUnboundedOrAmbiguousInput(t *testing.T) {
	for _, args := range [][]string{
		{"read", "--device", "serial-1", "--connection", "c", "--wait-ms", "10001"},
		{"send", "--device", "serial-1"},
		{"send", "--device", "serial-1", "--connection", "c", "--data", "secret"},
		{"delete", "--id", "saved", "--confirm", "different"},
		{"signals", "--device", "serial-1", "--connection", "c"},
	} {
		if code := runSerialCommand(context.Background(), args, networkIO{stdout: io.Discard, stderr: io.Discard}); code != 2 {
			t.Fatalf("%v: %d", args, code)
		}
	}
	if _, err := serialPayload("-", strings.NewReader(strings.Repeat("a", 16385))); err == nil {
		t.Fatal("oversized payload accepted")
	}
	if _, err := serialPayload("-", bytes.NewReader([]byte{255})); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	file := filepath.Join(t.TempDir(), "send.hex")
	if err := os.WriteFile(file, []byte("00 ff\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := serialPayload(file, nil); err != nil || got != "00 ff\n" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestSerialCLIReadFollowsCursorAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		want := "0"
		if calls == 2 {
			want = "7"
		}
		if r.URL.Query().Get("after") != want || r.URL.Query().Get("connection_id") != "connection-one" {
			t.Errorf("query=%s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"events":[],"next_cursor":7,"gap":true}`)
		if calls == 2 {
			cancel()
		}
	}))
	defer server.Close()
	output := new(bytes.Buffer)
	code := runSerialCommand(ctx, []string{"read", "--device", "serial-1", "--connection", "connection-one", "--follow", "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--json"}, networkIO{stdout: output, stderr: io.Discard})
	if code != 0 || calls != 2 || !strings.Contains(output.String(), `"gap":true`) {
		t.Fatalf("code=%d calls=%d output=%s", code, calls, output)
	}
}

func TestSerialCLIExportAndDeletionUseLogRoutes(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/content") {
			_, _ = w.Write([]byte{0, 255, 10})
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	remote := []string{"--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--json"}
	out := filepath.Join(t.TempDir(), "capture.bin")
	streams := networkIO{stdout: io.Discard, stderr: io.Discard}
	args := append([]string{"export", "--id", "log-one", "--format", "rx", "--out", out}, remote...)
	if code := runSerialCommand(context.Background(), args, streams); code != 0 {
		t.Fatalf("export code=%d", code)
	}
	if data, err := os.ReadFile(out); err != nil || !bytes.Equal(data, []byte{0, 255, 10}) {
		t.Fatalf("raw export=%v %v", data, err)
	}
	if code := runSerialCommand(context.Background(), args, streams); code != 1 {
		t.Fatalf("existing file overwritten: %d", code)
	}
	args = append([]string{"delete", "--id", "log-one", "--confirm", "log-one"}, remote...)
	if code := runSerialCommand(context.Background(), args, streams); code != 0 {
		t.Fatalf("delete code=%d", code)
	}
	if calls[0] != "GET /api/v1/serial-logs/log-one/content?format=rx" || calls[len(calls)-1] != "DELETE /api/v1/serial-logs/log-one" {
		t.Fatal(calls)
	}
}

func TestSerialCLIUnknownSendIsNeverAutomaticallyRetried(t *testing.T) {
	server, calls := networkStub(t, 200, map[string]any{"status": "unknown", "bytes_written": 0})
	code := runSerialCommand(context.Background(), []string{"send", "--device", "serial-1", "--connection", "connection-one", "--key", "send-once", "--server", server.URL, "--token-file", tokenFile(t, "test-token")}, networkIO{stdin: strings.NewReader("hello"), stdout: io.Discard, stderr: io.Discard})
	if code != 0 || len(*calls) != 1 {
		t.Fatalf("code=%d calls=%d", code, len(*calls))
	}
}
