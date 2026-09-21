//go:build !windows

package bench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPackageLifecyclePreservesEmbeddedRuntimeExecuteModes(t *testing.T) {
	manager, actor, _ := testManager(t, "ask")
	assertExecutable := func() {
		t.Helper()
		root, err := manager.packageRoot("sigrok")
		if err != nil {
			t.Fatal(err)
		}
		for _, relative := range []string{"runtime/bin/python3", "runtime/bin/sigrok-cli"} {
			path := filepath.Join(root, relative)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Fatalf("installed runtime is not executable: %s mode=%o", relative, info.Mode().Perm())
			}
			if output, err := exec.Command(path).CombinedOutput(); err != nil {
				t.Fatalf("execute installed runtime %s: %v (%s)", relative, err, output)
			}
		}
	}
	assertExecutable()
	rewriteCatalogVersion(t, manager, "2.0.0")
	if _, err := manager.Lifecycle(context.Background(), actor, "sigrok", "upgrade", ""); err != nil {
		t.Fatal(err)
	}
	assertExecutable()
}
