//go:build linux

package nethelper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"strings"
	"sync"
	"time"

	"nilo.local/nilo/internal/network"
)

type linuxCommissioningServices struct {
	mu       sync.Mutex
	dnsmasq  string
	ip       string
	process  *exec.Cmd
	server   *http.Server
	listener net.Listener
	plan     network.CommissioningAccessPoint
}

func NewCommissioningServices() (commissioningServiceDriver, error) {
	binary, err := exec.LookPath("dnsmasq")
	if err != nil {
		return nil, errors.New("dnsmasq is required for commissioning Wi-Fi")
	}
	ip, err := exec.LookPath("ip")
	if err != nil {
		return nil, errors.New("iproute2 is required for commissioning Wi-Fi")
	}
	return &linuxCommissioningServices{dnsmasq: binary, ip: ip}, nil
}

func (services *linuxCommissioningServices) Check(plan network.CommissioningAccessPoint) error {
	subnet := netip.MustParsePrefix("192.168.247.0/24")
	var routes []hostRoute
	interfaces, err := net.Interfaces()
	if err != nil {
		return errors.New("commissioning route inventory is unavailable")
	}
	for _, current := range interfaces {
		addresses, _ := current.Addrs()
		for _, raw := range addresses {
			prefix, parseErr := netip.ParsePrefix(raw.String())
			if parseErr == nil && prefix.Addr().Is4() {
				routes = append(routes, hostRoute{Interface: current.Name, Prefix: prefix.Masked()})
			}
		}
	}
	output, err := exec.Command(services.ip, "-o", "-4", "route", "show", "table", "all").Output()
	if err != nil {
		return errors.New("commissioning route inventory is unavailable")
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "default" || fields[0] == "broadcast" || fields[0] == "local" {
			continue
		}
		prefix, parseErr := netip.ParsePrefix(fields[0])
		if parseErr != nil || !prefix.Addr().Is4() {
			continue
		}
		interfaceName := ""
		for index := 1; index+1 < len(fields); index++ {
			if fields[index] == "dev" {
				interfaceName = fields[index+1]
				break
			}
		}
		routes = append(routes, hostRoute{Interface: interfaceName, Prefix: prefix.Masked()})
	}
	if !commissioningSubnetAvailable(subnet, plan.Interface, routes) {
		return errors.New("commissioning access point unavailable: route_overlap")
	}
	return nil
}

func (services *linuxCommissioningServices) Start(plan network.CommissioningAccessPoint) error {
	services.mu.Lock()
	defer services.mu.Unlock()
	if services.process != nil && services.plan == plan {
		return nil
	}
	services.stopLocked()
	address, _, err := net.SplitHostPort(fmt.Sprintf("192.168.247.1:%d", 80))
	if err != nil || address != "192.168.247.1" {
		return errors.New("invalid commissioning portal address")
	}
	listener, err := net.Listen("tcp4", "192.168.247.1:80")
	if err != nil {
		return errors.New("commissioning portal listener unavailable")
	}
	target := fmt.Sprintf("https://%s.local:%d/", plan.SerialName, plan.Port)
	server := &http.Server{
		Handler: http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.Header().Set("Cache-Control", "no-store")
			http.Redirect(response, request, target, http.StatusTemporaryRedirect)
		}),
		ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 10 * time.Second,
	}
	command := exec.Command(services.dnsmasq,
		"--keep-in-foreground", "--bind-dynamic", "--interface="+plan.Interface,
		"--listen-address=192.168.247.1", "--except-interface=lo", "--no-resolv", "--no-hosts",
		"--dhcp-authoritative", "--dhcp-range=192.168.247.20,192.168.247.240,255.255.255.0,12h",
		"--dhcp-leasefile=/run/nilo/nilo-dnsmasq.leases", "--pid-file=",
		"--address=/#/192.168.247.1",
	)
	if err := command.Start(); err != nil {
		listener.Close()
		return errors.New("commissioning DHCP and DNS unavailable")
	}
	services.process, services.server, services.listener, services.plan = command, server, listener, plan
	go func() {
		_ = command.Wait()
		services.mu.Lock()
		defer services.mu.Unlock()
		if services.process != command {
			return
		}
		_ = server.Close()
		_ = listener.Close()
		services.process, services.server, services.listener = nil, nil, nil
		services.plan = network.CommissioningAccessPoint{}
	}()
	go func() { _ = server.Serve(listener) }()
	return nil
}

func (services *linuxCommissioningServices) Stop(ctx context.Context) error {
	services.mu.Lock()
	defer services.mu.Unlock()
	if services.server != nil {
		shutdown, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = services.server.Shutdown(shutdown)
		cancel()
	}
	services.stopLocked()
	return nil
}

func (services *linuxCommissioningServices) stopLocked() {
	if services.listener != nil {
		_ = services.listener.Close()
	}
	if services.process != nil && services.process.Process != nil {
		_ = services.process.Process.Kill()
	}
	services.process, services.server, services.listener = nil, nil, nil
	services.plan = network.CommissioningAccessPoint{}
}
