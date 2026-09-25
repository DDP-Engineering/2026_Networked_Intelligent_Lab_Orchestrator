//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The appliance verification stage.
//
// Runs only inside the disposable container created by
// 02_Tools/tests/appliance/check.sh, where a real dbus-daemon, a real
// NetworkManager and this container's own eth0 are already up. Nothing here is
// simulated: the adapters asserted below are the ones NetworkManager detected
// in that namespace, and the privileged helper and Bench Core are the binaries
// an appliance installs.
//
// The stage opts in explicitly so a developer's `go test ./...` skips it.

const applianceStageServiceUID = 65534

func requireApplianceStage(t *testing.T) {
	t.Helper()
	if os.Getenv("NILO_APPLIANCE_STAGE") != "1" {
		t.Skip("requires the appliance verification stage container")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil || os.Geteuid() != 0 {
		t.Fatal("the appliance stage runs as root inside its own disposable container")
	}
}

// applianceStageRoot lays out the directories the appliance profile needs, with
// the ownership the real deployment has: a root-owned helper state directory, a
// service-owned data directory, and TLS material the service account can read
// but not write.
func applianceStageRoot(t *testing.T) (root, certPath, keyPath string, leaf *x509.Certificate) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "nilo-appliance-stage-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
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
	_ = os.Chown(filepath.Join(root, "run"), 0, applianceStageServiceUID)
	_ = os.Chown(filepath.Join(root, "data"), applianceStageServiceUID, applianceStageServiceUID)

	// TLS material is minted here and dies with the container: no certificate
	// is checked in, and none outlives the run.
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		t.Fatalf("read the appliance-stage hostname: %v", err)
	}
	certificate, leaf := disposableServerCertificate(
		t,
		[]string{hostname + ".local"},
		[]net.IP{net.ParseIP("127.0.0.1")},
	)
	keyDER, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	certPath = filepath.Join(root, "tls", "server.crt")
	keyPath = filepath.Join(root, "tls", "server.key")
	_ = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0644)
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0640)
	_ = os.Chown(keyPath, 0, applianceStageServiceUID)
	_ = os.WriteFile(filepath.Join(root, "gui", "index.html"),
		[]byte("<!doctype html><html><head></head><body>NILO appliance interface</body></html>"), 0644)
	return root, certPath, keyPath, leaf
}

// nmcliDevices returns the adapters NetworkManager reports in this namespace.
func nmcliDevices(t *testing.T, ctx context.Context) []string {
	t.Helper()
	output, err := exec.CommandContext(ctx, "nmcli", "--escape", "no", "-t", "-f", "DEVICE", "device", "status").Output()
	if err != nil {
		t.Fatalf("read the NetworkManager device list: %v", err)
	}
	names := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// TestApplianceStageStartsUnderRealHelperAndNetworkManager proves the stage
// stands up the pairing an appliance ships and reports where it listens.
//
// Covers REQ-TEST-3a6e2094.
func TestApplianceStageStartsUnderRealHelperAndNetworkManager(t *testing.T) {
	requireApplianceStage(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	stage := startApplianceStage(t, ctx)
	defer stage.stop()

	// Unauthenticated first: the boundary is the same one an appliance serves.
	stage.expect(t, ctx, "GET", "/api/v1/status", http.StatusUnauthorized)
	stage.login(t, ctx)
	status := stage.get(t, ctx, "/api/v1/status")
	if status["profile"] != "appliance" {
		t.Fatalf("the stage did not serve the appliance profile: %v", status)
	}
	t.Logf("appliance listening at %s under a root helper and a real NetworkManager", stage.base)
}

// TestApplianceStageReportsDetectedAdapters proves the interface would show
// what this machine actually has.
//
// Covers REQ-TEST-4138ae32.
func TestApplianceStageReportsDetectedAdapters(t *testing.T) {
	requireApplianceStage(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	detected := nmcliDevices(t, ctx)
	if len(detected) == 0 {
		t.Fatal("NetworkManager reports no device; the stage has nothing real to measure")
	}

	stage := startApplianceStage(t, ctx)
	defer stage.stop()
	stage.login(t, ctx)

	network := stage.get(t, ctx, "/api/v1/network/status")
	if network["measured_by"] == "simulation" {
		t.Fatal("a stage reading is marked simulated")
	}
	if network["measured_by"] == nil || network["measured_by"] == "" {
		t.Fatalf("the reading names no measuring host: %v", network["measured_by"])
	}

	known := map[string]bool{}
	for _, name := range detected {
		known[name] = true
	}
	reported := map[string]map[string]any{}
	for _, raw := range network["interfaces"].([]any) {
		adapter := raw.(map[string]any)
		name, _ := adapter["name"].(string)
		if !known[name] {
			t.Fatalf("Bench Core reports adapter %q, which NetworkManager did not detect in %v", name, detected)
		}
		reported[name] = adapter
	}
	if len(reported) == 0 {
		t.Fatalf("Bench Core reported no adapter while NetworkManager detected %v", detected)
	}

	// The adapter the stage configured carries the address it was given, which
	// is what proves the reading came from the host rather than a catalog.
	stageAdapter, present := reported["eth0"]
	if !present {
		t.Fatalf("the configured stage adapter is missing from %v", reported)
	}
	addresses, _ := stageAdapter["ipv4"].([]any)
	found := false
	for _, value := range addresses {
		if text, _ := value.(string); strings.HasPrefix(text, "192.0.2.2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the configured address is absent from the reported adapter: %v", stageAdapter)
	}
	t.Logf("Bench Core reported %d adapter(s), all detected by NetworkManager: %v", len(reported), detected)
}

// applianceStage is one running appliance: a root helper and an unprivileged
// Bench Core, with an HTTPS client that trusts only this run's certificate.
type applianceStage struct {
	base   string
	client *http.Client
	csrf   string
	user   string
	pass   string
	core   *exec.Cmd
	helper *exec.Cmd
}

func startApplianceStage(t *testing.T, ctx context.Context) *applianceStage {
	t.Helper()
	root, certPath, keyPath, leaf := applianceStageRoot(t)
	socket := filepath.Join(root, "run", "nethelper.sock")

	helperBinary := os.Getenv("NILO_NETHELPER_BINARY")
	if helperBinary == "" {
		helperBinary = "/tmp/nilo-nethelper"
	}
	helper := exec.CommandContext(ctx, helperBinary,
		"--socket", socket, "--state-dir", filepath.Join(root, "state"),
		"--allow-uid", "65534", "--socket-gid", "65534")
	helper.Stderr = os.Stderr
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = helper.Process.Signal(syscall.SIGTERM); _ = helper.Wait() })
	waitSystem(t, ctx, func() bool { _, err := os.Stat(socket); return err == nil })

	binary := os.Getenv("NILO_BINARY")
	if binary == "" {
		binary = "/tmp/nilo"
	}
	asService := func(args ...string) *exec.Cmd {
		command := exec.CommandContext(ctx, binary, args...)
		command.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{Uid: applianceStageServiceUID, Gid: applianceStageServiceUID},
		}
		return command
	}

	user, password := "appliance-stage", "appliance-stage-password-4417"
	bootstrap := asService("admin", "bootstrap", "--data-dir", filepath.Join(root, "data"), "--username", user)
	bootstrap.Stdin = strings.NewReader(password + "\n" + password + "\n")
	if output, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("bootstrap the stage administrator: %v %s", err, output)
	}

	core := asService("serve", "--profile", "appliance",
		"--data-dir", filepath.Join(root, "data"), "--bind", "127.0.0.1:0",
		"--gui-dir", filepath.Join(root, "gui"),
		"--tls-cert", certPath, "--tls-key", keyPath,
		"--nethelper-socket", socket)
	stdout, err := core.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	core.Stderr = os.Stderr
	if err = core.Start(); err != nil {
		t.Fatal(err)
	}
	var ready struct {
		Address string `json:"address"`
		Profile string `json:"profile"`
	}
	if err = json.NewDecoder(stdout).Decode(&ready); err != nil {
		t.Fatalf("the appliance did not report a listening address; the privileged helper at %s is the first thing to check: %v", socket, err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	jar, _ := cookiejar.New(nil)

	return &applianceStage{
		base:   "https://" + ready.Address,
		client: &http.Client{Transport: transport, Jar: jar, Timeout: 35 * time.Second},
		user:   user,
		pass:   password,
		core:   core,
		helper: helper,
	}
}

func (stage *applianceStage) stop() {
	if stage.core != nil {
		_ = stage.core.Process.Signal(syscall.SIGTERM)
		_ = stage.core.Wait()
		stage.core = nil
	}
}

func (stage *applianceStage) do(t *testing.T, ctx context.Context, method, path string, body any) (int, []byte) {
	t.Helper()
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, stage.base+path, payload)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", stage.base)
	if stage.csrf != "" {
		request.Header.Set("X-NILO-CSRF", stage.csrf)
	}
	response, err := stage.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	for _, cookie := range response.Cookies() {
		if cookie.Name == "nilo_csrf" {
			stage.csrf = cookie.Value
		}
	}
	return response.StatusCode, raw
}

func (stage *applianceStage) expect(t *testing.T, ctx context.Context, method, path string, want int) {
	t.Helper()
	code, raw := stage.do(t, ctx, method, path, nil)
	if code != want {
		t.Fatalf("%s %s: %d, want %d; body=%s", method, path, code, want, raw)
	}
}

func (stage *applianceStage) login(t *testing.T, ctx context.Context) {
	t.Helper()
	code, raw := stage.do(t, ctx, "POST", "/api/v1/auth/login",
		map[string]string{"username": stage.user, "password": stage.pass})
	if code != http.StatusOK {
		t.Fatalf("sign in to the stage appliance: %d %s", code, raw)
	}
}

func (stage *applianceStage) get(t *testing.T, ctx context.Context, path string) map[string]any {
	t.Helper()
	code, raw := stage.do(t, ctx, "GET", path, nil)
	if code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, code, raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return decoded
}
