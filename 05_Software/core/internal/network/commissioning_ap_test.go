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
	const ownedConnectionUUID = "0199a2d0-1234-7abc-8def-0123456789ab"
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
	provider.interfaces[0].SavedConnections = []string{ownedConnectionUUID}
	if err := service.Reconcile(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(driver.plans) != 2 || !driver.plans[1].RetainOwned ||
		len(driver.plans[1].EligibleAdapterIDs) != 1 || driver.plans[1].EligibleAdapterIDs[0] != "02:00:00:00:00:03" {
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
	if len(restartedDriver.plans) != 1 || !restartedDriver.plans[0].RetainOwned ||
		len(restartedDriver.plans[0].EligibleAdapterIDs) != 1 ||
		restartedDriver.plans[0].EligibleAdapterIDs[0] != "02:00:00:00:00:03" {
		t.Fatalf("restart plan = %+v", restartedDriver.plans)
	}
}

func TestCommissioningAccessPointStopsWhenOwnedRadioBecomesIneligible(t *testing.T) {
	const ownedConnectionUUID = "0199a2d0-1234-7abc-8def-0123456789ab"
	tests := []struct {
		name   string
		mutate func(*commissioningAPProvider, *[]AdapterRoleClaim)
	}{
		{
			name: "foreign saved profile",
			mutate: func(provider *commissioningAPProvider, _ *[]AdapterRoleClaim) {
				provider.interfaces[0].SavedConnections = []string{
					ownedConnectionUUID, "0199a2d0-1234-7abc-8def-0123456789ac",
				}
			},
		},
		{
			name: "protected role",
			mutate: func(_ *commissioningAPProvider, claims *[]AdapterRoleClaim) {
				*claims = []AdapterRoleClaim{{
					AdapterID: "02:00:00:00:00:03", Interface: "wlan1", Role: AdapterRoleDevice,
				}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &commissioningAPProvider{interfaces: []Interface{{
				Name: "wlan1", AdapterID: "02:00:00:00:00:03", Kind: KindWiFi, Physical: true,
			}}}
			driver := &commissioningAPDriver{}
			claims := []AdapterRoleClaim{}
			roles := NewAdapterRoleCoordinator()
			roles.RegisterClaimSource(func() []AdapterRoleClaim { return claims })
			service, err := NewCommissioningAccessPointService(CommissioningAccessPointOptions{
				Provider: provider, Driver: driver, Roles: roles, SerialName: "nilo-p2rjmzdkf1", Port: 8443,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := service.Reconcile(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			provider.interfaces[0].State = StateConnected
			provider.interfaces[0].SSID = "nilo-p2rjmzdkf1"
			provider.interfaces[0].SavedConnections = []string{ownedConnectionUUID}
			test.mutate(provider, &claims)
			if err := service.Reconcile(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			last := driver.plans[len(driver.plans)-1]
			if last.Enabled || last.RetainOwned {
				t.Fatalf("ineligible owned AP was retained: plan=%+v", last)
			}
		})
	}
}

type commissioningAPProvider struct{ interfaces []Interface }

func (provider *commissioningAPProvider) Interfaces(context.Context) ([]Interface, error) {
	return append([]Interface(nil), provider.interfaces...), nil
}
func (*commissioningAPProvider) Source() Source { return "test" }
