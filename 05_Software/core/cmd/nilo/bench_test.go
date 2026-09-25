package main

import (
	"bytes"
	"context"
	"encoding/json"
	"nilo.local/nilo/internal/bench"
	"testing"
)

func TestBenchCLIQueuesOneCorrelatedCommandThroughHTTP(t *testing.T) {
	server, calls := networkStub(t, 202, map[string]any{"transaction_id": "transaction-one", "state": "queued"})
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := runBenchCommand(context.Background(), "bench", []string{"capture", "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--device", "atk-dl16-1.3", "--channels", "D0,D1", "--key", "repeatable-command", "--timeout-seconds", "17", "--json"}, networkIO{stdout: stdout, stderr: stderr})
	if code != 0 || len(*calls) != 1 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	call := (*calls)[0]
	if call.method != "POST" || call.path != "/api/v1/bench/jobs" || call.authorized != "Bearer test-token" {
		t.Fatal(call)
	}
	var input bench.Submission
	if err := json.Unmarshal([]byte(call.body), &input); err != nil {
		t.Fatal(err)
	}
	if input.IdempotencyKey != "repeatable-command" || input.TimeoutSeconds != 17 || input.Operation != "capture" {
		t.Fatal(input)
	}
}

func TestPluginCLIUsesManagedLifecycle(t *testing.T) {
	for _, action := range []string{"install", "enable", "disable", "restart", "uninstall", "discover"} {
		t.Run(action, func(t *testing.T) {
			server, calls := networkStub(t, 200, map[string]any{"id": "sigrok"})
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := runBenchCommand(context.Background(), "plugin", []string{action, "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--json"}, networkIO{stdout: stdout, stderr: stderr})
			if code != 0 || len(*calls) != 1 || (*calls)[0].path != "/api/v1/plugins/sigrok/"+action {
				t.Fatalf("code %d: %s", code, stderr)
			}
		})
	}
}
