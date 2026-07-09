"""
Tests for build.py diagnostic report generation.

Tests are deterministic and require no external toolchains or network access.
They import and call functions from build.py directly.

Usage:
    python3 -m pytest test_build_diagnostic.py -v
    # or
    python3 test_build_diagnostic.py
"""
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import build


class TestDiagnosticReport(unittest.TestCase):

    def test_metadata_shape(self):
        """Verify that a successful diagnostic report contains all required fields."""
        results = [
            ("backend", True, 12.345, "Build OK", "backend/target/debug/backend"),
            ("frontend", True, 8.210, "Build OK", "frontend/dist"),
            ("market", False, 0.500, "Go module not found", None),
        ]
        report = build.build_diagnostic_report(results, commit_id="a1b2c3d4")

        self.assertIn("generated_at", report)
        self.assertIn("commit", report)
        self.assertEqual(report["commit"], "a1b2c3d4")
        self.assertIn("diagnostic_logd", report)
        self.assertIn("diagnostic_logd_error", report)
        self.assertIn("chunked", report)
        self.assertIs(report["chunked"], False)
        self.assertIn("chunk_size_bytes", report)
        self.assertIsNone(report["chunk_size_bytes"])
        self.assertIn("password", report)
        self.assertIn("decrypt_command", report)
        self.assertIn("total_modules", report)
        self.assertEqual(report["total_modules"], 3)
        self.assertIn("passed", report)
        self.assertEqual(report["passed"], 2)
        self.assertIn("failed", report)
        self.assertEqual(report["failed"], 1)
        self.assertIn("modules", report)
        self.assertEqual(len(report["modules"]), 3)
        self.assertIn("pr_note", report)
        self.assertEqual(report["modules"][0]["name"], "backend")
        self.assertEqual(report["modules"][0]["status"], "PASS")
        self.assertEqual(report["modules"][1]["status"], "PASS")
        self.assertEqual(report["modules"][2]["status"], "FAIL")
        self.assertIsNone(report["modules"][2]["artifact"])

    def test_with_logd_path(self):
        """Verify diagnostic_logd field with a single logd path."""
        results = [("frailbox", True, 1.000, "Built", None)]
        report = build.build_diagnostic_report(
            results, commit_id="deadbeef",
            logd_relpaths=["diagnostic/build-deadbeef.logd"],
            password="testpw123",
        )
        self.assertEqual(report["diagnostic_logd"], "diagnostic/build-deadbeef.logd")
        self.assertIsNone(report["diagnostic_logd_error"])
        self.assertEqual(report["password"], "testpw123")
        self.assertIn("testpw123", report["decrypt_command"])
        self.assertIs(report["chunked"], False)

    def test_logd_error(self):
        """Verify that logd_error populates diagnostic_logd_error without claiming a valid archive."""
        results = [("backend", False, 0.100, "encryptly not found", None)]
        report = build.build_diagnostic_report(
            results, commit_id="00000000",
            logd_error="encryptly binary not found",
        )
        self.assertIsNone(report["diagnostic_logd"])
        self.assertEqual(report["diagnostic_logd_error"], "encryptly binary not found")
        self.assertIsNone(report["password"])
        self.assertIsNone(report["decrypt_command"])

    def test_chunked_logd(self):
        """Verify that chunked (multi-file) logd references are properly recorded."""
        results = [("backend", True, 5.000, "OK", "backend/target/debug/backend")]
        logd_files = [
            "diagnostic/build-chunk123-part001.logd",
            "diagnostic/build-chunk123-part002.logd",
            "diagnostic/build-chunk123-part003.logd",
        ]
        report = build.build_diagnostic_report(
            results, commit_id="chunk123",
            logd_relpaths=logd_files,
            password="chunkpw",
            chunked=True,
        )
        self.assertIsInstance(report["diagnostic_logd"], list)
        self.assertEqual(len(report["diagnostic_logd"]), 3)
        self.assertIs(report["chunked"], True)
        self.assertEqual(report["chunk_size_bytes"], build.DIAGNOSTIC_CHUNK_SIZE)
        self.assertEqual(report["password"], "chunkpw")
        self.assertIn("build-chunk123.logd", report["decrypt_command"])

    def test_module_details(self):
        """Verify each module entry contains name, status, elapsed, artifact, output."""
        results = [
            ("backend", True, 2.500, "All good", "backend/target/debug/backend"),
            ("frontend", False, 1.200, "npm install failed", None),
        ]
        report = build.build_diagnostic_report(results, commit_id="testmod")
        mods = report["modules"]
        self.assertEqual(len(mods), 2)
        self.assertEqual(mods[0]["name"], "backend")
        self.assertEqual(mods[0]["status"], "PASS")
        self.assertEqual(mods[0]["elapsed_seconds"], 2.5)
        self.assertEqual(mods[0]["artifact"], "backend/target/debug/backend")
        self.assertEqual(mods[0]["output"], "All good")
        self.assertEqual(mods[1]["name"], "frontend")
        self.assertEqual(mods[1]["status"], "FAIL")
        self.assertEqual(mods[1]["elapsed_seconds"], 1.2)
        self.assertIsNone(mods[1]["artifact"])
        self.assertEqual(mods[1]["output"], "npm install failed")

    def test_write_report_creates_file(self):
        """Verify write_diagnostic_report writes valid JSON to disk."""
        results = [("frailbox", True, 0.500, "Built", "frailbox/frailbox")]
        report = build.build_diagnostic_report(results, commit_id="write_test")
        tmppath = HERE / "_test_write_tmp.json"
        try:
            build.write_diagnostic_report(tmppath, report)
            self.assertTrue(tmppath.exists())
            with open(tmppath) as f:
                data = json.load(f)
            self.assertEqual(data["commit"], "write_test")
            self.assertEqual(data["passed"], 1)
            self.assertEqual(data["failed"], 0)
        finally:
            if tmppath.exists():
                tmppath.unlink()

    def test_empty_results(self):
        """Verify report works with empty results list (no modules)."""
        report = build.build_diagnostic_report([], commit_id="empty")
        self.assertEqual(report["total_modules"], 0)
        self.assertEqual(report["passed"], 0)
        self.assertEqual(report["failed"], 0)
        self.assertEqual(report["modules"], [])

    def test_pr_note(self):
        """Verify pr_note is present and mentions both logd and JSON."""
        results = [("backend", True, 1.0, "OK", None)]
        report = build.build_diagnostic_report(
            results, commit_id="prnote",
            logd_relpaths=["diagnostic/build-prnote.logd"],
        )
        note = report["pr_note"]
        self.assertIn("encrypted", note.lower())
        self.assertIn("JSON", note)

    def test_password_and_decrypt(self):
        """Verify password and decrypt_command are present when logd is available."""
        results = [("backend", True, 2.0, "OK", None)]
        report = build.build_diagnostic_report(
            results, commit_id="pwtest",
            logd_relpaths=["diagnostic/build-pwtest.logd"],
            password="secret123",
        )
        self.assertEqual(report["password"], "secret123")
        self.assertIsNotNone(report["decrypt_command"])
        self.assertIn("secret123", report["decrypt_command"])


if __name__ == "__main__":
    unittest.main()