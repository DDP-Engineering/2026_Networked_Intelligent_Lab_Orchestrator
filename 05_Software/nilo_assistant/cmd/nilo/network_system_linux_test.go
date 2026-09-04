//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// disposableTunnelKeys makes a real Curve25519 pair for a test peer. The keys
// live for one container and identify nothing outside it.
func disposableTunnelKeys(t *testing.T) (private, public string) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key.Bytes()), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
}

// Runs only in the disposable namespace created by 02_Tools/tests/network.
// The actual binaries run as separate root helper and unprivileged core users.
func TestApplianceSystemHTTPSAndHelperRecovery(t *testing.T) {
	if os.Getenv("NILO_NETWORK_SYSTEM") != "1" {
		t.Skip("requires isolated system harness")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil || os.Geteuid() != 0 {
		t.Fatal("requires root inside disposable container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	root, err := os.MkdirTemp("/tmp", "nilo-appliance-")
	if err != nil {
		t.Fatal(err)
	}
	// This directory and every child were created by this test in /tmp.
	defer os.RemoveAll(root)
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"state", "run", "data", "gui", "tls"} {
		if err = os.Mkdir(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.Chmod(filepath.Join(root, "state"), 0700)
	_ = os.Chmod(filepath.Join(root, "run"), 0750)
	_ = os.Chown(filepath.Join(root, "run"), 0, 65534)
	_ = os.Chown(filepath.Join(root, "data"), 65534, 65534)
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	certificate := fixture.TLS.Certificates[0]
	leaf := fixture.Certificate()
	fixture.Close()
	keyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(root, "tls", "server.crt")
	keyPath := filepath.Join(root, "tls", "server.key")
	_ = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0644)
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0640)
	_ = os.Chown(keyPath, 0, 65534)
	_ = os.WriteFile(filepath.Join(root, "gui", "index.html"), []byte("<!doctype html><html><head></head><body>NILO appliance interface</body></html>"), 0644)
	socket := filepath.Join(root, "run", "nethelper.sock")
	helper := exec.CommandContext(ctx, "/tmp/nilo-nethelper", "--socket", socket, "--state-dir", filepath.Join(root, "state"), "--allow-uid", "65534", "--socket-gid", "65534")
	helper.Stderr = os.Stderr
	if err = helper.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = helper.Process.Signal(syscall.SIGTERM); _ = helper.Wait() }()
	waitSystem(t, ctx, func() bool { _, e := os.Stat(socket); return e == nil })
	asService := func(args ...string) *exec.Cmd {
		c := exec.CommandContext(ctx, "/tmp/nilo", args...)
		c.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534}}
		return c
	}
	bootstrap := asService("admin", "bootstrap", "--data-dir", filepath.Join(root, "data"), "--username", "appliance-test")
	password := "appliance-test-password-9346"
	bootstrap.Stdin = strings.NewReader(password + "\n" + password + "\n")
	if output, e := bootstrap.CombinedOutput(); e != nil {
		t.Fatalf("bootstrap: %v %s", e, output)
	}
	var core *exec.Cmd
	startCore := func() string {
		core = asService("serve", "--profile", "appliance", "--data-dir", filepath.Join(root, "data"), "--bind", "127.0.0.1:0", "--gui-dir", filepath.Join(root, "gui"), "--tls-cert", certPath, "--tls-key", keyPath, "--nethelper-socket", socket)
		stdout, e := core.StdoutPipe()
		if e != nil {
			t.Fatal(e)
		}
		core.Stderr = os.Stderr
		if e = core.Start(); e != nil {
			t.Fatal(e)
		}
		var ready struct {
			Address string `json:"address"`
		}
		if e = json.NewDecoder(stdout).Decode(&ready); e != nil {
			record, _ := os.ReadFile(filepath.Join(root, "state", "recovery.json"))
			t.Logf("recovery record: %s", record)
			t.Fatalf("start appliance: %v", e)
		}
		return "https://" + ready.Address
	}
	stopCore := func() {
		if core != nil {
			_ = core.Process.Signal(syscall.SIGTERM)
			_ = core.Wait()
			core = nil
		}
	}
	base := startCore()
	defer stopCore()
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Transport: transport, Jar: jar, Timeout: 35 * time.Second}
	csrf := ""
	var lastBody []byte
	request := func(method, path string, body any, want int) map[string]any {
		var payload io.Reader
		if body != nil {
			b, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			payload = bytes.NewReader(b)
		}
		req, e := http.NewRequestWithContext(ctx, method, base+path, payload)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", base)
		if csrf != "" {
			req.Header.Set("X-NILO-CSRF", csrf)
		}
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		lastBody = b
		if resp.StatusCode != want {
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, b)
		}
		var decoded map[string]any
		_ = json.Unmarshal(b, &decoded)
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "nilo_csrf" {
				csrf = cookie.Value
			}
			if cookie.Name == "nilo_session" && !cookie.Secure {
				t.Fatal("insecure appliance session")
			}
		}
		return decoded
	}
	request("GET", "/network", nil, 200)
	request("GET", "/api/v1/network/status", nil, 401)
	request("POST", "/api/v1/auth/login", map[string]string{"username": "appliance-test", "password": password}, 200)
	status := request("GET", "/api/v1/status", nil, 200)
	if status["simulation"] != false || status["profile"] != "appliance" {
		t.Fatalf("false runtime status: %v", status)
	}
	vpn := request("GET", "/api/v1/network/wireguard", nil, 200)
	if vpn["readiness_only"] == true || vpn["measured_by"] == "simulation" || vpn["networkmanager_available"] != true || vpn["can_install"] != false {
		t.Fatalf("incorrect live readiness: %v", vpn)
	}
	if profiles, _ := vpn["profiles"].([]any); len(profiles) != 0 {
		t.Fatalf("a fresh appliance lists %d profiles, want none", len(profiles))
	}
	tools := vpn["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["installed"] != true || tools[0].(map[string]any)["version"] == "" {
		t.Fatalf("tool probe not exposed through HTTPS: %v", tools)
	}
	// The already-installed path still goes through permission, preview,
	// acknowledgement, helper job and the production maintenance poll.
	install := request("POST", "/api/v1/network/wireguard/install/preview", map[string]any{}, 200)
	installPath := "/api/v1/network/operations/" + install["id"].(string)
	install = request("POST", installPath+"/acknowledge", map[string]any{"revision": install["revision"]}, 202)
	if install["state"] != "applying" || install["progress"] == nil {
		t.Fatalf("installation progress not exposed: %v", install)
	}
	waitSystem(t, ctx, func() bool {
		install = request("GET", installPath, nil, 200)
		return install["state"] != "applying"
	})
	if install["state"] != "confirmed" || install["progress"] != "complete" {
		t.Fatalf("installation result: %v", install)
	}
	inventory := request("GET", "/api/v1/devices", nil, 200)
	if len(inventory["items"].([]any)) != 0 {
		t.Fatal("simulated instruments in appliance")
	}
	readInterface := func() (string, string) {
		status := request("GET", "/api/v1/network/status", nil, 200)
		if status["measured_by"] == "simulation" {
			t.Fatal("live reading marked simulated")
		}
		for _, raw := range status["interfaces"].([]any) {
			d := raw.(map[string]any)
			if d["name"] == "eth0" {
				return d["id"].(string), d["state"].(string)
			}
		}
		t.Fatal("missing eth0")
		return "", ""
	}
	id, state := readInterface()
	if state != "connected" {
		t.Fatal(state)
	}
	// Confirm disable, then enable without a UUID: the helper must remember the
	// OS-owned saved connection and never modify its persistent settings.
	change := func(action string) string {
		preview := request("POST", "/api/v1/network/interfaces/"+id+"/preview", map[string]string{"action": action}, 200)
		op := preview["id"].(string)
		applied := request("POST", "/api/v1/network/operations/"+op+"/acknowledge", map[string]any{"revision": preview["revision"]}, 202)
		if applied["state"] != "awaiting_confirmation" {
			t.Fatalf("operation not applied: %v", applied)
		}
		return op
	}
	op := change("disable")
	request("POST", "/api/v1/network/operations/"+op+"/confirm", map[string]any{}, 200)
	_, state = readInterface()
	if state != "disconnected" {
		t.Fatal(state)
	}
	op = change("enable")
	request("POST", "/api/v1/network/operations/"+op+"/confirm", map[string]any{}, 200)
	// Kill Bench Core while a change is unconfirmed. Its replacement must ask
	// the surviving privileged helper to reconcile before it accepts HTTP.
	_ = change("disable")
	_ = core.Process.Kill()
	_ = core.Wait()
	core = nil
	base = startCore()
	_, state = readInterface()
	if state != "connected" {
		t.Fatalf("core restart did not recover: %s", state)
	}
	t.Log("unprivileged HTTPS core, root helper, confirmed disable/enable and killed-core restart recovery verified")

	// --- WireGuard profile lifecycle over the same HTTPS path (P2 exit) ---
	// Two disposable peers. The private keys are sentinels: after the run they
	// must appear in no response, no operation record and nowhere in the
	// application database, which the root test reads directly.
	privateA, _ := disposableTunnelKeys(t)
	privateB, _ := disposableTunnelKeys(t)
	_, peerA := disposableTunnelKeys(t)
	_, peerB := disposableTunnelKeys(t)
	profileText := func(private, peer, address, allowed, endpoint string) string {
		return "[Interface]\nPrivateKey = " + private + "\nAddress = " + address + "\nDNS = 10.66.1.1\n" +
			"[Peer]\nPublicKey = " + peer + "\nAllowedIPs = " + allowed + "\nEndpoint = " + endpoint + "\n"
	}
	wireguard := func(action string, extra map[string]any, want int) map[string]any {
		body := map[string]any{"action": action}
		for key, value := range extra {
			body[key] = value
		}
		preview := request("POST", "/api/v1/network/wireguard/preview", body, want)
		if want != 200 {
			return preview
		}
		return request("POST", "/api/v1/network/operations/"+preview["id"].(string)+"/acknowledge", map[string]any{"revision": preview["revision"]}, 202)
	}
	profilesByName := func() map[string]map[string]any {
		vpn := request("GET", "/api/v1/network/wireguard", nil, 200)
		if strings.Contains(string(lastBody), privateA) || strings.Contains(string(lastBody), privateB) {
			t.Fatal("the WireGuard status carries a private key")
		}
		result := map[string]map[string]any{}
		for _, raw := range vpn["profiles"].([]any) {
			profile := raw.(map[string]any)
			result[profile["name"].(string)] = profile
		}
		return result
	}
	if applied := wireguard("import", map[string]any{"target": "office", "profile_text": profileText(privateA, peerA, "10.66.1.2/32", "10.66.0.0/24", "192.0.2.10:51820")}, 200); applied["state"] != "confirmed" {
		t.Fatalf("import office: %v", applied)
	}
	if applied := wireguard("import", map[string]any{"target": "site-b", "profile_text": profileText(privateB, peerB, "10.67.1.2/32", "0.0.0.0/0", "192.0.2.11:51820")}, 200); applied["state"] != "confirmed" {
		t.Fatalf("import site-b: %v", applied)
	}
	profiles := profilesByName()
	if len(profiles) != 2 {
		t.Fatalf("profiles after two imports = %v", profiles)
	}
	for name, profile := range profiles {
		if profile["nilo_owned"] != true || profile["active"] != false || profile["autoconnect"] != false {
			t.Fatalf("%s after import = %v, want inactive, NILO-owned, not selected", name, profile)
		}
	}
	office, siteB := profiles["office"]["id"].(string), profiles["site-b"]["id"].(string)
	// Activate, then switch, then put the switch back explicitly.
	activated := wireguard("activate", map[string]any{"target": office}, 200)
	if activated["state"] != "awaiting_confirmation" {
		t.Fatalf("activate office: %v", activated)
	}
	request("POST", "/api/v1/network/operations/"+activated["id"].(string)+"/confirm", map[string]any{}, 200)
	profiles = profilesByName()
	if profiles["office"]["active"] != true || profiles["office"]["interface"] != "nilo0" {
		t.Fatalf("office after activation = %v", profiles["office"])
	}
	switched := wireguard("activate", map[string]any{"target": siteB}, 200)
	profiles = profilesByName()
	if profiles["site-b"]["active"] != true || profiles["office"]["active"] != false {
		t.Fatalf("after the switch office=%v site-b=%v", profiles["office"], profiles["site-b"])
	}
	if rolled := request("POST", "/api/v1/network/operations/"+switched["id"].(string)+"/rollback", map[string]any{}, 200); rolled["state"] != "rolled_back" {
		t.Fatalf("rollback of the switch: %v", rolled)
	}
	profiles = profilesByName()
	if profiles["office"]["active"] != true || profiles["site-b"]["active"] != false {
		t.Fatalf("after rolling the switch back office=%v site-b=%v", profiles["office"], profiles["site-b"])
	}
	// Rename keeps the identity; removing the connected profile is refused.
	if renamed := wireguard("rename", map[string]any{"target": siteB, "name": "Site B"}, 200); renamed["state"] != "confirmed" {
		t.Fatalf("rename: %v", renamed)
	}
	profiles = profilesByName()
	if profiles["Site B"] == nil || profiles["Site B"]["id"] != siteB {
		t.Fatalf("rename lost the identity: %v", profiles)
	}
	if refused := wireguard("remove", map[string]any{"target": office}, 409); refused["error"] != "wireguard_profile_active" {
		t.Fatalf("removing the connected profile: %v", refused)
	}
	// Deactivate, select Site B for startup, restart the core: the system
	// connects the selection before it serves, attributed to nobody but itself.
	deactivated := wireguard("deactivate", map[string]any{}, 200)
	request("POST", "/api/v1/network/operations/"+deactivated["id"].(string)+"/confirm", map[string]any{}, 200)
	if selected := wireguard("autostart", map[string]any{"target": siteB, "enabled": true}, 200); selected["state"] != "confirmed" {
		t.Fatalf("startup selection: %v", selected)
	}
	stopCore()
	base = startCore()
	profiles = profilesByName()
	if profiles["Site B"]["active"] != true || profiles["Site B"]["autoconnect"] != true || profiles["office"]["active"] != false {
		t.Fatalf("after the restart office=%v Site B=%v, want the selection connected", profiles["office"], profiles["Site B"])
	}
	operations := request("GET", "/api/v1/network/operations", nil, 200)
	if strings.Contains(string(lastBody), privateA) || strings.Contains(string(lastBody), privateB) {
		t.Fatal("the operation listing carries a private key")
	}
	if items, _ := operations["items"].([]any); len(items) < 8 {
		t.Fatalf("operation history after the lifecycle holds %d entries", len(items))
	}
	// The application database and its journal are readable here as root.
	dataFiles, err := filepath.Glob(filepath.Join(root, "data", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range dataFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if bytes.Contains(content, []byte(privateA)) || bytes.Contains(content, []byte(privateB)) {
			t.Fatalf("%s carries an imported private key", path)
		}
	}
	// The helper keystore is the one place the keys live, and it is root-only.
	secretEntries, err := os.ReadDir(filepath.Join(root, "state", "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	stored := 0
	for _, entry := range secretEntries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 || info.Sys().(*syscall.Stat_t).Uid != 0 {
			t.Fatalf("keystore entry %s is not root-only", entry.Name())
		}
		stored++
	}
	if stored != 2 {
		t.Fatalf("%d keystore entries, want 2", stored)
	}
	// Clean up through the product path: disconnect, then remove both.
	deactivated = wireguard("deactivate", map[string]any{}, 200)
	request("POST", "/api/v1/network/operations/"+deactivated["id"].(string)+"/confirm", map[string]any{}, 200)
	for _, target := range []string{office, siteB} {
		if removed := wireguard("remove", map[string]any{"target": target}, 200); removed["state"] != "confirmed" {
			t.Fatalf("remove %s: %v", target, removed)
		}
	}
	if profiles = profilesByName(); len(profiles) != 0 {
		t.Fatalf("profiles after removal = %v", profiles)
	}
	if secretEntries, _ = os.ReadDir(filepath.Join(root, "state", "secrets")); len(secretEntries) != 0 {
		t.Fatalf("%d keystore entries survived removal", len(secretEntries))
	}
	t.Log("two imported profiles survived a restart, were switched, put back, renamed, selected for startup and connected by the system, with no key outside the root-only keystore")
}

func waitSystem(t *testing.T, ctx context.Context, ready func() bool) {
	t.Helper()
	for !ready() {
		select {
		case <-ctx.Done():
			t.Fatal(fmt.Errorf("system startup: %w", ctx.Err()))
		case <-time.After(50 * time.Millisecond):
		}
	}
}
