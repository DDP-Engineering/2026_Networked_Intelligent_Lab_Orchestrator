package activity

import (
	"context"
	"sync"
	"time"

	"nilo.local/nilo/internal/audit"
)

// Health is whether the appliance can still record what it does. An audit
// store that cannot be written is degraded: reads, established forwarding and
// recovery continue, but no new attributable change is made.
type Health struct {
	AuditWritable bool
	Failures      int64
	LastFailure   string
	LastFailureAt time.Time
	LastSuccessAt time.Time
	// DroppedDebug counts debug records the rate limit refused, so a bounded
	// debug log never claims to be complete.
	DroppedDebug int64
}

// Monitor wraps the audit repository and watches every append.
type Monitor struct {
	inner audit.Repository
	now   func() time.Time

	mu     sync.Mutex
	health Health
}

// NewMonitor wraps a repository.
func NewMonitor(inner audit.Repository, now func() time.Time) *Monitor {
	return &Monitor{inner: inner, now: now, health: Health{AuditWritable: true}}
}

func (monitor *Monitor) Append(ctx context.Context, event audit.Event) error {
	err := monitor.inner.Append(ctx, event)
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	if err != nil {
		monitor.health.AuditWritable = false
		monitor.health.Failures++
		monitor.health.LastFailure = sanitizeMessage(err.Error())
		monitor.health.LastFailureAt = monitor.now().UTC()
		return err
	}
	monitor.health.AuditWritable = true
	monitor.health.LastSuccessAt = monitor.now().UTC()
	return nil
}

func (monitor *Monitor) List(ctx context.Context, filter audit.Filter) ([]audit.Event, error) {
	return monitor.inner.List(ctx, filter)
}

func (monitor *Monitor) PurgeBefore(ctx context.Context, category audit.Category, cutoff time.Time, limit int) (int, error) {
	return monitor.inner.PurgeBefore(ctx, category, cutoff, limit)
}

// DroppedDebug adds to the count of refused debug records.
func (monitor *Monitor) DroppedDebug(count int) {
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	monitor.health.DroppedDebug += int64(count)
}

// Health is a snapshot.
func (monitor *Monitor) Health() Health {
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	return monitor.health
}

var _ audit.Repository = (*Monitor)(nil)
