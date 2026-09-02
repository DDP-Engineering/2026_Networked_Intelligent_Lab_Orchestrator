package auth

import (
	"strings"
	"sync"
	"time"
)

// LoginThrottle bounds how many password guesses one username may absorb.
// Argon2id makes each guess expensive but does not by itself limit their
// number. The counter is in-process only: it is a brake on a running server,
// not durable lockout state, and it therefore resets when the process restarts.
type LoginThrottle struct {
	mutex    sync.Mutex
	limit    int
	window   time.Duration
	failures map[string]throttleEntry
}

type throttleEntry struct {
	count int
	first time.Time
}

// NewLoginThrottle refuses further attempts once limit consecutive failures
// occur inside window. A non-positive limit disables throttling entirely.
func NewLoginThrottle(limit int, window time.Duration) *LoginThrottle {
	return &LoginThrottle{limit: limit, window: window, failures: make(map[string]throttleEntry)}
}

// Allow reports whether username may attempt a password login at now.
func (throttle *LoginThrottle) Allow(username string, now time.Time) bool {
	if throttle == nil || throttle.limit <= 0 {
		return true
	}
	key := throttleKey(username)
	throttle.mutex.Lock()
	defer throttle.mutex.Unlock()
	entry, tracked := throttle.failures[key]
	if !tracked {
		return true
	}
	if now.Sub(entry.first) >= throttle.window {
		delete(throttle.failures, key)
		return true
	}
	return entry.count < throttle.limit
}

// RecordFailure counts one failed password login for username.
func (throttle *LoginThrottle) RecordFailure(username string, now time.Time) {
	if throttle == nil || throttle.limit <= 0 {
		return
	}
	key := throttleKey(username)
	throttle.mutex.Lock()
	defer throttle.mutex.Unlock()
	entry, tracked := throttle.failures[key]
	if !tracked || now.Sub(entry.first) >= throttle.window {
		throttle.failures[key] = throttleEntry{count: 1, first: now}
		return
	}
	entry.count++
	throttle.failures[key] = entry
}

// Reset clears the counted failures for username after a successful login.
func (throttle *LoginThrottle) Reset(username string) {
	if throttle == nil {
		return
	}
	throttle.mutex.Lock()
	defer throttle.mutex.Unlock()
	delete(throttle.failures, throttleKey(username))
}

func throttleKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
