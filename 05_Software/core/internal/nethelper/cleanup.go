package nethelper

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"nilo.local/nilo/internal/network"
)

// OwnedTunnel is the durable application record which authorizes cleanup of
// one exact helper-created NetworkManager connection and its key reference.
type OwnedTunnel struct {
	ProfileID     string `json:"profile_id"`
	ConnectionRef string `json:"connection_ref"`
}

// CleanupDisposition explains a removed, retained, or failed owned resource.
type CleanupDisposition struct {
	Resource string `json:"resource"`
	Reason   string `json:"reason,omitempty"`
}

// CleanupReport is safe to retain as removal evidence; it carries identities
// and outcomes but no configuration or key material.
type CleanupReport struct {
	Schema   int                  `json:"schema"`
	Removed  []CleanupDisposition `json:"removed"`
	Retained []CleanupDisposition `json:"retained"`
	Failed   []CleanupDisposition `json:"failed"`
}

type managementAccessCleanupDriver interface {
	ManagementAccessDriver
	RemoveManagementAccess(context.Context) error
}

type ownedStatePurger interface {
	PurgeOwnedState() error
}

// CleanupOwned converges only the exact external effects backed by helper or
// application ownership records. An active tunnel is treated as a possible
// sole remote management path and survives unless the caller proves local or
// out-of-band confirmation.
func (e *Engine) CleanupOwned(
	ctx context.Context,
	tunnels []OwnedTunnel,
	localConfirmed bool,
	purgeState bool,
) (CleanupReport, error) {
	report := CleanupReport{
		Schema: 1, Removed: []CleanupDisposition{}, Retained: []CleanupDisposition{}, Failed: []CleanupDisposition{},
	}
	seenProfiles := map[string]bool{}
	seenConnections := map[string]bool{}
	for _, owned := range tunnels {
		if !validID.MatchString(owned.ProfileID) || strings.TrimSpace(owned.ConnectionRef) == "" ||
			len(owned.ConnectionRef) > 256 || strings.ContainsAny(owned.ConnectionRef, "\r\n\x00") ||
			seenProfiles[owned.ProfileID] || seenConnections[owned.ConnectionRef] {
			return report, errors.New("owned tunnel inventory is invalid or duplicated")
		}
		seenProfiles[owned.ProfileID] = true
		seenConnections[owned.ConnectionRef] = true
	}

	var failures []error
	recordFailure := func(resource string, err error) {
		report.Failed = append(report.Failed, CleanupDisposition{Resource: resource, Reason: err.Error()})
		failures = append(failures, fmt.Errorf("%s: %w", resource, err))
	}
	if e.forwarding != nil {
		plans, err := e.AppliedForwarding(ctx)
		if err != nil {
			recordFailure("forwarding/inventory", err)
		} else {
			for _, plan := range plans {
				resource := "forwarding/" + plan.DeviceNetworkID
				if err := e.RemoveForwarding(ctx, plan.DeviceNetworkID); err != nil {
					recordFailure(resource, err)
				} else {
					report.Removed = append(report.Removed, CleanupDisposition{Resource: resource})
				}
			}
		}
	}

	if _, ok := e.driver.(CommissioningLinkDriver); ok {
		if err := e.ReconcileCommissioningLinks(ctx, []network.CommissioningLink{}); err != nil {
			recordFailure("commissioning/links", err)
		} else {
			report.Removed = append(report.Removed, CleanupDisposition{Resource: "commissioning/links"})
		}
	}
	if _, ok := e.driver.(network.CommissioningAccessPointDriver); ok {
		if err := e.ReconcileCommissioningAccessPoint(ctx, network.CommissioningAccessPoint{}); err != nil {
			recordFailure("commissioning/access-point", err)
		} else {
			report.Removed = append(report.Removed, CleanupDisposition{Resource: "commissioning/access-point"})
		}
	}

	observedTunnels, tunnelErr := e.Tunnels(ctx)
	if tunnelErr != nil && len(tunnels) != 0 {
		recordFailure("wireguard/inventory", tunnelErr)
	} else {
		for _, owned := range tunnels {
			resource := "wireguard/" + owned.ConnectionRef
			observed, found := findTunnel(observedTunnels, owned.ConnectionRef)
			if found && !observed.NILOOwned {
				report.Retained = append(report.Retained, CleanupDisposition{
					Resource: resource, Reason: "live connection does not prove NILO ownership",
				})
				continue
			}
			if found && observed.Active && !localConfirmed {
				report.Retained = append(report.Retained, CleanupDisposition{
					Resource: resource, Reason: "active management path requires local confirmation",
				})
				continue
			}
			if found && observed.Active {
				driver, err := e.tunnelDriver()
				if err != nil {
					recordFailure(resource, err)
					continue
				}
				if err := driver.DeactivateTunnel(ctx, owned.ConnectionRef); err != nil {
					recordFailure(resource, fmt.Errorf("deactivate confirmed management tunnel: %w", err))
					continue
				}
			}
			if err := e.DeleteProfile(ctx, owned.ProfileID, owned.ConnectionRef); err != nil {
				recordFailure(resource, err)
			} else {
				report.Removed = append(report.Removed, CleanupDisposition{Resource: resource})
			}
		}
	}

	e.mu.Lock()
	mdnsPublished := e.mdnsPublished
	ssdpPublished := e.ssdpPublished
	e.mu.Unlock()
	if mdnsPublished {
		if err := e.WithdrawMDNS(ctx); err != nil {
			recordFailure("advertisement/mdns", err)
		} else {
			report.Removed = append(report.Removed, CleanupDisposition{Resource: "advertisement/mdns"})
		}
	}
	if ssdpPublished {
		if err := e.WithdrawSSDP(ctx); err != nil {
			recordFailure("advertisement/ssdp", err)
		} else {
			report.Removed = append(report.Removed, CleanupDisposition{Resource: "advertisement/ssdp"})
		}
	}
	if driver, ok := e.driver.(managementAccessCleanupDriver); ok {
		state, err := driver.ManagementAccessTable(ctx)
		switch {
		case err != nil:
			recordFailure("firewall/management", err)
		case state == TableForeign:
			report.Retained = append(report.Retained, CleanupDisposition{
				Resource: "firewall/management", Reason: "live table does not prove NILO ownership",
			})
		case state == TableOwned:
			if err := driver.RemoveManagementAccess(ctx); err != nil {
				recordFailure("firewall/management", err)
			} else {
				report.Removed = append(report.Removed, CleanupDisposition{Resource: "firewall/management"})
			}
		}
	}
	if purgeState && !localConfirmed {
		recordFailure("helper/state", errors.New("helper state purge requires local confirmation"))
	}
	if purgeState && localConfirmed && len(failures) == 0 {
		if purger, ok := e.store.(ownedStatePurger); ok {
			if err := purger.PurgeOwnedState(); err != nil {
				recordFailure("helper/state", err)
			} else {
				report.Removed = append(report.Removed, CleanupDisposition{Resource: "helper/state"})
			}
		}
	}
	byResource := func(values []CleanupDisposition) {
		sort.Slice(values, func(i, j int) bool { return values[i].Resource < values[j].Resource })
	}
	byResource(report.Removed)
	byResource(report.Retained)
	byResource(report.Failed)
	return report, errors.Join(failures...)
}
