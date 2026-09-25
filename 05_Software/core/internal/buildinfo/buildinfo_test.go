package buildinfo

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentHasProtocolAndVersion(t *testing.T) {
	got := Current()
	if got.Name != "nilo" || got.Version == "" || got.IntegrationProtocol != "nilo.integration/1" {
		t.Fatalf("unexpected build info: %#v", got)
	}
}

func TestVersionMirrorsRepositoryVersionFile(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "VERSION")
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("repository VERSION file is not present outside a source checkout")
	}
	if err != nil {
		t.Fatalf("read repository VERSION file: %v", err)
	}
	declared := strings.TrimSpace(string(content))
	if Version != declared {
		t.Fatalf("buildinfo.Version = %q, repository VERSION file declares %q", Version, declared)
	}
}
