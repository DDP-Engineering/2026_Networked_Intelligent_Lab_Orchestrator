package nethelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/power"
)

// Client is the unprivileged service's typed helper adapter. A simulation never
// constructs one. Its address comes only from local process configuration.
type Client struct {
	socket   string
	source   network.Source
	mu       sync.Mutex
	armed    string
	previous map[string]rateSample
}
type rateSample struct {
	at     time.Time
	rx, tx uint64
}

func NewClient(socket string, source network.Source) *Client {
	return &Client{socket: socket, source: source, previous: map[string]rateSample{}}
}

// Restart asks the authenticated privileged helper for an orderly reboot.
func (c *Client) Restart(ctx context.Context) error {
	_, err := c.call(ctx, Request{Op: "system.restart"})
	return err
}

// PowerOff asks the authenticated privileged helper for an orderly shutdown.
func (c *Client) PowerOff(ctx context.Context) error {
	_, err := c.call(ctx, Request{Op: "system.power_off"})
	return err
}

var _ power.Controller = (*Client)(nil)

// ApplyManagementAccess asks the privileged helper to install the typed
// interface-and-port policy. The core receives no network-administration
// capability and cannot submit raw firewall syntax.
func (c *Client) ApplyManagementAccess(ctx context.Context, configuration network.ManagementAccessConfiguration) error {
	_, err := c.call(ctx, Request{Op: "management.access.apply", ManagementAccess: &configuration})
	return err
}

var _ network.ManagementAccessFirewall = (*Client)(nil)

// PublishMDNS asks the privileged helper to answer the appliance's local name
// on the given interfaces. It sends the validated advertisement, never
// responder configuration.
func (c *Client) PublishMDNS(ctx context.Context, advertisement network.MDNSAdvertisement) error {
	_, err := c.call(ctx, Request{Op: "mdns.publish", MDNS: &advertisement})
	return err
}

func (c *Client) WithdrawMDNS(ctx context.Context) error {
	_, err := c.call(ctx, Request{Op: "mdns.withdraw"})
	return err
}

func (c *Client) MDNSPublicationCurrent(ctx context.Context) (bool, error) {
	response, err := c.call(ctx, Request{Op: "mdns.status"})
	if err != nil || response.MDNSCurrent == nil {
		return false, err
	}
	return *response.MDNSCurrent, nil
}

func (c *Client) PublishSSDP(ctx context.Context, presence network.SSDPPresence) error {
	_, err := c.call(ctx, Request{Op: "ssdp.publish", SSDP: &presence})
	return err
}

func (c *Client) WithdrawSSDP(ctx context.Context) error {
	_, err := c.call(ctx, Request{Op: "ssdp.withdraw"})
	return err
}

func (c *Client) SSDPPublicationCurrent(ctx context.Context) (bool, error) {
	response, err := c.call(ctx, Request{Op: "ssdp.status"})
	if err != nil || response.SSDPCurrent == nil {
		return false, err
	}
	return *response.SSDPCurrent, nil
}

// ActivateRecoveryLink asks the helper to perform its independent safety
// checks and create one volatile link-local profile.
func (c *Client) ActivateRecoveryLink(ctx context.Context, adapterID, interfaceName string, expiresAt time.Time) (string, error) {
	response, err := c.call(ctx, Request{
		Op: "recovery.link.activate", AdapterID: adapterID, InterfaceName: interfaceName,
		RecoveryExpiresAt: expiresAt,
	})
	return response.RecoveryRecordID, err
}

func (c *Client) ExtendRecoveryLink(ctx context.Context, recordID string, expiresAt time.Time) error {
	_, err := c.call(ctx, Request{
		Op: "recovery.link.extend", RecoveryRecordID: recordID, RecoveryExpiresAt: expiresAt,
	})
	return err
}

// RestoreRecoveryLink removes only the exact helper-owned volatile profile.
func (c *Client) RestoreRecoveryLink(ctx context.Context, recordID string) error {
	_, err := c.call(ctx, Request{Op: "recovery.link.restore", RecoveryRecordID: recordID})
	return err
}

var _ network.HardwareRecoveryDriver = (*Client)(nil)

func (c *Client) ReadRecoveryEvent(ctx context.Context) (HardwareRecoveryEvent, bool, error) {
	response, err := c.call(ctx, Request{Op: "hardware.recovery.read"})
	if err != nil || response.HardwareEvent == nil {
		return HardwareRecoveryEvent{}, false, err
	}
	return *response.HardwareEvent, true, nil
}

func (c *Client) AcknowledgeRecoveryEvent(ctx context.Context, eventID string) error {
	_, err := c.call(ctx, Request{Op: "hardware.recovery.ack", HardwareEventID: eventID})
	return err
}

func (c *Client) SetRecoveryIndicator(ctx context.Context, state HardwareIndicatorState) error {
	_, err := c.call(ctx, Request{Op: "hardware.indicator.set", Indicator: &state})
	return err
}

func (c *Client) ExecuteIdentifyAction(
	ctx context.Context, request network.IdentifyActionRequest,
) (network.IdentifyActionResult, error) {
	response, err := c.call(ctx, Request{Op: "identify.execute", IdentifyAction: &request})
	if err != nil {
		return network.IdentifyActionResult{}, err
	}
	if response.IdentifyAction == nil {
		return network.IdentifyActionResult{}, errors.New("helper returned no Identify action result")
	}
	return *response.IdentifyAction, nil
}

func (c *Client) AcknowledgeIdentifyAction(ctx context.Context, operationID string) error {
	_, err := c.call(ctx, Request{Op: "identify.ack", ID: operationID})
	return err
}

var _ network.IdentifyActionExecutor = (*Client)(nil)

func (c *Client) ReconcileCommissioningLinks(ctx context.Context, links []network.CommissioningLink) error {
	copyOfLinks := make([]network.CommissioningLink, len(links))
	copy(copyOfLinks, links)
	_, err := c.call(ctx, Request{Op: "commissioning.links.reconcile", CommissioningLinks: &copyOfLinks})
	return err
}

func (c *Client) ReconcileCommissioningAccessPoint(ctx context.Context, plan network.CommissioningAccessPoint) error {
	_, err := c.call(ctx, Request{Op: "commissioning.ap.reconcile", CommissioningAccessPoint: &plan})
	return err
}

var _ network.CommissioningLinkDriver = (*Client)(nil)

func (c *Client) call(ctx context.Context, q Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.socket)
	if err != nil {
		return Response{}, fmt.Errorf("privileged helper unavailable: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if err = json.NewEncoder(conn).Encode(q); err != nil {
		return Response{}, err
	}
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return Response{}, errors.New("helper requires a Unix connection")
	}
	if err = unixConn.CloseWrite(); err != nil {
		return Response{}, err
	}
	var r Response
	if err = json.NewDecoder(io.LimitReader(conn, 1<<20)).Decode(&r); err != nil {
		return r, err
	}
	if r.Error != "" {
		if r.Code == "mdns_collision" {
			return r, network.NewMDNSCollisionError(r.MDNSCollisionInterfaces)
		}
		if known := knownErrors[r.Code]; known != nil {
			return r, fmt.Errorf("%w: %s", known, r.Error)
		}
		return r, errors.New(r.Error)
	}
	return r, nil
}
func (c *Client) Source() network.Source { return c.source }

// ApplianceIdentity reads the helper's public permanent identity projection.
func (c *Client) ApplianceIdentity(ctx context.Context) (ApplianceIdentity, error) {
	r, err := c.call(ctx, Request{Op: "appliance.identity.read"})
	if err != nil {
		return ApplianceIdentity{}, err
	}
	if r.ApplianceIdentity == nil {
		return ApplianceIdentity{}, errors.New("helper omitted appliance identity")
	}
	return *r.ApplianceIdentity, nil
}

// TLSIdentity returns only the public certificate and generation token.
func (c *Client) TLSIdentity(ctx context.Context) (TLSIdentityPublic, error) {
	r, err := c.call(ctx, Request{Op: "tls.identity.read"})
	if err != nil {
		return TLSIdentityPublic{}, err
	}
	if r.TLSIdentity == nil {
		return TLSIdentityPublic{}, errors.New("helper omitted TLS identity")
	}
	return *r.TLSIdentity, nil
}

// ActivateNames rotates the helper-held certificate before callers publish
// the corresponding discovery names.
func (c *Client) ActivateNames(ctx context.Context, names []string) error {
	copyOfNames := append([]string(nil), names...)
	_, err := c.call(ctx, Request{Op: "tls.identity.activate", TLSNames: &copyOfNames})
	return err
}

// SignTLS delegates one supported TLS handshake digest to the helper.
func (c *Client) SignTLS(ctx context.Context, generation string, digest []byte) ([]byte, error) {
	r, err := c.call(ctx, Request{
		Op: "tls.identity.sign", TLSGeneration: generation,
		TLSDigest: append([]byte(nil), digest...), TLSHash: "sha256", TLSAlgorithm: "ecdsa-p256-sha256",
	})
	return r.TLSSignature, err
}

func (c *Client) Interfaces(ctx context.Context) ([]network.Interface, error) {
	r, err := c.call(ctx, Request{Op: "interface.list"})
	if err != nil {
		return nil, err
	}
	out := make([]network.Interface, 0, len(r.Devices))
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	next := map[string]rateSample{}
	for _, d := range r.Devices {
		item := d.Interface
		count := d.Counters
		old, ok := c.previous[item.ID]
		seconds := now.Sub(old.at).Seconds()
		if ok && seconds > 0 && seconds <= 30 && count.RXBytes >= old.rx && count.TXBytes >= old.tx {
			item.RXBytesPerSecond = float64(count.RXBytes-old.rx) / seconds
			item.TXBytesPerSecond = float64(count.TXBytes-old.tx) / seconds
		}
		next[item.ID] = rateSample{now, count.RXBytes, count.TXBytes}
		out = append(out, item)
	}
	c.previous = next
	return out, nil
}
func (c *Client) Counters(ctx context.Context) ([]network.CounterReading, error) {
	r, err := c.call(ctx, Request{Op: "interface.list"})
	if err != nil {
		return nil, err
	}
	out := make([]network.CounterReading, 0, len(r.Devices))
	for _, d := range r.Devices {
		out = append(out, d.Counters)
	}
	return out, nil
}
func (c *Client) Inspect(ctx context.Context, change network.Change) (map[string]string, error) {
	if change.Action == network.ActionWireGuardInstall {
		// The origin is the boundary's note of who asked and is not part of
		// the change the helper is asked about.
		normalized := change
		normalized.Origin = ""
		if normalized != (network.Change{Action: network.ActionWireGuardInstall, Target: "wireguard"}) {
			return nil, network.ErrUnsupported
		}
		r, err := c.call(ctx, Request{Op: "wireguard.install.inspect"})
		if err != nil {
			return nil, err
		}
		if r.InstallPlan == nil {
			return nil, errors.New("helper omitted installation plan")
		}
		return map[string]string{"install_revision": r.InstallPlan.Revision}, nil
	}
	r, err := c.call(ctx, Request{Op: "change.inspect", Change: change})
	if err != nil {
		return nil, err
	}
	if r.Device == nil || r.Change == nil {
		return nil, errors.New("helper omitted observation")
	}
	d := *r.Device
	d.Counters = network.CounterReading{}
	raw, _ := json.Marshal(d)
	return map[string]string{"device": string(raw), "connection": r.Change.ConnectionID}, nil
}
func (c *Client) Observe(ctx context.Context, action network.Action, target string) (map[string]string, error) {
	return c.Inspect(ctx, network.Change{Action: action, Target: target})
}
func (c *Client) ArmRecovery(context.Context, string, time.Duration) (network.Checkpoint, error) {
	return network.Checkpoint{}, errors.New("live recovery requires an exact change")
}
func (c *Client) ArmChange(ctx context.Context, id string, change network.Change, window time.Duration) (network.Checkpoint, error) {
	if window != network.ConfirmationWindow {
		return network.Checkpoint{}, network.ErrRecoveryUnavailable
	}
	_, err := c.call(ctx, Request{Op: "checkpoint.create", ID: id, Change: change})
	if err != nil {
		return network.Checkpoint{}, err
	}
	c.mu.Lock()
	c.armed = id
	c.mu.Unlock()
	return network.Checkpoint{ID: id}, nil
}
func (c *Client) Apply(ctx context.Context, action network.Action, target string) error {
	return c.ApplyChange(ctx, network.Change{Action: action, Target: target})
}
func (c *Client) ApplyChange(ctx context.Context, _ network.Change) error {
	c.mu.Lock()
	id := c.armed
	c.mu.Unlock()
	_, err := c.call(ctx, Request{Op: "change.apply", ID: id})
	return err
}
func (c *Client) Discard(ctx context.Context, cp network.Checkpoint) error {
	_, err := c.call(ctx, Request{Op: "checkpoint.destroy", ID: cp.ID})
	return err
}
func (c *Client) Restore(ctx context.Context, cp network.Checkpoint) (bool, error) {
	_, err := c.call(ctx, Request{Op: "checkpoint.rollback", ID: cp.ID})
	return err == nil, err
}
func (c *Client) Reconcile(ctx context.Context) error {
	_, err := c.call(ctx, Request{Op: "recovery.reconcile"})
	return err
}

// LastRecovery reads the helper's own record of the last change, so a
// restarted core can settle the operations it adopted from what actually
// happened rather than from what it hoped.
func (c *Client) LastRecovery(ctx context.Context) (network.RecoveryOutcome, bool, error) {
	r, err := c.call(ctx, Request{Op: "recovery.read"})
	if err != nil {
		return network.RecoveryOutcome{}, false, err
	}
	if r.Record == nil {
		return network.RecoveryOutcome{}, false, nil
	}
	return network.RecoveryOutcome{OperationID: r.Record.ID, Phase: r.Record.Phase}, true, nil
}

var _ network.RecoveryObserver = (*Client)(nil)

// WiFiScan asks the helper to scan on one wireless adapter.
func (c *Client) WiFiScan(ctx context.Context, interfaceID string) (network.WiFiScanResult, error) {
	r, err := c.call(ctx, Request{Op: "wifi.scan", Change: network.Change{Action: network.ActionWiFiJoin, Target: interfaceID}})
	if err != nil {
		return network.WiFiScanResult{}, err
	}
	networks := r.Networks
	if networks == nil {
		networks = []network.WiFiNetwork{}
	}
	return network.WiFiScanResult{InterfaceID: interfaceID, Supported: true, Networks: networks, ObservedAt: time.Now().UTC()}, nil
}

var _ network.WiFiScanner = (*Client)(nil)

// Stage hands an import's key material to the privileged helper and keeps only
// the handle it returns.
//
// This is the one call in this client that carries a secret, and it carries it
// exactly once: the value is already in this process because an operator
// uploaded it, and this is how it leaves. There is deliberately no counterpart
// that reads it back.
func (c *Client) StageSecrets(ctx context.Context, secrets network.Secrets) (string, error) {
	if secrets.PrivateKey == "" && secrets.Password == "" {
		return "", errors.New("nothing to stage")
	}
	r, err := c.call(ctx, Request{Op: "wireguard.secrets.stage", Secret: &SecretPayload{
		PrivateKey: secrets.PrivateKey, PresharedKeys: secrets.PresharedKeys, Password: secrets.Password,
	}})
	if err != nil {
		return "", err
	}
	if r.Handle == "" {
		return "", errors.New("helper staged key material without returning a handle")
	}
	return r.Handle, nil
}

func (c *Client) BindSecrets(ctx context.Context, handle, profileID string) error {
	_, err := c.call(ctx, Request{Op: "wireguard.secrets.promote", Handle: handle, ProfileID: profileID})
	return err
}

func (c *Client) DropSecrets(ctx context.Context, handle string) error {
	_, err := c.call(ctx, Request{Op: "wireguard.secrets.discard", Handle: handle})
	return err
}

var _ network.SecretStager = (*Client)(nil)

// CreateTunnel asks the helper to create the connection for an imported
// profile. The request carries the staged handle and the public configuration;
// the helper binds the handle and reads the key itself.
func (c *Client) CreateTunnel(ctx context.Context, record network.ProfileRecord, handle string) (string, error) {
	profile := record.Profile
	r, err := c.call(ctx, Request{
		Op: "wireguard.profile.create", Handle: handle, ProfileID: record.ID,
		Change: network.Change{Action: network.ActionProfileImport, Target: record.Name, Profile: &profile},
	})
	if err != nil {
		return "", err
	}
	if r.ConnectionRef == "" {
		return "", errors.New("helper created a tunnel without reporting its connection")
	}
	return r.ConnectionRef, nil
}

func (c *Client) DeleteTunnel(ctx context.Context, profileID, connectionRef string) error {
	_, err := c.call(ctx, Request{Op: "wireguard.profile.delete", ProfileID: profileID, ConnectionRef: connectionRef})
	return err
}

func (c *Client) Tunnels(ctx context.Context) ([]network.TunnelObservation, error) {
	r, err := c.call(ctx, Request{Op: "wireguard.inventory"})
	if err != nil {
		return nil, err
	}
	if r.Tunnels == nil {
		return []network.TunnelObservation{}, nil
	}
	return r.Tunnels, nil
}

func (c *Client) ConnectTunnel(ctx context.Context, connectionRef string) error {
	_, err := c.call(ctx, Request{Op: "wireguard.startup", ConnectionRef: connectionRef})
	return err
}

var _ network.TunnelDriver = (*Client)(nil)

// ApplyForwarding installs one device network's plan through the helper.
func (c *Client) ApplyForwarding(ctx context.Context, plan network.ForwardingPlan) error {
	_, err := c.call(ctx, Request{Op: "forwarding.apply", Plan: &plan})
	return err
}

// RemoveForwarding takes one device network's forwarding away.
func (c *Client) RemoveForwarding(ctx context.Context, deviceNetworkID string) error {
	_, err := c.call(ctx, Request{Op: "forwarding.remove", DeviceNetworkID: deviceNetworkID})
	return err
}

// AppliedForwarding lists what the helper's inventory holds.
func (c *Client) AppliedForwarding(ctx context.Context) ([]network.ForwardingPlan, error) {
	r, err := c.call(ctx, Request{Op: "forwarding.inventory"})
	if err != nil {
		return nil, err
	}
	if r.Plans == nil {
		return []network.ForwardingPlan{}, nil
	}
	return r.Plans, nil
}

var _ network.Forwarder = (*Client)(nil)
