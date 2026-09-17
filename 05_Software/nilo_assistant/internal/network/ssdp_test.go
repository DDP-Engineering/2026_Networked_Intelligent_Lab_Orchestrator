package network

import (
	"context"
	"strings"
	"testing"
)

type ssdpPublisher struct {
	published []SSDPPresence
	withdrawn int
	current   bool
}

func (publisher *ssdpPublisher) PublishSSDP(_ context.Context, presence SSDPPresence) error {
	publisher.published = append(publisher.published, presence)
	publisher.current = true
	return nil
}
func (publisher *ssdpPublisher) WithdrawSSDP(context.Context) error {
	publisher.withdrawn++
	publisher.current = false
	return nil
}
func (publisher *ssdpPublisher) SSDPPublicationCurrent(context.Context) (bool, error) {
	return publisher.current, nil
}

func TestSSDPAnnouncerFollowsManagementInterfaces(t *testing.T) {
	publisher := &ssdpPublisher{}
	announcer, err := NewSSDPAnnouncer(publisher, SSDPPresence{
		ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", Version: "0.8.0", Port: 8443, BootID: 1, ConfigID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := announcer.Announce(context.Background(), []string{"wlan0", "eth0"}); err != nil {
		t.Fatal(err)
	}
	if err := announcer.Announce(context.Background(), []string{"eth0", "wlan0"}); err != nil {
		t.Fatal(err)
	}
	if len(publisher.published) != 1 || publisher.published[0].Interfaces[0] != "eth0" {
		t.Fatalf("publications = %+v", publisher.published)
	}
	if err := announcer.Announce(context.Background(), nil); err != nil || publisher.withdrawn != 1 {
		t.Fatalf("withdraw=%d err=%v", publisher.withdrawn, err)
	}
}

func TestSSDPAnnouncerRepublishesUnchangedStateAfterHelperLoss(t *testing.T) {
	publisher := &ssdpPublisher{}
	announcer, err := NewSSDPAnnouncer(publisher, SSDPPresence{
		ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", Version: "0.8.0", Port: 8443, BootID: 1, ConfigID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	interfaces := []string{"eth0"}
	if err := announcer.Announce(context.Background(), interfaces); err != nil {
		t.Fatal(err)
	}
	publisher.current = false
	if err := announcer.Announce(context.Background(), interfaces); err != nil {
		t.Fatal(err)
	}
	if len(publisher.published) != 2 {
		t.Fatalf("publications after helper loss = %d, want 2", len(publisher.published))
	}
}

func TestSSDPPresenceRendersExactRootDeviceMessages(t *testing.T) {
	presence, err := NewSSDPPresence(SSDPPresence{
		ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab",
		Version:     "0.8.0",
		Port:        8443,
		Interfaces:  []string{"eth0", "wlan0"},
		BootID:      7,
		ConfigID:    3,
	})
	if err != nil {
		t.Fatal(err)
	}
	alive, err := presence.Alive("192.168.1.42")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"NOTIFY * HTTP/1.1\r\n",
		"HOST: 239.255.255.250:1900\r\n",
		"CACHE-CONTROL: max-age=120\r\n",
		"LOCATION: https://192.168.1.42:8443/.well-known/nilo-device.xml\r\n",
		"NT: upnp:rootdevice\r\n",
		"NTS: ssdp:alive\r\n",
		"USN: uuid:0199a2d0-1234-7abc-8def-0123456789ab::upnp:rootdevice\r\n",
		"BOOTID.UPNP.ORG: 7\r\n",
		"CONFIGID.UPNP.ORG: 3\r\n",
		"SERVER: NILO/0.8.0 UPnP/1.1\r\n\r\n",
	} {
		if !strings.Contains(alive, line) {
			t.Errorf("alive message omitted %q:\n%s", line, alive)
		}
	}
	response, err := presence.SearchResponse("192.168.1.42")
	if err != nil || !strings.HasPrefix(response, "HTTP/1.1 200 OK\r\n") || !strings.Contains(response, "ST: upnp:rootdevice\r\n") {
		t.Fatalf("search response = %q, %v", response, err)
	}
	byebye := presence.Byebye()
	if !strings.Contains(byebye, "NTS: ssdp:byebye\r\n") || strings.Contains(byebye, "LOCATION:") {
		t.Fatalf("invalid byebye message: %s", byebye)
	}
}

func TestSSDPPresenceRejectsUnsafeScopeAndMetadata(t *testing.T) {
	base := SSDPPresence{ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", Version: "0.8.0", Port: 8443, Interfaces: []string{"eth0"}, BootID: 1, ConfigID: 1}
	for _, mutate := range []func(*SSDPPresence){
		func(value *SSDPPresence) { value.ApplianceID = "bad" },
		func(value *SSDPPresence) { value.Version = "bad header\r\nX: injected" },
		func(value *SSDPPresence) { value.Interfaces = nil },
		func(value *SSDPPresence) { value.Interfaces = []string{"lo"} },
		func(value *SSDPPresence) { value.Interfaces = []string{"eth0", "eth0"} },
		func(value *SSDPPresence) { value.BootID = 0 },
	} {
		candidate := base
		mutate(&candidate)
		if _, err := NewSSDPPresence(candidate); err == nil {
			t.Fatalf("accepted unsafe SSDP presence: %+v", candidate)
		}
	}
}
