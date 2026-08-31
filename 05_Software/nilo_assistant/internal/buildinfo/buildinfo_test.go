package buildinfo

import "testing"

func TestCurrentHasProtocolAndVersion(t *testing.T) {
	got := Current()
	if got.Name != "nilo" || got.Version == "" || got.IntegrationProtocol != "nilo.integration/1" {
		t.Fatalf("unexpected build info: %#v", got)
	}
}
