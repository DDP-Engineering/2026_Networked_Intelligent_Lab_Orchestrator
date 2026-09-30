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

func TestModbusCLIMapSaveUsesCore(t *testing.T) {
	file := filepath.Join(t.TempDir(), "map.json")
	if err := os.WriteFile(file, []byte(`{"schema_version":1,"id":"motors","name":"Motors","points":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	server, calls := networkStub(t, 200, map[string]any{"id": "motors", "revision": 1})
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := runModbusCommand(context.Background(), []string{"map-save", "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--map", "motors", "--file", file}, networkIO{stdout: stdout, stderr: stderr})
	if code != 0 || len(*calls) != 1 || (*calls)[0].method != "PUT" || (*calls)[0].path != "/api/v1/modbus/maps/motors" {
		t.Fatalf("code=%d calls=%+v stderr=%s", code, *calls, stderr)
	}
}

func TestModbusCLIMapSavePreservesDecimalLexeme(t *testing.T) {
	file := filepath.Join(t.TempDir(), "map.json")
	raw := `{"schema_version":1,"id":"precise","name":"Precise","points":[{"id":"p","name":"Point","area":"holding","address":0,"type":"uint64","width":4,"order":"normal","unit":"","scale":1.00000000000000000001,"offset":0,"access":"read","write_min":null,"write_max":null,"labels":{}}]}`
	if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	server, calls := networkStub(t, 200, map[string]any{"id": "precise"})
	code := runModbusCommand(context.Background(), []string{"map-save", "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--map", "precise", "--file", file}, networkIO{stdout: new(bytes.Buffer), stderr: new(bytes.Buffer)})
	if code != 0 || len(*calls) != 1 || !bytes.Contains([]byte((*calls)[0].body), []byte("1.00000000000000000001")) {
		t.Fatalf("rounded map value: code=%d calls=%+v", code, *calls)
	}
}

func TestModbusCLIWorkspaceImportPreviewUsesCore(t *testing.T) {
	file := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(file, []byte(`{"schema_version":1,"id":"desk"}`), 0600); err != nil {
		t.Fatal(err)
	}
	server, calls := networkStub(t, 200, map[string]any{"can_apply": false})
	code := runModbusCommand(context.Background(), []string{"workspace-import-preview", "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--file", file}, networkIO{stdout: new(bytes.Buffer), stderr: new(bytes.Buffer)})
	if code != 0 || len(*calls) != 1 || (*calls)[0].path != "/api/v1/modbus/workspaces/import-preview" {
		t.Fatalf("route: code=%d calls=%+v", code, *calls)
	}
}
