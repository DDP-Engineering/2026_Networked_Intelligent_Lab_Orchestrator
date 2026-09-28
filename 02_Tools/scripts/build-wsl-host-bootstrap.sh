#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
archive_root="$repo_root/02_Tools/archives/wsl-host-bootstrap"
lock="$archive_root/host-bootstrap.lock.json"
[ -z "$(find "$archive_root" -maxdepth 1 -type f -name 'nilo-wsl-host-bootstrap-noble-20260905-01.tar.gz.part*' -print -quit)" ] || {
    printf '%s\n' "error: refusing to overwrite existing host-bootstrap generation parts; archive or remove them explicitly first" >&2
    exit 1
}
staging=$(mktemp -d "${TMPDIR:-/tmp}/nilo-host-bootstrap.XXXXXX")
trap 'rm -rf "$staging"' EXIT HUP INT TERM
mkdir -p "$staging/packages/common/partial" "$staging/packages/docker/partial" "$staging/packages/podman/partial"
mkdir -p "$staging/apt/lists/partial" "$staging/apt/archives/partial" "$staging/apt/sourceparts"
: > "$staging/apt/status"

python3 - "$lock" "$staging/apt/sources.list" "$staging/apt/docker.asc" <<'PY'
import hashlib,json,pathlib,sys,urllib.request
lock_path,sources_path,key_path=map(pathlib.Path,sys.argv[1:]); lock=json.loads(lock_path.read_text(encoding="utf-8"))
docker=lock["docker_repository"]
key=urllib.request.urlopen(docker["key_url"],timeout=120).read()
if hashlib.sha256(key).hexdigest()!=docker["key_sha256"]: raise SystemExit("Docker repository signing-key SHA-256 mismatch")
key_path.write_bytes(key)
snapshot=lock["ubuntu_snapshot"]
sources_path.write_text(
    f"deb [arch=amd64 check-valid-until=no signed-by=/usr/share/keyrings/ubuntu-archive-keyring.gpg] {snapshot} noble main universe restricted multiverse\n"
    f"deb [arch=amd64 check-valid-until=no signed-by=/usr/share/keyrings/ubuntu-archive-keyring.gpg] {snapshot} noble-updates main universe restricted multiverse\n"
    f"deb [arch=amd64 check-valid-until=no signed-by=/usr/share/keyrings/ubuntu-archive-keyring.gpg] {snapshot} noble-security main universe restricted multiverse\n"
    f"deb [arch=amd64 signed-by={key_path}] {docker['url']} noble stable\n", encoding="utf-8")
PY

apt_get() {
    apt-get -o "Dir::Etc::sourcelist=$staging/apt/sources.list" \
        -o "Dir::Etc::sourceparts=$staging/apt/sourceparts" \
        -o "Dir::State::lists=$staging/apt/lists" \
        -o "Dir::State::status=$staging/apt/status" \
        -o "Dir::Cache::archives=$staging/apt/archives" "$@"
}
apt_get update

python3 - "$lock" > "$staging/roots.txt" <<'PY'
import json,sys
d=json.load(open(sys.argv[1], encoding="utf-8"))
for name, group in (("common", d["common"]), ("docker", d["engines"]["docker"]), ("podman", d["engines"]["podman"])):
    print(name + "|" + " ".join(group))
PY

printf '%s\n' "This explicit generator accesses only the pinned Ubuntu snapshot and Docker package repository."
while IFS='|' read -r group roots; do
    [ -n "$roots" ] || continue
    # Let APT choose one valid dependency alternative for an empty target host.
    # shellcheck disable=SC2086
    apt_get -o "Dir::Cache::archives=$staging/packages/$group" \
        --download-only --no-install-recommends --reinstall -y install $roots
done < "$staging/roots.txt"

python3 - "$staging/packages" "$lock" "$staging/current.json" <<'PY'
import hashlib,json,pathlib,subprocess,sys
root,lock_path,out=map(pathlib.Path,sys.argv[1:])
lock=json.loads(lock_path.read_text(encoding="utf-8"))
def sha256(path):
    digest=hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024*1024), b""): digest.update(chunk)
    return digest.hexdigest()
def control_field(path, field):
    return subprocess.check_output(
        ["dpkg-deb","-f",str(path),field], text=True
    ).strip()
def records(group):
    found=[]
    for path in sorted((root/group).glob("*.deb")):
        found.append({"package":control_field(path,"Package"),"version":control_field(path,"Version"),"architecture":control_field(path,"Architecture"),"filename":f"packages/{group}/{path.name}","size":path.stat().st_size,"sha256":sha256(path),"source":"reviewed repositories in host-bootstrap.lock.json","license":"/usr/share/doc/<package>/copyright after installation"})
    return found
common=records("common"); docker=records("docker"); podman=records("podman")
resolved={item["package"]:item for item in common+docker+podman}
for pin in lock["root_package_pins"]:
    item=resolved.get(pin["name"])
    if item is None or item["version"]!=pin["version"] or item["sha256"]!=pin["sha256"]:
        raise SystemExit(f"resolved host package does not match reviewed pin: {pin['name']}")
data={"schema":1,"generation":lock["generation"],"status":"generated-unpublished","archive":{"filename":f"nilo-wsl-host-bootstrap-{lock['generation']}.tar.gz","size":None,"sha256":None},"parts":[],"common":common,"engines":{"docker":docker,"podman":podman}}
out.write_text(json.dumps(data,indent=2)+"\n",encoding="utf-8")
PY

cp "$lock" "$staging/host-bootstrap.lock.json"
tar --sort=name --mtime='@1788566400' --owner=0 --group=0 --numeric-owner -czf "$staging/host-bootstrap.tar.gz" -C "$staging" packages host-bootstrap.lock.json current.json
size=$(wc -c < "$staging/host-bootstrap.tar.gz" | tr -d ' ')
digest=$(sha256sum "$staging/host-bootstrap.tar.gz" | awk '{print $1}')
python3 - "$staging/current.json" "$size" "$digest" <<'PY'
import json,sys
p=sys.argv[1]; d=json.load(open(p,encoding="utf-8")); d["archive"]["size"]=int(sys.argv[2]); d["archive"]["sha256"]=sys.argv[3]; open(p,"w",encoding="utf-8").write(json.dumps(d,indent=2)+"\n")
PY
split -b 1073741824 -d -a 3 "$staging/host-bootstrap.tar.gz" "$archive_root/nilo-wsl-host-bootstrap-noble-20260905-01.tar.gz.part"
python3 - "$staging/current.json" "$archive_root" <<'PY'
import hashlib,json,pathlib,sys
manifest=pathlib.Path(sys.argv[1]); root=pathlib.Path(sys.argv[2]); d=json.loads(manifest.read_text(encoding="utf-8")); parts=[]
def sha256(path):
    digest=hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024*1024), b""): digest.update(chunk)
    return digest.hexdigest()
for ordinal,path in enumerate(sorted(root.glob("nilo-wsl-host-bootstrap-*.tar.gz.part[0-9][0-9][0-9]"))):
    digest=sha256(path); parts.append({"ordinal":ordinal,"filename":path.name,"size":path.stat().st_size,"sha256":digest,"lfs_oid":"sha256:"+digest})
d["parts"]=parts; manifest.write_text(json.dumps(d,indent=2)+"\n",encoding="utf-8")
PY
cp "$staging/current.json" "$archive_root/current.json"
printf '%s\n' "host-bootstrap archive generated; review current.json and every part before publication"
