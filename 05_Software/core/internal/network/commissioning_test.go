package network

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"
)

type recordingCommissioner struct {
	mu        sync.Mutex
	completed int
	fail      error
}

func (commissioner *recordingCommissioner) CompleteCommissioning(context.Context) error {
	commissioner.mu.Lock()
	defer commissioner.mu.Unlock()
	if commissioner.fail != nil {
		return commissioner.fail
	}
	commissioner.completed++
	return nil
}

func (commissioner *recordingCommissioner) count() int {
	commissioner.mu.Lock()
	defer commissioner.mu.Unlock()
	return commissioner.completed
}

// stubPublishedCredential stands in for the appliance adapter that reports
// whether the credential published with the image still opens this appliance.
type stubPublishedCredential struct {
	mu         sync.Mutex
	stillOpens bool
	fail       error
	calls      int
}

func (credential *stubPublishedCredential) PublishedVerifierRemains(context.Context) (bool, error) {
	credential.mu.Lock()
	defer credential.mu.Unlock()
	credential.calls++
	if credential.fail != nil {
		return false, credential.fail
	}
	return credential.stillOpens, nil
}

func (credential *stubPublishedCredential) set(stillOpens bool) {
	credential.mu.Lock()
	defer credential.mu.Unlock()
	credential.stillOpens = stillOpens
}

func newCommissionableService(
	t *testing.T, now *time.Time, provider *accessProvider, store *accessStore,
	firewall *accessFirewall, commissioner *recordingCommissioner,
) *ManagementAccessService {
	t.Helper()
	// An appliance whose published password has already been replaced is the
	// ordinary case these tests are about, so they supply a checker that says so.
	return newCommissionableServiceWithCredential(
		t, now, provider, store, firewall, commissioner, &stubPublishedCredential{})
}

func newCommissionableServiceWithCredential(
	t *testing.T, now *time.Time, provider *accessProvider, store *accessStore,
	firewall *accessFirewall, commissioner *recordingCommissioner,
	credential PublishedCredential,
) *ManagementAccessService {
	t.Helper()
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Now: func() time.Time { return *now },
		Provisioning: true, ProvisioningTLSNames: []string{"ddp-remote.local"},
		Commissioner: commissioner, ConfirmWindow: time.Minute,
		PublishedCredential: credential,
	})
	if err != nil {
		t.Fatalf("NewManagementAccessService: %v", err)
	}
	return service
}

// Acceptance 9 and 14: commissioning fixes the allowlist to what the
// administrator chose, and from then on the appliance stops following the
// hardware.
func TestCommissioningFixesTheAllowlistAndStopsDeriving(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}
	commissioner := &recordingCommissioner{}

	service := newCommissionableService(t, &now, provider, store, firewall, commissioner)
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0", "wlp2s0"}) {
		t.Fatalf("derived allowlist = %v, want both eligible adapters", got)
	}

	status, err := service.Commission(ctx, []string{"enp1s0"}, mustAddr(t, "192.168.10.24/24"))
	if err != nil {
		t.Fatalf("Commission: %v", err)
	}
	if !slices.Equal(status.Configured, []string{"enp1s0"}) {
		t.Fatalf("configured after commissioning = %v, want only enp1s0", status.Configured)
	}
	if commissioner.count() != 1 {
		t.Fatalf("the lifecycle state was recorded %d times, want once", commissioner.count())
	}
	if got := firewall.applied[len(firewall.applied)-1].Interfaces; !slices.Equal(got, []string{"enp1s0"}) {
		t.Fatalf("firewall after commissioning = %v, want only enp1s0", got)
	}

	// The adapter that was dropped must stop being admitted, and a newly
	// attached one must never join.
	if service.Allows(mustAddr(t, "192.168.20.15/24")) {
		t.Fatal("the dropped adapter is still admitted after commissioning")
	}
	provider.set(append(provisioningInterfaces(), usbEthernet()))
	now = now.Add(time.Second)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile after commissioning: %v", err)
	}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0"}) {
		t.Fatalf("allowlist after a later attach = %v, want only enp1s0", got)
	}
	if service.Allows(mustAddr(t, "192.168.30.7/24")) {
		t.Fatal("an adapter attached after commissioning was admitted")
	}
}

// Acceptance 9: an appliance must never be commissioned into unreachability.
func TestCommissioningRefusesAnAllowlistWithNoUsablePath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}
	commissioner := &recordingCommissioner{}

	service := newCommissionableService(t, &now, provider, store, firewall, commissioner)
	applied := len(firewall.applied)

	for _, testCase := range []struct {
		name       string
		interfaces []string
	}{
		{name: "nothing at all"},
		{name: "an interface that is not present", interfaces: []string{"enp9s0"}},
		{name: "an ineligible interface", interfaces: []string{"veth9f2a41"}},
		{name: "the loopback device", interfaces: []string{"lo"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := service.Commission(ctx, testCase.interfaces, netip.Addr{}); err == nil {
				t.Fatalf("Commission(%v) was accepted", testCase.interfaces)
			}
		})
	}

	if commissioner.count() != 0 {
		t.Fatal("a refused commissioning still recorded the lifecycle state")
	}
	if len(firewall.applied) != applied {
		t.Fatal("a refused commissioning still reached the firewall")
	}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0", "wlp2s0"}) {
		t.Fatalf("allowlist after refusals = %v, want the derived set intact", got)
	}
	if _, err := service.Commission(ctx, []string{"enp1s0"}, netip.Addr{}); err != nil {
		t.Fatalf("a usable allowlist was refused after the earlier refusals: %v", err)
	}
}

// Acceptance 11: if the lifecycle state cannot be recorded, the appliance stays
// provisioning on the allowlist it was already serving, rather than ending up
// half-commissioned.
func TestCommissioningRestoresTheDerivedAllowlistWhenTheStateCannotBeRecorded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}
	refused := errors.New("the lifecycle state could not be recorded")
	commissioner := &recordingCommissioner{fail: refused}

	service := newCommissionableService(t, &now, provider, store, firewall, commissioner)

	if _, err := service.Commission(ctx, []string{"enp1s0"}, netip.Addr{}); !errors.Is(err, refused) {
		t.Fatalf("Commission = %v, want the recording failure reported", err)
	}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0", "wlp2s0"}) {
		t.Fatalf("allowlist after a failed transition = %v, want the derived set restored", got)
	}
	if got := firewall.applied[len(firewall.applied)-1].Interfaces; !slices.Equal(got, []string{"enp1s0", "wlp2s0"}) {
		t.Fatalf("firewall after a failed transition = %v, want the derived set restored", got)
	}
	if !service.Allows(mustAddr(t, "192.168.20.15/24")) {
		t.Fatal("an adapter that was serving before the failed transition is no longer admitted")
	}

	// Still provisioning, so the allowlist still follows the hardware.
	provider.set(append(provisioningInterfaces(), usbEthernet()))
	now = now.Add(time.Second)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile after the failed transition: %v", err)
	}
	if !service.Allows(mustAddr(t, "192.168.30.7/24")) {
		t.Fatal("the appliance stopped following the hardware after a failed transition")
	}
}

// Commissioning is a one-way step, and it belongs to an appliance that has not
// taken it yet.
func TestCommissioningIsRefusedOnceAlreadyCommissioned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	commissioner := &recordingCommissioner{}

	service := newCommissionableService(t, &now, provider, store, &accessFirewall{}, commissioner)
	if _, err := service.Commission(ctx, []string{"enp1s0"}, netip.Addr{}); err != nil {
		t.Fatalf("first Commission: %v", err)
	}
	if _, err := service.Commission(ctx, []string{"wlp2s0"}, netip.Addr{}); !errors.Is(err, ErrAlreadyCommissioned) {
		t.Fatalf("second Commission = %v, want ErrAlreadyCommissioned", err)
	}
	if commissioner.count() != 1 {
		t.Fatalf("the lifecycle state was recorded %d times, want once", commissioner.count())
	}
}

// Dropping the interface the administrator is using is how an appliance gets
// locked out, so it takes a deliberate confirmation.
func TestCommissioningAwayFromTheCurrentInterfaceNeedsConfirmation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	commissioner := &recordingCommissioner{}

	service := newCommissionableService(t, &now, provider, store, &accessFirewall{}, commissioner)

	current := mustAddr(t, "192.168.20.15/24")
	_, err := service.Commission(ctx, []string{"enp1s0"}, current)
	if !errors.Is(err, ErrCurrentAccessConfirmation) {
		t.Fatalf("Commission dropping the caller's own interface = %v, want ErrCurrentAccessConfirmation", err)
	}
	if commissioner.count() != 0 {
		t.Fatal("a refused commissioning still recorded the lifecycle state")
	}

	if _, err := service.CommissionConfirmed(ctx, []string{"enp1s0"}, current); err != nil {
		t.Fatalf("confirmed Commission: %v", err)
	}
	if commissioner.count() != 1 {
		t.Fatalf("the lifecycle state was recorded %d times, want once", commissioner.count())
	}
}

// REQ-SEC-c9e034de: the published credential cannot survive commissioning. While
// it still opens the appliance, completing commissioning is refused, and the
// refusal changes nothing; once it no longer does, the identical request works.
func TestCommissioningIsRefusedWhileThePublishedCredentialStillOpensTheAppliance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}
	commissioner := &recordingCommissioner{}
	credential := &stubPublishedCredential{stillOpens: true}

	service := newCommissionableServiceWithCredential(
		t, &now, provider, store, firewall, commissioner, credential)
	// The service derives and applies a provisioning allowlist as it starts, so
	// the baseline is what it was already serving, not an empty one.
	applied := len(firewall.applied)
	persisted := store.record

	_, err := service.Commission(ctx, []string{"enp1s0"}, mustAddr(t, "192.168.10.24/24"))
	if !errors.Is(err, ErrPublishedCredentialUnchanged) {
		t.Fatalf("Commission with the published password in place = %v, want ErrPublishedCredentialUnchanged", err)
	}
	if commissioner.count() != 0 {
		t.Fatal("a refused commissioning still recorded the lifecycle state")
	}
	if len(firewall.applied) != applied {
		t.Fatal("a refused commissioning still reached the firewall")
	}
	if store.record.State != persisted.State ||
		!slices.Equal(store.record.Configuration.Interfaces, persisted.Configuration.Interfaces) {
		t.Fatal("a refused commissioning still changed the persisted management access record")
	}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0", "wlp2s0"}) {
		t.Fatalf("allowlist after the refusal = %v, want the derived set intact", got)
	}
	if !service.Provisioning() {
		t.Fatal("a refused commissioning left the appliance no longer provisioning")
	}

	// The administrator changes the password; the identical request now works.
	credential.set(false)
	if _, err := service.Commission(ctx, []string{"enp1s0"}, mustAddr(t, "192.168.10.24/24")); err != nil {
		t.Fatalf("Commission after the published password was changed: %v", err)
	}
	if commissioner.count() != 1 {
		t.Fatalf("lifecycle state recorded %d times, want exactly once", commissioner.count())
	}
}

// REQ-SEC-c9e034de: a gate that cannot answer must not silently pass, so a
// missing or failing checker refuses commissioning rather than allowing it.
func TestCommissioningIsRefusedWhenThePublishedCredentialCannotBeChecked(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)

	for _, testCase := range []struct {
		name       string
		credential PublishedCredential
	}{
		{name: "no checker at all", credential: nil},
		{name: "a checker that fails", credential: &stubPublishedCredential{fail: errors.New("identity store unavailable")}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := &accessProvider{}
			provider.set(provisioningInterfaces())
			store := &accessStore{}
			firewall := &accessFirewall{}
			commissioner := &recordingCommissioner{}
			service := newCommissionableServiceWithCredential(
				t, &now, provider, store, firewall, commissioner, testCase.credential)
			applied := len(firewall.applied)

			_, err := service.Commission(ctx, []string{"enp1s0"}, mustAddr(t, "192.168.10.24/24"))
			if !errors.Is(err, ErrPublishedCredentialUnknown) {
				t.Fatalf("Commission = %v, want ErrPublishedCredentialUnknown", err)
			}
			if commissioner.count() != 0 {
				t.Fatal("an unanswerable gate still recorded the lifecycle state")
			}
			if len(firewall.applied) != applied {
				t.Fatal("an unanswerable gate still reached the firewall")
			}
			if !service.Provisioning() {
				t.Fatal("an unanswerable gate left the appliance no longer provisioning")
			}
		})
	}
}

// refusingCommissioner is the store refusing to record the appliance because
// the published verifier was present at the moment the record would have been
// written. It reports itself the way the real one does, through a method rather
// than a shared sentinel value.
type refusingCommissioner struct{}

func (refusingCommissioner) CompleteCommissioning(context.Context) error {
	return publishedCredentialStillThere{}
}

type publishedCredentialStillThere struct{}

func (publishedCredentialStillThere) Error() string {
	return "the appliance still stores the published administrator verifier"
}

func (publishedCredentialStillThere) PublishedCredentialPresent() bool { return true }

// REQ-SEC-c9e034de: the early check is a fast refusal, not the guarantee. A
// password reset commits on its own transaction, so the published verifier can
// be back by the time the appliance would be recorded as commissioned. The
// commissioner settles both together and refuses; commissioning must report
// that as the same conflict and leave the appliance provisioning.
func TestCommissioningIsRefusedWhenTheCommitFindsThePublishedCredentialBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	provider := &accessProvider{}
	provider.set(provisioningInterfaces())
	store := &accessStore{}
	firewall := &accessFirewall{}

	// The gate sees a changed password; the commit sees it restored.
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Now: func() time.Time { return now },
		Provisioning: true, ProvisioningTLSNames: []string{"ddp-remote.local"},
		Commissioner: refusingCommissioner{}, ConfirmWindow: time.Minute,
		PublishedCredential: &stubPublishedCredential{},
	})
	if err != nil {
		t.Fatalf("NewManagementAccessService: %v", err)
	}

	_, err = service.Commission(ctx, []string{"enp1s0"}, mustAddr(t, "192.168.10.24/24"))
	if !errors.Is(err, ErrPublishedCredentialUnchanged) {
		t.Fatalf("Commission when the commit refuses = %v, want ErrPublishedCredentialUnchanged", err)
	}
	if !service.Provisioning() {
		t.Fatal("a refused commit left the appliance no longer provisioning")
	}
	if got := service.ConfiguredInterfaces(); !slices.Equal(got, []string{"enp1s0", "wlp2s0"}) {
		t.Fatalf("allowlist after the refused commit = %v, want the derived set restored", got)
	}
}
