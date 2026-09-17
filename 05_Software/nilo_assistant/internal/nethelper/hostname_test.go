package nethelper

import "testing"

func TestPersonalizeHostnameChangesOnlyConfiguredImageDefault(t *testing.T) {
	writes := []string{}
	changed, err := PersonalizeHostname("nilo-image", []string{"nilo-image", "nilo-appliance"}, "nilo-p2rjmzdkf1", func(value string) error {
		writes = append(writes, value)
		return nil
	})
	if err != nil || !changed || len(writes) != 1 || writes[0] != "nilo-p2rjmzdkf1" {
		t.Fatalf("default personalization = changed %t, writes %v, error %v", changed, writes, err)
	}
	writes = nil
	changed, err = PersonalizeHostname("custom-host", []string{"nilo-image"}, "nilo-p2rjmzdkf1", func(value string) error {
		writes = append(writes, value)
		return nil
	})
	if err != nil || changed || len(writes) != 0 {
		t.Fatalf("custom hostname changed = %t, writes %v, error %v", changed, writes, err)
	}
}

func TestPersonalizeHostnameRejectsUnvalidatedInputs(t *testing.T) {
	for _, test := range []struct {
		current  string
		defaults []string
		serial   string
	}{
		{current: "nilo-image", defaults: []string{"nilo-image", "bad\nname"}, serial: "nilo-p2rjmzdkf1"},
	} {
		if _, err := PersonalizeHostname(test.current, test.defaults, test.serial, func(string) error { return nil }); err == nil {
			t.Fatalf("accepted invalid hostname input: %+v", test)
		}
	}
}
