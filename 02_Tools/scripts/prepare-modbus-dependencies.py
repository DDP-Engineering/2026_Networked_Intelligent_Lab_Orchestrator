#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# -----------------------------------------------------------------------------
# Copyright (c) 2026 DDP Engineering
# This software is licensed under the PolyForm Noncommercial License 1.0.0.
# See LICENSE.txt in the repository root for full license details.
# SPDX-License-Identifier: LicenseRef-PolyForm-Noncommercial-1.0.0
# -----------------------------------------------------------------------------
"""@file prepare-modbus-dependencies.py
@brief Prepare exact Modbus candidate inputs and license notices.
@details Verify the transport wheel/source closure before offline bundle assembly.
"""

# IMPORTS
import argparse
import hashlib
import json
import re
import tarfile
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path

# CONSTANTS
REPO = Path(__file__).resolve().parents[2]
MANIFEST = REPO / "05_Software/integrations/modbus/evidence/dependencies.json"
MAX_ARCHIVE_BYTES = 10 << 20


# HELPER FUNCTIONS
def validate_pin(artifact):
    """@brief Admit immutable upstream filenames, HTTPS sources and SHA-256 pins."""
    url = urllib.parse.urlsplit(artifact["url"])
    filename = artifact["filename"]
    if (
        url.scheme != "https"
        or url.netloc != "files.pythonhosted.org"
        or url.query
        or url.fragment
        or Path(url.path).name != filename
        or Path(filename).name != filename
        or not re.fullmatch(r"[a-zA-Z0-9_.-]+", filename)
        or not re.fullmatch(r"[0-9a-f]{64}", artifact["sha256"])
    ):
        raise ValueError("invalid pinned source")


def verified_bytes(path, expected):
    """@brief Bound and hash one cached archive before use."""
    if path.stat().st_size > MAX_ARCHIVE_BYTES:
        raise ValueError("archive size exceeds bound")
    data = path.read_bytes()
    if hashlib.sha256(data).hexdigest() != expected:
        raise ValueError("dependency checksum mismatch")
    return data


def license_bytes(path, member):
    """@brief Read one pinned license member without filesystem extraction."""
    if path.name.endswith(".whl"):
        with zipfile.ZipFile(path) as archive:
            if archive.getinfo(member).file_size > 65536:
                raise ValueError("license size exceeds bound")
            return archive.read(member)
    with tarfile.open(path) as archive:
        entry = archive.getmember(member)
        if not entry.isfile() or entry.size > 65536:
            raise ValueError("license member must be a bounded regular file")
        return archive.extractfile(entry).read()


# PUBLIC API
def prepare(directory: Path, offline=False):
    """@brief Verify complete runtime wheel/source closure and retained licenses.
    @param[in] directory Explicit repository cache or isolated test directory.
    @param[in] offline Refuse network access and report missing pinned inputs.
    @return Verified artifact paths.
    @exception ValueError Missing, malformed, oversized or mismatched input.
    """
    pins = json.loads(MANIFEST.read_text())
    paths = []
    for package in pins["packages"]:
        for artifact in package["artifacts"]:
            validate_pin(artifact)
            path = directory / artifact["filename"]
            if not path.is_file():
                if offline:
                    raise ValueError("pinned dependency missing")
                with urllib.request.urlopen(artifact["url"], timeout=30) as response:
                    data = response.read(MAX_ARCHIVE_BYTES + 1)
                if (
                    len(data) > MAX_ARCHIVE_BYTES
                    or hashlib.sha256(data).hexdigest() != artifact["sha256"]
                ):
                    raise ValueError("downloaded dependency checksum mismatch")
                directory.mkdir(parents=True, exist_ok=True)
                temporary = path.with_suffix(path.suffix + ".download")
                temporary.write_bytes(data)
                temporary.replace(path)
            verified_bytes(path, artifact["sha256"])
            paths.append(path)
        notice = license_bytes(directory / package["license_artifact"], package["license_member"])
        if hashlib.sha256(notice).hexdigest() != package["license_sha256"]:
            raise ValueError("license checksum mismatch")
        target = directory / "licenses" / (package["name"] + "-LICENSE.txt")
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(notice)
    return paths


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=REPO / "02_Tools/cache/modbus-feasibility")
    parser.add_argument("--offline", action="store_true")
    options = parser.parse_args()
    # CLI acquisition stays in the ignored repository cache; tests use isolated directories.
    options.output.resolve().relative_to((REPO / "02_Tools/cache").resolve())
    for result in prepare(options.output, options.offline):
        print(result)

# END OF FILE
