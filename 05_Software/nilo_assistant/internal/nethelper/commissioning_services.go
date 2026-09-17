package nethelper

import (
	"context"
	"net/netip"

	"nilo.local/nilo/internal/network"
)

type hostRoute struct {
	Interface string
	Prefix    netip.Prefix
}

func commissioningSubnetAvailable(subnet netip.Prefix, ownedInterface string, routes []hostRoute) bool {
	for _, route := range routes {
		if route.Interface == ownedInterface && route.Prefix == subnet {
			continue
		}
		if route.Prefix.IsValid() && (subnet.Contains(route.Prefix.Addr()) || route.Prefix.Contains(subnet.Addr())) {
			return false
		}
	}
	return true
}

type commissioningServiceDriver interface {
	Check(network.CommissioningAccessPoint) error
	Start(network.CommissioningAccessPoint) error
	Stop(context.Context) error
}
