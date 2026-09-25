package nethelper

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"time"
)

const remoteTLSSignTimeout = 5 * time.Second

var remoteTLSSignLimit = make(chan struct{}, 32)

// TLSIdentityClient is the narrow helper surface needed by the TLS stack.
type TLSIdentityClient interface {
	TLSIdentity(context.Context) (TLSIdentityPublic, error)
	SignTLS(context.Context, string, []byte) ([]byte, error)
}

type remoteTLSSigner struct {
	helper     TLSIdentityClient
	generation string
	publicKey  *ecdsa.PublicKey
	limit      chan struct{}
	timeout    time.Duration
}

// NewRemoteTLSCertificate constructs a tls.Certificate whose private signing
// operations cross the helper protocol while key material remains privileged.
func NewRemoteTLSCertificate(ctx context.Context, helper TLSIdentityClient) (tls.Certificate, error) {
	if helper == nil {
		return tls.Certificate{}, errors.New("TLS identity helper is required")
	}
	identity, err := helper.TLSIdentity(ctx)
	if err != nil {
		return tls.Certificate{}, err
	}
	certificate, err := x509.ParseCertificate(identity.CertificateDER)
	if err != nil {
		return tls.Certificate{}, errors.New("helper returned an invalid TLS certificate")
	}
	publicKey, ok := certificate.PublicKey.(*ecdsa.PublicKey)
	if !ok || identity.Generation == "" {
		return tls.Certificate{}, errors.New("helper returned an unsupported TLS identity")
	}
	signer := &remoteTLSSigner{
		helper: helper, generation: identity.Generation, publicKey: publicKey,
		limit: remoteTLSSignLimit, timeout: remoteTLSSignTimeout,
	}
	return tls.Certificate{
		Certificate: [][]byte{append([]byte(nil), identity.CertificateDER...)},
		PrivateKey:  signer,
		Leaf:        certificate,
	}, nil
}

func (signer *remoteTLSSigner) Public() crypto.PublicKey {
	return signer.publicKey
}

func (signer *remoteTLSSigner) Sign(_ io.Reader, digest []byte, options crypto.SignerOpts) ([]byte, error) {
	if options == nil || options.HashFunc() != crypto.SHA256 {
		return nil, errors.New("TLS helper supports only SHA-256")
	}
	limit := signer.limit
	if limit == nil {
		limit = remoteTLSSignLimit
	}
	select {
	case limit <- struct{}{}:
		defer func() { <-limit }()
	default:
		return nil, errors.New("TLS signing helper is busy")
	}
	timeout := signer.timeout
	if timeout <= 0 {
		timeout = remoteTLSSignTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return signer.helper.SignTLS(ctx, signer.generation, digest)
}
