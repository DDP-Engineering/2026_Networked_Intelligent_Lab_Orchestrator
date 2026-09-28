#!/bin/sh
# Run inside a disposable exact-target system namespace with no external route.
set -eu
[ "$#" -eq 5 ] || exit 2
target=$1
check=$2
deb=$3
installer=$4
prior_installer=$5
version=$(dpkg-deb -f "$deb" Version)

wait_for_service() {
  attempts=0
  until systemctl is-active --quiet nilo.service; do
    attempts=$((attempts + 1))
    [ "$attempts" -lt 60 ] || { echo 'NILO service did not become active' >&2; return 1; }
    sleep 1
  done
}

prepare_first_administrator() {
  credential_file=$1
  password=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
  [ ${#password} -eq 48 ] || { echo 'could not generate an acceptance credential' >&2; return 1; }
  install -d -m 0750 /var/lib/nilo
  install -d -o 0 -g 0 -m 0700 /var/lib/nilo-installer
  umask 077
  printf '%s\n' "$password" >"$credential_file"
  printf '%s\n' "$password" >/var/lib/nilo-installer/first-admin.password
}

install_prior_state() {
  [ -n "$prior_installer" ] && [ -f "$prior_installer" ] && [ ! -L "$prior_installer" ] || {
    echo 'rollback acceptance requires a distinct prior-installer' >&2; return 2;
  }
  sh "$prior_installer"
  prior_version=$(dpkg-query -W -f='${Version}\n' nilo)
  [ "$prior_version" != "$version" ] || { echo 'prior installer is not a distinct version' >&2; return 2; }
  install -d -m 0750 /var/lib/nilo/integrations/acceptance
  printf '%s\n' 'representative-configuration' >/var/lib/nilo/acceptance-rollback-state
  printf '%s\n' 'installed-and-enabled' >/var/lib/nilo/integrations/acceptance/state
  printf '%s\n' 'representative-audit-entry' >/var/lib/nilo/acceptance-audit
}

verify_prior_state() {
  after=$(dpkg-query -W -f='${Version}\n' nilo 2>/dev/null || true)
  [ "$after" = "$prior_version" ] || { echo 'rollback did not restore the prior package version' >&2; return 1; }
  grep -qx 'representative-configuration' /var/lib/nilo/acceptance-rollback-state || {
    echo 'rollback did not preserve representative application state' >&2; return 1;
  }
}

verify_fault_rollback() {
  phase=$1
  install_prior_state
  if NILO_ACCEPTANCE_FAIL_PHASE="$phase" sh "$installer"; then
    echo "fault-injected installer unexpectedly succeeded at $phase" >&2; return 1
  fi
  verify_prior_state
}

verify_operation_rollback() {
  operation=$1
  install_prior_state
  if NILO_ACCEPTANCE_FAIL_OPERATION="$operation" sh "$installer"; then
    echo "operation fault unexpectedly succeeded inside $operation installation" >&2; return 1
  fi
  verify_prior_state
}

verify_interrupted_rollback() {
  phase=$1
  install_prior_state
  if NILO_ACCEPTANCE_FAIL_PHASE=migrated \
      NILO_ACCEPTANCE_INTERRUPT_PHASE="$phase" sh "$installer"; then
    echo "rollback interruption unexpectedly succeeded at $phase" >&2; return 1
  fi
  /usr/libexec/nilo/nilo-installer-recover
  verify_prior_state
}

case "$check" in
  package-install)
    # Provision the offline dependencies through the authenticated installer,
    # remove only NILO, then exercise the Debian package itself.
    sh "$installer"
    dpkg --remove nilo
    dpkg -i "$deb"
    dpkg-query -W -f='${Status}\n' nilo | grep -qx 'install ok installed'
    ;;
  package-reinstall)
    sh "$installer"
    dpkg -i "$deb"
    dpkg -i "$deb"
    dpkg-query -W -f='${Version}\n' nilo | grep -qx "$version"
    ;;
  installer-clean-install)
    sh "$installer"
    dpkg-query -W -f='${Version}\n' nilo | grep -qx "$version"
    ;;
  installer-upgrade)
    install_prior_state
    sh "$installer"
    dpkg-query -W -f='${Version}\n' nilo | grep -qx "$version"
    grep -qx 'representative-configuration' /var/lib/nilo/acceptance-rollback-state
    grep -qx 'installed-and-enabled' /var/lib/nilo/integrations/acceptance/state
    ;;
  installer-clean-rollback)
    [ ! -e /var/lib/nilo ] || { echo 'clean rollback check did not start clean' >&2; exit 2; }
    if NILO_ACCEPTANCE_FAIL_PHASE=package sh "$installer"; then
      echo 'clean fault-injected installer unexpectedly succeeded' >&2; exit 1
    fi
    ! dpkg-query -W -f='${Status}\n' nilo 2>/dev/null | grep -qx 'install ok installed'
    [ ! -e /var/lib/nilo ] || { echo 'clean rollback left candidate application state' >&2; exit 1; }
    ;;
  installer-rollback-dependencies) verify_operation_rollback dependencies ;;
  installer-rollback-package) verify_operation_rollback package ;;
  installer-rollback-migrated) verify_fault_rollback migrated ;;
  installer-rollback-started) verify_fault_rollback started ;;
  installer-rollback-verified) verify_fault_rollback verified ;;
  installer-interruption-preflight|installer-interruption-quiesced|installer-interruption-snapshotted|installer-interruption-dependencies|installer-interruption-package|installer-interruption-migrated|installer-interruption-started|installer-interruption-verified)
    interruption_phase=${check#installer-interruption-}
    install_prior_state
    if NILO_ACCEPTANCE_INTERRUPT_PHASE="$interruption_phase" sh "$installer"; then
      echo 'interrupted installer unexpectedly succeeded' >&2; exit 1
    fi
    /usr/libexec/nilo/nilo-installer-recover
    verify_prior_state
    ;;
  installer-interruption-committed)
    install_prior_state
    if NILO_ACCEPTANCE_INTERRUPT_PHASE=committed sh "$installer"; then
      echo 'committed installer interruption unexpectedly succeeded' >&2; exit 1
    fi
    /usr/libexec/nilo/nilo-installer-recover
    dpkg-query -W -f='${Version}\n' nilo | grep -qx "$version"
    grep -qx 'representative-configuration' /var/lib/nilo/acceptance-rollback-state
    ;;
  installer-interruption-rolling_back) verify_interrupted_rollback rolling_back ;;
  installer-interruption-rolled_back) verify_interrupted_rollback rolled_back ;;
  installer-interruption-recovery-required)
    install_prior_state
    if NILO_ACCEPTANCE_FAIL_PHASE=migrated \
        NILO_ACCEPTANCE_FORCE_RECOVERY_REQUIRED=1 \
        NILO_ACCEPTANCE_INTERRUPT_PHASE=recovery-required sh "$installer"; then
      echo 'recovery-required interruption unexpectedly succeeded' >&2; exit 1
    fi
    /usr/libexec/nilo/nilo-installer-recover
    verify_prior_state
    ;;
  removal-purge)
    sh "$installer"
    test -f /var/lib/nilo/nilo.db
    /usr/bin/nilo maintenance purge --data-dir /var/lib/nilo --confirm-local
    test ! -f /var/lib/nilo/nilo.db
    dpkg --remove nilo
    ;;
  core-status-gui-helper-catalogue)
    sh "$installer"
    wait_for_service
    systemctl is-active nilo.service nilo-nethelper.socket nilo-nethelper.service
    test -S /run/nilo/nethelper.sock
    curl --fail --silent --show-error --max-time 10 http://127.0.0.1:8091/ | grep -F "$version"
    test -s /usr/share/nilo/gui/index.html
    find /usr/lib/nilo/integration-bundles -mindepth 2 -maxdepth 2 -name manifest.json -print -quit | grep -q .
    ;;
  integration-install-upgrade-health)
    credential_file=$(mktemp)
    lifecycle_program=$(mktemp)
    trap 'rm -f "$credential_file" "$lifecycle_program"' EXIT HUP INT TERM
    prepare_first_administrator "$credential_file"
    sh "$installer"
    wait_for_service
    runtime=/usr/lib/nilo/integration-bundles/sigrok/runtime/bin/python3
    [ -x "$runtime" ] || { echo 'packaged Sigrok Python runtime is not executable' >&2; exit 1; }
    cat >"$lifecycle_program" <<'PY'
import hashlib
import http.cookiejar
import json
import pathlib
import ssl
import sys
import time
import urllib.error
import urllib.request

credential_path = pathlib.Path(sys.argv[1])
catalog = pathlib.Path("/usr/lib/nilo/integration-bundles/sigrok")
password = credential_path.read_text(encoding="utf-8").strip()
context = ssl._create_unverified_context()
opener = urllib.request.build_opener(
    urllib.request.HTTPSHandler(context=context),
    urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
)
csrf = ""

def request(path, body=None):
    headers = {"Content-Type": "application/json"}
    if csrf:
        headers["X-NILO-CSRF"] = csrf
    data = None if body is None else json.dumps(body).encode("utf-8")
    with opener.open(
        urllib.request.Request("https://127.0.0.1:8443" + path, data=data, headers=headers),
        timeout=15,
    ) as response:
        return json.load(response)

for attempt in range(30):
    try:
        login = request("/api/v1/auth/login", {"username": "admin", "password": password})
        break
    except (OSError, urllib.error.URLError):
        if attempt == 29:
            raise
        time.sleep(1)
csrf = login["csrf_token"]
installed = request("/api/v1/plugins/sigrok/install", {})
prior_version = installed["version"]
manifest_path = catalog / "manifest.json"
index_path = catalog / "files.sha256.json"
manifest_before = manifest_path.read_bytes()
index_before = index_path.read_bytes()
try:
    manifest = json.loads(manifest_before)
    manifest["package"]["version"] = "99.0.0-acceptance"
    encoded = (json.dumps(manifest, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")
    manifest_path.write_bytes(encoded)
    index = json.loads(index_before)
    index["manifest.json"] = hashlib.sha256(encoded).hexdigest()
    index_path.write_text(json.dumps(index, sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")
    upgraded = request("/api/v1/plugins/sigrok/upgrade", {})
    if upgraded["version"] != "99.0.0-acceptance" or upgraded["version"] == prior_version:
        raise RuntimeError("integration upgrade did not activate the authenticated candidate")
finally:
    manifest_path.write_bytes(manifest_before)
    index_path.write_bytes(index_before)
PY
    chmod 0700 "$lifecycle_program"
    "$runtime" "$lifecycle_program" "$credential_file"
    ;;
  *) echo "unsupported target lifecycle check: $target/$check" >&2; exit 2 ;;
esac
