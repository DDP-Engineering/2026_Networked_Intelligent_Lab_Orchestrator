# OCI build definition

Build from the repository root with
`../../scripts/build-build-environment.sh --engine docker` (or `podman`). The
script validates `build-environment.lock.json` and refuses to start while its
`reconstruction` record is incomplete. Candidate generation `noble-20260929-01` has a
complete 680-record Linux amd64 closure in `reconstruction-inputs.json`: 244 apt
packages, 264 npm tarballs, 138 Go proxy objects, 24 Python wheels, two source-license
archives, one pinned Git source archive, three browser payloads, and one archive each for Go, uv,
Node, and managed CPython. The
materializer rejects missing, extra, symlinked, size-mismatched, or SHA-256-
mismatched content before the engine starts. The Containerfile consumes the
verified directory through the `nilo_inputs` named context while the engine
build runs with `--network=none`.

`locked_build_inputs.py verify` checks every source Go checksum lock, Python
lock and frontend tarball URL/version against this inventory before build and
export. Go restore checks
the module content against `go.sum`. Python preparation verifies wheel and Git
archive hashes before creating environments, installs each project's exact
locked wheels and local dependencies, builds the pinned Git archive offline
with pinned setuptools/wheel, and checks the installed dependency closure.
The image includes `iproute2` for unprivileged preflight checks.
The Modbus feasibility environment includes PyModbus 3.15.0 and pySerial 3.5;
their hash-verified BSD notices are retained under
`/opt/nilo/environment/licenses/modbus/` and included in generated licence evidence.

The archive's `current.json` continues to describe its existing immutable
generation. Rebuild, offline verification, export and import must succeed before
replacing it with an archive for this recipe; publication and independent
backup readback remain human operations.

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
