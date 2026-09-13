package network

import (
	"context"
	"sync"
)

// MDNSPublisher is implemented by the privileged helper. The unprivileged
// process hands it a validated advertisement and never responder syntax.
type MDNSPublisher interface {
	PublishMDNS(context.Context, MDNSAdvertisement) error
}

// MDNSAnnouncer keeps the appliance's published local name in step with the
// interfaces currently carrying management access.
//
// It republishes only when the advertisement changes, so the ordinary reconcile
// cadence does not restart the responder every few seconds. A failure is
// reported and remembered as not-published, so the next reconcile tries again:
// an appliance whose name cannot be answered is still reachable by address, and
// silently giving up would hide that from the operator.
type MDNSAnnouncer struct {
	mu        sync.Mutex
	publisher MDNSPublisher
	hostName  string
	published MDNSAdvertisement
	announced bool
	lastError error
}

// NewMDNSAnnouncer returns nil when there is nothing to publish through, so a
// deployment without a responder simply has no announcer.
func NewMDNSAnnouncer(publisher MDNSPublisher, hostName string) *MDNSAnnouncer {
	if publisher == nil {
		return nil
	}
	return &MDNSAnnouncer{publisher: publisher, hostName: hostName}
}

// Announce publishes the local name on the given interfaces. Calling it with
// the interfaces already published does nothing.
func (announcer *MDNSAnnouncer) Announce(ctx context.Context, interfaces []string) error {
	if announcer == nil {
		return nil
	}
	wanted := MDNSAdvertisement{
		HostName: announcer.hostName, Interfaces: append([]string(nil), interfaces...),
	}
	announcer.mu.Lock()
	unchanged := announcer.announced && SameMDNSAdvertisement(announcer.published, wanted)
	announcer.mu.Unlock()
	if unchanged {
		return nil
	}
	if err := wanted.Validate(); err != nil {
		// Nothing eligible is present yet, or the name is unusable. Neither is
		// an error the reconcile loop should report on every tick; the next
		// call with a usable advertisement publishes.
		announcer.mu.Lock()
		announcer.announced = false
		announcer.mu.Unlock()
		return nil
	}
	err := announcer.publisher.PublishMDNS(ctx, wanted)
	announcer.mu.Lock()
	defer announcer.mu.Unlock()
	announcer.lastError = err
	if err != nil {
		announcer.announced = false
		return err
	}
	announcer.published, announcer.announced = wanted, true
	return nil
}

// Announced reports the advertisement currently published, if any.
func (announcer *MDNSAnnouncer) Announced() (MDNSAdvertisement, bool) {
	if announcer == nil {
		return MDNSAdvertisement{}, false
	}
	announcer.mu.Lock()
	defer announcer.mu.Unlock()
	return announcer.published, announcer.announced
}
