package network

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// Management-path protection: the appliance must never take away the last way
// an administrator can reach it. A path is traced from what is observed, not
// declared - the adapter whose link the client is on, the tunnel whose routes
// carry the client, an adapter with a gateway when the client is routed - plus
// whatever the operator marked protected. A change that would remove the last
// traced path is refused and names it.
//
// Nothing here can promise access through a power failure or a pulled cable;
// it decides only what the software will refuse to do to itself.

// ErrManagementPathRequired refuses a change that would leave no usable
// management path.
var ErrManagementPathRequired = errors.New("management_path_required")

// TunnelPath describes the active NILO-owned tunnel for tracing.
type TunnelPath struct {
	ProfileID string
	Name      string
	Interface string
	// Routes are the addresses the tunnel carries, from the profile's allowed
	// addresses.
	Routes []netip.Prefix
}

// ManagementContext is what the trace needs beyond the adapters: where the
// request came from and which tunnel is up.
type ManagementContext struct {
	// ClientAddress is the requesting client's address. A loopback or unknown
	// address traces to nothing, so only operator-protected paths remain.
	ClientAddress netip.Addr
	Tunnel        *TunnelPath
}

// ManagementPath is one usable way to reach the appliance and why it counts.
type ManagementPath struct {
	InterfaceID string
	Name        string
	Reason      string
	Tunnel      bool
	ProfileID   string
}

func (path ManagementPath) String() string {
	return fmt.Sprintf("%s (%s)", path.Name, path.Reason)
}

// clientKnown reports whether the client address says anything about a path.
func (context ManagementContext) clientKnown() bool {
	address := context.ClientAddress
	return address.IsValid() && !address.IsLoopback() && !address.IsUnspecified()
}

func prefixesOf(texts []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(texts))
	for _, text := range texts {
		if prefix, err := netip.ParsePrefix(text); err == nil {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}

func containsAny(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address.Unmap()) {
			return true
		}
	}
	return false
}

// TraceManagementPaths lists every usable management path.
func TraceManagementPaths(interfaces []Interface, saved []InterfaceSettings, context ManagementContext) []ManagementPath {
	client := context.ClientAddress.Unmap()
	known := context.clientKnown()
	throughTunnel := known && context.Tunnel != nil && containsAny(context.Tunnel.Routes, client)
	onLink := false
	if known {
		for _, candidate := range interfaces {
			if usableAdapter(candidate) && containsAny(prefixesOf(candidate.IPv4), client) {
				onLink = true
			}
		}
	}
	routed := known && !onLink && !throughTunnel

	paths := make([]ManagementPath, 0, len(interfaces)+1)
	for _, candidate := range interfaces {
		if !usableAdapter(candidate) {
			continue
		}
		reasons := make([]string, 0, 3)
		if settings, ok := findSettings(saved, candidate.AdapterID); ok && settings.ManagementProtected {
			reasons = append(reasons, "protected by the operator")
		}
		if known && containsAny(prefixesOf(candidate.IPv4), client) {
			reasons = append(reasons, fmt.Sprintf("carries the client %s on its link", client))
		}
		if routed && (candidate.Gateway4 != "" || candidate.Gateway6 != "") {
			reasons = append(reasons, fmt.Sprintf("reaches the routed client %s through its gateway", client))
		}
		if len(reasons) > 0 {
			paths = append(paths, ManagementPath{InterfaceID: candidate.ID, Name: candidate.Name, Reason: strings.Join(reasons, "; ")})
		}
	}
	if throughTunnel {
		paths = append(paths, ManagementPath{
			Name: context.Tunnel.Name, Tunnel: true, ProfileID: context.Tunnel.ProfileID,
			Reason: fmt.Sprintf("carries the client %s through the tunnel", client),
		})
	}
	return paths
}

// usableAdapter is a connected wired or wireless adapter with an IPv4 address.
// An unplugged adapter is never a path, protected or not.
func usableAdapter(candidate Interface) bool {
	return (candidate.Kind == KindEthernet || candidate.Kind == KindWiFi) &&
		candidate.State == StateConnected && len(candidate.IPv4) > 0
}

// ValidateManagementChange refuses a change that would remove the last usable
// management path.
func ValidateManagementChange(change Change, interfaces []Interface, saved []InterfaceSettings, context ManagementContext) error {
	paths := TraceManagementPaths(interfaces, saved, context)
	var affected []ManagementPath
	var remaining []ManagementPath
	for _, path := range paths {
		if changeRemovesPath(change, path, interfaces) {
			affected = append(affected, path)
		} else {
			remaining = append(remaining, path)
		}
	}
	if len(affected) == 0 || len(remaining) > 0 {
		return nil
	}
	path := affected[0]
	return fmt.Errorf("%w: %s is the only usable management path - it %s; keep another path in place before changing it",
		ErrManagementPathRequired, path.Name, path.Reason)
}

// changeRemovesPath decides whether one change takes one path away.
func changeRemovesPath(change Change, path ManagementPath, interfaces []Interface) bool {
	switch change.Action {
	case ActionInterfaceDisable:
		return !path.Tunnel && path.InterfaceID == change.Target
	case ActionWiFiJoin:
		return !path.Tunnel && path.InterfaceID == change.Target
	case ActionInterfaceSettings:
		if path.Tunnel || path.InterfaceID != change.Target || change.Settings == nil {
			return false
		}
		return !settingsKeepAddress(*change.Settings, interfaces, change.Target)
	case ActionVPNDeactivate, ActionVPNActivate:
		// Activating another profile deactivates the one that is up, so both
		// take the tunnel path away.
		return path.Tunnel
	default:
		return false
	}
}

// settingsKeepAddress reports whether a settings change leaves the adapter's
// current address in place: a static configuration naming the address the
// adapter already carries changes nothing about how the client reaches it.
func settingsKeepAddress(settings InterfaceSettings, interfaces []Interface, interfaceID string) bool {
	if settings.Method != MethodStatic || !settings.Address.IsValid() {
		return false
	}
	for _, candidate := range interfaces {
		if candidate.ID != interfaceID {
			continue
		}
		for _, address := range candidate.IPv4 {
			if address == settings.Address.String() {
				return true
			}
		}
	}
	return false
}
