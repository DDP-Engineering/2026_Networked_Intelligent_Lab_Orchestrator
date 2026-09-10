# OCI build definition

Build from the repository root with
`../../scripts/build-build-environment.sh --engine docker` (or `podman`). The
script validates `build-environment.lock.json` and refuses to start while its
`reconstruction` record is incomplete. The first-generation reconstruction is a
complete 650-record Linux amd64 closure in `reconstruction-inputs.json`: 235 apt
packages, 264 npm tarballs, 126 Go proxy objects, 18 Python wheels, three browser
payloads, and one archive each for Go, uv, Node, and managed CPython. The
materializer rejects missing, extra, symlinked, size-mismatched, or SHA-256-
mismatched content before the engine starts. The Containerfile consumes the
verified directory through the `nilo_inputs` named context while the engine
build runs with `--network=none`.

The tracked SBOM and provenance files describe the reviewed source definition.
A real build must replace their pending image fields with generated image
evidence and collect `/usr/share/doc/*/copyright` for all installed dependency
packages. The first accepted generation additionally requires two clean builds,
offline repository verification, export/import digest proof, and the external
evidence listed in `../../tests/build-environment/README.md`.

A local proof image named `nilo/build-environment:closure-test` has exercised the
complete network-disabled build and inventory, but it is not the canonical lock
reference and has no source-provenance labels. It therefore does not satisfy the
canonical build/export gate or any archive, replica, or independent-runner gate.

Do not add secrets, Git configuration, SSH agents, credentials, an engine
daemon, or its socket to the build context or image. Do not add Clang/LLVM as a
second compiler path: native and ARM64 targets use GCC plus binutils.
