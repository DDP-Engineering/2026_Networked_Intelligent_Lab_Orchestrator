package buildinfo

type Info struct {
	Name                string `json:"name"`
	Version             string `json:"version"`
	IntegrationProtocol string `json:"integration_protocol"`
}

// Version mirrors the project-level semantic version declared in the
// repository-root VERSION file.
var Version = "0.2.0"

func Current() Info {
	return Info{Name: "nilo", Version: Version, IntegrationProtocol: "nilo.integration/1"}
}
