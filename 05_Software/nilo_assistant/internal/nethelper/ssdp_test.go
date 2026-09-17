package nethelper

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"nilo.local/nilo/internal/network"
)

type fakeSSDPResponder struct {
	published network.SSDPPresence
	withdrawn int
}

func TestSSDPResponsePolicyRefreshesJittersAndRateBounds(t *testing.T) {
	if ssdpRefreshInterval <= 0 || ssdpRefreshInterval >= 120*time.Second {
		t.Fatalf("refresh interval = %s", ssdpRefreshInterval)
	}
	delay, err := ssdpSearchDelay(5, bytes.NewReader(bytes.Repeat([]byte{0xff}, 16)))
	if err != nil || delay <= 0 || delay > 5*time.Second {
		t.Fatalf("MX jitter = %s, %v", delay, err)
	}
	policy := newSSDPResponsePolicy(time.Second)
	now := time.Unix(1_700_000_000, 0)
	remote := &net.UDPAddr{IP: net.ParseIP("192.0.2.4"), Port: 1900}
	if !policy.Allow("eth0", remote, now) || policy.Allow("eth0", remote, now.Add(100*time.Millisecond)) {
		t.Fatal("same source/interface was not rate bounded")
	}
	if !policy.Allow("wlan0", remote, now.Add(100*time.Millisecond)) ||
		!policy.Allow("eth0", &net.UDPAddr{IP: net.ParseIP("192.0.2.5"), Port: 1900}, now.Add(100*time.Millisecond)) {
		t.Fatal("rate bound leaked across interface or source")
	}
	for index := 0; index < 1024; index++ {
		address := &net.UDPAddr{IP: net.IPv4(198, 51, byte(index/256), byte(index%256)), Port: 1900}
		policy.Allow("eth0", address, now.Add(time.Duration(index)*time.Millisecond))
	}
	if entries := policy.EntryCount(); entries > ssdpRateLimitEntries {
		t.Fatalf("rate limiter retained %d sources, limit %d", entries, ssdpRateLimitEntries)
	}
}

func TestSSDPSocketBoundsPendingResponsesGloballyAndStopsThemOnWithdrawal(t *testing.T) {
	budget := newSSDPResponseBudget()
	first := &ssdpSocket{done: make(chan struct{}), budget: budget}
	second := &ssdpSocket{done: make(chan struct{}), budget: budget}
	for index := 0; index < ssdpPendingResponses; index++ {
		if !first.reserveResponse() {
			t.Fatalf("pending response %d was unexpectedly refused", index)
		}
	}
	if second.reserveResponse() {
		t.Fatal("second interface multiplied the responder-wide pending response cap")
	}
	first.releaseResponse()
	if !second.reserveResponse() {
		t.Fatal("released responder-wide capacity was not reusable by another interface")
	}
	close(first.done)
	first.releaseResponse()
	if first.reserveResponse() {
		t.Fatal("withdrawn socket accepted new delayed work")
	}
}

func TestSSDPCurrentRequiresLiveReceiveLoopAndCurrentAddress(t *testing.T) {
	socket := &ssdpSocket{interfaceName: "eth0", addressCurrent: func() bool { return true }}
	socket.alive.Store(true)
	responder := &udpSSDPResponder{
		presence: network.SSDPPresence{Interfaces: []string{"eth0"}},
		sockets:  []*ssdpSocket{socket},
	}
	if !responder.Current() {
		t.Fatal("live socket with its published address was not current")
	}
	socket.alive.Store(false)
	if responder.Current() {
		t.Fatal("stopped SSDP receive loop remained current")
	}
	socket.alive.Store(true)
	socket.addressCurrent = func() bool { return false }
	if responder.Current() {
		t.Fatal("socket with a replaced interface address remained current")
	}
}

func TestSSDPCurrentRequiresEveryRequestedInterface(t *testing.T) {
	socket := &ssdpSocket{interfaceName: "eth0", addressCurrent: func() bool { return true }}
	socket.alive.Store(true)
	responder := &udpSSDPResponder{
		presence: network.SSDPPresence{Interfaces: []string{"eth0", "wlan0"}},
		sockets:  []*ssdpSocket{socket},
	}
	if responder.Current() {
		t.Fatal("partial SSDP publication remained current")
	}
}

func (responder *fakeSSDPResponder) Publish(_ context.Context, presence network.SSDPPresence) error {
	responder.published = presence
	return nil
}
func (responder *fakeSSDPResponder) Withdraw(context.Context) error {
	responder.withdrawn++
	return nil
}
func (responder *fakeSSDPResponder) Current() bool {
	return responder.published.ApplianceID != "" && responder.withdrawn == 0
}

func TestSSDPProtocolOwnsTypedPresenceLifecycle(t *testing.T) {
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	responder := &fakeSSDPResponder{}
	if err := engine.ConfigureSSDP(responder); err != nil {
		t.Fatal(err)
	}
	presence, _ := network.NewSSDPPresence(network.SSDPPresence{
		ApplianceID: "0199a2d0-1234-7abc-8def-0123456789ab", Version: "0.8.0", Port: 8443,
		Interfaces: []string{"eth0"}, BootID: 1, ConfigID: 1,
	})
	response := dispatch(context.Background(), engine, Request{Op: "ssdp.publish", SSDP: &presence})
	if response.Error != "" || responder.published.ApplianceID != presence.ApplianceID {
		t.Fatalf("publish response=%+v presence=%+v", response, responder.published)
	}
	response = dispatch(context.Background(), engine, Request{Op: "ssdp.status"})
	if response.Error != "" || response.SSDPCurrent == nil || !*response.SSDPCurrent {
		t.Fatalf("status response=%+v", response)
	}
	response = dispatch(context.Background(), engine, Request{Op: "ssdp.withdraw"})
	if response.Error != "" || responder.withdrawn != 1 {
		t.Fatalf("withdraw response=%+v count=%d", response, responder.withdrawn)
	}
}
