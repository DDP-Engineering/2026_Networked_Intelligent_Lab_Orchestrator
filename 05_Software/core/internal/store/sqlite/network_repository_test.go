package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

func minute(hour, minute int) time.Time {
	return time.Date(2026, 9, 2, hour, minute, 0, 0, time.UTC)
}

func aggregateAt(at time.Time, rx uint64) network.Aggregate {
	return network.Aggregate{
		InterfaceID: "sim-enp1s0", MinuteStart: at,
		RXBytes: rx, TXBytes: rx / 2,
		RXErrors: 1, TXErrors: 2, RXDropped: 3, TXDropped: 4,
		Samples: 11, MeasuredBy: network.SourceSimulation,
	}
}

func openNetworkRepository(t *testing.T) (*NetworkRepository, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nilo.db")
	database, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return NewNetworkRepository(database), path
}

func TestNetworkRepositoryRoundTripsEveryStoredField(t *testing.T) {
	t.Parallel()

	repository, _ := openNetworkRepository(t)
	want := aggregateAt(minute(10, 0), 11_000)
	if err := repository.WriteAggregate(context.Background(), want); err != nil {
		t.Fatalf("WriteAggregate: %v", err)
	}

	history, err := repository.History(context.Background(), "sim-enp1s0", minute(9, 0), minute(11, 0))
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("History returned %d rows, want 1", len(history))
	}
	got := history[0]
	if !got.MinuteStart.Equal(want.MinuteStart) || got.MinuteStart.Location() != time.UTC {
		t.Fatalf("minute start = %s, want %s in UTC", got.MinuteStart, want.MinuteStart)
	}
	got.MinuteStart = want.MinuteStart
	if got != want {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

// History is what a chart draws, so it must never return more points than the
// requested period has minutes, and must exclude anything outside it.
func TestNetworkRepositoryBoundsHistoryToTheRequestedPeriod(t *testing.T) {
	t.Parallel()

	repository, _ := openNetworkRepository(t)
	for offset := 0; offset < 90; offset++ {
		at := minute(9, 0).Add(time.Duration(offset) * time.Minute)
		if err := repository.WriteAggregate(context.Background(), aggregateAt(at, uint64(offset))); err != nil {
			t.Fatalf("WriteAggregate: %v", err)
		}
	}

	from, to := minute(10, 0), minute(10, 15)
	history, err := repository.History(context.Background(), "sim-enp1s0", from, to)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) > 15 {
		t.Fatalf("a 15-minute period returned %d points, want at most 15", len(history))
	}
	if len(history) == 0 {
		t.Fatal("a 15-minute period covering written minutes returned nothing")
	}
	previous := time.Time{}
	for _, point := range history {
		if point.MinuteStart.Before(from) || !point.MinuteStart.Before(to) {
			t.Fatalf("point at %s falls outside [%s, %s)", point.MinuteStart, from, to)
		}
		if !previous.IsZero() && !point.MinuteStart.After(previous) {
			t.Fatalf("history is not ascending: %s came after %s", point.MinuteStart, previous)
		}
		previous = point.MinuteStart
	}
}

// A gap must stay a gap. The store returns the minutes it holds and invents no
// zero-valued point for the ones it does not.
func TestNetworkRepositoryReturnsNoRowForAMinuteItNeverStored(t *testing.T) {
	t.Parallel()

	repository, _ := openNetworkRepository(t)
	for _, at := range []time.Time{minute(10, 0), minute(10, 3)} {
		if err := repository.WriteAggregate(context.Background(), aggregateAt(at, 500)); err != nil {
			t.Fatalf("WriteAggregate: %v", err)
		}
	}

	history, err := repository.History(context.Background(), "sim-enp1s0", minute(10, 0), minute(10, 5))
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("History returned %d rows, want exactly the 2 stored minutes", len(history))
	}
	for _, point := range history {
		if point.Samples == 0 {
			t.Fatalf("history carries a point with no contributing sample: %+v", point)
		}
	}
}

func TestNetworkRepositoryPrunesOnlyExpiredMinutes(t *testing.T) {
	t.Parallel()

	repository, _ := openNetworkRepository(t)
	old, kept := minute(9, 0), minute(10, 0)
	for _, at := range []time.Time{old, kept} {
		if err := repository.WriteAggregate(context.Background(), aggregateAt(at, 700)); err != nil {
			t.Fatalf("WriteAggregate: %v", err)
		}
	}

	if err := repository.PruneBefore(context.Background(), minute(9, 30)); err != nil {
		t.Fatalf("PruneBefore: %v", err)
	}
	history, err := repository.History(context.Background(), "sim-enp1s0", minute(8, 0), minute(11, 0))
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 1 || !history[0].MinuteStart.Equal(kept) {
		t.Fatalf("after pruning, history = %+v, want only the %s minute", history, kept)
	}
}

// Writing the same minute twice is a repeat, not new traffic: the sampler is
// the only writer and it closes a minute once, so the stored figure must not
// double.
func TestNetworkRepositoryKeepsOneRowPerInterfaceMinute(t *testing.T) {
	t.Parallel()

	repository, _ := openNetworkRepository(t)
	at := minute(10, 0)
	if err := repository.WriteAggregate(context.Background(), aggregateAt(at, 1_000)); err != nil {
		t.Fatalf("first WriteAggregate: %v", err)
	}
	if err := repository.WriteAggregate(context.Background(), aggregateAt(at, 9_999)); err != nil {
		t.Fatalf("repeated WriteAggregate: %v", err)
	}

	history, err := repository.History(context.Background(), "sim-enp1s0", minute(9, 0), minute(11, 0))
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("History returned %d rows for one minute, want 1", len(history))
	}
	if history[0].RXBytes != 1_000 {
		t.Fatalf("a repeated write changed the stored minute to %d, want the first value 1000", history[0].RXBytes)
	}
}

// History has to survive the process that recorded it.
func TestNetworkHistorySurvivesAnApplicationRestart(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nilo.db")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := NewNetworkRepository(first).WriteAggregate(context.Background(), aggregateAt(minute(10, 0), 4_242)); err != nil {
		t.Fatalf("WriteAggregate: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	history, err := NewNetworkRepository(second).History(context.Background(), "sim-enp1s0", minute(9, 0), minute(11, 0))
	if err != nil {
		t.Fatalf("History after restart: %v", err)
	}
	if len(history) != 1 || history[0].RXBytes != 4_242 {
		t.Fatalf("history after restart = %+v, want the 4242-byte minute", history)
	}
}
