#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# ---------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# ---------------------------------------------------------------------------
"""
@file    offline_ci.py
@brief   Run canonical offline validation from a Gitea job container.
@details Verify the accepted OCI digest, stream source into owned Docker volumes,
         collect evidence and remove this run's containers and volumes.
"""
from __future__ import annotations

# IMPORTS
import hashlib
import json
from pathlib import Path
import signal
import subprocess
import sys
import time
import uuid

# CONSTANTS
SKOPEO_IMAGE = 'quay.io/skopeo/stable@sha256:8b0da17eb9aeb16d677dca283193b7e6921c3e7c0b27b0844013b11fb69c8c21'


# LOCAL HELPERS
def docker(*arguments: str, capture: bool = False) -> str:
    """@brief Run a Docker command and reject a non-zero exit status."""
    result = subprocess.run(['docker', *arguments], text=True, capture_output=capture)
    if result.returncode:
        raise RuntimeError(f'Docker {arguments[0]} failed with exit {result.returncode}')
    return result.stdout.strip() if capture else ''


def cancelled(signum, frame):
    """@brief Convert runner cancellation into the normal cleanup path."""
    raise InterruptedError(f'CI interrupted by signal {signum}')


OFFLINE_COMMAND = r'''
set -eu
cp -R /opt/nilo/cache-seed/. /work/runtime-cache/
mkdir -p /work/source
tar -C /workspace -cf - \
  --exclude=./.git --exclude=./02_Tools/cache --exclude=./02_Tools/runtime \
  --exclude='*.part[0-9][0-9][0-9]' --exclude='*.oci.tar' --exclude='*.oci.tar.gz' \
  --exclude='nilo-wsl-host-bootstrap-*.tar.gz' --exclude=node_modules \
  --exclude=.venv --exclude='.venv-*' --exclude=dist --exclude=coverage \
  --exclude=__pycache__ . | tar -C /work/source -xf -
find /work/source -depth -type d -empty -delete
ln -s /workspace/.git /work/source/.git
cd /work/source
timeout --signal=TERM --kill-after=30s 45m ./02_Tools/tests/build-environment/verify-offline.sh
'''


def main() -> int:
    """
    @brief Execute the accepted suite and retain evidence on success or failure.
    @return The test container's exit code, or 1 for a controller failure.
    """
    root = Path(__file__).resolve().parents[2]
    evidence = root / '02_Tools/cache/ci'
    evidence.mkdir(parents=True, exist_ok=True)
    manifest = json.loads((root / '02_Tools/archives/build-environment/current.json').read_text())
    image = manifest['image']['reference']
    expected = manifest['image']['digest']
    identifier = 'nilo-ci-' + uuid.uuid4().hex
    source_volume, output_volume = identifier + '-source', identifier + '-outputs'
    seed, test = identifier + '-seed', identifier + '-test'
    owned_volumes, owned_containers = [], []
    record = {'image': image, 'expected_digest': expected, 'status': 'preflight', 'exit_code': None}
    start = time.monotonic()
    for signum in (signal.SIGINT, signal.SIGTERM):
        signal.signal(signum, cancelled)
    try:
        if manifest['verification']['status'] != 'local-round-trip-complete':
            raise RuntimeError('The selected archive has not completed offline round-trip verification.')
        # The connected controller verifies the accepted digest before the
        # capability-dropped offline test container is created.
        result = subprocess.run(['docker', 'run', '--rm', '--pull', 'never', '--network', 'none',
                                 '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
                                 '--memory', '512m', '--pids-limit', '128',
                                 '--mount', 'type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock',
                                 SKOPEO_IMAGE, 'inspect', '--format', '{{.Digest}}',
                                 'docker-daemon:' + image], text=True, capture_output=True)
        if result.returncode or result.stdout.strip() != expected:
            raise RuntimeError(f'Accepted image missing or stale: expected {expected}; provision it through the DCP Server CI setup.')
        record['actual_digest'] = result.stdout.strip()
        record['source_commit'] = subprocess.check_output(['git', '-C', str(root), 'rev-parse', 'HEAD'], text=True).strip()
        record['lock_sha256'] = hashlib.sha256((root / '02_Tools/containers/build/build-environment.lock.json').read_bytes()).hexdigest()
        for volume in (source_volume, output_volume):
            docker('volume', 'create', '--label', 'io.nilo.ci=' + identifier, volume, capture=True)
            owned_volumes.append(volume)
        docker('run', '-d', '--name', seed, '--network', 'none', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--memory', '128m', '--pids-limit', '32', '--user', '0:0', '--mount', f'type=volume,source={source_volume},target=/workspace', '--mount', f'type=volume,source={output_volume},target=/outputs', '--entrypoint', 'sh', image, '-c', 'chmod 1777 /outputs; sleep 3600', capture=True)
        owned_containers.append(seed)
        # A runner checkout lives inside the controller container. docker cp
        # streams it to a named volume; its path is never treated as a host path.
        docker('cp', str(root) + '/.', seed + ':/workspace')
        # UID 1000 is the image's named ubuntu account. Recovery tooling uses
        # id -un, so an arbitrary UID without a passwd entry is insufficient.
        docker('create', '--name', test, '--network', 'none', '--cap-drop', 'ALL',
               '--security-opt', 'no-new-privileges', '--cpus', '2',
               '--memory', '6g', '--memory-swap', '6g', '--pids-limit', '1024',
               '--user', '1000:1000',
               '--mount', f'type=volume,source={source_volume},target=/workspace,readonly',
               '--mount', f'type=volume,source={output_volume},target=/outputs',
               '--mount', 'type=bind,source=/lib/modules,target=/lib/modules,readonly',
               '-e', 'GIT_OPTIONAL_LOCKS=0', '-e', 'GIT_CONFIG_COUNT=1',
               '-e', 'GIT_CONFIG_KEY_0=safe.directory',
               '-e', 'GIT_CONFIG_VALUE_0=/work/source',
               '-e', 'NILO_ISOLATED_SOURCE=/work/source',
               '--entrypoint', 'bash', image, '-lc', OFFLINE_COMMAND, capture=True)
        owned_containers.append(test)
        record['status'] = 'running'
        with (evidence / 'offline.log').open('w', encoding='utf-8') as log:
            process = subprocess.Popen(['docker', 'start', '-a', test], stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
            assert process.stdout is not None
            for line in process.stdout:
                print(line, end='', flush=True)
                log.write(line)
            process.wait()
        state = json.loads(docker('inspect', test, capture=True))[0]['State']
        record.update(exit_code=state['ExitCode'], oom_killed=state['OOMKilled'], status='passed' if state['ExitCode'] == 0 else 'failed')
        docker('cp', seed + ':/outputs/.', str(evidence))
        return state['ExitCode']
    except (OSError, RuntimeError, InterruptedError, subprocess.SubprocessError) as error:
        record.update(status='failed', error=str(error))
        print(str(error), file=sys.stderr)
        return 1
    finally:
        record['duration_seconds'] = round(time.monotonic() - start, 2)
        (evidence / 'result.json').write_text(json.dumps(record, indent=2) + '\n')
        for container in reversed(owned_containers):
            subprocess.run(['docker', 'rm', '-f', container], stdout=subprocess.DEVNULL)
        for volume in reversed(owned_volumes):
            subprocess.run(['docker', 'volume', 'rm', volume], stdout=subprocess.DEVNULL)


if __name__ == '__main__':
    raise SystemExit(main())

# END OF FILE
