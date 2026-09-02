package auth

import (
	"testing"
	"time"
)

func TestLoginThrottleRefusesAfterTheFailureLimitUntilTheWindowElapses(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	throttle := NewLoginThrottle(3, 15*time.Minute)

	for attempt := 0; attempt < 3; attempt++ {
		if !throttle.Allow("operator", start) {
			t.Fatalf("attempt %d refused before the limit was reached", attempt)
		}
		throttle.RecordFailure("operator", start)
	}
	if throttle.Allow("operator", start.Add(time.Minute)) {
		t.Fatal("throttle allowed an attempt after the failure limit")
	}
	if !throttle.Allow("other", start.Add(time.Minute)) {
		t.Fatal("throttle refused an unrelated username")
	}
	if throttle.Allow("operator", start.Add(15*time.Minute-time.Nanosecond)) {
		t.Fatal("throttle released the username before the window elapsed")
	}
	if !throttle.Allow("operator", start.Add(15*time.Minute)) {
		t.Fatal("throttle did not release the username after the window elapsed")
	}
}

func TestLoginThrottleClearsCountedFailuresOnSuccess(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	throttle := NewLoginThrottle(2, 15*time.Minute)

	throttle.RecordFailure("operator", start)
	throttle.Reset("operator")
	throttle.RecordFailure("operator", start)

	if !throttle.Allow("operator", start) {
		t.Fatal("throttle counted failures that a successful login had cleared")
	}
}

func TestLoginThrottleTreatsUsernamesCaseInsensitively(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	throttle := NewLoginThrottle(1, time.Minute)

	throttle.RecordFailure("Operator", start)

	if throttle.Allow("operator", start) {
		t.Fatal("throttle let a different letter case escape the counter")
	}
}
