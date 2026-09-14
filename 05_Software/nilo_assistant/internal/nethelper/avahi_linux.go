//go:build linux

package nethelper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/godbus/dbus/v5"
	"nilo.local/nilo/internal/network"
)

const (
	avahiService           = "org.freedesktop.Avahi"
	avahiServerAPI         = avahiService + ".Server"
	avahiGroupAPI          = avahiService + ".EntryGroup"
	avahiProtocolAny int32 = -1
)

// ReplaceMDNS resets only the D-Bus entry group owned by this helper process.
// The stock Avahi configuration and every other publisher remain untouched.
func (n *NetworkManager) ReplaceMDNS(ctx context.Context, advertisement network.MDNSAdvertisement) error {
	plan, err := avahiPublicationPlan(advertisement)
	if err != nil {
		return err
	}
	group := n.mdnsGroup
	server := n.bus.Object(avahiService, dbus.ObjectPath("/"))
	if plan.Service != nil {
		var effectiveFQDN string
		if err := server.CallWithContext(ctx, avahiServerAPI+".GetHostNameFqdn", 0).Store(&effectiveFQDN); err != nil {
			return fmt.Errorf("read effective Avahi hostname: %w", err)
		}
		plan, err = applyAvahiEffectiveHost(plan, effectiveFQDN)
		if err != nil {
			return err
		}
	}
	if group == "" {
		if err := server.CallWithContext(ctx, avahiServerAPI+".EntryGroupNew", 0).Store(&group); err != nil {
			return fmt.Errorf("create Avahi entry group: %w", err)
		}
	} else if err := n.bus.Object(avahiService, group).CallWithContext(ctx, avahiGroupAPI+".Reset", 0).Err; err != nil {
		group = ""
		if createErr := server.CallWithContext(ctx, avahiServerAPI+".EntryGroupNew", 0).Store(&group); createErr != nil {
			return fmt.Errorf("recreate Avahi entry group after responder restart: %w", createErr)
		}
	}
	entry := n.bus.Object(avahiService, group)
	failed := true
	defer func() {
		if failed {
			_ = entry.Call(avahiGroupAPI+".Reset", 0).Err
		}
	}()
	devices, err := n.Devices(ctx)
	if err != nil {
		return err
	}
	byName := make(map[string]Device, len(devices))
	for _, device := range devices {
		byName[device.Interface.Name] = device
	}
	names := append([]avahiAliasPlan(nil), plan.Aliases...)
	if plan.Service == nil {
		names = append([]avahiAliasPlan{{HostFQDN: plan.HostFQDN, Interfaces: plan.Interfaces}}, names...)
	}
	for _, publication := range names {
		for _, interfaceName := range publication.Interfaces {
			device, ok := byName[interfaceName]
			if !ok {
				return fmt.Errorf("mDNS interface %s is no longer present", interfaceName)
			}
			link, err := net.InterfaceByName(interfaceName)
			if err != nil {
				return fmt.Errorf("resolve mDNS interface %s: %w", interfaceName, err)
			}
			addresses := append(append([]string(nil), device.Interface.IPv4...), device.Interface.IPv6...)
			for _, prefixText := range addresses {
				prefix, err := netip.ParsePrefix(prefixText)
				if err != nil || prefix.Addr().IsUnspecified() {
					continue
				}
				if err := entry.CallWithContext(ctx, avahiGroupAPI+".AddAddress", 0,
					int32(link.Index), avahiProtocolAny, uint32(0), publication.HostFQDN, prefix.Addr().String()).Err; err != nil {
					return fmt.Errorf("publish %s on %s: %w", publication.HostFQDN, interfaceName, err)
				}
			}
		}
	}
	if plan.Service != nil {
		for _, interfaceName := range plan.Interfaces {
			link, err := net.InterfaceByName(interfaceName)
			if err != nil {
				return fmt.Errorf("resolve service interface %s: %w", interfaceName, err)
			}
			txt := make([][]byte, len(plan.Service.TXT))
			for index, value := range plan.Service.TXT {
				txt[index] = []byte(value)
			}
			if err := entry.CallWithContext(ctx, avahiGroupAPI+".AddService", 0,
				int32(link.Index), avahiProtocolAny, uint32(0), plan.Service.Instance,
				plan.Service.Type, plan.Service.Domain, plan.Service.HostFQDN,
				plan.Service.Port, txt).Err; err != nil {
				return fmt.Errorf("publish NILO service on %s: %w", interfaceName, err)
			}
		}
	}
	if err := entry.CallWithContext(ctx, avahiGroupAPI+".Commit", 0).Err; err != nil {
		return fmt.Errorf("commit Avahi entry group: %w", err)
	}
	n.mdnsGroup = group
	failed = false
	return nil
}

func (n *NetworkManager) MDNSPublicationCurrent(ctx context.Context) (bool, error) {
	if n.mdnsGroup == "" {
		return false, nil
	}
	var state int32
	if err := n.bus.Object(avahiService, n.mdnsGroup).CallWithContext(ctx, avahiGroupAPI+".GetState", 0).Store(&state); err != nil {
		return false, nil
	}
	return state == 2, nil
}

func (n *NetworkManager) WithdrawMDNS(ctx context.Context) error {
	if n.mdnsGroup == "" {
		return nil
	}
	if err := n.bus.Object(avahiService, n.mdnsGroup).CallWithContext(ctx, avahiGroupAPI+".Reset", 0).Err; err != nil {
		if avahiGroupMissing(err) {
			n.mdnsGroup = ""
			return nil
		}
		return fmt.Errorf("withdraw Avahi entry group: %w", err)
	}
	return nil
}

func avahiGroupMissing(err error) bool {
	var busErr dbus.Error
	if !errors.As(err, &busErr) {
		return false
	}
	switch busErr.Name {
	case "org.freedesktop.DBus.Error.UnknownObject",
		"org.freedesktop.DBus.Error.ServiceUnknown",
		"org.freedesktop.DBus.Error.NameHasNoOwner",
		"org.freedesktop.Avahi.InvalidObject":
		return true
	default:
		return false
	}
}

var _ MDNSDriver = (*NetworkManager)(nil)
