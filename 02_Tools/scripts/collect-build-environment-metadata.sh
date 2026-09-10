#!/usr/bin/env sh
set -eu

usage() { printf '%s\n' "usage: $0 --engine docker|podman --image REF --output DIR" >&2; exit 2; }
engine=
image=
output=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --engine) engine=${2:-}; shift 2 ;;
        --image) image=${2:-}; shift 2 ;;
        --output) output=${2:-}; shift 2 ;;
        *) usage ;;
    esac
done
[ -n "$engine" ] && [ -n "$image" ] && [ -n "$output" ] || usage
case "$engine" in docker|podman) ;; *) usage ;; esac
repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
: "${NILO_BUILDER_ID:=local Ubuntu 24.04 linux/amd64 OCI engine (native or WSL2)}"
export NILO_BUILDER_ID
source_identity=$(python3 "$repo_root/02_Tools/scripts/build_environment.py" verify-source-state --repository-root "$repo_root")
source_base_commit=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["base_commit"])')
source_tree_sha256=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["tree_sha256"])')
source_status=$(printf '%s' "$source_identity" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
mkdir -p "$output"
packages="$output/packages.tsv"
"$engine" run --rm --network none --entrypoint dpkg-query "$image" \
    -W '-f=${binary:Package}\t${Version}\t${Architecture}\n' > "$packages"
"$engine" run --rm --network none --entrypoint sh "$image" -lc \
    "find /usr/share/doc -type f -name copyright -print0 | sort -z | tar --null --files-from=- --owner=0 --group=0 --numeric-owner --sort=name --mtime='@1788566400' -czf -" \
    > "$output/licenses.tar.gz"
if [ "$engine" = docker ]; then transport="docker-daemon:$image"; else transport="containers-storage:$image"; fi
digest=$(skopeo inspect --format '{{.Digest}}' "$transport")
lock="$repo_root/02_Tools/containers/build/build-environment.lock.json"
python3 - "$packages" "$output/sbom.spdx.json" "$output/provenance.json" "$digest" "$source_base_commit" "$source_tree_sha256" "$source_status" "$lock" <<'PY'
import hashlib,json,os,pathlib,sys
from datetime import UTC,datetime
packages_path,sbom_path,provenance_path=map(pathlib.Path,sys.argv[1:4]); digest,base_commit,tree_sha256,status=sys.argv[4:8]; lock=pathlib.Path(sys.argv[8])
packages=[]
for index,line in enumerate(packages_path.read_text(encoding="utf-8").splitlines(),1):
    name,version,architecture=line.split("\t")
    packages.append({"name":name,"SPDXID":f"SPDXRef-Package-{index}","versionInfo":version,"downloadLocation":"NOASSERTION","filesAnalyzed":False,"licenseConcluded":"NOASSERTION","licenseDeclared":"NOASSERTION","comment":f"architecture={architecture}; licence text is in licenses.tar.gz"})
created=datetime.now(UTC).replace(microsecond=0).isoformat()
sbom={"spdxVersion":"SPDX-2.3","dataLicense":"CC0-1.0","SPDXID":"SPDXRef-DOCUMENT","name":"nilo-build-environment-image","documentNamespace":f"https://nilo.invalid/sbom/build-environment/{digest.removeprefix('sha256:')}","creationInfo":{"created":created,"creators":["Tool: collect-build-environment-metadata.sh"]},"packages":packages}
sbom_path.write_text(json.dumps(sbom,indent=2)+"\n",encoding="utf-8")
provenance={"schema":1,"image_digest":digest,"source":{"base_commit":base_commit,"tree_sha256":tree_sha256,"status":status},"lock_sha256":hashlib.sha256(lock.read_bytes()).hexdigest(),"built_at":created,"builder":os.environ["NILO_BUILDER_ID"],"definition":"02_Tools/containers/build/Containerfile"}
provenance_path.write_text(json.dumps(provenance,indent=2)+"\n",encoding="utf-8")
PY
rm -f "$packages"
printf '%s\n' "generated image SBOM, licence texts, and provenance in $output"
