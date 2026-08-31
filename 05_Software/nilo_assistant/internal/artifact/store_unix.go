//go:build !windows

package artifact

import (
	"fmt"
	"os"
)

// placeNoReplace creates the durable name atomically without replacement.
// The temporary hard link remains as an identity token until registration has
// either committed metadata or completed ownership-checked cleanup.
func placeNoReplace(root *os.Root, temporary, destination string) error {
	return root.Link(temporary, destination)
}

func syncContainingDirectory(root *os.Root, relative string) error {
	directory, err := root.Open(relative)
	if err != nil {
		return fmt.Errorf("open containing directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync containing directory: %w", err)
	}
	return nil
}
