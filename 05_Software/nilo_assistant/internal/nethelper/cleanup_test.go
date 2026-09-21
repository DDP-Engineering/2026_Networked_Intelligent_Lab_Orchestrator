package nethelper

import (
	"context"
	"errors"
	"testing"

	"nilo.local/nilo/internal/network"
)

func TestCleanupOwnedRetainsTheActiveManagementTunnelUntilLocalConfirmation(t *testing.T) {
	engine, driver, _ := tunnelEngine(t)
	first := createProfile(t, engine, "profile-office", "office")
	second := createProfile(t, engine, "profile-lab", "lab")
	driver.tunnels.external = true
	driver.tunnels.active = first.ConnectionRef
	owned := []OwnedTunnel{
		{ProfileID: "profile-lab", ConnectionRef: second.ConnectionRef},
		{ProfileID: "profile-office", ConnectionRef: first.ConnectionRef},
	}

	report, err := engine.CleanupOwned(context.Background(), owned, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Retained) != 1 || report.Retained[0].Resource != "wireguard/"+first.ConnectionRef ||
		report.Retained[0].Reason != "active management path requires local confirmation" {
		t.Fatalf("retained = %#v", report.Retained)
	}
	tunnels, err := driver.Tunnels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, found := findTunnel(tunnels, first.ConnectionRef); !found {
		t.Fatal("active owned tunnel was removed without local confirmation")
	}
	if external, found := findTunnel(tunnels, "external"); !found || external.NILOOwned {
		t.Fatalf("foreign tunnel changed: %#v", tunnels)
	}
	if _, found := findTunnel(tunnels, second.ConnectionRef); found {
		t.Fatal("inactive, durably owned tunnel was retained")
	}

	report, err = engine.CleanupOwned(context.Background(), owned, true, true)
	if err != nil {
		t.Fatal(err)
	}
	tunnels, err = driver.Tunnels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, found := findTunnel(tunnels, first.ConnectionRef); found {
		t.Fatal("locally confirmed owned tunnel was retained")
	}
	if external, found := findTunnel(tunnels, "external"); !found || external.ConnectionRef != "external" ||
		external.Name != "wg0" || external.Interface != "wg0" || !external.Active || external.NILOOwned {
		t.Fatalf("foreign tunnel changed after confirmed cleanup: %#v", tunnels)
	}
}

func TestCleanupOwnedReportsPartialFailureAndRetainsItsOwnershipRecord(t *testing.T) {
	engine, driver, _ := tunnelEngine(t)
	created := createProfile(t, engine, "profile-office", "office")
	driver.tunnels.deleteErr = errors.New("network manager unavailable")
	report, err := engine.CleanupOwned(context.Background(), []OwnedTunnel{{
		ProfileID: "profile-office", ConnectionRef: created.ConnectionRef,
	}}, true, false)
	if err == nil || len(report.Failed) != 1 || report.Failed[0].Resource != "wireguard/"+created.ConnectionRef {
		t.Fatalf("cleanup result = %#v, %v", report, err)
	}
	if _, found := findTunnel(mustTunnels(t, driver), created.ConnectionRef); !found {
		t.Fatal("failed cleanup deleted the owned live resource")
	}
}

func mustTunnels(t *testing.T, driver *fakeDriver) []network.TunnelObservation {
	t.Helper()
	tunnels, err := driver.Tunnels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return tunnels
}
