package network

import (
	"context"
	"testing"
)

type commissioningAPDriver struct{ plans []CommissioningAccessPoint }

func (driver *commissioningAPDriver) ReconcileCommissioningAccessPoint(_ context.Context, plan CommissioningAccessPoint) error {
	driver.plans = append(driver.plans, plan)
	return nil
}

func TestCommissioningAccessPointSelectsOneUnusedUnprotectedRadio(t *testing.T) {
	driver := &commissioningAPDriver{}
	roles := NewAdapterRoleCoordinator()
	roles.RegisterClaimSource(func() []AdapterRoleClaim {
		return []AdapterRoleClaim{{AdapterID: "02:00:00:00:00:02", Interface: "wlan0", Role: AdapterRoleDevice}}
	})
	service, err := NewCommissioningAccessPointService(CommissioningAccessPointOptions{
		Provider: &commissioningAPProvider{interfaces: []Interface{
			{Name: "wlan-saved", AdapterID: "02:00:00:00:00:01", Kind: KindWiFi, Physical: true, SavedConnections: []string{"home-wifi"}},
			{Name: "wlan0", AdapterID: "02:00:00:00:00:02", Kind: KindWiFi, Physical: true},
			{Name: "wlan2", AdapterID: "02:00:00:00:00:04", Kind: KindWiFi, Physical: true, Settings: &SettingsState{}},
			{Name: "wlan1", AdapterID: "02:00:00:00:00:03", Kind: KindWiFi, Physical: true},
		}},
		Driver: driver, Roles: roles, SerialName: "nilo-p2rjmzdkf1", Port: 8443,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(driver.plans) != 1 || !driver.plans[0].Enabled || driver.plans[0].Interface != "wlan1" || driver.plans[0].Address != "192.168.247.1/24" {
		t.Fatalf("plans = %+v", driver.plans)
	}
	if err := service.Reconcile(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if driver.plans[1].Enabled {
		t.Fatalf("commissioning teardown plan = %+v", driver.plans[1])
	}
}

func TestCommissioningAccessPointRetainsOwnedRadioAcrossMaintenanceAndRestart(t *testing.T) {
	provider := &commissioningAPProvider{interfaces: []Interface{{
		Name: "wlan1", AdapterID: "02:00:00:00:00:03", Kind: KindWiFi, Physical: true,
	}}}
	driver := &commissioningAPDriver{}
	service, err := NewCommissioningAccessPointService(CommissioningAccessPointOptions{
		Provider: provider, Driver: driver, SerialName: "nilo-p2rjmzdkf1", Port: 8443,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	provider.interfaces[0].State = StateConnected
	provider.interfaces[0].SSID = "nilo-p2rjmzdkf1"
	provider.interfaces[0].SavedConnections = []string{"helper-owned-uuid"}
	if err := service.Reconcile(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(driver.plans) != 2 || !driver.plans[1].Enabled || driver.plans[1].AdapterID != "02:00:00:00:00:03" {
		t.Fatalf("maintenance plans = %+v", driver.plans)
	}

	restartedDriver := &commissioningAPDriver{}
	restarted, err := NewCommissioningAccessPointService(CommissioningAccessPointOptions{
		Provider: provider, Driver: restartedDriver, SerialName: "nilo-p2rjmzdkf1", Port: 8443,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Reconcile(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(restartedDriver.plans) != 1 || !restartedDriver.plans[0].RetainOwned {
		t.Fatalf("restart plan = %+v", restartedDriver.plans)
	}
}

type commissioningAPProvider struct{ interfaces []Interface }

func (provider *commissioningAPProvider) Interfaces(context.Context) ([]Interface, error) {
	return append([]Interface(nil), provider.interfaces...), nil
}
func (*commissioningAPProvider) Source() Source { return "test" }
