package network

import (
	"context"
	"fmt"
	"time"
)

// StaleAfter is how far behind the clock the newest observation may fall before
// a series stops being presented as current.
//
// Two minutes is one closed minute plus one in progress: a healthy sampler is
// always inside it, and a sampler that has stopped falls out of it within one
// refresh. The point is that a refresh failure shows as stale rather than as an
// unchanging chart that still looks live.
const StaleAfter = 2 * time.Minute

// Period is one of the three windows the interface offers.
type Period string

const (
	Period15m Period = "15m"
	Period1h  Period = "1h"
	Period24h Period = "24h"
)

var periodDurations = map[Period]time.Duration{
	Period15m: 15 * time.Minute,
	Period1h:  time.Hour,
	Period24h: 24 * time.Hour,
}

// ParsePeriod accepts exactly the three offered windows. An arbitrary duration
// is refused rather than honoured, because the retention window only supports
// these and a longer one would silently return a truncated answer.
func ParsePeriod(value string) (Period, error) {
	period := Period(value)
	if _, known := periodDurations[period]; !known {
		return "", fmt.Errorf("unsupported statistics period %q", value)
	}
	return period, nil
}

// Duration is how far back the period reaches. An unknown period has none.
func (period Period) Duration() time.Duration { return periodDurations[period] }

// StatisticsPoint is one measured minute.
type StatisticsPoint struct {
	MinuteStart time.Time

	RXBytes uint64
	TXBytes uint64

	RXErrors  uint64
	TXErrors  uint64
	RXDropped uint64
	TXDropped uint64

	// Samples is how many five-second intervals composed this minute, so a
	// partial minute is visibly weaker evidence than a complete one.
	Samples int
	// Stale marks a point that has aged out of the freshness window.
	Stale bool
}

// Statistics is one bounded answer about one interface.
type Statistics struct {
	InterfaceID string
	Period      Period
	From        time.Time
	To          time.Time
	MeasuredBy  Source

	// Stale is true when the series as a whole can no longer be presented as
	// current, including when it is empty: a window with no observation at all
	// is precisely the case where old values must not look live.
	Stale  bool
	Points []StatisticsPoint
}

// HistoryReader returns the stored minutes of one interface within [from, to).
type HistoryReader interface {
	History(ctx context.Context, interfaceID string, from, to time.Time) ([]Aggregate, error)
}

// Statistics reads one interface's traffic over one of the offered windows.
//
// The answer is bounded by construction: the window is exactly the period long
// and the store holds at most one row per interface-minute, so a caller cannot
// receive more points than the period has minutes.
func (service *Service) Statistics(
	ctx context.Context, interfaceID string, period Period,
) (Statistics, error) {
	if interfaceID == "" {
		return Statistics{}, fmt.Errorf("statistics require an interface identity")
	}
	if _, known := periodDurations[period]; !known {
		return Statistics{}, fmt.Errorf("unsupported statistics period %q", period)
	}
	if service.history == nil {
		return Statistics{}, fmt.Errorf("statistics require a history reader")
	}

	now := service.now().UTC()
	to := now.Truncate(time.Minute).Add(time.Minute)
	from := to.Add(-period.Duration())

	stored, err := service.history.History(ctx, interfaceID, from, to)
	if err != nil {
		return Statistics{}, fmt.Errorf("read traffic history: %w", err)
	}

	statistics := Statistics{
		InterfaceID: interfaceID, Period: period, From: from, To: to,
		MeasuredBy: service.provider.Source(),
		Points:     make([]StatisticsPoint, 0, len(stored)),
	}

	freshAfter := now.Add(-StaleAfter)
	newest := time.Time{}
	for _, aggregate := range stored {
		minuteStart := aggregate.MinuteStart.UTC()
		if minuteStart.After(newest) {
			newest = minuteStart
		}
		statistics.Points = append(statistics.Points, StatisticsPoint{
			MinuteStart: minuteStart,
			RXBytes:     aggregate.RXBytes, TXBytes: aggregate.TXBytes,
			RXErrors: aggregate.RXErrors, TXErrors: aggregate.TXErrors,
			RXDropped: aggregate.RXDropped, TXDropped: aggregate.TXDropped,
			Samples: aggregate.Samples,
			Stale:   minuteStart.Before(freshAfter),
		})
	}
	// An empty window is stale too. There is no recent observation to present,
	// and saying "current" about nothing is the failure this flag prevents.
	statistics.Stale = newest.IsZero() || newest.Before(freshAfter)
	return statistics, nil
}
