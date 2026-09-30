package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestModbusCLIUsesAuthenticatedCoreRoute(t *testing.T) {
	server, calls := networkStub(t, 202, map[string]any{"transaction_id": "admission", "state": "queued"})
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := runModbusCommand(context.Background(), []string{"connect", "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--endpoint", "gateway", "--endpoint-revision", "2", "--profile", "profile-1", "--profile-revision", "4", "--key", "modbus-connect-key", "--json"}, networkIO{stdout: stdout, stderr: stderr})
	if code != 0 || len(*calls) != 1 {
		t.Fatalf("code=%d calls=%d stderr=%s", code, len(*calls), stderr)
	}
	if (*calls)[0].method != "POST" || (*calls)[0].path != "/api/v1/modbus/endpoints/gateway/connect" || (*calls)[0].authorized != "Bearer test-token" {
		t.Fatal((*calls)[0])
	}
}

func TestModbusCLIPollRequiresGroupsArray(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(file, []byte(`{"not_groups":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := runModbusCommand(context.Background(), []string{"poll", "--server", "https://localhost", "--token-file", tokenFile(t, "test-token"), "--endpoint", "gateway", "--session", "session-1", "--file", file}, networkIO{stdout: stdout, stderr: stderr})
	if code != 2 || !bytes.Contains(stderr.Bytes(), []byte("groups array")) {
		t.Fatalf("invalid plan: code=%d stderr=%s", code, stderr)
	}
}
