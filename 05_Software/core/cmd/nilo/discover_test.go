package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"nilo.local/nilo/internal/discovery"
)

func discoveredFixtures() []discovery.Appliance {
	return []discovery.Appliance{{
		ID: "0199a2d0-1234-7abc-8def-0123456789ab", Name: "bench-west",
		Serial: "nilo-p2rjmzdkf1", FriendlyName: "bench-west", State: "commissioned", Version: "0.8.0",
		URL: "https://nilo-p2rjmzdkf1.local:8443/", Addresses: []string{"192.168.1.42"},
		HardwareAddresses: []string{"02:00:00:00:00:02"}, Sources: []discovery.Source{discovery.SourceMDNS, discovery.SourceSSDP},
	}}
}

func TestDiscoverCommandProducesStableJSONTableAndAtomicLabel(t *testing.T) {
	runner := func(context.Context, time.Duration) ([]discovery.Appliance, []string, error) {
		return discoveredFixtures(), []string{"bounded diagnostic"}, nil
	}
	for _, test := range []struct {
		name     string
		args     []string
		contains string
	}{
		{name: "table", contains: "bench-west  nilo-p2rjmzdkf1"},
		{name: "json", args: []string{"--json"}, contains: `"schema":1`},
		{name: "label", args: []string{"--label", "bench-west"}, contains: `<svg xmlns=`},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runDiscoverCommand(context.Background(), test.args, commandIO{stdout: &stdout, stderr: &stderr}, runner)
			if code != 0 || !strings.Contains(stdout.String(), test.contains) || !strings.Contains(stderr.String(), "bounded diagnostic") {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
		})
	}
	var stdout, stderr bytes.Buffer
	code := runDiscoverCommand(context.Background(), []string{"--json", "--label", "bench-west"}, commandIO{stdout: &stdout, stderr: &stderr}, runner)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("incompatible output code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestOpenAndServerResolutionNeverGuess(t *testing.T) {
	fixtures := discoveredFixtures()
	runner := func(context.Context, time.Duration) ([]discovery.Appliance, []string, error) {
		return fixtures, nil, nil
	}
	launched := []string{}
	launcher := func(value string) error { launched = append(launched, value); return nil }
	var stdout, stderr bytes.Buffer
	if code := runOpenCommand(context.Background(), []string{"bench-west"}, commandIO{stdout: &stdout, stderr: &stderr}, runner, launcher); code != 0 {
		t.Fatalf("open code=%d stderr=%s", code, stderr.String())
	}
	if len(launched) != 1 || launched[0] != fixtures[0].URL {
		t.Fatalf("launched=%v", launched)
	}
	launched = nil
	if code := runOpenCommand(context.Background(), []string{"https://192.0.2.1:8443"}, commandIO{stdout: &stdout, stderr: &stderr}, runner, launcher); code == 0 || len(launched) != 0 {
		t.Fatalf("explicit URL bypassed discovery: code=%d launched=%v", code, launched)
	}
	resolved, err := resolveDiscoveredServer(context.Background(), "nilo-p2rjmzdkf1", runner)
	if err != nil || resolved != fixtures[0].URL {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	if resolved, err = resolveDiscoveredServer(context.Background(), "https://192.0.2.1:8443", nil); err != nil || resolved != "https://192.0.2.1:8443" {
		t.Fatalf("explicit URL=%q err=%v", resolved, err)
	}
	fixtures = append(fixtures, discovery.Appliance{ID: "0299a2d0-1234-7abc-8def-0123456789ab", Serial: "nilo-0000000001", FriendlyName: "bench-west", URL: "https://nilo-0000000001.local/"})
	if _, err := resolveDiscoveredServer(context.Background(), "bench-west", runner); err == nil {
		t.Fatal("ambiguous friendly name resolved")
	}
}
