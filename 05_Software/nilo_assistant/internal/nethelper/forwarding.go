package nethelper

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"nilo.local/nilo/internal/network"
)

// The forwarding half of the privileged engine. Device-network forwarding is
// rendered into one firewall table NILO owns outright, from a durable inventory
// the helper keeps itself. The inventory, not the kernel, is the record of
// what NILO owns: it is written before any rule is, so a crash leaves a file
// that says what to put back or what to take away, and it is what a restarted
// helper renders from. A table NILO did not create is never replaced.

// TableState is what the owned table looks like on the host.
type TableState string

const (
	TableAbsent  TableState = "absent"
	TableOwned   TableState = "owned"
	TableForeign TableState = "foreign"
)

// SwitchOwnership records whether NILO turned the kernel's forwarding switch
// on, and what it was before, so it is given back exactly when the last
// device network goes.
type SwitchOwnership struct {
	Owned    bool   `json:"owned"`
	Previous string `json:"previous"`
}

// ForwardingInventory is everything the helper owns for device networks.
type ForwardingInventory struct {
	Version int                      `json:"version"`
	Plans   []network.ForwardingPlan `json:"plans"`
	Switch  SwitchOwnership          `json:"switch"`
}

// ForwardingDriver is what the Linux adapter provides for forwarding. The
// fake driver in the tests provides the same.
type ForwardingDriver interface {
	// ForwardingTable reports whether the owned table is absent, NILO's own or
	// somebody else's.
	ForwardingTable(ctx context.Context) (TableState, error)
	// InstallForwarding replaces the owned table with the inventory in one
	// transaction; an empty inventory removes it.
	InstallForwarding(ctx context.Context, inventory ForwardingInventory) error
	ForwardingSwitch(ctx context.Context) (string, error)
	SetForwardingSwitch(ctx context.Context, value string) error
}

// ForwardingStore keeps the inventory durable. The file store provides it.
type ForwardingStore interface {
	LoadForwarding() (*ForwardingInventory, error)
	SaveForwarding(ForwardingInventory) error
}

var (
	// ErrForwardingUnsupported means this helper's driver cannot forward.
	ErrForwardingUnsupported = errors.New("this helper cannot manage device network forwarding")
	// ErrForwardingForeign refuses to touch a table NILO did not create.
	ErrForwardingForeign = errors.New("the nilo firewall table exists but was not created by nilo-nethelper")
)

func isForwardingAction(action network.Action) bool {
	switch action {
	case network.ActionDeviceNetworkCreate, network.ActionDeviceNetworkUpdate, network.ActionDeviceNetworkEnable,
		network.ActionDeviceNetworkDisable, network.ActionDeviceNetworkDelete:
		return true
	}
	return false
}

func (e *Engine) forwardingDriver() (ForwardingDriver, error) {
	driver, ok := e.driver.(ForwardingDriver)
	if !ok {
		return nil, ErrForwardingUnsupported
	}
	if e.forwarding == nil {
		return nil, fmt.Errorf("%w: no durable forwarding inventory", ErrForwardingUnsupported)
	}
	return driver, nil
}

// inventoryLocked is the current inventory, read once from the store and
// cached. Called with the engine lock held.
func (e *Engine) inventoryLocked() (ForwardingInventory, error) {
	if e.inventory != nil {
		return copyInventory(*e.inventory), nil
	}
	loaded, err := e.forwarding.LoadForwarding()
	if err != nil {
		return ForwardingInventory{}, fmt.Errorf("read forwarding inventory: %w", err)
	}
	if loaded == nil {
		loaded = &ForwardingInventory{Version: 1}
	}
	if loaded.Version != 1 {
		return ForwardingInventory{}, errors.New("invalid forwarding inventory")
	}
	e.inventory = loaded
	return copyInventory(*loaded), nil
}

func copyInventory(inventory ForwardingInventory) ForwardingInventory {
	copied := inventory
	copied.Plans = append([]network.ForwardingPlan(nil), inventory.Plans...)
	return copied
}

// forwardingAllowedLocked refuses forwarding work while a change that is not a
// device-network change is in flight, or while a recovery is unresolved.
// Called with the engine lock held.
func (e *Engine) forwardingAllowedLocked() error {
	if e.installationPending() {
		return network.ErrChangeInProgress
	}
	if r := e.record; pending(r) {
		if r.Forwarding == nil || r.Phase != "applied" {
			return network.ErrChangeInProgress
		}
	}
	return nil
}

// ApplyForwarding installs one plan, replacing the plan the same device
// network had. Applying what is already installed changes nothing.
func (e *Engine) ApplyForwarding(ctx context.Context, plan network.ForwardingPlan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, err := e.forwardingDriver()
	if err != nil {
		return err
	}
	if plan.DeviceNetworkID == "" {
		return errors.New("a forwarding plan names its device network")
	}
	if err := e.forwardingAllowedLocked(); err != nil {
		return err
	}
	inventory, err := e.inventoryLocked()
	if err != nil {
		return err
	}
	state, err := driver.ForwardingTable(ctx)
	if err != nil {
		return err
	}
	if state == TableForeign {
		return ErrForwardingForeign
	}
	replaced := false
	for index, existing := range inventory.Plans {
		if existing.DeviceNetworkID == plan.DeviceNetworkID {
			if reflect.DeepEqual(existing, plan) && state == TableOwned {
				return nil
			}
			inventory.Plans[index] = plan
			replaced = true
		}
	}
	if !replaced {
		inventory.Plans = append(inventory.Plans, plan)
	}
	sort.Slice(inventory.Plans, func(i, j int) bool { return inventory.Plans[i].DeviceNetworkID < inventory.Plans[j].DeviceNetworkID })
	return e.renderLocked(ctx, driver, inventory)
}

// RemoveForwarding takes one device network's resources away. Removing what is
// absent is not an error, so a retried removal converges.
func (e *Engine) RemoveForwarding(ctx context.Context, deviceNetworkID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, err := e.forwardingDriver()
	if err != nil {
		return err
	}
	if err := e.forwardingAllowedLocked(); err != nil {
		return err
	}
	inventory, err := e.inventoryLocked()
	if err != nil {
		return err
	}
	kept := inventory.Plans[:0:0]
	for _, existing := range inventory.Plans {
		if existing.DeviceNetworkID != deviceNetworkID {
			kept = append(kept, existing)
		}
	}
	if len(kept) == len(inventory.Plans) {
		return nil
	}
	inventory.Plans = kept
	return e.renderLocked(ctx, driver, inventory)
}

// AppliedForwarding lists what the inventory says is installed.
func (e *Engine) AppliedForwarding(context.Context) ([]network.ForwardingPlan, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.forwardingDriver(); err != nil {
		return nil, err
	}
	inventory, err := e.inventoryLocked()
	if err != nil {
		return nil, err
	}
	if inventory.Plans == nil {
		return []network.ForwardingPlan{}, nil
	}
	return inventory.Plans, nil
}

// switchNeeded reports whether any installed plan forwards at all.
func switchNeeded(inventory ForwardingInventory) bool {
	for _, plan := range inventory.Plans {
		if plan.Waiting {
			continue
		}
		for _, resource := range plan.Resources {
			if resource.Kind == network.ResourceForwardingSysctl {
				return true
			}
		}
	}
	return false
}

// renderLocked makes the host match the inventory: the inventory is saved
// first, the table is installed before forwarding is switched on, and
// forwarding is switched off before the table is taken away, so at no moment
// is anything forwarded that a guard does not cover. Called with the lock held.
func (e *Engine) renderLocked(ctx context.Context, driver ForwardingDriver, next ForwardingInventory) error {
	next.Version = 1
	// Ownership of the switch follows what is recorded now, not what the
	// inventory being rendered says: a snapshot taken before NILO turned the
	// switch on must still give it back.
	var owned SwitchOwnership
	if e.inventory != nil {
		owned = e.inventory.Switch
	}
	needed := switchNeeded(next)
	next.Switch = SwitchOwnership{}
	if needed {
		if owned.Owned {
			next.Switch = owned
		} else {
			current, err := driver.ForwardingSwitch(ctx)
			if err != nil {
				return err
			}
			if current != "1" {
				next.Switch = SwitchOwnership{Owned: true, Previous: current}
			}
		}
	}
	giveBack := !needed && owned.Owned
	previous := owned.Previous
	if err := e.forwarding.SaveForwarding(next); err != nil {
		return fmt.Errorf("record forwarding inventory: %w", err)
	}
	e.inventory = &next
	if giveBack {
		if err := driver.SetForwardingSwitch(ctx, previous); err != nil {
			return err
		}
	}
	if err := driver.InstallForwarding(ctx, next); err != nil {
		return err
	}
	if needed && next.Switch.Owned {
		if err := driver.SetForwardingSwitch(ctx, "1"); err != nil {
			return err
		}
	}
	return nil
}

// snapshotForwardingLocked is what a device-network change is put back to.
func (e *Engine) snapshotForwardingLocked() (*ForwardingInventory, error) {
	if _, err := e.forwardingDriver(); err != nil {
		return nil, err
	}
	inventory, err := e.inventoryLocked()
	if err != nil {
		return nil, err
	}
	return &inventory, nil
}

// restoreForwardingLocked puts the snapshot back and verifies the host shows
// it. Called with the lock held.
func (e *Engine) restoreForwardingLocked(ctx context.Context, r *Record) error {
	driver, err := e.forwardingDriver()
	if err != nil {
		return err
	}
	if err := e.renderLocked(ctx, driver, copyInventory(*r.Forwarding)); err != nil {
		return err
	}
	return e.verifyLocked(ctx, driver)
}

// verifyLocked checks that the host shows the inventory.
func (e *Engine) verifyLocked(ctx context.Context, driver ForwardingDriver) error {
	inventory, err := e.inventoryLocked()
	if err != nil {
		return err
	}
	state, err := driver.ForwardingTable(ctx)
	if err != nil {
		return err
	}
	switch {
	case len(inventory.Plans) == 0 && state == TableOwned:
		return errors.New("the owned table is still present")
	case len(inventory.Plans) > 0 && state != TableOwned:
		return fmt.Errorf("the owned table is %s", state)
	}
	if inventory.Switch.Owned {
		if current, err := driver.ForwardingSwitch(ctx); err != nil || current != "1" {
			return errors.New("the forwarding switch is not as recorded")
		}
	}
	return nil
}

// VerifyForwarding is the helper's own integrity check: an owned table that
// vanished is put back, because a device network without its guards is
// exposed rather than closed. It does nothing while a change is in flight.
func (e *Engine) VerifyForwarding(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	driver, err := e.forwardingDriver()
	if err != nil {
		return nil
	}
	if e.installationPending() || pending(e.record) {
		return nil
	}
	inventory, err := e.inventoryLocked()
	if err != nil {
		return err
	}
	if len(inventory.Plans) == 0 {
		return nil
	}
	state, err := driver.ForwardingTable(ctx)
	if err != nil {
		return err
	}
	switch state {
	case TableForeign:
		return ErrForwardingForeign
	case TableAbsent:
		return e.renderLocked(ctx, driver, inventory)
	}
	return nil
}
