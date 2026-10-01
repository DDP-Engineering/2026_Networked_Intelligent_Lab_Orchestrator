#!/usr/bin/env sh
set -eu

usage() {
    printf '%s\n' "usage: $0 [--engine docker|podman] [--source-date-epoch SECONDS]" >&2
    exit 2
}

engine=docker
source_date_epoch=1788566400
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) [ "$#" -ge 2 ] || usage; engine=$2; shift 2 ;;
        --source-date-epoch) [ "$#" -ge 2 ] || usage; source_date_epoch=$2; shift 2 ;;
        *) usage ;;
    esac
done
case "$engine" in docker|podman) ;; *) usage ;; esac
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
lock="$repo_root/02_Tools/containers/build/build-environment.lock.json"
containerfile="$repo_root/02_Tools/containers/build/Containerfile"
inputs="$repo_root/02_Tools/cache/containers/upstream"
python3 "$repo_root/02_Tools/scripts/build_environment.py" validate-lock --lock "$lock"
python3 "$repo_root/02_Tools/scripts/build_environment.py" validate-reconstruction --lock "$lock"
python3 "$repo_root/02_Tools/scripts/locked_build_inputs.py" verify --repository-root "$repo_root" --lock "$lock"
source_identity=$(python3 "$repo_root/02_Tools/scripts/build_environment.py" verify-source-state --repository-root "$repo_root")
source_base_commit=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["base_commit"])')
source_tree_sha256=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["tree_sha256"])')
source_status=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
command -v "$engine" >/dev/null 2>&1 || { printf '%s\n' "error: OCI engine '$engine' is unavailable" >&2; exit 1; }
command -v skopeo >/dev/null 2>&1 || { printf '%s\n' "error: skopeo is required for canonical OCI manifest digests after materialization" >&2; exit 1; }

generation=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["generation"])' "$lock")
image=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["image"]["reference"])' "$lock")
base=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["base_image"]["reference"])' "$lock")
frontend=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["oci"]["build_frontend"])' "$lock")

printf '%s\n' "networked rebuild selected: generation=$generation image=$image base=$base"

# Phase 1 - networked materialization. Every payload the offline build consumes,
# including the reviewed base and build-frontend images, is acquired and verified
# by immutable digest here, while network access is still permitted.
printf '%s\n' "networked materialization: acquiring the reviewed OCI images and the locked input closure"
python3 "$repo_root/02_Tools/scripts/build_environment.py" acquire-oci-images \
    --lock "$lock" --engine "$engine"
python3 "$repo_root/02_Tools/scripts/build_environment.py" materialize-inputs \
    --lock "$lock" --cache "$inputs"
if [ "$engine" = docker ]; then
    # BuildKit resolves the "# syntax=" frontend through its own content store, so a
    # daemon-side pull alone does not make the pinned frontend available to a
    # network-disabled build. Resolve it once here, by the same pinned digest.
    warm_context=$(mktemp -d "${TMPDIR:-/tmp}/nilo-frontend-warm.XXXXXX")
    printf '# syntax=%s\nFROM %s\n' "$frontend" "$base" > "$warm_context/Containerfile"
    if ! "$engine" build --pull=false --quiet --file "$warm_context/Containerfile" "$warm_context" >/dev/null; then
        rm -rf "$warm_context"
        printf '%s\n' "error: the pinned build frontend $frontend could not be resolved; the offline image build cannot proceed" >&2
        exit 1
    fi
    rm -rf "$warm_context"
    printf '%s\n' "build frontend resolved and cached by digest: $frontend"
fi

# Phase 2 - offline image build. No network, no pull, only verified local content.
printf '%s\n' "offline image build: --pull=false --network=none over the verified nilo_inputs context"
"$engine" build --pull=false --network=none \
    --build-context "nilo_inputs=$inputs" \
    --build-arg "BASE_IMAGE=$base" \
    --build-arg "SOURCE_DATE_EPOCH=$source_date_epoch" \
    --label "io.nilo.source-base-commit=$source_base_commit" \
    --label "io.nilo.source-tree.sha256=$source_tree_sha256" \
    --label "io.nilo.source-tree.status=$source_status" \
    --label "io.nilo.build-environment.generation=$generation" \
    --tag "$image" --file "$containerfile" "$repo_root"

"$repo_root/02_Tools/scripts/verify-build-environment.sh" --engine "$engine" --image "$image" --inventory-only
if [ "$engine" = docker ]; then transport="docker-daemon:$image"; else transport="containers-storage:$image"; fi
digest=$(skopeo inspect --format '{{.Digest}}' "$transport")
"$repo_root/02_Tools/scripts/collect-build-environment-metadata.sh" --engine "$engine" --image "$image" \
    --output "$repo_root/02_Tools/cache/containers/evidence/$generation"
printf '%s\n' "verified build environment: generation=$generation image digest=$digest"
printf '%s\n' "The image is locked and repeatable. Run verify-rebuild.sh twice on independent clean runners before claiming bit-reproducible."
