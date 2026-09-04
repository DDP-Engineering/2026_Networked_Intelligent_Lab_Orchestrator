// Package diagnostics is the timed per-function debug control: a session
// raises detail for one function scope and optional target for a bounded
// time, and the scope returns to normal when the session is stopped, when it
// expires by monotonic time, or when the appliance restarts. Nothing here can
// turn audit off, and nothing here reconfigures anything but NILO's own
// records.
package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/audit"
	"nilo.local/nilo/internal/domain"
)

// Scope is one product function whose detail can be raised.
type Scope string

// Scopes are fixed product functions, not code symbols. A parent covers its
// subfunctions.
var Scopes = []Scope{
	"network", "network.interfaces", "network.wifi", "network.wireguard",
	"network.device_networks", "network.recovery", "identity", "system",
}

// Level is the effective detail for a scope and target.
type Level string

const (
	LevelNormal Level = "normal"
	LevelDebug  Level = "debug"
)

// State is where one session has got to.
type State string

const (
	StateActive      State = "active"
	StateStopped     State = "stopped"
	StateExpired     State = "expired"
	StateReplaced    State = "replaced"
	StateInterrupted State = "interrupted"
)

// Durations are the offered session lengths, in minutes; the longest is the
// maximum.
var Durations = []int{5, 15, 60}

const (
	// DebugRate and DebugBurst bound debug records per scope per second.
	DebugRate  = 20
	DebugBurst = 50
)

var (
	ErrInvalidSession   = errors.New("diagnostic session rejected")
	ErrSessionNotFound  = errors.New("diagnostic session not found")
	ErrSessionNotActive = errors.New("diagnostic session is not active")
)

// Session is one timed debug request.
type Session struct {
	ID        string
	Scope     Scope
	Target    string
	ActorID   string
	Minutes   int
	StartedAt time.Time
	ExpiresAt time.Time
	BootID    string
	State     State
	EndedAt   *time.Time
	EndReason string
}

// Actor is who asked.
type Actor struct {
	Kind audit.ActorKind
	ID   string
}

// Store keeps sessions durable.
type Store interface {
	SaveSession(ctx context.Context, session Session) error
	Sessions(ctx context.Context) ([]Session, error)
}

// AuditJournal records the session lifecycle and the debug records.
type AuditJournal interface {
	Record(ctx context.Context, record audit.Record) error
}

// Options are the service's collaborators.
type Options struct {
	Store  Store
	Audit  AuditJournal
	BootID string
	Now    func() time.Time
	// Monotonic is time since boot by a clock nobody can set. Expiry is
	// decided by it, so a wall-clock adjustment can never extend a session.
	Monotonic func() time.Duration
	NewID     func() string
	// Dropped is told how many debug records the rate limit refused.
	Dropped func(count int)
}

type active struct {
	session   Session
	expiresAt time.Duration
}

// Service owns the sessions and the effective levels.
type Service struct {
	options Options
	mu      sync.Mutex
	active  map[string]*active
	buckets map[Scope]*bucket
}

var processStart = time.Now()

func NewService(options Options) (*Service, error) {
	if options.Store == nil || options.Audit == nil || options.BootID == "" || options.Now == nil {
		return nil, errors.New("diagnostics service requires a store, an audit journal, the boot identity and a clock")
	}
	if options.Monotonic == nil {
		options.Monotonic = func() time.Duration { return time.Since(processStart) }
	}
	if options.NewID == nil {
		options.NewID = uuid.NewString
	}
	if options.Dropped == nil {
		options.Dropped = func(int) {}
	}
	return &Service{options: options, active: map[string]*active{}, buckets: map[Scope]*bucket{}}, nil
}

func knownScope(scope Scope) bool {
	for _, candidate := range Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func knownDuration(minutes int) bool {
	for _, candidate := range Durations {
		if candidate == minutes {
			return true
		}
	}
	return false
}

func key(scope Scope, target string) string { return string(scope) + "\x00" + target }

func (service *Service) record(ctx context.Context, session Session, action string, actor Actor, outcome audit.Outcome, severity audit.Severity) error {
	record := audit.Record{
		Category: audit.CategoryAudit, Component: "system", Action: action,
		SubjectType: "diagnostic_session", SubjectID: session.ID, TargetLabel: string(session.Scope),
		ActorKind: actor.Kind, Outcome: outcome, Severity: severity,
		Details: map[string]any{"scope": string(session.Scope), "target": session.Target, "minutes": session.Minutes,
			"expires_at": session.ExpiresAt.UTC().Format(time.RFC3339), "state": string(session.State), "end_reason": session.EndReason},
	}
	if actor.Kind == audit.ActorKindUser || actor.Kind == audit.ActorKindToken {
		id, err := domain.ParseActorID(actor.ID)
		if err != nil {
			return fmt.Errorf("%w: actor identity", ErrInvalidSession)
		}
		record.ActorID = id
	}
	return service.options.Audit.Record(ctx, record)
}

// Start raises one scope and target for the requested minutes. A session
// already active for the same scope and target is replaced and says so.
func (service *Service) Start(ctx context.Context, actor Actor, scope Scope, target string, minutes int) (Session, error) {
	if !knownScope(scope) {
		return Session{}, fmt.Errorf("%w: unknown function scope %q", ErrInvalidSession, scope)
	}
	if !knownDuration(minutes) {
		return Session{}, fmt.Errorf("%w: duration must be 5, 15 or 60 minutes", ErrInvalidSession)
	}
	if len(target) > 200 || strings.ContainsAny(target, "\n\r\x00") {
		return Session{}, fmt.Errorf("%w: target", ErrInvalidSession)
	}
	now := service.options.Now().UTC()
	session := Session{
		ID: service.options.NewID(), Scope: scope, Target: target, ActorID: actor.ID, Minutes: minutes,
		StartedAt: now, ExpiresAt: now.Add(time.Duration(minutes) * time.Minute), BootID: service.options.BootID, State: StateActive,
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if previous, ok := service.active[key(scope, target)]; ok {
		if err := service.endLocked(ctx, previous, StateReplaced, "replaced by "+session.ID, actor); err != nil {
			return Session{}, err
		}
	}
	// The record comes before the effect, so a session nobody could account
	// for never raises anything.
	if err := service.record(ctx, session, "diagnostics.session.started", actor, audit.OutcomeApplied, audit.SeverityInfo); err != nil {
		return Session{}, err
	}
	if err := service.options.Store.SaveSession(ctx, session); err != nil {
		return Session{}, err
	}
	service.active[key(scope, target)] = &active{session: session, expiresAt: service.options.Monotonic() + time.Duration(minutes)*time.Minute}
	return session, nil
}

// endLocked closes one active session. Called with the lock held.
func (service *Service) endLocked(ctx context.Context, entry *active, state State, reason string, actor Actor) error {
	now := service.options.Now().UTC()
	entry.session.State, entry.session.EndedAt, entry.session.EndReason = state, &now, reason
	delete(service.active, key(entry.session.Scope, entry.session.Target))
	action := "diagnostics.session." + string(state)
	severity := audit.SeverityInfo
	if state == StateInterrupted {
		severity = audit.SeverityWarning
	}
	if err := service.record(ctx, entry.session, action, actor, audit.OutcomeApplied, severity); err != nil {
		return err
	}
	return service.options.Store.SaveSession(ctx, entry.session)
}

// Stop ends one session explicitly.
func (service *Service) Stop(ctx context.Context, actor Actor, id string) (Session, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	for _, entry := range service.active {
		if entry.session.ID == id {
			if err := service.endLocked(ctx, entry, StateStopped, "stopped by the operator", actor); err != nil {
				return Session{}, err
			}
			return entry.session, nil
		}
	}
	sessions, err := service.options.Store.Sessions(ctx)
	if err != nil {
		return Session{}, err
	}
	for _, session := range sessions {
		if session.ID == id {
			return Session{}, ErrSessionNotActive
		}
	}
	return Session{}, ErrSessionNotFound
}

// Expire ends every session whose monotonic deadline has passed. The runtime
// calls it on its cadence.
func (service *Service) Expire(ctx context.Context) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.options.Monotonic()
	system := Actor{Kind: audit.ActorKindSystem}
	for _, entry := range service.active {
		if now >= entry.expiresAt {
			if err := service.endLocked(ctx, entry, StateExpired, "expired", system); err != nil {
				return err
			}
		}
	}
	return nil
}

// Reconcile runs at startup: a session from an earlier boot is recorded as
// interrupted, so the scope is normal and the record says why.
func (service *Service) Reconcile(ctx context.Context) error {
	sessions, err := service.options.Store.Sessions(ctx)
	if err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	system := Actor{Kind: audit.ActorKindSystem}
	for _, session := range sessions {
		if session.State != StateActive || session.BootID == service.options.BootID {
			continue
		}
		entry := &active{session: session}
		service.active[key(session.Scope, session.Target)] = entry
		if err := service.endLocked(ctx, entry, StateInterrupted, "the appliance restarted", system); err != nil {
			return err
		}
	}
	return nil
}

// Get reads one session.
func (service *Service) Get(ctx context.Context, id string) (Session, error) {
	service.mu.Lock()
	for _, entry := range service.active {
		if entry.session.ID == id {
			session := entry.session
			service.mu.Unlock()
			return session, nil
		}
	}
	service.mu.Unlock()
	sessions, err := service.options.Store.Sessions(ctx)
	if err != nil {
		return Session{}, err
	}
	for _, session := range sessions {
		if session.ID == id {
			return session, nil
		}
	}
	return Session{}, ErrSessionNotFound
}

// List reads every session, newest first.
func (service *Service) List(ctx context.Context) ([]Session, error) {
	sessions, err := service.options.Store.Sessions(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].StartedAt.After(sessions[j].StartedAt) })
	if sessions == nil {
		sessions = []Session{}
	}
	return sessions, nil
}

// Effective is the level for one scope and target now.
func (service *Service) Effective(scope Scope, target string) Level {
	service.mu.Lock()
	defer service.mu.Unlock()
	for _, entry := range service.active {
		if entry.session.Scope != scope && !strings.HasPrefix(string(scope), string(entry.session.Scope)+".") {
			continue
		}
		if entry.session.Target != "" && entry.session.Target != target {
			continue
		}
		return LevelDebug
	}
	return LevelNormal
}

// Debug writes one debug record where the scope is raised. It is silent
// otherwise, and bounded where it is not: beyond the rate the record is
// dropped and counted.
func (service *Service) Debug(ctx context.Context, scope Scope, target, operationID, code string, details any) {
	if service.Effective(scope, target) != LevelDebug {
		return
	}
	service.mu.Lock()
	limiter, ok := service.buckets[scope]
	if !ok {
		limiter = &bucket{tokens: DebugBurst, last: service.options.Monotonic()}
		service.buckets[scope] = limiter
	}
	allowed := limiter.take(service.options.Monotonic())
	service.mu.Unlock()
	if !allowed {
		service.options.Dropped(1)
		return
	}
	_ = service.options.Audit.Record(ctx, audit.Record{
		Category: audit.CategoryDebug, Component: string(scope), Action: code,
		SubjectType: "target", SubjectID: target, ActorKind: audit.ActorKindSystem,
		Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo, OperationID: operationID, Details: details,
	})
}

// Trace is Debug with the scope as a string, which is how the services that
// know nothing of this package hand their detail over.
func (service *Service) Trace(ctx context.Context, scope, target, operationID, code string, details any) {
	service.Debug(ctx, Scope(scope), target, operationID, code, details)
}

// bucket is a token bucket over monotonic time.
type bucket struct {
	tokens float64
	last   time.Duration
}

func (limiter *bucket) take(now time.Duration) bool {
	elapsed := now - limiter.last
	if elapsed > 0 {
		limiter.tokens += elapsed.Seconds() * DebugRate
		if limiter.tokens > DebugBurst {
			limiter.tokens = DebugBurst
		}
		limiter.last = now
	}
	if limiter.tokens < 1 {
		return false
	}
	limiter.tokens--
	return true
}
