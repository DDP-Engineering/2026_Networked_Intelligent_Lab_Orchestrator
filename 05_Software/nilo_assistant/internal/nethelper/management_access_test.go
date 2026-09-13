package nethelper

import (
	"context"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

type managementDriver struct {
	fakeDriver
	state   TableState
	applied []network.ManagementAccessConfiguration
}

func (driver *managementDriver) ManagementAccessTable(context.Context) (TableState, error) {
	return driver.state, nil
}

func (driver *managementDriver) InstallManagementAccess(_ context.Context, configuration network.ManagementAccessConfiguration) error {
	driver.applied = append(driver.applied, configuration)
	return nil
}

func TestHelperAcceptsOnlyTypedValidManagementFirewallRequests(t *testing.T) {
	driver := &managementDriver{state: TableAbsent}
	engine, err := NewEngine(driver, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	configuration := network.ManagementAccessConfiguration{
		Interfaces: []string{"wlan0", "nilo0"}, Port: 8443,
		TLSNames: []string{"ddp-remote.local", "10.205.148.11"},
	}
	response := dispatch(context.Background(), engine, Request{Op: "management.access.apply", ManagementAccess: &configuration})
	if response.Error != "" || len(driver.applied) != 1 {
		t.Fatalf("valid request = %+v, applied=%v", response, driver.applied)
	}
	bad := configuration
	bad.Interfaces = []string{"eth0; flush ruleset"}
	response = dispatch(context.Background(), engine, Request{Op: "management.access.apply", ManagementAccess: &bad})
	if response.Error == "" || len(driver.applied) != 1 {
		t.Fatalf("unsafe interface reached driver: %+v, applied=%v", response, driver.applied)
	}
	response = dispatch(context.Background(), engine, Request{Op: "interface.list", ManagementAccess: &configuration})
	if response.Code != "invalid_request" {
		t.Fatalf("configuration rode an unrelated operation: %+v", response)
	}
}

func TestManagementFirewallCanDenyAllAfterAdaptersDisappear(t *testing.T) {
	driver := &managementDriver{state: TableOwned}
	engine, err := NewEngine(driver, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	configuration := network.ManagementAccessConfiguration{Port: 8443, TLSNames: []string{"nilo.local"}}
	if err := configuration.Validate(); err == nil {
		t.Fatal("empty user configuration must remain invalid")
	}
	response := dispatch(context.Background(), engine, Request{Op: "management.access.apply", ManagementAccess: &configuration})
	if response.Error != "" || len(driver.applied) != 1 || len(driver.applied[0].Interfaces) != 0 {
		t.Fatalf("deny-all firewall request = %+v, applied=%v", response, driver.applied)
	}
}
