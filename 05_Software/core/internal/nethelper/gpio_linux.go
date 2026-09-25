//go:build linux

package nethelper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	gpioButtonOffset = 17
	gpioLEDOffset    = 27

	gpioLineFlagActiveLow  = uint64(1 << 1)
	gpioLineFlagInput      = uint64(1 << 2)
	gpioLineFlagOutput     = uint64(1 << 3)
	gpioLineFlagBiasPullUp = uint64(1 << 8)

	gpioGetLineIOCTL   = uintptr(0xC250B407)
	gpioGetValuesIOCTL = uintptr(0xC010B40E)
	gpioSetValuesIOCTL = uintptr(0xC010B40F)
)

type gpioV2LineAttribute struct {
	ID      uint32
	Padding uint32
	Value   uint64
}

type gpioV2LineConfigAttribute struct {
	Attribute gpioV2LineAttribute
	Mask      uint64
}

type gpioV2LineConfig struct {
	Flags    uint64
	NumAttrs uint32
	Padding  [5]uint32
	Attrs    [10]gpioV2LineConfigAttribute
}

type gpioV2LineRequest struct {
	Offsets         [64]uint32
	Consumer        [32]byte
	Config          gpioV2LineConfig
	NumLines        uint32
	EventBufferSize uint32
	Padding         [5]uint32
	FD              int32
}

type gpioV2LineValues struct {
	Bits uint64
	Mask uint64
}

func gpioIOCTL(fd int, request uintptr, value unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), request, uintptr(value))
	if errno != 0 {
		return errno
	}
	return nil
}

func requestGPIOLine(chipFD int, offset uint32, flags uint64, consumer string) (int, error) {
	request := gpioV2LineRequest{NumLines: 1, FD: -1}
	request.Offsets[0] = offset
	request.Config.Flags = flags
	copy(request.Consumer[:], consumer)
	if unsafe.Sizeof(request) != 592 {
		return -1, errors.New("unsupported GPIO v2 request layout")
	}
	if err := gpioIOCTL(chipFD, gpioGetLineIOCTL, unsafe.Pointer(&request)); err != nil {
		return -1, err
	}
	if request.FD < 0 {
		return -1, errors.New("GPIO v2 returned no line descriptor")
	}
	return int(request.FD), nil
}

// PiRecoveryHardware is the explicitly enabled BCM17 button and BCM27 LED.
type PiRecoveryHardware struct {
	buttonFD int
	ledFD    int
	cancel   context.CancelFunc
	done     sync.WaitGroup

	mu          sync.Mutex
	detector    *RecoveryButtonDetector
	events      *recoveryEventQueue
	snapshotter interface {
		Devices(context.Context) ([]Device, error)
	}
	indicator     HardwareIndicatorState
	indicatorAt   time.Time
	recoveryState HardwareIndicatorState
	identifyUntil time.Time
	resetPending  bool
	ioErr         error
}

// NewPiRecoveryHardware opens GPIO v2 lines only when called by explicit Pi
// installation. Generic appliance startup never calls this constructor.
func NewPiRecoveryHardware(
	parent context.Context, chipPath string, store HardwareRecoveryEventStore,
	snapshotter interface {
		Devices(context.Context) ([]Device, error)
	},
) (*PiRecoveryHardware, error) {
	if snapshotter == nil {
		return nil, errors.New("Raspberry Pi recovery requires a link snapshot provider")
	}
	events, err := newRecoveryEventQueue(store)
	if err != nil {
		return nil, err
	}
	chip, err := os.OpenFile(chipPath, os.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open Raspberry Pi GPIO chip: %w", err)
	}
	defer chip.Close()
	buttonFD, err := requestGPIOLine(int(chip.Fd()), gpioButtonOffset,
		gpioLineFlagInput|gpioLineFlagActiveLow|gpioLineFlagBiasPullUp, "nilo-recovery-button")
	if err != nil {
		return nil, fmt.Errorf("request BCM17 recovery button: %w", err)
	}
	ledFD, err := requestGPIOLine(int(chip.Fd()), gpioLEDOffset, gpioLineFlagOutput, "nilo-recovery-led")
	if err != nil {
		_ = unix.Close(buttonFD)
		return nil, fmt.Errorf("request BCM27 recovery LED: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	now := time.Now()
	identifyUntil := restorePiIdentifyDeadline(events, now)
	hardware := &PiRecoveryHardware{
		buttonFD: buttonFD, ledFD: ledFD, cancel: cancel,
		detector: NewRecoveryButtonDetector(50 * time.Millisecond), events: events, snapshotter: snapshotter, indicator: HardwareIndicatorIdle,
		indicatorAt: now, recoveryState: HardwareIndicatorIdle, identifyUntil: identifyUntil,
	}
	if identifyUntil.After(now) {
		hardware.indicator = HardwareIndicatorIdentify
	}
	hardware.done.Add(2)
	go hardware.sampleButton(ctx)
	go hardware.driveIndicator(ctx)
	return hardware, nil
}

func restorePiIdentifyDeadline(events *recoveryEventQueue, now time.Time) time.Time {
	// An expired token never remains physically active. Cleanup persistence is
	// best effort during startup because the running indicator loop retries it;
	// a transient store error must not take GPIO recovery input offline.
	_ = events.ExpireIdentifyOperations(now)
	return events.ActiveIdentifyDeadline(now)
}

func (hardware *PiRecoveryHardware) sampleButton(ctx context.Context) {
	defer hardware.done.Done()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-ticker.C:
			values := gpioV2LineValues{Mask: 1}
			if err := gpioIOCTL(hardware.buttonFD, gpioGetValuesIOCTL, unsafe.Pointer(&values)); err != nil {
				hardware.detector.Discontinue()
				hardware.mu.Lock()
				hardware.resetPending = false
				hardware.mu.Unlock()
				hardware.setIOError(fmt.Errorf("read BCM17 recovery button: %w", err))
				continue
			}
			action := hardware.detector.Observe(at, values.Bits&1 != 0)
			pending := hardware.detector.ResetPending(at)
			hardware.mu.Lock()
			if hardware.resetPending != pending {
				hardware.resetPending = pending
				hardware.indicatorAt = at
			}
			hardware.mu.Unlock()
			if action == HardwareRecoveryNone {
				continue
			}
			event := HardwareRecoveryEvent{Action: action, ReleasedAt: at.UTC(), SnapshotValid: true}
			if action == HardwareRecoveryArm {
				devices, snapshotErr := hardware.snapshotter.Devices(ctx)
				if snapshotErr != nil {
					event.SnapshotValid = false
				} else {
					event.ConnectedAdapterIDs, event.SnapshotValid = recoveryLinkSnapshot(devices)
				}
			}
			if err := hardware.events.PublishEvent(event); err != nil {
				hardware.setIOError(err)
			}
		}
	}
}

func (hardware *PiRecoveryHardware) driveIndicator(ctx context.Context) {
	defer hardware.done.Done()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = hardware.writeLED(false)
			return
		case at := <-ticker.C:
			// Cleanup persistence is retried on every tick, but it must not
			// poison GPIO input or delay the physical absolute expiry.
			_ = hardware.refreshIdentifyIndicator(at)
			hardware.mu.Lock()
			state := hardware.indicator
			started := hardware.indicatorAt
			if hardware.resetPending {
				state = HardwareIndicatorResetPending
			}
			hardware.mu.Unlock()
			if err := hardware.writeLED(RecoveryIndicatorOn(state, at.Sub(started))); err != nil {
				hardware.setIOError(fmt.Errorf("write BCM27 recovery indicator: %w", err))
			}
		}
	}
}

func (hardware *PiRecoveryHardware) refreshIdentifyIndicator(at time.Time) error {
	activeUntil := hardware.events.ActiveIdentifyDeadline(at)
	hardware.mu.Lock()
	hardware.identifyUntil = activeUntil
	state := hardware.recoveryState
	if (state == "" || state == HardwareIndicatorIdle) && activeUntil.After(at) {
		state = HardwareIndicatorIdentify
	}
	if state != hardware.indicator {
		hardware.indicator = state
		hardware.indicatorAt = at
	}
	hardware.mu.Unlock()
	return hardware.events.ExpireIdentifyOperations(at)
}

func (hardware *PiRecoveryHardware) writeLED(on bool) error {
	values := gpioV2LineValues{Mask: 1}
	if on {
		values.Bits = 1
	}
	return gpioIOCTL(hardware.ledFD, gpioSetValuesIOCTL, unsafe.Pointer(&values))
}

func (hardware *PiRecoveryHardware) ReadRecoveryEvent(context.Context) (HardwareRecoveryEvent, bool, error) {
	hardware.mu.Lock()
	err := hardware.ioErr
	hardware.mu.Unlock()
	if err != nil {
		return HardwareRecoveryEvent{}, false, err
	}
	return hardware.events.Read(context.Background())
}

func (hardware *PiRecoveryHardware) AcknowledgeRecoveryEvent(ctx context.Context, eventID string) error {
	if err := hardware.events.Acknowledge(ctx, eventID); err != nil {
		return err
	}
	hardware.mu.Lock()
	hardware.resetPending = false
	hardware.indicatorAt = time.Now()
	hardware.mu.Unlock()
	return nil
}

func (hardware *PiRecoveryHardware) SetRecoveryIndicator(_ context.Context, state HardwareIndicatorState) error {
	if !state.valid() {
		return errors.New("invalid recovery indicator state")
	}
	hardware.mu.Lock()
	hardware.recoveryState = state
	if state == HardwareIndicatorIdle && hardware.identifyUntil.After(time.Now()) {
		hardware.indicator = HardwareIndicatorIdentify
	} else {
		hardware.indicator = state
	}
	hardware.indicatorAt = time.Now()
	err := hardware.ioErr
	hardware.mu.Unlock()
	return err
}

func (hardware *PiRecoveryHardware) SetIdentifyIndicator(
	_ context.Context, operationID string, activeUntil time.Time, recoveryState HardwareIndicatorState,
) (time.Time, error) {
	hardware.mu.Lock()
	ioErr := hardware.ioErr
	hardware.mu.Unlock()
	if ioErr != nil {
		return time.Time{}, ioErr
	}
	actualUntil, created, err := hardware.events.SetIdentifyOperation(operationID, activeUntil)
	if err != nil {
		return actualUntil, err
	}
	now := time.Now()
	hardware.mu.Lock()
	hardware.identifyUntil = hardware.events.ActiveIdentifyDeadline(now)
	if recoveryState == "" {
		recoveryState = HardwareIndicatorIdle
	}
	hardware.recoveryState = recoveryState
	if recoveryState == HardwareIndicatorIdle && hardware.identifyUntil.After(now) {
		hardware.indicator = HardwareIndicatorIdentify
	} else {
		hardware.indicator = recoveryState
	}
	if created {
		hardware.indicatorAt = now
	}
	hardware.mu.Unlock()
	return actualUntil, nil
}

func (hardware *PiRecoveryHardware) AcknowledgeIdentifyIndicator(_ context.Context, operationID string) error {
	now := time.Now()
	acknowledgeErr := hardware.events.AcknowledgeIdentifyOperation(operationID, now)
	// Refresh from the absolute deadline even when acknowledgement persistence
	// fails; durable settlement can retry without extending the physical effect.
	refreshErr := hardware.refreshIdentifyIndicator(now)
	return errors.Join(acknowledgeErr, refreshErr)
}

func (hardware *PiRecoveryHardware) setIOError(err error) {
	if err == nil {
		return
	}
	hardware.mu.Lock()
	if hardware.ioErr == nil {
		hardware.ioErr = err
	}
	hardware.mu.Unlock()
}

func (hardware *PiRecoveryHardware) Close() error {
	hardware.cancel()
	hardware.done.Wait()
	buttonErr := unix.Close(hardware.buttonFD)
	ledErr := unix.Close(hardware.ledFD)
	return errors.Join(buttonErr, ledErr)
}

var _ RecoveryHardware = (*PiRecoveryHardware)(nil)
