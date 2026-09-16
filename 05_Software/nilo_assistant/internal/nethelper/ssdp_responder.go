package nethelper

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"nilo.local/nilo/internal/network"
)

const (
	ssdpRefreshInterval  = 90 * time.Second
	ssdpRateLimitEntries = 256
	ssdpPendingResponses = 64
)

type SSDPResponder interface {
	Publish(context.Context, network.SSDPPresence) error
	Withdraw(context.Context) error
	Current() bool
}

type udpSSDPResponder struct {
	mu       sync.Mutex
	presence network.SSDPPresence
	sockets  []*ssdpSocket
}

type ssdpSocket struct {
	connection    *net.UDPConn
	interfaceName string
	address       string
	presence      network.SSDPPresence
	done          chan struct{}
	policy        *ssdpResponsePolicy
	pending       chan struct{}
}

type ssdpResponsePolicy struct {
	mu       sync.Mutex
	interval time.Duration
	last     map[string]time.Time
}

func newSSDPResponsePolicy(interval time.Duration) *ssdpResponsePolicy {
	return &ssdpResponsePolicy{interval: interval, last: map[string]time.Time{}}
}

func (policy *ssdpResponsePolicy) Allow(interfaceName string, remote *net.UDPAddr, now time.Time) bool {
	if remote == nil || remote.IP == nil {
		return false
	}
	key := interfaceName + "/" + remote.IP.String()
	policy.mu.Lock()
	defer policy.mu.Unlock()
	for candidate, seenAt := range policy.last {
		if now.Sub(seenAt) >= policy.interval {
			delete(policy.last, candidate)
		}
	}
	if previous, present := policy.last[key]; present && now.Sub(previous) < policy.interval {
		return false
	}
	if len(policy.last) >= ssdpRateLimitEntries {
		var oldestKey string
		var oldestTime time.Time
		for candidate, seenAt := range policy.last {
			if oldestKey == "" || seenAt.Before(oldestTime) {
				oldestKey, oldestTime = candidate, seenAt
			}
		}
		delete(policy.last, oldestKey)
	}
	policy.last[key] = now
	return true
}

func (policy *ssdpResponsePolicy) EntryCount() int {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	return len(policy.last)
}

func ssdpSearchDelay(mx int, source io.Reader) (time.Duration, error) {
	if mx < 1 {
		mx = 1
	} else if mx > 5 {
		mx = 5
	}
	var raw [8]byte
	if _, err := io.ReadFull(source, raw[:]); err != nil {
		return 0, err
	}
	maximum := uint64(time.Duration(mx) * time.Second)
	delay := time.Duration(binary.BigEndian.Uint64(raw[:]) % (maximum + 1))
	if delay == 0 {
		delay = time.Nanosecond
	}
	return delay, nil
}

func NewSSDPResponder() SSDPResponder { return &udpSSDPResponder{} }

func (responder *udpSSDPResponder) Publish(ctx context.Context, requested network.SSDPPresence) error {
	presence, err := network.NewSSDPPresence(requested)
	if err != nil {
		return err
	}
	responder.mu.Lock()
	defer responder.mu.Unlock()
	if err := responder.withdrawLocked(); err != nil {
		return err
	}
	for _, interfaceName := range presence.Interfaces {
		link, err := net.InterfaceByName(interfaceName)
		if err != nil {
			responder.withdrawLocked()
			return errors.New("SSDP management interface is unavailable")
		}
		addresses, _ := link.Addrs()
		for _, raw := range addresses {
			ip, _, parseErr := net.ParseCIDR(raw.String())
			if parseErr != nil || ip.To4() == nil || ip.IsLoopback() {
				continue
			}
			connection, listenErr := net.ListenMulticastUDP("udp4", link, &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900})
			if listenErr != nil {
				responder.withdrawLocked()
				return errors.New("SSDP multicast listener unavailable")
			}
			_ = connection.SetReadBuffer(64 << 10)
			socket := &ssdpSocket{
				connection: connection, interfaceName: interfaceName, address: ip.String(), presence: presence,
				done: make(chan struct{}), policy: newSSDPResponsePolicy(time.Second),
				pending: make(chan struct{}, ssdpPendingResponses),
			}
			responder.sockets = append(responder.sockets, socket)
			go socket.run()
			go socket.refresh()
			if message, renderErr := presence.Alive(ip.String()); renderErr == nil {
				_, _ = connection.WriteToUDP([]byte(message), &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900})
			}
			break
		}
	}
	if len(responder.sockets) == 0 {
		return errors.New("SSDP has no IPv4 management address")
	}
	responder.presence = presence
	return nil
}

func (socket *ssdpSocket) run() {
	buffer := make([]byte, 8192)
	for {
		count, remote, err := socket.connection.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		mx, valid := ssdpSearchMX(buffer[:count])
		if !valid || !socket.policy.Allow(socket.interfaceName, remote, time.Now()) {
			continue
		}
		delay, err := ssdpSearchDelay(mx, rand.Reader)
		if err != nil {
			continue
		}
		if !socket.reserveResponse() {
			continue
		}
		remoteCopy := *remote
		time.AfterFunc(delay, func() {
			defer socket.releaseResponse()
			select {
			case <-socket.done:
				return
			default:
			}
			message, err := socket.presence.SearchResponse(socket.address)
			if err == nil {
				_, _ = socket.connection.WriteToUDP([]byte(message), &remoteCopy)
			}
		})
	}
}

func (socket *ssdpSocket) reserveResponse() bool {
	select {
	case <-socket.done:
		return false
	default:
	}
	select {
	case socket.pending <- struct{}{}:
		return true
	default:
		return false
	}
}

func (socket *ssdpSocket) releaseResponse() {
	select {
	case <-socket.pending:
	default:
	}
}

func validSSDPSearch(packet []byte) bool {
	_, valid := ssdpSearchMX(packet)
	return valid
}

func ssdpSearchMX(packet []byte) (int, bool) {
	if len(packet) == 0 || len(packet) > 8192 {
		return 0, false
	}
	lines := strings.Split(strings.ReplaceAll(string(packet), "\r\n", "\n"), "\n")
	if len(lines) < 4 || strings.TrimSpace(lines[0]) != "M-SEARCH * HTTP/1.1" {
		return 0, false
	}
	headers := map[string]string{}
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return 0, false
		}
		headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	mx, err := strconv.Atoi(headers["mx"])
	valid := err == nil && headers["man"] == `"ssdp:discover"` &&
		(headers["st"] == "upnp:rootdevice" || headers["st"] == "ssdp:all")
	if !valid {
		return 0, false
	}
	if mx < 1 {
		mx = 1
	} else if mx > 5 {
		mx = 5
	}
	return mx, true
}

func (socket *ssdpSocket) refresh() {
	ticker := time.NewTicker(ssdpRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-socket.done:
			return
		case <-ticker.C:
			message, err := socket.presence.Alive(socket.address)
			if err == nil {
				_, _ = socket.connection.WriteToUDP([]byte(message), &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900})
			}
		}
	}
}

func (responder *udpSSDPResponder) Withdraw(context.Context) error {
	responder.mu.Lock()
	defer responder.mu.Unlock()
	return responder.withdrawLocked()
}

func (responder *udpSSDPResponder) withdrawLocked() error {
	for _, socket := range responder.sockets {
		_, _ = socket.connection.WriteToUDP([]byte(socket.presence.Byebye()), &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900})
		close(socket.done)
		_ = socket.connection.Close()
	}
	responder.sockets = nil
	responder.presence = network.SSDPPresence{}
	return nil
}

func (responder *udpSSDPResponder) Current() bool {
	responder.mu.Lock()
	defer responder.mu.Unlock()
	return len(responder.sockets) > 0
}
