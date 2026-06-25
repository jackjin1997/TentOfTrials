#!/usr/bin/env python3
"""Tests for build diagnostic metadata generation."""
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch, MagicMock

# Add build.py to path
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
from build import (
    build_diagnostic_report,
    diagnostic_paths_for_commit,
    current_commit_id,
    DIAGNOSTIC_CHUNK_SIZE,
)


class TestCurrentCommitId(unittest.TestCase):
    def test_returns_8_char_hex(self):
        """Commit ID should be 8 hex characters."""
        commit_id = current_commit_id()
        self.assertEqual(len(commit_id), 8)
        self.assertTrue(all(c in "0123456789abcdef" for c in commit_id))

    @patch("subprocess.run")
    def test_fallback_on_error(self, mock_run):
        """Should return default on git error."""
        mock_run.return_value = MagicMock(returncode=1, stdout="")
        commit_id = current_commit_id()
        self.assertEqual(commit_id, "00000000")


class TestDiagnosticPaths(unittest.TestCase):
    def test_paths_contain_commit_id(self):
        """Diagnostic paths should contain the commit ID."""
        logd_path, metadata_path, commit_id = diagnostic_paths_for_commit()
        self.assertIn(commit_id, logd_path.name)
        self.assertIn(commit_id, metadata_path.name)
        self.assertTrue(logd_path.name.endswith(".logd"))
        self.assertTrue(metadata_path.name.endswith(".json"))


class TestBuildDiagnosticReport(unittest.TestCase):
    def test_successful_report_structure(self):
        """Report should have correct structure for successful build."""
        results = [
            ("backend", True, 5.2, "Build output", "/path/to/binary"),
            ("frontend", True, 3.1, "Build output", None),
        ]
        commit_id = "abc12345"
        
        report = build_diagnostic_report(results, commit_id)
        
        self.assertEqual(report["commit"], commit_id)
        self.assertIn("generated_at", report)
        self.assertEqual(report["total_modules"], 2)
        self.assertEqual(report["passed"], 2)
        self.assertEqual(report["failed"], 0)
        self.assertIsNone(report["diagnostic_logd"])
        self.assertIsNone(report["diagnostic_logd_error"])
        self.assertFalse(report["chunked"])
    
    def test_failed_report_structure(self):
        """Report should handle failed builds."""
        results = [
            ("backend", False, 2.0, "Build failed", None),
        ]
        commit_id = "def67890"
        
        report = build_diagnostic_report(results, commit_id)
        
        self.assertEqual(report["commit"], commit_id)
        self.assertEqual(report["total_modules"], 1)
        self.assertEqual(report["passed"], 0)
        self.assertEqual(report["failed"], 1)
    
    def test_report_with_logd(self):
        """Report should include logd path when provided."""
        results = [("backend", True, 1.0, "ok", None)]
        commit_id = "11223344"
        logd_relpaths = ["diagnostic/build-11223344.logd"]
        
        report = build_diagnostic_report(results, commit_id, logd_relpaths=logd_relpaths)
        
        self.assertEqual(report["diagnostic_logd"], "diagnostic/build-11223344.logd")
        self.assertIsNotNone(report["password"])
        self.assertIsNotNone(report["decrypt_command"])
    
    def test_report_with_chunked_logd(self):
        """Report should handle chunked logd files."""
        results = [("backend", True, 1.0, "ok", None)]
        commit_id = "55667788"
        logd_relpaths = [
            "diagnostic/build-55667788-part1.logd",
            "diagnostic/build-55667788-part2.logd",
        ]
        
        report = build_diagnostic_report(
            results, commit_id, 
            logd_relpaths=logd_relpaths, 
            chunked=True
        )
        
        self.assertIsInstance(report["diagnostic_logd"], list)
        self.assertEqual(len(report["diagnostic_logd"]), 2)
        self.assertTrue(report["chunked"])
        self.assertEqual(report["chunk_size_bytes"], DIAGNOSTIC_CHUNK_SIZE)
    
    def test_report_with_logd_error(self):
        """Report should populate diagnostic_logd_error on failure."""
        results = [("backend", True, 1.0, "ok", None)]
        commit_id = "99aabbcc"
        error = "encryptly binary not found"
        
        report = build_diagnostic_report(
            results, commit_id, 
            logd_error=error
        )
        
        self.assertEqual(report["diagnostic_logd_error"], error)
        self.assertIsNone(report["diagnostic_logd"])
    
    def test_report_pr_note_with_logd(self):
        """PR note should mention logd artifacts."""
        results = [("backend", True, 1.0, "ok", None)]
        logd_relpaths = ["diagnostic/build-abc.logd"]
        
        report = build_diagnostic_report(results, "abc", 
            logd_relpaths=logd_relpaths
        )
        
        self.assertIn("logd", report["pr_note"].lower())
    
    def test_report_pr_note_without_logd(self):
        """PR note should explain missing logd."""
        results = [("backend", True, 1.0, "ok", None)]
        
        report = build_diagnostic_report(results, "abc")
        
        self.assertIn("not created", report["pr_note"].lower())


class TestModuleResults(unittest.TestCase):
    def test_module_results_structure(self):
        """Module results should have correct fields."""
        results = [
            ("backend", True, 5.2, "output", "/path"),
            ("frontend", False, 1.0, "error", None),
        ]
        
        report = build_diagnostic_report(results, "test1234")
        
        self.assertEqual(len(report["modules"]), 2)
        
        backend = report["modules"][0]
        self.assertEqual(backend["name"], "backend")
        self.assertEqual(backend["status"], "PASS")
        self.assertEqual(backend["elapsed_seconds"], 5.2)
        
        frontend = report["modules"][1]
        self.assertEqual(frontend["name"], "frontend")
        self.assertEqual(frontend["status"], "FAIL")


if __name__ == "__main__":
    unittest.main()
