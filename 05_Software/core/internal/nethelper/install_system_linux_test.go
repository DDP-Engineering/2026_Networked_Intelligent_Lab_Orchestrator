//go:build linux

package nethelper

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"nilo.local/nilo/internal/network"
)

func packageInventory(t *testing.T) map[string]string {
	t.Helper()
	raw, err := exec.Command("/usr/bin/dpkg-query", "-W", "-f=${Package}\t${Version}\t${Status}\n").Output()
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		name, state, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatal("invalid package inventory")
		}
		result[name] = state
	}
	return result
}

// Real APT and dpkg run only inside the disposable, still-online namespace.
// The regular suite never installs or removes host packages.
func TestWireGuardInstallSystem(t *testing.T) {
	if os.Getenv("NILO_INSTALL_SYSTEM") != "1" {
		t.Skip("explicit disposable installation fixture only")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil || os.Geteuid() != 0 {
		t.Fatal("root disposable container required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	tool, err := installedWG(ctx)
	if err != nil || tool.Installed {
		t.Fatalf("fixture must start without wg: %+v %v", tool, err)
	}
	n, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := NewAPTInstaller(dir, "/tmp/nilo-nethelper", n)
	if err != nil {
		t.Fatal(err)
	}
	before := packageInventory(t)
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(a.config)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("held package lock", func(t *testing.T) {
		lock, err := os.OpenFile("/var/lib/dpkg/lock-frontend", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		if err = unix.FcntlFlock(lock.Fd(), unix.F_SETLK, &unix.Flock_t{Type: unix.F_WRLCK, Whence: 0}); err != nil {
			t.Fatal(err)
		}
		_, err = a.apt(ctx, "install", "wireguard-tools")
		if err == nil || !strings.HasPrefix(err.Error(), "package_lock_busy:") {
			t.Fatalf("lock result: %v", err)
		}
	})
	t.Run("offline repository", func(t *testing.T) {
		source := filepath.Join(dir, "offline.list")
		if err := os.WriteFile(source, []byte("deb http://private-user:private-token@127.0.0.1:9/debian trixie main\n"), 0600); err != nil {
			t.Fatal(err)
		}
		data := string(config) + "\nDir::Etc::sourcelist \"" + source + "\";\nDir::Etc::sourceparts \"" + filepath.Join(dir, "apt.conf.d") + "\";\n"
		if err := os.WriteFile(a.config, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.WriteFile(a.config, config, 0600); err != nil {
				t.Error(err)
			}
		}()
		err := a.Run(ctx, func(string) error { return nil })
		if err == nil || !strings.HasPrefix(err.Error(), "repository_unavailable:") || strings.Contains(err.Error(), "private-token") {
			t.Fatalf("offline result: %v", err)
		}
	})
	if !reflect.DeepEqual(before, packageInventory(t)) {
		t.Fatal("failed attempts changed package inventory")
	}

	// Use the real authenticated socket and core operation manager for the
	// successful path, including acknowledgement and asynchronous polling.
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewEngine(n, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.ConfigureInstaller(a, store); err != nil {
		t.Fatal(err)
	}
	service, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); e.RunInstaller(service) }()
	defer func() { stop(); <-done }()
	socket := filepath.Join(dir, "helper.sock")
	l, err := ListenUnix(socket, os.Getgid())
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- ServeUnix(service, l, e, uint32(os.Getuid())) }()
	defer func() { stop(); l.Close(); <-served }()
	client := NewClient(socket, "installation-container")
	manager, err := network.NewOperationManager(network.OperationManagerOptions{Changer: client, Observer: client, Now: time.Now, NewID: func() string { return "system-install" }})
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.WireGuard(ctx)
	if err != nil || !status.CanInstall {
		t.Fatalf("missing installation capability: %+v %v", status, err)
	}
	p, err := manager.Preview(ctx, "administrator", network.ActionWireGuardInstall, "wireguard")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, packageInventory(t)) {
		t.Fatal("preview installed packages")
	}
	op, err := manager.Acknowledge(ctx, "administrator", p.ID, p.Revision)
	if err != nil || op.State != network.StateApplying {
		t.Fatalf("not asynchronous: %+v %v", op, err)
	}
	phases := map[string]bool{op.Progress: true}
	for op.State == network.StateApplying && ctx.Err() == nil {
		time.Sleep(100 * time.Millisecond)
		manager.ExpireDue(ctx)
		op, err = manager.Get(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		phases[op.Progress] = true
	}
	if op.State != network.StateConfirmed || len(phases) < 2 {
		t.Fatalf("installation did not complete: %+v phases=%v", op, phases)
	}
	fresh, err := client.WireGuard(ctx)
	if err != nil || fresh.CanInstall || len(fresh.Tools) != 1 || !fresh.Tools[0].Installed || fresh.Tools[0].Version == "" {
		t.Fatalf("fresh readiness: %+v %v", fresh, err)
	}
	after := packageInventory(t)
	if _, ok := after["wireguard-tools"]; !ok {
		t.Fatal("WireGuard package missing")
	}
	delete(after, "wireguard-tools")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("installation changed unrelated packages, including kernel")
	}
	bootAfter, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || string(bootAfter) != string(boot) {
		t.Fatal("boot changed")
	}

	// No update, download or dpkg invocation is necessary on an installed host.
	idempotentBefore := packageInventory(t)
	var repeated []string
	if err := a.Run(ctx, func(phase string) error { repeated = append(repeated, phase); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(repeated, []string{"verifying"}) || !reflect.DeepEqual(idempotentBefore, packageInventory(t)) {
		t.Fatal("repeat changed packages")
	}
	t.Logf("only wireguard-tools added; progress=%v; fresh wg version=%s; second run verified without package changes", phases, fresh.Tools[0].Version)
}
