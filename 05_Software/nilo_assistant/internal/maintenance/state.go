// Package maintenance owns offline inventory, snapshot, restore and purge
// operations for the NILO application data directory.
package maintenance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nilo.local/nilo/internal/buildinfo"
	storesqlite "nilo.local/nilo/internal/store/sqlite"
)

const snapshotSchema = 3
const ownershipFile = ".nilo-owner.json"

var transactionName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// File describes one application-owned regular file.
type File struct {
	GID    int               `json:"-"`
	Mode   fs.FileMode       `json:"-"`
	Path   string            `json:"path"`
	SHA256 string            `json:"sha256"`
	Size   int64             `json:"size"`
	UID    int               `json:"-"`
	XAttrs map[string]string `json:"-"`
}

type manifestFile struct {
	GID    int               `json:"gid"`
	Mode   uint32            `json:"mode"`
	Path   string            `json:"path"`
	SHA256 string            `json:"sha256"`
	Size   int64             `json:"size"`
	UID    int               `json:"uid"`
	XAttrs map[string]string `json:"xattrs"`
}

type manifestDirectory struct {
	GID    int               `json:"gid"`
	Mode   uint32            `json:"mode"`
	Path   string            `json:"path"`
	UID    int               `json:"uid"`
	XAttrs map[string]string `json:"xattrs"`
}

type snapshotManifest struct {
	DatabaseSchema string              `json:"database_schema"`
	Directories    []manifestDirectory `json:"directories"`
	Files          []manifestFile      `json:"files"`
	Schema         int                 `json:"schema"`
	SourceVersion  string              `json:"source_version"`
	TargetVersion  string              `json:"target_version"`
	Transaction    string              `json:"transaction"`
}

func inventoryDirectories(root string) ([]manifestDirectory, error) {
	directories := make([]manifestDirectory, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		first := strings.Split(relative, string(filepath.Separator))[0]
		if relative != "." && entry.IsDir() && (first == "snapshots" || first == "transactions") {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("application state contains a symbolic link: %s", relative)
		}
		uid, gid, attributes, err := captureFileMetadata(path, info)
		if err != nil {
			return fmt.Errorf("inspect directory metadata for %s: %w", relative, err)
		}
		directories = append(directories, manifestDirectory{
			GID: gid, Mode: uint32(info.Mode().Perm()), Path: filepath.ToSlash(relative),
			UID: uid, XAttrs: attributes,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(directories, func(left, right int) bool { return directories[left].Path < directories[right].Path })
	return directories, nil
}

type ownershipMarker struct {
	ApplianceID string `json:"appliance_id"`
	DataDir     string `json:"data_dir"`
	Schema      int    `json:"schema"`
}

// EnsureOwnership binds the data directory to the appliance identity stored in
// its database. It never replaces a conflicting marker.
func EnsureOwnership(dataDir, applianceID string) error {
	root, err := canonicalDataDir(dataDir)
	if err != nil {
		return err
	}
	if applianceID == "" || strings.ContainsAny(applianceID, "\r\n\t ") {
		return errors.New("appliance ownership identity is invalid")
	}
	markerPath := filepath.Join(root, ownershipFile)
	encoded, err := json.Marshal(ownershipMarker{ApplianceID: applianceID, DataDir: root, Schema: 1})
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if existing, readErr := os.ReadFile(markerPath); readErr == nil {
		if !bytes.Equal(existing, encoded) {
			return errors.New("data directory ownership marker conflicts with the database identity")
		}
		return nil
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}
	if err := writeFileDurable(markerPath, encoded, 0o600); err != nil {
		return err
	}
	return syncDirectory(root)
}

func verifyOwnership(root string) error {
	markerPath := filepath.Join(root, ownershipFile)
	info, err := os.Lstat(markerPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("purge requires a verified NILO ownership marker")
	}
	encoded, err := os.ReadFile(markerPath)
	if err != nil {
		return errors.New("purge requires a verified NILO ownership marker")
	}
	var marker ownershipMarker
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&marker) != nil || decoder.Decode(&struct{}{}) != io.EOF || marker.Schema != 1 || marker.ApplianceID == "" || marker.DataDir != root {
		return errors.New("purge requires a verified NILO ownership marker")
	}
	identity, err := storesqlite.ApplianceIdentity(context.Background(), filepath.Join(root, "nilo.db"))
	if err != nil || identity != marker.ApplianceID {
		return errors.New("purge requires a data-directory marker bound to its NILO database")
	}
	return nil
}

func sqliteDatabase(path string) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	header := make([]byte, 16)
	if _, err := io.ReadFull(file, header); err != nil {
		return false, nil
	}
	return string(header) == "SQLite format 3\x00", nil
}

func inspectDatabase(path string) (string, error) {
	isSQLite, err := sqliteDatabase(path)
	if err != nil {
		return "", err
	}
	if !isSQLite {
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			return "", nil
		}
		return "", errors.New("nilo.db is not a NILO SQLite database")
	}
	return storesqlite.CheckpointAndInspect(context.Background(), path)
}

// SnapshotResult reports a completed, integrity-described snapshot.
type SnapshotResult struct {
	Files    int    `json:"files"`
	Schema   int    `json:"schema"`
	Snapshot string `json:"snapshot"`
}

func canonicalDataDir(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("data directory must be an absolute path")
	}
	clean := filepath.Clean(path)
	if clean == filepath.VolumeName(clean)+string(filepath.Separator) {
		return "", errors.New("data directory must not be a filesystem root")
	}
	current := filepath.VolumeName(clean) + string(filepath.Separator)
	relative := strings.TrimPrefix(clean, current)
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("data directory must not contain symbolic-link components")
		}
	}
	return clean, nil
}

func hashFile(path string) (string, error) {
	stream, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, stream); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// Inventory returns every regular application-owned file outside maintenance
// snapshots and transaction journals. Links and special files are refused.
func Inventory(dataDir string) ([]File, error) {
	root, err := canonicalDataDir(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("prepare data directory: %w", err)
	}
	files := make([]File, 0)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		first := strings.Split(relative, string(filepath.Separator))[0]
		if entry.IsDir() && (first == "snapshots" || first == "transactions") {
			return filepath.SkipDir
		}
		if relative == ".maintenance.lock" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("application state contains a symbolic link: %s", relative)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("application state contains a special file: %s", relative)
		}
		digest, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash %s: %w", relative, err)
		}
		uid, gid, attributes, err := captureFileMetadata(path, info)
		if err != nil {
			return fmt.Errorf("inspect metadata for %s: %w", relative, err)
		}
		files = append(files, File{GID: gid, Mode: info.Mode().Perm(), Path: filepath.ToSlash(relative), SHA256: digest, Size: info.Size(), UID: uid, XAttrs: attributes})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	return files, nil
}

func acquire(dataDir string) (func() error, error) {
	if _, err := os.Lstat(portableExchangeMarker(dataDir)); err == nil {
		return nil, errors.New("another maintenance operation is active")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	path := filepath.Join(dataDir, ".maintenance.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, errors.New("another maintenance operation is active")
		}
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return func() error { return errors.Join(file.Close(), os.Remove(path)) }, nil
}

func portableExchangeMarker(dataDir string) string {
	return filepath.Join(filepath.Dir(dataDir), "."+filepath.Base(dataDir)+".maintenance-exchange.lock")
}

func acquirePortableExchangeExclusion(dataDir string) (func() error, error) {
	path := portableExchangeMarker(dataDir)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, errors.New("another maintenance exchange is active")
		}
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	if err := syncDirectory(filepath.Dir(dataDir)); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return func() error {
		return errors.Join(os.Remove(path), syncDirectory(filepath.Dir(dataDir)))
	}, nil
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	return errors.Join(copyErr, syncErr, closeErr)
}

func copyFileWithMetadata(source, destination string, file File) error {
	if err := copyFile(source, destination, file.Mode); err != nil {
		return err
	}
	return applyFileMetadata(destination, file.Mode, file.UID, file.GID, file.XAttrs)
}

func verifyFileMetadata(path string, expected File) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	uid, gid, attributes, err := captureFileMetadata(path, info)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != expected.Mode.Perm() || uid != expected.UID || gid != expected.GID || !maps.Equal(attributes, expected.XAttrs) {
		return errors.New("file metadata mismatch")
	}
	return nil
}

func copyDirectoryTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("maintenance state contains a symbolic link")
		}
		uid, gid, attributes, err := captureFileMetadata(path, info)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative == "." {
				return os.Mkdir(target, info.Mode().Perm())
			}
			if err := os.Mkdir(target, info.Mode().Perm()); err != nil {
				return err
			}
			return applyFileMetadata(target, info.Mode(), uid, gid, attributes)
		}
		if !info.Mode().IsRegular() {
			return errors.New("maintenance state contains a special file")
		}
		return copyFileWithMetadata(path, target, File{GID: gid, Mode: info.Mode(), UID: uid, XAttrs: attributes})
	})
}

func syncTree(root string) error {
	directories := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		// The staging lock was already fsynced by writeFileDurable. Avoid a
		// second read-handle Sync, which Windows rejects for this lock file.
		if entry.Name() == ".maintenance.lock" {
			return nil
		}
		return syncFile(path)
	})
	if err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncDirectory(directories[index]); err != nil {
			return err
		}
	}
	return nil
}

func writeFileDurable(path string, contents []byte, mode fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	_, writeErr := file.Write(contents)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

// Snapshot copies a verified point-in-time application state. The caller must
// quiesce the running service before invoking it.
func Snapshot(dataDir, transaction, targetVersion string) (result SnapshotResult, err error) {
	root, err := canonicalDataDir(dataDir)
	if err != nil {
		return result, err
	}
	if !transactionName.MatchString(transaction) {
		return result, errors.New("transaction must contain only letters, digits, dot, dash or underscore")
	}
	if targetVersion == "" {
		targetVersion = buildinfo.Version
	}
	if strings.ContainsAny(targetVersion, "\r\n\t ") {
		return result, errors.New("target version must be one non-empty token")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return result, err
	}
	release, err := acquire(root)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, release()) }()
	databaseSchema, err := inspectDatabase(filepath.Join(root, "nilo.db"))
	if err != nil {
		return result, fmt.Errorf("prepare SQLite snapshot: %w", err)
	}
	files, err := Inventory(root)
	if err != nil {
		return result, err
	}
	directories, err := inventoryDirectories(root)
	if err != nil {
		return result, err
	}
	snapshots := filepath.Join(root, "snapshots")
	if err := os.MkdirAll(snapshots, 0o750); err != nil {
		return result, err
	}
	var contentBytes uint64
	for _, file := range files {
		if file.Size < 0 {
			return result, errors.New("inventory contains a negative file size")
		}
		contentBytes += uint64(file.Size)
	}
	available, err := availableBytes(snapshots)
	if err != nil {
		return result, fmt.Errorf("inspect snapshot free space: %w", err)
	}
	snapshotReserve := contentBytes / 10
	const minimumSnapshotReserve = uint64(256 * 1024 * 1024)
	if snapshotReserve < minimumSnapshotReserve {
		snapshotReserve = minimumSnapshotReserve
	}
	if available < contentBytes+snapshotReserve {
		return result, fmt.Errorf("insufficient free space for snapshot: have %d bytes, require %d", available, contentBytes+snapshotReserve)
	}
	final := filepath.Join(snapshots, transaction)
	temporary := filepath.Join(snapshots, "."+transaction+".tmp")
	if _, err := os.Lstat(final); err == nil {
		return result, errors.New("snapshot transaction already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return result, err
	}
	if err := os.Mkdir(temporary, 0o750); err != nil {
		return result, err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	manifest := snapshotManifest{
		DatabaseSchema: databaseSchema, Directories: directories,
		Files: make([]manifestFile, 0, len(files)), Schema: snapshotSchema,
		SourceVersion: buildinfo.Version, TargetVersion: targetVersion, Transaction: transaction,
	}
	filesRoot := filepath.Join(temporary, "files")
	for _, directory := range directories {
		destination := filesRoot
		if directory.Path != "." {
			destination = filepath.Join(filesRoot, filepath.FromSlash(directory.Path))
		}
		if err := os.Mkdir(destination, fs.FileMode(directory.Mode)); err != nil {
			return result, fmt.Errorf("snapshot directory %s: %w", directory.Path, err)
		}
	}
	for _, file := range files {
		source := filepath.Join(root, filepath.FromSlash(file.Path))
		destination := filepath.Join(temporary, "files", filepath.FromSlash(file.Path))
		if err := copyFileWithMetadata(source, destination, file); err != nil {
			return result, fmt.Errorf("snapshot %s: %w", file.Path, err)
		}
		if err := verifyFileMetadata(destination, file); err != nil {
			return result, fmt.Errorf("snapshot metadata changed for %s: %w", file.Path, err)
		}
		digest, hashErr := hashFile(destination)
		if hashErr != nil || digest != file.SHA256 {
			return result, fmt.Errorf("application state changed while snapshotting: %s", file.Path)
		}
		manifest.Files = append(manifest.Files, manifestFile{GID: file.GID, Mode: uint32(file.Mode.Perm()), Path: file.Path, SHA256: file.SHA256, Size: file.Size, UID: file.UID, XAttrs: file.XAttrs})
	}
	for index := len(directories) - 1; index >= 0; index-- {
		directory := directories[index]
		destination := filesRoot
		if directory.Path != "." {
			destination = filepath.Join(filesRoot, filepath.FromSlash(directory.Path))
		}
		if err := applyFileMetadata(destination, fs.FileMode(directory.Mode), directory.UID, directory.GID, directory.XAttrs); err != nil {
			return result, fmt.Errorf("snapshot directory metadata %s: %w", directory.Path, err)
		}
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return result, err
	}
	encoded = append(encoded, '\n')
	manifestPath := filepath.Join(temporary, "snapshot.json")
	if err := writeFileDurable(manifestPath, encoded, 0o640); err != nil {
		return result, err
	}
	if err := syncDirectory(temporary); err != nil {
		return result, fmt.Errorf("sync snapshot staging directory: %w", err)
	}
	if err := os.Rename(temporary, final); err != nil {
		return result, err
	}
	if err := syncDirectory(snapshots); err != nil {
		return result, fmt.Errorf("sync snapshot directory: %w", err)
	}
	if err := retainNewest(snapshots, 2); err != nil {
		return result, err
	}
	return SnapshotResult{Files: len(files), Schema: snapshotSchema, Snapshot: final}, nil
}

// CreatePurgeBackup creates and retains one verified sibling snapshot before
// destructive purge. Keeping it outside the owned data root prevents the purge
// itself from deleting its recovery material.
func CreatePurgeBackup(dataDir string) (string, error) {
	root, err := canonicalDataDir(dataDir)
	if err != nil {
		return "", err
	}
	if err := verifyPurgeOwnership(root); err != nil {
		return "", err
	}
	backup := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-purge-backup")
	if backupInfo, err := os.Lstat(backup); err == nil {
		if !backupInfo.IsDir() || backupInfo.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("purge backup path exists but is not a real directory")
		}
		manifest, manifestErr := readManifest(backup)
		if manifestErr != nil || manifest.Transaction != "purge-backup" || manifest.SourceVersion != buildinfo.Version {
			return "", errors.New("existing purge backup is invalid or belongs to another version")
		}
		if err := verifySnapshotFiles(backup, manifest); err != nil {
			return "", fmt.Errorf("existing purge backup is invalid: %w", err)
		}
		current, inventoryErr := Inventory(root)
		if inventoryErr != nil {
			return "", inventoryErr
		}
		if len(current) != len(manifest.Files) {
			return "", errors.New("existing purge backup does not match current application state")
		}
		for index, file := range current {
			stored := manifest.Files[index]
			if file.Path != stored.Path || file.SHA256 != stored.SHA256 || file.Size != stored.Size ||
				uint32(file.Mode.Perm()) != stored.Mode || file.UID != stored.UID || file.GID != stored.GID ||
				!maps.Equal(file.XAttrs, stored.XAttrs) {
				return "", errors.New("existing purge backup does not match current application state")
			}
		}
		currentDirectories, inventoryErr := inventoryDirectories(root)
		if inventoryErr != nil {
			return "", inventoryErr
		}
		if len(currentDirectories) != len(manifest.Directories) {
			return "", errors.New("existing purge backup does not match current application state")
		}
		for index, directory := range currentDirectories {
			stored := manifest.Directories[index]
			if directory.Path != stored.Path || directory.Mode != stored.Mode || directory.UID != stored.UID ||
				directory.GID != stored.GID || !maps.Equal(directory.XAttrs, stored.XAttrs) {
				return "", errors.New("existing purge backup does not match current application state")
			}
		}
		return backup, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	result, err := Snapshot(root, "purge-backup", buildinfo.Version)
	if err != nil {
		return "", err
	}
	if err := os.Rename(result.Snapshot, backup); err != nil {
		return "", fmt.Errorf("retain purge backup outside application state: %w", err)
	}
	if err := syncDirectory(filepath.Dir(root)); err != nil {
		return "", fmt.Errorf("sync purge backup directory: %w", err)
	}
	return backup, nil
}

func verifyPurgeOwnership(root string) error {
	isSQLite, inspectErr := sqliteDatabase(filepath.Join(root, "nilo.db"))
	if inspectErr != nil || !isSQLite {
		return errors.New("purge requires a verified NILO-owned data directory")
	}
	if _, inspectErr = inspectDatabase(filepath.Join(root, "nilo.db")); inspectErr != nil {
		return fmt.Errorf("purge requires a verified NILO-owned data directory: %w", inspectErr)
	}
	return verifyOwnership(root)
}

func readManifest(snapshot string) (snapshotManifest, error) {
	var manifest snapshotManifest
	encoded, err := os.ReadFile(filepath.Join(snapshot, "snapshot.json"))
	if err != nil {
		return manifest, fmt.Errorf("read snapshot manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode snapshot manifest: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return manifest, errors.New("snapshot manifest contains trailing data")
	}
	if manifest.Schema != snapshotSchema || !transactionName.MatchString(manifest.Transaction) {
		return manifest, errors.New("snapshot manifest identity is invalid")
	}
	if manifest.SourceVersion == "" || manifest.TargetVersion == "" {
		return manifest, errors.New("snapshot manifest version binding is invalid")
	}
	seen := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path)))
		if file.Path == "" || file.Path != clean || filepath.IsAbs(file.Path) || strings.Contains(file.Path, "\\") || strings.HasPrefix(file.Path, "../") || file.Mode > 0o777 {
			return manifest, errors.New("snapshot manifest contains an unsafe file declaration")
		}
		if _, exists := seen[file.Path]; exists {
			return manifest, errors.New("snapshot manifest contains a duplicate file declaration")
		}
		if file.UID < -1 || file.GID < -1 || file.XAttrs == nil {
			return manifest, errors.New("snapshot manifest contains invalid ownership metadata")
		}
		for name, encoded := range file.XAttrs {
			if (!strings.HasPrefix(name, "user.") && name != "system.posix_acl_access") || len(name) > 255 {
				return manifest, errors.New("snapshot manifest contains an unsafe extended attribute")
			}
			if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
				return manifest, errors.New("snapshot manifest contains an invalid extended attribute")
			}
		}
		seen[file.Path] = struct{}{}
	}
	directorySeen := make(map[string]struct{}, len(manifest.Directories))
	for _, directory := range manifest.Directories {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(directory.Path)))
		if directory.Path == "" || directory.Path != clean || filepath.IsAbs(directory.Path) ||
			strings.Contains(directory.Path, "\\") || strings.HasPrefix(directory.Path, "../") ||
			directory.Mode > 0o777 || directory.UID < -1 || directory.GID < -1 || directory.XAttrs == nil {
			return manifest, errors.New("snapshot manifest contains an unsafe directory declaration")
		}
		if _, exists := directorySeen[directory.Path]; exists {
			return manifest, errors.New("snapshot manifest contains a duplicate directory declaration")
		}
		for name, encoded := range directory.XAttrs {
			if (!strings.HasPrefix(name, "user.") && name != "system.posix_acl_access" && name != "system.posix_acl_default") || len(name) > 255 {
				return manifest, errors.New("snapshot manifest contains an unsafe extended attribute")
			}
			if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
				return manifest, errors.New("snapshot manifest contains an invalid extended attribute")
			}
		}
		directorySeen[directory.Path] = struct{}{}
	}
	if _, exists := directorySeen["."]; !exists {
		return manifest, errors.New("snapshot manifest lacks root directory metadata")
	}
	return manifest, nil
}

func verifySnapshotFiles(snapshot string, manifest snapshotManifest) error {
	if manifest.DatabaseSchema != "" {
		snapshotDatabase := filepath.Join(snapshot, "files", "nilo.db")
		temporary, createErr := os.CreateTemp("", "nilo-restore-database-*.db")
		if createErr != nil {
			return createErr
		}
		temporaryPath := temporary.Name()
		if closeErr := temporary.Close(); closeErr != nil {
			_ = os.Remove(temporaryPath)
			return closeErr
		}
		_ = os.Remove(temporaryPath)
		defer os.Remove(temporaryPath)
		if err := copyFile(snapshotDatabase, temporaryPath, 0o600); err != nil {
			return fmt.Errorf("prepare snapshot database verification: %w", err)
		}
		schema, err := inspectDatabase(temporaryPath)
		if err != nil {
			return fmt.Errorf("snapshot database schema mismatch: %w", err)
		}
		if schema != manifest.DatabaseSchema {
			return fmt.Errorf("snapshot database schema mismatch: got %q, want %q", schema, manifest.DatabaseSchema)
		}
	}
	for _, file := range manifest.Files {
		path := filepath.Join(snapshot, "files", filepath.FromSlash(file.Path))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != file.Size {
			return fmt.Errorf("snapshot file is invalid: %s", file.Path)
		}
		digest, err := hashFile(path)
		if err != nil || digest != file.SHA256 {
			return fmt.Errorf("snapshot digest mismatch: %s", file.Path)
		}
		expected := File{GID: file.GID, Mode: fs.FileMode(file.Mode), UID: file.UID, XAttrs: file.XAttrs}
		if err := verifyFileMetadata(path, expected); err != nil {
			return fmt.Errorf("snapshot metadata mismatch: %s", file.Path)
		}
	}
	for _, directory := range manifest.Directories {
		path := filepath.Join(snapshot, "files")
		if directory.Path != "." {
			path = filepath.Join(path, filepath.FromSlash(directory.Path))
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot directory is invalid: %s", directory.Path)
		}
		expected := File{GID: directory.GID, Mode: fs.FileMode(directory.Mode), UID: directory.UID, XAttrs: directory.XAttrs}
		if err := verifyFileMetadata(path, expected); err != nil {
			return fmt.Errorf("snapshot directory metadata mismatch: %s", directory.Path)
		}
	}
	return nil
}

// Restore verifies every snapshot byte before atomically replacing current
// application files.
func Restore(dataDir, snapshot, failedTargetVersion string) (err error) {
	root, err := canonicalDataDir(dataDir)
	if err != nil {
		return err
	}
	snapshot, err = filepath.Abs(snapshot)
	if err != nil {
		return err
	}
	snapshotRoot := filepath.Join(root, "snapshots")
	relative, err := filepath.Rel(snapshotRoot, snapshot)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return errors.New("snapshot must be below the data directory snapshot root")
	}
	manifest, err := readManifest(snapshot)
	if err != nil {
		return err
	}
	if manifest.SourceVersion != buildinfo.Version {
		return fmt.Errorf("snapshot source version %s does not match running version %s", manifest.SourceVersion, buildinfo.Version)
	}
	if failedTargetVersion == "" || manifest.TargetVersion != failedTargetVersion {
		return fmt.Errorf("snapshot target version %s does not match failed target version %s", manifest.TargetVersion, failedTargetVersion)
	}
	if err := verifySnapshotFiles(snapshot, manifest); err != nil {
		return err
	}
	release, err := acquire(root)
	if err != nil {
		return err
	}
	released := false
	defer func() {
		if !released {
			err = errors.Join(err, release())
		}
	}()
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	stage := filepath.Join(filepath.Dir(root), "."+filepath.Base(root)+".restore-"+manifest.Transaction)
	if _, err := os.Lstat(stage); err == nil {
		return errors.New("restore staging directory already exists")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Mkdir(stage, rootInfo.Mode().Perm()); err != nil {
		return err
	}
	swapped := false
	defer func() {
		if !swapped {
			_ = os.RemoveAll(stage)
		}
	}()
	for _, directory := range manifest.Directories {
		if directory.Path == "." {
			continue
		}
		if err := os.MkdirAll(filepath.Join(stage, filepath.FromSlash(directory.Path)), fs.FileMode(directory.Mode)); err != nil {
			return err
		}
	}
	for _, file := range manifest.Files {
		source := filepath.Join(snapshot, "files", filepath.FromSlash(file.Path))
		destination := filepath.Join(stage, filepath.FromSlash(file.Path))
		metadata := File{GID: file.GID, Mode: fs.FileMode(file.Mode), UID: file.UID, XAttrs: file.XAttrs}
		if err := copyFileWithMetadata(source, destination, metadata); err != nil {
			return err
		}
	}
	for index := len(manifest.Directories) - 1; index >= 0; index-- {
		directory := manifest.Directories[index]
		destination := stage
		if directory.Path != "." {
			destination = filepath.Join(stage, filepath.FromSlash(directory.Path))
		}
		if err := applyFileMetadata(destination, fs.FileMode(directory.Mode), directory.UID, directory.GID, directory.XAttrs); err != nil {
			return err
		}
	}
	for _, name := range []string{"snapshots", "transactions"} {
		source := filepath.Join(root, name)
		if _, statErr := os.Lstat(source); errors.Is(statErr, fs.ErrNotExist) {
			continue
		} else if statErr != nil {
			return statErr
		}
		if err := copyDirectoryTree(source, filepath.Join(stage, name)); err != nil {
			return err
		}
	}
	if err := writeFileDurable(filepath.Join(stage, ".maintenance.lock"), []byte("restore\n"), 0o600); err != nil {
		return err
	}
	if err := syncTree(stage); err != nil {
		return err
	}
	var releaseErr error
	releasedBeforeExchange := releaseBeforeDirectoryExchange()
	var releaseExclusion func() error
	if releasedBeforeExchange {
		releaseExclusion, err = acquirePortableExchangeExclusion(root)
		if err != nil {
			return err
		}
		defer func() {
			if releaseExclusion != nil {
				err = errors.Join(err, releaseExclusion())
			}
		}()
		releaseErr = release()
		released = true
		if releaseErr != nil {
			return releaseErr
		}
	}
	if err := exchangeDirectories(root, stage); err != nil {
		return fmt.Errorf("atomically exchange restored application state: %w", err)
	}
	swapped = true
	if releasedBeforeExchange {
		releaseErr = errors.Join(
			os.Remove(filepath.Join(root, ".maintenance.lock")),
			syncDirectory(root),
		)
	} else {
		releaseErr = release()
		released = true
	}
	cleanupErr := os.RemoveAll(stage)
	syncErr := syncDirectory(filepath.Dir(root))
	return errors.Join(releaseErr, cleanupErr, syncErr)
}

func retainNewest(root string, keep int) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type candidate struct {
		name string
		when int64
	}
	candidates := make([]candidate, 0)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		candidates = append(candidates, candidate{name: entry.Name(), when: info.ModTime().UnixNano()})
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].when == candidates[right].when {
			return candidates[left].name > candidates[right].name
		}
		return candidates[left].when > candidates[right].when
	})
	if len(candidates) <= keep {
		return nil
	}
	for _, stale := range candidates[keep:] {
		if err := os.RemoveAll(filepath.Join(root, stale.name)); err != nil {
			return err
		}
	}
	return nil
}

// Purge removes only contents of the explicit application data directory and
// leaves the selected directory itself in place.
func Purge(dataDir string) (err error) {
	root, err := canonicalDataDir(dataDir)
	if err != nil {
		return err
	}
	if err := verifyPurgeOwnership(root); err != nil {
		return err
	}
	release, err := acquire(root)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		_ = release()
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".maintenance.lock" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			_ = release()
			return err
		}
	}
	return release()
}
