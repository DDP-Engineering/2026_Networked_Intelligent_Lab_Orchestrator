package network

import (
	"context"
	"reflect"
	"testing"
)

func TestSimulatedProviderMeasuresAsSimulation(t *testing.T) {
	t.Parallel()

	if source := NewSimulatedProvider().Source(); source != SourceSimulation {
		t.Fatalf("simulated provider Source() = %q, want %q", source, SourceSimulation)
	}
}

func TestSimulatedProviderReturnsAStableDeterministicCatalog(t *testing.T) {
	t.Parallel()

	first, err := NewSimulatedProvider().Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces: %v", err)
	}
	second, err := NewSimulatedProvider().Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("simulated provider returned no interfaces")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("catalog is not deterministic:\nfirst  %+v\nsecond %+v", first, second)
	}
}

// A caller must be able to tell a device it may act on from one it may not, so
// the simulated catalog carries both and every refusal carries its reason.
func TestSimulatedCatalogMarksLoopbackNotControllableWithAReason(t *testing.T) {
	t.Parallel()

	interfaces, err := NewSimulatedProvider().Interfaces(context.Background())
	if err != nil {
		t.Fatalf("Interfaces: %v", err)
	}

	var sawLoopback, sawControllable bool
	for _, candidate := range interfaces {
		if candidate.Kind == KindLoopback {
			sawLoopback = true
			if candidate.Controllable {
				t.Fatalf("loopback device %q is controllable", candidate.ID)
			}
		}
		if candidate.Controllable {
			sawControllable = true
		}
		if !candidate.Controllable && candidate.NotControllableReason == "" {
			t.Fatalf("device %q is not controllable and says nothing about why", candidate.ID)
		}
		if candidate.Controllable && candidate.NotControllableReason != "" {
			t.Fatalf("device %q is controllable but carries a refusal reason", candidate.ID)
		}
	}
	if !sawLoopback {
		t.Fatal("simulated catalog has no loopback device to exercise the uncontrollable case")
	}
	if !sawControllable {
		t.Fatal("simulated catalog has no controllable device to exercise the ordinary case")
	}
}

// The simulation profile must never be able to change the host. The structural
// statement of that is that the simulated provider carries no reference of any
// kind - no helper client, no D-Bus connection, no command runner - through
// which a host could be reached.
func TestSimulatedProviderCarriesNoReferenceToAnything(t *testing.T) {
	t.Parallel()

	value := reflect.ValueOf(NewSimulatedProvider()).Elem()
	for index := 0; index < value.NumField(); index++ {
		field := value.Type().Field(index)
		switch field.Type.Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
			t.Fatalf("simulated provider field %q is a %s and could reach outside the catalog",
				field.Name, field.Type.Kind())
		}
	}
}
