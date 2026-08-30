package buildinfo

type Info struct {
	Name                string `json:"name"`
	Version             string `json:"version"`
	IntegrationProtocol string `json:"integration_protocol"`
}

var Version = "0.1.0-dev"

func Current() Info {
	return Info{Name: "nilo", Version: Version, IntegrationProtocol: "nilo.integration/1"}
}
