package nethelper

import (
	"context"
	"errors"
	"testing"

	"nilo.local/nilo/internal/network"
)

func TestWireGuardReadinessKeepsProbeFactsSeparate(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		manager, kernel, installed bool
		probeErr                   error
	}{
		{name: "ready", manager: true, kernel: true, installed: true},
		{name: "missing tools", manager: true, kernel: true},
		{name: "unsupported kernel", manager: true, installed: true},
		{name: "missing manager", kernel: true, installed: true},
		{name: "inconclusive kernel", manager: true, installed: true, probeErr: errors.New("probe failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := readWireGuard(context.Background(), readinessProbes{
				manager: func(context.Context) (bool, string, error) { return tc.manager, "1.52.1", nil },
				kernel:  func(context.Context) (bool, error) { return tc.kernel, tc.probeErr },
				tool: func(context.Context) (network.ToolStatus, error) {
					version := ""
					if tc.installed {
						version = "1.0.20210914"
					}
					return network.ToolStatus{Name: "wg", Installed: tc.installed, Version: version}, nil
				},
			})
			if status.Reported != (tc.probeErr == nil) || status.NetworkManagerAvailable != tc.manager || status.KernelSupported != tc.kernel || !status.HelperAvailable || status.CanInstall {
				t.Fatalf("incorrect readiness: %+v", status)
			}
			if len(status.Tools) != 1 || status.Tools[0].Installed != tc.installed || (!tc.installed && status.Tools[0].Version != "") || status.Profiles == nil {
				t.Fatalf("missing facts: %+v", status)
			}
			if !tc.manager && status.NetworkManagerVersion != "" {
				t.Fatal("version without manager")
			}
			if tc.probeErr != nil && status.Detail == "" {
				t.Fatal("inconclusive probe needs explanation")
			}
		})
	}
}

type readinessDriver struct{ *fakeDriver }

func (d readinessDriver) WireGuard(context.Context) (network.WireGuardStatus, error) {
	return network.WireGuardStatus{Reported: true, KernelSupported: true, Tools: []network.ToolStatus{{Name: "wg"}}, Profiles: []network.ProfileStatus{}}, nil
}

func TestWireGuardHelperReadIsAllowlistedAndDoesNotMutate(t *testing.T) {
	e, d, s, _ := fixture(t)
	e.driver = readinessDriver{d}
	r := dispatch(context.Background(), e, Request{Op: "wireguard.check"})
	if r.Error != "" || r.WireGuard == nil || !r.WireGuard.Reported {
		t.Fatalf("read failed: %+v", r)
	}
	for _, q := range []Request{
		{Op: "wireguard.check", ID: "unexpected"},
		{Op: "wireguard.check", Change: network.Change{Target: "; touch unwanted"}},
		{Op: "wireguard.install"},
	} {
		if got := dispatch(context.Background(), e, q); got.Error == "" {
			t.Fatalf("accepted %+v", q)
		}
	}
	if len(d.events) != 0 || s.record != nil {
		t.Fatal("read changed network or recovery state")
	}
}
