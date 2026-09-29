#!/usr/bin/env sh
# Public Linux command surface. Bootstrap and privileged fixtures stay separate.
set -eu

tools=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH= cd -- "$tools/.." && pwd)
usage() {
    cat <<'HELP'
NILO repository tools (Ubuntu 24.04 amd64)
  setup [all|tools|container] [--offline] [--engine docker|podman]
        [--archive-path DIR] [--rebuild-from-pinned-inputs]
        [--install-engine --engine-archive-path DIR [--allow-engine-downgrades]]
  dev backend --data-dir /absolute/Linux/path [--backend-port PORT]
        [--scenario healthy|unavailable]
  doctor [all|tools|container] [--engine docker|podman]
  test [offline|tools|network|appliance|packaging] [--engine docker|podman]
  build [--engine docker|podman]
  package TARGET [--engine docker|podman]
  package release ARTIFACTS ACCEPTANCE SOURCE_OFFERS UBUNTU_RECEIPT PI_RECEIPT OUTPUT
  package verify RELEASE_DIRECTORY

setup prepares pinned host tools and the canonical container. --offline forbids
downloads; WSL/engine installation and connected rebuilds require explicit flags.
dev runs the real simulation backend in this terminal; Ctrl+C stops it. On
Windows use nilo-tools.ps1 dev start for the backend and GUI development servers.
test defaults to the network-disabled container. Named acceptance fixtures are
explicit host actions; network/packaging fixtures currently require Docker.
build publishes binaries and GUI under 02_Tools/cache/containers/outputs.
package TARGET prepares offline inputs; it does not assemble a finished release.
Release assembly/verification use already-built inputs and local pinned tools.
Targets: ubuntu-24.04-amd64, rpi-debian13-arm64.
Advanced archive/image operations: see 02_Tools/containers/build/README.md.
HELP
}
bad() { printf '%s\n' "error: $*" >&2; usage >&2; exit 2; }
command=${1:-help}
[ "$#" -eq 0 ] || shift
case "$command" in help|--help|-h) [ "$#" -eq 0 ] || bad 'help takes no arguments'; usage; exit 0 ;; esac
case "$command" in setup|dev|doctor|test|build|package) ;; *) bad "unknown command: $command" ;; esac

mode=
if [ "$#" -gt 0 ]; then
    case "$1" in --*) ;; *) mode=$1; shift ;; esac
fi
# Release paths are kept as individual arguments and never evaluated as shell code.
if [ "$command" = package ] && [ "$mode" = release ]; then
    [ "$#" -eq 6 ] || bad 'package release requires six paths'
    exec sh "$tools/env/with-tools.sh" sh "$tools/packaging/build/build-release.sh" "$@"
fi
if [ "$command" = package ] && [ "$mode" = verify ]; then
    [ "$#" -eq 1 ] || bad 'package verify requires one directory'
    exec sh "$tools/env/with-tools.sh" sh "$tools/packaging/verify/verify-release.sh" "$@"
fi

engine=docker
offline=0
rebuild=0
install=0
downgrades=0
archive=
engine_archive=
data=
port=8090
scenario=healthy
in_container=0
while [ "$#" -gt 0 ]; do
    option=$1
    case "$option" in
        --in-container)
            case "$command" in test|build) in_container=1; shift ;; *) bad '--in-container is only valid for test/build' ;; esac ;;
        --offline|--rebuild-from-pinned-inputs|--install-engine|--allow-engine-downgrades)
            [ "$command" = setup ] || bad "$option is only valid for setup"
            case "$option" in
                --offline) offline=1 ;; --rebuild-from-pinned-inputs) rebuild=1 ;;
                --install-engine) install=1 ;; --allow-engine-downgrades) downgrades=1 ;;
            esac
            shift ;;
        --engine|--archive-path|--engine-archive-path|--data-dir|--backend-port|--scenario)
            [ "$#" -ge 2 ] && [ -n "$2" ] || bad "$option requires a value"
            case "$2" in --*) bad "$option requires a value before the next option" ;; esac
            case "$option" in
                --engine) [ "$command" != dev ] || bad '--engine is not a dev option'; engine=$2 ;;
                --archive-path|--engine-archive-path)
                    [ "$command" = setup ] || bad "$option is only valid for setup"
                    if [ "$option" = --archive-path ]; then archive=$2; else engine_archive=$2; fi ;;
                *) [ "$command" = dev ] || bad "$option is only valid for dev"
                    case "$option" in --data-dir) data=$2 ;; --backend-port) port=$2 ;; --scenario) scenario=$2 ;; esac ;;
            esac
            shift 2 ;;
        *) bad "unknown option: $option" ;;
    esac
done
case "$engine" in docker|podman) ;; *) bad 'engine must be docker or podman' ;; esac
if [ "$in_container" -eq 1 ]; then
    # CI and the outer wrapper share this route. Refuse it on an ordinary host;
    # it must not become an accidental way to bypass the container boundary.
    # Source locks evolve independently of the image; offline preparation checks
    # that the image's verified inputs cover the current source dependencies.
    image_lock=/opt/nilo/environment/lock.json
    [ "${NILO_ISOLATED_SOURCE:-}" = "$repo" ] && \
        [ -r "$image_lock" ] || \
        bad '--in-container requires an isolated source tree inside the build environment'
fi
# Existing appliance lifecycle tests need observed kernel inventory. Supply it
# read-only through the container wrapper; this grants no engine or capabilities.
if [ -z "${NILO_KERNEL_MODULES_PATH:-}" ] && [ -d /lib/modules ]; then
    NILO_KERNEL_MODULES_PATH=/lib/modules
    export NILO_KERNEL_MODULES_PATH
fi
case "$command" in
    setup)
        mode=${mode:-all}
        case "$mode" in all|tools|container) ;; *) bad 'setup mode must be all, tools or container' ;; esac
        [ "$offline" -eq 0 ] || [ "$rebuild" -eq 0 ] || bad '--offline cannot rebuild from network inputs'
        [ "$install" -eq 0 ] || [ -n "$engine_archive" ] || bad '--install-engine requires --engine-archive-path'
        [ -z "$engine_archive" ] || [ "$install" -eq 1 ] || bad '--engine-archive-path requires --install-engine'
        [ "$downgrades" -eq 0 ] || [ "$install" -eq 1 ] || bad '--allow-engine-downgrades requires --install-engine'
        if [ "$mode" = tools ]; then
            [ "$rebuild$install$downgrades" = 000 ] && [ -z "$archive$engine_archive" ] || bad 'container options require setup all or container'
        fi
        # Validate the entire request before either bootstrap can mutate state.
        if [ "$mode" != container ]; then
            if [ "$offline" -eq 1 ]; then sh "$tools/bootstrap/bootstrap.sh" --offline; else sh "$tools/bootstrap/bootstrap.sh"; fi
        fi
        [ "$mode" != tools ] || exit 0
        set -- --engine "$engine"
        [ "$offline" -eq 0 ] || set -- "$@" --offline
        [ "$rebuild" -eq 0 ] || set -- "$@" --rebuild-from-pinned-inputs
        [ "$install" -eq 0 ] || set -- "$@" --install-engine --engine-archive-path "$engine_archive"
        [ "$downgrades" -eq 0 ] || set -- "$@" --allow-engine-downgrades
        [ -z "$archive" ] || set -- "$@" --archive-path "$archive"
        exec sh "$tools/bootstrap/bootstrap-build-environment.sh" "$@" ;;
    dev)
        [ "$mode" = backend ] || bad 'Linux dev uses backend; Windows dev start launches both servers'
        case "$data" in /*) ;; *) bad 'dev backend requires an absolute --data-dir' ;; esac
        data=$(realpath -m -- "$data")
        case "$data" in /mnt/*) bad 'simulation state must stay on the Linux filesystem' ;; esac
        case "$port" in ''|*[!0-9]*) bad 'backend port must be 1..65535' ;; esac
        [ "$port" -ge 1 ] && [ "$port" -le 65535 ] || bad 'backend port must be 1..65535'
        case "$scenario" in healthy|unavailable) ;; *) bad 'unknown simulation scenario' ;; esac
        cd "$repo/05_Software/core"
        exec sh "$tools/env/with-tools.sh" go run ./cmd/nilo serve --profile simulation \
            --data-dir "$data" --bind "127.0.0.1:$port" --scenario "$scenario" --development-administrator ;;
    doctor)
        mode=${mode:-all}
        case "$mode" in all|tools|container) ;; *) bad 'doctor mode must be all, tools or container' ;; esac
        [ "$mode" = container ] || sh "$tools/scripts/doctor.sh"
        [ "$mode" != tools ] || exit 0
        "$engine" info >/dev/null
        manifest="$tools/archives/build-environment/current.json"
        image=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["image"]["reference"])' "$manifest")
        expected=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["image"]["digest"])' "$manifest")
        if [ "$engine" = docker ]; then transport="docker-daemon:$image"; else transport="containers-storage:$image"; fi
        actual=$(skopeo inspect --format '{{.Digest}}' "$transport")
        [ "$actual" = "$expected" ] || { printf '%s\n' 'canonical image digest differs; run nilo-tools setup container' >&2; exit 1; }
        exec sh "$tools/scripts/verify-build-environment.sh" --engine "$engine" --image "$image" --inventory-only ;;
    test)
        mode=${mode:-offline}
        if [ "$in_container" -eq 1 ]; then
            [ "$mode" = offline ] || bad '--in-container is only valid for the offline test suite'
            exec sh "$tools/tests/build-environment/verify-offline.sh"
        fi
        case "$mode" in
            offline) exec sh "$tools/env/with-build-environment.sh" --engine "$engine" test ;;
            tools) exec sh "$tools/env/with-build-environment.sh" --engine "$engine" run python -m unittest discover -s 02_Tools/tests/software -p 'test_*.py' -v ;;
            network|packaging) [ "$engine" = docker ] || bad "$mode fixture requires Docker"; exec sh "$tools/tests/$mode/check.sh" ;;
            appliance) export NILO_APPLIANCE_ENGINE=$engine; exec sh "$tools/tests/appliance/check.sh" ;;
            *) bad 'test mode must be offline, tools, network, appliance or packaging' ;;
        esac ;;
    build)
        [ -z "$mode" ] || bad 'build takes options only'
        if [ "$in_container" -eq 1 ]; then exec sh "$tools/tests/build-environment/build-all.sh"; fi
        exec sh "$tools/env/with-build-environment.sh" --engine "$engine" build ;;
    package)
        case "$mode" in ubuntu-24.04-amd64|rpi-debian13-arm64) ;; *) bad 'package requires an exact target, release or verify' ;; esac
        exec sh "$tools/env/with-build-environment.sh" --engine "$engine" package "$mode" ;;
esac
