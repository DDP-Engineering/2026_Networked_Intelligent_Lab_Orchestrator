package discovery

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxDiscoveryPacket = 64 << 10

// Discover queries both standard local discovery channels on every eligible
// IPv4 interface and deterministically merges their validated results.
func Discover(ctx context.Context, duration time.Duration) ([]Appliance, []string, error) {
	if duration <= 0 || duration > 60*time.Second {
		return nil, nil, errors.New("discovery duration must be between zero and sixty seconds")
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	var mutex sync.Mutex
	var observations []Observation
	started := 0
	var wait sync.WaitGroup
	for _, current := range interfaces {
		if current.Flags&net.FlagUp == 0 || current.Flags&net.FlagLoopback != 0 || current.Flags&net.FlagMulticast == 0 {
			continue
		}
		rawAddresses, _ := current.Addrs()
		prefixes := make([]netip.Prefix, 0, len(rawAddresses))
		for _, raw := range rawAddresses {
			if prefix, parseErr := netip.ParsePrefix(raw.String()); parseErr == nil {
				prefixes = append(prefixes, prefix)
			}
		}
		for _, address := range eligibleDiscoveryAddresses(prefixes) {
			wait.Add(1)
			go func(name string, address netip.Addr) {
				defer wait.Done()
				found, active := browseMDNS(ctx, name, address)
				mutex.Lock()
				if active {
					started++
				}
				observations = append(observations, found...)
				mutex.Unlock()
			}(current.Name, address)
			if address.Is4() {
				wait.Add(1)
				go func(name string, address netip.Addr) {
					defer wait.Done()
					found, active := searchSSDP(ctx, name, address)
					mutex.Lock()
					if active {
						started++
					}
					observations = append(observations, found...)
					mutex.Unlock()
				}(current.Name, address)
			}
		}
	}
	wait.Wait()
	if started == 0 {
		return nil, nil, errors.New("discovery is unavailable on every eligible interface")
	}
	result, diagnostics := Merge(observations)
	return result, diagnostics, nil
}

func eligibleDiscoveryAddresses(prefixes []netip.Prefix) []netip.Addr {
	var ipv4, ipv6 netip.Addr
	for _, prefix := range prefixes {
		address := prefix.Addr().Unmap()
		if !address.IsValid() || address.IsLoopback() || address.IsUnspecified() {
			continue
		}
		if address.Is4() && !ipv4.IsValid() {
			ipv4 = address
		} else if address.Is6() && !ipv6.IsValid() {
			ipv6 = address
		}
	}
	result := make([]netip.Addr, 0, 2)
	if ipv4.IsValid() {
		result = append(result, ipv4)
	}
	if ipv6.IsValid() {
		result = append(result, ipv6)
	}
	return result
}

func browseMDNS(ctx context.Context, interfaceName string, address netip.Addr) ([]Observation, bool) {
	networkName := "udp4"
	local := &net.UDPAddr{IP: net.IP(address.AsSlice())}
	remote := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	if address.Is6() {
		networkName = "udp6"
		local.Zone = interfaceName
		remote = &net.UDPAddr{IP: net.ParseIP("ff02::fb"), Port: 5353, Zone: interfaceName}
	}
	connection, err := net.ListenUDP(networkName, local)
	if err != nil {
		return nil, false
	}
	defer connection.Close()
	query := make([]byte, 12)
	binary.BigEndian.PutUint16(query[4:6], 1)
	query = append(query, encodeDNSName("_nilo._tcp.local")...)
	query = append(query, 0, 12, 0x80, 1) // PTR with the unicast-response bit.
	if _, err := connection.WriteToUDP(query, remote); err != nil {
		return nil, false
	}
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	buffer := make([]byte, maxDiscoveryPacket)
	var result []Observation
	for {
		if deadline, ok := ctx.Deadline(); ok {
			_ = connection.SetReadDeadline(deadline)
		}
		count, _, readErr := connection.ReadFromUDP(buffer)
		if readErr != nil {
			return result, true
		}
		result = append(result, parseMDNSPacket(buffer[:count], interfaceName)...)
	}
}

func searchSSDP(ctx context.Context, _ string, address netip.Addr) ([]Observation, bool) {
	connection, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IP(address.AsSlice())})
	if err != nil {
		return nil, false
	}
	defer connection.Close()
	request := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: upnp:rootdevice\r\n\r\n"
	if _, err := connection.WriteToUDP([]byte(request), &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}); err != nil {
		return nil, false
	}
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	buffer := make([]byte, maxDiscoveryPacket)
	var result []Observation
	for {
		if deadline, ok := ctx.Deadline(); ok {
			_ = connection.SetReadDeadline(deadline)
		}
		count, remote, readErr := connection.ReadFromUDP(buffer)
		if readErr != nil {
			return result, true
		}
		observation, ok := parseSSDPResponse(buffer[:count], remote.IP.String())
		if !ok {
			continue
		}
		if enriched, ok := readSSDPDescription(ctx, observation); ok {
			result = append(result, enriched)
		}
	}
}

func parseSSDPResponse(packet []byte, remoteAddress string) (Observation, bool) {
	if len(packet) > maxDiscoveryPacket || net.ParseIP(remoteAddress) == nil {
		return Observation{}, false
	}
	lines := strings.Split(strings.ReplaceAll(string(packet), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "HTTP/1.1 200 OK" {
		return Observation{}, false
	}
	headers := map[string]string{}
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found || len(headers) > 32 {
			return Observation{}, false
		}
		headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	if headers["st"] != "upnp:rootdevice" {
		return Observation{}, false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(headers["usn"], "uuid:"), "::upnp:rootdevice")
	location, err := url.Parse(headers["location"])
	if err != nil || location.Scheme != "https" || location.User != nil || location.Path != "/.well-known/nilo-device.xml" || location.RawQuery != "" || location.Fragment != "" {
		return Observation{}, false
	}
	host, port, err := net.SplitHostPort(location.Host)
	if err != nil || net.ParseIP(host) == nil || host != remoteAddress {
		return Observation{}, false
	}
	version := ""
	if server := headers["server"]; strings.HasPrefix(server, "NILO/") && strings.HasSuffix(server, " UPnP/1.1") {
		version = strings.TrimSuffix(strings.TrimPrefix(server, "NILO/"), " UPnP/1.1")
	}
	return Observation{Source: SourceSSDP, ID: id, Version: version, URL: "https://" + net.JoinHostPort(host, port) + "/", Addresses: []string{host}}, true
}

func readSSDPDescription(ctx context.Context, observation Observation) (Observation, bool) {
	requestURL := strings.TrimSuffix(observation.URL, "/") + "/.well-known/nilo-device.xml"
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}} // #nosec G402 -- discovery metadata is validated and carries no credential.
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return Observation{}, false
	}
	response, err := client.Do(request)
	if err != nil || response.StatusCode != http.StatusOK {
		if response != nil {
			response.Body.Close()
		}
		return Observation{}, false
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return Observation{}, false
	}
	enriched, ok := parseSSDPDescription(data, observation)
	if !ok {
		return Observation{}, false
	}
	return enriched, true
}

func parseSSDPDescription(data []byte, observation Observation) (Observation, bool) {
	var root struct {
		Device struct {
			FriendlyName      string   `xml:"friendlyName"`
			SerialNumber      string   `xml:"serialNumber"`
			UDN               string   `xml:"UDN"`
			PresentationURL   string   `xml:"presentationURL"`
			HardwareAddresses []string `xml:"X_NILO_HardwareAddress"`
		} `xml:"device"`
	}
	if xml.Unmarshal(data, &root) != nil || strings.TrimPrefix(root.Device.UDN, "uuid:") != observation.ID {
		return Observation{}, false
	}
	observation.Serial = root.Device.SerialNumber
	observation.HardwareAddresses = append([]string(nil), root.Device.HardwareAddresses...)
	if root.Device.FriendlyName != root.Device.SerialNumber {
		observation.FriendlyName = root.Device.FriendlyName
	}
	presentation, err := url.Parse(root.Device.PresentationURL)
	location, locationErr := url.Parse(observation.URL)
	if err != nil || locationErr != nil || presentation.Scheme != "https" || presentation.User != nil ||
		presentation.RawQuery != "" || presentation.Fragment != "" || presentation.Path != "/" ||
		!strings.EqualFold(presentation.Hostname(), root.Device.SerialNumber+".local") ||
		presentation.Port() != location.Port() {
		return Observation{}, false
	}
	observation.URL = presentation.String()
	if validateObservation(observation) != nil {
		return Observation{}, false
	}
	return observation, true
}

type dnsResource struct {
	name       string
	kind       uint16
	dataOffset int
	dataLength int
}

func parseMDNSPacket(packet []byte, interfaceName string) []Observation {
	if len(packet) < 12 || len(packet) > maxDiscoveryPacket {
		return nil
	}
	questions := int(binary.BigEndian.Uint16(packet[4:6]))
	resources := int(binary.BigEndian.Uint16(packet[6:8])) + int(binary.BigEndian.Uint16(packet[8:10])) + int(binary.BigEndian.Uint16(packet[10:12]))
	offset := 12
	for index := 0; index < questions; index++ {
		_, next, ok := decodeDNSName(packet, offset, 0)
		if !ok || next+4 > len(packet) {
			return nil
		}
		offset = next + 4
	}
	records := make([]dnsResource, 0, resources)
	for index := 0; index < resources; index++ {
		name, next, ok := decodeDNSName(packet, offset, 0)
		if !ok || next+10 > len(packet) {
			return nil
		}
		length := int(binary.BigEndian.Uint16(packet[next+8 : next+10]))
		dataOffset := next + 10
		if dataOffset+length > len(packet) {
			return nil
		}
		records = append(records, dnsResource{name: strings.ToLower(name), kind: binary.BigEndian.Uint16(packet[next : next+2]), dataOffset: dataOffset, dataLength: length})
		offset = dataOffset + length
	}
	addresses := map[string][]string{}
	instances := map[string]bool{}
	txt := map[string]map[string]string{}
	for _, record := range records {
		switch record.kind {
		case 1:
			if record.dataLength == 4 {
				addresses[record.name] = append(addresses[record.name], net.IP(packet[record.dataOffset:record.dataOffset+4]).String())
			}
		case 28:
			if record.dataLength == 16 {
				addresses[record.name] = append(addresses[record.name], net.IP(packet[record.dataOffset:record.dataOffset+16]).String()+"%"+interfaceName)
			}
		case 12:
			if record.name == "_nilo._tcp.local" {
				name, _, ok := decodeDNSName(packet, record.dataOffset, 0)
				if ok {
					instances[strings.ToLower(name)] = true
				}
			}
		case 16:
			values := map[string]string{}
			for cursor, end := record.dataOffset, record.dataOffset+record.dataLength; cursor < end; {
				length := int(packet[cursor])
				cursor++
				if length == 0 || cursor+length > end {
					values = nil
					break
				}
				key, value, found := strings.Cut(string(packet[cursor:cursor+length]), "=")
				if !found || values[key] != "" {
					values = nil
					break
				}
				values[key] = value
				cursor += length
			}
			if values != nil {
				txt[record.name] = values
			}
		}
	}
	var result []Observation
	for _, record := range records {
		if record.kind != 33 || !instances[record.name] || record.dataLength < 7 {
			continue
		}
		port := binary.BigEndian.Uint16(packet[record.dataOffset+4 : record.dataOffset+6])
		target, _, ok := decodeDNSName(packet, record.dataOffset+6, 0)
		values := txt[record.name]
		if !ok || values["txtvers"] != "2" || values["path"] != "/" || port == 0 {
			continue
		}
		if !validMDNSDestination(values["serial"], target, values["url"], port) {
			continue
		}
		observation := Observation{
			Source: SourceMDNS, ID: values["id"], Serial: values["serial"], FriendlyName: values["name"],
			State: values["state"], Version: values["version"], URL: values["url"],
			Addresses: append([]string(nil), addresses[strings.ToLower(target)]...),
		}
		if validateObservation(observation) == nil {
			sort.Strings(observation.Addresses)
			result = append(result, observation)
		}
	}
	return result
}

func validMDNSDestination(serial, target, rawURL string, port uint16) bool {
	if !validSerial(serial) || !strings.EqualFold(target, serial+".local") {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil ||
		!strings.EqualFold(parsed.Hostname(), serial+".local") || parsed.Port() != strconv.Itoa(int(port)) ||
		parsed.Path != "/" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Opaque != "" || parsed.ForceQuery {
		return false
	}
	return true
}

func decodeDNSName(packet []byte, offset, depth int) (string, int, bool) {
	if depth > 16 || offset < 0 || offset >= len(packet) {
		return "", offset, false
	}
	labels := []string{}
	next := offset
	jumped := false
	for {
		if offset >= len(packet) {
			return "", next, false
		}
		length := int(packet[offset])
		if length&0xc0 == 0xc0 {
			if offset+1 >= len(packet) {
				return "", next, false
			}
			pointer := int(binary.BigEndian.Uint16(packet[offset:offset+2]) & 0x3fff)
			name, _, ok := decodeDNSName(packet, pointer, depth+1)
			if !ok {
				return "", next, false
			}
			if name != "" {
				labels = append(labels, name)
			}
			if !jumped {
				next = offset + 2
			}
			break
		}
		if length == 0 {
			if !jumped {
				next = offset + 1
			}
			break
		}
		if length > 63 || offset+1+length > len(packet) {
			return "", next, false
		}
		labels = append(labels, string(packet[offset+1:offset+1+length]))
		offset += 1 + length
	}
	return strings.Join(labels, "."), next, true
}

func encodeDNSName(value string) []byte {
	var result []byte
	for _, label := range strings.Split(value, ".") {
		result = append(result, byte(len(label)))
		result = append(result, label...)
	}
	return append(result, 0)
}
