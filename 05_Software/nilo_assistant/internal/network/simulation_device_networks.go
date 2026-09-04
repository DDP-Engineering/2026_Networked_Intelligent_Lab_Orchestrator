package network

import (
	"context"
	"net/netip"
	"sort"
	"sync"
)

// The simulation's device networks: the shared manager over an in-memory
// forwarder, so the whole decision surface - create, enable, waiting on a
// tunnel, conflicts, checks and instructions - is exercised without a host.

// memoryForwarder holds the installed plans by device network.
type memoryForwarder struct {
	mu    sync.Mutex
	plans map[string]ForwardingPlan
}

func newMemoryForwarder() *memoryForwarder {
	return &memoryForwarder{plans: map[string]ForwardingPlan{}}
}

func (forwarder *memoryForwarder) ApplyForwarding(_ context.Context, plan ForwardingPlan) error {
	forwarder.mu.Lock()
	defer forwarder.mu.Unlock()
	forwarder.plans[plan.DeviceNetworkID] = plan
	return nil
}

func (forwarder *memoryForwarder) RemoveForwarding(_ context.Context, id string) error {
	forwarder.mu.Lock()
	defer forwarder.mu.Unlock()
	delete(forwarder.plans, id)
	return nil
}

func (forwarder *memoryForwarder) AppliedForwarding(context.Context) ([]ForwardingPlan, error) {
	forwarder.mu.Lock()
	defer forwarder.mu.Unlock()
	plans := make([]ForwardingPlan, 0, len(forwarder.plans))
	for _, plan := range forwarder.plans {
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].DeviceNetworkID < plans[j].DeviceNetworkID })
	return plans, nil
}

func (forwarder *memoryForwarder) snapshot() map[string]ForwardingPlan {
	forwarder.mu.Lock()
	defer forwarder.mu.Unlock()
	copied := make(map[string]ForwardingPlan, len(forwarder.plans))
	for id, plan := range forwarder.plans {
		copied[id] = plan
	}
	return copied
}

func (forwarder *memoryForwarder) restore(saved map[string]ForwardingPlan) {
	forwarder.mu.Lock()
	defer forwarder.mu.Unlock()
	forwarder.plans = make(map[string]ForwardingPlan, len(saved))
	for id, plan := range saved {
		forwarder.plans[id] = plan
	}
}

// simulationEnvironment is the manager's view of the simulation. It reads the
// simulation without its lock, because the manager is only ever called from
// paths that hold it or from readers that do not need it.
type simulationEnvironment struct{ s *Simulation }

func (env simulationEnvironment) Interfaces(context.Context) ([]Interface, error) {
	return BindSettings(env.s.interfaces, env.s.settings).Interfaces, nil
}

func (env simulationEnvironment) SavedSettings(context.Context) ([]InterfaceSettings, error) {
	return append([]InterfaceSettings(nil), env.s.settings...), nil
}

func (env simulationEnvironment) Profiles(context.Context) ([]ProfileStatus, error) {
	profiles := make([]ProfileStatus, 0, len(env.s.profiles))
	for _, profile := range env.s.profiles {
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles, nil
}

// simulatedProbe answers as a device on an active network would: the target
// accepts when its network is applied, and is unknown otherwise.
func (s *Simulation) simulatedProbe(_ context.Context, target netip.Addr, tcpPort int) DeviceNetworkFact {
	for _, plan := range s.forwarder.snapshot() {
		if plan.Waiting {
			continue
		}
		for _, resource := range plan.Resources {
			if resource.Kind == ResourceForwardGrant && resource.Destination.Contains(target) {
				return DeviceNetworkFact{Result: FactPassed, Detail: "the simulated device accepted a connection"}
			}
		}
	}
	return DeviceNetworkFact{Result: FactUnknown, Detail: "the probe did not complete; this says nothing about whether the device is online"}
}

// UseAudit gives the runtime's own re-plans a journal. It is called before
// the device-network store is attached.
func (s *Simulation) UseAudit(journal AuditJournal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = journal
}

// UseDeviceNetworkStore attaches the saved device networks and builds the
// manager over the simulation's own forwarder.
func (s *Simulation) UseDeviceNetworkStore(ctx context.Context, store DeviceNetworkStore) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.forwarder == nil {
		s.forwarder = newMemoryForwarder()
	}
	if s.deviceNetworks == nil {
		manager, err := NewDeviceNetworkManager(DeviceNetworkManagerOptions{
			Forwarder: s.forwarder, Environment: simulationEnvironment{s: s}, Now: s.now,
			NewID:  func() string { return "sim-dn-" + randomIdentity()[:8] },
			Prober: s.simulatedProbe, Audit: s.audit,
		})
		if err != nil {
			return err
		}
		s.deviceNetworks = manager
	}
	if err := s.deviceNetworks.UseStore(ctx, store); err != nil {
		return err
	}
	// Enabled networks are applied at startup when their prerequisites hold,
	// and wait with their guards closed otherwise.
	return s.deviceNetworks.Reconcile(ctx)
}

// DeviceNetworks lists the saved device networks with their observed state.
func (s *Simulation) DeviceNetworks(ctx context.Context) ([]DeviceNetworkStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expire()
	if s.deviceNetworks == nil {
		return []DeviceNetworkStatus{}, nil
	}
	return s.deviceNetworks.List(ctx)
}

// DeviceNetwork reads one saved device network.
func (s *Simulation) DeviceNetwork(ctx context.Context, id string) (DeviceNetworkStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deviceNetworks == nil {
		return DeviceNetworkStatus{}, ErrDeviceNetworkNotFound
	}
	return s.deviceNetworks.Get(ctx, id)
}

// AppliedForwarding is what the simulated host holds, for tests and status.
func (s *Simulation) AppliedForwarding() []ForwardingPlan {
	if s.forwarder == nil {
		return nil
	}
	plans, _ := s.forwarder.AppliedForwarding(context.Background())
	return plans
}

// CheckDeviceNetwork runs the bounded connection check.
func (s *Simulation) CheckDeviceNetwork(ctx context.Context, id string, probe DeviceNetworkProbe) (DeviceNetworkCheck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deviceNetworks == nil {
		return DeviceNetworkCheck{}, ErrDeviceNetworkNotFound
	}
	return s.deviceNetworks.Check(ctx, id, probe)
}

// AccessInstructions produces the copyable steps for one device network.
func (s *Simulation) AccessInstructions(ctx context.Context, id string) (AccessInstructionSet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deviceNetworks == nil {
		return AccessInstructionSet{}, ErrDeviceNetworkNotFound
	}
	return s.deviceNetworks.AccessInstructions(ctx, id)
}

// deviceNetworkDependencies feeds the profile removal check.
func (s *Simulation) deviceNetworkDependencies(ctx context.Context, profileID string) ([]ProfileDependency, error) {
	if s.deviceNetworks == nil {
		return nil, nil
	}
	return s.deviceNetworks.Dependencies(ctx, profileID)
}

// ReconcileDeviceNetworks re-plans on the runtime's maintenance cadence.
func (s *Simulation) ReconcileDeviceNetworks(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deviceNetworks == nil {
		return nil
	}
	return s.deviceNetworks.Reconcile(ctx)
}

// reconcileDeviceNetworks re-plans after anything that could change a
// prerequisite. Called with the lock held.
func (s *Simulation) reconcileDeviceNetworks(ctx context.Context) {
	if s.deviceNetworks != nil {
		_ = s.deviceNetworks.Reconcile(ctx)
	}
}

// snapshotDeviceNetworks captures the saved decisions and installed forwarding
// under a checkpoint. Called with the lock held.
func (s *Simulation) snapshotDeviceNetworks() {
	s.savedForwarding, s.savedNetworks = nil, nil
	if s.forwarder != nil {
		s.savedForwarding = s.forwarder.snapshot()
	}
	if s.deviceNetworks != nil {
		if records, err := s.deviceNetworks.records(context.Background()); err == nil {
			s.savedNetworks = records
		}
	}
}

// restoreDeviceNetworks puts the saved decisions and the installed forwarding
// back as they were when the checkpoint was armed. Called with the lock held.
func (s *Simulation) restoreDeviceNetworks(ctx context.Context) {
	if s.forwarder != nil && s.savedForwarding != nil {
		s.forwarder.restore(s.savedForwarding)
	}
	if s.deviceNetworks == nil || s.savedNetworks == nil {
		return
	}
	store, err := s.deviceNetworks.storeOrErr()
	if err != nil {
		return
	}
	saved := map[string]bool{}
	for _, record := range s.savedNetworks {
		saved[record.ID] = true
		_, _ = store.Save(ctx, record, record.UpdatedAt)
	}
	if current, err := store.List(ctx); err == nil {
		for _, record := range current {
			if !saved[record.ID] {
				_ = store.Delete(ctx, record.ID)
			}
		}
	}
}
