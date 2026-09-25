package network

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Provider is the one seam between this package and whatever it is looking at.
// The simulated provider answers from a fixed catalog and reaches nothing; the
// Linux provider answers from NetworkManager. A Service holds exactly one, so
// there is no runtime branch that could reach a host it was not given.
type Provider interface {
	// Interfaces returns every device the provider can see, including the ones
	// it will not let anybody control.
	Interfaces(ctx context.Context) ([]Interface, error)
	// Source names this provider as the measurer of its own readings.
	Source() Source
}

// ServiceOptions are the shared Network service's collaborators.
type ServiceOptions struct {
	// Provider is what the service is looking at. It is required, and there is
	// no default: the default would have to be one of the two profiles, and
	// choosing wrong means either a dead service or a live host.
	Provider Provider
	// History supplies persisted traffic aggregates. A service built without
	// one still answers status; it refuses statistics rather than inventing an
	// empty series that would read as "no traffic".
	History HistoryReader
	// Now supplies wall-clock time, used to place and age the statistics
	// window. It defaults to the system clock.
	Now func() time.Time
}

// Service is the shared Network service. The HTTP boundary, the command line
// and the read-only MCP endpoint all call this one object, so an answer cannot
// differ by the door it came through.
type Service struct {
	provider Provider
	history  HistoryReader
	now      func() time.Time
}

// NewService binds a service to its collaborators.
func NewService(options ServiceOptions) (*Service, error) {
	if options.Provider == nil {
		return nil, errors.New("network service requires a provider")
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{provider: options.Provider, history: options.History, now: now}, nil
}

// Status reads every interface the provider can see. A provider failure is
// returned as a failure: an empty reading would be indistinguishable from an
// appliance with no interfaces.
func (service *Service) Status(ctx context.Context) (Status, error) {
	interfaces, err := service.provider.Interfaces(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("read network interfaces: %w", err)
	}
	status := Status{Interfaces: interfaces, MeasuredBy: service.provider.Source(), Unattached: []InterfaceSettings{}}
	// Saved configuration is attached here, at the one place every reader
	// comes through, so the GUI, the CLI and the MCP tools all see the same
	// binding: which adapter a decision follows and whether it is in force.
	if reader, ok := service.provider.(SavedSettingsReader); ok {
		saved, err := reader.SavedSettings(ctx)
		if err != nil {
			return Status{}, fmt.Errorf("read saved interface settings: %w", err)
		}
		bound := BindSettings(interfaces, saved)
		status.Interfaces = bound.Interfaces
		if bound.Unattached != nil {
			status.Unattached = bound.Unattached
		}
	}
	return status, nil
}
