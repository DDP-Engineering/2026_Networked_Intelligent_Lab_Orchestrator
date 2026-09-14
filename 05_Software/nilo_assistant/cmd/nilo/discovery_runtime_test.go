package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// writeCertificate produces a self-signed leaf carrying exactly names, which is
// what a shipped appliance certificate is: signed by nobody a browser trusts.
func writeCertificate(t *testing.T, names ...string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "nilo test appliance"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     names,
	}
	encoded, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	path := filepath.Join(t.TempDir(), "appliance.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: encoded}), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	return path
}

// REQ-PLAT-3c35bacd: the appliance reports every first-contact name its
// certificate does not carry, and keeps serving, so a misprepared image is a
// visible defect rather than an appliance nobody can reach.
func TestMissingDiscoveryNamesReportsEveryAdvertisedNameTheCertificateLacks(t *testing.T) {
	t.Parallel()

	const host = "bench-west"

	for _, testCase := range []struct {
		name        string
		certificate []string
		wantMissing string
	}{
		{name: "both names present", certificate: []string{"bench-west.local", "nilo.local"}},
		{name: "only the host name", certificate: []string{"bench-west.local"}, wantMissing: "nilo.local"},
		{name: "only the recovery alias", certificate: []string{"nilo.local"}, wantMissing: "bench-west.local"},
		{name: "neither name", certificate: []string{"example.invalid"}, wantMissing: "bench-west.local"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path := writeCertificate(t, testCase.certificate...)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read certificate: %v", err)
			}

			missing, err := missingDiscoveryNames(path, host, true)
			if err != nil {
				t.Fatalf("missingDiscoveryNames: %v", err)
			}
			if testCase.wantMissing == "" {
				if len(missing) != 0 {
					t.Fatalf("missingDiscoveryNames with both names = %v, want none", missing)
				}
			} else {
				if !slices.Contains(missing, testCase.wantMissing) {
					t.Fatalf("missingDiscoveryNames = %v, want it to report %q", missing, testCase.wantMissing)
				}
			}

			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("re-read certificate: %v", err)
			}
			if string(before) != string(after) {
				t.Fatal("checking the certificate modified it; the appliance never generates or replaces one")
			}
		})
	}
}

// REQ-PLAT-3c35bacd: a certificate that cannot be read or parsed is an error,
// never a silently complete one. There is nothing to serve TLS from at all,
// which is a different problem from a certificate that is merely incomplete.
func TestMissingDiscoveryNamesErrorsOnAnUnreadableOrMalformedCertificate(t *testing.T) {
	t.Parallel()

	if _, err := missingDiscoveryNames(filepath.Join(t.TempDir(), "absent.pem"), "bench-west", true); err == nil {
		t.Fatal("an absent certificate was accepted")
	}

	malformed := filepath.Join(t.TempDir(), "malformed.pem")
	if err := os.WriteFile(malformed, []byte("not a certificate at all"), 0o600); err != nil {
		t.Fatalf("write malformed certificate: %v", err)
	}
	if _, err := missingDiscoveryNames(malformed, "bench-west", true); err == nil {
		t.Fatal("a malformed certificate was accepted")
	}
}

// REQ-PLAT-3c35bacd: a commissioned appliance without hardware recovery is told
// to visit its own name only, so only that name is required of it.
func TestMissingDiscoveryNamesIgnoresTheRecoveryAliasWhenItIsNotAdvertised(t *testing.T) {
	t.Parallel()

	path := writeCertificate(t, "bench-west.local")
	missing, err := missingDiscoveryNames(path, "bench-west", false)
	if err != nil {
		t.Fatalf("missingDiscoveryNames: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missingDiscoveryNames without the recovery alias = %v, want none", missing)
	}
}

// REQ-SEC-c9e034de: a hardware recovery reset turns a commissioned service back
// into a provisioning one while the process runs, so both start modes need
// everything completing commissioning needs. Wiring one dependency and not the
// other leaves an appliance that can be recovered but not recommissioned until
// it is restarted, which the recovery tests cannot catch because they inject
// their own commissioner.
func TestCommissioningDependenciesSuppliesBothHalvesOfTheCommissioningPath(t *testing.T) {
	application := openApplication(t)

	commissioner, credential := commissioningDependencies(application)
	if commissioner == nil {
		t.Fatal("no commissioner: a recovered appliance could not record itself commissioned again")
	}
	if credential == nil {
		t.Fatal("no published-credential checker: commissioning would refuse as unable to check")
	}
	remains, err := credential.PublishedVerifierRemains(context.Background())
	if err != nil {
		t.Fatalf("PublishedVerifierRemains over the real identity service: %v", err)
	}
	if remains {
		t.Fatal("a fresh data directory holds no identity, so the published verifier cannot be present")
	}
}

// REQ-PLAT-3c35bacd: every listener mode reports a certificate name gap rather
// than refusing to start. What differs is where it is reported: the first-boot
// and interface-managed listeners carry it into the interface through
// TLSMissing, while the operator-configured legacy bound listener has no
// management-access interface and reports it on its diagnostic output alone.
// The decision that produces the report is the same one in all three.
func TestMissingDiscoveryNamesAnswersEveryListenerModeTheSameWay(t *testing.T) {
	t.Parallel()

	// A legacy bound appliance listener advertises its own host name only.
	path := writeCertificate(t, "nilo.local")
	missing, err := missingDiscoveryNames(path, "bench-west", false)
	if err != nil {
		t.Fatalf("missingDiscoveryNames: %v", err)
	}
	if !slices.Contains(missing, "bench-west.local") {
		t.Fatalf("legacy listener missing names = %v, want it to report bench-west.local", missing)
	}

	// The same certificate under first boot, which advertises both names, is
	// short only the host name: the recovery alias is present.
	missing, err = missingDiscoveryNames(path, "bench-west", true)
	if err != nil {
		t.Fatalf("missingDiscoveryNames: %v", err)
	}
	if !slices.Equal(missing, []string{"bench-west.local"}) {
		t.Fatalf("first-boot missing names = %v, want exactly [bench-west.local]", missing)
	}
}

// REQ-PLAT-3c35bacd: an overlapping name is reported once, not once per list.
func TestMergeMissingNamesReportsAnOverlappingNameOnce(t *testing.T) {
	t.Parallel()

	merged := mergeMissingNames(
		[]string{"bench-west.local", "10.0.0.5"},
		[]string{"bench-west.local", "nilo.local"},
	)
	if !slices.Equal(merged, []string{"bench-west.local", "10.0.0.5", "nilo.local"}) {
		t.Fatalf("merged = %v, want each name once in first-seen order", merged)
	}
}
