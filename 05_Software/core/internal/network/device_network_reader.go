package network

import (
	"context"
	"fmt"
)

// DeviceNetworkReader is a provider that holds saved device networks. Both
// runtimes implement it over the shared manager.
type DeviceNetworkReader interface {
	DeviceNetworks(ctx context.Context) ([]DeviceNetworkStatus, error)
	DeviceNetwork(ctx context.Context, id string) (DeviceNetworkStatus, error)
	CheckDeviceNetwork(ctx context.Context, id string, probe DeviceNetworkProbe) (DeviceNetworkCheck, error)
	AccessInstructions(ctx context.Context, id string) (AccessInstructionSet, error)
}

func (service *Service) deviceNetworks() (DeviceNetworkReader, error) {
	reader, ok := service.provider.(DeviceNetworkReader)
	if !ok {
		return nil, fmt.Errorf("%w: this runtime has no device networks", ErrUnsupported)
	}
	return reader, nil
}

// DeviceNetworks lists the saved device networks with their observed state.
func (service *Service) DeviceNetworks(ctx context.Context) ([]DeviceNetworkStatus, error) {
	reader, err := service.deviceNetworks()
	if err != nil {
		return nil, err
	}
	statuses, err := reader.DeviceNetworks(ctx)
	if err != nil {
		return nil, err
	}
	if statuses == nil {
		statuses = []DeviceNetworkStatus{}
	}
	return statuses, nil
}

// DeviceNetwork reads one saved device network.
func (service *Service) DeviceNetwork(ctx context.Context, id string) (DeviceNetworkStatus, error) {
	reader, err := service.deviceNetworks()
	if err != nil {
		return DeviceNetworkStatus{}, err
	}
	return reader.DeviceNetwork(ctx, id)
}

// CheckDeviceNetwork runs the bounded connection check.
func (service *Service) CheckDeviceNetwork(ctx context.Context, id string, probe DeviceNetworkProbe) (DeviceNetworkCheck, error) {
	reader, err := service.deviceNetworks()
	if err != nil {
		return DeviceNetworkCheck{}, err
	}
	return reader.CheckDeviceNetwork(ctx, id, probe)
}

// AccessInstructions produces the copyable steps for one device network.
func (service *Service) AccessInstructions(ctx context.Context, id string) (AccessInstructionSet, error) {
	reader, err := service.deviceNetworks()
	if err != nil {
		return AccessInstructionSet{}, err
	}
	return reader.AccessInstructions(ctx, id)
}
