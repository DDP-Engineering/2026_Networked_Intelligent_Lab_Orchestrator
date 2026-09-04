package nethelper

import (
	"errors"
	"io"
	"io/fs"
)

const maxProbeFile = 4 << 20

func readProbeFile(root fs.FS, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxProbeFile+1))
	if len(raw) > maxProbeFile {
		return nil, errors.New("metadata exceeds limit")
	}
	return raw, err
}
