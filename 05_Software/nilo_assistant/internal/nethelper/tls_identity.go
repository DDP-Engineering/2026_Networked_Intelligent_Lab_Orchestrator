package nethelper

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	tlsPrivateKeyFile = "tls-private-key.pem"
	tlsIdentityFile   = "tls-identity.json"
)

// TLSIdentityPublic is the only TLS identity data allowed to leave the helper.
type TLSIdentityPublic struct {
	Generation     string    `json:"generation"`
	CertificateDER []byte    `json:"certificate_der"`
	NotAfter       time.Time `json:"not_after"`
}

type tlsIdentityRecord struct {
	Schema         int      `json:"schema"`
	Generation     string   `json:"generation"`
	CertificateDER []byte   `json:"certificate_der"`
	Names          []string `json:"names"`
}

// TLSIdentityManager owns the private key and serializes rotation/signing.
type TLSIdentityManager struct {
	mu        sync.Mutex
	directory string
	now       func() time.Time
	key       *ecdsa.PrivateKey
	record    *tlsIdentityRecord
	cert      *x509.Certificate
	previous  map[string]time.Time
}

// NewTLSIdentityManager opens a private helper-owned directory and restores a
// valid current generation when present.
func NewTLSIdentityManager(directory string, now func() time.Time) (*TLSIdentityManager, error) {
	if !filepath.IsAbs(directory) || now == nil {
		return nil, errors.New("absolute TLS identity directory and clock are required")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("TLS identity directory must be private and real")
	}
	manager := &TLSIdentityManager{directory: directory, now: now, previous: map[string]time.Time{}}
	if err := manager.load(); err != nil {
		return nil, err
	}
	return manager, nil
}

// ActivateNames ensures the current certificate covers exactly the supplied
// validated local labels. The prior files remain authoritative until the new
// certificate has been durably written.
func (manager *TLSIdentityManager) ActivateNames(ctx context.Context, names []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := normalizeTLSNames(names)
	if err != nil {
		return err
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	previousGeneration := ""
	if manager.record != nil {
		previousGeneration = manager.record.Generation
	}
	if manager.record != nil && slices.Equal(manager.record.Names, normalized) && manager.cert.NotAfter.Sub(manager.now().UTC()) >= 180*24*time.Hour {
		return nil
	}
	if manager.key == nil {
		manager.key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return fmt.Errorf("generate TLS private key: %w", err)
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(manager.key)
		if err != nil {
			return err
		}
		if err := atomicPrivateWrite(manager.directory, tlsPrivateKeyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})); err != nil {
			return fmt.Errorf("persist TLS private key: %w", err)
		}
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := manager.now().UTC()
	dnsNames := make([]string, len(normalized))
	for index, name := range normalized {
		dnsNames[index] = name + ".local"
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: dnsNames[0]},
		DNSNames:              dnsNames,
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &manager.key.PublicKey, manager.key)
	if err != nil {
		return fmt.Errorf("create TLS certificate: %w", err)
	}
	digest := sha256.Sum256(der)
	record := tlsIdentityRecord{Schema: 1, Generation: hex.EncodeToString(digest[:16]), CertificateDER: der, Names: normalized}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := atomicPrivateWrite(manager.directory, tlsIdentityFile, data); err != nil {
		return fmt.Errorf("persist TLS certificate: %w", err)
	}
	manager.record = &record
	manager.cert, err = x509.ParseCertificate(der)
	if err == nil && previousGeneration != "" && previousGeneration != record.Generation {
		manager.previous[previousGeneration] = manager.now().UTC().Add(5 * time.Minute)
	}
	return err
}

// PublicIdentity returns a copy of the current public certificate.
func (manager *TLSIdentityManager) PublicIdentity() (TLSIdentityPublic, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.record == nil || manager.cert == nil {
		return TLSIdentityPublic{}, errors.New("TLS identity is not initialized")
	}
	return TLSIdentityPublic{
		Generation: manager.record.Generation, CertificateDER: append([]byte(nil), manager.record.CertificateDER...),
		NotAfter: manager.cert.NotAfter,
	}, nil
}

// Sign signs exactly one SHA-256 digest through the current generation.
func (manager *TLSIdentityManager) Sign(generation string, digest []byte) ([]byte, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.record == nil || manager.key == nil || len(digest) != sha256.Size {
		return nil, errors.New("invalid TLS signing request")
	}
	if generation != manager.record.Generation {
		expiresAt, leased := manager.previous[generation]
		if !leased || !manager.now().UTC().Before(expiresAt) {
			delete(manager.previous, generation)
			return nil, errors.New("invalid TLS signing request")
		}
	}
	return ecdsa.SignASN1(rand.Reader, manager.key, digest)
}

func (manager *TLSIdentityManager) load() error {
	keyData, keyErr := readPrivateFile(manager.directory, tlsPrivateKeyFile)
	recordData, recordErr := readPrivateFile(manager.directory, tlsIdentityFile)
	if errors.Is(keyErr, os.ErrNotExist) && errors.Is(recordErr, os.ErrNotExist) {
		return nil
	}
	if keyErr != nil || recordErr != nil {
		return errors.New("incomplete TLS identity state")
	}
	block, _ := pem.Decode(keyData)
	if block == nil || block.Type != "PRIVATE KEY" {
		return errors.New("invalid TLS private key")
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return errors.New("invalid TLS private key")
	}
	key, ok := parsedKey.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return errors.New("invalid TLS private key")
	}
	var record tlsIdentityRecord
	decoder := json.NewDecoder(io.LimitReader(strings.NewReader(string(recordData)), 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil || decoder.Decode(new(any)) != io.EOF || record.Schema != 1 {
		return errors.New("invalid TLS identity record")
	}
	certificate, err := x509.ParseCertificate(record.CertificateDER)
	if err != nil {
		return errors.New("invalid TLS certificate")
	}
	publicKey, ok := certificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || !publicKey.Equal(&key.PublicKey) ||
		certificate.CheckSignature(certificate.SignatureAlgorithm, certificate.RawTBSCertificate, certificate.Signature) != nil {
		return errors.New("TLS certificate does not match helper key")
	}
	names, err := normalizeTLSNames(record.Names)
	if err != nil || !slices.Equal(names, record.Names) {
		return errors.New("invalid TLS certificate names")
	}
	manager.key, manager.record, manager.cert = key, &record, certificate
	return nil
}

func normalizeTLSNames(names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, errors.New("at least one TLS name is required")
	}
	unique := map[string]bool{}
	result := make([]string, 0, len(names))
	for _, name := range names {
		if !systemHostLabel.MatchString(name) || unique[name] {
			return nil, errors.New("TLS names must be unique lowercase DNS labels")
		}
		unique[name] = true
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func readPrivateFile(directory, name string) ([]byte, error) {
	path := filepath.Join(directory, name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("TLS identity file must be private and regular")
	}
	return os.ReadFile(path)
}

func atomicPrivateWrite(directory, name string, data []byte) error {
	file, err := os.CreateTemp(directory, ".tls-*")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(directory, name)); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer directoryFile.Close()
	return directoryFile.Sync()
}
