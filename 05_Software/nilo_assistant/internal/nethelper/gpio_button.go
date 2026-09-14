package nethelper

import "time"

// HardwareRecoveryAction is the bounded event emitted when the Pi recovery
// button is released. Holding the line never performs an action by itself.
type HardwareRecoveryAction string

const (
	HardwareRecoveryNone               HardwareRecoveryAction = ""
	HardwareRecoveryArm                HardwareRecoveryAction = "arm_recovery"
	HardwareRecoveryResetCommissioning HardwareRecoveryAction = "reset_commissioning"
)

// RecoveryButtonDetector turns sampled electrical levels into one action on a
// debounced release. The initial level establishes state without an action.
type RecoveryButtonDetector struct {
	debounce     time.Duration
	initialized  bool
	stable       bool
	pending      bool
	pendingSince time.Time
	pressAt      time.Time
	needsRelease bool
	releasedAt   time.Time
}

func NewRecoveryButtonDetector(debounce time.Duration) *RecoveryButtonDetector {
	if debounce <= 0 {
		debounce = 50 * time.Millisecond
	}
	return &RecoveryButtonDetector{debounce: debounce}
}

// Observe consumes one sampled logical pressed state.
func (detector *RecoveryButtonDetector) Observe(at time.Time, pressed bool) HardwareRecoveryAction {
	if detector.needsRelease {
		if pressed {
			detector.releasedAt = time.Time{}
			return HardwareRecoveryNone
		}
		if detector.releasedAt.IsZero() {
			detector.releasedAt = at
			return HardwareRecoveryNone
		}
		if at.Sub(detector.releasedAt) < detector.debounce {
			return HardwareRecoveryNone
		}
		detector.needsRelease = false
		detector.releasedAt = time.Time{}
		detector.initialized = true
		detector.stable = false
		detector.pending = false
		detector.pendingSince = at
		return HardwareRecoveryNone
	}
	if !detector.initialized {
		detector.initialized = true
		detector.stable = pressed
		detector.pending = pressed
		detector.pendingSince = at
		if pressed {
			detector.pressAt = at
		}
		return HardwareRecoveryNone
	}
	if pressed != detector.pending {
		detector.pending = pressed
		detector.pendingSince = at
		return HardwareRecoveryNone
	}
	if pressed == detector.stable || at.Sub(detector.pendingSince) < detector.debounce {
		return HardwareRecoveryNone
	}
	detector.stable = pressed
	if pressed {
		detector.pressAt = at
		return HardwareRecoveryNone
	}
	if detector.pressAt.IsZero() {
		return HardwareRecoveryNone
	}
	held := at.Sub(detector.pressAt)
	detector.pressAt = time.Time{}
	return ClassifyRecoveryButtonRelease(held)
}

// ResetPending reports whether a stable press has crossed the reset threshold.
// It is indication only; the action is still emitted exactly once on release.
func (detector *RecoveryButtonDetector) ResetPending(at time.Time) bool {
	return detector.initialized && detector.stable && !detector.pressAt.IsZero() && at.Sub(detector.pressAt) >= 10*time.Second
}

// Discontinue invalidates the current electrical gesture after an unreadable
// sample. A later level must establish a fresh baseline and press.
func (detector *RecoveryButtonDetector) Discontinue() {
	detector.initialized = false
	detector.stable = false
	detector.pending = false
	detector.pendingSince = time.Time{}
	detector.pressAt = time.Time{}
	detector.needsRelease = true
	detector.releasedAt = time.Time{}
}

// ClassifyRecoveryButtonRelease applies the reference button timing contract.
func ClassifyRecoveryButtonRelease(held time.Duration) HardwareRecoveryAction {
	switch {
	case held >= 10*time.Second:
		return HardwareRecoveryResetCommissioning
	case held >= 3*time.Second:
		return HardwareRecoveryArm
	default:
		return HardwareRecoveryNone
	}
}

// HardwareIndicatorState is the physical recovery indication requested by the
// unprivileged coordinator. The helper translates it to one LED only.
type HardwareIndicatorState string

const (
	HardwareIndicatorIdle         HardwareIndicatorState = "idle"
	HardwareIndicatorArmed        HardwareIndicatorState = "armed"
	HardwareIndicatorActive       HardwareIndicatorState = "active"
	HardwareIndicatorResetPending HardwareIndicatorState = "reset_pending"
	HardwareIndicatorFailed       HardwareIndicatorState = "failed"
)

func (state HardwareIndicatorState) valid() bool {
	switch state {
	case HardwareIndicatorIdle, HardwareIndicatorArmed, HardwareIndicatorActive,
		HardwareIndicatorResetPending, HardwareIndicatorFailed:
		return true
	default:
		return false
	}
}

// RecoveryIndicatorOn returns the reference LED level at an elapsed offset.
// Callers restart the phase when the state changes.
func RecoveryIndicatorOn(state HardwareIndicatorState, elapsed time.Duration) bool {
	if elapsed < 0 {
		elapsed = 0
	}
	switch state {
	case HardwareIndicatorArmed:
		return elapsed%(time.Second) < 500*time.Millisecond
	case HardwareIndicatorActive:
		return true
	case HardwareIndicatorResetPending:
		return elapsed%(250*time.Millisecond) < 125*time.Millisecond
	case HardwareIndicatorFailed:
		phase := elapsed % (2 * time.Second)
		return phase < 200*time.Millisecond || phase >= 400*time.Millisecond && phase < 600*time.Millisecond
	default:
		return false
	}
}
