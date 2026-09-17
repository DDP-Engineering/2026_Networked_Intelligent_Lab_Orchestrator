package nethelper

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

type tlsSignerHelper struct {
	manager *TLSIdentityManager
}

func (helper tlsSignerHelper) TLSIdentity(context.Context) (TLSIdentityPublic, error) {
	return helper.manager.PublicIdentity()
}

func (helper tlsSignerHelper) SignTLS(_ context.Context, generation string, digest []byte) ([]byte, error) {
	return helper.manager.Sign(generation, digest)
}

func TestRemoteTLSSignerBuildsUsableCertificateWithoutPrivateKeyExport(t *testing.T) {
	directory := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	manager, err := NewTLSIdentityManager(directory, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ActivateNames(context.Background(), []string{"nilo-p2rjmzdkf1"}); err != nil {
		t.Fatal(err)
	}
	certificate, err := NewRemoteTLSCertificate(context.Background(), tlsSignerHelper{manager: manager})
	if err != nil {
		t.Fatal(err)
	}
	if len(certificate.Certificate) != 1 {
		t.Fatalf("certificate chain length = %d", len(certificate.Certificate))
	}
	signer, ok := certificate.PrivateKey.(crypto.Signer)
	if !ok {
		t.Fatalf("private key adapter = %T, want crypto.Signer", certificate.PrivateKey)
	}
	if _, local := certificate.PrivateKey.(*ecdsa.PrivateKey); local {
		t.Fatal("Bench Core received the helper private key")
	}
	digest := sha256.Sum256([]byte("remote TLS handshake"))
	signature, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	public := parsed.PublicKey.(*ecdsa.PublicKey)
	if !ecdsa.VerifyASN1(public, digest[:], signature) {
		t.Fatal("remote signature did not verify")
	}
	if _, err := signer.Sign(rand.Reader, digest[:], crypto.SHA512); err == nil {
		t.Fatal("remote signer accepted a non-SHA-256 algorithm")
	}
}

type blockingTLSHelper struct {
	started chan struct{}
	release chan struct{}
}

func (helper *blockingTLSHelper) TLSIdentity(context.Context) (TLSIdentityPublic, error) {
	return TLSIdentityPublic{}, errors.New("not used")
}

func (helper *blockingTLSHelper) SignTLS(ctx context.Context, _ string, _ []byte) ([]byte, error) {
	helper.started <- struct{}{}
	select {
	case <-helper.release:
		return []byte("signature"), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestRemoteTLSSignerBoundsConcurrencyAndHelperDeadline(t *testing.T) {
	digest := sha256.Sum256([]byte("bounded handshake"))
	helper := &blockingTLSHelper{started: make(chan struct{}, 64), release: make(chan struct{})}
	signer := &remoteTLSSigner{
		helper: helper, generation: "generation", limit: make(chan struct{}, 32), timeout: time.Second,
	}
	results := make(chan error, 32)
	for index := 0; index < 32; index++ {
		go func() {
			_, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
			results <- err
		}()
	}
	for index := 0; index < 32; index++ {
		select {
		case <-helper.started:
		case <-time.After(time.Second):
			t.Fatal("signature request did not reach the helper")
		}
	}
	started := time.Now()
	if _, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("saturated signer error = %v", err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("saturated signer waited instead of failing one handshake")
	}
	close(helper.release)
	for index := 0; index < 32; index++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}

	timeoutHelper := &blockingTLSHelper{started: make(chan struct{}, 1), release: make(chan struct{})}
	timeoutSigner := &remoteTLSSigner{
		helper: timeoutHelper, generation: "generation", limit: make(chan struct{}, 32), timeout: 10 * time.Millisecond,
	}
	if _, err := timeoutSigner.Sign(rand.Reader, digest[:], crypto.SHA256); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error = %v", err)
	}
}
