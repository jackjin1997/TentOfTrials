#!/usr/bin/env python3
"""
Test build diagnostic metadata generation.

Covers successful report metadata, logd generation failure,
and chunked/multi-file logd references.
"""

import json
import os
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
ROOT = Path(__file__).resolve().parent

# Import build_diagnostic_report from build.py
import importlib.util
spec = importlib.util.spec_from_file_location("build_module", str(ROOT.parent / "build.py"))
build_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(build_module)
build_diagnostic_report = build_module.build_diagnostic_report


def make_sample_results():
    """Create sample build results for testing."""
    return [
        ("backend", True, 12.5, "Build completed successfully", "target/debug/backend"),
        ("frontend", True, 8.2, "Build completed", "dist/"),
        ("market", False, 0.5, "Go module not found: golang.org/x/net", None),
    ]


def test_successful_report_metadata():
    """Cover successful report metadata including commit id and module summaries."""
    results = make_sample_results()
    report = build_diagnostic_report(results, "abcdef12")

    assert report["commit"] == "abcdef12"
    assert report["total_modules"] == 3
    assert report["passed"] == 2
    assert report["failed"] == 1
    assert report["chunked"] is False
    assert report["chunk_size_bytes"] is None

    # Check module summaries
    assert len(report["modules"]) == 3
    assert report["modules"][0]["name"] == "backend"
    assert report["modules"][0]["status"] == "PASS"
    assert report["modules"][0]["elapsed_seconds"] == 12.5
    assert report["modules"][1]["name"] == "frontend"
    assert report["modules"][1]["status"] == "PASS"
    assert report["modules"][2]["name"] == "market"
    assert report["modules"][2]["status"] == "FAIL"

    # Check diagnostic_logd is None when not provided
    assert report["diagnostic_logd"] is None
    assert report["diagnostic_logd_error"] is None

    print(f"    ✓ Successful report: {report['passed']} passed, {report['failed']} failed")


def test_logd_single_path():
    """Cover single-file diagnostic_logd reference."""
    results = make_sample_results()
    report = build_diagnostic_report(
        results, "abcdef12",
        logd_relpaths=["diagnostic/build-abcdef12.logd"],
    )
    assert report["diagnostic_logd"] == "diagnostic/build-abcdef12.logd"
    assert isinstance(report["diagnostic_logd"], str)
    print(f"    ✓ Single logd path: {report['diagnostic_logd']}")


def test_logd_chunked_paths():
    """Cover chunked multi-file logd references."""
    results = make_sample_results()
    chunks = [
        "diagnostic/build-abcdef12-part001.logd",
        "diagnostic/build-abcdef12-part002.logd",
        "diagnostic/build-abcdef12-part003.logd",
    ]
    report = build_diagnostic_report(
        results, "abcdef12",
        logd_relpaths=chunks,
        chunked=True,
    )
    assert isinstance(report["diagnostic_logd"], list)
    assert len(report["diagnostic_logd"]) == 3
    assert report["chunked"] is True
    assert report["chunk_size_bytes"] is not None
    print(f"    ✓ Chunked logd: {len(report['diagnostic_logd'])} parts")


def test_logd_generation_failure():
    """Cover logd generation failure populating diagnostic_logd_error."""
    results = make_sample_results()
    report = build_diagnostic_report(
        results, "abcdef12",
        logd_error="encryptly binary not found (detected macos-arm64); cannot create build-abcdef12.logd",
    )
    assert report["diagnostic_logd"] is None
    assert report["diagnostic_logd_error"] is not None
    assert "encryptly" in report["diagnostic_logd_error"]
    assert report["password"] is None
    print(f"    ✓ Failure error: {report['diagnostic_logd_error'][:50]}...")


def test_password_and_decrypt_command():
    """Cover password and decrypt_command in report."""
    results = make_sample_results()
    report = build_diagnostic_report(
        results, "abcdef12",
        logd_relpaths=["diagnostic/build-abcdef12.logd"],
        password="s3cur3-p4ssw0rd-12345",
    )
    assert report["password"] == "s3cur3-p4ssw0rd-12345"
    assert report["decrypt_command"] is not None
    assert "encryptly unpack" in report["decrypt_command"]
    print(f"    ✓ Password and decrypt command present")


def test_mixed_success_failure_results():
    """Cover report with all-failed and all-passed scenarios."""
    results_all_pass = [
        ("mod_a", True, 1.0, "ok", "bin/a"),
        ("mod_b", True, 2.0, "ok", "bin/b"),
    ]
    report = build_diagnostic_report(results_all_pass, "aaaa0000")
    assert report["passed"] == 2
    assert report["failed"] == 0

    results_all_fail = [
        ("mod_a", False, 0.1, "error", None),
        ("mod_b", False, 0.2, "error", None),
    ]
    report = build_diagnostic_report(results_all_fail, "ffff0000")
    assert report["passed"] == 0
    assert report["failed"] == 2

    print(f"    ✓ All-pass: {report['passed']} passed, All-fail: mixed")


def test_pr_note_content():
    """Cover pr_note field in both success and failure scenarios."""
    results = make_sample_results()

    # With logd
    report = build_diagnostic_report(
        results, "abcdef12",
        logd_relpaths=["diagnostic/build-abcdef12.logd"],
    )
    assert "Include the encrypted" in report["pr_note"]
    assert "diagnostic/build-abcdef12.logd" in report["pr_note"]

    # Without logd
    report = build_diagnostic_report(results, "abcdef12")
    assert "Encrypted diagnostic logd artifact was not created" in report["pr_note"]

    print(f"    ✓ pr_note content varies by context")


def main():
    print("=" * 60)
    print("Build Diagnostic Metadata Tests")
    print("=" * 60)

    tests = [
        ("Successful report metadata", test_successful_report_metadata),
        ("Single logd path", test_logd_single_path),
        ("Chunked logd paths", test_logd_chunked_paths),
        ("Logd generation failure", test_logd_generation_failure),
        ("Password and decrypt command", test_password_and_decrypt_command),
        ("Mixed success/failure", test_mixed_success_failure_results),
        ("PR note content", test_pr_note_content),
    ]

    failures = 0
    for label, test in tests:
        print(f"\n  {label}:")
        try:
            test()
        except Exception as e:
            import traceback
            print(f"    FAILED: {e}")
            traceback.print_exc()
            failures += 1

    print(f"\n  {'=' * 50}")
    total = len(tests)
    passed = total - failures
    print(f"  Results: {total} tests, {passed} passed, {failures} failed")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())