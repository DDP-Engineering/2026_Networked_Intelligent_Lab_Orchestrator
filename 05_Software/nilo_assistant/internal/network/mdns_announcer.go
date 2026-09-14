package network

import (
	"context"
	"fmt"
	"sync"
)

// MDNSPublisher is implemented by the privileged helper. The unprivileged
// process hands it a validated advertisement and never responder syntax.
type MDNSPublisher interface {
	PublishMDNS(context.Context, MDNSAdvertisement) error
	WithdrawMDNS(context.Context) error
}

type mdnsPublicationVerifier interface {
	MDNSPublicationCurrent(context.Context) (bool, error)
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
	mu                sync.Mutex
	publisher         MDNSPublisher
	template          MDNSAdvertisement
	published         MDNSAdvertisement
	announced         bool
	lastError         error
	recoveryInterface string
}

// NewMDNSAnnouncer returns nil when there is nothing to publish through, so a
// deployment without a responder simply has no announcer.
func NewMDNSAnnouncer(publisher MDNSPublisher, hostName string) *MDNSAnnouncer {
	if publisher == nil {
		return nil
	}
	return &MDNSAnnouncer{publisher: publisher, template: MDNSAdvertisement{HostName: hostName}}
}

// NewProvisioningMDNSAnnouncer publishes the one well-known direct-connect
// alias. It intentionally carries no permanent service identity.
func NewProvisioningMDNSAnnouncer(publisher MDNSPublisher) *MDNSAnnouncer {
	return NewMDNSAnnouncer(publisher, "nilo")
}

// NewCommissionedMDNSAnnouncer publishes a unique permanent service for one
// appliance while preserving its configured system hostname.
func NewCommissionedMDNSAnnouncer(
	publisher MDNSPublisher, hostName, identity string, port uint16,
) (*MDNSAnnouncer, error) {
	if publisher == nil {
		return nil, nil
	}
	service := &MDNSService{
		Type:     "_nilo._tcp",
		Domain:   "local",
		Instance: "NILO " + identity,
		Port:     port,
		TXT: []string{
			"txtvers=1",
			"id=" + identity,
			"hostname=" + hostName,
			fmt.Sprintf("url=https://%s.local:%d/", hostName, port),
			"path=/",
			"state=commissioned",
		},
	}
	template := MDNSAdvertisement{HostName: hostName, Service: service, Interfaces: []string{"validate"}}
	if err := template.Validate(); err != nil {
		return nil, err
	}
	template.Interfaces = nil
	return &MDNSAnnouncer{publisher: publisher, template: template}, nil
}

// Announce publishes the local name on the given interfaces. Calling it with
// the interfaces already published does nothing.
func (announcer *MDNSAnnouncer) Announce(ctx context.Context, interfaces []string) error {
	if announcer == nil {
		return nil
	}
	wanted := announcer.template
	announcer.mu.Lock()
	recoveryInterface := announcer.recoveryInterface
	announcer.mu.Unlock()
	wanted.Interfaces = append([]string(nil), interfaces...)
	if recoveryInterface != "" {
		wanted.Aliases = []MDNSAlias{{HostName: "nilo", Interfaces: []string{recoveryInterface}}}
	}
	if len(wanted.Interfaces) == 0 && len(wanted.Aliases) == 0 {
		announcer.mu.Lock()
		announced := announcer.announced
		announcer.mu.Unlock()
		if !announced {
			return nil
		}
		err := announcer.publisher.WithdrawMDNS(ctx)
		announcer.mu.Lock()
		defer announcer.mu.Unlock()
		announcer.lastError = err
		if err != nil {
			return err
		}
		announcer.published = MDNSAdvertisement{}
		announcer.announced = false
		return nil
	}
	if wanted.Service != nil {
		service := *wanted.Service
		service.TXT = append([]string(nil), service.TXT...)
		wanted.Service = &service
	}
	announcer.mu.Lock()
	unchanged := announcer.announced && SameMDNSAdvertisement(announcer.published, wanted)
	announcer.mu.Unlock()
	if unchanged {
		if verifier, ok := announcer.publisher.(mdnsPublicationVerifier); ok {
			current, err := verifier.MDNSPublicationCurrent(ctx)
			if err != nil {
				announcer.mu.Lock()
				announcer.announced = false
				announcer.lastError = err
				announcer.mu.Unlock()
				return err
			}
			if current {
				return nil
			}
			announcer.mu.Lock()
			announcer.announced = false
			announcer.mu.Unlock()
		} else {
			return nil
		}
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

// SetRecoveryInterface selects the one link on which a commissioned appliance
// additionally answers as nilo.local. Empty removes the temporary alias.
func (announcer *MDNSAnnouncer) SetRecoveryInterface(interfaceName string) {
	if announcer == nil {
		return
	}
	announcer.mu.Lock()
	announcer.recoveryInterface = interfaceName
	announcer.mu.Unlock()
}

// Invalidate forces the next reconcile to replace the helper-owned group. It
// is used when the lifecycle selects a different advertisement template.
func (announcer *MDNSAnnouncer) Invalidate() {
	if announcer == nil {
		return
	}
	announcer.mu.Lock()
	announcer.announced = false
	announcer.mu.Unlock()
}

// Withdraw removes the helper-owned publication even when this process no
// longer trusts its local cache. Lifecycle changes use it before publishing a
// different name or service so a failed replacement cannot leave the old
// commissioning alias reachable.
func (announcer *MDNSAnnouncer) Withdraw(ctx context.Context) error {
	if announcer == nil {
		return nil
	}
	err := announcer.publisher.WithdrawMDNS(ctx)
	announcer.mu.Lock()
	defer announcer.mu.Unlock()
	announcer.lastError = err
	if err == nil {
		announcer.published = MDNSAdvertisement{}
		announcer.announced = false
	}
	return err
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
