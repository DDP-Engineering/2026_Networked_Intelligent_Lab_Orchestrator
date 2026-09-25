//go:build linux

package nethelper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"github.com/godbus/dbus/v5"
	"nilo.local/nilo/internal/network"
)

func (n *NetworkManager) ReannounceMDNS(
	ctx context.Context, operationID string, advertisement network.MDNSAdvertisement,
) error {
	if n.mdnsReannouncements[operationID] {
		return nil
	}
	if err := n.ReplaceMDNS(ctx, advertisement); err != nil {
		return err
	}
	n.mdnsReannouncements[operationID] = true
	return nil
}

func (n *NetworkManager) AcknowledgeMDNSReannouncement(operationID string) {
	delete(n.mdnsReannouncements, operationID)
}

const (
	avahiService                 = "org.freedesktop.Avahi"
	avahiServerAPI               = avahiService + ".Server"
	avahiGroupAPI                = avahiService + ".EntryGroup"
	avahiProtocolAny      int32  = -1
	avahiPublishNoReverse uint32 = 16
)

// ReplaceMDNS resets only the D-Bus entry group owned by this helper process.
// The stock Avahi configuration and every other publisher remain untouched.
func (n *NetworkManager) ReplaceMDNS(ctx context.Context, advertisement network.MDNSAdvertisement) error {
	plans, err := avahiPublicationGroups(advertisement)
	if err != nil {
		return err
	}
	server := n.bus.Object(avahiService, dbus.ObjectPath("/"))
	var effectiveFQDN string
	if err := server.CallWithContext(ctx, avahiServerAPI+".GetHostNameFqdn", 0).Store(&effectiveFQDN); err != nil {
		return fmt.Errorf("read effective Avahi hostname: %w", err)
	}
	for index := range plans {
		if plans[index].Service == nil {
			continue
		}
		plans[index], err = applyAvahiEffectiveHost(plans[index], effectiveFQDN)
		if err != nil {
			return err
		}
	}
	devices, err := n.Devices(ctx)
	if err != nil {
		return err
	}
	byName := make(map[string]Device, len(devices))
	for _, device := range devices {
		byName[device.Interface.Name] = device
	}
	desired := make(map[string]bool, len(plans))
	var publicationErrors []error
	// Permanent groups are committed first, so an optional alias collision
	// cannot take the serial service down with it.
	for pass := 0; pass < 2; pass++ {
		for _, plan := range plans {
			if (plan.Service == nil) != (pass == 1) {
				continue
			}
			desired[plan.Key] = true
			group, publishErr := n.replaceAvahiGroup(ctx, server, n.mdnsGroups[plan.Key], plan, byName)
			if publishErr != nil {
				publicationErrors = append(publicationErrors, publishErr)
				continue
			}
			n.mdnsGroups[plan.Key] = group
		}
	}
	for key, group := range n.mdnsGroups {
		if desired[key] {
			continue
		}
		if err := n.bus.Object(avahiService, group).CallWithContext(ctx, avahiGroupAPI+".Reset", 0).Err; err != nil && !avahiGroupMissing(err) {
			publicationErrors = append(publicationErrors, fmt.Errorf("withdraw stale Avahi group: %w", err))
			continue
		}
		delete(n.mdnsGroups, key)
	}
	return errors.Join(publicationErrors...)
}

func (n *NetworkManager) replaceAvahiGroup(ctx context.Context, server dbus.BusObject, group dbus.ObjectPath, plan avahiPlan, devices map[string]Device) (dbus.ObjectPath, error) {
	if group == "" {
		if err := server.CallWithContext(ctx, avahiServerAPI+".EntryGroupNew", 0).Store(&group); err != nil {
			return "", fmt.Errorf("create Avahi entry group: %w", err)
		}
	} else if err := n.bus.Object(avahiService, group).CallWithContext(ctx, avahiGroupAPI+".Reset", 0).Err; err != nil {
		group = ""
		if err := server.CallWithContext(ctx, avahiServerAPI+".EntryGroupNew", 0).Store(&group); err != nil {
			return "", fmt.Errorf("recreate Avahi entry group: %w", err)
		}
	}
	entry := n.bus.Object(avahiService, group)
	interfaceName := plan.Interfaces[0]
	device, ok := devices[interfaceName]
	if !ok {
		return "", fmt.Errorf("mDNS interface %s is no longer present", interfaceName)
	}
	link, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return "", fmt.Errorf("resolve mDNS interface %s: %w", interfaceName, err)
	}
	if !plan.UseGlobalHost {
		for _, prefixText := range append(append([]string(nil), device.Interface.IPv4...), device.Interface.IPv6...) {
			prefix, parseErr := netip.ParsePrefix(prefixText)
			if parseErr == nil && !prefix.Addr().IsUnspecified() {
				if err := entry.CallWithContext(ctx, avahiGroupAPI+".AddAddress", 0, int32(link.Index), avahiProtocolAny, avahiPublishNoReverse, plan.HostFQDN, prefix.Addr().String()).Err; err != nil {
					_ = entry.Call(avahiGroupAPI+".Reset", 0).Err
					return "", fmt.Errorf("publish %s on %s: %w", plan.HostFQDN, interfaceName, err)
				}
			}
		}
	}
	if plan.Service != nil {
		txt := make([][]byte, len(plan.Service.TXT))
		for index, value := range plan.Service.TXT {
			txt[index] = []byte(value)
		}
		if err := entry.CallWithContext(ctx, avahiGroupAPI+".AddService", 0, int32(link.Index), avahiProtocolAny, uint32(0), plan.Service.Instance, plan.Service.Type, plan.Service.Domain, plan.Service.HostFQDN, plan.Service.Port, txt).Err; err != nil {
			_ = entry.Call(avahiGroupAPI+".Reset", 0).Err
			return "", fmt.Errorf("publish NILO service on %s: %w", interfaceName, err)
		}
	}
	if err := entry.CallWithContext(ctx, avahiGroupAPI+".Commit", 0).Err; err != nil {
		_ = entry.Call(avahiGroupAPI+".Reset", 0).Err
		return "", fmt.Errorf("commit Avahi entry group: %w", err)
	}
	if err := waitForAvahiGroup(ctx, entry); err != nil {
		_ = entry.Call(avahiGroupAPI+".Reset", 0).Err
		if errors.Is(err, network.ErrMDNSCollision) {
			err = network.NewMDNSCollisionError([]string{interfaceName})
		}
		return "", fmt.Errorf("establish Avahi entry group: %w", err)
	}
	return group, nil
}

func waitForAvahiGroup(ctx context.Context, entry dbus.BusObject) error {
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var state int32
		if err := entry.CallWithContext(ctx, avahiGroupAPI+".GetState", 0).Store(&state); err != nil {
			return err
		}
		switch state {
		case 2:
			return nil
		case 3:
			return network.ErrMDNSCollision
		case 4:
			return errors.New("Avahi entry group failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("Avahi entry group establishment timed out")
		case <-ticker.C:
		}
	}
}

func (n *NetworkManager) MDNSPublicationCurrent(ctx context.Context) (bool, error) {
	if len(n.mdnsGroups) == 0 {
		return false, nil
	}
	for _, group := range n.mdnsGroups {
		var state int32
		if err := n.bus.Object(avahiService, group).CallWithContext(ctx, avahiGroupAPI+".GetState", 0).Store(&state); err != nil || state != 2 {
			return false, nil
		}
	}
	return true, nil
}

func (n *NetworkManager) WithdrawMDNS(ctx context.Context) error {
	if len(n.mdnsGroups) == 0 {
		return nil
	}
	var withdrawalErrors []error
	for key, group := range n.mdnsGroups {
		if err := n.bus.Object(avahiService, group).CallWithContext(ctx, avahiGroupAPI+".Reset", 0).Err; err != nil {
			if !avahiGroupMissing(err) {
				withdrawalErrors = append(withdrawalErrors, fmt.Errorf("withdraw Avahi entry group: %w", err))
				continue
			}
		}
		delete(n.mdnsGroups, key)
	}
	return errors.Join(withdrawalErrors...)
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
