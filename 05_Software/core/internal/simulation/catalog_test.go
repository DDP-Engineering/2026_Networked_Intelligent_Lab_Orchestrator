package simulation

import (
	"reflect"
	"testing"
)

func TestParseScenarioDefaultsAndSelectsSupportedStates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		input string
		want  Scenario
	}{
		{input: "", want: ScenarioHealthy},
		{input: "healthy", want: ScenarioHealthy},
		{input: "unavailable", want: ScenarioUnavailable},
	} {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseScenario(test.input)
			if err != nil {
				t.Fatalf("ParseScenario(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("ParseScenario(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestParseScenarioRejectsUnsupportedIdentifier(t *testing.T) {
	t.Parallel()

	if _, err := ParseScenario("real-hardware"); err == nil {
		t.Fatal("ParseScenario accepted an unsupported identifier")
	}
}

func TestCatalogReturnsStableScenarioIntegrationInventory(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		scenario Scenario
		status   string
	}{
		{scenario: ScenarioHealthy, status: "healthy"},
		{scenario: ScenarioUnavailable, status: "unavailable"},
	} {
		test := test
		t.Run(string(test.scenario), func(t *testing.T) {
			t.Parallel()
			catalog, err := NewCatalog(test.scenario)
			if err != nil {
				t.Fatalf("NewCatalog(%q): %v", test.scenario, err)
			}
			want := []Integration{{
				ID:          "simulation.bench",
				DisplayName: "SIMULATION - Demo Integration",
				Status:      test.status,
				Simulation:  true,
				Operations:  []string{"artifact", "crash", "echo", "fail", "sleep"},
			}}
			first := catalog.Integrations()
			if !reflect.DeepEqual(first, want) {
				t.Fatalf("Integrations() = %#v, want %#v", first, want)
			}
			first[0].Operations[0] = "mutated"
			if got := catalog.Integrations(); !reflect.DeepEqual(got, want) {
				t.Fatalf("Integrations() after caller mutation = %#v, want %#v", got, want)
			}
		})
	}
}

func TestCatalogReturnsStableScenarioDeviceInventory(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		scenario Scenario
		status   string
	}{
		{scenario: ScenarioHealthy, status: "ready"},
		{scenario: ScenarioUnavailable, status: "unavailable"},
	} {
		test := test
		t.Run(string(test.scenario), func(t *testing.T) {
			t.Parallel()
			catalog, err := NewCatalog(test.scenario)
			if err != nil {
				t.Fatalf("NewCatalog(%q): %v", test.scenario, err)
			}
			want := []Device{{
				ID:            "simulation.bench.device.stable-001",
				IntegrationID: "simulation.bench",
				DisplayName:   "SIMULATION - Electronics Bench 001",
				Kind:          "simulated-electronics-bench",
				Status:        test.status,
				Simulation:    true,
				Capabilities:  []string{"artifact", "crash", "echo", "fail", "sleep"},
			}}
			first := catalog.Devices()
			if !reflect.DeepEqual(first, want) {
				t.Fatalf("Devices() = %#v, want %#v", first, want)
			}
			first[0].Capabilities[0] = "mutated"
			if got := catalog.Devices(); !reflect.DeepEqual(got, want) {
				t.Fatalf("Devices() after caller mutation = %#v, want %#v", got, want)
			}
		})
	}
}
