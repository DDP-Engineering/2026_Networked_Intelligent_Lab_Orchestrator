#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)
python_root="$repo_root/02_Tools/runtime/linux-x86_64/python/cpython-3.12.10-linux-x86_64-gnu"
base_image=nilo-appliance-stage:local
run_id="packaging-integration-$$"
test_image="nilo-packaging-boundary-test:$run_id"
cache_volume="nilo-build-cache-$run_id"
temporary_root=$(mktemp -d)
staged_root="$temporary_root/repository"
shim_root="$temporary_root/bin"
docker_log="$temporary_root/docker-arguments.log"
real_docker=$(command -v docker || true)

cleanup() {
    if [ -n "$real_docker" ]; then
        "$real_docker" volume rm -f "$cache_volume" >/dev/null 2>&1 || true
        "$real_docker" image rm -f "$test_image" >/dev/null 2>&1 || true
    fi
    case "$temporary_root" in
        /tmp/*) rm -rf -- "$temporary_root" ;;
        *) printf '%s\n' "error: refusing to remove unexpected temporary path: $temporary_root" >&2 ;;
    esac
}
trap cleanup EXIT HUP INT TERM

[ -n "$real_docker" ] || { printf '%s\n' "error: docker is required" >&2; exit 1; }
[ -x "$python_root/bin/python" ] || {
    printf '%s\n' "error: pinned Linux Python runtime is missing: $python_root/bin/python" >&2
    exit 1
}
"$real_docker" image inspect "$base_image" >/dev/null 2>&1 || {
    printf '%s\n' "error: appliance test image is missing: $base_image" >&2
    exit 1
}

mkdir -p \
    "$staged_root/.git" \
    "$staged_root/02_Tools/archives/build-environment" \
    "$staged_root/02_Tools/env" \
    "$staged_root/02_Tools/packaging/build" \
    "$staged_root/02_Tools/packaging/config" \
    "$staged_root/02_Tools/cache/packaging" \
    "$shim_root"
cp "$repo_root/02_Tools/archives/build-environment/current.json" \
    "$staged_root/02_Tools/archives/build-environment/current.json"
cp "$repo_root/02_Tools/env/with-build-environment.sh" \
    "$staged_root/02_Tools/env/with-build-environment.sh"
cp "$repo_root/02_Tools/packaging/package_inputs.py" \
    "$staged_root/02_Tools/packaging/package_inputs.py"
cp "$repo_root/02_Tools/packaging/build/prepare-offline.sh" \
    "$staged_root/02_Tools/packaging/build/prepare-offline.sh"
cp "$repo_root/02_Tools/packaging/config/targets.json" \
    "$staged_root/02_Tools/packaging/config/targets.json"

python3 - "$staged_root" <<'PY'
import hashlib
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
payload = b"nilo-packaging-boundary-fixture\n"
digest = hashlib.sha256(payload).hexdigest()
lock = {
    "complete": True,
    "incomplete_reasons": [],
    "inputs": [
        {
            "license": "MIT",
            "name": "boundary-fixture",
            "scope": "ubuntu-24.04-amd64",
            "sha256": digest,
            "size": len(payload),
            "url": "https://example.invalid/boundary-fixture",
            "version": "1.0.0",
        }
    ],
    "schema": 1,
}
lock_path = root / "02_Tools/packaging/config/inputs.lock.json"
lock_path.write_text(json.dumps(lock, indent=2, sort_keys=True) + "\n", encoding="utf-8")
object_path = root / "02_Tools/cache/packaging/objects/sha256" / digest[:2] / digest
object_path.parent.mkdir(parents=True, exist_ok=True)
object_path.write_bytes(payload)
PY

"$real_docker" build --network none --pull=false \
    --build-arg "GENERATION=$run_id" \
    --build-arg "HOST_UID=$(id -u)" \
    --build-arg "HOST_GID=$(id -g)" \
    --tag "$test_image" \
    --file "$repo_root/02_Tools/tests/packaging/Dockerfile" \
    "$python_root" >/dev/null

cat >"$shim_root/skopeo" <<'SH'
#!/usr/bin/env sh
printf '%s\n' 'sha256:packaging-boundary-test'
SH
cat >"$shim_root/docker" <<SH
#!/usr/bin/env sh
printf '%s\n' "\$*" >>"$docker_log"
exec "$real_docker" "\$@"
SH
chmod +x "$shim_root/skopeo" "$shim_root/docker"

PATH="$shim_root:$PATH" sh "$staged_root/02_Tools/env/with-build-environment.sh" \
    --engine docker \
    --image "$test_image" \
    package ubuntu-24.04-amd64

grep -F -- '--network none' "$docker_log" >/dev/null
grep -F -- '/packaging-cache:ro' "$docker_log" >/dev/null

receipt="$staged_root/02_Tools/cache/containers/outputs/packaging/ubuntu-24.04-amd64/preparation.json"
python3 - "$receipt" <<'PY'
import json
import pathlib
import sys

receipt_path = pathlib.Path(sys.argv[1])
receipt = json.loads(receipt_path.read_text(encoding="utf-8"))
assert receipt["target"]["id"] == "ubuntu-24.04-amd64"
assert receipt["offline"] == {
    "interfaces": ["lo"],
    "ipv4_default_route": False,
    "ipv6_default_route": False,
    "platform": "linux",
    "verified": True,
}
assert receipt["hardware_acceptance"]["status"] == "not_run"
assert [item["name"] for item in receipt["inputs"]] == ["boundary-fixture"]
PY

printf '%s\n' "packaging OCI boundary check passed"
