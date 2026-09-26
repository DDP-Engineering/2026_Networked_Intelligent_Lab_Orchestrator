package bench

import (
	"path/filepath"
	"regexp"
	"strings"

	"nilo.local/nilo/internal/integration/manifest"
)

var packageVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func versionParts(value string) []string {
	parts := packageVersionPattern.FindStringSubmatch(value)
	if parts == nil {
		return nil
	}
	for _, identifier := range strings.Split(parts[4], ".") {
		if numericIdentifier(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return nil
		}
	}
	return parts
}

func numericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func compareNumeric(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return strings.Compare(left, right)
}

// newerPackageVersion implements SemVer precedence without integer size limits.
func newerPackageVersion(candidate, installed string) bool {
	a, b := versionParts(candidate), versionParts(installed)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		if comparison := compareNumeric(a[i], b[i]); comparison != 0 {
			return comparison > 0
		}
	}
	if a[4] == b[4] {
		return false
	}
	if a[4] == "" {
		return true
	}
	if b[4] == "" {
		return false
	}
	left, right := strings.Split(a[4], "."), strings.Split(b[4], ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] == right[i] {
			continue
		}
		ln, rn := numericIdentifier(left[i]), numericIdentifier(right[i])
		if ln && rn {
			return compareNumeric(left[i], right[i]) > 0
		}
		if ln != rn {
			return !ln
		}
		return left[i] > right[i]
	}
	return len(left) > len(right)
}

// Update information describes only a newer, locally staged, verified bundle.
func (m *Manager) describeUpdate(plugin *Plugin) {
	if !plugin.Installed || m.options.BundleDir == "" {
		return
	}
	root := filepath.Join(m.options.BundleDir, plugin.ID)
	descriptor, err := manifest.Load(root)
	if err != nil || descriptor.Package.ID != plugin.ID || !newerPackageVersion(descriptor.Package.Version, plugin.Version) || verifyAdapterContract(descriptor) != nil {
		return
	}
	if _, err := verifyPackage(root); err != nil {
		return
	}
	plugin.AvailableVersion = descriptor.Package.Version
	plugin.UpdateAvailable = true
}
