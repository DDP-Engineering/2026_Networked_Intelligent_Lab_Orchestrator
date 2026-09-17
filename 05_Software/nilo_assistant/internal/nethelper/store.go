package nethelper

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// FileStore uses a private helper-owned directory supplied by local startup,
// never by RPC. Atomic rename plus file and directory sync precede mutations.
type FileStore struct{ directory string }

func NewFileStore(directory string) (*FileStore, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("recovery directory must be absolute")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("recovery directory must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("recovery directory must have mode 0700")
	}
	return &FileStore{directory: directory}, nil
}
func (s *FileStore) Load() (*Record, error) {
	var r Record
	found, err := s.load("recovery.json", &r)
	if !found || err != nil {
		return nil, err
	}
	return &r, nil
}
func (s *FileStore) LoadInstall() (*InstallRecord, error) {
	var r InstallRecord
	found, err := s.load("install.json", &r)
	if !found || err != nil {
		return nil, err
	}
	return &r, nil
}
func (s *FileStore) load(name string, into any) (bool, error) {
	path := filepath.Join(s.directory, name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return false, errors.New("helper state file must be private and regular")
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 256<<10))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(into); err != nil {
		return false, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return false, errors.New("trailing helper state data")
	}
	return true, nil
}
func (s *FileStore) Save(r Record) error {
	return s.save("recovery.json", r)
}
func (s *FileStore) SaveInstall(r InstallRecord) error {
	return s.save("install.json", r)
}

func (s *FileStore) LoadIdentifyActions() (IdentifyActionLedger, error) {
	var ledger IdentifyActionLedger
	found, err := s.load("identify-actions.json", &ledger)
	if !found || err != nil {
		return ledger, err
	}
	return ledger, validateIdentifyActionLedger(ledger)
}

func (s *FileStore) SaveIdentifyActions(ledger IdentifyActionLedger) error {
	if err := validateIdentifyActionLedger(ledger); err != nil {
		return err
	}
	return s.save("identify-actions.json", ledger)
}

func (s *FileStore) loadApplianceIdentity() (applianceIdentityRecord, bool, error) {
	var record applianceIdentityRecord
	found, err := s.load(applianceIdentityFileName, &record)
	return record, found, err
}

func (s *FileStore) saveApplianceIdentity(record applianceIdentityRecord) error {
	if err := validateApplianceIdentityRecord(record); err != nil {
		return err
	}
	return s.save(applianceIdentityFileName, record)
}

func (s *FileStore) LoadDirectRecovery() (*DirectRecoveryRecord, error) {
	var record DirectRecoveryRecord
	found, err := s.load("direct-recovery.json", &record)
	if !found || err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *FileStore) SaveDirectRecovery(record DirectRecoveryRecord) error {
	return s.save("direct-recovery.json", record)
}

func (s *FileStore) ClearDirectRecovery() error {
	path := filepath.Join(s.directory, "direct-recovery.json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(s.directory)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (s *FileStore) LoadHardwareRecoveryEvent() (HardwareRecoveryEventRecord, error) {
	var record HardwareRecoveryEventRecord
	_, err := s.load("hardware-recovery-event.json", &record)
	return record, err
}

func (s *FileStore) SaveHardwareRecoveryEvent(record HardwareRecoveryEventRecord) error {
	return s.save("hardware-recovery-event.json", record)
}

func (s *FileStore) LoadCommissioningProfiles() (CommissioningProfileOwnership, error) {
	record := CommissioningProfileOwnership{Version: 1, Profiles: []CommissioningProfileRecord{}}
	found, err := s.load("commissioning-profiles.json", &record)
	if err != nil {
		return CommissioningProfileOwnership{}, err
	}
	if !found {
		return record, nil
	}
	if !validCommissioningProfileOwnership(record) {
		return CommissioningProfileOwnership{}, errors.New("invalid commissioning profile ownership record")
	}
	return record, nil
}

func (s *FileStore) SaveCommissioningProfiles(record CommissioningProfileOwnership) error {
	if !validCommissioningProfileOwnership(record) {
		return errors.New("invalid commissioning profile ownership record")
	}
	return s.save("commissioning-profiles.json", record)
}

// LoadForwarding reads the device-network inventory, absent when nothing was
// ever installed.
func (s *FileStore) LoadForwarding() (*ForwardingInventory, error) {
	var inventory ForwardingInventory
	found, err := s.load("forwarding.json", &inventory)
	if !found || err != nil {
		return nil, err
	}
	return &inventory, nil
}

// SaveForwarding writes the device-network inventory before the host changes.
func (s *FileStore) SaveForwarding(inventory ForwardingInventory) error {
	return s.save("forwarding.json", inventory)
}
func (s *FileStore) save(target string, r any) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if len(data) > 256<<10 {
		return errors.New("recovery state exceeds bound")
	}
	f, err := os.CreateTemp(s.directory, ".recovery-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(s.directory, target)); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(s.directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
