package main

import (
	"context"
	"fmt"
	"nilo.local/nilo/internal/network"
	"time"
)

type networkProvider interface {
	network.Provider
	network.CounterSource
	network.ConnectivityChanger
	network.StateObserver
}

// secretStagerFor returns the provider's own key-staging path when it has one.
// A provider without one is a provider that manages no real tunnel, and it is
// given no way to retain key material rather than a place to put it.
func secretStagerFor(provider networkProvider) network.SecretStager {
	if stager, ok := provider.(network.SecretStager); ok {
		return stager
	}
	return nil
}

// recoveryObserverFor returns the provider's view of the helper's recovery
// record when it has one. The simulation has no helper and no record.
func recoveryObserverFor(provider networkProvider) network.RecoveryObserver {
	if observer, ok := provider.(network.RecoveryObserver); ok {
		return observer
	}
	return nil
}

func newNetworkProvider(ctx context.Context, profile, socket string) (networkProvider, error) {
	switch profile {
	case "simulation":
		return network.NewSimulation(time.Now), nil
	case "appliance":
		return newApplianceProvider(ctx, socket)
	default:
		return nil, fmt.Errorf("unsupported runtime profile %q", profile)
	}
}
