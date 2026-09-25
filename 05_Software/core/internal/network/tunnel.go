package network

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"nilo.local/nilo/internal/audit"
)

// TunnelObservation is one WireGuard connection as the privileged side reports
// it: identity, whether it is up, and the public half of its configuration.
// It has no field that could hold a key, because it is built for listing.
type TunnelObservation struct {
	// ConnectionRef is the NetworkManager connection identity.
	ConnectionRef string `json:"connection_ref"`
	// Name is the connection's own label on the host, which for a NILO-owned
	// tunnel is derived from the profile identity rather than the operator's
	// name for it.
	Name string `json:"name"`
	// Interface is the kernel device while the tunnel is up, otherwise empty.
	Interface     string     `json:"interface"`
	Active        bool       `json:"active"`
	LastHandshake *time.Time `json:"last_handshake,omitempty"`
	// NILOOwned separates the tunnels this appliance created from ones that
	// were already on the host. An external tunnel is observable and never
	// acted on.
	NILOOwned bool    `json:"nilo_owned"`
	Profile   Profile `json:"profile"`
}

// TunnelDriver is what the profile lifecycle needs from the privileged side.
//
// CreateTunnel receives the staged key handle and binds it itself, atomically
// with creating the connection, so that key material is bound exactly once and
// on failure nothing is left on disk. The unprivileged side never reads a key
// back: there is no method here that could return one.
type TunnelDriver interface {
	CreateTunnel(ctx context.Context, record ProfileRecord, secretHandle string) (connectionRef string, err error)
	DeleteTunnel(ctx context.Context, profileID, connectionRef string) error
	Tunnels(ctx context.Context) ([]TunnelObservation, error)
	// ConnectTunnel brings a saved connection up without a confirmation
	// window. It exists for the startup selection, which the administrator
	// chose in advance and nobody is present to confirm.
	ConnectTunnel(ctx context.Context, connectionRef string) error
}

// StartupOutcome is what one startup attempt found and did.
type StartupOutcome struct {
	// Selected reports whether any profile is chosen for startup.
	Selected  bool
	ProfileID string
	Name      string
	// Connected reports that this attempt brought the selection up.
	Connected bool
	// Active reports that the selection is up after this attempt, whether or
	// not this attempt did it.
	Active bool
	// Waiting names the prerequisite that is not there yet, when nothing was
	// done for that reason.
	Waiting string
}

// StartupConnector connects the startup selection. Both runtime profiles
// implement it.
type StartupConnector interface {
	ConnectStartup(ctx context.Context) (StartupOutcome, error)
}

// StartupOptions configure the bounded startup runner.
type StartupOptions struct {
	Connector StartupConnector
	Audit     AuditJournal
	// Attempts bounds how many times a missing prerequisite or a failure is
	// retried before the runner gives up and records that it did. Zero means
	// the default.
	Attempts int
}

// defaultStartupAttempts, at the maintenance loop's thirty-second cadence, is
// twenty minutes of waiting for an uplink before the appliance stops trying and
// says so.
const defaultStartupAttempts = 40

// Startup connects the selected profile once the appliance can. It is driven
// by the runtime, first synchronously at start and then from the maintenance
// loop until it reports it is done.
type Startup struct {
	options StartupOptions

	mu       sync.Mutex
	done     bool
	attempts int
	lastWait string
}

// NewStartup builds a runner. A nil connector means there is nothing to
// connect, and the runner is done from the start.
func NewStartup(options StartupOptions) *Startup {
	if options.Attempts <= 0 {
		options.Attempts = defaultStartupAttempts
	}
	return &Startup{options: options, done: options.Connector == nil}
}

// Done reports whether nothing more will be attempted.
func (startup *Startup) Done() bool {
	startup.mu.Lock()
	defer startup.mu.Unlock()
	return startup.done
}

// Attempt tries once and reports whether the runner is finished. It is
// finished when nothing is selected, when the selection is up, or when the
// bounded retries are spent; a missing uplink or a failed connection is
// retried until then.
func (startup *Startup) Attempt(ctx context.Context) bool {
	startup.mu.Lock()
	defer startup.mu.Unlock()
	if startup.done {
		return true
	}
	outcome, err := startup.options.Connector.ConnectStartup(ctx)
	switch {
	case err != nil:
		startup.retryOrGiveUp(ctx, outcome, err.Error())
	case !outcome.Selected:
		startup.done = true
	case outcome.Connected:
		startup.done = true
		startup.record(ctx, outcome, audit.OutcomeApplied, audit.SeverityInfo, map[string]any{
			"attempts": startup.attempts + 1,
		})
	case outcome.Active:
		startup.done = true
	default:
		startup.retryOrGiveUp(ctx, outcome, outcome.Waiting)
	}
	return startup.done
}

// retryOrGiveUp counts one unsuccessful attempt. Only the final one is
// recorded, with the count, so an appliance waiting twenty minutes for a
// hotspot writes one event rather than forty.
func (startup *Startup) retryOrGiveUp(ctx context.Context, outcome StartupOutcome, reason string) {
	startup.attempts++
	startup.lastWait = reason
	if startup.attempts < startup.options.Attempts {
		return
	}
	startup.done = true
	startup.record(ctx, outcome, audit.OutcomeFailed, audit.SeverityWarning, map[string]any{
		"attempts": startup.attempts, "reason": reason, "suppressed": startup.attempts - 1,
	})
}

func (startup *Startup) record(ctx context.Context, outcome StartupOutcome, result audit.Outcome, severity audit.Severity, details map[string]any) {
	if startup.options.Audit == nil {
		return
	}
	subject := outcome.ProfileID
	if subject == "" {
		subject = "startup"
	}
	_ = startup.options.Audit.Record(ctx, audit.Record{
		Category: audit.CategoryAudit, Component: "network.wireguard", Action: "vpn.startup",
		SubjectType: "vpn_profile", SubjectID: subject, TargetLabel: outcome.Name,
		ActorKind: audit.ActorKindSystem, Outcome: result, Severity: severity, Details: details,
	})
}

// ErrStartupUnavailable means the runtime cannot connect a startup selection
// because nothing that could is configured.
var ErrStartupUnavailable = errors.New("startup connection is not available on this runtime")

// startupWaiting builds the waiting outcome for one missing prerequisite.
func startupWaiting(record ProfileRecord, reason string) StartupOutcome {
	return StartupOutcome{Selected: true, ProfileID: record.ID, Name: record.Name, Waiting: reason}
}

// String renders the outcome for a log line without the profile's configuration.
func (outcome StartupOutcome) String() string {
	switch {
	case !outcome.Selected:
		return "no startup profile is selected"
	case outcome.Connected:
		return fmt.Sprintf("startup profile %s connected", outcome.Name)
	case outcome.Active:
		return fmt.Sprintf("startup profile %s is already active", outcome.Name)
	default:
		return fmt.Sprintf("startup profile %s is waiting: %s", outcome.Name, outcome.Waiting)
	}
}
