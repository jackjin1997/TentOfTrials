"""Regression tests for build diagnostic metadata.

Run locally with:
    python -m unittest discover -s tests -v
"""

import unittest
from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

import build


class BuildDiagnosticReportTests(unittest.TestCase):
    def test_successful_report_includes_commit_modules_and_logd_path(self):
        results = [
            ("backend", True, 1.2345, "cargo build ok", "backend/target/debug/backend"),
            ("frontend", False, 0.25, "npm unavailable", None),
        ]

        report = build.build_diagnostic_report(
            results,
            "abc12345",
            logd_relpaths=["diagnostic/build-abc12345.logd"],
            password="test-password",
        )

        self.assertEqual(report["commit"], "abc12345")
        self.assertEqual(report["diagnostic_logd"], "diagnostic/build-abc12345.logd")
        self.assertIsNone(report["diagnostic_logd_error"])
        self.assertFalse(report["chunked"])
        self.assertEqual(report["total_modules"], 2)
        self.assertEqual(report["passed"], 1)
        self.assertEqual(report["failed"], 1)
        self.assertEqual(report["password"], "test-password")
        self.assertIn("diagnostic/build-abc12345.logd", report["decrypt_command"])
        self.assertEqual(report["modules"][0]["name"], "backend")
        self.assertEqual(report["modules"][0]["status"], "PASS")
        self.assertEqual(report["modules"][0]["artifact"], "backend/target/debug/backend")
        self.assertEqual(report["modules"][1]["name"], "frontend")
        self.assertEqual(report["modules"][1]["status"], "FAIL")

    def test_logd_failure_populates_error_without_claiming_archive(self):
        results = [("backend", False, 0.0, "cargo missing", None)]

        report = build.build_diagnostic_report(
            results,
            "def67890",
            logd_error="encryptly binary not found",
        )

        self.assertEqual(report["commit"], "def67890")
        self.assertIsNone(report["diagnostic_logd"])
        self.assertEqual(report["diagnostic_logd_error"], "encryptly binary not found")
        self.assertIsNone(report["password"])
        self.assertIsNone(report["decrypt_command"])
        self.assertIn("was not created", report["pr_note"])
        self.assertEqual(report["failed"], 1)

    def test_chunked_report_uses_multi_file_logd_references(self):
        results = [("backend", True, 2.0, "ok", "backend/target/debug/backend")]
        chunks = [
            "diagnostic/build-a1b2c3d4-part001.logd",
            "diagnostic/build-a1b2c3d4-part002.logd",
        ]

        report = build.build_diagnostic_report(
            results,
            "a1b2c3d4",
            logd_relpaths=chunks,
            password="chunk-password",
            chunked=True,
        )

        self.assertEqual(report["diagnostic_logd"], chunks)
        self.assertTrue(report["chunked"])
        self.assertEqual(report["chunk_size_bytes"], build.DIAGNOSTIC_CHUNK_SIZE)
        self.assertIn("build-a1b2c3d4.logd", report["decrypt_command"])
        self.assertIn(chunks[0], report["pr_note"])
        self.assertIn(chunks[1], report["pr_note"])


if __name__ == "__main__":
    unittest.main()
