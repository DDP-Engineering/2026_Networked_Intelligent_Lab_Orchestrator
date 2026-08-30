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
	var sanitized strings.Builder
	for index := 0; index < len(value); {
		runeValue := rune(value[index])
		if runeValue == '\x1b' {
			index++
			if index < len(value) && value[index] == '[' {
				index++
				for index < len(value) {
					character := value[index]
					index++
					if character >= '@' && character <= '~' {
						break
					}
				}
			}
			continue
		}
		if runeValue < 0x20 && runeValue != '\n' && runeValue != '\t' {
			index++
			continue
		}
		sanitized.WriteByte(value[index])
		index++
	}
	value = sanitized.String()
	if len(value) > maxDiagnosticBytes {
		value = value[len(value)-maxDiagnosticBytes:]
	}
	return value
}
