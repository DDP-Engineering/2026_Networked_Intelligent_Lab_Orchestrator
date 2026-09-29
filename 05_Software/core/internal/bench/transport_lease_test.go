package bench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"nilo.local/nilo/internal/identity"
	"nilo.local/nilo/internal/modbus"
	"nilo.local/nilo/internal/store/sqlite"
)

type modbusConsumerService interface {
	runModbusConsumer(context.Context, string, int64, string, int64, time.Time, func(context.Context, modbus.Endpoint, modbus.Profile) error) (context.CancelFunc, <-chan struct{}, error)
}

func installModbusFoundation(t *testing.T, m *Manager) {
	t.Helper()
	// This fixture installs only a database marker, never a package or OS transport.
	if _, err := m.db().Exec(`INSERT INTO bench_plugins(id,instance_id,installed,enabled,mode) VALUES('modbus',?,1,1,'auto')`, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
}

func saveConsumerDefinitions(t *testing.T, m *Manager, a identity.Actor, e modbus.Endpoint) (modbus.Endpoint, modbus.Profile) {
	t.Helper()
	e, err := m.SaveModbusEndpoint(t.Context(), a, 0, e)
	if err != nil {
		t.Fatal(err)
	}
	p, err := m.SaveModbusProfile(t.Context(), a, 0, profileDefinition("profile-"+e.ID, e.ID))
	if err != nil {
		t.Fatal(err)
	}
	return e, p
}

func awaitTransportDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("transport cleanup did not complete")
	}
}

func TestModbusGatewayAliasesUnitsAndIndependentResources(t *testing.T) {
	m, a, _ := testManager(t, "auto")
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, a, networkDefinition("one", "bus"))
	alias := networkDefinition("alias", "bus")
	alias.Network.Host = "other-alias.local"
	alias.Network.UnitMin = 7
	alias.Network.UnitMax = 7
	alias, ap := saveConsumerDefinitions(t, m, a, alias)
	relabeled, rp := saveConsumerDefinitions(t, m, a, networkDefinition("relabeled", "different-bus"))
	independent := networkDefinition("independent", "separate")
	independent.Network.Port = 503
	independent, ip := saveConsumerDefinitions(t, m, a, independent)
	lifetime := func(ctx context.Context, _ modbus.Endpoint, _ modbus.Profile) error { <-ctx.Done(); return nil }
	cancel, done, err := m.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision, time.Now().Add(time.Minute), lifetime)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	for _, pair := range []struct {
		e modbus.Endpoint
		p modbus.Profile
	}{{alias, ap}, {relabeled, rp}} {
		if _, _, err = m.runModbusConsumer(t.Context(), pair.e.ID, pair.e.Revision, pair.p.ID, pair.p.Revision, time.Now().Add(time.Minute), lifetime); !errors.Is(err, ErrBusy) {
			t.Fatal("alias bypass", err)
		}
	}
	otherCancel, otherDone, err := m.runModbusConsumer(t.Context(), independent.ID, independent.Revision, ip.ID, ip.Revision, time.Now().Add(time.Minute), lifetime)
	if err != nil {
		t.Fatal("independent resource blocked", err)
	}
	otherCancel()
	awaitTransportDone(t, otherDone)
	e.Name = "Changed"
	if _, err = m.SaveModbusEndpoint(t.Context(), a, e.Revision, e); !errors.Is(err, ErrBusy) {
		t.Fatal("live endpoint changed", err)
	}
	p.Name = "Changed"
	if _, err = m.SaveModbusProfile(t.Context(), a, p.Revision, p); !errors.Is(err, ErrBusy) {
		t.Fatal("live profile changed", err)
	}
	cancel()
	awaitTransportDone(t, done)
	if _, _, err = m.runModbusConsumer(t.Context(), e.ID, e.Revision-1, p.ID, p.Revision, time.Now().Add(time.Minute), lifetime); !errors.Is(err, ErrConflict) {
		t.Fatal("stale endpoint admitted", err)
	}
	if _, _, err = m.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision-1, time.Now().Add(time.Minute), lifetime); !errors.Is(err, ErrConflict) {
		t.Fatal("stale profile admitted", err)
	}
}

func TestSerialAndModbusConcurrentPhysicalAdmissionHasOneOwner(t *testing.T) {
	m, a, input, runner := serialLifecycleFixture(t, "auto")
	installModbusFoundation(t, m)
	close(runner.ready)
	endpoints, err := m.reconcileModbusDiscovery(t.Context(), json.RawMessage(strings.ReplaceAll(modbusUSBObservation, "ttyUSB0", "ttyUSB7")))
	if err != nil {
		t.Fatal(err)
	}
	e := endpoints[0]
	e.Enabled = true
	e, err = m.SaveModbusEndpoint(t.Context(), a, e.Revision, e)
	if err != nil {
		t.Fatal(err)
	}
	p := profileDefinition("usb-profile", e.ID)
	p.Settings = json.RawMessage(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0,"baud_rate":9600,"data_bits":8,"parity":"even","stop_bits":1,"local_echo":false}`)
	p, err = m.SaveModbusProfile(t.Context(), a, 0, p)
	if err != nil {
		t.Fatal(err)
	}
	serialJob, err := m.ConnectSerial(t.Context(), a, "serial-1", input)
	if err != nil {
		t.Fatal(err)
	}
	// Setup-phase jobs may finish before the peer races. Both paths must still claim
	// the same physical resources throughout the resulting live transport lifetime.
	var cancel context.CancelFunc
	var done <-chan struct{}
	cancel, done, err = m.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision, time.Now().Add(time.Minute), func(ctx context.Context, _ modbus.Endpoint, _ modbus.Profile) error { <-ctx.Done(); return nil })
	if err == nil {
		defer cancel()
		awaitJob(t, m, a, serialJob.ID, "incomplete")
		m.mu.Lock()
		serial := m.serials["serial-1"]
		serialOwns := serial != nil && !serial.stopped
		m.mu.Unlock()
		if serialOwns {
			t.Fatal("both families own the adapter")
		}
		if _, err = m.ConfigureDevice(t.Context(), a, "serial", "serial-1", input.Revision, "Changed", "", json.RawMessage(`{}`)); !errors.Is(err, ErrBusy) {
			t.Fatal("cross-family USB configuration admitted", err)
		}
		cancel()
		awaitTransportDone(t, done)
	} else if errors.Is(err, ErrBusy) {
		awaitJob(t, m, a, serialJob.ID, "completed")
	} else {
		t.Fatal(err)
	}
}

func TestCanonicalTransportClaimsRaceAndStaleCleanupCannotReleasePeer(t *testing.T) {
	m, _, _ := testManager(t, "auto")
	start := make(chan struct{})
	results := make(chan *transportLease, 2)
	var workers sync.WaitGroup
	for _, family := range []string{"serial", "modbus"} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			m.mu.Lock()
			claim, _ := m.claimTransportLocked(family, "different-instance", "", []string{"usb:port:1-2:if00:port0"})
			m.mu.Unlock()
			results <- claim
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	winners := []*transportLease{}
	for claim := range results {
		if claim != nil {
			winners = append(winners, claim)
		}
	}
	if len(winners) != 1 {
		t.Fatal("physical admission race winners", len(winners))
	}
	m.finishTransport(winners[0], nil)
	m.mu.Lock()
	peer, err := m.claimTransportLocked("modbus", "peer", "", winners[0].keys)
	if err != nil {
		m.mu.Unlock()
		t.Fatal(err)
	}
	m.mu.Unlock()
	// A repeated late completion must remain harmless after another owner enters.
	m.finishTransport(winners[0], nil)
	m.mu.Lock()
	busy := m.transportBusyLocked(peer.keys)
	m.mu.Unlock()
	if !busy {
		t.Fatal("stale cleanup released peer")
	}
	m.finishTransport(peer, nil)
}

func TestModbusLifecycleRetainsClaimsThroughCleanup(t *testing.T) {
	for _, trigger := range []string{"disable", "plugin_disable", "unplug", "expiry", "runner_exit", "offline", "shutdown"} {
		t.Run(trigger, func(t *testing.T) {
			m, a, _ := testManager(t, "auto")
			installModbusFoundation(t, m)
			var e modbus.Endpoint
			var p modbus.Profile
			var err error
			if trigger == "unplug" {
				endpoints, readErr := m.reconcileModbusDiscovery(t.Context(), json.RawMessage(modbusUSBObservation))
				if readErr != nil {
					t.Fatal(readErr)
				}
				e = endpoints[0]
				e.Enabled = true
				e, err = m.SaveModbusEndpoint(t.Context(), a, e.Revision, e)
				if err != nil {
					t.Fatal(err)
				}
				p = profileDefinition("usb-profile", e.ID)
				p.Settings = json.RawMessage(`{"response_timeout_ms":1000,"connect_timeout_ms":1000,"inter_request_ms":0,"read_retries":0,"baud_rate":9600,"data_bits":8,"parity":"even","stop_bits":1,"local_echo":false}`)
				p, err = m.SaveModbusProfile(t.Context(), a, 0, p)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				e, p = saveConsumerDefinitions(t, m, a, networkDefinition("peer", "bus"))
			}
			cleanupStarted := make(chan struct{})
			cleanupRelease := make(chan struct{})
			exited := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(cleanupRelease) }) }
			t.Cleanup(release)
			ctx, expire := context.WithCancel(t.Context())
			defer expire()
			expires := time.Now().Add(time.Minute)
			if trigger == "expiry" {
				expires = time.Now().Add(100 * time.Millisecond)
			}
			cancel, done, err := m.runModbusConsumer(ctx, e.ID, e.Revision, p.ID, p.Revision, expires, func(ctx context.Context, _ modbus.Endpoint, _ modbus.Profile) error {
				select {
				case <-ctx.Done():
				case <-exited:
				}
				close(cleanupStarted)
				<-cleanupRelease
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()
			shutdownDone := make(chan error, 1)
			switch trigger {
			case "disable":
				e.Enabled = false
				if _, err = m.SaveModbusEndpoint(t.Context(), a, e.Revision, e); err != nil {
					t.Fatal(err)
				}
			case "plugin_disable":
				if _, err = m.Lifecycle(t.Context(), a, "modbus", "disable", ""); err != nil {
					t.Fatal(err)
				}
			case "unplug":
				if _, err = m.reconcileModbusDiscovery(t.Context(), json.RawMessage(`{"devices":[]}`)); err != nil {
					t.Fatal(err)
				}
			case "runner_exit":
				close(exited)
			case "offline":
				if _, err = m.observeModbusReadiness(t.Context(), e.ID, e.Revision, "offline"); err != nil {
					t.Fatal(err)
				}
			case "shutdown":
				go func() { shutdownDone <- m.Close() }()
			}
			awaitTransportDone(t, cleanupStarted)
			m.mu.Lock()
			busy := m.transportBusyLocked(e.ResourceKeys())
			m.mu.Unlock()
			if !busy {
				t.Fatal("terminal event released claims before cleanup", trigger)
			}
			select {
			case <-done:
				t.Fatal("cleanup published early")
			default:
			}
			if trigger == "shutdown" {
				select {
				case <-shutdownDone:
					t.Fatal("shutdown did not wait")
				default:
				}
			}
			release()
			awaitTransportDone(t, done)
			m.mu.Lock()
			busy = m.transportBusyLocked(e.ResourceKeys())
			m.mu.Unlock()
			if busy {
				t.Fatal("confirmed cleanup retained claims")
			}
			if trigger == "shutdown" {
				if err = <-shutdownDone; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestModbusFailedCleanupFencesResourceAndReportsShutdownError(t *testing.T) {
	m, a, _ := testManager(t, "auto")
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, a, networkDefinition("peer", "bus"))
	cleanupFailure := errors.New("cleanup unconfirmed")
	_, done, err := m.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision, time.Now().Add(time.Minute), func(context.Context, modbus.Endpoint, modbus.Profile) error { return cleanupFailure })
	if err != nil {
		t.Fatal(err)
	}
	awaitTransportDone(t, done)
	if _, _, err = m.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision, time.Now().Add(time.Minute), func(context.Context, modbus.Endpoint, modbus.Profile) error {
		t.Error("fenced transport restarted")
		return nil
	}); !errors.Is(err, ErrBusy) {
		t.Fatal("failed cleanup admitted reuse", err)
	}
	if err = m.Close(); !errors.Is(err, cleanupFailure) {
		t.Fatal("shutdown cleanup error lost", err)
	}
	stored, err := sqlite.NewModbusRepository(m.options.Database).Endpoint(t.Context(), e.ID)
	if err != nil || stored.ID != e.ID {
		t.Fatal("cleanup deleted definition", err)
	}
}

func TestCanonicalModbusTransportOwnershipFoundation(t *testing.T) {
	m, a, _ := testManager(t, "auto")
	service, ok := any(m).(modbusConsumerService)
	if !ok {
		t.Fatal("canonical Modbus transport consumer admission unavailable")
	}
	e, p := saveConsumerDefinitions(t, m, a, networkDefinition("peer", "bus"))
	lifetime := func(context.Context, modbus.Endpoint, modbus.Profile) error {
		t.Error("unavailable transport launched")
		return nil
	}
	if _, _, err := service.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision, time.Now().Add(time.Minute), lifetime); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing plugin admitted", err)
	}
	installModbusFoundation(t, m)
	if _, err := m.db().Exec(`UPDATE bench_plugins SET enabled=0 WHERE id='modbus'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision, time.Now().Add(time.Minute), lifetime); !errors.Is(err, ErrUnavailable) {
		t.Fatal("disabled plugin admitted", err)
	}
	if _, err := m.db().Exec(`UPDATE bench_plugins SET enabled=1 WHERE id='modbus'`); err != nil {
		t.Fatal(err)
	}
	e.Enabled = false
	e, err := m.SaveModbusEndpoint(t.Context(), a, e.Revision, e)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision, time.Now().Add(time.Minute), lifetime); !errors.Is(err, ErrUnavailable) {
		t.Fatal("disabled endpoint admitted", err)
	}
}

func TestUSBClaimsCoverMovedStrongIdentityAndIndependentWeakPorts(t *testing.T) {
	m, _, _ := testManager(t, "auto")
	serial := "stable/if00/port0"
	strong := modbus.USBIdentity{VendorID: "0403", ProductID: "6001", SerialNumber: &serial, BusPath: "1-2:if00:port0"}
	moved := strong
	moved.BusPath = "1-4:if00:port0"
	weak := strong
	weak.SerialNumber = nil
	independent := weak
	independent.BusPath = "1-3:if00:port0"
	m.mu.Lock()
	claim, err := m.claimTransportLocked("serial", "serial-instance", "", modbus.USBResourceKeys(strong))
	if err != nil {
		m.mu.Unlock()
		t.Fatal(err)
	}
	for _, identity := range []modbus.USBIdentity{moved, weak} {
		if _, err = m.claimTransportLocked("modbus", "modbus-instance", "", modbus.USBResourceKeys(identity)); !errors.Is(err, ErrBusy) {
			m.mu.Unlock()
			t.Fatal("USB alias or classification bypass", err)
		}
	}
	peer, err := m.claimTransportLocked("modbus", "independent", "", modbus.USBResourceKeys(independent))
	m.mu.Unlock()
	if err != nil {
		t.Fatal("independent weak port blocked", err)
	}
	m.finishTransport(claim, nil)
	m.finishTransport(peer, nil)
}

func TestModbusClaimedDefinitionRevisionAndCanonicalEditAdmission(t *testing.T) {
	m, a, _ := testManager(t, "auto")
	installModbusFoundation(t, m)
	e, p := saveConsumerDefinitions(t, m, a, networkDefinition("active", "bus"))
	other := networkDefinition("other", "separate")
	other.Network.Host = "other.local"
	other, _ = saveConsumerDefinitions(t, m, a, other)
	cancel, done, err := m.runModbusConsumer(t.Context(), e.ID, e.Revision, p.ID, p.Revision, time.Now().Add(time.Minute), func(ctx context.Context, _ modbus.Endpoint, _ modbus.Profile) error { <-ctx.Done(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); awaitTransportDone(t, done) }()
	if _, err = m.SaveModbusEndpoint(t.Context(), a, 0, e); !errors.Is(err, ErrConflict) {
		t.Error("stale claimed endpoint must conflict", err)
	}
	if _, err = m.SaveModbusProfile(t.Context(), a, 0, p); !errors.Is(err, ErrConflict) {
		t.Error("stale claimed profile must conflict", err)
	}
	other.Network.Host = "GATEWAY.LOCAL."
	if _, err = m.SaveModbusEndpoint(t.Context(), a, other.Revision, other); !errors.Is(err, ErrBusy) {
		t.Error("normalized peer edit bypassed active owner", err)
	}
}
