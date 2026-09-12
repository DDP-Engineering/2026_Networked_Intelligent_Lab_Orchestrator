package network

import (
	"fmt"
	"strings"
	"sync"
)

// AdapterRole is the exclusive purpose of one physical network adapter.
type AdapterRole string

const (
	AdapterRoleManagement AdapterRole = "management"
	AdapterRoleDevice     AdapterRole = "device"
)

// AdapterRoleClaim is safe role ownership metadata. AdapterID is empty for a
// virtual or unresolved interface, which never participates in physical role
// exclusion.
type AdapterRoleClaim struct {
	AdapterID        string
	Interface        string
	Role             AdapterRole
	ResourceID       string
	ResourceName     string
	ReservationOwner string
}

// AdapterRoleConflictError reports why one physical adapter cannot take a
// requested role. It deliberately carries no addressing or credential data.
type AdapterRoleConflictError struct {
	AdapterID               string      `json:"adapter_id"`
	Interface               string      `json:"interface"`
	ExistingRole            AdapterRole `json:"existing_role"`
	RequestedRole           AdapterRole `json:"requested_role"`
	ConflictingResourceID   string      `json:"conflicting_resource_id"`
	ConflictingResourceName string      `json:"conflicting_resource_name"`
}

func (conflict *AdapterRoleConflictError) Error() string {
	adapter := conflict.Interface
	if adapter == "" {
		adapter = conflict.AdapterID
	}
	return fmt.Sprintf(
		"adapter %s is already assigned to the %s role and cannot be assigned to the %s role",
		adapter, conflict.ExistingRole, conflict.RequestedRole,
	)
}

type adapterRoleClaimSource func() []AdapterRoleClaim

// AdapterRoleCoordinator serializes physical role checks and tracks claims
// that must outlive one store mutation. It owns no persistence or privileges.
type AdapterRoleCoordinator struct {
	mu           sync.Mutex
	sources      []adapterRoleClaimSource
	reservations map[string][]AdapterRoleClaim
	// afterCheckAndRun is a package-test seam that runs after the coordinator
	// unlocks but before CheckAndRun returns. Production construction leaves it
	// nil; tests use it to prove publication happened inside the critical section.
	afterCheckAndRun func()
}

// NewAdapterRoleCoordinator creates an empty process-local coordinator.
func NewAdapterRoleCoordinator() *AdapterRoleCoordinator {
	return &AdapterRoleCoordinator{reservations: map[string][]AdapterRoleClaim{}}
}

// RegisterClaimSource adds a read-only durable-claim snapshot provider.
func (coordinator *AdapterRoleCoordinator) RegisterClaimSource(source func() []AdapterRoleClaim) {
	if source == nil {
		return
	}
	coordinator.mu.Lock()
	coordinator.sources = append(coordinator.sources, source)
	coordinator.mu.Unlock()
}

// CheckAndRun holds the coordinator across validation and the supplied
// mutation, preventing an opposite-role request from passing concurrently.
func (coordinator *AdapterRoleCoordinator) CheckAndRun(requested []AdapterRoleClaim, run func() error) error {
	coordinator.mu.Lock()
	defer func() {
		coordinator.mu.Unlock()
		if coordinator.afterCheckAndRun != nil {
			coordinator.afterCheckAndRun()
		}
	}()
	if err := coordinator.conflictLocked(requested, ""); err != nil {
		return err
	}
	return run()
}

// Check validates claims without retaining them or running a mutation.
func (coordinator *AdapterRoleCoordinator) Check(requested []AdapterRoleClaim) error {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	return coordinator.conflictLocked(requested, "")
}

// Conflicts returns every opposite-role collision in request order.
func (coordinator *AdapterRoleCoordinator) Conflicts(requested []AdapterRoleClaim) []*AdapterRoleConflictError {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	conflicts := make([]*AdapterRoleConflictError, 0)
	for _, claim := range requested {
		if err := coordinator.conflictLocked([]AdapterRoleClaim{claim}, ""); err != nil {
			if conflict, ok := err.(*AdapterRoleConflictError); ok {
				conflicts = append(conflicts, conflict)
			}
		}
	}
	return conflicts
}

// Reserve retains requested claims under owner until Release. Re-reserving an
// owner replaces only that owner's earlier reservation.
func (coordinator *AdapterRoleCoordinator) Reserve(owner string, requested []AdapterRoleClaim) error {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if err := coordinator.conflictLocked(requested, owner); err != nil {
		return err
	}
	claims := make([]AdapterRoleClaim, len(requested))
	copy(claims, requested)
	for index := range claims {
		claims[index].AdapterID = canonicalAdapterID(claims[index].AdapterID)
		claims[index].ReservationOwner = owner
	}
	coordinator.reservations[owner] = claims
	return nil
}

// Release drops all pending claims owned by one operation.
func (coordinator *AdapterRoleCoordinator) Release(owner string) {
	coordinator.mu.Lock()
	delete(coordinator.reservations, owner)
	coordinator.mu.Unlock()
}

func (coordinator *AdapterRoleCoordinator) conflictLocked(requested []AdapterRoleClaim, exceptOwner string) error {
	existing := make([]AdapterRoleClaim, 0)
	for _, source := range coordinator.sources {
		existing = append(existing, source()...)
	}
	for owner, claims := range coordinator.reservations {
		if owner != exceptOwner {
			existing = append(existing, claims...)
		}
	}
	for _, held := range existing {
		if canonicalAdapterID(held.AdapterID) != "" && !validAdapterRole(held.Role) {
			return fmt.Errorf("invalid adapter role %q", held.Role)
		}
	}
	for _, wanted := range requested {
		adapterID := canonicalAdapterID(wanted.AdapterID)
		if adapterID == "" {
			continue
		}
		if !validAdapterRole(wanted.Role) {
			return fmt.Errorf("invalid adapter role %q", wanted.Role)
		}
		for _, held := range existing {
			if canonicalAdapterID(held.AdapterID) != adapterID || held.Role == wanted.Role {
				continue
			}
			name := wanted.Interface
			if name == "" {
				name = held.Interface
			}
			return &AdapterRoleConflictError{
				AdapterID:               adapterID,
				Interface:               name,
				ExistingRole:            held.Role,
				RequestedRole:           wanted.Role,
				ConflictingResourceID:   held.ResourceID,
				ConflictingResourceName: held.ResourceName,
			}
		}
	}
	return nil
}

func validAdapterRole(role AdapterRole) bool {
	return role == AdapterRoleManagement || role == AdapterRoleDevice
}

func canonicalAdapterID(adapterID string) string {
	return strings.ToLower(strings.TrimSpace(adapterID))
}
