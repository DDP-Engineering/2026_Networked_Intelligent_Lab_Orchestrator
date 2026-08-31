//go:build windows

package artifact

import "os"

// NTFS hard-link creation is atomic and fails when destination already exists.
// Keeping the temporary link provides the same cleanup identity token as Unix.
func placeNoReplace(root *os.Root, temporary, destination string) error {
	return root.Link(temporary, destination)
}

// Windows does not expose a portable directory-fsync operation through Go's
// os package. The verified file itself is flushed before atomic placement.
func syncContainingDirectory(_ *os.Root, _ string) error {
	return nil
}
