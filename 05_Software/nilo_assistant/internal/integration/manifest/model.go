// Package manifest loads and validates confined NILO integration packages.
package manifest

import (
	"errors"

	"nilo.local/nilo/internal/policy"
)

const protocolV1 = "nilo.integration/1"

var (
	ErrInvalidManifest      = errors.New("invalid integration manifest")
	ErrUnsupportedProtocol  = errors.New("unsupported integration protocol")
	ErrUnsupportedPlatform  = errors.New("unsupported integration platform")
	ErrDuplicateOperationID = errors.New("duplicate operation ID")
	ErrRemoteReference      = errors.New("remote schema reference is not allowed")
	ErrDocumentTooLarge     = errors.New("manifest document exceeds size limit")
)

// Package identifies and attests an integration package.
type Package struct {
	ID        string `json:"id"`
	Version   string `json:"version"`
	Publisher string `json:"publisher"`
	Integrity string `json:"integrity"`
}

// Platform declares an operating system and architecture the package supports.
type Platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

// Runner deliberately models an executable and arguments separately so no
// manifest value is ever interpreted as a shell command.
type Runner struct {
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments"`
}

// ResourceClaim is a normalized exclusive or shared hardware resource name.
type ResourceClaim string

// Operation describes a manifest-declared command and its approval policy.
type Operation struct {
	ID            string                 `json:"id"`
	Policy        policy.OperationPolicy `json:"policy"`
	InputSchema   string                 `json:"input_schema"`
	ResultSchema  string                 `json:"result_schema"`
	Timeout       string                 `json:"timeout"`
	Cancellable   bool                   `json:"cancellable"`
	StateChanging bool                   `json:"state_changing"`
	Resources     []ResourceClaim        `json:"resources"`
}

type Cancellation struct {
	Supported    bool   `json:"supported"`
	SafeShutdown string `json:"safe_shutdown"`
}

type Event struct {
	ID     string `json:"id"`
	Schema string `json:"schema"`
}

type Artifact struct {
	ID     string `json:"id"`
	Schema string `json:"schema"`
}

type HealthCheck struct {
	ID      string `json:"id"`
	Timeout string `json:"timeout"`
}

// Manifest is the validated, package-root-confined integration contract.
type Manifest struct {
	Package                Package       `json:"package"`
	Protocol               string        `json:"protocol"`
	Platforms              []Platform    `json:"platforms"`
	Runner                 Runner        `json:"runner"`
	AppConfigurationSchema string        `json:"app_configuration_schema"`
	Operations             []Operation   `json:"operations"`
	Cancellation           Cancellation  `json:"cancellation"`
	Events                 []Event       `json:"events"`
	Artifacts              []Artifact    `json:"artifacts"`
	HealthChecks           []HealthCheck `json:"health_checks"`
	ContractFixtures       []string      `json:"contract_fixtures"`
}
