package activity

import (
	"context"
	"errors"
	"fmt"

	"nilo.local/nilo/internal/audit"
)

const (
	// purgeBatch and purgeBatches bound one sweep, so housekeeping never
	// holds the writer for long.
	purgeBatch   = 500
	purgeBatches = 10
)

// AuditJournal records the sweep itself.
type AuditJournal interface {
	Record(ctx context.Context, record audit.Record) error
}

// HousekeeperOptions are the sweeper's collaborators.
type HousekeeperOptions struct {
	Service *Service
	Audit   AuditJournal
	// Pending names the operations whose evidence must be kept until they are
	// reconciled, whatever their age. Optional.
	Pending func(ctx context.Context) ([]string, error)
}

// Housekeeper applies the policy in bounded batches.
type Housekeeper struct{ options HousekeeperOptions }

func NewHousekeeper(options HousekeeperOptions) (*Housekeeper, error) {
	if options.Service == nil {
		return nil, errors.New("housekeeper requires the activity service")
	}
	return &Housekeeper{options: options}, nil
}

// SweepReport is what one sweep removed and why.
type SweepReport struct {
	Removed map[audit.Category]int
	Reasons map[audit.Category]string
}

// Sweep purges by age, then by quota, records the watermarks and, when it
// removed anything, one operational event saying so.
func (keeper *Housekeeper) Sweep(ctx context.Context) (SweepReport, error) {
	service := keeper.options.Service
	report := SweepReport{Removed: map[audit.Category]int{}, Reasons: map[audit.Category]string{}}
	policy, err := service.Policy(ctx)
	if err != nil {
		return report, err
	}
	var keep []string
	if keeper.options.Pending != nil {
		if keep, err = keeper.options.Pending(ctx); err != nil {
			return report, err
		}
	}
	now := service.options.Now().UTC()
	store := service.options.Store
	for _, category := range categories {
		cutoff := policy.Retention(category, now)
		removed := 0
		for batch := 0; batch < purgeBatches; batch++ {
			count, err := store.PurgeExpired(ctx, category, cutoff, purgeBatch, keep)
			if err != nil {
				return report, err
			}
			removed += count
			if count < purgeBatch {
				break
			}
		}
		if removed > 0 {
			report.Removed[category] += removed
			report.Reasons[category] = "age"
			if err := store.SetWatermark(ctx, category, Watermark{PurgedBefore: cutoff, Reason: "age", UpdatedAt: now}); err != nil {
				return report, err
			}
		}
		for batch := 0; batch < purgeBatches; batch++ {
			windows, err := store.RetainedWindow(ctx)
			if err != nil {
				return report, err
			}
			if windows[category].Bytes <= policy.QuotaBytes[category] {
				break
			}
			count, newest, err := store.PurgeOldest(ctx, category, purgeBatch)
			if err != nil {
				return report, err
			}
			if count == 0 {
				break
			}
			report.Removed[category] += count
			report.Reasons[category] = "quota"
			if err := store.SetWatermark(ctx, category, Watermark{PurgedBefore: newest, Reason: "quota", UpdatedAt: now}); err != nil {
				return report, err
			}
		}
	}
	total := 0
	for _, count := range report.Removed {
		total += count
	}
	if total == 0 || keeper.options.Audit == nil {
		return report, nil
	}
	details := map[string]any{"removed": map[string]int{}, "reasons": map[string]string{}}
	for category, count := range report.Removed {
		details["removed"].(map[string]int)[string(category)] = count
		details["reasons"].(map[string]string)[string(category)] = report.Reasons[category]
	}
	err = keeper.options.Audit.Record(ctx, audit.Record{
		Category: audit.CategoryEvent, Component: "system", Action: "activity.retention.purged",
		SubjectType: "activity", SubjectID: "retention", ActorKind: audit.ActorKindSystem,
		Outcome: audit.OutcomeApplied, Severity: audit.SeverityInfo, Details: details,
	})
	if err != nil {
		return report, fmt.Errorf("record retention sweep: %w", err)
	}
	return report, nil
}
