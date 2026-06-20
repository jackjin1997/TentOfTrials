"""
Unit tests for build diagnostic metadata reports.
"""

import sys
import os
import unittest

# Add parent directory to path so we can import build
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
import build

class TestDiagnosticReport(unittest.TestCase):
    """
    Test suite for build diagnostic report generation functionality.
    """
    def test_successful_report_metadata(self):
        """
        Verify metadata structure and calculations for a successful diagnostic report.
        """
        results = [
            ("backend", True, 1.25, "Cargo build complete", "target/release/backend"),
            ("frontend", False, 0.45, "npm install failed", None)
        ]
        commit_id = "abcd1234ef"
        logd_relpaths = ["diagnostic/build-abcd1234ef.logd"]
        password = "safe-password"
        
        report = build.build_diagnostic_report(
            results=results,
            commit_id=commit_id,
            logd_relpaths=logd_relpaths,
            password=password,
            chunked=False
        )
        
        self.assertEqual(report["commit"], commit_id)
        self.assertEqual(report["diagnostic_logd"], "diagnostic/build-abcd1234ef.logd")
        self.assertIsNone(report["diagnostic_logd_error"])
        self.assertEqual(report["password"], password)
        self.assertFalse(report["chunked"])
        self.assertEqual(report["total_modules"], 2)
        self.assertEqual(report["passed"], 1)
        self.assertEqual(report["failed"], 1)
        self.assertEqual(len(report["modules"]), 2)
        self.assertEqual(report["modules"][0]["name"], "backend")
        self.assertEqual(report["modules"][0]["status"], "PASS")
        self.assertEqual(report["modules"][1]["name"], "frontend")
        self.assertEqual(report["modules"][1]["status"], "FAIL")

    def test_logd_generation_failure_metadata(self):
        """
        Verify metadata structure when logd generation fails.
        """
        results = [
            ("backend", True, 0.5, "Success", "bin/backend")
        ]
        commit_id = "abcd1234ef"
        logd_error = "encryptly binary not found"
        
        report = build.build_diagnostic_report(
            results=results,
            commit_id=commit_id,
            logd_relpaths=None,
            password=None,
            logd_error=logd_error,
            chunked=False
        )
        
        self.assertEqual(report["commit"], commit_id)
        self.assertIsNone(report["diagnostic_logd"])
        self.assertEqual(report["diagnostic_logd_error"], logd_error)
        self.assertIsNone(report["password"])
        self.assertIsNone(report["decrypt_command"])

    def test_chunked_logd_references(self):
        """
        Verify metadata fields when multiple chunked logd references are returned.
        """
        results = [
            ("backend", True, 0.5, "Success", "bin/backend")
        ]
        commit_id = "abcd1234ef"
        logd_relpaths = ["diagnostic/build-abcd-part001.logd", "diagnostic/build-abcd-part002.logd"]
        password = "safe-password"
        
        report = build.build_diagnostic_report(
            results=results,
            commit_id=commit_id,
            logd_relpaths=logd_relpaths,
            password=password,
            chunked=True
        )
        
        self.assertEqual(report["commit"], commit_id)
        self.assertEqual(report["diagnostic_logd"], logd_relpaths)
        self.assertEqual(report["chunk_size_bytes"], build.DIAGNOSTIC_CHUNK_SIZE)
        self.assertTrue(report["chunked"])
        self.assertEqual(report["password"], password)

if __name__ == '__main__':
    unittest.main()
