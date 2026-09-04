package network

import (
	"context"
	"errors"
	"testing"
)

// fixedProvider reports exactly what a test hands it, so an assertion about a
// reported field cannot accidentally be an assertion about the host.
type fixedProvider struct {
	interfaces []Interface
	source     Source
	err        error
	calls      int
}

func (provider *fixedProvider) Interfaces(context.Context) ([]Interface, error) {
	provider.calls++
	if provider.err != nil {
		return nil, provider.err
	}
	return provider.interfaces, nil
}

func (provider *fixedProvider) Source() Source { return provider.source }

func bench() Interface {
	return Interface{
		ID:               "enp1s0",
		Name:             "enp1s0",
		Kind:             KindEthernet,
		State:            StateConnected,
		IPv4:             []string{"192.168.10.24/24"},
		IPv6:             []string{"fd00::24/64"},
		LinkCapacityMbps: 1000,
		RXBytesPerSecond: 8192,
		TXBytesPerSecond: 2048,
		Controllable:     true,
	}
}

func TestStatusReportsEveryFieldOfEveryManageableInterface(t *testing.T) {
	t.Parallel()

	provider := &fixedProvider{interfaces: []Interface{bench()}, source: SourceSimulation}
	service, err := NewService(ServiceOptions{Provider: provider})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(status.Interfaces) != 1 {
		t.Fatalf("Status reported %d interfaces, want 1", len(status.Interfaces))
	}

	got, want := status.Interfaces[0], bench()
	if got.ID != want.ID || got.Name != want.Name || got.Kind != want.Kind || got.State != want.State {
		t.Fatalf("identity and state = %+v, want %+v", got, want)
	}
	if len(got.IPv4) != 1 || got.IPv4[0] != want.IPv4[0] {
		t.Fatalf("IPv4 = %v, want %v", got.IPv4, want.IPv4)
	}
	if len(got.IPv6) != 1 || got.IPv6[0] != want.IPv6[0] {
		t.Fatalf("IPv6 = %v, want %v", got.IPv6, want.IPv6)
	}
	if got.LinkCapacityMbps != want.LinkCapacityMbps {
		t.Fatalf("link capacity = %d, want %d", got.LinkCapacityMbps, want.LinkCapacityMbps)
	}
	if got.RXBytesPerSecond != want.RXBytesPerSecond || got.TXBytesPerSecond != want.TXBytesPerSecond {
		t.Fatalf("rates = %v/%v, want %v/%v",
			got.RXBytesPerSecond, got.TXBytesPerSecond, want.RXBytesPerSecond, want.TXBytesPerSecond)
	}
}

// The plan requires the measurement source to be explicit, so nobody reads a
// simulated figure as a bench observation.
func TestStatusNamesTheSourceThatMeasuredIt(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		source Source
	}{
		{name: "simulation", source: SourceSimulation},
		{name: "appliance host", source: Source("bench-pi")},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, err := NewService(ServiceOptions{
				Provider: &fixedProvider{interfaces: []Interface{bench()}, source: test.source},
			})
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			status, err := service.Status(context.Background())
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if status.MeasuredBy != test.source {
				t.Fatalf("MeasuredBy = %q, want %q", status.MeasuredBy, test.source)
			}
		})
	}
}

func TestNewServiceRequiresAProvider(t *testing.T) {
	t.Parallel()

	if _, err := NewService(ServiceOptions{}); err == nil {
		t.Fatal("NewService accepted options with no provider")
	}
}

func TestStatusReturnsTheProviderFailureRatherThanAnEmptyReading(t *testing.T) {
	t.Parallel()

	failure := errors.New("netlink unavailable")
	service, err := NewService(ServiceOptions{
		Provider: &fixedProvider{source: SourceSimulation, err: failure},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := service.Status(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Status error = %v, want it to wrap %v", err, failure)
	}
}
