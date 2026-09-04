package network

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

const (
	// SampleInterval is how often interface counters are read.
	SampleInterval = 5 * time.Second
	// RetentionWindow is how much aggregate history is kept.
	RetentionWindow = 24 * time.Hour
)

// CounterReading is one device's cumulative counters at one instant. These are
// the raw totals the operating system keeps; the sampler turns successive
// readings into the traffic that crossed the interface between them.
type CounterReading struct {
	InterfaceID string

	RXBytes uint64
	TXBytes uint64

	RXErrors  uint64
	TXErrors  uint64
	RXDropped uint64
	TXDropped uint64
}

// CounterSource reads every visible interface's counters at once, so one
// reading is one consistent instant rather than a series of staggered ones.
type CounterSource interface {
	Counters(ctx context.Context) ([]CounterReading, error)
}

// Aggregate is one interface's traffic during one whole minute.
//
// Samples is how many sampling intervals contributed. It is stored rather than
// assumed, because a minute assembled from four usable readings is a weaker
// observation than one assembled from twelve, and a reader deserves to know
// which it is holding.
type Aggregate struct {
	InterfaceID string
	MinuteStart time.Time

	RXBytes uint64
	TXBytes uint64

	RXErrors  uint64
	TXErrors  uint64
	RXDropped uint64
	TXDropped uint64

	Samples    int
	MeasuredBy Source
}

// AggregateStore persists closed minutes and drops expired ones. A minute is
// only ever written once, so the store may treat a repeat as a conflict.
type AggregateStore interface {
	WriteAggregate(ctx context.Context, aggregate Aggregate) error
	PruneBefore(ctx context.Context, cutoff time.Time) error
}

// SamplerOptions are the sampler's collaborators. Every one is required: a
// sampler missing its clock or its store would silently record nothing, which
// is the failure this service exists to make impossible.
type SamplerOptions struct {
	Counters   CounterSource
	Store      AggregateStore
	MeasuredBy Source

	// Now supplies wall-clock time, used only to decide which minute a sample
	// belongs to.
	Now func() time.Time
	// Elapsed supplies monotonic time since the process started. Interval
	// lengths come from here, never from a difference of two Now values, so a
	// clock correction cannot manufacture or erase traffic.
	Elapsed func() time.Duration
}

// openMinute is the minute currently being assembled for one interface.
type openMinute struct {
	start   time.Time
	samples int

	rxBytes   uint64
	txBytes   uint64
	rxErrors  uint64
	txErrors  uint64
	rxDropped uint64
	txDropped uint64
}

// previousReading is the last usable counter reading for one interface, with
// the monotonic instant it was taken at.
type previousReading struct {
	counters CounterReading
	at       time.Duration
	valid    bool
}

// Sampler folds five-second counter readings into one-minute aggregates.
//
// It never writes a minute it did not measure. A minute with no usable sample
// is simply absent from the history, which is what lets a reader tell "no
// traffic" from "no measurement" — the distinction a flat line at zero destroys.
type Sampler struct {
	options SamplerOptions

	previous map[string]previousReading
	open     map[string]*openMinute
}

// NewSampler builds a sampler over its collaborators.
func NewSampler(options SamplerOptions) (*Sampler, error) {
	switch {
	case options.Counters == nil:
		return nil, errors.New("sampler requires a counter source")
	case options.Store == nil:
		return nil, errors.New("sampler requires an aggregate store")
	case options.Now == nil:
		return nil, errors.New("sampler requires a wall clock")
	case options.Elapsed == nil:
		return nil, errors.New("sampler requires a monotonic elapsed-time source")
	case options.MeasuredBy == "":
		return nil, errors.New("sampler requires the name of what measures it")
	}
	return &Sampler{
		options:  options,
		previous: make(map[string]previousReading),
		open:     make(map[string]*openMinute),
	}, nil
}

// OpenSamples reports how many intervals have contributed to the minute
// currently being assembled for an interface. It exists so a test can assert
// that a discarded interval contributed nothing.
func (sampler *Sampler) OpenSamples(interfaceID string) int {
	if open, ok := sampler.open[interfaceID]; ok {
		return open.samples
	}
	return 0
}

// Sample takes one reading and folds it into the open minute of each interface
// it covers, closing any minute the clock has moved past.
//
// A failed read is not an error the caller must handle: the interfaces it would
// have covered simply receive no sample, which is exactly the gap that should
// appear in the history. The error is returned so a caller may log it.
func (sampler *Sampler) Sample(ctx context.Context) error {
	now := sampler.options.Now().UTC()
	at := sampler.options.Elapsed()

	// Close first, fold second. A sample taken at 10:01:00 belongs to the new
	// minute, and the minute it just ended has to be written before that sample
	// opens its successor — otherwise the completed minute is overwritten by
	// the one starting on top of it.
	if err := sampler.closeMinutesBefore(ctx, now); err != nil {
		clear(sampler.previous)
		return err
	}

	readings, readErr := sampler.options.Counters.Counters(ctx)
	if readErr != nil {
		clear(sampler.previous)
		// Nothing was observed, so nothing is folded in. A minute that ends
		// with no usable sample at all is never written.
		return fmt.Errorf("read interface counters: %w", readErr)
	}

	seen := make(map[string]bool, len(readings))
	for _, reading := range readings {
		seen[reading.InterfaceID] = true
		sampler.fold(reading, at, now)
	}
	// A device that vanished keeps no carried-over baseline: when it returns,
	// its first reading starts a new interval rather than being differenced
	// against a total from before it disappeared.
	for id := range sampler.previous {
		if !seen[id] {
			delete(sampler.previous, id)
		}
	}
	return nil
}

// fold turns one reading into the traffic since the previous reading and adds
// it to that interface's open minute.
func (sampler *Sampler) fold(reading CounterReading, at time.Duration, now time.Time) {
	previous, had := sampler.previous[reading.InterfaceID]
	sampler.previous[reading.InterfaceID] = previousReading{counters: reading, at: at, valid: true}

	if !had || !previous.valid {
		// The first reading establishes a baseline and is not itself traffic.
		return
	}
	if counterWentBackwards(previous.counters, reading) {
		// The device reset or was replaced. The straddling interval is not a
		// measurement of anything, so it is discarded and the reading just
		// stored becomes the new baseline.
		return
	}
	if at <= previous.at || at-previous.at > 2*SampleInterval {
		// Monotonic time did not advance, so no interval elapsed.
		return
	}

	open := sampler.open[reading.InterfaceID]
	if open == nil || !open.start.Equal(minuteOf(now)) {
		open = &openMinute{start: minuteOf(now)}
		sampler.open[reading.InterfaceID] = open
	}
	open.samples++
	open.rxBytes += reading.RXBytes - previous.counters.RXBytes
	open.txBytes += reading.TXBytes - previous.counters.TXBytes
	open.rxErrors += saturatingDelta(previous.counters.RXErrors, reading.RXErrors)
	open.txErrors += saturatingDelta(previous.counters.TXErrors, reading.TXErrors)
	open.rxDropped += saturatingDelta(previous.counters.RXDropped, reading.RXDropped)
	open.txDropped += saturatingDelta(previous.counters.TXDropped, reading.TXDropped)
}

// closeMinutesBefore writes out every open minute the clock has moved past, and
// prunes history that has aged out of the retention window.
func (sampler *Sampler) closeMinutesBefore(ctx context.Context, now time.Time) error {
	current := minuteOf(now)

	// Deterministic order, so a test reading the store sees a stable sequence.
	ids := make([]string, 0, len(sampler.open))
	for id := range sampler.open {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	wrote := false
	for _, id := range ids {
		open := sampler.open[id]
		if !open.start.Before(current) {
			continue
		}
		if open.samples == 0 {
			delete(sampler.open, id)
			// Nothing usable landed in this minute. It stays a gap.
			continue
		}
		if err := sampler.options.Store.WriteAggregate(ctx, Aggregate{
			InterfaceID: id, MinuteStart: open.start,
			RXBytes: open.rxBytes, TXBytes: open.txBytes,
			RXErrors: open.rxErrors, TXErrors: open.txErrors,
			RXDropped: open.rxDropped, TXDropped: open.txDropped,
			Samples: open.samples, MeasuredBy: sampler.options.MeasuredBy,
		}); err != nil {
			return fmt.Errorf("persist traffic minute: %w", err)
		}
		delete(sampler.open, id)
		wrote = true
	}
	if wrote {
		return sampler.options.Store.PruneBefore(ctx, now.Add(-RetentionWindow))
	}
	return nil
}

func minuteOf(instant time.Time) time.Time {
	return instant.UTC().Truncate(time.Minute)
}

// counterWentBackwards reports whether any byte counter decreased, which only
// happens when the device reset or was replaced.
func counterWentBackwards(previous, current CounterReading) bool {
	return current.RXBytes < previous.RXBytes || current.TXBytes < previous.TXBytes
}

// saturatingDelta treats a decreased auxiliary counter as no change rather than
// wrapping to an enormous number.
func saturatingDelta(previous, current uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}
