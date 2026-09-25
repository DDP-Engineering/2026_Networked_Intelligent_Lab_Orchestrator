package nethelper

import (
	"errors"
	"regexp"
)

var systemHostLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// PersonalizeHostname changes an exact configured image default and leaves
// every other administrator-selected hostname untouched.
func PersonalizeHostname(current string, imageDefaults []string, serial string, set func(string) error) (bool, error) {
	if !systemHostLabel.MatchString(current) || !validPermanentSerialName(serial) || set == nil {
		return false, errors.New("valid hostname personalization inputs are required")
	}
	matched := false
	seen := map[string]bool{}
	for _, candidate := range imageDefaults {
		if !systemHostLabel.MatchString(candidate) || seen[candidate] {
			return false, errors.New("image-default hostnames must be unique lowercase DNS labels")
		}
		seen[candidate] = true
		matched = matched || current == candidate
	}
	if !matched || current == serial {
		return false, nil
	}
	if err := set(serial); err != nil {
		return false, err
	}
	return true, nil
}
