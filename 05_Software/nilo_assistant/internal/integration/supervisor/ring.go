package supervisor

import (
	"strings"
	"sync"
)

const maxDiagnosticBytes = 64 << 10

// diagnosticRing retains only the tail of untrusted runner stderr.
type diagnosticRing struct {
	mu   sync.Mutex
	data []byte
}

func (ring *diagnosticRing) Write(value []byte) (int, error) {
	ring.mu.Lock()
	defer ring.mu.Unlock()
	ring.data = append(ring.data, value...)
	if len(ring.data) > maxDiagnosticBytes {
		ring.data = append([]byte(nil), ring.data[len(ring.data)-maxDiagnosticBytes:]...)
	}
	return len(value), nil
}

func (ring *diagnosticRing) String() string {
	ring.mu.Lock()
	defer ring.mu.Unlock()
	return sanitizeDiagnostics(string(ring.data))
}

func sanitizeDiagnostics(value string) string {
	value = strings.ToValidUTF8(value, "?")
	value = strings.ReplaceAll(value, "\x00", "?")
	if len(value) > maxDiagnosticBytes {
		value = value[len(value)-maxDiagnosticBytes:]
	}
	return value
}
