// Package simulation defines deterministic development-only inventory.
package simulation

import "fmt"

// Scenario identifies one stable simulation state.
type Scenario string

const (
	// ScenarioHealthy exposes ready simulated inventory.
	ScenarioHealthy Scenario = "healthy"
	// ScenarioUnavailable exposes unavailable simulated inventory.
	ScenarioUnavailable Scenario = "unavailable"
)

// Integration is one simulation-only integration inventory entry.
type Integration struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Status      string   `json:"status"`
	Simulation  bool     `json:"simulation"`
	Operations  []string `json:"operations"`
}

// Device is one simulation-only device inventory entry.
type Device struct {
	ID            string   `json:"id"`
	IntegrationID string   `json:"integration_id"`
	DisplayName   string   `json:"display_name"`
	Kind          string   `json:"kind"`
	Status        string   `json:"status"`
	Simulation    bool     `json:"simulation"`
	Capabilities  []string `json:"capabilities"`
}

// Catalog is an immutable snapshot for one deterministic scenario.
type Catalog struct {
	scenario     Scenario
	integrations []Integration
	devices      []Device
}

// ParseScenario applies the healthy default and returns the selected identifier.
func ParseScenario(value string) (Scenario, error) {
	switch Scenario(value) {
	case "", ScenarioHealthy:
		return ScenarioHealthy, nil
	case ScenarioUnavailable:
		return ScenarioUnavailable, nil
	default:
		return "", fmt.Errorf("unsupported simulation scenario %q", value)
	}
}

// NewCatalog constructs the deterministic inventory for scenario.
func NewCatalog(scenario Scenario) (Catalog, error) {
	if scenario != ScenarioHealthy && scenario != ScenarioUnavailable {
		return Catalog{}, fmt.Errorf("unsupported simulation scenario %q", scenario)
	}
	status := "healthy"
	deviceStatus := "ready"
	if scenario == ScenarioUnavailable {
		status = "unavailable"
		deviceStatus = "unavailable"
	}
	return Catalog{
		scenario: scenario,
		integrations: []Integration{{
			ID:          "simulation.fake-python",
			DisplayName: "SIMULATION - Fake Python Integration",
			Status:      status,
			Simulation:  true,
			Operations:  []string{"artifact", "crash", "echo", "fail", "sleep"},
		}},
		devices: []Device{{
			ID:            "simulation.fake-python.device.stable-001",
			IntegrationID: "simulation.fake-python",
			DisplayName:   "SIMULATION - Electronics Bench 001",
			Kind:          "simulated-electronics-bench",
			Status:        deviceStatus,
			Simulation:    true,
			Capabilities:  []string{"artifact", "crash", "echo", "fail", "sleep"},
		}},
	}, nil
}

// Scenario returns the stable identifier used to build the catalog.
func (catalog Catalog) Scenario() Scenario { return catalog.scenario }

// Integrations returns a deep copy of the ordered integration inventory.
func (catalog Catalog) Integrations() []Integration {
	result := make([]Integration, len(catalog.integrations))
	for index, integration := range catalog.integrations {
		result[index] = integration
		result[index].Operations = append([]string(nil), integration.Operations...)
	}
	return result
}

// Devices returns a deep copy of the ordered device inventory.
func (catalog Catalog) Devices() []Device {
	result := make([]Device, len(catalog.devices))
	for index, device := range catalog.devices {
		result[index] = device
		result[index].Capabilities = append([]string(nil), device.Capabilities...)
	}
	return result
}
