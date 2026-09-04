package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNetworkVPNCommandsUsePreviewAndSeparateAcknowledgement(t *testing.T) {
	file := filepath.Join(t.TempDir(), "profile.conf")
	deviceNetworkFile := filepath.Join(t.TempDir(), "line-3.json")
	if err := os.WriteFile(deviceNetworkFile, []byte(`{"name":"Line 3","device_interface":"02:00:00:00:00:02","appliance_address":"192.168.0.1/24","scope":"host","target":"192.168.0.130","access":"local","local_interface":"02:00:00:00:00:01","local_sources":["192.168.10.0/24"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("profile sent as data"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args     []string
		path     string
		mutation bool
	}{
		{[]string{"vpn", "check"}, "/api/v1/network/wireguard", false},
		{[]string{"vpn", "profiles", "list"}, "/api/v1/network/wireguard/profiles", false},
		{[]string{"vpn", "install"}, "/api/v1/network/wireguard/install/preview", true},
		{[]string{"vpn", "profiles", "import", "--file", file, "--name", "office"}, "/api/v1/network/wireguard/preview", true},
		{[]string{"vpn", "profiles", "remove", "office"}, "/api/v1/network/wireguard/preview", true},
		{[]string{"vpn", "profiles", "rename", "office", "--name", "Head office"}, "/api/v1/network/wireguard/preview", true},
		{[]string{"vpn", "enable", "office"}, "/api/v1/network/wireguard/preview", true},
		{[]string{"vpn", "disable"}, "/api/v1/network/wireguard/preview", true},
		{[]string{"vpn", "autostart", "office", "--enabled", "false"}, "/api/v1/network/wireguard/preview", true},
		{[]string{"interface", "disable", "sim-enp1s0"}, "/api/v1/network/interfaces/sim-enp1s0/preview", true},
		{[]string{"interface", "settings", "sim-enp1s0", "--method", "static", "--address", "192.168.0.1/24", "--dns", "192.168.0.53", "--never-default"}, "/api/v1/network/interfaces/sim-enp1s0/settings/preview", true},
		{[]string{"wifi", "scan", "sim-wlan0"}, "/api/v1/network/interfaces/sim-wlan0/wifi", false},
		{[]string{"wifi", "join", "sim-wlan0", "--ssid", "lab-hotspot", "--security", "wpa2-personal", "--password-file", file}, "/api/v1/network/interfaces/sim-wlan0/wifi/preview", true},
		{[]string{"device-networks", "list"}, "/api/v1/network/device-networks", false},
		{[]string{"device-networks", "show", "line-3"}, "/api/v1/network/device-networks/line-3", false},
		{[]string{"device-networks", "instructions", "line-3"}, "/api/v1/network/device-networks/line-3/access-instructions", false},
		{[]string{"device-networks", "create", "--file", deviceNetworkFile}, "/api/v1/network/device-networks/preview", true},
		{[]string{"device-networks", "update", "line-3", "--file", deviceNetworkFile}, "/api/v1/network/device-networks/preview", true},
		{[]string{"device-networks", "enable", "line-3"}, "/api/v1/network/device-networks/preview", true},
		{[]string{"device-networks", "disable", "line-3"}, "/api/v1/network/device-networks/preview", true},
		{[]string{"device-networks", "delete", "line-3"}, "/api/v1/network/device-networks/preview", true},
		{[]string{"device-networks", "check", "line-3", "--tcp-port", "502"}, "/api/v1/network/device-networks/line-3/checks", false},
	} {
		t.Run(strings.Join(tc.args[:2], " "), func(t *testing.T) {
			server, calls := networkStub(t, http.StatusOK, map[string]any{"id": "operation-a", "revision": "revision-a", "state": "awaiting_confirmation"})
			args := append([]string{}, tc.args...)
			args = append(args, "--server", server.URL, "--token-file", tokenFile(t, "token-from-file"), "--json")
			if tc.mutation {
				args = append(args, "--acknowledge")
			}
			code, _, stderr := runNetwork(args)
			if code != 0 {
				t.Fatalf("exit=%d: %s", code, stderr)
			}
			want := 1
			if tc.mutation {
				want = 2
			}
			if len(*calls) != want {
				t.Fatalf("calls=%d want=%d", len(*calls), want)
			}
			if (*calls)[0].path != tc.path {
				t.Fatalf("path=%s", (*calls)[0].path)
			}
			for _, call := range *calls {
				if strings.HasSuffix(call.path, "/confirm") || call.authorized != "Bearer token-from-file" {
					t.Fatal("bypassed separate confirmation or authentication")
				}
			}
		})
	}
}

func TestNoninteractiveNetworkMutationRequiresExplicitAcknowledgement(t *testing.T) {
	server, calls := networkStub(t, http.StatusOK, map[string]any{"id": "operation-a", "revision": "revision-a", "state": "previewed"})
	code, _, _ := runNetwork([]string{"interface", "disable", "sim-enp1s0", "--server", server.URL, "--token-file", tokenFile(t, "token")})
	if code != 2 || len(*calls) != 1 {
		t.Fatalf("noninteractive mutation: exit=%d calls=%d", code, len(*calls))
	}
}

func TestStartupPreviewShowsSelectedValue(t *testing.T) {
	server, _ := networkStub(t, http.StatusOK, map[string]any{"id": "operation-a", "revision": "revision-a", "state": "previewed"})
	_, _, stderr := runNetwork([]string{"vpn", "autostart", "office", "--enabled", "false", "--server", server.URL, "--token-file", tokenFile(t, "token")})
	if !strings.Contains(stderr, "Connect at startup: false") {
		t.Fatalf("missing selected startup value: %s", stderr)
	}
}

// TestActivityAndDiagnosticsCommandsCallTheAuthenticatedAPI covers the CLI
// door of REQ-SEC-d19a7b30 and REQ-FUNC-d4f81a26: every command reaches the
// activity or diagnostics route with the bearer token and its filters, and an
// export is copied untouched.
func TestActivityAndDiagnosticsCommandsCallTheAuthenticatedAPI(t *testing.T) {
	for _, tc := range []struct {
		args         []string
		path, query  string
		method, body string
	}{
		{[]string{"list", "--category", "audit", "--operation", "op-1", "--limit", "5"}, "/api/v1/activity", "category=audit&limit=5&operation=op-1", http.MethodGet, ""},
		{[]string{"export", "--format", "csv", "--component", "network.wifi"}, "/api/v1/activity/export", "component=network.wifi&format=csv", http.MethodGet, ""},
	} {
		t.Run(strings.Join(tc.args[:1], " "), func(t *testing.T) {
			server, calls := networkStub(t, http.StatusOK, map[string]any{"items": []any{}})
			args := append([]string{}, tc.args...)
			args = append(args, "--server", server.URL, "--token-file", tokenFile(t, "token-from-file"), "--json")
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := runActivityCommand(context.Background(), args, networkIO{stdout: stdout, stderr: stderr})
			if code != 0 {
				t.Fatalf("exit=%d: %s", code, stderr)
			}
			if len(*calls) != 1 || (*calls)[0].path != tc.path || (*calls)[0].rawQuery != tc.query || (*calls)[0].authorized != "Bearer token-from-file" {
				t.Fatalf("calls = %+v", *calls)
			}
			if !strings.Contains(stdout.String(), "items") {
				t.Fatalf("stdout = %q", stdout.String())
			}
		})
	}
	for _, tc := range []struct {
		args   []string
		path   string
		method string
	}{
		{[]string{"sessions"}, "/api/v1/diagnostics/sessions", http.MethodGet},
		{[]string{"start", "--scope", "network.wireguard", "--target", "office", "--minutes", "15"}, "/api/v1/diagnostics/sessions", http.MethodPost},
		{[]string{"show", "s-1"}, "/api/v1/diagnostics/sessions/s-1", http.MethodGet},
		{[]string{"stop", "s-1"}, "/api/v1/diagnostics/sessions/s-1/stop", http.MethodPost},
		{[]string{"settings"}, "/api/v1/diagnostics/settings", http.MethodGet},
		{[]string{"settings", "--retention", "audit=30,event=7,debug=1", "--quota", "audit=16777216,event=8388608,debug=4194304"}, "/api/v1/diagnostics/settings", http.MethodPut},
	} {
		t.Run(strings.Join(tc.args[:1], " "), func(t *testing.T) {
			server, calls := networkStub(t, http.StatusOK, map[string]any{"id": "s-1", "state": "active"})
			args := append([]string{}, tc.args...)
			args = append(args, "--server", server.URL, "--token-file", tokenFile(t, "token-from-file"), "--json")
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := runDiagnosticsCommand(context.Background(), args, networkIO{stdout: stdout, stderr: stderr})
			if code != 0 {
				t.Fatalf("exit=%d: %s", code, stderr)
			}
			if len(*calls) != 1 || (*calls)[0].path != tc.path || (*calls)[0].method != tc.method || (*calls)[0].authorized != "Bearer token-from-file" {
				t.Fatalf("calls = %+v", *calls)
			}
			if tc.method == http.MethodPut && !strings.Contains((*calls)[0].body, `"retention_days":{"audit":30`) {
				t.Fatalf("body = %s", (*calls)[0].body)
			}
		})
	}
}
