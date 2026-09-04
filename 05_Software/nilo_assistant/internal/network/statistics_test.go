package network

import (
	"context"
	"testing"
	"time"
)

// historyStub returns whatever minutes a test stored, honouring the requested
// window so a bound assertion is about the service and not about the stub.
type historyStub struct {
	minutes []Aggregate
	lastadd struct {
		interfaceID string
		from, to    time.Time
	}
}

func (stub *historyStub) History(
	_ context.Context, interfaceID string, from, to time.Time,
) ([]Aggregate, error) {
	stub.lastadd.interfaceID, stub.lastadd.from, stub.lastadd.to = interfaceID, from, to
	var within []Aggregate
	for _, candidate := range stub.minutes {
		if candidate.InterfaceID != interfaceID {
			continue
		}
		if candidate.MinuteStart.Before(from) || !candidate.MinuteStart.Before(to) {
			continue
		}
		within = append(within, candidate)
	}
	return within, nil
}

func minutesEndingAt(end time.Time, count int) []Aggregate {
	aggregates := make([]Aggregate, 0, count)
	for index := count; index > 0; index-- {
		aggregates = append(aggregates, Aggregate{
			InterfaceID: "sim-enp1s0",
			MinuteStart: end.Add(-time.Duration(index) * time.Minute).UTC().Truncate(time.Minute),
			RXBytes:     1_000, TXBytes: 500, Samples: 11, MeasuredBy: SourceSimulation,
		})
	}
	return aggregates
}

func statisticsService(t *testing.T, now time.Time, history *historyStub) *Service {
	t.Helper()
	service, err := NewService(ServiceOptions{
		Provider: NewSimulatedProvider(),
		History:  history,
		Now:      func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func TestParsePeriodAcceptsOnlyTheThreeOfferedWindows(t *testing.T) {
	t.Parallel()

	for spelling, want := range map[string]time.Duration{
		"15m": 15 * time.Minute,
		"1h":  time.Hour,
		"24h": 24 * time.Hour,
	} {
		period, err := ParsePeriod(spelling)
		if err != nil {
			t.Fatalf("ParsePeriod(%q): %v", spelling, err)
		}
		if period.Duration() != want {
			t.Fatalf("ParsePeriod(%q).Duration() = %s, want %s", spelling, period.Duration(), want)
		}
	}
	for _, rejected := range []string{"", "5m", "7d", "24H", "1h ", "forever"} {
		if _, err := ParsePeriod(rejected); err == nil {
			t.Fatalf("ParsePeriod(%q) was accepted", rejected)
		}
	}
}

// A chart may not receive more points than the period has minutes, however many
// rows the store happens to hold.
func TestStatisticsReturnsAtMostOnePointPerMinuteOfThePeriod(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 2, 12, 0, 30, 0, time.UTC)
	history := &historyStub{minutes: minutesEndingAt(now, 300)}
	service := statisticsService(t, now, history)

	for _, test := range []struct {
		period Period
		most   int
	}{
		{period: Period15m, most: 15},
		{period: Period1h, most: 60},
		{period: Period24h, most: 24 * 60},
	} {
		test := test
		t.Run(string(test.period), func(t *testing.T) {
			statistics, err := service.Statistics(context.Background(), "sim-enp1s0", test.period)
			if err != nil {
				t.Fatalf("Statistics: %v", err)
			}
			if len(statistics.Points) > test.most {
				t.Fatalf("%s returned %d points, want at most %d",
					test.period, len(statistics.Points), test.most)
			}
			if statistics.Period != test.period {
				t.Fatalf("Period = %q, want %q", statistics.Period, test.period)
			}
			if !statistics.To.After(statistics.From) {
				t.Fatalf("window [%s, %s) does not advance", statistics.From, statistics.To)
			}
			if got := statistics.To.Sub(statistics.From); got != test.period.Duration() {
				t.Fatalf("window spans %s, want %s", got, test.period.Duration())
			}
			for _, point := range statistics.Points {
				if point.MinuteStart.IsZero() {
					t.Fatal("a point carries no measurement time")
				}
			}
		})
	}
}

// Refresh failure must not leave old values looking current. A series whose
// newest minute has aged past the freshness window is marked stale.
func TestStatisticsMarksPointsStaleOnceSamplingHasStopped(t *testing.T) {
	t.Parallel()

	sampledUntil := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	history := &historyStub{minutes: minutesEndingAt(sampledUntil, 10)}

	t.Run("still sampling", func(t *testing.T) {
		service := statisticsService(t, sampledUntil.Add(30*time.Second), history)
		statistics, err := service.Statistics(context.Background(), "sim-enp1s0", Period15m)
		if err != nil {
			t.Fatalf("Statistics: %v", err)
		}
		if statistics.Stale {
			t.Fatal("a series sampled half a minute ago is reported stale")
		}
		if len(statistics.Points) == 0 {
			t.Fatal("no points returned while sampling")
		}
		if statistics.Points[len(statistics.Points)-1].Stale {
			t.Fatal("the newest point of a live series is marked stale")
		}
	})

	t.Run("sampling stopped", func(t *testing.T) {
		service := statisticsService(t, sampledUntil.Add(20*time.Minute), history)
		statistics, err := service.Statistics(context.Background(), "sim-enp1s0", Period24h)
		if err != nil {
			t.Fatalf("Statistics: %v", err)
		}
		if !statistics.Stale {
			t.Fatal("a series that stopped twenty minutes ago is not reported stale")
		}
		for _, point := range statistics.Points {
			if !point.Stale {
				t.Fatalf("point at %s is not marked stale after sampling stopped", point.MinuteStart)
			}
		}
	})
}

// An empty period is a legitimate answer, and it is not a failure: it says the
// appliance measured nothing in that window, which is what a gap looks like.
func TestStatisticsReportsAnEmptyWindowAsStaleRatherThanAsAFailure(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	service := statisticsService(t, now, &historyStub{})

	statistics, err := service.Statistics(context.Background(), "sim-enp1s0", Period1h)
	if err != nil {
		t.Fatalf("Statistics over an empty window: %v", err)
	}
	if len(statistics.Points) != 0 {
		t.Fatalf("an empty store produced %d points", len(statistics.Points))
	}
	if !statistics.Stale {
		t.Fatal("a window with no observation at all is not marked stale")
	}
	if statistics.MeasuredBy != SourceSimulation {
		t.Fatalf("MeasuredBy = %q, want %q", statistics.MeasuredBy, SourceSimulation)
	}
}

func TestStatisticsRequiresAnInterfaceAndAKnownPeriod(t *testing.T) {
	t.Parallel()

	service := statisticsService(t, time.Now().UTC(), &historyStub{})
	if _, err := service.Statistics(context.Background(), "", Period15m); err == nil {
		t.Fatal("Statistics accepted an empty interface identity")
	}
	if _, err := service.Statistics(context.Background(), "sim-enp1s0", Period("7d")); err == nil {
		t.Fatal("Statistics accepted a period outside the offered windows")
	}
}
