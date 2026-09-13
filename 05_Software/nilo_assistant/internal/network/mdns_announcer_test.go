package network

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
)

type recordingPublisher struct {
	mu        sync.Mutex
	published []MDNSAdvertisement
	fail      error
}

func (publisher *recordingPublisher) PublishMDNS(_ context.Context, advertisement MDNSAdvertisement) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.fail != nil {
		return publisher.fail
	}
	publisher.published = append(publisher.published, advertisement)
	return nil
}

func (publisher *recordingPublisher) count() int {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	return len(publisher.published)
}

// The reconcile loop runs every few seconds, so the responder is restarted only
// when what it should answer has actually changed.
func TestAnnouncerPublishesOnlyWhenTheAdvertisementChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	publisher := &recordingPublisher{}
	announcer := NewMDNSAnnouncer(publisher, "ddp-remote")

	if err := announcer.Announce(ctx, []string{"enp1s0", "wlp2s0"}); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	if publisher.count() != 1 {
		t.Fatalf("published %d times, want 1", publisher.count())
	}
	if err := announcer.Announce(ctx, []string{"enp1s0", "wlp2s0"}); err != nil {
		t.Fatalf("repeat announce: %v", err)
	}
	if publisher.count() != 1 {
		t.Fatalf("published %d times for an unchanged advertisement, want 1", publisher.count())
	}

	if err := announcer.Announce(ctx, []string{"enp1s0", "wlp2s0", "enx00e04c680001"}); err != nil {
		t.Fatalf("announce after an adapter appeared: %v", err)
	}
	if publisher.count() != 2 {
		t.Fatalf("published %d times after a change, want 2", publisher.count())
	}
	advertised, ok := announcer.Announced()
	if !ok {
		t.Fatal("the announcer reports nothing published")
	}
	if advertised.LocalName() != "ddp-remote.local" {
		t.Fatalf("published name = %q, want ddp-remote.local", advertised.LocalName())
	}
	if !slices.Equal(advertised.Interfaces, []string{"enp1s0", "wlp2s0", "enx00e04c680001"}) {
		t.Fatalf("published interfaces = %v", advertised.Interfaces)
	}
}

// An appliance whose name cannot be answered is still reachable by address, so
// a failure is reported and retried rather than swallowed or made fatal.
func TestAnnouncerRetriesAfterAFailedPublication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	refused := errors.New("responder unavailable")
	publisher := &recordingPublisher{fail: refused}
	announcer := NewMDNSAnnouncer(publisher, "ddp-remote")

	if err := announcer.Announce(ctx, []string{"enp1s0"}); !errors.Is(err, refused) {
		t.Fatalf("Announce = %v, want the publication failure reported", err)
	}
	if _, ok := announcer.Announced(); ok {
		t.Fatal("a failed publication was recorded as published")
	}

	publisher.mu.Lock()
	publisher.fail = nil
	publisher.mu.Unlock()
	if err := announcer.Announce(ctx, []string{"enp1s0"}); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
	if _, ok := announcer.Announced(); !ok {
		t.Fatal("the retry did not publish")
	}
}

// With no eligible interface there is nothing to answer on. That is an ordinary
// first-boot moment, not an error to report on every reconcile tick.
func TestAnnouncerStaysQuietWithNothingToAdvertise(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	publisher := &recordingPublisher{}
	announcer := NewMDNSAnnouncer(publisher, "ddp-remote")

	if err := announcer.Announce(ctx, nil); err != nil {
		t.Fatalf("Announce with no interface = %v, want no error", err)
	}
	if publisher.count() != 0 {
		t.Fatal("an unusable advertisement was published")
	}
	if _, ok := announcer.Announced(); ok {
		t.Fatal("the announcer claims to have published nothing usable")
	}
}

// A deployment with no responder to drive simply has no announcer, and every
// call on it is a no-op rather than a nil dereference in the reconcile loop.
func TestAnnouncerWithoutAPublisherIsInert(t *testing.T) {
	t.Parallel()

	announcer := NewMDNSAnnouncer(nil, "ddp-remote")
	if announcer != nil {
		t.Fatal("an announcer was built with nothing to publish through")
	}
	if err := announcer.Announce(context.Background(), []string{"enp1s0"}); err != nil {
		t.Fatalf("Announce on a nil announcer = %v, want no error", err)
	}
	if _, ok := announcer.Announced(); ok {
		t.Fatal("a nil announcer reports something published")
	}
}
