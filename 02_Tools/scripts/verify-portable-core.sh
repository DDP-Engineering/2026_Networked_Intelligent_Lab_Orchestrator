#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repository_root="$(cd -- "$script_dir/../.." && pwd)"
module_root="$repository_root/05_Software/core"
temporary_directory="$(mktemp -d "${TMPDIR:-/tmp}/nilo-portable-core.XXXXXX")"

cleanup() {
    rm -rf -- "$temporary_directory"
}
trap cleanup EXIT

cd "$module_root"
go test ./...
go build -o "$temporary_directory/nilo-host" ./cmd/nilo

for target in linux/arm64 linux/amd64 windows/amd64; do
    os="${target%/*}"
    arch="${target#*/}"
    extension=""
    if [[ "$os" == "windows" ]]; then
        extension=".exe"
    fi
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$temporary_directory/nilo-$os-$arch$extension" ./cmd/nilo
done
