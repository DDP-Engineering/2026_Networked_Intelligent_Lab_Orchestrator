package network

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A saved VPN profile has two halves that live in two places. The public half -
// addresses, DNS, peers, allowed IPs, endpoints - is ordinary configuration and
// is stored in the appliance database. The private half is key material and is
// stored only by the privileged helper, referenced from here by an opaque
// identity. Keeping them apart is what makes a database backup, an export and a
// support bundle safe to hand over.

// ProviderWireGuard is the only VPN provider this milestone implements. It is a
// value rather than a plugin boundary, so adding a second one later is a change
// to one switch rather than to an architecture.
const ProviderWireGuard = "wireguard"

// maximumSavedProfiles bounds the inventory. An appliance that reaches it is
// misconfigured rather than busy.
const maximumSavedProfiles = 32

var (
	// ErrProfileNotFound is a profile identity nothing is saved under.
	ErrProfileNotFound = errors.New("wireguard_profile_not_found")
	// ErrProfileNameTaken refuses two profiles an operator could not tell apart.
	ErrProfileNameTaken = errors.New("wireguard_profile_name_taken")
	// ErrProfileReferenced refuses removing a profile something still depends
	// on. The dependency is named rather than cascaded, because a hidden cascade
	// deletes the operator's work without asking.
	ErrProfileReferenced = errors.New("wireguard_profile_referenced")
)

// ProfileRecord is one saved profile as the appliance stores it.
type ProfileRecord struct {
	ID       string
	Name     string
	Provider string
	// Profile is the parsed public configuration. The type has no field that
	// could hold key material, so a record built from it cannot carry one.
	Profile Profile
	// SecretRef is the opaque identity the privileged keystore knows the key
	// material by. It is not a secret and nothing can be derived from it.
	SecretRef string
	// ConnectionRef is the NetworkManager connection NILO created, empty until
	// one exists.
	ConnectionRef string

	Revision string
	// StartSelected is the configured intent to connect this profile when the
	// appliance starts. Zero or one profile carries it.
	StartSelected bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate refuses a record that could not be stored or read back honestly.
func (record ProfileRecord) Validate() error {
	switch {
	case strings.TrimSpace(record.ID) == "":
		return errors.New("a saved profile requires its identity")
	case strings.TrimSpace(record.Name) == "":
		return errors.New("a saved profile requires a name")
	case record.Provider != ProviderWireGuard:
		return fmt.Errorf("unsupported VPN provider %q", record.Provider)
	case strings.TrimSpace(record.SecretRef) == "":
		return errors.New("a saved profile requires the reference to its key material")
	case record.CreatedAt.IsZero() || record.UpdatedAt.IsZero():
		return errors.New("a saved profile requires its timestamps")
	}
	return nil
}

// ProfileStore is the saved profile inventory.
type ProfileStore interface {
	Save(ctx context.Context, record ProfileRecord) error
	List(ctx context.Context) ([]ProfileRecord, error)
	Rename(ctx context.Context, profileID, name string, at time.Time) (ProfileRecord, error)
	SelectStartup(ctx context.Context, profileID string, at time.Time) error
	ClearStartup(ctx context.Context, at time.Time) error
	Delete(ctx context.Context, profileID string) error
}

// ProfileDependency is one thing that would break if a profile were removed.
type ProfileDependency struct {
	Kind string
	ID   string
	Name string
}

// ValidateProfileRemoval refuses removing a profile that is in use, and names
// what is using it.
//
// A cascade is deliberately not offered: deleting an operator's device network
// because they deleted a tunnel is a decision they did not make.
func ValidateProfileRemoval(record ProfileRecord, active bool, dependencies []ProfileDependency) error {
	if active {
		return fmt.Errorf("%w: %s is the connected tunnel; disconnect it first", ErrProfileActive, record.Name)
	}
	if len(dependencies) == 0 {
		return nil
	}
	names := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		names = append(names, fmt.Sprintf("%s %q", dependency.Kind, dependency.Name))
	}
	return fmt.Errorf("%w: %s is still used by %s", ErrProfileReferenced, record.Name, strings.Join(names, ", "))
}

// ValidateProfileImport refuses an import the inventory cannot take.
func ValidateProfileImport(name string, existing []ProfileRecord) error {
	if len(existing) >= maximumSavedProfiles {
		return fmt.Errorf("at most %d saved profiles are supported", maximumSavedProfiles)
	}
	for _, record := range existing {
		if strings.EqualFold(record.Name, name) {
			return fmt.Errorf("%w: %s", ErrProfileNameTaken, name)
		}
	}
	return nil
}

// ActiveProfile returns the one connected NILO-managed tunnel, if there is one.
// Two would be a state this appliance cannot reason about, so finding two is
// reported rather than silently resolved.
func ActiveProfile(statuses []ProfileStatus) (ProfileStatus, bool, error) {
	var found ProfileStatus
	count := 0
	for _, status := range statuses {
		if status.Active && status.NILOOwned {
			found = status
			count++
		}
	}
	if count > 1 {
		return ProfileStatus{}, false, errors.New("more than one NILO-managed tunnel is connected")
	}
	return found, count == 1, nil
}
