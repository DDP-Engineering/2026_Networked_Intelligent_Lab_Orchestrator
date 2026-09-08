// Package nethelper owns NILO's privileged appliance boundary and recovery.
// It accepts fixed typed operations, never commands, scripts or client paths.
package nethelper

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"nilo.local/nilo/internal/network"
	"nilo.local/nilo/internal/power"
)

// Device contains only public runtime state and saved connection identifiers.
type Device struct {
	Transitioning bool `json:"transitioning"`
	// Tunnel marks a WireGuard device. Tunnel devices come and go with the
	// tunnel changes this helper makes, so they are left out of the system
	// digest a tunnel change is verified against.
	Tunnel       bool                   `json:"tunnel,omitempty"`
	Interface    network.Interface      `json:"interface"`
	Path         string                 `json:"path"`
	Connection   string                 `json:"connection"`
	Available    []string               `json:"available"`
	AutoConnect  bool                   `json:"autoconnect"`
	NetworkState string                 `json:"network_state"`
	Counters     network.CounterReading `json:"counters"`
}

// Record is the durable intent, written before creating a checkpoint or applying
// a change. Before contains no profile credentials or packet contents.
type Record struct {
	Version          int               `json:"version"`
	ID               string            `json:"id"`
	Phase            string            `json:"phase"`
	Deadline         time.Time         `json:"deadline"`
	Checkpoint       string            `json:"checkpoint"`
	Change           network.Change    `json:"change"`
	Before           Device            `json:"before"`
	Disabled         map[string]string `json:"disabled"`
	PreviousDisabled map[string]string `json:"previous_disabled"`
	// System is set for a tunnel change, whose recovery target is the whole
	// appliance rather than one device.
	System *SystemSnapshot `json:"system,omitempty"`
	// Forwarding is set for a device-network change: the forwarding inventory
	// the change is put back to.
	Forwarding *ForwardingInventory `json:"forwarding,omitempty"`
}

type Store interface {
	Load() (*Record, error)
	Save(Record) error
}
type Driver interface {
	Devices(context.Context) ([]Device, error)
	Checkpoint(context.Context, Device, time.Duration) (string, error)
	Apply(context.Context, network.Change, Device) error
	Restore(context.Context, Record) error
	Destroy(context.Context, string) error
}

// Engine serializes privileged changes and refuses further writes until an
// interrupted change is actually observed restored.
type Engine struct {
	mu       sync.Mutex
	driver   Driver
	store    Store
	now      func() time.Time
	record   *Record
	disabled map[string]string
	install  *installation
	keystore *Keystore
	// forwarding is the durable device-network inventory, present when the
	// store keeps one; inventory caches what it holds.
	forwarding ForwardingStore
	inventory  *ForwardingInventory
	power      power.Controller
}

// ConfigurePower installs the fixed whole-appliance power implementation.
func (e *Engine) ConfigurePower(controller power.Controller) error {
	if controller == nil {
		return errors.New("power controller is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.power = controller
	return nil
}

func (e *Engine) Restart(ctx context.Context) error {
	e.mu.Lock()
	controller := e.power
	e.mu.Unlock()
	if controller == nil {
		return power.ErrUnavailable
	}
	return controller.Restart(ctx)
}

func (e *Engine) PowerOff(ctx context.Context) error {
	e.mu.Lock()
	controller := e.power
	e.mu.Unlock()
	if controller == nil {
		return power.ErrUnavailable
	}
	return controller.PowerOff(ctx)
}

func NewEngine(d Driver, s Store, now func() time.Time) (*Engine, error) {
	if d == nil || s == nil || now == nil {
		return nil, errors.New("helper requires driver, durable store and clock")
	}
	r, err := s.Load()
	if err != nil {
		return nil, fmt.Errorf("read recovery: %w", err)
	}
	forwarding, _ := s.(ForwardingStore)
	if r != nil && (r.Version != 1 || !validID.MatchString(r.ID) || !knownPhase(r.Phase)) {
		return nil, errors.New("invalid recovery record")
	}
	e := &Engine{driver: d, store: s, now: now, record: r, disabled: map[string]string{}, forwarding: forwarding}
	if r != nil {
		e.disabled = copyMap(r.Disabled)
	}
	return e, nil
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func knownPhase(p string) bool {
	return p == "intent" || p == "prepared" || p == "applied" || p == "confirmed" || p == "restored" || p == "recovery_failed"
}
func pending(r *Record) bool { return r != nil && r.Phase != "confirmed" && r.Phase != "restored" }
func copyMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (e *Engine) Devices(ctx context.Context) ([]Device, error) { return e.driver.Devices(ctx) }
func (e *Engine) Inspect(ctx context.Context, c network.Change) (Device, network.Change, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.inspect(ctx, c)
}
func (e *Engine) inspect(ctx context.Context, c network.Change) (Device, network.Change, error) {
	if c.ProfileText != "" {
		return Device{}, c, network.ErrUnsupported
	}
	if isForwardingAction(c.Action) {
		// The unprivileged side validates the decision; the helper only needs
		// to be able to forward and to own its table.
		driver, err := e.forwardingDriver()
		if err != nil {
			return Device{}, c, err
		}
		state, err := driver.ForwardingTable(ctx)
		if err != nil {
			return Device{}, c, err
		}
		if state == TableForeign {
			return Device{}, c, ErrForwardingForeign
		}
		return Device{}, c, nil
	}
	if isTunnelAction(c.Action) {
		return Device{}, c, e.inspectTunnel(ctx, c)
	}
	if isSettingsAction(c.Action) {
		return e.inspectSettings(ctx, c)
	}
	if (c.Action != network.ActionInterfaceEnable && c.Action != network.ActionInterfaceDisable) || c.Enabled || c.Name != "" || c.Settings != nil || c.WiFi != nil {
		return Device{}, c, network.ErrUnsupported
	}
	devices, err := e.driver.Devices(ctx)
	if err != nil {
		return Device{}, c, err
	}
	for _, d := range devices {
		if d.Interface.ID == c.Target {
			if !d.Interface.Controllable || (d.Interface.Kind != network.KindEthernet && d.Interface.Kind != network.KindWiFi) {
				return Device{}, c, network.ErrNotControllable
			}
			if d.Transitioning {
				return Device{}, c, network.ErrPreviewStale
			}
			if c.Action == network.ActionInterfaceEnable {
				if c.ConnectionID == "" {
					c.ConnectionID = e.disabled[c.Target]
				}
				if c.ConnectionID == "" {
					c.ConnectionID = d.Connection
				}
				if c.ConnectionID == "" {
					return Device{}, c, network.ErrConnectionRequired
				}
				found := false
				for _, id := range d.Available {
					if id == c.ConnectionID {
						found = true
					}
				}
				if !found {
					return Device{}, c, network.ErrConnectionRequired
				}
			} else if c.ConnectionID != "" {
				return Device{}, c, network.ErrUnsupported
			}
			return d, c, nil
		}
	}
	return Device{}, c, network.ErrNotControllable
}
func (e *Engine) Prepare(ctx context.Context, id string, c network.Change, window time.Duration) (Record, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !validID.MatchString(id) || window != network.ConfirmationWindow {
		return Record{}, errors.New("invalid checkpoint request")
	}
	if pending(e.record) || e.installationPending() {
		return Record{}, network.ErrChangeInProgress
	}
	before, c, err := e.inspect(ctx, c)
	if err != nil {
		return Record{}, err
	}
	var system *SystemSnapshot
	if isTunnelAction(c.Action) {
		snapshot, err := e.snapshot(ctx)
		if err != nil {
			return Record{}, err
		}
		system = &snapshot
	}
	var forwarding *ForwardingInventory
	if isForwardingAction(c.Action) {
		if forwarding, err = e.snapshotForwardingLocked(); err != nil {
			return Record{}, err
		}
	}
	r := Record{Version: 1, ID: id, Phase: "intent", Deadline: e.now().Add(window), Before: before, Change: c, Disabled: copyMap(e.disabled), PreviousDisabled: copyMap(e.disabled), System: system, Forwarding: forwarding}
	if err = e.store.Save(r); err != nil {
		return Record{}, err
	}
	e.record = &r
	// A device-network change is recovered from the helper's own inventory
	// snapshot; NetworkManager has nothing to checkpoint for it.
	if forwarding == nil {
		cp, err := e.driver.Checkpoint(ctx, before, window)
		if err != nil {
			return Record{}, fmt.Errorf("checkpoint unavailable: %w", err)
		}
		r.Checkpoint = cp
	}
	r.Phase = "prepared"
	if err = e.store.Save(r); err != nil {
		return Record{}, err
	}
	result := r
	result.Disabled = copyMap(r.Disabled)
	result.PreviousDisabled = copyMap(r.PreviousDisabled)
	return result, nil
}
func (e *Engine) Apply(ctx context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.record
	if r == nil || r.ID != id || r.Phase != "prepared" || !e.now().Before(r.Deadline) {
		return network.ErrRecoveryUnavailable
	}
	// Persist consumed intent before driver I/O, so a crash cannot replay it.
	r.Phase = "applied"
	if r.Change.Action == network.ActionInterfaceDisable && r.Before.Connection != "" {
		r.Disabled[r.Change.Target] = r.Before.Connection
	}
	if err := e.store.Save(*r); err != nil {
		return err
	}
	if isTunnelAction(r.Change.Action) {
		return e.applyTunnel(ctx, r)
	}
	if isSettingsAction(r.Change.Action) {
		return e.applySettings(ctx, r)
	}
	if r.Forwarding != nil {
		// The forwarding itself arrives through ApplyForwarding and
		// RemoveForwarding while this record is applied.
		return nil
	}
	return e.driver.Apply(ctx, r.Change, r.Before)
}
func (e *Engine) Confirm(ctx context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := e.record
	if r == nil || r.ID != id || r.Phase != "applied" || !e.now().Before(r.Deadline) {
		return network.ErrNotAwaitingConfirm
	}
	if r.Checkpoint != "" {
		if err := e.driver.Destroy(ctx, r.Checkpoint); err != nil {
			return err
		}
	}
	next := *r
	next.Phase = "confirmed"
	if err := e.store.Save(next); err != nil {
		return err
	}
	e.record = &next
	e.disabled = copyMap(next.Disabled)
	return nil
}
func (e *Engine) restore(ctx context.Context) error {
	r := e.record
	if !pending(r) {
		return nil
	}
	var err error
	if r.Forwarding != nil {
		err = e.restoreForwardingLocked(ctx, r)
	} else {
		err = e.driver.Restore(ctx, *r)
	}
	if err != nil {
		r.Phase = "recovery_failed"
		_ = e.store.Save(*r)
		return fmt.Errorf("previous network state not verified: %w", err)
	}
	next := *r
	next.Phase = "restored"
	next.Disabled = copyMap(next.PreviousDisabled)
	if err := e.store.Save(next); err != nil {
		return err
	}
	e.record = &next
	e.disabled = copyMap(next.Disabled)
	return nil
}
func (e *Engine) Restore(ctx context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.record == nil || e.record.ID != id {
		return network.ErrOperationNotFound
	}
	if e.record.Phase == "confirmed" {
		return network.ErrNotAwaitingConfirm
	}
	return e.restore(ctx)
}
func (e *Engine) Reconcile(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.restore(ctx)
}
func (e *Engine) Expire(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if pending(e.record) && !e.now().Before(e.record.Deadline) {
		return e.restore(ctx)
	}
	return nil
}
func (e *Engine) Recovery() (Record, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.record == nil {
		return Record{}, false
	}
	r := *e.record
	r.Disabled = copyMap(r.Disabled)
	r.PreviousDisabled = copyMap(r.PreviousDisabled)
	return r, true
}

// RunRecovery supplies an independent helper timer; NetworkManager also owns a
// checkpoint timer that survives the helper itself being killed.
func (e *Engine) RunRecovery(ctx context.Context, report func(error)) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			recovery, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := e.Expire(recovery)
			cancel()
			if err != nil && report != nil {
				report(err)
			}
			// The owned firewall table is checked on its own cadence, so a
			// device network whose guards vanished is closed again without
			// the core's help.
			ticks++
			if ticks%10 == 0 {
				verify, cancelVerify := context.WithTimeout(ctx, 30*time.Second)
				err := e.VerifyForwarding(verify)
				cancelVerify()
				if err != nil && report != nil {
					report(err)
				}
			}
		}
	}
}
