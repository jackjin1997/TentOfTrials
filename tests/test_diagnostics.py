"""Tests for build diagnostic metadata generation"""
import sys, os, json, tempfile, unittest
from pathlib import Path

_REPO = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(_REPO))

try:
    from build import build_diagnostic_report, write_diagnostic_report
    HAS_BUILD = True
except ImportError:
    HAS_BUILD = False

class TestDiagnosticMetadata(unittest.TestCase):
    def test_report_has_required_fields(self):
        if not HAS_BUILD: self.skipTest("build.py not importable")
        r = build_diagnostic_report([("test", True, 1.0, "ok", None)], "abc123")
        for key in ["generated_at", "commit", "total_modules", "passed", "failed", "modules"]:
            self.assertIn(key, r)

    def test_report_counts_pass_fail(self):
        if not HAS_BUILD: self.skipTest("build.py not importable")
        r = build_diagnostic_report([
            ("a", True, 1.0, "ok", None),
            ("b", False, 2.0, "fail", None),
        ], "abc123")
        self.assertEqual(r["passed"], 1)
        self.assertEqual(r["failed"], 1)
        self.assertEqual(r["total_modules"], 2)

    def test_report_module_status(self):
        if not HAS_BUILD: self.skipTest("build.py not importable")
        r = build_diagnostic_report([("test", True, 1.0, "ok", None)], "abc123")
        self.assertEqual(r["modules"][0]["status"], "PASS")
        self.assertIn("elapsed_seconds", r["modules"][0])

    def test_empty_results(self):
        if not HAS_BUILD: self.skipTest("build.py not importable")
        r = build_diagnostic_report([], "abc123")
        self.assertEqual(r["total_modules"], 0)

if __name__ == "__main__":
    unittest.main()
