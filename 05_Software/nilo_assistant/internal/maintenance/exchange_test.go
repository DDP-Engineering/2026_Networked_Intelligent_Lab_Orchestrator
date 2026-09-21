package maintenance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPortableDirectoryExchangeReversesAFailedFinalRename(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	left := filepath.Join(root, "live")
	right := filepath.Join(root, "restored")
	if err := os.Mkdir(left, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(right, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "identity"), []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(right, "identity"), []byte("restored"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	rename := func(oldPath, newPath string) error {
		calls++
		if calls == 3 {
			return errors.New("injected final rename failure")
		}
		return os.Rename(oldPath, newPath)
	}
	if err := exchangeDirectoriesPortable(left, right, rename); err == nil {
		t.Fatal("exchange unexpectedly succeeded")
	}
	contents, err := os.ReadFile(filepath.Join(left, "identity"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "live" {
		t.Fatalf("live directory after failed exchange = %q", contents)
	}
	contents, err = os.ReadFile(filepath.Join(right, "identity"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "restored" {
		t.Fatalf("restored directory after failed exchange = %q", contents)
	}
}
