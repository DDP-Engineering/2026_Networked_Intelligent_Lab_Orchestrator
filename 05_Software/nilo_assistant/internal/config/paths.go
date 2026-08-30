package config

import (
	"errors"
	"path/filepath"
	"strings"
)

func ResolveWithin(root, candidate string) (string, error) {
	if strings.TrimSpace(candidate) == "" {
		return "", errors.New("path candidate is required")
	}
	if filepath.IsAbs(candidate) {
		return "", errors.New("path candidate must be relative")
	}

	absoluteRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	absoluteCandidate, err := filepath.Abs(filepath.Join(absoluteRoot, candidate))
	if err != nil {
		return "", err
	}

	relative, err := filepath.Rel(absoluteRoot, absoluteCandidate)
	if err != nil {
		return "", err
	}
	components := strings.Split(relative, string(filepath.Separator))
	if relative == "." || filepath.IsAbs(relative) || components[0] == ".." {
		return "", errors.New("path candidate escapes root")
	}

	return absoluteCandidate, nil
}
