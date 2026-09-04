//go:build linux

package nethelper

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

// The P5 exit criterion on a real kernel: a client on one subnet reaches a
// Modbus server on the device subnet through the appliance with only a route
// to the appliance, the device itself keeps no route back, an unauthorized
// source is refused, an established session ends when the network is removed,
// a table NILO did not create is never replaced, and an unconfirmed change is
// put back by the helper's own timer. The topology is three network
// namespaces inside the disposable container; the server and the clients are
// this test binary re-executed inside them.

const (
	forwardingRoleEnv   = "NILO_FORWARDING_ROLE"
	forwardingListenEnv = "NILO_FORWARDING_LISTEN"
	forwardingTargetEnv = "NILO_FORWARDING_TARGET"
	forwardingSourceEnv = "NILO_FORWARDING_SOURCE"
	// forwardingRegistersEnv selects what a server answers, as hex fields.
	forwardingRegistersEnv = "NILO_FORWARDING_REGISTERS"
)

func ipCommand(t *testing.T, arguments ...string) {
	t.Helper()
	output, err := exec.Command("/usr/sbin/ip", arguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("ip %s: %v %s", strings.Join(arguments, " "), err, output)
	}
}

// forwardingTopology builds the client, appliance and device namespaces.
func forwardingTopology(t *testing.T) {
	t.Helper()
	cleanup := func() {
		_ = exec.Command("/usr/sbin/ip", "netns", "del", "nilo-client").Run()
		_ = exec.Command("/usr/sbin/ip", "netns", "del", "nilo-device").Run()
		_ = exec.Command("/usr/sbin/ip", "link", "del", "lan0").Run()
		_ = exec.Command("/usr/sbin/ip", "link", "del", "dev0").Run()
	}
	cleanup()
	t.Cleanup(cleanup)
	ipCommand(t, "netns", "add", "nilo-client")
	ipCommand(t, "netns", "add", "nilo-device")
	ipCommand(t, "link", "add", "lan0", "type", "veth", "peer", "name", "lan-client")
	ipCommand(t, "link", "set", "lan-client", "netns", "nilo-client")
	ipCommand(t, "link", "add", "dev0", "type", "veth", "peer", "name", "dev-device")
	ipCommand(t, "link", "set", "dev-device", "netns", "nilo-device")
	ipCommand(t, "address", "add", "192.168.10.1/24", "dev", "lan0")
	ipCommand(t, "link", "set", "lan0", "up")
	ipCommand(t, "address", "add", "192.168.0.1/24", "dev", "dev0")
	ipCommand(t, "link", "set", "dev0", "up")
	// The client has the documented route to the device subnet via the
	// appliance and two addresses: one authorized, one not.
	ipCommand(t, "-n", "nilo-client", "link", "set", "lo", "up")
	ipCommand(t, "-n", "nilo-client", "address", "add", "192.168.10.13/24", "dev", "lan-client")
	ipCommand(t, "-n", "nilo-client", "address", "add", "192.168.10.99/24", "dev", "lan-client")
	ipCommand(t, "-n", "nilo-client", "link", "set", "lan-client", "up")
	ipCommand(t, "-n", "nilo-client", "route", "add", "192.168.0.0/24", "via", "192.168.10.1")
	// The device knows only its own subnet: no gateway is changed or added,
	// which is what the source translation is for.
	ipCommand(t, "-n", "nilo-device", "link", "set", "lo", "up")
	ipCommand(t, "-n", "nilo-device", "address", "add", "192.168.0.130/24", "dev", "dev-device")
	ipCommand(t, "-n", "nilo-device", "link", "set", "dev-device", "up")
}

// inNamespace re-executes this binary inside a namespace in one role.
func inNamespace(namespace, role string, environment ...string) *exec.Cmd {
	executable, _ := os.Executable()
	command := exec.Command("/usr/sbin/ip", "netns", "exec", namespace, executable, "-test.run", "^TestForwardingRoleProcess$", "-test.v")
	command.Env = append([]string{"PATH=/usr/sbin:/sbin:/usr/bin:/bin", forwardingRoleEnv + "=" + role}, environment...)
	return command
}

// modbusProbe runs one client in the client namespace from a source address
// and reports whether the read succeeded.
func modbusProbe(t *testing.T, source string) (string, error) {
	t.Helper()
	return modbusProbeFrom(t, "nilo-client", source, "192.168.0.130:502", "0x1234 0x5678")
}

// modbusProbeFrom runs one client in a namespace from a source address and
// reports whether the read returned the expected registers.
func modbusProbeFrom(t *testing.T, namespace, source, target, expected string) (string, error) {
	t.Helper()
	command := inNamespace(namespace, "client", forwardingTargetEnv+"="+target, forwardingSourceEnv+"="+source)
	output, err := command.CombinedOutput()
	text := string(output)
	if err != nil || !strings.Contains(text, "registers "+expected) {
		return text, fmt.Errorf("probe from %s to %s failed: %v", source, target, err)
	}
	return text, nil
}

func linePlanSystem(t *testing.T, sources ...string) network.ForwardingPlan {
	t.Helper()
	prefixes := make([]netip.Prefix, 0, len(sources))
	for _, source := range sources {
		prefixes = append(prefixes, netip.MustParsePrefix(source))
	}
	plan, err := network.PlanForwarding(network.DeviceNetwork{
		ID: "line3", Name: "Line 3", DeviceInterface: "02:00:00:00:00:02",
		ApplianceAddress: netip.MustParsePrefix("192.168.0.1/24"), Scope: network.ScopeHost,
		Target: netip.MustParseAddr("192.168.0.130"), Addressing: network.AddressingOriginal,
		Access: network.AccessLocal, LocalInterface: "02:00:00:00:00:01", LocalSources: prefixes,
		Services: network.ServicePolicy{TCPPorts: []int{502}},
	}, network.ForwardingContext{DeviceLink: "dev0", LocalLink: "lan0"})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestForwardingSystem(t *testing.T) {
	if os.Getenv(forwardingRoleEnv) != "" {
		t.Skip("role process")
	}
	systemTunnelGate(t)
	if _, err := nftBinary(); err != nil {
		t.Fatalf("the harness image must carry nft: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	forwardingTopology(t)

	// A real Modbus TCP responder in the device namespace.
	server := inNamespace("nilo-device", "server", forwardingListenEnv+"=192.168.0.130:502")
	serverOut, err := server.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	server.Stderr = os.Stderr
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Process.Kill(); _ = server.Wait() })
	ready := bufio.NewScanner(serverOut)
	for ready.Scan() {
		if strings.Contains(ready.Text(), "listening") {
			break
		}
	}
	go func() {
		for ready.Scan() {
		}
	}()

	driver, err := NewNetworkManager()
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	dir := t.TempDir()
	_ = os.Chmod(dir, 0700)
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(driver, store, time.Now)
	if err != nil {
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
	// The container may start with forwarding on; the appliance under test
	// starts with it off, which is the state a fresh installation has.
	if err := driver.SetForwardingSwitch(ctx, "0"); err != nil {
		t.Fatal(err)
	}
	const baseline = "0"

	// Nothing is reachable before NILO grants it, from either address.
	if _, err := modbusProbe(t, "192.168.10.13"); err == nil {
		t.Fatal("the device was reachable before any grant")
	}

	// A table somebody else made is refused, untouched.
	if _, err := runNft(ctx, "", "add", "table", "inet", "nilo"); err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyForwarding(ctx, linePlanSystem(t, "192.168.10.0/28")); !errors.Is(err, ErrForwardingForeign) {
		t.Fatalf("foreign table: err = %v", err)
	}
	if state, _ := driver.ForwardingTable(ctx); state != TableForeign {
		t.Fatalf("the foreign table was touched: %s", state)
	}
	if _, err := runNft(ctx, "", "delete", "table", "inet", "nilo"); err != nil {
		t.Fatal(err)
	}

	// An unconfirmed enable is put back by the timer: the change is armed,
	// applied, the client reaches the device, then everything goes.
	record, err := engine.Prepare(ctx, "enable", network.Change{Action: network.ActionDeviceNetworkEnable, Target: "line3"}, network.ConfirmationWindow)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if record.Checkpoint != "" || record.Forwarding == nil {
		t.Fatalf("record = %+v", record)
	}
	if err := engine.Apply(ctx, "enable"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := engine.ApplyForwarding(ctx, linePlanSystem(t, "192.168.10.0/28")); err != nil {
		t.Fatalf("apply forwarding: %v", err)
	}
	if state, _ := driver.ForwardingTable(ctx); state != TableOwned {
		t.Fatalf("table state = %s", state)
	}
	if output, err := modbusProbe(t, "192.168.10.13"); err != nil {
		t.Fatalf("authorized client: %v\n%s", err, output)
	}
	if _, err := modbusProbe(t, "192.168.10.99"); err == nil {
		t.Fatal("an unauthorized source reached the device")
	}
	if err := engine.Restore(ctx, "enable"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if state, _ := driver.ForwardingTable(ctx); state != TableAbsent {
		t.Fatalf("table after restoration = %s", state)
	}
	if after, _ := driver.ForwardingSwitch(ctx); after != baseline {
		t.Fatalf("forwarding switch = %s, was %s", after, baseline)
	}
	if _, err := modbusProbe(t, "192.168.10.13"); err == nil {
		t.Fatal("the device stayed reachable after the restoration")
	}

	// A confirmed enable stays; an established session ends with the
	// network's removal; a restarted helper renders what its file says.
	if _, err := engine.Prepare(ctx, "again", network.Change{Action: network.ActionDeviceNetworkEnable, Target: "line3"}, network.ConfirmationWindow); err != nil {
		t.Fatal(err)
	}
	if err := engine.Apply(ctx, "again"); err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyForwarding(ctx, linePlanSystem(t, "192.168.10.0/28")); err != nil {
		t.Fatal(err)
	}
	if err := engine.Confirm(ctx, "again"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	held := inNamespace("nilo-client", "hold", forwardingTargetEnv+"=192.168.0.130:502", forwardingSourceEnv+"=192.168.10.13")
	heldIn, err := held.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	heldOut, err := held.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	held.Stderr = os.Stderr
	if err := held.Start(); err != nil {
		t.Fatal(err)
	}
	heldLines := bufio.NewScanner(heldOut)
	// The role process is a test binary too: its own run banner precedes the
	// lines the role prints, so the reader waits for the line it wants.
	waitForLine := func(want string) {
		t.Helper()
		for heldLines.Scan() {
			if strings.Contains(heldLines.Text(), want) {
				return
			}
		}
		t.Fatalf("held session never printed %q", want)
	}
	waitForLine("first registers 0x1234 0x5678")
	// The owned table vanishing is noticed and repaired by the helper.
	if _, err := runNft(ctx, "", "delete", "table", "inet", "nilo"); err != nil {
		t.Fatal(err)
	}
	if err := engine.VerifyForwarding(ctx); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if state, _ := driver.ForwardingTable(ctx); state != TableOwned {
		t.Fatalf("table after verify = %s", state)
	}
	restarted, err := NewEngine(driver, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := restarted.AppliedForwarding(ctx); err != nil || len(applied) != 1 {
		t.Fatalf("inventory after restart = %+v %v", applied, err)
	}
	if err := restarted.RemoveForwarding(ctx, "line3"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := fmt.Fprintln(heldIn, "again"); err != nil {
		t.Fatal(err)
	}
	waitForLine("second")
	if !strings.Contains(heldLines.Text(), "second failed") {
		t.Fatalf("the established session survived the removal: %q", heldLines.Text())
	}
	_ = held.Wait()
	if state, _ := driver.ForwardingTable(ctx); state != TableAbsent {
		t.Fatalf("table after removal = %s", state)
	}
	if after, _ := driver.ForwardingSwitch(ctx); after != baseline {
		t.Fatalf("forwarding switch = %s, was %s", after, baseline)
	}
	// The device's own routes were never touched.
	routes, err := exec.Command("/usr/sbin/ip", "-n", "nilo-device", "route").CombinedOutput()
	if err != nil || strings.Contains(string(routes), "default") || strings.Contains(string(routes), "192.168.10") {
		t.Fatalf("device routes changed: %s %v", routes, err)
	}
}

// TestForwardingRoleProcess is the server and the clients, selected by
// environment; it is never a test of its own.
func TestForwardingRoleProcess(t *testing.T) {
	role := os.Getenv(forwardingRoleEnv)
	switch role {
	case "server":
		runModbusServer(t, os.Getenv(forwardingListenEnv))
	case "client":
		conn := dialFrom(t, os.Getenv(forwardingSourceEnv), os.Getenv(forwardingTargetEnv))
		defer conn.Close()
		first, second, err := modbusRead(conn)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		fmt.Printf("registers 0x%04x 0x%04x\n", first, second)
	case "hold":
		conn := dialFrom(t, os.Getenv(forwardingSourceEnv), os.Getenv(forwardingTargetEnv))
		defer conn.Close()
		first, second, err := modbusRead(conn)
		if err != nil {
			t.Fatalf("first read failed: %v", err)
		}
		fmt.Printf("first registers 0x%04x 0x%04x\n", first, second)
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		if _, _, err := modbusRead(conn); err != nil {
			fmt.Printf("second failed: %v\n", err)
			return
		}
		fmt.Println("second succeeded")
	case "site", "bulk-server", "bulk-client":
		runVPNRole(t, role)
	default:
		t.Skip("not a role process")
	}
}

func dialFrom(t *testing.T, source, target string) net.Conn {
	t.Helper()
	dialer := net.Dialer{Timeout: 3 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(source)}}
	conn, err := dialer.Dial("tcp", target)
	if err != nil {
		t.Fatalf("dial from %s: %v", source, err)
	}
	return conn
}

// modbusRead issues one Read Holding Registers request for two registers.
func modbusRead(conn net.Conn) (uint16, uint16, error) {
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	request := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01, 0x03, 0x00, 0x00, 0x00, 0x02}
	if _, err := conn.Write(request); err != nil {
		return 0, 0, err
	}
	response := make([]byte, 13)
	if _, err := io.ReadFull(conn, response); err != nil {
		return 0, 0, err
	}
	if response[7] != 0x03 || response[8] != 4 {
		return 0, 0, fmt.Errorf("unexpected response % x", response)
	}
	return binary.BigEndian.Uint16(response[9:11]), binary.BigEndian.Uint16(response[11:13]), nil
}

// runModbusServer answers function 3 with two fixed registers until killed.
func runModbusServer(t *testing.T, listen string) {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	// The registers answered are the server's identity: two sites with the
	// same device address are told apart by what they return.
	registers := []byte{0x12, 0x34, 0x56, 0x78}
	if text := os.Getenv(forwardingRegistersEnv); text != "" {
		registers = registers[:0]
		for _, field := range strings.Split(text, ",") {
			var value uint16
			_, _ = fmt.Sscanf(strings.TrimSpace(field), "%x", &value)
			registers = binary.BigEndian.AppendUint16(registers, value)
		}
	}
	fmt.Println("listening on", listen)
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			defer conn.Close()
			for {
				header := make([]byte, 7)
				if _, err := io.ReadFull(conn, header); err != nil {
					return
				}
				length := int(binary.BigEndian.Uint16(header[4:6]))
				pdu := make([]byte, length-1)
				if _, err := io.ReadFull(conn, pdu); err != nil {
					return
				}
				if len(pdu) < 5 || pdu[0] != 0x03 {
					_, _ = conn.Write(append(header[:5], 0x03, header[6], 0x83, 0x01))
					continue
				}
				response := append([]byte{header[0], header[1], 0, 0, 0, 7, header[6], 0x03, 4}, registers...)
				if _, err := conn.Write(response); err != nil {
					return
				}
			}
		}(conn)
	}
}
