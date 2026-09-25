package nethelper

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestTLSIdentityKeepsPrivateKeyAndSignsOnlyCurrentDigest(t *testing.T) {
	directory := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	manager, err := NewTLSIdentityManager(directory, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ActivateNames(context.Background(), []string{"nilo-p2rjmzdkf1", "nilo"}); err != nil {
		t.Fatal(err)
	}
	public, err := manager.PublicIdentity()
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(public.CertificateDER)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(certificate.DNSNames, []string{"nilo.local", "nilo-p2rjmzdkf1.local"}) {
		t.Fatalf("certificate DNS names = %v", certificate.DNSNames)
	}
	if certificate.NotBefore != now.Add(-24*time.Hour) || certificate.NotAfter != now.AddDate(10, 0, 0) {
		t.Fatalf("certificate validity = %s - %s", certificate.NotBefore, certificate.NotAfter)
	}
	digest := sha256.Sum256([]byte("handshake transcript"))
	signature, err := manager.Sign(public.Generation, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	key, ok := certificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || !ecdsa.VerifyASN1(key, digest[:], signature) {
		t.Fatal("helper signature did not verify against public certificate")
	}
	if _, err := manager.Sign(public.Generation, digest[:16]); err == nil {
		t.Fatal("helper signed a non-SHA-256-sized digest")
	}
	if _, err := manager.Sign("stale", digest[:]); err == nil {
		t.Fatal("helper signed through an unknown generation")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(directory, tlsPrivateKeyFile))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("private key mode = %v, %v", info, err)
		}
	}
}

func TestTLSIdentityReloadsCurrentGeneration(t *testing.T) {
	directory := t.TempDir()
	if runtime.GOOS != "windows" {
		_ = os.Chmod(directory, 0700)
	}
	now := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	first, err := NewTLSIdentityManager(directory, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := first.ActivateNames(context.Background(), []string{"nilo-p2rjmzdkf1"}); err != nil {
		t.Fatal(err)
	}
	before, _ := first.PublicIdentity()
	restarted, err := NewTLSIdentityManager(directory, func() time.Time { return now.Add(time.Hour) })
	if err != nil {
		t.Fatal(err)
	}
	after, err := restarted.PublicIdentity()
	if err != nil || before.Generation != after.Generation || !slices.Equal(before.CertificateDER, after.CertificateDER) {
		t.Fatalf("reloaded public identity = %+v, %v; want generation %s", after, err, before.Generation)
	}
}

func TestTLSIdentityLeasesPriorGenerationForFiveMinutes(t *testing.T) {
	directory := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	manager, err := NewTLSIdentityManager(directory, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ActivateNames(context.Background(), []string{"nilo-p2rjmzdkf1"}); err != nil {
		t.Fatal(err)
	}
	old, _ := manager.PublicIdentity()
	if err := manager.ActivateNames(context.Background(), []string{"nilo-p2rjmzdkf1", "bench-west"}); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("in-flight handshake"))
	if _, err := manager.Sign(old.Generation, digest[:]); err != nil {
		t.Fatalf("prior generation was not leased: %v", err)
	}
	now = now.Add(5*time.Minute + time.Nanosecond)
	if _, err := manager.Sign(old.Generation, digest[:]); err == nil {
		t.Fatal("expired prior generation still signed")
	}
}

func TestTLSIdentityProtocolExposesCertificateAndConstrainedSignature(t *testing.T) {
	directory := t.TempDir()
	if runtime.GOOS != "windows" {
		_ = os.Chmod(directory, 0700)
	}
	manager, err := NewTLSIdentityManager(directory, func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ActivateNames(context.Background(), []string{"nilo-p2rjmzdkf1"}); err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(&fakeDriver{}, &memoryStore{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ConfigureTLSIdentity(manager); err != nil {
		t.Fatal(err)
	}
	read := dispatch(context.Background(), engine, Request{Op: "tls.identity.read"})
	if read.Error != "" || read.TLSIdentity == nil || len(read.TLSIdentity.CertificateDER) == 0 {
		t.Fatalf("TLS identity read = %+v", read)
	}
	digest := sha256.Sum256([]byte("protocol handshake"))
	signed := dispatch(context.Background(), engine, Request{
		Op: "tls.identity.sign", TLSGeneration: read.TLSIdentity.Generation,
		TLSDigest: digest[:], TLSHash: "sha256", TLSAlgorithm: "ecdsa-p256-sha256",
	})
	if signed.Error != "" || len(signed.TLSSignature) == 0 {
		t.Fatalf("TLS identity sign = %+v", signed)
	}
	refused := dispatch(context.Background(), engine, Request{
		Op: "tls.identity.sign", TLSGeneration: read.TLSIdentity.Generation,
		TLSDigest: digest[:], TLSHash: "sha512", TLSAlgorithm: "ecdsa-p256-sha256",
	})
	if refused.Code != "invalid_request" || len(refused.TLSSignature) != 0 {
		t.Fatalf("unsupported sign request = %+v", refused)
	}
	encoded, err := json.Marshal(read)
	if err != nil || bytes.Contains(encoded, []byte("PRIVATE KEY")) {
		t.Fatalf("public response leaked private material: %s, %v", encoded, err)
	}
}
