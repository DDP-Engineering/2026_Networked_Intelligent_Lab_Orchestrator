package diagnostics

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"nilo.local/nilo/internal/audit"
)

// memorySessions keeps sessions in memory.
type memorySessions struct {
	mu       sync.Mutex
	sessions map[string]Session
}

func newMemorySessions() *memorySessions { return &memorySessions{sessions: map[string]Session{}} }

func (store *memorySessions) SaveSession(_ context.Context, session Session) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.sessions[session.ID] = session
	return nil
}

func (store *memorySessions) Sessions(context.Context) ([]Session, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	sessions := make([]Session, 0, len(store.sessions))
	for _, session := range store.sessions {
		sessions = append(sessions, session)
	}
	return sessions, nil
}

// recordingJournal keeps every record it is handed.
type recordingJournal struct {
	mu      sync.Mutex
	records []audit.Record
}

func (journal *recordingJournal) Record(_ context.Context, record audit.Record) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	journal.records = append(journal.records, record)
	return nil
}

func (journal *recordingJournal) actions() []string {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	actions := make([]string, 0, len(journal.records))
	for _, record := range journal.records {
		actions = append(actions, record.Action)
	}
	return actions
}

func (journal *recordingJournal) count(category audit.Category) int {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	count := 0
	for _, record := range journal.records {
		if record.Category == category {
			count++
		}
	}
	return count
}

type fixture struct {
	store   *memorySessions
	journal *recordingJournal
	service *Service
	now     time.Time
	mono    time.Duration
	dropped int
}

func newFixture(t *testing.T, bootID string, store *memorySessions) *fixture {
	t.Helper()
	f := &fixture{store: store, journal: &recordingJournal{}, now: time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)}
	service, err := NewService(Options{
		Store: store, Audit: f.journal, BootID: bootID,
		Now:       func() time.Time { return f.now },
		Monotonic: func() time.Duration { return f.mono },
		Dropped:   func(count int) { f.dropped += count },
	})
	if err != nil {
		t.Fatal(err)
	}
	f.service = service
	return f
}

func (f *fixture) advance(d time.Duration) {
	f.now = f.now.Add(d)
	f.mono += d
}

var admin = Actor{Kind: audit.ActorKindUser, ID: "6f2c1c3e-0000-4000-8000-000000000001"}

// TestSessionRaisesTheScopeAndReturnsToNormal covers REQ-FUNC-35da79f1: a
// session raises exactly its scope and target for its minutes, a parent
// scope covers its subfunctions, stopping and expiring return to normal, and
// every transition is recorded.
func TestSessionRaisesTheScopeAndReturnsToNormal(t *testing.T) {
	f := newFixture(t, "boot-1", newMemorySessions())
	ctx := context.Background()
	if _, err := f.service.Start(ctx, admin, "network.wifi", "", 7); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("seven minutes accepted: %v", err)
	}
	if _, err := f.service.Start(ctx, admin, "kernel", "", 5); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("unknown scope accepted: %v", err)
	}
	session, err := f.service.Start(ctx, admin, "network", "eth0", 15)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if session.State != StateActive || !session.ExpiresAt.Equal(f.now.Add(15*time.Minute)) || session.BootID != "boot-1" {
		t.Fatalf("session = %+v", session)
	}
	if f.service.Effective("network.interfaces", "eth0") != LevelDebug || f.service.Effective("network", "eth0") != LevelDebug {
		t.Fatal("the parent scope does not cover its subfunction")
	}
	if f.service.Effective("network.interfaces", "wlan0") != LevelNormal || f.service.Effective("identity", "eth0") != LevelNormal {
		t.Fatal("the session leaked beyond its target or scope")
	}
	// A debug record is written only where the scope is raised.
	f.service.Debug(ctx, "network.interfaces", "eth0", "op-1", "interface.wait", map[string]string{"state": "activating"})
	f.service.Debug(ctx, "network.interfaces", "wlan0", "op-2", "interface.wait", nil)
	if f.journal.count(audit.CategoryDebug) != 1 {
		t.Fatalf("debug records = %d, want 1", f.journal.count(audit.CategoryDebug))
	}
	// Advancing the wall clock alone does not expire; monotonic time does.
	f.now = f.now.Add(time.Hour)
	if err := f.service.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	if f.service.Effective("network", "eth0") != LevelDebug {
		t.Fatal("a wall-clock jump expired the session")
	}
	f.mono += 16 * time.Minute
	if err := f.service.Expire(ctx); err != nil {
		t.Fatal(err)
	}
	if f.service.Effective("network", "eth0") != LevelNormal {
		t.Fatal("the session did not expire")
	}
	expired, err := f.service.Get(ctx, session.ID)
	if err != nil || expired.State != StateExpired || expired.EndedAt == nil {
		t.Fatalf("expired session = %+v %v", expired, err)
	}
	// Stopping is explicit and recorded.
	again, err := f.service.Start(ctx, admin, "identity", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := f.service.Stop(ctx, admin, again.ID)
	if err != nil || stopped.State != StateStopped {
		t.Fatalf("stopped = %+v %v", stopped, err)
	}
	if _, err := f.service.Stop(ctx, admin, again.ID); !errors.Is(err, ErrSessionNotActive) {
		t.Fatalf("stopping twice: %v", err)
	}
	if _, err := f.service.Get(ctx, "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing: %v", err)
	}
	actions := f.journal.actions()
	want := []string{"diagnostics.session.started", "interface.wait", "diagnostics.session.expired", "diagnostics.session.started", "diagnostics.session.stopped"}
	if len(actions) != len(want) {
		t.Fatalf("actions = %v, want %v", actions, want)
	}
	for index := range want {
		if actions[index] != want[index] {
			t.Fatalf("actions = %v, want %v", actions, want)
		}
	}
}

// TestOverlapReplacesAndRestartInterrupts: one effective session per scope
// and target - a new one replaces the old and says so - and a restart never
// carries a session over; it is recorded as interrupted.
func TestOverlapReplacesAndRestartInterrupts(t *testing.T) {
	store := newMemorySessions()
	f := newFixture(t, "boot-1", store)
	ctx := context.Background()
	first, err := f.service.Start(ctx, admin, "network.wireguard", "office", 5)
	if err != nil {
		t.Fatal(err)
	}
	f.advance(time.Second)
	second, err := f.service.Start(ctx, admin, "network.wireguard", "office", 60)
	if err != nil {
		t.Fatal(err)
	}
	replaced, _ := f.service.Get(ctx, first.ID)
	if replaced.State != StateReplaced || second.State != StateActive {
		t.Fatalf("first = %+v second = %+v", replaced, second)
	}
	sessions, err := f.service.List(ctx)
	if err != nil || len(sessions) != 2 || sessions[0].ID != second.ID {
		t.Fatalf("list = %+v %v", sessions, err)
	}
	if _, err := f.service.Start(ctx, admin, "network.wireguard", "", 5); err != nil {
		t.Fatal(err)
	}
	if f.service.Effective("network.wireguard", "office") != LevelDebug || f.service.Effective("network.wireguard", "other") != LevelDebug {
		t.Fatal("scope-wide session missing")
	}
	// Restart with the same store and a new boot.
	restarted := newFixture(t, "boot-2", store)
	if err := restarted.service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if restarted.service.Effective("network.wireguard", "office") != LevelNormal {
		t.Fatal("a session survived the restart")
	}
	after, _ := restarted.service.Get(ctx, second.ID)
	if after.State != StateInterrupted {
		t.Fatalf("after restart = %+v", after)
	}
	interrupted := 0
	for _, action := range restarted.journal.actions() {
		if action == "diagnostics.session.interrupted" {
			interrupted++
		}
	}
	if interrupted != 2 {
		t.Fatalf("interrupted records = %d, want 2", interrupted)
	}
}

// TestDebugIsRateLimitedAndDropsAreCounted: a burst beyond the bound is
// dropped and counted rather than written or silently lost.
func TestDebugIsRateLimitedAndDropsAreCounted(t *testing.T) {
	f := newFixture(t, "boot-1", newMemorySessions())
	ctx := context.Background()
	if _, err := f.service.Start(ctx, admin, "system", "", 5); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < DebugBurst+25; index++ {
		f.service.Debug(ctx, "system", "", "", "tick", nil)
	}
	if got := f.journal.count(audit.CategoryDebug); got != DebugBurst {
		t.Fatalf("written = %d, want %d", got, DebugBurst)
	}
	if f.dropped != 25 {
		t.Fatalf("dropped = %d, want 25", f.dropped)
	}
	f.advance(2 * time.Second)
	f.service.Debug(ctx, "system", "", "", "tick", nil)
	if got := f.journal.count(audit.CategoryDebug); got != DebugBurst+1 {
		t.Fatalf("the bucket did not refill: %d", got)
	}
}
