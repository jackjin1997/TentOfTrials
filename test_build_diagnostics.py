#!/usr/bin/env python3
"""Tests for build.py diagnostic metadata generation.

Run with: python3 -m pytest test_build_diagnostics.py -v
"""

import json
import os
import tempfile
from pathlib import Path
from unittest.mock import patch

import pytest

# Import functions from build.py
from build import (
    build_diagnostic_report,
    collect_system_info,
    current_commit_id,
    diagnostic_paths_for_commit,
    split_diagnostic_logd,
    DIAGNOSTIC_CHUNK_SIZE,
)


# ---------------------------------------------------------------------------
# current_commit_id
# ---------------------------------------------------------------------------

class TestCurrentCommitId:
    def test_returns_8_hex_chars(self):
        commit_id = current_commit_id()
        assert len(commit_id) == 8
        assert all(c in "0123456789abcdef" for c in commit_id)

    def test_returns_nonzero_in_repo(self):
        # We're inside a git repo, so should get a real commit
        commit_id = current_commit_id()
        assert commit_id != "00000000"

    def test_returns_zero_outside_repo(self):
        with patch("subprocess.run") as mock_run:
            mock_run.return_value = mock_run
            mock_run.returncode = 1
            mock_run.stdout = ""
            result = current_commit_id()
            assert result == "00000000"


# ---------------------------------------------------------------------------
# diagnostic_paths_for_commit
# ---------------------------------------------------------------------------

class TestDiagnosticPathsForCommit:
    def test_returns_correct_paths(self, tmp_path):
        with patch("build.DIAGNOSTIC_DIR", tmp_path):
            logd, meta, commit = diagnostic_paths_for_commit()
            assert logd.name == f"build-{commit}.logd"
            assert meta.name == f"build-{commit}.json"
            assert logd.parent == tmp_path
            assert meta.parent == tmp_path

    def test_commit_id_matches_current_commit(self, tmp_path):
        with patch("build.DIAGNOSTIC_DIR", tmp_path):
            _, _, commit = diagnostic_paths_for_commit()
            assert commit == current_commit_id()

    def test_creates_diagnostic_dir(self, tmp_path):
        target = tmp_path / "new_subdir"
        with patch("build.DIAGNOSTIC_DIR", target):
            diagnostic_paths_for_commit()
            assert target.exists()


# ---------------------------------------------------------------------------
# build_diagnostic_report
# ---------------------------------------------------------------------------

class TestBuildDiagnosticReport:
    def _sample_results(self):
        return [
            ("market", True, 1.234, "build output", "/path/to/market"),
            ("backend", False, 5.678, "compile error", None),
        ]

    def test_successful_report_metadata(self):
        results = self._sample_results()
        report = build_diagnostic_report(results, "abc12345")

        assert report["commit"] == "abc12345"
        assert report["total_modules"] == 2
        assert report["passed"] == 1
        assert report["failed"] == 1
        assert report["diagnostic_logd"] is None
        assert report["diagnostic_logd_error"] is None
        assert report["chunked"] is False

    def test_module_result_summaries(self):
        results = self._sample_results()
        report = build_diagnostic_report(results, "abc12345")

        modules = report["modules"]
        assert len(modules) == 2

        assert modules[0]["name"] == "market"
        assert modules[0]["status"] == "PASS"
        assert modules[0]["elapsed_seconds"] == 1.234
        assert modules[0]["artifact"] == "/path/to/market"

        assert modules[1]["name"] == "backend"
        assert modules[1]["status"] == "FAIL"
        assert modules[1]["artifact"] is None

    def test_logd_path_single_file(self):
        results = self._sample_results()
        report = build_diagnostic_report(
            results, "abc12345",
            logd_relpaths=["diagnostic/build-abc12345.logd"],
            password="secret123",
        )

        assert report["diagnostic_logd"] == "diagnostic/build-abc12345.logd"
        assert report["password"] == "secret123"
        assert report["decrypt_command"] is not None
        assert "secret123" in report["decrypt_command"]

    def test_logd_path_chunked_multi_file(self):
        results = self._sample_results()
        paths = [
            "diagnostic/build-abc12345-part001.logd",
            "diagnostic/build-abc12345-part002.logd",
        ]
        report = build_diagnostic_report(
            results, "abc12345",
            logd_relpaths=paths,
            password="pw",
            chunked=True,
        )

        assert report["diagnostic_logd"] == paths
        assert report["chunked"] is True
        assert report["chunk_size_bytes"] == DIAGNOSTIC_CHUNK_SIZE

    def test_logd_error_populated(self):
        results = self._sample_results()
        report = build_diagnostic_report(
            results, "abc12345",
            logd_error="encryptly binary not found",
        )

        assert report["diagnostic_logd_error"] == "encryptly binary not found"
        assert report["diagnostic_logd"] is None

    def test_pr_note_present(self):
        results = self._sample_results()
        report = build_diagnostic_report(results, "abc12345")
        assert "pr_note" in report
        assert "diagnostic" in report["pr_note"].lower()

    def test_generated_at_is_iso_format(self):
        results = self._sample_results()
        report = build_diagnostic_report(results, "abc12345")
        # Should be parseable as ISO format
        from datetime import datetime
        datetime.fromisoformat(report["generated_at"].replace("Z", "+00:00"))

    def test_empty_results(self):
        report = build_diagnostic_report([], "00000000")
        assert report["total_modules"] == 0
        assert report["passed"] == 0
        assert report["failed"] == 0
        assert report["modules"] == []


# ---------------------------------------------------------------------------
# split_diagnostic_logd
# ---------------------------------------------------------------------------

class TestSplitDiagnosticLogd:
    def test_small_file_not_split(self, tmp_path):
        logd = tmp_path / "build-test.logd"
        logd.write_bytes(b"small content")

        chunks = split_diagnostic_logd(logd, chunk_size=1024)
        assert len(chunks) == 1
        assert chunks[0] == logd
        assert logd.exists()

    def test_large_file_split_into_chunks(self, tmp_path):
        logd = tmp_path / "build-test.logd"
        # Write 3 chunks worth of data
        data = b"x" * (DIAGNOSTIC_CHUNK_SIZE * 3 + 100)
        logd.write_bytes(data)

        chunks = split_diagnostic_logd(logd, chunk_size=DIAGNOSTIC_CHUNK_SIZE)
        assert len(chunks) == 4  # 3 full + 1 partial
        assert not logd.exists()  # original removed

        # Verify chunk naming
        for i, chunk in enumerate(chunks):
            assert f"-part{i+1:03d}.logd" in chunk.name

    def test_chunk_content_matches_original(self, tmp_path):
        logd = tmp_path / "build-test.logd"
        data = b"A" * 100 + b"B" * 100 + b"C" * 100
        logd.write_bytes(data)

        chunks = split_diagnostic_logd(logd, chunk_size=100)
        assert len(chunks) == 3

        reassembled = b""
        for chunk in chunks:
            reassembled += chunk.read_bytes()
        assert reassembled == data

    def test_exact_chunk_size_boundary(self, tmp_path):
        logd = tmp_path / "build-test.logd"
        data = b"x" * 200
        logd.write_bytes(data)

        chunks = split_diagnostic_logd(logd, chunk_size=200)
        assert len(chunks) == 1
        assert chunks[0].read_bytes() == data


# ---------------------------------------------------------------------------
# collect_system_info
# ---------------------------------------------------------------------------

class TestCollectSystemInfo:
    def test_returns_nonempty_string(self):
        info = collect_system_info()
        assert isinstance(info, str)
        assert len(info) > 0

    def test_contains_expected_sections(self):
        info = collect_system_info()
        assert "Tent of Trials" in info
        assert "generated_at" in info
        assert "python" in info


# ---------------------------------------------------------------------------
# JSON report file format
# ---------------------------------------------------------------------------

class TestDiagnosticReportFileFormat:
    def test_report_is_valid_json(self, tmp_path):
        results = [("market", True, 1.0, "ok", None)]
        report = build_diagnostic_report(results, "deadbeef")

        out = tmp_path / "report.json"
        out.write_text(json.dumps(report, indent=2) + "\n")

        loaded = json.loads(out.read_text())
        assert loaded["commit"] == "deadbeef"
        assert loaded["modules"][0]["name"] == "market"

    def test_report_module_status_values(self):
        results = [
            ("mod_a", True, 0.1, "", None),
            ("mod_b", False, 0.2, "err", None),
        ]
        report = build_diagnostic_report(results, "00000000")
        statuses = [m["status"] for m in report["modules"]]
        assert statuses == ["PASS", "FAIL"]
