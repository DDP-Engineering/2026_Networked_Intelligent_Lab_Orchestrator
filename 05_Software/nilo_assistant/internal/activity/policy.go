// Package activity is the shared history: audit and event records read through
// one filtered, paginated query, exported within bounds, and kept within an
// administrator's retention policy.
package activity

import (
	"errors"
	"fmt"
	"time"

	"nilo.local/nilo/internal/audit"
)

// RowOverheadBytes is the per-record allowance added to the payload size when
// usage is measured against a quota. The row also carries its envelope and
// index entries; this keeps the estimate honest without measuring the file.
const RowOverheadBytes = 256

// ErrInvalidPolicy refuses a policy that is not bounded on every category.
var ErrInvalidPolicy = errors.New("activity policy rejected")

// Policy is the administrator's retention target and quota per category.
// Retention is a target, not a promise: quota pressure shortens it, and the
// retained window says what is actually there.
type Policy struct {
	RetentionDays map[audit.Category]int   `json:"retention_days"`
	QuotaBytes    map[audit.Category]int64 `json:"quota_bytes"`
}

// DefaultPolicy is what ships: audit 90 days and events 14, with budgets sized
// for an initial Pi installation.
func DefaultPolicy() Policy {
	return Policy{
		RetentionDays: map[audit.Category]int{audit.CategoryAudit: 90, audit.CategoryEvent: 14},
		QuotaBytes:    map[audit.Category]int64{audit.CategoryAudit: 256 << 20, audit.CategoryEvent: 128 << 20},
	}
}

var categories = []audit.Category{audit.CategoryAudit, audit.CategoryEvent}

// Validate refuses a policy that leaves any category unbounded or absurd.
func (policy Policy) Validate() error {
	for _, category := range categories {
		days, ok := policy.RetentionDays[category]
		if !ok || days < 1 || days > 3650 {
			return fmt.Errorf("%w: %s retention must be between 1 and 3650 days", ErrInvalidPolicy, category)
		}
		quota, ok := policy.QuotaBytes[category]
		if !ok || quota < 1<<20 || quota > 16<<30 {
			return fmt.Errorf("%w: %s quota must be between 1 MiB and 16 GiB", ErrInvalidPolicy, category)
		}
	}
	if len(policy.RetentionDays) != len(categories) || len(policy.QuotaBytes) != len(categories) {
		return fmt.Errorf("%w: unknown category", ErrInvalidPolicy)
	}
	return nil
}

// Retention is the cutoff for one category at a moment.
func (policy Policy) Retention(category audit.Category, now time.Time) time.Time {
	return now.Add(-time.Duration(policy.RetentionDays[category]) * 24 * time.Hour)
}

// Window is what one category holds now.
type Window struct {
	Oldest time.Time
	Newest time.Time
	Count  int64
	Bytes  int64
}

// Watermark records what retention removed for one category, so a query for
// an earlier time can disclose the gap.
type Watermark struct {
	PurgedBefore time.Time
	// Reason is "age" or "quota".
	Reason    string
	UpdatedAt time.Time
}
