package network

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrProfileActive      = errors.New("wireguard_profile_active")
	ErrExternalProfile    = errors.New("wireguard_externally_managed")
	ErrUnsupported        = errors.New("wireguard_unsupported")
	ErrConnectionRequired = errors.New("network_saved_connection_required")
)

// Change carries the allowlisted parameters of an acknowledged operation.
//
// ProfileText is the raw uploaded file and is transient in the strongest sense:
// a preview parses it, stages its key material out of reach, and clears this
// field. Nothing downstream of the preview ever sees it, so it cannot reach a
// stored operation, an audit record, the helper socket or a response.
type Change struct {
	Action       Action `json:"action"`
	Target       string `json:"target"`
	ConnectionID string `json:"connection_id,omitempty"`
	ProfileText  string `json:"profile_text,omitempty"`
	Enabled      bool   `json:"enabled"`
	// Name is the new label a rename assigns. It is a label, never an identity.
	Name string `json:"name,omitempty"`
	// Settings is the proposed adapter configuration for interface.settings.
	Settings *InterfaceSettings `json:"settings,omitempty"`
	// WiFi is the network to join for wifi.join. Its password is transient in
	// the same way ProfileText is: staged at preview and cleared.
	WiFi *WiFiJoin `json:"wifi,omitempty"`
	// Origin is the requesting client's address, which the management-path
	// trace is computed from. It is observed, never supplied by the client.
	Origin string `json:"origin,omitempty"`
	// DeviceNetwork is the proposed decision for device_network.create and
	// device_network.update.
	DeviceNetwork *DeviceNetwork `json:"device_network,omitempty"`

	// Profile is what survives the parse: the safe public configuration. It is
	// a pointer so that Change stays comparable, which the helper's request
	// validation relies on.
	Profile *Profile `json:"profile,omitempty"`
	// SecretHandle is the opaque, single-use identity the privileged keystore
	// knows the staged key material by. It is not a secret and nothing can be
	// derived from it, which is what lets it travel with the operation.
	SecretHandle string `json:"secret_handle,omitempty"`
}

// WiFiJoin is the request to join one wireless network. It has no password
// field: the password is staged through the keystore before the change is
// built, and Change.SecretHandle names it.
type WiFiJoin struct {
	SSID             string       `json:"ssid"`
	Security         WiFiSecurity `json:"security"`
	ConnectAtStartup bool         `json:"connect_at_startup"`
}

func (c Change) String() string {
	return fmt.Sprintf("network change %s on %s (parameters redacted)", c.Action, c.Target)
}

// ChangeController validates and applies typed parameters through the selected
// simulation or helper boundary. Inspect must never change host state.
type ChangeController interface {
	Inspect(context.Context, Change) (map[string]string, error)
	ApplyChange(context.Context, Change) error
}

// ProfileImporter is a controller that creates an imported profile and reports
// the identity it assigned.
//
// It receives the staged key handle in Change.SecretHandle and is responsible
// for binding that material to the identity it returns, atomically with
// creating the profile: the privileged side needs the key to create the
// connection and must never hand it back, so the binding cannot be a separate
// step taken from here. When it fails, the operation manager discards the
// handle.
type ProfileImporter interface {
	ImportProfile(context.Context, Change) (profileID string, err error)
}

// SecretStager keeps an import's key material out of the unprivileged core.
//
// StageSecrets is called by the preview and returns an opaque handle.
// BindSecrets ties that handle to a profile identity and is called only after
// the import has actually happened. DropSecrets is called when it has not.
//
// The names are explicit rather than Stage/Promote/Discard because the same
// object also implements the connectivity changer, which has a Discard of its
// own for recovery checkpoints; two unrelated things called Discard on one type
// is a mistake waiting to be made.
type SecretStager interface {
	StageSecrets(ctx context.Context, secrets Secrets) (handle string, err error)
	BindSecrets(ctx context.Context, handle, profileID string) error
	DropSecrets(ctx context.Context, handle string) error
}

type ToolStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
}

// ProfileStatus includes safe metadata only; its keys are intentionally absent.
type ProfileStatus struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Interface     string     `json:"interface"`
	NILOOwned     bool       `json:"nilo_owned"`
	Active        bool       `json:"active"`
	AutoConnect   bool       `json:"autoconnect"`
	LastHandshake *time.Time `json:"last_handshake"`
	Profile       Profile    `json:"profile"`
}

type WireGuardStatus struct {
	// ReadinessOnly means tunnel inventory and management are not reported.
	ReadinessOnly           bool            `json:"readiness_only,omitempty"`
	Reported                bool            `json:"reported"`
	MeasuredBy              Source          `json:"measured_by"`
	NetworkManagerAvailable bool            `json:"networkmanager_available"`
	NetworkManagerVersion   string          `json:"networkmanager_version"`
	KernelSupported         bool            `json:"kernel_supported"`
	HelperAvailable         bool            `json:"helper_available"`
	CanInstall              bool            `json:"can_install"`
	Tools                   []ToolStatus    `json:"tools"`
	Profiles                []ProfileStatus `json:"profiles"`
	Detail                  string          `json:"detail"`
}

type WireGuardReader interface {
	WireGuard(context.Context) (WireGuardStatus, error)
}

func (service *Service) WireGuard(ctx context.Context) (WireGuardStatus, error) {
	if reader, ok := service.provider.(WireGuardReader); ok {
		return reader.WireGuard(ctx)
	}
	return WireGuardStatus{MeasuredBy: service.provider.Source(), Tools: []ToolStatus{}, Profiles: []ProfileStatus{}, Detail: "WireGuard observation unavailable"}, nil
}
