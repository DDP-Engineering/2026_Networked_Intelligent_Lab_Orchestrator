package nethelper

import (
	"context"
	"strings"

	"nilo.local/nilo/internal/network"
)

// Each probe distinguishes a known negative from an observation failure.
type readinessProbes struct {
	manager func(context.Context) (bool, string, error)
	kernel  func(context.Context) (bool, error)
	tool    func(context.Context) (network.ToolStatus, error)
}

func readWireGuard(ctx context.Context, p readinessProbes) network.WireGuardStatus {
	// Readiness alone says nothing about profiles. The appliance provider merges
	// the saved inventory in and clears ReadinessOnly; a reader of this reply
	// on its own must not take the empty list for an empty inventory.
	s := network.WireGuardStatus{Reported: true, ReadinessOnly: true, HelperAvailable: true, Tools: []network.ToolStatus{}, Profiles: []network.ProfileStatus{}}
	details := []string{}
	var err error
	s.NetworkManagerAvailable, s.NetworkManagerVersion, err = p.manager(ctx)
	if err != nil {
		s.Reported = false
		details = append(details, "NetworkManager readiness could not be verified.")
	}
	if !s.NetworkManagerAvailable {
		s.NetworkManagerVersion = ""
	}
	s.KernelSupported, err = p.kernel(ctx)
	if err != nil {
		s.Reported = false
		details = append(details, "Kernel WireGuard support could not be verified.")
	}
	tool, err := p.tool(ctx)
	s.Tools = append(s.Tools, tool)
	if err != nil {
		s.Reported = false
		details = append(details, "The wg tool version could not be verified.")
	}
	s.Detail = strings.Join(details, " ")
	return s
}

func (e *Engine) WireGuard(ctx context.Context) (network.WireGuardStatus, error) {
	reader, ok := e.driver.(network.WireGuardReader)
	if !ok {
		return network.WireGuardStatus{}, network.ErrUnsupported
	}
	status, err := reader.WireGuard(ctx)
	if err != nil {
		return status, err
	}
	if e.install != nil {
		plan, planErr := e.InstallPlan(ctx)
		status.CanInstall = planErr == nil && !plan.Installed && status.NetworkManagerAvailable
		if planErr != nil {
			status.Detail += " Tool installation is unavailable on this host or its repository configuration."
		}
	}
	return status, nil
}

// WireGuard reads readiness through the same authenticated helper as interfaces.
func (c *Client) WireGuard(ctx context.Context) (network.WireGuardStatus, error) {
	r, err := c.call(ctx, Request{Op: "wireguard.check"})
	if err != nil {
		return network.WireGuardStatus{}, err
	}
	if r.WireGuard == nil {
		return network.WireGuardStatus{}, network.ErrUnsupported
	}
	s := *r.WireGuard
	s.MeasuredBy = c.Source()
	return s, nil
}
