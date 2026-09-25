//go:build linux

package nethelper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// The nftables adapter. The helper owns exactly one table, `inet nilo`, and
// tells it apart from anybody else's by the comment it writes on it. The
// forwarding switch is the kernel's own file.

const (
	forwardingSwitchPath = "/proc/sys/net/ipv4/ip_forward"
	nftTimeout           = 20 * time.Second
)

// nftBinary is the pinned location of nft; PATH is never consulted.
func nftBinary() (string, error) {
	for _, candidate := range []string{"/usr/sbin/nft", "/sbin/nft"} {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: nft is not installed", ErrForwardingUnsupported)
}

func runNft(ctx context.Context, stdin string, arguments ...string) (string, error) {
	binary, err := nftBinary()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, nftTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, binary, arguments...)
	command.Env = []string{"PATH=/usr/sbin:/sbin:/usr/bin:/bin", "LC_ALL=C"}
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return stdout.String(), fmt.Errorf("nft %s: %s", strings.Join(arguments, " "), strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// ForwardingTable reports whether the owned table exists and who made it.
func (n *NetworkManager) ForwardingTable(ctx context.Context) (TableState, error) {
	output, err := runNft(ctx, "", "list", "table", "inet", "nilo")
	if err != nil {
		if strings.Contains(err.Error(), "No such file or directory") {
			return TableAbsent, nil
		}
		return "", err
	}
	if strings.Contains(output, ownedTableComment) {
		return TableOwned, nil
	}
	return TableForeign, nil
}

// InstallForwarding replaces the owned table in one transaction.
func (n *NetworkManager) InstallForwarding(ctx context.Context, inventory ForwardingInventory) error {
	_, err := runNft(ctx, renderNftables(inventory), "-f", "-")
	return err
}

// ForwardingSwitch reads the kernel's IPv4 forwarding switch.
func (n *NetworkManager) ForwardingSwitch(context.Context) (string, error) {
	data, err := os.ReadFile(forwardingSwitchPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SetForwardingSwitch writes the kernel's IPv4 forwarding switch.
func (n *NetworkManager) SetForwardingSwitch(_ context.Context, value string) error {
	if value != "0" && value != "1" {
		return errors.New("the forwarding switch is 0 or 1")
	}
	return os.WriteFile(forwardingSwitchPath, []byte(value+"\n"), 0o644)
}

var _ ForwardingDriver = (*NetworkManager)(nil)
