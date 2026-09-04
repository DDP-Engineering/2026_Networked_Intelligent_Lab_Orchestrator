//go:build linux

package nethelper

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// The P6 exit criterion on a real kernel: a disposable WireGuard hub, a
// remote client attached to it, and two independent sites whose devices share
// the address 192.168.0.130. Site A is this appliance (its tunnel is the
// NILO-managed NetworkManager connection); site B is a second gateway in its
// own namespace running the same helper engine. Both devices are reached
// through distinct exposed addresses, neither through the internal address or
// from an unauthorized source, a large transfer crosses the tunnel, and a
// waiting plan closes the grants while the tunnel is up.

const (
	forwardingStateEnv  = "NILO_FORWARDING_STATE"
	forwardingTunnelEnv = "NILO_FORWARDING_TUNNEL"
	forwardingLinkEnv   = "NILO_FORWARDING_LINK"
	bulkBytes           = 8 << 20
)

func namespaceCommand(t *testing.T, namespace string, arguments ...string) string {
	t.Helper()
	command := exec.Command("/usr/sbin/ip", append([]string{"netns", "exec", namespace}, arguments...)...)
	command.Env = []string{"PATH=/usr/sbin:/sbin:/usr/bin:/bin"}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("in %s: %s: %v %s", namespace, strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

// writeDisposableKey writes one per-test private key root-only for wg.
func writeDisposableKey(t *testing.T, directory, name, key string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// vpnTopology builds hub, site B and the two device namespaces around this
// appliance, and returns the public keys the appliance needs.
func vpnTopology(t *testing.T, keys string) (hubPublic, sitePublicB string) {
	t.Helper()
	namespaces := []string{"nilo-hub", "nilo-siteb", "nilo-devicea", "nilo-deviceb"}
	cleanup := func() {
		for _, namespace := range namespaces {
			_ = exec.Command("/usr/sbin/ip", "netns", "del", namespace).Run()
		}
		_ = exec.Command("/usr/sbin/ip", "link", "del", "hub0").Run()
		_ = exec.Command("/usr/sbin/ip", "link", "del", "dev0").Run()
	}
	cleanup()
	t.Cleanup(cleanup)
	for _, namespace := range namespaces {
		ipCommand(t, "netns", "add", namespace)
		ipCommand(t, "-n", namespace, "link", "set", "lo", "up")
	}
	// Appliance (site A) <-> hub.
	ipCommand(t, "link", "add", "hub0", "type", "veth", "peer", "name", "hub-a")
	ipCommand(t, "link", "set", "hub-a", "netns", "nilo-hub")
	ipCommand(t, "address", "add", "172.16.0.2/24", "dev", "hub0")
	ipCommand(t, "link", "set", "hub0", "up")
	ipCommand(t, "-n", "nilo-hub", "address", "add", "172.16.0.1/24", "dev", "hub-a")
	ipCommand(t, "-n", "nilo-hub", "link", "set", "hub-a", "up")
	// Site B <-> hub.
	ipCommand(t, "-n", "nilo-hub", "link", "add", "hub-b", "type", "veth", "peer", "name", "wan0")
	ipCommand(t, "-n", "nilo-hub", "link", "set", "wan0", "netns", "nilo-siteb")
	ipCommand(t, "-n", "nilo-hub", "address", "add", "172.16.2.1/24", "dev", "hub-b")
	ipCommand(t, "-n", "nilo-hub", "link", "set", "hub-b", "up")
	ipCommand(t, "-n", "nilo-siteb", "address", "add", "172.16.2.2/24", "dev", "wan0")
	ipCommand(t, "-n", "nilo-siteb", "link", "set", "wan0", "up")
	// Device A behind this appliance, device B behind site B, same address.
	ipCommand(t, "link", "add", "dev0", "type", "veth", "peer", "name", "dev-a")
	ipCommand(t, "link", "set", "dev-a", "netns", "nilo-devicea")
	ipCommand(t, "address", "add", "192.168.0.1/24", "dev", "dev0")
	ipCommand(t, "link", "set", "dev0", "up")
	ipCommand(t, "-n", "nilo-devicea", "address", "add", "192.168.0.130/24", "dev", "dev-a")
	ipCommand(t, "-n", "nilo-devicea", "link", "set", "dev-a", "up")
	ipCommand(t, "-n", "nilo-siteb", "link", "add", "devb0", "type", "veth", "peer", "name", "dev-b")
	ipCommand(t, "-n", "nilo-siteb", "link", "set", "dev-b", "netns", "nilo-deviceb")
	ipCommand(t, "-n", "nilo-siteb", "address", "add", "192.168.0.1/24", "dev", "devb0")
	ipCommand(t, "-n", "nilo-siteb", "link", "set", "devb0", "up")
	ipCommand(t, "-n", "nilo-deviceb", "address", "add", "192.168.0.130/24", "dev", "dev-b")
	ipCommand(t, "-n", "nilo-deviceb", "link", "set", "dev-b", "up")

	// Disposable keys for the hub and site B; the appliance's own pair is
	// staged through the helper like any import.
	hubPrivate, hubPublicKey := disposableKeyPair(t)
	sitePrivate, sitePublicKeyB := disposableKeyPair(t)
	hubKey := writeDisposableKey(t, keys, "hub.key", hubPrivate)
	siteKey := writeDisposableKey(t, keys, "siteb.key", sitePrivate)
	ipCommand(t, "-n", "nilo-hub", "link", "add", "wg-hub", "type", "wireguard")
	namespaceCommand(t, "nilo-hub", "/usr/bin/wg", "set", "wg-hub", "listen-port", "51820", "private-key", hubKey)
	ipCommand(t, "-n", "nilo-hub", "address", "add", "10.8.0.1/24", "dev", "wg-hub")
	ipCommand(t, "-n", "nilo-hub", "address", "add", "10.8.0.99/24", "dev", "wg-hub")
	ipCommand(t, "-n", "nilo-hub", "link", "set", "wg-hub", "up")
	ipCommand(t, "-n", "nilo-siteb", "link", "add", "wg-siteb", "type", "wireguard")
	namespaceCommand(t, "nilo-siteb", "/usr/bin/wg", "set", "wg-siteb", "private-key", siteKey,
		"peer", hubPublicKey, "endpoint", "172.16.2.1:51820", "allowed-ips", "10.8.0.0/24", "persistent-keepalive", "1")
	ipCommand(t, "-n", "nilo-siteb", "address", "add", "10.8.0.3/32", "dev", "wg-siteb")
	ipCommand(t, "-n", "nilo-siteb", "link", "set", "wg-siteb", "up")
	ipCommand(t, "-n", "nilo-siteb", "route", "add", "10.8.0.0/24", "dev", "wg-siteb")
	return hubPublicKey, sitePublicKeyB
}

// hubPeers tells the hub about both sites: the appliance's exposed prefix and
// its internal subnet route to it (the unselected path under test), site B's
// exposed prefix to site B.
func hubPeers(t *testing.T, appliancePublic, sitePublicB string) {
	t.Helper()
	namespaceCommand(t, "nilo-hub", "/usr/bin/wg", "set", "wg-hub",
		"peer", appliancePublic, "allowed-ips", "10.8.0.2/32,10.7.1.0/24,192.168.0.0/24",
		"peer", sitePublicB, "allowed-ips", "10.8.0.3/32,10.7.2.0/24")
	for _, prefix := range []string{"10.7.1.0/24", "10.7.2.0/24", "192.168.0.0/24"} {
		ipCommand(t, "-n", "nilo-hub", "route", "add", prefix, "dev", "wg-hub")
	}
}

func startRole(t *testing.T, namespace, role string, readyText string, environment ...string) *exec.Cmd {
	t.Helper()
	command := inNamespace(namespace, role, environment...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = command.Process.Kill(); _ = command.Wait() })
	lines := bufio.NewScanner(stdout)
	for lines.Scan() {
		if strings.Contains(lines.Text(), readyText) {
			go func() {
				for lines.Scan() {
				}
			}()
			return command
		}
	}
	t.Fatalf("%s in %s never reported %q", role, namespace, readyText)
	return nil
}

func sitePlan(t *testing.T, id, exposed, deviceLink, tunnelDevice string, sources ...string) network.ForwardingPlan {
	t.Helper()
	prefixes := make([]netip.Prefix, 0, len(sources))
	for _, source := range sources {
		prefixes = append(prefixes, netip.MustParsePrefix(source))
	}
	plan, err := network.PlanForwarding(network.DeviceNetwork{
		ID: id, Name: id, DeviceInterface: "02:00:00:00:00:02",
		ApplianceAddress: netip.MustParsePrefix("192.168.0.1/24"), Scope: network.ScopeHost,
		Target: netip.MustParseAddr("192.168.0.130"), Addressing: network.AddressingTranslated,
		Exposed: netip.MustParsePrefix(exposed), Access: network.AccessWireGuard, VPNProfileID: "hub",
		VPNSources: prefixes, Services: network.ServicePolicy{TCPPorts: []int{502, 5020}},
	}, network.ForwardingContext{DeviceLink: deviceLink, TunnelDevice: tunnelDevice})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestForwardingVPNSystem(t *testing.T) {
	if os.Getenv(forwardingRoleEnv) != "" {
		t.Skip("role process")
	}
	systemTunnelGate(t)
	if _, err := nftBinary(); err != nil {
		t.Fatalf("the harness image must carry nft: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	keys := t.TempDir()
	_ = os.Chmod(keys, 0o700)
	hubPublic, sitePublicB := vpnTopology(t, keys)

	// Two Modbus servers that answer differently, and a bulk server behind A.
	startRole(t, "nilo-devicea", "server", "listening", forwardingListenEnv+"=192.168.0.130:502", forwardingRegistersEnv+"=1234,5678")
	startRole(t, "nilo-deviceb", "server", "listening", forwardingListenEnv+"=192.168.0.130:502", forwardingRegistersEnv+"=bbbb,cccc")
	startRole(t, "nilo-devicea", "bulk-server", "listening", forwardingListenEnv+"=192.168.0.130:5020")

	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	dir := t.TempDir()
	_ = os.Chmod(dir, 0o700)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	keystore, _ := newKeystore(t)
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ConfigureKeystore(keystore); err != nil {
		t.Fatal(err)
	}
	switchBefore, err := driver.ForwardingSwitch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = runNft(context.Background(), "", "delete", "table", "inet", "nilo")
		_ = driver.SetForwardingSwitch(context.Background(), switchBefore)
	})
	if err := driver.SetForwardingSwitch(ctx, "0"); err != nil {
		t.Fatal(err)
	}

	// The appliance's tunnel to the hub is a NILO-managed connection made
	// from staged keys, exactly as an import would make it.
	appliancePrivate, appliancePublic := disposableKeyPair(t)
	hubPeers(t, appliancePublic, sitePublicB)
	staged := dispatch(ctx, engine, Request{Op: "wireguard.secrets.stage", Secret: &SecretPayload{PrivateKey: appliancePrivate, PresharedKeys: []string{""}}})
	if staged.Error != "" {
		t.Fatal(staged.Error)
	}
	profile := network.Profile{Name: "hub", Addresses: []string{"10.8.0.2/32"},
		Peers: []network.ProfilePeer{{PublicKey: hubPublic, AllowedIPs: []string{"10.8.0.0/24"}, Endpoint: "172.16.0.1:51820", PersistentKeepalive: 1}}}
	created := dispatch(ctx, engine, Request{Op: "wireguard.profile.create", Handle: staged.Handle, ProfileID: "profile-hub",
		Change: network.Change{Action: network.ActionProfileImport, Target: "hub", Profile: &profile}})
	if created.Error != "" {
		t.Fatalf("create the hub profile: %s", created.Error)
	}
	tunnelRef := created.ConnectionRef
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		_ = driver.DeactivateTunnel(cleanup, tunnelRef)
		_ = driver.DeleteTunnel(cleanup, tunnelRef)
	})
	tunnelChange := func(id string, action network.Action) {
		t.Helper()
		if _, err := engine.Prepare(ctx, id, network.Change{Action: action, Target: "hub", ConnectionID: tunnelRef}, network.ConfirmationWindow); err != nil {
			t.Fatalf("prepare %s: %v", id, err)
		}
		if err := engine.Apply(ctx, id); err != nil {
			t.Fatalf("apply %s: %v", id, err)
		}
		if err := engine.Confirm(ctx, id); err != nil {
			t.Fatalf("confirm %s: %v", id, err)
		}
	}
	tunnelChange("up-hub", network.ActionVPNActivate)
	// The hub reaches the appliance through the tunnel once the handshake is
	// done; site B's keepalive brings its own peer up.
	deadline := time.Now().Add(30 * time.Second)
	for {
		hubToA := exec.Command("/usr/sbin/ip", "netns", "exec", "nilo-hub", "/usr/bin/ping", "-c", "1", "-W", "1", "10.8.0.2").Run()
		hubToB := exec.Command("/usr/sbin/ip", "netns", "exec", "nilo-hub", "/usr/bin/ping", "-c", "1", "-W", "1", "10.8.0.3").Run()
		if hubToA == nil && hubToB == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tunnels never came up: A=%v B=%v\n%s", hubToA, hubToB, namespaceCommand(t, "nilo-hub", "/usr/bin/wg"))
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Site B exposes its device at 10.7.2.130 through its own engine.
	siteState := t.TempDir()
	_ = os.Chmod(siteState, 0o700)
	startRole(t, "nilo-siteb", "site", "site applied", forwardingStateEnv+"="+siteState, forwardingTunnelEnv+"=wg-siteb", forwardingLinkEnv+"=devb0")

	// Site A exposes its device at 10.7.1.130 through this appliance, as one
	// confirmed device-network change.
	planA := sitePlan(t, "site-a", "10.7.1.130/32", "dev0", niloTunnelInterface, "10.8.0.1/32")
	if _, err := engine.Prepare(ctx, "enable-a", network.Change{Action: network.ActionDeviceNetworkEnable, Target: "site-a"}, network.ConfirmationWindow); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := engine.Apply(ctx, "enable-a"); err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyForwarding(ctx, planA); err != nil {
		t.Fatalf("apply forwarding: %v", err)
	}
	if err := engine.Confirm(ctx, "enable-a"); err != nil {
		t.Fatal(err)
	}

	// Both identical device addresses, each through its own exposed address.
	if output, err := modbusProbeFrom(t, "nilo-hub", "10.8.0.1", "10.7.1.130:502", "0x1234 0x5678"); err != nil {
		t.Fatalf("site A through the tunnel: %v\n%s\n%s", err, output, namespaceCommand(t, "nilo-hub", "/usr/bin/wg"))
	}
	if output, err := modbusProbeFrom(t, "nilo-hub", "10.8.0.1", "10.7.2.130:502", "0xbbbb 0xcccc"); err != nil {
		t.Fatalf("site B through the tunnel: %v\n%s", err, output)
	}
	// Not through the internal address the hub could route to A, and not
	// from a source nobody authorized.
	if _, err := modbusProbeFrom(t, "nilo-hub", "10.8.0.1", "192.168.0.130:502", "0x1234 0x5678"); err == nil {
		t.Fatal("the internal address was reachable through the tunnel")
	}
	if _, err := modbusProbeFrom(t, "nilo-hub", "10.8.0.99", "10.7.1.130:502", "0x1234 0x5678"); err == nil {
		t.Fatal("an unauthorized source reached site A")
	}
	// A large transfer crosses the tunnel intact.
	bulk := inNamespace("nilo-hub", "bulk-client", forwardingTargetEnv+"=10.7.1.130:5020", forwardingSourceEnv+"=10.8.0.1")
	if output, err := bulk.CombinedOutput(); err != nil || !strings.Contains(string(output), fmt.Sprintf("received %d bytes", bulkBytes)) {
		t.Fatalf("bulk transfer: %v\n%s", err, output)
	}
	// Tunnel loss as the planner sees it: the waiting plan keeps the guards
	// and grants nothing, even while the kernel tunnel is still up; the full
	// plan opens it again.
	waiting := sitePlan(t, "site-a", "10.7.1.130/32", "dev0", "", "10.8.0.1/32")
	if !waiting.Waiting {
		t.Fatal("a plan without its tunnel must wait")
	}
	if err := engine.ApplyForwarding(ctx, waiting); err != nil {
		t.Fatal(err)
	}
	if _, err := modbusProbeFrom(t, "nilo-hub", "10.8.0.1", "10.7.1.130:502", "0x1234 0x5678"); err == nil {
		t.Fatal("a waiting device network forwarded traffic")
	}
	if err := engine.ApplyForwarding(ctx, planA); err != nil {
		t.Fatal(err)
	}
	if _, err := modbusProbeFrom(t, "nilo-hub", "10.8.0.1", "10.7.1.130:502", "0x1234 0x5678"); err != nil {
		t.Fatalf("site A after the tunnel returned: %v", err)
	}
	// Real tunnel loss: the site is unreachable and site B is unaffected.
	tunnelChange("down-hub", network.ActionVPNDeactivate)
	if _, err := modbusProbeFrom(t, "nilo-hub", "10.8.0.1", "10.7.1.130:502", "0x1234 0x5678"); err == nil {
		t.Fatal("site A stayed reachable without its tunnel")
	}
	if _, err := modbusProbeFrom(t, "nilo-hub", "10.8.0.1", "10.7.2.130:502", "0xbbbb 0xcccc"); err != nil {
		t.Fatalf("site B was affected by site A's tunnel: %v", err)
	}
	if err := engine.RemoveForwarding(ctx, "site-a"); err != nil {
		t.Fatal(err)
	}
	if after, _ := driver.ForwardingSwitch(ctx); after != "0" {
		t.Fatalf("forwarding switch = %s after removal", after)
	}
}

// runVPNRole is the site B gateway and the bulk transfer endpoints.
func runVPNRole(t *testing.T, role string) {
	switch role {
	case "site":
		state := os.Getenv(forwardingStateEnv)
		store, err := NewFileStore(state)
		if err != nil {
			t.Fatalf("site store: %v", err)
		}
		// Site B has no NetworkManager of its own; only the forwarding half of
		// the driver is exercised in its namespace.
		engine, err := NewEngine(&NetworkManager{}, store, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		plan := sitePlan(t, "site-b", "10.7.2.130/32", os.Getenv(forwardingLinkEnv), os.Getenv(forwardingTunnelEnv), "10.8.0.1/32")
		if err := engine.ApplyForwarding(context.Background(), plan); err != nil {
			t.Fatalf("site apply: %v", err)
		}
		fmt.Println("site applied")
		_, _ = io.ReadAll(os.Stdin)
		_ = engine.RemoveForwarding(context.Background(), "site-b")
	case "bulk-server":
		listener, err := net.Listen("tcp", os.Getenv(forwardingListenEnv))
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		fmt.Println("listening")
		payload := make([]byte, bulkBytes)
		_, _ = rand.Read(payload)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
				_, _ = conn.Write(payload)
			}(conn)
		}
	case "bulk-client":
		conn := dialFrom(t, os.Getenv(forwardingSourceEnv), os.Getenv(forwardingTargetEnv))
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
		received, err := io.Copy(io.Discard, conn)
		if err != nil {
			t.Fatalf("received %d bytes then failed: %v", received, err)
		}
		fmt.Printf("received %d bytes\n", received)
	}
}
