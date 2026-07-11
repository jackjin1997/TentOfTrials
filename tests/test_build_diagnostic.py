"""Regression tests for build.py diagnostic JSON/logd metadata contract."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parent.parent
BUILD_PATH = ROOT / "build.py"


def load_build_module():
    spec = importlib.util.spec_from_file_location("build", BUILD_PATH)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules["build"] = module
    spec.loader.exec_module(module)
    return module


build = load_build_module()

SAMPLE_RESULTS = [
    ("backend", True, 1.234, "compiled ok", "backend/target/debug/backend"),
    ("frontend", False, 0.5, "npm ERR!", None),
]


def test_diagnostic_report_success_with_single_logd():
    logd = "diagnostic/build-deadbeef.logd"
    report = build.build_diagnostic_report(
        SAMPLE_RESULTS,
        "deadbeef",
        logd_relpaths=[logd],
        password="secret123",
    )

    assert report["commit"] == "deadbeef"
    assert report["diagnostic_logd"] == logd
    assert report["diagnostic_logd_error"] is None
    assert report["chunked"] is False
    assert report["password"] == "secret123"
    assert report["total_modules"] == 2
    assert report["passed"] == 1
    assert report["failed"] == 1
    assert len(report["modules"]) == 2
    assert report["modules"][0]["name"] == "backend"
    assert report["modules"][0]["status"] == "PASS"
    assert report["modules"][1]["status"] == "FAIL"
    assert report["decrypt_command"] == (
        f"encryptly unpack {logd} <outdir> --password secret123"
    )
    assert "encrypted diagnostic logd" in report["pr_note"].lower()


def test_diagnostic_report_logd_failure_sets_error_without_path():
    error = "encryptly binary not found; cannot create diagnostic/build-deadbeef.logd"
    report = build.build_diagnostic_report(SAMPLE_RESULTS, "deadbeef", logd_error=error)

    assert report["diagnostic_logd"] is None
    assert report["diagnostic_logd_error"] == error
    assert report["password"] is None
    assert report["decrypt_command"] is None
    assert "was not created" in report["pr_note"]


def test_diagnostic_report_chunked_logd_references():
    chunks = [
        "diagnostic/build-deadbeef-part001.logd",
        "diagnostic/build-deadbeef-part002.logd",
    ]
    report = build.build_diagnostic_report(
        SAMPLE_RESULTS,
        "deadbeef",
        logd_relpaths=chunks,
        password="chunkpw",
        chunked=True,
    )

    assert report["diagnostic_logd"] == chunks
    assert report["chunked"] is True
    assert report["chunk_size_bytes"] == build.DIAGNOSTIC_CHUNK_SIZE
    assert report["decrypt_command"].endswith("--password chunkpw")
    assert "build-deadbeef" in report["decrypt_command"]


def test_split_diagnostic_logd_keeps_small_file(tmp_path: Path):
    logd = tmp_path / "build-abcd1234.logd"
    logd.write_bytes(b"small diagnostic payload")

    chunks = build.split_diagnostic_logd(logd, chunk_size=1024)

    assert chunks == [logd]
    assert logd.exists()


def test_split_diagnostic_logd_splits_oversized_file(tmp_path: Path):
    logd = tmp_path / "build-abcd1234.logd"
    logd.write_bytes(b"x" * 2500)

    chunks = build.split_diagnostic_logd(logd, chunk_size=1024)

    assert len(chunks) == 3
    assert not logd.exists()
    assert [path.name for path in chunks] == [
        "build-abcd1234-part001.logd",
        "build-abcd1234-part002.logd",
        "build-abcd1234-part003.logd",
    ]
    assert sum(path.stat().st_size for path in chunks) == 2500


def test_diagnostic_paths_for_commit_uses_commit_prefix(tmp_path: Path, monkeypatch):
    monkeypatch.setattr(build, "ROOT", tmp_path)
    monkeypatch.setattr(build, "DIAGNOSTIC_DIR", tmp_path / "diagnostic")
    monkeypatch.setattr(build, "current_commit_id", lambda: "cafebabe")

    logd_path, metadata_path, commit_id = build.diagnostic_paths_for_commit()

    assert commit_id == "cafebabe"
    assert logd_path.name == "build-cafebabe.logd"
    assert metadata_path.name == "build-cafebabe.json"
    assert (tmp_path / "diagnostic").is_dir()
