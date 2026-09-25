package network

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SimulatedChanger satisfies the connectivity contract without touching
// anything. It exists so the whole operation lifecycle — preview,
// acknowledgement, the confirmation window, rollback — is exercisable under the
// simulation profile, where changing the developer's own host would be a bug.
//
// Like the simulated provider, it holds no reference through which a host could
// be reached; it only counts what it was asked to do.
type SimulatedChanger struct {
	mutex   sync.Mutex
	applied []string
}

func NewSimulatedChanger() *SimulatedChanger { return &SimulatedChanger{} }

// Applied returns what the simulation was asked to change, in order. Nothing on
// the host changed; this is only what a real changer would have done.
func (changer *SimulatedChanger) Applied() []string {
	changer.mutex.Lock()
	defer changer.mutex.Unlock()
	return append([]string(nil), changer.applied...)
}

func (changer *SimulatedChanger) ArmRecovery(
	_ context.Context, operationID string, _ time.Duration,
) (Checkpoint, error) {
	return Checkpoint{ID: "simulated-" + operationID}, nil
}

func (changer *SimulatedChanger) Apply(_ context.Context, action Action, target string) error {
	changer.mutex.Lock()
	defer changer.mutex.Unlock()
	changer.applied = append(changer.applied, fmt.Sprintf("%s:%s", action, target))
	return nil
}

// Restore reports the previous state as observed, because in a simulation it
// never actually left it.
func (changer *SimulatedChanger) Restore(context.Context, Checkpoint) (bool, error) {
	return true, nil
}

func (changer *SimulatedChanger) Discard(context.Context, Checkpoint) error { return nil }

// ProviderObserver reports the state a preview is computed against, read from
// whatever provider the service was given.
//
// The values it returns go into the revision digest, so anything included here
// becomes something that can make a preview go stale. It deliberately reports
// the target's connection state rather than the whole world: an unrelated
// interface changing must not invalidate an agreed change.
type ProviderObserver struct{ provider Provider }

func NewProviderObserver(provider Provider) *ProviderObserver {
	return &ProviderObserver{provider: provider}
}

func (observer *ProviderObserver) Observe(
	ctx context.Context, action Action, target string,
) (map[string]string, error) {
	observed := map[string]string{"action": string(action), "target": target}

	switch action {
	case ActionInterfaceDisable, ActionInterfaceEnable:
		interfaces, err := observer.provider.Interfaces(ctx)
		if err != nil {
			return nil, err
		}
		for _, candidate := range interfaces {
			if candidate.ID != target {
				continue
			}
			if !candidate.Controllable {
				return nil, fmt.Errorf("%w: %s", ErrNotControllable, candidate.NotControllableReason)
			}
			observed["state"] = string(candidate.State)
			observed["kind"] = string(candidate.Kind)
			return observed, nil
		}
		return nil, fmt.Errorf("%w: no interface %q", ErrNotControllable, target)
	default:
		// Actions that do not name an interface carry no observed state beyond
		// the proposal itself; their staleness comes from the target existing.
		return observed, nil
	}
}
