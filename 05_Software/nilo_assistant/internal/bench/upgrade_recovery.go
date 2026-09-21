package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const upgradeJournalSchema = 1

type upgradeJournal struct {
	Backup          string `json:"backup"`
	CandidateDigest string `json:"candidate_digest"`
	ID              string `json:"id"`
	Phase           string `json:"phase"`
	PriorDigest     string `json:"prior_digest"`
	Schema          int    `json:"schema"`
	Stage           string `json:"stage"`
	Target          string `json:"target"`
}

func upgradeJournalPath(root, id string) string {
	return filepath.Join(root, ".upgrade-"+id+".json")
}

func validateUpgradeJournal(journal upgradeJournal) error {
	validPhase := journal.Phase == "prepared" || journal.Phase == "backup_moved" || journal.Phase == "candidate_active" || journal.Phase == "state_committed"
	prefix := ".upgrade-" + journal.ID + "-"
	if journal.Schema != upgradeJournalSchema || !packageIDPattern.MatchString(journal.ID) || journal.Target != journal.ID ||
		!validPhase || journal.PriorDigest == "" || journal.CandidateDigest == "" ||
		filepath.Base(journal.Stage) != journal.Stage || filepath.Base(journal.Backup) != journal.Backup ||
		!strings.HasPrefix(journal.Stage, prefix) || !strings.HasPrefix(journal.Backup, prefix) || journal.Stage == journal.Backup {
		return errors.New("integration upgrade journal is invalid")
	}
	return nil
}

func writeUpgradeJournal(root string, journal upgradeJournal) error {
	if err := validateUpgradeJournal(journal); err != nil {
		return err
	}
	encoded, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	path := upgradeJournalPath(root, journal.ID)
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		if removeErr := os.Remove(temporary); removeErr != nil {
			return removeErr
		}
		file, err = os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(encoded)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return syncPackageDirectory(root)
}

func readUpgradeJournal(path string) (upgradeJournal, error) {
	var journal upgradeJournal
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return journal, errors.New("integration upgrade journal is not a regular file")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return journal, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return journal, errors.New("integration upgrade journal is invalid")
	}
	return journal, validateUpgradeJournal(journal)
}

func removeUpgradeJournal(root, id string) error {
	if err := os.Remove(upgradeJournalPath(root, id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncPackageDirectory(root)
}

func syncPackageTree(root string) error {
	directories := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		return syncPackageFile(path)
	})
	if err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncPackageDirectory(directories[index]); err != nil {
			return err
		}
	}
	return nil
}

func packageDigestIfPresent(path string) (string, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", false, errors.Join(errors.New("integration package path is invalid"), err)
	}
	digest, err := verifyPackage(path)
	return digest, true, err
}

func recoverPackageUpgrades(options Options) error {
	root := filepath.Join(options.DataDir, "integrations")
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".upgrade-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		journal, err := readUpgradeJournal(path)
		if err != nil || path != upgradeJournalPath(root, journal.ID) {
			return errors.Join(errors.New("cannot reconcile integration upgrade"), err)
		}
		target := filepath.Join(root, journal.Target)
		stage := filepath.Join(root, journal.Stage)
		backup := filepath.Join(root, journal.Backup)
		previous := filepath.Join(backup, "previous")
		var databaseDigest string
		if err := options.Database.SQL().QueryRowContext(context.Background(), `SELECT digest FROM bench_plugins WHERE id=?`, journal.ID).Scan(&databaseDigest); err != nil {
			return fmt.Errorf("read integration upgrade state: %w", err)
		}
		targetDigest, targetPresent, err := packageDigestIfPresent(target)
		if err != nil {
			return err
		}
		previousDigest, previousPresent, err := packageDigestIfPresent(previous)
		if err != nil {
			return err
		}
		stageDigest, stagePresent, err := packageDigestIfPresent(stage)
		if err != nil {
			return err
		}
		switch databaseDigest {
		case journal.PriorDigest:
			switch {
			case targetPresent && targetDigest == journal.PriorDigest:
			case targetPresent && targetDigest == journal.CandidateDigest && previousPresent && previousDigest == journal.PriorDigest:
				if err := rollbackPackageSwitch(target, backup); err != nil {
					return err
				}
			case !targetPresent && previousPresent && previousDigest == journal.PriorDigest:
				if err := os.Rename(previous, target); err != nil {
					return err
				}
				if err := syncPackageDirectory(root); err != nil {
					return err
				}
			default:
				return errors.New("interrupted integration upgrade cannot restore the database-authoritative package")
			}
		case journal.CandidateDigest:
			switch {
			case targetPresent && targetDigest == journal.CandidateDigest:
			case !targetPresent && stagePresent && stageDigest == journal.CandidateDigest:
				if err := os.Rename(stage, target); err != nil {
					return err
				}
				if err := syncPackageDirectory(root); err != nil {
					return err
				}
			default:
				return errors.New("interrupted integration upgrade cannot activate the database-authoritative package")
			}
		default:
			return errors.New("integration upgrade journal disagrees with database state")
		}
		if digest, verifyErr := verifyPackage(target); verifyErr != nil || digest != databaseDigest {
			return errors.Join(errors.New("reconciled integration package does not match database state"), verifyErr)
		}
		if err := errors.Join(os.RemoveAll(stage), os.RemoveAll(backup)); err != nil {
			return err
		}
		if err := removeUpgradeJournal(root, journal.ID); err != nil {
			return err
		}
	}
	return nil
}
