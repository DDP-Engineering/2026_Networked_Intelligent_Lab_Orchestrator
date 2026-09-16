package discovery

import (
	"encoding/binary"
	"net/netip"
	"strings"
	"testing"
)

func dnsName(value string) []byte {
	result := []byte{}
	start := 0
	for index := 0; index <= len(value); index++ {
		if index != len(value) && value[index] != '.' {
			continue
		}
		result = append(result, byte(index-start))
		result = append(result, value[start:index]...)
		start = index + 1
	}
	return append(result, 0)
}

func TestSSDPDescriptionSelectsCanonicalPresentationURL(t *testing.T) {
	observation := Observation{
		Source: SourceSSDP, ID: "0199a2d0-1234-7abc-8def-0123456789ab", Version: "0.8.0",
		URL: "https://192.168.1.42:8443/", Addresses: []string{"192.168.1.42"},
	}
	document := `<root><device><friendlyName>bench-west</friendlyName><serialNumber>nilo-p2rjmzdkf1</serialNumber>` +
		`<UDN>uuid:0199a2d0-1234-7abc-8def-0123456789ab</UDN>` +
		`<presentationURL>https://nilo-p2rjmzdkf1.local:8443/</presentationURL></device></root>`
	got, ok := parseSSDPDescription([]byte(document), observation)
	if !ok || got.URL != "https://nilo-p2rjmzdkf1.local:8443/" || got.Serial != "nilo-p2rjmzdkf1" {
		t.Fatalf("description = %+v, %t", got, ok)
	}
}

func TestDiscoveryAddressPlanIncludesIPv6OnlyInterfaces(t *testing.T) {
	addresses := eligibleDiscoveryAddresses([]netip.Prefix{
		netip.MustParsePrefix("fe80::1234/64"),
	})
	if len(addresses) != 1 || !addresses[0].Is6() || !strings.EqualFold(addresses[0].String(), "fe80::1234") {
		t.Fatalf("IPv6 discovery addresses = %v", addresses)
	}
}

func dnsRecord(name string, kind uint16, data []byte) []byte {
	result := dnsName(name)
	fields := make([]byte, 10)
	binary.BigEndian.PutUint16(fields[0:2], kind)
	binary.BigEndian.PutUint16(fields[2:4], 1)
	binary.BigEndian.PutUint32(fields[4:8], 120)
	binary.BigEndian.PutUint16(fields[8:10], uint16(len(data)))
	return append(append(result, fields...), data...)
}

func TestParseMDNSAndSSDPObservations(t *testing.T) {
	instance := "NILO nilo-p2rjmzdkf1._nilo._tcp.local"
	txt := []byte{}
	for _, value := range []string{
		"txtvers=2", "id=0199a2d0-1234-7abc-8def-0123456789ab", "serial=nilo-p2rjmzdkf1",
		"name=bench-west", "url=https://nilo-p2rjmzdkf1.local:8443/", "path=/", "state=commissioned", "version=0.8.0",
	} {
		txt = append(txt, byte(len(value)))
		txt = append(txt, value...)
	}
	srv := make([]byte, 6)
	binary.BigEndian.PutUint16(srv[4:6], 8443)
	srv = append(srv, dnsName("nilo-p2rjmzdkf1.local")...)
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[2:4], 0x8400)
	binary.BigEndian.PutUint16(header[6:8], 4)
	packet := append([]byte(nil), header...)
	packet = append(packet, dnsRecord("_nilo._tcp.local", 12, dnsName(instance))...)
	packet = append(packet, dnsRecord(instance, 33, srv)...)
	packet = append(packet, dnsRecord(instance, 16, txt)...)
	packet = append(packet, dnsRecord("nilo-p2rjmzdkf1.local", 1, []byte{192, 168, 1, 42})...)
	observations := parseMDNSPacket(packet, "eth0")
	if len(observations) != 1 || observations[0].FriendlyName != "bench-west" ||
		len(observations[0].Addresses) != 1 || observations[0].Addresses[0] != "192.168.1.42" {
		t.Fatalf("mDNS observations = %+v", observations)
	}

	ssdp := "HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\n" +
		"USN: uuid:0199a2d0-1234-7abc-8def-0123456789ab::upnp:rootdevice\r\n" +
		"LOCATION: https://192.168.1.42:8443/.well-known/nilo-device.xml\r\n" +
		"SERVER: NILO/0.8.0 UPnP/1.1\r\n\r\n"
	observation, ok := parseSSDPResponse([]byte(ssdp), "192.168.1.42")
	if !ok || observation.Source != SourceSSDP || observation.ID == "" || observation.URL != "https://192.168.1.42:8443/" {
		t.Fatalf("SSDP observation = %+v, %t", observation, ok)
	}
	if _, ok := parseSSDPResponse([]byte(ssdp+"X: secret\r\n"), "not-an-ip"); ok {
		t.Fatal("unsafe SSDP response accepted")
	}
}
