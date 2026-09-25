package nethelper

import (
	"os"
	"path/filepath"
	"strings"
)

// busDeviceRoot is the kernel directory carrying one entry per network
// interface. It is a variable only so a test can point the probe at a temporary
// tree; nothing in the helper reassigns it.
var busDeviceRoot = "/sys/class/net"

// physicalAdapter reports whether recorded device facts describe a physical
// network adapter.
//
// Both signals are required. NetworkManager reports built-in Ethernet and a
// container veth through the same wired device API, so the device type does not
// separate them; a veth carries a current hardware address but no permanent
// one, and a bridge carries neither a permanent address nor a bus device.
func physicalAdapter(permanentHardwareAddress string, busDevice bool) bool {
	return busDevice && strings.TrimSpace(permanentHardwareAddress) != ""
}

// busDevicePresent reports whether the kernel presents this interface as a
// device on a real bus. A virtual interface has no device entry at all.
//
// The name is refused before it is joined to the root unless it is one ordinary
// path element: empty, "." and ".." are rejected alongside anything carrying a
// path separator or NUL, so an interface name cannot reach outside
// busDeviceRoot. The caller validates the name too; this is the second lock.
func busDevicePresent(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`+"\x00") {
		return false
	}
	info, err := os.Stat(filepath.Join(busDeviceRoot, name, "device"))
	return err == nil && info.IsDir()
}
