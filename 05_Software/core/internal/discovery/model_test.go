package discovery

import (
	"reflect"
	"testing"
)

func TestMergeObservationsByUUIDDeterministically(t *testing.T) {
	observations := []Observation{
		{Source: SourceSSDP, ID: "0199a2d0-1234-7abc-8def-0123456789ab", Serial: "nilo-p2rjmzdkf1", URL: "https://nilo-p2rjmzdkf1.local/", Addresses: []string{"fe80::2%eth0"}},
		{Source: SourceMDNS, ID: "0199a2d0-1234-7abc-8def-0123456789ab", Serial: "nilo-p2rjmzdkf1", FriendlyName: "bench-west", URL: "https://nilo-p2rjmzdkf1.local/", State: "commissioned", Version: "0.8.0", Addresses: []string{"192.168.1.42", "fe80::2%eth0"}},
		{Source: SourceMDNS, ID: "0199a2d0-1234-7abc-8def-0123456789ac", Serial: "nilo-0123456789", State: "provisioning", Version: "0.8.0", URL: "https://nilo-0123456789.local/", Addresses: []string{"169.254.4.2"}},
	}
	got, diagnostics := Merge(observations)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	want := []Appliance{
		{ID: "0199a2d0-1234-7abc-8def-0123456789ac", Name: "nilo-0123456789", Serial: "nilo-0123456789", State: "provisioning", Version: "0.8.0", URL: "https://nilo-0123456789.local/", Addresses: []string{"169.254.4.2"}, Sources: []Source{SourceMDNS}},
		{ID: "0199a2d0-1234-7abc-8def-0123456789ab", Name: "bench-west", Serial: "nilo-p2rjmzdkf1", FriendlyName: "bench-west", State: "commissioned", Version: "0.8.0", URL: "https://nilo-p2rjmzdkf1.local/", Addresses: []string{"192.168.1.42", "fe80::2%eth0"}, Sources: []Source{SourceMDNS, SourceSSDP}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged appliances:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMergeOmitsConflictingURLAndRejectsUnsafeRecords(t *testing.T) {
	id := "0199a2d0-1234-7abc-8def-0123456789ab"
	got, diagnostics := Merge([]Observation{
		{Source: SourceMDNS, ID: id, Serial: "nilo-p2rjmzdkf1", URL: "https://nilo-p2rjmzdkf1.local/"},
		{Source: SourceSSDP, ID: id, Serial: "nilo-p2rjmzdkf1", URL: "https://192.168.1.7:8443/"},
		{Source: SourceMDNS, ID: "not-a-uuid", Serial: "nilo-p2rjmzdkf1", URL: "https://evil.example/"},
	})
	if len(got) != 1 || got[0].URL != "" {
		t.Fatalf("conflicting URL was guessed: %#v", got)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %v, want invalid record and URL conflict", diagnostics)
	}
}

func TestSelectUniqueMatchesSerialOrFriendlyName(t *testing.T) {
	appliances := []Appliance{
		{ID: "0199a2d0-1234-7abc-8def-0123456789ab", Serial: "nilo-p2rjmzdkf1", FriendlyName: "bench"},
		{ID: "0199a2d0-1234-7abc-8def-0123456789ac", Serial: "nilo-0123456789", FriendlyName: "bench"},
	}
	if selected, err := SelectUnique(appliances, "nilo-p2rjmzdkf1"); err != nil || selected.ID != appliances[0].ID {
		t.Fatalf("serial selection = %+v, %v", selected, err)
	}
	if _, err := SelectUnique(appliances, "bench"); err != ErrAmbiguous {
		t.Fatalf("ambiguous selection error = %v", err)
	}
	if _, err := SelectUnique(appliances, "missing"); err != ErrNotFound {
		t.Fatalf("missing selection error = %v", err)
	}
}
