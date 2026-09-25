"""Validate target interpreter inputs before Camera bundle assembly."""

import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[2] / "scripts" / "build-camera-bundle.py"
SPEC = importlib.util.spec_from_file_location("camera_bundle_builder", SCRIPT)
assert SPEC and SPEC.loader
builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(builder)


def runtime_input(root: Path, architecture: str) -> None:
    (root / "bin").mkdir(parents=True, exist_ok=True)
    header = bytearray(20)
    header[:6] = b"\x7fELF\x02\x01"
    header[18:20] = {"arm64": 183, "amd64": 62}[architecture].to_bytes(2, "little")
    (root / "bin/python3").write_bytes(header)
    (root / "THIRD_PARTY_NOTICES.txt").write_text("Runtime licences\n", encoding="utf-8")
    files = [{"path": item.relative_to(root).as_posix(),
              "sha256": hashlib.sha256(item.read_bytes()).hexdigest(),
              "size": item.stat().st_size} for item in sorted(root.rglob("*")) if item.is_file()]
    (root / "runtime-manifest.json").write_text(
        json.dumps({"architecture": architecture,
                    "files": sorted(files, key=lambda item: item["path"])}), encoding="utf-8"
    )


class CameraBundleTests(unittest.TestCase):
    def test_runtime_copy_requires_matching_interpreter_and_notices(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            runtime_input(source, "arm64")
            target = root / "target"
            digest = builder.copy_target_runtime(source, target, "arm64")
            self.assertEqual(target.joinpath("bin/python3").read_bytes(),
                             source.joinpath("bin/python3").read_bytes())
            self.assertEqual(len(digest), 64)
            with self.assertRaisesRegex(ValueError, "architecture"):
                builder.copy_target_runtime(source, root / "wrong", "amd64")
            (source / "bin/python3").write_bytes(b"changed")
            with self.assertRaisesRegex(ValueError, "architecture"):
                builder.copy_target_runtime(source, root / "changed", "arm64")
            runtime_input(source, "arm64")
            (source / "THIRD_PARTY_NOTICES.txt").write_text("Changed licences\n", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "manifest"):
                builder.copy_target_runtime(source, root / "tampered", "arm64")
            (source / "THIRD_PARTY_NOTICES.txt").write_text("", encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "notices"):
                builder.copy_target_runtime(source, root / "missing-notices", "arm64")
