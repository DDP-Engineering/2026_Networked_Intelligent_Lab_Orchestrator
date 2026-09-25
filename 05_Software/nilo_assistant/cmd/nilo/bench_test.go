package main

import (
	"bytes"
	"context"
	"encoding/json"
	"nilo.local/nilo/internal/bench"
	"os"
	"path/filepath"
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

func TestCameraCLIQueuesPhysicalSnapshotAndTypedControls(t *testing.T) {
	for _, test := range []struct {
		args      []string
		operation string
	}{
		{[]string{"snapshot", "--device", "cam-1", "--zoom", "2", "--rotation", "90"}, "snapshot"},
		{[]string{"snapshot", "--device", "cam-1", "--maximum-dimension", "320"}, "snapshot"},
		{[]string{"controls", "get", "--device", "cam-1"}, "get_controls"},
		{[]string{"controls", "reset", "--device", "cam-1"}, "reset_controls"},
	} {
		server, calls := networkStub(t, 202, map[string]any{"transaction_id": "transaction-one", "state": "queued"})
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		args := append(test.args, "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--json")
		if code := runBenchCommand(context.Background(), "camera", args, networkIO{stdout: stdout, stderr: stderr}); code != 0 {
			t.Fatalf("%s: %d %s", test.operation, code, stderr)
		}
		var input bench.Submission
		if len(*calls) != 1 || json.Unmarshal([]byte((*calls)[0].body), &input) != nil || input.PluginID != "camera" || input.DeviceID != "cam-1" || input.Operation != test.operation {
			t.Fatalf("%s: call %+v, input %+v", test.operation, *calls, input)
		}
		if test.operation == "snapshot" && len(test.args) > 4 && test.args[3] == "--maximum-dimension" {
			var parameters map[string]any
			if err := json.Unmarshal(input.Parameters, &parameters); err != nil || parameters["maximum_dimension"] != float64(320) {
				t.Fatalf("maximum dimension was not submitted: %s, %v", input.Parameters, err)
			}
		}
	}
}

func TestBenchCLIGenericSubmitKeepsCameraOperation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, []byte(`{"width":1280,"height":720}`), 0600); err != nil {
		t.Fatal(err)
	}
	server, calls := networkStub(t, 202, map[string]any{"transaction_id": "transaction-one", "state": "queued"})
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := runBenchCommand(context.Background(), "bench", []string{"submit", "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--plugin", "camera", "--operation", "snapshot", "--device", "cam-1", "--parameters-file", path, "--json"}, networkIO{stdout: stdout, stderr: stderr})
	if code != 0 || len(*calls) != 1 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	var input bench.Submission
	if err := json.Unmarshal([]byte((*calls)[0].body), &input); err != nil || input.PluginID != "camera" || input.Operation != "snapshot" || string(input.Parameters) != `{"height":720,"width":1280}` {
		t.Fatalf("generic submit = %+v, %v", input, err)
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

func TestPluginCLIDeviceActionsCarryRevisionsAndExactConfirmation(t *testing.T) {
	for _, test := range []struct {
		args         []string
		method, path string
	}{
		{[]string{"devices", "--plugin", "camera"}, "GET", "/api/v1/plugins/camera/devices"},
		{[]string{"device", "rebind", "--plugin", "camera", "--device", "cam-1", "--revision", "4", "--provisional", "cam-2", "--provisional-revision", "1", "--confirm", "cam-1"}, "POST", "/api/v1/plugins/camera/devices/cam-1/rebind"},
		{[]string{"device", "forget", "--plugin", "camera", "--device", "cam-1", "--revision", "4", "--confirm", "cam-1"}, "DELETE", "/api/v1/plugins/camera/devices/cam-1"},
	} {
		server, calls := networkStub(t, 200, map[string]any{"ok": true})
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		args := append(test.args, "--server", server.URL, "--token-file", tokenFile(t, "test-token"), "--json")
		code := runBenchCommand(context.Background(), "plugin", args, networkIO{stdout: stdout, stderr: stderr})
		if code != 0 || len(*calls) != 1 || (*calls)[0].method != test.method || (*calls)[0].path != test.path {
			t.Fatalf("%v: code %d, calls %+v, stderr %s", test.args, code, *calls, stderr)
		}
	}
}
