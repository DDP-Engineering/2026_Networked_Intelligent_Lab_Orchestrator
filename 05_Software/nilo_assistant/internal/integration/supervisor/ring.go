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
	runes := []rune(value)
	var sanitized strings.Builder
	for index := 0; index < len(runes); {
		runeValue := runes[index]
		if runeValue == '\x1b' {
			index++
			if index < len(runes) && runes[index] == '[' {
				index++
				for index < len(runes) {
					character := runes[index]
					index++
					if character >= '@' && character <= '~' {
						break
					}
				}
			}
			continue
		}
		if (runeValue < 0x20 && runeValue != '\n' && runeValue != '\t') || runeValue == 0x7f || (runeValue >= 0x80 && runeValue <= 0x9f) {
			index++
			continue
		}
		sanitized.WriteRune(runeValue)
		index++
	}
	value = sanitized.String()
	if len(value) > maxDiagnosticBytes {
		value = value[len(value)-maxDiagnosticBytes:]
	}
	return value
}
