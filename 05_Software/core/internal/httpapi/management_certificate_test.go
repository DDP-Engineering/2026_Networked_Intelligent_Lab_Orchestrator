package httpapi

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagementCertificateNamesRequireDNSAndVPNAddressSANs(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "NILO test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"ddp-remote.local"}, IPAddresses: []net.IP{net.ParseIP("10.205.148.11")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "listener.crt")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	missing, err := ValidateCertificateNames(path, []string{"ddp-remote.local", "10.205.148.11"})
	if err != nil || len(missing) != 0 {
		t.Fatalf("valid names missing=%v error=%v", missing, err)
	}
	missing, err = ValidateCertificateNames(path, []string{"other.local", "10.205.148.12"})
	if err != nil || len(missing) != 2 {
		t.Fatalf("wrong names missing=%v error=%v", missing, err)
	}
}
