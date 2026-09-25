//go:build linux

package nethelper

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"nilo.local/nilo/internal/network"
)

// WireGuard checks installed capabilities without loading modules, reading
// tunnel secrets, changing connections or running imported configuration.
func (n *NetworkManager) WireGuard(ctx context.Context) (network.WireGuardStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readWireGuard(ctx, readinessProbes{
		manager: n.wireGuardManager,
		kernel: func(ctx context.Context) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			var uname syscall.Utsname
			if err := syscall.Uname(&uname); err != nil {
				return false, err
			}
			var release []byte
			for _, c := range uname.Release {
				if c == 0 {
					break
				}
				release = append(release, byte(c))
			}
			return kernelWireGuard(os.DirFS("/"), string(release))
		},
		tool: installedWG,
	}), nil
}

func (n *NetworkManager) wireGuardManager(ctx context.Context) (bool, string, error) {
	var props map[string]dbus.Variant
	err := n.bus.Object(nm, nmPath).CallWithContext(ctx, "org.freedesktop.DBus.Properties.GetAll", dbus.FlagNoAutoStart, nm).Store(&props)
	if err != nil {
		var busErr dbus.Error
		if errors.As(err, &busErr) && (busErr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || busErr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner") {
			return false, "", nil
		}
		return false, "", err
	}
	version := value[string](props, "Version")
	if version == "" {
		return true, "", errors.New("NetworkManager omitted version")
	}
	return true, version, nil
}

var kernelRelease = regexp.MustCompile(`^[a-zA-Z0-9_+.-]+$`)

// A release number alone is not evidence: distributors can disable WireGuard.
// Containers without host module/config metadata yield an unknown observation.
func kernelWireGuard(root fs.FS, release string) (bool, error) {
	if !kernelRelease.MatchString(release) || release == "." || release == ".." {
		return false, errors.New("invalid kernel release")
	}
	if info, err := fs.Stat(root, "sys/module/wireguard"); err == nil && info.IsDir() {
		return true, nil
	}
	for _, name := range []string{"proc/config.gz", "boot/config-" + release} {
		raw, err := readProbeFile(root, name)
		if err != nil {
			continue
		}
		if strings.HasSuffix(name, ".gz") {
			reader, err := gzip.NewReader(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			raw, err = io.ReadAll(io.LimitReader(reader, maxProbeFile+1))
			reader.Close()
			if err != nil || len(raw) > maxProbeFile {
				continue
			}
		}
		for _, line := range strings.Split(string(raw), "\n") {
			switch strings.TrimSpace(line) {
			case "CONFIG_WIREGUARD=y":
				return true, nil
			case "# CONFIG_WIREGUARD is not set", "CONFIG_WIREGUARD=n":
				return false, nil
			}
		}
	}
	base := "lib/modules/" + release + "/"
	for _, name := range []string{"modules.builtin", "modules.dep"} {
		raw, err := readProbeFile(root, base+name)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			module, _, _ := strings.Cut(line, ":")
			module = strings.TrimSpace(module)
			file := path.Base(module)
			if file != "wireguard.ko" && file != "wireguard.ko.xz" && file != "wireguard.ko.zst" && file != "wireguard.ko.gz" {
				continue
			}
			if !fs.ValidPath(module) {
				continue
			}
			if name == "modules.builtin" {
				return true, nil
			}
			if info, err := fs.Stat(root, base+module); err == nil && info.Mode().IsRegular() {
				return true, nil
			}
		}
	}
	return false, errors.New("running-kernel WireGuard evidence unavailable")
}

// NILO requires wg only; wg-quick is not used because NetworkManager owns the
// connections. Neither PATH nor request parameters select an executable.
func installedWG(ctx context.Context) (network.ToolStatus, error) {
	s := network.ToolStatus{Name: "wg"}
	info, err := os.Lstat("/usr/bin/wg")
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.Installed = true
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 {
		return s, errors.New("wg executable must be a protected root-owned regular file")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/wg", "--version")
	cmd.Dir = "/"
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = 250 * time.Millisecond
	output := &probeOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return s, errors.New("wg version probe failed")
	}
	s.Version, err = parseWGVersion(output.Bytes())
	return s, err
}

type probeOutput struct{ buffer bytes.Buffer }

func (b *probeOutput) Bytes() []byte { return b.buffer.Bytes() }

func (b *probeOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 4096 {
		return 0, errors.New("wg version output exceeds limit")
	}
	return b.buffer.Write(p)
}

var wgVersion = regexp.MustCompile(`^wireguard-tools v([0-9]+(?:\.[0-9]+)*)(?: - https://git\.zx2c4\.com/wireguard-tools/)?$`)

func parseWGVersion(raw []byte) (string, error) {
	match := wgVersion.FindStringSubmatch(strings.TrimSpace(string(raw)))
	if len(match) != 2 {
		return "", errors.New("unrecognized wg version output")
	}
	return match[1], nil
}
