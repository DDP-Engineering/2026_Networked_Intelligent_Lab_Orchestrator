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
	withdrawn int
	fail      error
	current   bool
}

func (publisher *recordingPublisher) MDNSPublicationCurrent(context.Context) (bool, error) {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.fail != nil {
		return false, publisher.fail
	}
	return publisher.current, nil
}

func (publisher *recordingPublisher) WithdrawMDNS(context.Context) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.fail != nil {
		return publisher.fail
	}
	publisher.withdrawn++
	return nil
}

func (publisher *recordingPublisher) PublishMDNS(_ context.Context, advertisement MDNSAdvertisement) error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.fail != nil {
		return publisher.fail
	}
	publisher.published = append(publisher.published, advertisement)
	publisher.current = true
	return nil
}

func TestAnnouncerReassertsAnUnchangedPublicationLostByTheHelper(t *testing.T) {
	publisher := &recordingPublisher{}
	announcer := NewProvisioningMDNSAnnouncer(publisher)
	if err := announcer.Announce(context.Background(), []string{"eth0"}); err != nil {
		t.Fatal(err)
	}
	publisher.mu.Lock()
	publisher.current = false
	publisher.mu.Unlock()
	if err := announcer.Announce(context.Background(), []string{"eth0"}); err != nil {
		t.Fatal(err)
	}
	if publisher.count() != 2 {
		t.Fatalf("publications = %d, want reassertion after responder state was lost", publisher.count())
	}
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

func TestAnnouncerWithdrawsANameWhenItsLastInterfaceLeaves(t *testing.T) {
	t.Parallel()
	publisher := &recordingPublisher{}
	announcer := NewProvisioningMDNSAnnouncer(publisher)
	if err := announcer.Announce(context.Background(), []string{"eth0"}); err != nil {
		t.Fatal(err)
	}
	if err := announcer.Announce(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if publisher.withdrawn != 1 {
		t.Fatalf("withdrawals = %d, want 1", publisher.withdrawn)
	}
	if _, announced := announcer.Announced(); announced {
		t.Fatal("announcer retained a publication after its last interface left")
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

func TestCommissionedAnnouncerPublishesStableServiceIdentityAndManagementURL(t *testing.T) {
	t.Parallel()

	publisher := &recordingPublisher{}
	announcer, err := NewCommissionedMDNSAnnouncer(
		publisher,
		"ddp-remote",
		"7e799b52-624f-4cc8-9be6-213a3525a726",
		8443,
	)
	if err != nil {
		t.Fatalf("NewCommissionedMDNSAnnouncer: %v", err)
	}
	if err := announcer.Announce(context.Background(), []string{"enp1s0"}); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	advertised, ok := announcer.Announced()
	if !ok {
		t.Fatal("the commissioned service was not published")
	}
	if advertised.HostName != "ddp-remote" || advertised.LocalName() != "ddp-remote.local" {
		t.Fatalf("host identity = %#v, want ddp-remote.local", advertised)
	}
	if advertised.Service == nil {
		t.Fatal("the permanent DNS-SD service is missing")
	}
	service := *advertised.Service
	if service.Type != "_nilo._tcp" || service.Domain != "local" {
		t.Fatalf("service kind = %s.%s, want _nilo._tcp.local", service.Type, service.Domain)
	}
	if service.Instance != "NILO 7e799b52-624f-4cc8-9be6-213a3525a726" {
		t.Fatalf("service instance = %q", service.Instance)
	}
	wantTXT := []string{
		"txtvers=1",
		"id=7e799b52-624f-4cc8-9be6-213a3525a726",
		"hostname=ddp-remote",
		"url=https://ddp-remote.local:8443/",
		"path=/",
		"state=commissioned",
	}
	if !slices.Equal(service.TXT, wantTXT) {
		t.Fatalf("TXT = %v, want %v", service.TXT, wantTXT)
	}
}

func TestCommissionedAnnouncerAcceptsTheApplianceUUIDv7Identity(t *testing.T) {
	publisher := &recordingPublisher{}
	announcer, err := NewCommissionedMDNSAnnouncer(
		publisher, "bench-west", "0199a2d0-1234-7abc-8def-0123456789ab", 8443,
	)
	if err != nil {
		t.Fatalf("UUIDv7 appliance identity was refused: %v", err)
	}
	if err := announcer.Announce(context.Background(), []string{"eth0"}); err != nil {
		t.Fatal(err)
	}
}

func TestApplianceAnnouncerPublishesVersionTwoServiceInProvisioning(t *testing.T) {
	publisher := &recordingPublisher{}
	announcer, err := NewApplianceMDNSAnnouncer(publisher, MDNSApplianceIdentity{
		ApplianceID:  "0199a2d0-1234-7abc-8def-0123456789ab",
		SerialName:   "nilo-p2rjmzdkf1",
		FriendlyName: "bench-west",
		Port:         8443,
		State:        "provisioning",
		Version:      "0.8.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := announcer.Announce(context.Background(), []string{"eth0", "wlan0"}); err != nil {
		t.Fatal(err)
	}
	advertisement, ok := announcer.Announced()
	if !ok || advertisement.HostName != "nilo-p2rjmzdkf1" || advertisement.Service == nil {
		t.Fatalf("missing serial publication: %+v", advertisement)
	}
	wantTXT := []string{
		"txtvers=2",
		"id=0199a2d0-1234-7abc-8def-0123456789ab",
		"serial=nilo-p2rjmzdkf1",
		"name=bench-west",
		"url=https://nilo-p2rjmzdkf1.local:8443/",
		"path=/",
		"state=provisioning",
		"version=0.8.0",
	}
	if advertisement.Service.Instance != "NILO nilo-p2rjmzdkf1" || !slices.Equal(advertisement.Service.TXT, wantTXT) {
		t.Fatalf("service = %+v, want TXT %v", advertisement.Service, wantTXT)
	}
	if len(advertisement.Aliases) != 2 || advertisement.Aliases[0].HostName != "nilo" ||
		advertisement.Aliases[1].HostName != "bench-west" ||
		!slices.Equal(advertisement.Aliases[0].Interfaces, []string{"eth0", "wlan0"}) ||
		!slices.Equal(advertisement.Aliases[1].Interfaces, []string{"eth0", "wlan0"}) {
		t.Fatalf("provisioning aliases = %+v", advertisement.Aliases)
	}
}

func TestApplianceAnnouncerReplacesOnlyTheFriendlyAlias(t *testing.T) {
	publisher := &recordingPublisher{}
	announcer, err := NewApplianceMDNSAnnouncer(publisher, MDNSApplianceIdentity{
		ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", SerialName: "nilo-p2rjmzdkf1",
		Port: 8443, State: "commissioned", Version: "0.8.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := announcer.Announce(context.Background(), []string{"eth0"}); err != nil {
		t.Fatal(err)
	}
	if err := announcer.SetFriendlyName("bench-east"); err != nil {
		t.Fatal(err)
	}
	if err := announcer.Announce(context.Background(), []string{"eth0"}); err != nil {
		t.Fatal(err)
	}
	advertisement, _ := announcer.Announced()
	if len(advertisement.Aliases) != 1 || advertisement.Aliases[0].HostName != "bench-east" {
		t.Fatalf("friendly aliases = %+v", advertisement.Aliases)
	}
	if advertisement.HostName != "nilo-p2rjmzdkf1" || advertisement.Service == nil || !slices.Contains(advertisement.Service.TXT, "name=bench-east") {
		t.Fatalf("serial service was not preserved: %+v", advertisement)
	}
}

func TestProvisioningAnnouncerUsesSharedRecoveryNameWithoutPermanentService(t *testing.T) {
	t.Parallel()

	publisher := &recordingPublisher{}
	announcer := NewProvisioningMDNSAnnouncer(publisher)
	if err := announcer.Announce(context.Background(), []string{"enp1s0", "enp2s0"}); err != nil {
		t.Fatalf("Announce: %v", err)
	}
	advertised, ok := announcer.Announced()
	if !ok {
		t.Fatal("the provisioning name was not published")
	}
	if advertised.LocalName() != "nilo.local" || advertised.Service != nil {
		t.Fatalf("provisioning advertisement = %#v", advertised)
	}
}

func TestCommissionedAnnouncerScopesTemporaryNiloAliasToRecoveryLink(t *testing.T) {
	t.Parallel()

	publisher := &recordingPublisher{}
	announcer, err := NewCommissionedMDNSAnnouncer(
		publisher, "ddp-remote", "7e799b52-624f-4cc8-9be6-213a3525a726", 8443,
	)
	if err != nil {
		t.Fatal(err)
	}
	announcer.SetRecoveryInterface("eth9")
	if err := announcer.Announce(context.Background(), []string{"wlan0"}); err != nil {
		t.Fatal(err)
	}
	advertised, _ := announcer.Announced()
	if len(advertised.Aliases) != 1 || advertised.Aliases[0].HostName != "nilo" ||
		!slices.Equal(advertised.Aliases[0].Interfaces, []string{"eth9"}) {
		t.Fatalf("recovery aliases = %+v", advertised.Aliases)
	}
	if !slices.Equal(advertised.Interfaces, []string{"wlan0"}) {
		t.Fatalf("permanent service interfaces widened: %v", advertised.Interfaces)
	}
	announcer.SetRecoveryInterface("")
	if err := announcer.Announce(context.Background(), []string{"wlan0"}); err != nil {
		t.Fatal(err)
	}
	advertised, _ = announcer.Announced()
	if len(advertised.Aliases) != 0 {
		t.Fatalf("recovery alias remained after restore: %+v", advertised.Aliases)
	}
}
