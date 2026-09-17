# REQ-SEC-618f47b2 - Keep appliance TLS private keys inside the helper

## Behavior

The helper creates one ECDSA P-256 key inside its root-owned keystore. Private
files are atomic mode `0600` below a mode-`0700` directory. Immutable
certificate generations and an atomic current pointer make failed staging or
activation preserve the previous generation.

Each certificate is self-signed with `NotBefore` set to 24 hours before helper
generation time and `NotAfter` ten years later. The helper renews it when fewer
than 180 days remain or when the advertised DNS-name set changes. Activation
keeps the preceding generation signable for a five-minute bounded lease so an
in-flight handshake cannot lose its key generation.

Bench Core reads public certificate DER and uses a local `crypto.Signer` proxy
for TLS handshake digests. No operation returns private PEM/DER, a reusable key
handle, a path, a command, or an arbitrary cryptographic primitive.

## Data / interface

The closed authenticated helper protocol adds:

```text
tls.identity.read -> generation, certificate_der[]
tls.identity.sign(generation, digest_base64, hash="sha256",
                  algorithm="ecdsa-p256-sha256") -> signature_base64
```

Sign accepts only a leased current generation, exactly 32 digest bytes and the
one algorithm. It rejects raw messages, other hashes, oversized payloads,
arbitrary key names and expired generations. Certificate preparation is a
helper-internal operation over validated local DNS labels.

Bench Core permits at most 32 concurrent helper-backed signatures. Each signer
call has a five-second helper deadline; saturation or timeout fails only that
TLS handshake. Certificate reads and rotations use an atomic public snapshot
and do not restart the listener.

## Examples

A Go TLS 1.2/1.3 handshake signs its SHA-256 digest through the helper. SHA-512
or an unleased generation returns `invalid_request` without a signature.

## Acceptance

- Real TLS clients complete handshakes through the proxy.
- Bench Core cannot read a private-key file or response field.
- Malformed, stale and unauthorized signing fails closed.
- Restart and failed rotation retain the prior valid generation.
- Sentinel key bytes appear in no response, log, diagnostic or export.

## Resolved design decisions

The validity, renewal, generation lease, concurrency and timeout bounds above
are the appliance contract. Target-browser acceptance and clock-failure tests
remain validation evidence, not open product policy.
