package session

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"nilo.local/nilo/internal/domain"
)

var (
	ErrSessionNotFound          = errors.New("session not found")
	ErrSessionAlreadyExists     = errors.New("session already exists")
	ErrSessionStateConflict     = errors.New("session state compare-and-swap conflict")
	ErrSessionTimeRegression    = errors.New("session update time precedes persisted state")
	ErrInvalidResourceClaim     = errors.New("invalid resource claim")
	ErrStartupReconciliationAPI = errors.New("repository does not support atomic startup reconciliation")
	ErrAtomicTerminalTransition = errors.New("repository does not support atomic terminal transition")
)

var resourceKindPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var stableDeviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

// ResourceBusyError identifies the current owner without disclosing the
// contested resource or any request parameters.
type ResourceBusyError struct {
	OwnerSessionID domain.SessionID
}

func (errorValue *ResourceBusyError) Error() string { return "requested resource is busy" }

// Repository owns durable session state and resource leases.
type Repository interface {
	Create(ctx context.Context, session domain.Session) error
	Get(ctx context.Context, id domain.SessionID) (domain.Session, error)
	Transition(ctx context.Context, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error)
	AcquireLeases(ctx context.Context, id domain.SessionID, resources []string, at time.Time) error
	ReleaseLeases(ctx context.Context, id domain.SessionID, at time.Time) error
	ListInterrupted(ctx context.Context) ([]domain.Session, error)
}

// StartupReconciler keeps crash recovery atomic in repository implementations
// that share the session, lease, and audit transaction boundary.
type StartupReconciler interface {
	ReconcileStartup(ctx context.Context, at time.Time) error
}

// TerminalTransitioner performs a terminal compare-and-swap and release of
// that session's active leases in one durable transaction.
type TerminalTransitioner interface {
	TransitionAndRelease(ctx context.Context, id domain.SessionID, from, to domain.SessionState, at time.Time) (domain.Session, error)
}

// NormalizeResourceClaims validates, canonicalizes, sorts, and deduplicates
// claims before an all-or-nothing lease acquisition.
func NormalizeResourceClaims(resources []string) ([]string, error) {
	normalized := make([]string, 0, len(resources))
	for _, resource := range resources {
		parts := strings.Split(resource, "/")
		if len(parts) != 3 || strings.TrimSpace(resource) != resource || !resourceKindPattern.MatchString(parts[1]) || !stableDeviceIDPattern.MatchString(parts[2]) {
			return nil, fmt.Errorf("%w: expected <integration-id>/<resource-kind>/<stable-device-id>", ErrInvalidResourceClaim)
		}
		integrationID, err := domain.ParseIntegrationID(parts[0])
		if err != nil || integrationID.String() == "00000000-0000-0000-0000-000000000000" {
			return nil, fmt.Errorf("%w: integration ID", ErrInvalidResourceClaim)
		}
		normalized = append(normalized, integrationID.String()+"/"+parts[1]+"/"+parts[2])
	}
	sort.Strings(normalized)
	return slices.Compact(normalized), nil
}
