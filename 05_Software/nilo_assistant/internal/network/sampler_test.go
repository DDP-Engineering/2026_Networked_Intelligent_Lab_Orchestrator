package network

import (
	"context"
	"errors"
	"testing"
	"time"
)

// countersStub returns one scripted reading per call, so a test states the
// exact counter sequence a device produced.
type countersStub struct {
	readings [][]CounterReading
	failures []error
	index    int
}

func (stub *countersStub) Counters(context.Context) ([]CounterReading, error) {
	defer func() { stub.index++ }()
	if stub.index < len(stub.failures) && stub.failures[stub.index] != nil {
		return nil, stub.failures[stub.index]
	}
	if stub.index >= len(stub.readings) {
		return nil, errors.New("counter script exhausted")
	}
	return stub.readings[stub.index], nil
}

// recordingStore keeps what the sampler wrote, in order.
type recordingStore struct {
	written  []Aggregate
	pruned   []time.Time
	writeErr error
}

func (store *recordingStore) WriteAggregate(_ context.Context, aggregate Aggregate) error {
	if store.writeErr != nil {
		return store.writeErr
	}
	store.written = append(store.written, aggregate)
	return nil
}

func TestSamplerDoesNotBridgeAFailedReading(t *testing.T) {
	counters := &countersStub{readings: steady(1000, 100, 4), failures: []error{nil, errors.New("missing"), nil, nil}}
	h := newSamplerHarness(t, counters)
	_ = h.sampler.Sample(context.Background())
	h.now = h.now.Add(SampleInterval)
	h.elapsed += SampleInterval
	_ = h.sampler.Sample(context.Background())
	h.tick(t)
	if h.sampler.OpenSamples("sim-enp1s0") != 0 {
		t.Fatal("traffic was fabricated across a failed observation")
	}
	h.tick(t)
	if h.sampler.OpenSamples("sim-enp1s0") != 1 {
		t.Fatal("new baseline did not resume measurements")
	}
}

func TestSamplerRetriesStorageFailureWithoutLosingTheClosedMinute(t *testing.T) {
	h := newSamplerHarness(t, &countersStub{readings: steady(1000, 100, 20)})
	_ = h.sampler.Sample(context.Background())
	h.tick(t)
	h.now = h.now.Add(time.Minute)
	h.elapsed += time.Minute
	h.store.writeErr = errors.New("database busy")
	if err := h.sampler.Sample(context.Background()); err == nil {
		t.Fatal("storage failure was ignored")
	}
	h.store.writeErr = nil
	h.tick(t)
	if len(h.store.written) != 1 || h.store.written[0].RXBytes != 100 {
		t.Fatalf("lost or duplicated the minute: %+v", h.store.written)
	}
}

func (store *recordingStore) PruneBefore(_ context.Context, cutoff time.Time) error {
	store.pruned = append(store.pruned, cutoff)
	return nil
}

// samplerHarness drives the sampler on a clock the test owns entirely.
type samplerHarness struct {
	sampler *Sampler
	store   *recordingStore
	now     time.Time
	elapsed time.Duration
}

func newSamplerHarness(t *testing.T, counters CounterSource) *samplerHarness {
	t.Helper()
	harness := &samplerHarness{
		store: &recordingStore{},
		now:   time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
	}
	sampler, err := NewSampler(SamplerOptions{
		Counters:   counters,
		Store:      harness.store,
		MeasuredBy: SourceSimulation,
		Now:        func() time.Time { return harness.now },
		Elapsed:    func() time.Duration { return harness.elapsed },
	})
	if err != nil {
		t.Fatalf("NewSampler: %v", err)
	}
	harness.sampler = sampler
	return harness
}

// tick advances both clocks by the sampling interval and takes one sample.
func (harness *samplerHarness) tick(t *testing.T) {
	t.Helper()
	harness.now = harness.now.Add(SampleInterval)
	harness.elapsed += SampleInterval
	if err := harness.sampler.Sample(context.Background()); err != nil {
		t.Fatalf("Sample: %v", err)
	}
}

func steady(base uint64, step uint64, count int) [][]CounterReading {
	readings := make([][]CounterReading, 0, count)
	for index := 0; index < count; index++ {
		value := base + step*uint64(index)
		readings = append(readings, []CounterReading{{
			InterfaceID: "sim-enp1s0",
			RXBytes:     value,
			TXBytes:     value / 2,
		}})
	}
	return readings
}

// Twelve five-second samples make one minute, and that minute is written once
// with the traffic that actually crossed the interface.
func TestSamplerWritesOneAggregatePerInterfacePerMinute(t *testing.T) {
	t.Parallel()

	if SampleInterval != 5*time.Second {
		t.Fatalf("SampleInterval = %s, want 5s", SampleInterval)
	}

	// The baseline is taken at 10:00:00 and is not itself traffic. Eleven ticks
	// then land inside the 10:00 minute, at :05 through :55, each adding 1000
	// RX and 500 TX. The twelfth tick lands on 10:01:00, which belongs to the
	// next minute and is what closes and writes the one before it.
	harness := newSamplerHarness(t, &countersStub{readings: steady(10_000, 1_000, 14)})
	if err := harness.sampler.Sample(context.Background()); err != nil {
		t.Fatalf("baseline Sample: %v", err)
	}
	for index := 0; index < 12; index++ {
		harness.tick(t)
	}

	if len(harness.store.written) != 1 {
		t.Fatalf("wrote %d aggregates, want 1: %+v", len(harness.store.written), harness.store.written)
	}
	written := harness.store.written[0]
	if written.InterfaceID != "sim-enp1s0" {
		t.Fatalf("interface = %q, want sim-enp1s0", written.InterfaceID)
	}
	if !written.MinuteStart.Equal(time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("minute start = %s, want 10:00:00Z", written.MinuteStart)
	}
	if written.MinuteStart.Second() != 0 || written.MinuteStart.Nanosecond() != 0 {
		t.Fatalf("minute start %s is not minute-aligned", written.MinuteStart)
	}
	if written.RXBytes != 11_000 || written.TXBytes != 5_500 {
		t.Fatalf("traffic = %d/%d, want 11000/5500", written.RXBytes, written.TXBytes)
	}
	if written.Samples != 11 {
		t.Fatalf("samples = %d, want 11", written.Samples)
	}
	if written.MeasuredBy != SourceSimulation {
		t.Fatalf("measured_by = %q, want %q", written.MeasuredBy, SourceSimulation)
	}
}

// A wall clock that jumps backwards must not manufacture or erase traffic: the
// interval length comes from a monotonic reading, never from two timestamps.
func TestSamplerMeasuresIntervalsWithTheMonotonicClock(t *testing.T) {
	t.Parallel()

	harness := newSamplerHarness(t, &countersStub{readings: steady(0, 1_000, 4)})
	if err := harness.sampler.Sample(context.Background()); err != nil {
		t.Fatalf("baseline Sample: %v", err)
	}

	// The wall clock steps backwards while monotonic time moves forward.
	harness.now = harness.now.Add(-30 * time.Second)
	harness.elapsed += SampleInterval
	if err := harness.sampler.Sample(context.Background()); err != nil {
		t.Fatalf("Sample after a backwards wall clock: %v", err)
	}

	for _, aggregate := range harness.store.written {
		if aggregate.RXBytes > 1_000 {
			t.Fatalf("a backwards wall clock inflated traffic to %d", aggregate.RXBytes)
		}
	}
}

// A counter that decreases means the device reset or was replaced. The straddled
// interval is not traffic, and a minute made only of such intervals is a gap.
func TestSamplerTreatsACounterResetAsAGapRatherThanZeroTraffic(t *testing.T) {
	t.Parallel()

	readings := [][]CounterReading{
		{{InterfaceID: "sim-enp1s0", RXBytes: 900_000, TXBytes: 400_000}},
		{{InterfaceID: "sim-enp1s0", RXBytes: 12, TXBytes: 4}},
	}
	harness := newSamplerHarness(t, &countersStub{readings: readings})
	if err := harness.sampler.Sample(context.Background()); err != nil {
		t.Fatalf("baseline Sample: %v", err)
	}
	harness.tick(t)

	for _, aggregate := range harness.store.written {
		if aggregate.RXBytes == 0 && aggregate.Samples == 0 {
			t.Fatal("a counter reset was written as a zero-traffic minute")
		}
	}
	if open := harness.sampler.OpenSamples("sim-enp1s0"); open != 0 {
		t.Fatalf("the interval straddling a counter reset contributed %d samples, want 0", open)
	}
}

// A device that disappears, and a read that fails outright, both leave a gap.
// Neither may be recorded as a minute in which nothing was transferred.
func TestSamplerLeavesAGapWhenAMinuteHasNoUsableSample(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		stub  *countersStub
		ticks int
	}{
		{
			name: "the read fails",
			stub: &countersStub{
				readings: [][]CounterReading{{{InterfaceID: "sim-enp1s0", RXBytes: 5_000}}},
				failures: []error{nil, errors.New("netlink unavailable"), errors.New("netlink unavailable")},
			},
			ticks: 2,
		},
		{
			name: "the device disappears",
			stub: &countersStub{
				readings: [][]CounterReading{
					{{InterfaceID: "sim-enp1s0", RXBytes: 5_000}},
					{},
					{},
				},
			},
			ticks: 2,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			harness := newSamplerHarness(t, test.stub)
			_ = harness.sampler.Sample(context.Background())
			for index := 0; index < test.ticks; index++ {
				harness.now = harness.now.Add(SampleInterval)
				harness.elapsed += SampleInterval
				_ = harness.sampler.Sample(context.Background())
			}

			for _, aggregate := range harness.store.written {
				if aggregate.Samples == 0 {
					t.Fatalf("wrote an aggregate with no contributing sample: %+v", aggregate)
				}
				if aggregate.RXBytes == 0 && aggregate.TXBytes == 0 {
					t.Fatalf("wrote a zero-traffic minute where the reading was missing: %+v", aggregate)
				}
			}
		})
	}
}

// Retention is applied as the history grows, so the store is never asked to
// hold more than the rolling twenty-four hours the plan allows.
func TestSamplerPrunesAnythingOlderThanTwentyFourHours(t *testing.T) {
	t.Parallel()

	harness := newSamplerHarness(t, &countersStub{readings: steady(0, 1_000, 16)})
	if err := harness.sampler.Sample(context.Background()); err != nil {
		t.Fatalf("baseline Sample: %v", err)
	}
	// Twelve ticks reach 10:01:00, the sample that closes and writes the 10:00
	// minute. Pruning happens with that write, so the cutoff is measured from
	// this instant.
	for index := 0; index < 12; index++ {
		harness.tick(t)
	}

	if len(harness.store.pruned) == 0 {
		t.Fatal("the sampler never asked the store to drop expired history")
	}
	cutoff := harness.store.pruned[len(harness.store.pruned)-1]
	if want := harness.now.Add(-RetentionWindow); !cutoff.Equal(want) {
		t.Fatalf("prune cutoff = %s, want %s", cutoff, want)
	}
	if RetentionWindow != 24*time.Hour {
		t.Fatalf("RetentionWindow = %s, want 24h", RetentionWindow)
	}
}

func TestNewSamplerRequiresItsCollaborators(t *testing.T) {
	t.Parallel()

	complete := SamplerOptions{
		Counters: &countersStub{}, Store: &recordingStore{}, MeasuredBy: SourceSimulation,
		Now: time.Now, Elapsed: func() time.Duration { return 0 },
	}
	for _, test := range []struct {
		name  string
		spoil func(*SamplerOptions)
	}{
		{name: "no counter source", spoil: func(o *SamplerOptions) { o.Counters = nil }},
		{name: "no store", spoil: func(o *SamplerOptions) { o.Store = nil }},
		{name: "no clock", spoil: func(o *SamplerOptions) { o.Now = nil }},
		{name: "no monotonic source", spoil: func(o *SamplerOptions) { o.Elapsed = nil }},
		{name: "no measurement source", spoil: func(o *SamplerOptions) { o.MeasuredBy = "" }},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options := complete
			test.spoil(&options)
			if _, err := NewSampler(options); err == nil {
				t.Fatalf("NewSampler accepted options with %s", test.name)
			}
		})
	}
}
