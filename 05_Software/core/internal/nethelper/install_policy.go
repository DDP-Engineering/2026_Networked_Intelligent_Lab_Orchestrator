package nethelper

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"regexp"
	"strings"
)

var errPackageScope = errors.New("package_scope_refused: the transaction would change packages other than wireguard-tools; update dependencies separately")

var sourceOverride = regexp.MustCompile(`(?:^|[\s\[])(?:trusted|allow-insecure|allow-weak|allow-downgrade-to-insecure|check-valid-until|check-date|valid-until-min|date-max-future)\s*[:=]`)

// VerifyWireGuardTransaction validates APT's version-2 pre-install protocol at
// the point where APT holds its package locks. Simulation alone has a race with
// other package managers. This hook receives no HTTP or RPC parameters.
func VerifyWireGuardTransaction(input io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return errPackageScope
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) < 3 || lines[0] != "VERSION 2" {
		return errPackageScope
	}
	separator := 1
	for separator < len(lines) && lines[separator] != "" {
		separator++
	}
	if separator == len(lines) {
		return errPackageScope
	}
	for _, line := range lines[separator+1:] {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 5 || fields[0] != "wireguard-tools" || fields[4] == "**REMOVE**" {
			return errPackageScope
		}
		if fields[4] != "**CONFIGURE**" && (!strings.HasPrefix(fields[4], "/") || !strings.HasSuffix(fields[4], ".deb")) {
			return errPackageScope
		}
		// Existing installations are handled without invoking APT; a race may
		// turn an install into an upgrade/downgrade, which must be refused.
		if fields[1] != "-" && fields[1] != fields[3] {
			return errPackageScope
		}
	}
	return nil
}

var packageCandidate = regexp.MustCompile(`^Inst wireguard-tools \(([0-9][a-zA-Z0-9.+:~_-]*) `)

func plannedVersion(raw string) (string, error) {
	version := ""
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "Remv":
			return "", errPackageScope
		case "Inst":
			match := packageCandidate.FindStringSubmatch(line)
			if len(match) != 2 || version != "" {
				return "", errPackageScope
			}
			version = match[1]
		case "Conf":
			if len(fields) < 2 || fields[1] != "wireguard-tools" {
				return "", errPackageScope
			}
		}
	}
	if version == "" {
		return "", errors.New("package_candidate_unavailable: no installable wireguard-tools candidate")
	}
	return version, nil
}

func supportedDistribution(raw string) bool {
	values := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = strings.Trim(value, `"'`)
		}
	}
	return (values["ID"] == "debian" && values["VERSION_ID"] == "13") || (values["ID"] == "ubuntu" && values["VERSION_ID"] == "24.04")
}

// Repository credentials stay in local APT files; only their digest leaves this
// function. Reject per-source overrides that bypass archive authentication or
// expiry, including deb822 continuation lines, instead of overriding them.
func signedSources(root fs.FS) ([]byte, error) {
	names := []string{"etc/apt/sources.list"}
	entries, err := fs.ReadDir(root, "etc/apt/sources.list.d")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".list") || strings.HasSuffix(entry.Name(), ".sources") {
			names = append(names, "etc/apt/sources.list.d/"+entry.Name())
		}
	}
	var combined []byte
	for _, name := range names {
		raw, err := readProbeFile(root, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(strings.NewReader(strings.ToLower(string(raw))))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "#") {
				continue
			}
			if sourceOverride.MatchString(line) {
				return nil, errors.New("repository_policy_refused: remove source trust or expiry overrides before installing")
			}
		}
		if scanner.Err() != nil {
			return nil, errors.New("repository_policy_refused: source entry exceeds limit")
		}
		combined = append(combined, []byte(name+"\x00")...)
		combined = append(combined, raw...)
		if len(combined) > maxProbeFile {
			return nil, errors.New("repository_policy_refused: sources exceed limit")
		}
	}
	if len(combined) == 0 {
		return nil, errors.New("repository_unavailable: configure a signed distribution repository")
	}
	return combined, nil
}
