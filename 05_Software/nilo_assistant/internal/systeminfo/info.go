// Package systeminfo reports a small, non-sensitive appliance host summary.
package systeminfo

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// Info contains the host facts safe to expose through authenticated status.
// Storage values describe the filesystem containing the NILO data directory;
// the directory path itself is deliberately not disclosed.
type Info struct {
	OperatingSystem   string `json:"operating_system"`
	KernelVersion     string `json:"kernel_version"`
	Architecture      string `json:"architecture"`
	StorageFreeBytes  uint64 `json:"storage_free_bytes"`
	StorageTotalBytes uint64 `json:"storage_total_bytes"`
}

// Probe reads host information for the filesystem containing dataPath.
type Probe struct {
	dataPath string
}

// New creates a host-information probe for the NILO data filesystem.
func New(dataPath string) *Probe {
	return &Probe{dataPath: dataPath}
}

// Snapshot returns the currently observable host and storage facts.
func (probe *Probe) Snapshot() Info {
	return platformInfo(probe.dataPath)
}

func parseOSRelease(contents []byte) string {
	values := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || (key != "PRETTY_NAME" && key != "NAME" && key != "VERSION_ID") {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		values[key] = strings.TrimSpace(value)
	}
	if values["PRETTY_NAME"] != "" {
		return values["PRETTY_NAME"]
	}
	return strings.TrimSpace(values["NAME"] + " " + values["VERSION_ID"])
}
