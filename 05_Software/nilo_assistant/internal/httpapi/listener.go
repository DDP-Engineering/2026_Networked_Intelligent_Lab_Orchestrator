// Package httpapi exposes the authenticated Bench Core HTTP boundary.
package httpapi

import (
	"fmt"
	"net"
)

// ValidateSimulationBind rejects any listener address outside IP loopback.
func ValidateSimulationBind(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse simulation bind address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("simulation bind host %q must be a literal loopback address", host)
	}
	return nil
}
