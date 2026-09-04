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
func (c *Client) call(ctx context.Context, q Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.socket)
	if err != nil {
		return Response{}, fmt.Errorf("network helper unavailable: %w", err)
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
		if known := knownErrors[r.Code]; known != nil {
			return r, fmt.Errorf("%w: %s", known, r.Error)
		}
		return r, errors.New(r.Error)
	}
	return r, nil
}
func (c *Client) Source() network.Source { return c.source }
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
