#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 --engine docker|podman [--install --archive-path DIR [--allow-downgrades]]" >&2; exit 2; }
engine=
install=0
archive_path=
allow_downgrades=0
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) engine=${2:-}; shift 2 ;;
        --install) install=1; shift ;;
        --archive-path) archive_path=${2:-}; shift 2 ;;
        --allow-downgrades) allow_downgrades=1; shift ;;
        *) usage ;;
    esac
done
case "$engine" in docker|podman) ;; *) usage ;; esac
[ "$allow_downgrades" -eq 0 ] || [ "$install" -eq 1 ] || usage
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
engine_preinstalled=0
if command -v "$engine" >/dev/null 2>&1; then
    engine_preinstalled=1
fi

if [ "$install" -eq 1 ]; then
    [ -n "$archive_path" ] || { printf '%s\n' "error: explicit engine installation requires --archive-path with verified local packages" >&2; exit 1; }
    manifest="$archive_path/current.json"
    [ -f "$manifest" ] || { printf '%s\n' "error: host-bootstrap manifest is missing: $manifest" >&2; exit 1; }
    staging=$(mktemp -d "${TMPDIR:-/tmp}/nilo-host-bootstrap-install.XXXXXX")
    trap 'rm -rf "$staging"' EXIT HUP INT TERM
    python3 - "$manifest" "$archive_path" "$staging/archive.tar.gz" <<'PY'
import hashlib,json,pathlib,sys
manifest,root,joined=map(pathlib.Path,sys.argv[1:]); data=json.loads(manifest.read_text(encoding="utf-8"))
if data.get("schema") != 1 or not data.get("parts"):
    raise SystemExit("host-bootstrap archive has not been generated")
digest=hashlib.sha256()
with joined.open("wb") as output:
    for ordinal,item in enumerate(data["parts"]):
        if item.get("ordinal") != ordinal: raise SystemExit("host-bootstrap part order mismatch")
        path=root/item["filename"]
        if not path.is_file(): raise SystemExit(f"missing host-bootstrap part: {item['filename']}")
        part_digest=hashlib.sha256(); size=0
        with path.open("rb") as stream:
            for payload in iter(lambda: stream.read(1024*1024), b""):
                output.write(payload); digest.update(payload); part_digest.update(payload); size+=len(payload)
        actual=part_digest.hexdigest()
        if size!=item["size"] or actual!=item["sha256"] or item["lfs_oid"]!="sha256:"+actual: raise SystemExit(f"invalid host-bootstrap part: {item['filename']}")
if joined.stat().st_size!=data["archive"]["size"] or digest.hexdigest()!=data["archive"]["sha256"]: raise SystemExit("host-bootstrap joined archive mismatch")
PY
    tar -xzf "$staging/archive.tar.gz" -C "$staging"
    extracted_manifest="$staging/current.json"
    python3 - "$extracted_manifest" "$staging" "$engine" <<'PY'
import hashlib,json,pathlib,sys
manifest,root,selected=pathlib.Path(sys.argv[1]),pathlib.Path(sys.argv[2]),sys.argv[3]; data=json.loads(manifest.read_text(encoding="utf-8"))
if selected not in data.get("engines", {}): raise SystemExit("unsupported engine in host-bootstrap archive")
def sha256(path):
    digest=hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024*1024), b""): digest.update(chunk)
    return digest.hexdigest()
for item in data["common"]+data["engines"][selected]:
    path=root/item["filename"]
    if not path.is_file() or path.stat().st_size!=item["size"] or sha256(path)!=item["sha256"]: raise SystemExit(f"host-bootstrap package verification failed: {item['filename']}")
PY
    packages=$(python3 -c 'import json,sys,pathlib; d=json.load(open(sys.argv[1],encoding="utf-8")); r=pathlib.Path(sys.argv[2]); selected=sys.argv[3]; preserve=sys.argv[4]=="1"; items=d["common"] if preserve else d["common"]+d["engines"][selected]; print(" ".join(str(r/x["filename"]) for x in items))' "$extracted_manifest" "$staging" "$engine" "$engine_preinstalled")
    # Checksum verification proves package identity; it does not make host
    # mutation safe. Calculate and display the exact transaction, and refuse
    # downgrades and incompatible architecture replacements, before the host is
    # touched at all.
    plan_directory="$staging/nilo-transaction"
    mkdir -p "$plan_directory"
    for package in $packages; do
        cp "$package" "$plan_directory/"
    done
    if [ "$allow_downgrades" -eq 1 ]; then
        python3 "$repo_root/02_Tools/scripts/build_environment.py" plan-host-transaction \
            --package-dir "$plan_directory" --allow-downgrades
    else
        python3 "$repo_root/02_Tools/scripts/build_environment.py" plan-host-transaction \
            --package-dir "$plan_directory"
    fi

    # Host mutation is confined to this explicit branch. APT installs the
    # already-verified local closure only after the identical no-download,
    # no-removal transaction has passed simulation above.
    if [ "$allow_downgrades" -eq 1 ]; then
        sudo apt-get --no-download --no-install-recommends --no-remove \
            --allow-downgrades install -y $packages
    else
        sudo apt-get --no-download --no-install-recommends --no-remove \
            install -y $packages
    fi
    if [ "$engine_preinstalled" -eq 1 ]; then
        printf '%s\n' "preserving existing selected engine: $($engine --version)"
    elif [ "$engine" = docker ]; then
        sudo systemctl enable --now docker
    else
        systemctl --user enable --now podman.socket >/dev/null 2>&1 || true
    fi
fi

command -v "$engine" >/dev/null 2>&1 || { printf '%s\n' "error: selected engine '$engine' is not installed on this Ubuntu host" >&2; exit 1; }
command -v skopeo >/dev/null 2>&1 || { printf '%s\n' "error: skopeo is unavailable; it is required for canonical OCI archive and manifest digest operations" >&2; exit 1; }
if [ "$engine" = docker ]; then
    "$engine" info >/dev/null
    storage=$("$engine" info --format '{{.DockerRootDir}}')
else
    "$engine" info >/dev/null
    storage=$("$engine" info --format '{{.Store.GraphRoot}}')
fi
case "$storage" in /mnt/*) printf '%s\n' "error: engine storage must stay on the Linux filesystem, got $storage" >&2; exit 1 ;; esac
printf '%s\n' "engine verified: $($engine --version); storage=$storage"
