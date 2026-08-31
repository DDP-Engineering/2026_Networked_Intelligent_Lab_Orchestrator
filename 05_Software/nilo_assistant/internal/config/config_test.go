package config

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadUsesDefaultAndProvidedDataDirectories(t *testing.T) {
	defaultBase := t.TempDir()
	defaultDataDir := filepath.Join(defaultBase, "nilo")
	if runtime.GOOS == "windows" {
		t.Setenv("ProgramData", defaultBase)
		defaultDataDir = filepath.Join(defaultBase, "NILO")
	} else {
		t.Setenv("XDG_STATE_HOME", defaultBase)
	}
	providedDataDir := filepath.Join(t.TempDir(), "nilo-data")

	for _, test := range []struct {
		name    string
		dataDir string
		wantDir string
	}{
		{name: "default", wantDir: defaultDataDir},
		{name: "provided override", dataDir: providedDataDir, wantDir: providedDataDir},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := Load(Inputs{
				DataDir:     test.dataDir,
				BindAddress: "127.0.0.1:8080",
			})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			want := Config{
				DataDir:        test.wantDir,
				DatabasePath:   filepath.Join(test.wantDir, "nilo.db"),
				ArtifactRoot:   filepath.Join(test.wantDir, "artifacts"),
				IntegrationDir: filepath.Join(test.wantDir, "integrations"),
				BindAddress:    "127.0.0.1:8080",
			}
			if got != want {
				t.Fatalf("Load() = %#v, want %#v", got, want)
			}
		})
	}
}

func TestLoadRejectsRelativeDataDir(t *testing.T) {
	_, err := Load(Inputs{DataDir: "relative/data", BindAddress: "127.0.0.1:8080"})
	if !errors.Is(err, ErrDataDirNotAbsolute) {
		t.Fatalf("expected ErrDataDirNotAbsolute, got %v", err)
	}
}

func TestLoadRejectsMissingBindAddress(t *testing.T) {
	_, err := Load(Inputs{DataDir: t.TempDir()})
	if err == nil {
		t.Fatal("Load() succeeded without a bind address")
	}
}
