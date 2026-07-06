"""Regression tests for build.py diagnostic metadata contract."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from build import (  # noqa: E402
    DIAGNOSTIC_CHUNK_SIZE,
    build_diagnostic_report,
    split_diagnostic_logd,
    write_diagnostic_report,
)

SAMPLE_RESULTS = [
    ("compliance", True, 1.25, "ok", "compliance/build"),
    ("frontend", False, 0.5, "compile error", None),
]


class BuildDiagnosticMetadataTests(unittest.TestCase):
    def test_successful_report_includes_commit_modules_and_logd(self) -> None:
        report = build_diagnostic_report(
            SAMPLE_RESULTS,
            "deadbeef",
            logd_relpaths=["diagnostic/build-deadbeef.logd"],
            password="pw123",
        )
        self.assertEqual(report["commit"], "deadbeef")
        self.assertEqual(report["diagnostic_logd"], "diagnostic/build-deadbeef.logd")
        self.assertIsNone(report["diagnostic_logd_error"])
        self.assertEqual(report["total_modules"], 2)
        self.assertEqual(report["passed"], 1)
        self.assertEqual(report["failed"], 1)
        self.assertEqual(len(report["modules"]), 2)
        self.assertIn("encryptly unpack", report["decrypt_command"] or "")

    def test_logd_error_populated_without_valid_archive(self) -> None:
        report = build_diagnostic_report(
            SAMPLE_RESULTS,
            "cafebabe",
            logd_error="encryptly binary not found",
        )
        self.assertIsNone(report["diagnostic_logd"])
        self.assertEqual(report["diagnostic_logd_error"], "encryptly binary not found")
        self.assertIn("not created", report["pr_note"].lower())

    def test_chunked_logd_references_list_and_decrypt_target(self) -> None:
        parts = [
            "diagnostic/build-abcd1234-part001.logd",
            "diagnostic/build-abcd1234-part002.logd",
        ]
        report = build_diagnostic_report(
            SAMPLE_RESULTS,
            "abcd1234",
            logd_relpaths=parts,
            password="secret",
            chunked=True,
        )
        self.assertEqual(report["diagnostic_logd"], parts)
        self.assertTrue(report["chunked"])
        self.assertEqual(report["chunk_size_bytes"], DIAGNOSTIC_CHUNK_SIZE)
        self.assertIn("build-abcd1234.logd", report["decrypt_command"] or "")
        self.assertIn("part001.logd", report["pr_note"])

    def test_split_diagnostic_logd_emits_numbered_chunks(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            logd = root / "build-test.logd"
            logd.write_bytes(b"x" * 100)
            chunks = split_diagnostic_logd(logd, chunk_size=40)
            self.assertEqual(len(chunks), 3)
            self.assertFalse(logd.exists())
            names = [c.name for c in chunks]
            self.assertEqual(names, ["build-test-part001.logd", "build-test-part002.logd", "build-test-part003.logd"])

    def test_write_diagnostic_report_persists_json(self) -> None:
        metadata_path = ROOT / "diagnostic" / "build-test-metadata.json"
        metadata_path.parent.mkdir(parents=True, exist_ok=True)
        report = build_diagnostic_report(SAMPLE_RESULTS, "deadbeef")
        try:
            write_diagnostic_report(metadata_path, report)
            loaded = json.loads(metadata_path.read_text(encoding="utf-8"))
            self.assertEqual(loaded["commit"], "deadbeef")
            self.assertEqual(loaded["modules"][0]["name"], "compliance")
        finally:
            if metadata_path.exists():
                metadata_path.unlink()


if __name__ == "__main__":
    unittest.main()
