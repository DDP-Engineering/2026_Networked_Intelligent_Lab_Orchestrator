package nethelper

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/power"
)

// SecretPayload is the one type in this protocol that carries key material, and
// it exists only on a Request.
//
// network.Secrets deliberately redacts itself when marshalled, which is right
// everywhere except on the single hop that is allowed to carry the value: an
// operator uploads a profile to the unprivileged core over HTTPS, so the core
// unavoidably holds it for a moment, and the socket is the designated boundary
// for handing it to the privileged side. This type makes that hop explicit and
// auditable rather than reachable by accident.
//
// It has no String or MarshalJSON of its own on purpose: a redacting encoder
// here would silently store the word "redacted" as the key.
type SecretPayload struct {
	PrivateKey    string   `json:"private_key,omitempty"`
	PresharedKeys []string `json:"preshared_keys,omitempty"`
	// Password is a wireless network password. A payload carries either the
	// tunnel keys or a password.
	Password string `json:"password,omitempty"`
}

// Request is deliberately closed: no command, path, environment or raw payload.
type Request struct {
	Op       string         `json:"op"`
	ID       string         `json:"id,omitempty"`
	Change   network.Change `json:"change,omitempty"`
	Revision string         `json:"revision,omitempty"`

	// Secret is accepted only by wireguard.secrets.stage. Every other operation
	// refuses a request that carries one, so key material cannot ride along on
	// an operation that has no business with it.
	Secret *SecretPayload `json:"secret,omitempty"`
	// Handle names staged key material. ProfileID is what it gets bound to.
	Handle    string `json:"handle,omitempty"`
	ProfileID string `json:"profile_id,omitempty"`
	// ConnectionRef names a NetworkManager connection the helper created, for
	// deleting it or connecting the startup selection.
	ConnectionRef string `json:"connection_ref,omitempty"`
	// Plan is the forwarding to install; only forwarding.apply carries one.
	Plan *network.ForwardingPlan `json:"plan,omitempty"`
	// DeviceNetworkID names the forwarding to remove.
	DeviceNetworkID string `json:"device_network_id,omitempty"`
	// ManagementAccess is accepted only by management.access.apply. It carries
	// validated public interface names and a port, never firewall syntax.
	ManagementAccess *network.ManagementAccessConfiguration `json:"management_access,omitempty"`
	// MDNS is accepted only by mdns.publish. It carries a validated host label
	// and interface names, never responder configuration syntax.
	MDNS *network.MDNSAdvertisement `json:"mdns,omitempty"`
	SSDP *network.SSDPPresence      `json:"ssdp,omitempty"`
	// AdapterID and InterfaceName are accepted together only by recovery link
	// activation. RecoveryRecordID is the opaque owned link returned by it.
	AdapterID                string                            `json:"adapter_id,omitempty"`
	InterfaceName            string                            `json:"interface_name,omitempty"`
	RecoveryRecordID         string                            `json:"recovery_record_id,omitempty"`
	RecoveryExpiresAt        time.Time                         `json:"recovery_expires_at,omitempty"`
	HardwareEventID          string                            `json:"hardware_event_id,omitempty"`
	Indicator                *HardwareIndicatorState           `json:"indicator,omitempty"`
	CommissioningLinks       *[]network.CommissioningLink      `json:"commissioning_links,omitempty"`
	CommissioningAccessPoint *network.CommissioningAccessPoint `json:"commissioning_access_point,omitempty"`
	TLSNames                 *[]string                         `json:"tls_names,omitempty"`
	TLSGeneration            string                            `json:"tls_generation,omitempty"`
	TLSDigest                []byte                            `json:"tls_digest,omitempty"`
	TLSHash                  string                            `json:"tls_hash,omitempty"`
	TLSAlgorithm             string                            `json:"tls_algorithm,omitempty"`
}

// Response has no field that can hold key material, and must never gain one.
// A handle is not a secret: it is a random identity with no relationship to the
// value it stands for.
type Response struct {
	Error       string                   `json:"error,omitempty"`
	Code        string                   `json:"code,omitempty"`
	Devices     []Device                 `json:"devices,omitempty"`
	Device      *Device                  `json:"device,omitempty"`
	Change      *network.Change          `json:"change,omitempty"`
	Record      *Record                  `json:"record,omitempty"`
	WireGuard   *network.WireGuardStatus `json:"wireguard,omitempty"`
	InstallPlan *InstallPlan             `json:"install_plan,omitempty"`
	Install     *InstallRecord           `json:"install,omitempty"`
	Handle      string                   `json:"handle,omitempty"`
	// Tunnels is the WireGuard inventory: public configuration and state only.
	Tunnels []network.TunnelObservation `json:"tunnels,omitempty"`
	// ConnectionRef is the NetworkManager connection a profile creation made.
	ConnectionRef string `json:"connection_ref,omitempty"`
	// Networks is a wireless scan: names, security and strength only.
	Networks []network.WiFiNetwork `json:"networks,omitempty"`
	// Plans is the installed device-network forwarding.
	Plans []network.ForwardingPlan `json:"plans,omitempty"`
	// RecoveryRecordID names only a helper-owned volatile recovery link.
	RecoveryRecordID  string                 `json:"recovery_record_id,omitempty"`
	HardwareEvent     *HardwareRecoveryEvent `json:"hardware_event,omitempty"`
	MDNSCurrent       *bool                  `json:"mdns_current,omitempty"`
	SSDPCurrent       *bool                  `json:"ssdp_current,omitempty"`
	ApplianceIdentity *ApplianceIdentity     `json:"appliance_identity,omitempty"`
	TLSIdentity       *TLSIdentityPublic     `json:"tls_identity,omitempty"`
	TLSSignature      []byte                 `json:"tls_signature,omitempty"`
}

// secretOperations are the only operations that may name staged key material.
// Profile creation is among them because it consumes the staged handle;
// deletion names the profile whose key material it removes.
var secretOperations = map[string]bool{
	"wireguard.secrets.stage":   true,
	"wireguard.secrets.promote": true,
	"wireguard.secrets.discard": true,
	"wireguard.profile.create":  true,
	"wireguard.profile.delete":  true,
}

// connectionOperations are the only operations that may name a connection.
var connectionOperations = map[string]bool{
	"wireguard.profile.delete": true,
	"wireguard.startup":        true,
}

func dispatch(ctx context.Context, e *Engine, q Request) Response {
	var r Response
	var err error
	if q.Op != "wireguard.install.start" && q.Revision != "" {
		return Response{Error: "unexpected revision", Code: "invalid_request"}
	}
	if q.Op != "change.inspect" && q.Op != "checkpoint.create" && q.Op != "wireguard.profile.create" && q.Op != "wifi.scan" && q.Change != (network.Change{}) {
		return Response{Error: "unexpected change parameters", Code: "invalid_request"}
	}
	if q.Secret != nil && q.Op != "wireguard.secrets.stage" {
		return Response{Error: "unexpected key material", Code: "invalid_request"}
	}
	if (q.Handle != "" || q.ProfileID != "") && !secretOperations[q.Op] {
		return Response{Error: "unexpected key material reference", Code: "invalid_request"}
	}
	if q.ConnectionRef != "" && !connectionOperations[q.Op] {
		return Response{Error: "unexpected connection reference", Code: "invalid_request"}
	}
	if q.Plan != nil && q.Op != "forwarding.apply" {
		return Response{Error: "unexpected forwarding plan", Code: "invalid_request"}
	}
	if q.DeviceNetworkID != "" && q.Op != "forwarding.remove" {
		return Response{Error: "unexpected device network reference", Code: "invalid_request"}
	}
	if q.ManagementAccess != nil && q.Op != "management.access.apply" {
		return Response{Error: "unexpected management access configuration", Code: "invalid_request"}
	}
	if q.MDNS != nil && q.Op != "mdns.publish" {
		return Response{Error: "unexpected mDNS advertisement", Code: "invalid_request"}
	}
	if q.SSDP != nil && q.Op != "ssdp.publish" {
		return Response{Error: "unexpected SSDP presence", Code: "invalid_request"}
	}
	if (q.AdapterID != "" || q.InterfaceName != "") && q.Op != "recovery.link.activate" {
		return Response{Error: "unexpected recovery adapter identity", Code: "invalid_request"}
	}
	if q.RecoveryRecordID != "" && q.Op != "recovery.link.restore" && q.Op != "recovery.link.extend" {
		return Response{Error: "unexpected recovery record identity", Code: "invalid_request"}
	}
	if !q.RecoveryExpiresAt.IsZero() && q.Op != "recovery.link.activate" && q.Op != "recovery.link.extend" {
		return Response{Error: "unexpected recovery expiry", Code: "invalid_request"}
	}
	if q.HardwareEventID != "" && q.Op != "hardware.recovery.ack" {
		return Response{Error: "unexpected hardware event identity", Code: "invalid_request"}
	}
	if q.Indicator != nil && q.Op != "hardware.indicator.set" {
		return Response{Error: "unexpected hardware indicator", Code: "invalid_request"}
	}
	if q.CommissioningLinks != nil && q.Op != "commissioning.links.reconcile" {
		return Response{Error: "unexpected commissioning links", Code: "invalid_request"}
	}
	if q.CommissioningAccessPoint != nil && q.Op != "commissioning.ap.reconcile" {
		return Response{Error: "unexpected commissioning access point", Code: "invalid_request"}
	}
	if q.TLSNames != nil && q.Op != "tls.identity.activate" {
		return Response{Error: "unexpected TLS names", Code: "invalid_request"}
	}
	if (q.TLSGeneration != "" || len(q.TLSDigest) != 0 || q.TLSHash != "" || q.TLSAlgorithm != "") && q.Op != "tls.identity.sign" {
		return Response{Error: "unexpected TLS signing parameters", Code: "invalid_request"}
	}
	switch q.Op {
	case "appliance.identity.read":
		if q.ID != "" {
			return Response{Error: "appliance identity read requires an empty envelope", Code: "invalid_request"}
		}
		var identity ApplianceIdentity
		identity, err = e.ApplianceIdentity()
		if err == nil {
			r.ApplianceIdentity = &identity
		}
	case "tls.identity.read":
		if q.ID != "" {
			return Response{Error: "TLS identity read requires an empty envelope", Code: "invalid_request"}
		}
		var identity TLSIdentityPublic
		identity, err = e.ReadTLSIdentity()
		if err == nil {
			r.TLSIdentity = &identity
		}
	case "tls.identity.activate":
		if q.ID != "" || q.TLSNames == nil {
			return Response{Error: "TLS identity activation requires a name set", Code: "invalid_request"}
		}
		var identity TLSIdentityPublic
		identity, err = e.ActivateTLSNames(ctx, *q.TLSNames)
		if err == nil {
			r.TLSIdentity = &identity
		}
	case "tls.identity.sign":
		if q.ID != "" || q.TLSGeneration == "" || len(q.TLSDigest) != sha256.Size || q.TLSHash != "sha256" || q.TLSAlgorithm != "ecdsa-p256-sha256" {
			return Response{Error: "TLS signing requires the current P-256 generation and one SHA-256 digest", Code: "invalid_request"}
		}
		r.TLSSignature, err = e.SignTLS(q.TLSGeneration, q.TLSDigest)
	case "management.access.apply":
		if q.ID != "" || q.ManagementAccess == nil {
			return Response{Error: "management access requires a configuration and nothing else", Code: "invalid_request"}
		}
		err = e.ApplyManagementAccess(ctx, *q.ManagementAccess)
	case "mdns.publish":
		if q.ID != "" || q.MDNS == nil {
			return Response{Error: "mDNS publication requires an advertisement and nothing else", Code: "invalid_request"}
		}
		err = e.PublishMDNS(ctx, *q.MDNS)
	case "mdns.withdraw":
		if q.ID != "" || q.MDNS != nil {
			return Response{Error: "mDNS withdrawal requires an empty envelope", Code: "invalid_request"}
		}
		err = e.WithdrawMDNS(ctx)
	case "mdns.status":
		if q.ID != "" || q.MDNS != nil {
			return Response{Error: "mDNS status requires an empty envelope", Code: "invalid_request"}
		}
		var current bool
		current, err = e.MDNSPublicationCurrent(ctx)
		r.MDNSCurrent = &current
	case "ssdp.publish":
		if q.ID != "" || q.SSDP == nil {
			return Response{Error: "SSDP publication requires typed presence", Code: "invalid_request"}
		}
		err = e.PublishSSDP(ctx, *q.SSDP)
	case "ssdp.withdraw":
		if q.ID != "" || q.SSDP != nil {
			return Response{Error: "SSDP withdrawal requires an empty envelope", Code: "invalid_request"}
		}
		err = e.WithdrawSSDP(ctx)
	case "ssdp.status":
		if q.ID != "" || q.SSDP != nil {
			return Response{Error: "SSDP status requires an empty envelope", Code: "invalid_request"}
		}
		current := e.SSDPCurrent()
		r.SSDPCurrent = &current
	case "recovery.link.activate":
		if q.ID != "" || q.AdapterID == "" || q.InterfaceName == "" || q.RecoveryRecordID != "" || q.RecoveryExpiresAt.IsZero() {
			return Response{Error: "recovery activation requires one adapter identity and interface", Code: "invalid_request"}
		}
		r.RecoveryRecordID, err = e.ActivateRecoveryLink(ctx, q.AdapterID, q.InterfaceName, q.RecoveryExpiresAt)
	case "recovery.link.extend":
		if q.ID != "" || q.RecoveryRecordID == "" || q.AdapterID != "" || q.InterfaceName != "" || q.RecoveryExpiresAt.IsZero() {
			return Response{Error: "recovery extension requires one owned record identity and expiry", Code: "invalid_request"}
		}
		err = e.ExtendRecoveryLink(ctx, q.RecoveryRecordID, q.RecoveryExpiresAt)
	case "recovery.link.restore":
		if q.ID != "" || q.RecoveryRecordID == "" || q.AdapterID != "" || q.InterfaceName != "" || !q.RecoveryExpiresAt.IsZero() {
			return Response{Error: "recovery restoration requires one owned record identity", Code: "invalid_request"}
		}
		err = e.RestoreRecoveryLink(ctx, q.RecoveryRecordID)
	case "hardware.recovery.read":
		if q.ID != "" || q.HardwareEventID != "" || q.Indicator != nil {
			return Response{Error: "hardware event read requires an empty envelope", Code: "invalid_request"}
		}
		var event HardwareRecoveryEvent
		var present bool
		event, present, err = e.ReadRecoveryEvent(ctx)
		if present {
			r.HardwareEvent = &event
		}
	case "hardware.recovery.ack":
		if q.ID != "" || q.HardwareEventID == "" || q.Indicator != nil {
			return Response{Error: "hardware event acknowledgement requires one event identity", Code: "invalid_request"}
		}
		err = e.AcknowledgeRecoveryEvent(ctx, q.HardwareEventID)
	case "hardware.indicator.set":
		if q.ID != "" || q.Indicator == nil || q.HardwareEventID != "" {
			return Response{Error: "hardware indicator requires one bounded state", Code: "invalid_request"}
		}
		err = e.SetRecoveryIndicator(ctx, *q.Indicator)
	case "commissioning.links.reconcile":
		if q.ID != "" || q.CommissioningLinks == nil {
			return Response{Error: "commissioning reconciliation requires a typed link list", Code: "invalid_request"}
		}
		err = e.ReconcileCommissioningLinks(ctx, *q.CommissioningLinks)
	case "commissioning.ap.reconcile":
		if q.ID != "" || q.CommissioningAccessPoint == nil {
			return Response{Error: "commissioning access point reconciliation requires one typed plan", Code: "invalid_request"}
		}
		err = e.ReconcileCommissioningAccessPoint(ctx, *q.CommissioningAccessPoint)
	case "system.restart":
		if !emptyPowerRequest(q) {
			return Response{Error: "power operation requires an empty envelope", Code: "invalid_request"}
		}
		err = e.Restart(ctx)
	case "system.power_off":
		if !emptyPowerRequest(q) {
			return Response{Error: "power operation requires an empty envelope", Code: "invalid_request"}
		}
		err = e.PowerOff(ctx)
	case "forwarding.apply":
		if q.ID != "" || q.Plan == nil {
			return Response{Error: "forwarding requires the plan and nothing else", Code: "invalid_request"}
		}
		err = e.ApplyForwarding(ctx, *q.Plan)
	case "forwarding.remove":
		if q.ID != "" || q.DeviceNetworkID == "" {
			return Response{Error: "removal names the device network and nothing else", Code: "invalid_request"}
		}
		err = e.RemoveForwarding(ctx, q.DeviceNetworkID)
	case "forwarding.inventory":
		if q.ID != "" {
			return Response{Error: "unexpected identity", Code: "invalid_request"}
		}
		r.Plans, err = e.AppliedForwarding(ctx)
	case "wireguard.profile.create":
		if q.ID != "" || q.Handle == "" || q.ProfileID == "" || q.Change.Action != network.ActionProfileImport || q.Change.Profile == nil || q.Change.ProfileText != "" {
			return Response{Error: "profile creation requires a staged handle, a profile identity and the parsed profile", Code: "invalid_request"}
		}
		r.ConnectionRef, err = e.CreateProfile(ctx, q.Handle, q.ProfileID, q.Change)
	case "wireguard.profile.delete":
		if q.ID != "" || q.Handle != "" || q.ProfileID == "" || q.ConnectionRef == "" {
			return Response{Error: "profile deletion requires the profile identity and its connection", Code: "invalid_request"}
		}
		err = e.DeleteProfile(ctx, q.ProfileID, q.ConnectionRef)
	case "wireguard.inventory":
		if q.ID != "" {
			return Response{Error: "unexpected identity", Code: "invalid_request"}
		}
		r.Tunnels, err = e.Tunnels(ctx)
	case "wifi.scan":
		if q.ID != "" || q.Change.Target == "" || q.Change.Action != network.ActionWiFiJoin || q.Change.WiFi != nil || q.Change.Settings != nil {
			return Response{Error: "a scan names the wireless adapter and nothing else", Code: "invalid_request"}
		}
		r.Networks, err = e.WiFiScan(ctx, q.Change.Target)
	case "wireguard.startup":
		if q.ID != "" || q.ConnectionRef == "" {
			return Response{Error: "startup requires the selected connection", Code: "invalid_request"}
		}
		err = e.ConnectStartup(ctx, q.ConnectionRef)
	case "wireguard.secrets.stage":
		if q.Secret == nil {
			return Response{Error: "staging requires key material", Code: "invalid_request"}
		}
		if q.ID != "" || q.Handle != "" || q.ProfileID != "" {
			return Response{Error: "unexpected identity", Code: "invalid_request"}
		}
		r.Handle, err = e.StageSecrets(*q.Secret)
	case "wireguard.secrets.promote":
		if q.ID != "" {
			return Response{Error: "unexpected identity", Code: "invalid_request"}
		}
		err = e.PromoteSecrets(q.Handle, q.ProfileID)
	case "wireguard.secrets.discard":
		if q.ID != "" || q.ProfileID != "" {
			return Response{Error: "unexpected identity", Code: "invalid_request"}
		}
		err = e.DiscardSecrets(q.Handle)
	case "wireguard.install.inspect":
		if q.ID != "" {
			return Response{Error: "unexpected identity", Code: "invalid_request"}
		}
		var plan InstallPlan
		plan, err = e.InstallPlan(ctx)
		r.InstallPlan = &plan
	case "wireguard.install.start":
		var record InstallRecord
		record, err = e.StartInstall(ctx, q.ID, q.Revision)
		r.Install = &record
	case "wireguard.install.read":
		var record InstallRecord
		record, err = e.InstallStatus(q.ID)
		r.Install = &record
	case "wireguard.check":
		if q.ID != "" {
			return Response{Error: "unexpected operation identity", Code: "invalid_request"}
		}
		var status network.WireGuardStatus
		status, err = e.WireGuard(ctx)
		r.WireGuard = &status
	case "interface.list":
		r.Devices, err = e.Devices(ctx)
	case "change.inspect":
		var d Device
		var c network.Change
		d, c, err = e.Inspect(ctx, q.Change)
		r.Device = &d
		r.Change = &c
	case "checkpoint.create":
		var record Record
		record, err = e.Prepare(ctx, q.ID, q.Change, network.ConfirmationWindow)
		r.Record = &record
	case "change.apply":
		err = e.Apply(ctx, q.ID)
	case "checkpoint.rollback":
		err = e.Restore(ctx, q.ID)
	case "checkpoint.destroy":
		err = e.Confirm(ctx, q.ID)
	case "recovery.reconcile":
		err = e.Reconcile(ctx)
	case "recovery.read":
		if record, ok := e.Recovery(); ok {
			r.Record = &record
		}
	default:
		return Response{Error: "unsupported helper operation", Code: "invalid_request"}
	}
	if err != nil {
		// Some failed operations return an owned resource identity that the
		// caller must retain for exact cleanup. Preserve typed response fields
		// while adding the bounded error envelope.
		r.Error = err.Error()
		r.Code = "helper_failure"
		for code, candidate := range knownErrors {
			if errors.Is(err, candidate) {
				r.Code = code
				break
			}
		}
	}
	return r
}

func emptyPowerRequest(q Request) bool {
	return q.ID == "" && q.Change == (network.Change{}) && q.Revision == "" && q.Secret == nil &&
		q.Handle == "" && q.ProfileID == "" && q.ConnectionRef == "" && q.Plan == nil && q.DeviceNetworkID == "" && q.ManagementAccess == nil &&
		q.MDNS == nil && q.AdapterID == "" && q.InterfaceName == "" && q.RecoveryRecordID == ""
	// Hardware fields are checked separately by their only operations.
}

var knownErrors = map[string]error{"preview_stale": network.ErrPreviewStale, "not_controllable": network.ErrNotControllable, "unsupported": network.ErrUnsupported, "connection_required": network.ErrConnectionRequired, "busy": network.ErrChangeInProgress, "recovery_unavailable": network.ErrRecoveryUnavailable, "operation_not_found": network.ErrOperationNotFound, "not_awaiting_confirmation": network.ErrNotAwaitingConfirm, "external_profile": network.ErrExternalProfile, "profile_active": network.ErrProfileActive, "secrets_not_found": ErrSecretsNotFound, "interface_settings_rejected": network.ErrInterfaceSettingsRejected, "forwarding_foreign": ErrForwardingForeign, "forwarding_unsupported": ErrForwardingUnsupported, "management_access_foreign": ErrManagementAccessForeign, "management_access_unsupported": ErrManagementAccessUnsupported, "mdns_foreign": ErrMDNSForeign, "mdns_unsupported": ErrMDNSUnsupported, "direct_recovery_unavailable": ErrDirectRecoveryUnsupported, "commissioning_links_unavailable": ErrCommissioningLinkUnsupported, "power_unavailable": power.ErrUnavailable}
