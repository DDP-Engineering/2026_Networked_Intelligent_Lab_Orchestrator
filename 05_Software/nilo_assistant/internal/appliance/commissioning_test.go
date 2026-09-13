package appliance

import (
	"context"
	"errors"
	"testing"
)

type fixedCommissioningStore struct {
	state CommissioningState
	err   error
}

func (store fixedCommissioningStore) LoadCommissioning(context.Context) (CommissioningState, error) {
	return store.state, store.err
}

// The state has no safe default. Guessing commissioned would silently skip
// first boot on an appliance that needs it, and guessing provisioning would
// widen management exposure on one that does not, so an unrecognized value is
// refused rather than resolved.
func TestCommissioningStateAcceptsOnlyTheTwoRecordedStates(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		state CommissioningState
		valid bool
	}{
		{name: "provisioning", state: CommissioningProvisioning, valid: true},
		{name: "commissioned", state: CommissioningCommissioned, valid: true},
		{name: "empty", state: ""},
		{name: "unknown word", state: "decommissioned"},
		{name: "wrong case", state: "Provisioning"},
		{name: "padded", state: " provisioning "},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := testCase.state.Validate()
			if testCase.valid && err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", testCase.state, err)
			}
			if !testCase.valid && err == nil {
				t.Fatalf("Validate(%q) = nil, want an error", testCase.state)
			}
		})
	}
}

func TestLoadCommissioningReportsTheRecordedState(t *testing.T) {
	t.Parallel()

	for _, state := range []CommissioningState{CommissioningProvisioning, CommissioningCommissioned} {
		loaded, err := LoadCommissioning(context.Background(), fixedCommissioningStore{state: state})
		if err != nil {
			t.Fatalf("LoadCommissioning(%q): %v", state, err)
		}
		if loaded != state {
			t.Fatalf("LoadCommissioning(%q) = %q, want %q", state, loaded, state)
		}
	}
}

func TestLoadCommissioningRefusesAnAbsentOrUnrecognizedState(t *testing.T) {
	t.Parallel()

	missing := errors.New("no commissioning row")
	for _, testCase := range []struct {
		name  string
		store fixedCommissioningStore
	}{
		{name: "store reports no recorded state", store: fixedCommissioningStore{err: missing}},
		{name: "recorded state is empty", store: fixedCommissioningStore{state: ""}},
		{name: "recorded state is unrecognized", store: fixedCommissioningStore{state: "retired"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if _, err := LoadCommissioning(context.Background(), testCase.store); err == nil {
				t.Fatal("LoadCommissioning returned no error for an unusable recorded state")
			}
		})
	}
}

func TestLoadCommissioningRequiresAStore(t *testing.T) {
	t.Parallel()

	if _, err := LoadCommissioning(context.Background(), nil); err == nil {
		t.Fatal("LoadCommissioning returned no error without a store")
	}
}
