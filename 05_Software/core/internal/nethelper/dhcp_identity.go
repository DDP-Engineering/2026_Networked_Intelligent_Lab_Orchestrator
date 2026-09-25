package nethelper

import "errors"

type dhcpIdentitySettings struct {
	IPv4Hostname string
	IPv4Vendor   string
	IPv6Hostname string
}

func dhcpIdentityProperties(serialName, ipv4Method, ipv6Method string) (dhcpIdentitySettings, error) {
	if !validPermanentSerialName(serialName) {
		return dhcpIdentitySettings{}, errors.New("valid permanent appliance name is required")
	}
	var result dhcpIdentitySettings
	if ipv4Method == "auto" {
		result.IPv4Hostname = serialName
		result.IPv4Vendor = "NILO"
	}
	if ipv6Method == "auto" {
		result.IPv6Hostname = serialName
	}
	return result, nil
}
