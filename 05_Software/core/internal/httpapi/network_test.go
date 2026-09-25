package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"nilo.local/nilo/internal/auth"
	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/simulation"
	"nilo.local/nilo/internal/store/sqlite"
)

// Network telemetry is readable by every role, because an operator who cannot
// see whether the bench is connected cannot do their job. Changing the network
// is not, and that separation is proven here at the route rather than in the
// matrix, because the route is what an attacker reaches.
func TestNetworkStatusReportsGateways(t *testing.T) {
	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "gateways", nil, nil)
	response := f.get("/api/v1/network/status", bearer)
	var body struct {
		Interfaces []map[string]any `json:"interfaces"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Interfaces) == 0 {
		t.Fatal("no interfaces")
	}
	if body.Interfaces[0]["gateway4"] != "192.168.10.1" || body.Interfaces[0]["gateway6"] != "fd00:10::1" {
		t.Fatalf("missing gateways: %v", body.Interfaces[0])
	}
	if body.Interfaces[2]["gateway4"] != "" || body.Interfaces[2]["gateway6"] != "" {
		t.Fatal("loopback must have no gateway")
	}
}

func TestNetworkStatusIsReadableByEveryRole(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	f.createUser(t, "viewer", auth.RoleViewer, viewerPassword)

	for _, owner := range []string{"admin", "operator", "viewer"} {
		owner := owner
		t.Run(owner, func(t *testing.T) {
			bearer := f.issueToken(t, owner, "network-"+owner, nil, nil)
			response := f.get("/api/v1/network/status", bearer)
			if response.Code != http.StatusOK {
				t.Fatalf("%s reading network status = %d, want %d: %s",
					owner, response.Code, http.StatusOK, response.Body.String())
			}
		})
	}
}

func TestNetworkStatusRequiresACredential(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	if response := f.get("/api/v1/network/status", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous network status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

// A scope can only ever remove a permission, so a token narrowed away from
// network.read must be refused even though its owner is an administrator.
func TestNetworkStatusRefusesATokenNarrowedAwayFromNetworkRead(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	narrowed := f.issueToken(t, "admin", "status-only", []auth.Permission{auth.PermissionStatusRead}, nil)

	if response := f.get("/api/v1/network/status", narrowed); response.Code != http.StatusForbidden {
		t.Fatalf("narrowed token reading network status = %d, want %d: %s",
			response.Code, http.StatusForbidden, response.Body.String())
	}
	if response := f.get("/api/v1/status", narrowed); response.Code != http.StatusOK {
		t.Fatalf("narrowed token reading the status it does hold = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestNetworkStatusRefusesARevokedToken(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	issued, err := f.service.CreateToken(context.Background(), f.administrator, "admin", "revoked", nil, nil)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if response := f.get("/api/v1/network/status", issued.Secret); response.Code != http.StatusOK {
		t.Fatalf("token before revocation = %d, want %d", response.Code, http.StatusOK)
	}
	if err := f.service.RevokeToken(context.Background(), f.administrator, issued.Token.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if response := f.get("/api/v1/network/status", issued.Secret); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token reading network status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

// A demoted owner loses the permission immediately, because effective
// permissions are recomputed per request from the stored role.
func TestNetworkStatusFollowsTheOwnersCurrentState(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	f.createUser(t, "operator", auth.RoleOperator, operatorPassword)
	bearer := f.issueToken(t, "operator", "operator-network", nil, nil)

	if response := f.get("/api/v1/network/status", bearer); response.Code != http.StatusOK {
		t.Fatalf("enabled operator = %d, want %d", response.Code, http.StatusOK)
	}
	if err := f.service.SetEnabled(context.Background(), f.administrator, "operator", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if response := f.get("/api/v1/network/status", bearer); response.Code != http.StatusUnauthorized {
		t.Fatalf("disabled operator = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

// The statistics route answers only for a named interface over one of the three
// offered windows; anything else is a request the appliance cannot honestly
// serve, so it is refused rather than approximated.
func TestNetworkStatisticsRefusesAMissingInterfaceOrAnUnofferedPeriod(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "statistics", nil, nil)

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "no interface", path: "/api/v1/network/statistics?period=15m"},
		{name: "no period", path: "/api/v1/network/statistics?interface=sim-enp1s0"},
		{name: "unoffered period", path: "/api/v1/network/statistics?interface=sim-enp1s0&period=7d"},
		{name: "period is not a window", path: "/api/v1/network/statistics?interface=sim-enp1s0&period=90s"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if response := f.get(test.path, bearer); response.Code != http.StatusBadRequest {
				t.Fatalf("%s = %d, want %d: %s", test.path, response.Code, http.StatusBadRequest, response.Body.String())
			}
		})
	}
}

// A series that has stopped refreshing must say so. The stored minutes are old
// relative to the fixture clock, so both the envelope and every point are stale.
func TestNetworkStatisticsMarksAnUnrefreshedSeriesStale(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	repository := sqlite.NewNetworkRepository(f.database)
	// Both minutes sit well outside the freshness window but inside the hour,
	// which is what a series looks like after sampling has stopped.
	recent := f.clock.Now().UTC().Truncate(time.Minute).Add(-10 * time.Minute)
	old := recent.Add(-20 * time.Minute)
	for _, at := range []time.Time{old, recent} {
		if err := repository.WriteAggregate(context.Background(), network.Aggregate{
			InterfaceID: "sim-enp1s0", MinuteStart: at,
			RXBytes: 4_096, TXBytes: 1_024, Samples: 11, MeasuredBy: network.SourceSimulation,
		}); err != nil {
			t.Fatalf("WriteAggregate: %v", err)
		}
	}

	var body struct {
		Period     string `json:"period"`
		MeasuredBy string `json:"measured_by"`
		Stale      bool   `json:"stale"`
		Points     []struct {
			MinuteStart time.Time `json:"minute_start"`
			RXBytes     uint64    `json:"rx_bytes"`
			Samples     int       `json:"samples"`
			Stale       bool      `json:"stale"`
		} `json:"points"`
	}
	response := f.get("/api/v1/network/statistics?interface=sim-enp1s0&period=1h",
		f.issueToken(t, "admin", "statistics", nil, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("statistics = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode statistics: %v", err)
	}

	if body.Period != "1h" || body.MeasuredBy != "simulation" {
		t.Fatalf("period/measured_by = %q/%q, want 1h/simulation", body.Period, body.MeasuredBy)
	}
	if len(body.Points) != 2 {
		t.Fatalf("returned %d points, want the 2 stored minutes", len(body.Points))
	}
	if len(body.Points) > 60 {
		t.Fatalf("a one-hour window returned %d points", len(body.Points))
	}
	if !body.Stale {
		t.Fatal("a series whose newest minute has aged out is not reported stale")
	}
	for _, point := range body.Points {
		if point.MinuteStart.IsZero() {
			t.Fatal("a point carries no measurement time")
		}
		if point.Samples == 0 {
			t.Fatalf("point at %s claims no contributing sample", point.MinuteStart)
		}
	}
}

// A minute nobody measured stays absent. The route must not fill it with a zero.
func TestNetworkStatisticsLeavesUnmeasuredMinutesOutOfTheSeries(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	repository := sqlite.NewNetworkRepository(f.database)
	base := f.clock.Now().UTC().Truncate(time.Minute).Add(-10 * time.Minute)
	for _, offset := range []int{0, 4} {
		if err := repository.WriteAggregate(context.Background(), network.Aggregate{
			InterfaceID: "sim-enp1s0", MinuteStart: base.Add(time.Duration(offset) * time.Minute),
			RXBytes: 2_048, TXBytes: 512, Samples: 9, MeasuredBy: network.SourceSimulation,
		}); err != nil {
			t.Fatalf("WriteAggregate: %v", err)
		}
	}

	response := f.get("/api/v1/network/statistics?interface=sim-enp1s0&period=15m",
		f.issueToken(t, "admin", "statistics", nil, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("statistics = %d, want %d", response.Code, http.StatusOK)
	}
	var body struct {
		Points []struct {
			RXBytes uint64 `json:"rx_bytes"`
		} `json:"points"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode statistics: %v", err)
	}
	if len(body.Points) != 2 {
		t.Fatalf("returned %d points, want exactly the 2 measured minutes with the 3 unmeasured ones absent", len(body.Points))
	}
	for _, point := range body.Points {
		if point.RXBytes == 0 {
			t.Fatal("an unmeasured minute was filled in as zero traffic")
		}
	}
}

// The reading itself has to name every field an operator would act on, and say
// what measured it, so a simulated figure is never mistaken for the bench.
func TestNetworkStatusReportsTheSimulatedCatalogAndNamesItsSource(t *testing.T) {
	t.Parallel()

	f := newAPIFixture(t, simulation.ScenarioHealthy, CookieDevelopmentLocalhost)
	bearer := f.issueToken(t, "admin", "network", nil, nil)

	response := f.get("/api/v1/network/status", bearer)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	var body struct {
		APIVersion string `json:"api_version"`
		MeasuredBy string `json:"measured_by"`
		Interfaces []struct {
			ID                    string   `json:"id"`
			Name                  string   `json:"name"`
			Kind                  string   `json:"kind"`
			State                 string   `json:"state"`
			IPv4                  []string `json:"ipv4"`
			IPv6                  []string `json:"ipv6"`
			LinkCapacityMbps      uint64   `json:"link_capacity_mbps"`
			RXBytesPerSecond      float64  `json:"rx_bytes_per_second"`
			TXBytesPerSecond      float64  `json:"tx_bytes_per_second"`
			Controllable          bool     `json:"controllable"`
			NotControllableReason string   `json:"not_controllable_reason"`
		} `json:"interfaces"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode network status: %v", err)
	}
	if body.APIVersion != apiVersion {
		t.Fatalf("api_version = %q, want %q", body.APIVersion, apiVersion)
	}
	if body.MeasuredBy != "simulation" {
		t.Fatalf("measured_by = %q, want %q", body.MeasuredBy, "simulation")
	}
	if len(body.Interfaces) == 0 {
		t.Fatal("network status reported no interfaces")
	}

	var wired, loopback bool
	for _, candidate := range body.Interfaces {
		if candidate.ID == "" || candidate.Name == "" || candidate.Kind == "" || candidate.State == "" {
			t.Fatalf("interface %+v is missing part of its identity or state", candidate)
		}
		if candidate.Kind == "loopback" {
			loopback = true
			if candidate.Controllable || candidate.NotControllableReason == "" {
				t.Fatalf("loopback interface %q is offered for control", candidate.ID)
			}
		}
		if candidate.ID == "sim-enp1s0" {
			wired = true
			if len(candidate.IPv4) == 0 || len(candidate.IPv6) == 0 {
				t.Fatalf("wired interface reports no addresses: %+v", candidate)
			}
			if candidate.LinkCapacityMbps == 0 {
				t.Fatal("connected wired interface reports no link capacity")
			}
			if candidate.RXBytesPerSecond == 0 || candidate.TXBytesPerSecond == 0 {
				t.Fatal("connected wired interface reports no traffic rates")
			}
		}
	}
	if !wired || !loopback {
		t.Fatalf("status did not carry both the wired and the loopback device: %+v", body.Interfaces)
	}
}
