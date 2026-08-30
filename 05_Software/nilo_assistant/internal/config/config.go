package config

import (
	"errors"
	"path/filepath"
	"strings"
)

var ErrDataDirNotAbsolute = errors.New("data directory must be absolute")

type Inputs struct {
	DataDir     string
	BindAddress string
}

type Config struct {
	DataDir        string
	DatabasePath   string
	ArtifactRoot   string
	IntegrationDir string
	BindAddress    string
}

func Load(in Inputs) (Config, error) {
	dataDir := in.DataDir
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	if !filepath.IsAbs(dataDir) {
		return Config{}, ErrDataDirNotAbsolute
	}
	if strings.TrimSpace(in.BindAddress) == "" {
		return Config{}, errors.New("bind address is required")
	}

	dataDir, err := filepath.Abs(filepath.Clean(dataDir))
	if err != nil {
		return Config{}, err
	}

	return Config{
		DataDir:        dataDir,
		DatabasePath:   filepath.Join(dataDir, "nilo.db"),
		ArtifactRoot:   filepath.Join(dataDir, "artifacts"),
		IntegrationDir: filepath.Join(dataDir, "integrations"),
		BindAddress:    in.BindAddress,
	}, nil
}
