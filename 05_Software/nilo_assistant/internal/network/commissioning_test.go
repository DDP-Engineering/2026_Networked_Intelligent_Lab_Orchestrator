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

func newCommissionableService(
	t *testing.T, now *time.Time, provider *accessProvider, store *accessStore,
	firewall *accessFirewall, commissioner *recordingCommissioner,
) *ManagementAccessService {
	t.Helper()
	service, err := NewManagementAccessService(context.Background(), ManagementAccessOptions{
		Provider: provider, Store: store, Firewall: firewall, Now: func() time.Time { return *now },
		Provisioning: true, ProvisioningTLSNames: []string{"ddp-remote.local"},
		Commissioner: commissioner, ConfirmWindow: time.Minute,
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
