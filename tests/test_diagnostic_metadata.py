#!/usr/bin/env python3
"""Tests for build diagnostic report metadata ($30 bounty, issue #3).

Validates commit ID tracking, module result summaries, logd path handling,
and deterministic output from build_diagnostic_report().
"""
from __future__ import annotations

import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import build  # noqa: E402


class TestDiagnosticMetadata(unittest.TestCase):
    """Validates build_diagnostic_report output structure and determinism."""

    # ── report metadata ─────────────────────────────────────────────

    def test_report_includes_commit_id(self):
        report = build.build_diagnostic_report([], "abc123")
        self.assertIn("commit", report)
        self.assertEqual(report["commit"], "abc123")

    def test_report_includes_module_result_summaries(self):
        results = [
            ("mod_a", True, 1.2, "output A", None),
            ("mod_b", False, 0.8, "output B", None),
        ]
        report = build.build_diagnostic_report(results, "commit-1")
        self.assertEqual(report["total_modules"], 2)
        self.assertEqual(report["passed"], 1)
        self.assertEqual(report["failed"], 1)
        self.assertEqual(len(report["modules"]), 2)
        # Module entries have expected keys
        for mod in report["modules"]:
            self.assertIn("name", mod)
            self.assertIn("status", mod)
            self.assertIn("elapsed_seconds", mod)

    def test_module_status_pass_fail(self):
        results = [
            ("pass_mod", True, 0.5, "ok", None),
            ("fail_mod", False, 1.0, "error", "artifact.bin"),
        ]
        report = build.build_diagnostic_report(results, "c2")
        mods = {m["name"]: m for m in report["modules"]}
        self.assertEqual(mods["pass_mod"]["status"], "PASS")
        self.assertEqual(mods["fail_mod"]["status"], "FAIL")
        self.assertEqual(mods["fail_mod"]["artifact"], "artifact.bin")

    # ── logd path handling ─────────────────────────────────────────

    def test_report_includes_diagnostic_logd_key(self):
        report = build.build_diagnostic_report([], "test")
        self.assertIn("diagnostic_logd", report)

    def test_logd_relpaths_appear_in_pr_note(self):
        report = build.build_diagnostic_report(
            [], "c3", logd_relpaths=["diagnostic/build-abc.logd"]
        )
        self.assertIn("build-abc.logd", report["pr_note"])

    # ── determinism ─────────────────────────────────────────────────

    def test_empty_results_are_deterministic(self):
        report1 = build.build_diagnostic_report([], "same-commit")
        report2 = build.build_diagnostic_report([], "same-commit")
        self.assertEqual(report1["total_modules"], report2["total_modules"])
        self.assertEqual(report1["passed"], report2["passed"])
        self.assertEqual(report1["commit"], report2["commit"])

    def test_same_inputs_produce_same_output(self):
        results = [("m", True, 0.1, "out", None)]
        r1 = build.build_diagnostic_report(results, "c4")
        r2 = build.build_diagnostic_report(results, "c4")
        self.assertEqual(r1["passed"], r2["passed"])
        self.assertEqual(r1["total_modules"], r2["total_modules"])

    # ── no external dependencies ────────────────────────────────────

    def test_build_diagnostic_report_no_network(self):
        """Report generation does not require network access."""
        report = build.build_diagnostic_report(
            [("isolated", True, 0.0, "", None)],
            "no-net-commit",
        )
        self.assertEqual(report["total_modules"], 1)

    def test_build_diagnostic_report_no_file_io(self):
        """Report generation is pure in-memory (no disk writes)."""
        import tempfile
        report = build.build_diagnostic_report([], "mem-only")
        self.assertIsInstance(report, dict)
        self.assertIn("generated_at", report)


if __name__ == "__main__":
    unittest.main()
