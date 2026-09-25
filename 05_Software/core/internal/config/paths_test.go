package config

import (
	"path/filepath"
	"testing"
)

func TestResolveWithinAcceptsConfinedCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")

	for _, candidate := range []string{
		"result.json",
		filepath.Join("runs", "run-42", "stdout.log"),
		filepath.Join("runs", "run-42", "..", "run-43", "stderr.log"),
	} {
		t.Run(candidate, func(t *testing.T) {
			got, err := ResolveWithin(root, candidate)
			if err != nil {
				t.Fatalf("ResolveWithin(%q, %q) error = %v", root, candidate, err)
			}

			want := filepath.Join(root, candidate)
			if got != want {
				t.Fatalf("ResolveWithin(%q, %q) = %q, want %q", root, candidate, got, want)
			}
		})
	}
}

func TestResolveWithinRejectsUnconfinedCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	outside := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-outside", "result.json")

	for _, candidate := range []struct {
		name      string
		candidate string
	}{
		{name: "empty", candidate: ""},
		{name: "equality", candidate: "."},
		{name: "absolute escape", candidate: outside},
		{name: "traversal", candidate: filepath.Join("..", "result.json")},
		{name: "prefix confusion", candidate: filepath.Join("..", "artifacts-outside", "result.json")},
	} {
		t.Run(candidate.name, func(t *testing.T) {
			if got, err := ResolveWithin(root, candidate.candidate); err == nil {
				t.Fatalf("ResolveWithin(%q, %q) = %q, want rejection", root, candidate.candidate, got)
			}
		})
	}
}
