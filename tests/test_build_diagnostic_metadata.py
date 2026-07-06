#!/usr/bin/env python3
import importlib.util
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
BUILD_PY = ROOT / "build.py"


def load_build_module():
    spec = importlib.util.spec_from_file_location("build_module_under_test", BUILD_PY)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def sample_results():
    return [
        ("backend", True, 1.25, "backend ok", "backend/target/debug/backend"),
        ("frontend", False, 2.50, "npm missing", None),
    ]


def test_successful_diagnostic_report_metadata():
    build = load_build_module()

    report = build.build_diagnostic_report(
        sample_results(),
        "abcdef12",
        logd_relpaths=["diagnostic/build-abcdef12.logd"],
        password="pw-test",
        logd_error=None,
        chunked=False,
    )

    assert report["commit"] == "abcdef12"
    assert report["diagnostic_logd"] == "diagnostic/build-abcdef12.logd"
    assert report["diagnostic_logd_error"] is None
    assert report["chunked"] is False
    assert report["chunk_size_bytes"] is None
    assert report["password"] == "pw-test"
    assert report["total_modules"] == 2
    assert report["passed"] == 1
    assert report["failed"] == 1

    assert len(report["modules"]) == 2

    assert report["modules"][0]["name"] == "backend"
    assert report["modules"][0]["status"] == "PASS"
    assert report["modules"][0]["elapsed_seconds"] == 1.25
    assert report["modules"][0]["artifact"] == "backend/target/debug/backend"
    assert report["modules"][0]["output"] == "backend ok"

    assert report["modules"][1]["name"] == "frontend"
    assert report["modules"][1]["status"] == "FAIL"
    assert report["modules"][1]["elapsed_seconds"] == 2.5
    assert report["modules"][1]["artifact"] is None
    assert report["modules"][1]["output"] == "npm missing"

    assert "encryptly unpack diagnostic/build-abcdef12.logd" in report["decrypt_command"]
    assert "--password pw-test" in report["decrypt_command"]
    assert "metadata" in report["pr_note"].lower()


def test_logd_generation_failure_report_does_not_claim_archive():
    build = load_build_module()

    report = build.build_diagnostic_report(
        sample_results(),
        "abcdef12",
        logd_relpaths=None,
        password=None,
        logd_error="encryptly missing",
        chunked=False,
    )

    assert report["commit"] == "abcdef12"
    assert report["diagnostic_logd"] is None
    assert report["diagnostic_logd_error"] == "encryptly missing"
    assert report["password"] is None
    assert report["decrypt_command"] is None
    assert report["chunked"] is False
    assert report["chunk_size_bytes"] is None
    assert report["total_modules"] == 2
    assert report["passed"] == 1
    assert report["failed"] == 1
    assert "was not created" in report["pr_note"]


def test_chunked_diagnostic_report_metadata():
    build = load_build_module()

    report = build.build_diagnostic_report(
        sample_results(),
        "abcdef12",
        logd_relpaths=[
            "diagnostic/build-abcdef12-part001.logd",
            "diagnostic/build-abcdef12-part002.logd",
        ],
        password="pw-chunk",
        logd_error=None,
        chunked=True,
    )

    assert report["commit"] == "abcdef12"
    assert report["diagnostic_logd"] == [
        "diagnostic/build-abcdef12-part001.logd",
        "diagnostic/build-abcdef12-part002.logd",
    ]
    assert report["diagnostic_logd_error"] is None
    assert report["chunked"] is True
    assert report["chunk_size_bytes"] == build.DIAGNOSTIC_CHUNK_SIZE
    assert report["password"] == "pw-chunk"
    assert "encryptly unpack diagnostic/build-abcdef12.logd" in report["decrypt_command"]
    assert "--password pw-chunk" in report["decrypt_command"]


def test_split_diagnostic_logd_keeps_small_archive():
    build = load_build_module()

    with tempfile.TemporaryDirectory() as tmp:
        logd_path = Path(tmp) / "build-abcdef12.logd"
        logd_path.write_bytes(b"abcdef")

        parts = build.split_diagnostic_logd(logd_path, chunk_size=100)

        assert [p.name for p in parts] == ["build-abcdef12.logd"]
        assert logd_path.exists()
        assert logd_path.read_bytes() == b"abcdef"


def test_split_diagnostic_logd_splits_large_archive_and_removes_original():
    build = load_build_module()

    with tempfile.TemporaryDirectory() as tmp:
        logd_path = Path(tmp) / "build-abcdef12.logd"
        logd_path.write_bytes(b"abcdefghij")

        parts = build.split_diagnostic_logd(logd_path, chunk_size=4)

        assert [p.name for p in parts] == [
            "build-abcdef12-part001.logd",
            "build-abcdef12-part002.logd",
            "build-abcdef12-part003.logd",
        ]
        assert not logd_path.exists()
        assert [p.stat().st_size for p in parts] == [4, 4, 2]
        assert b"".join(p.read_bytes() for p in parts) == b"abcdefghij"


def run_all():
    tests = [
        test_successful_diagnostic_report_metadata,
        test_logd_generation_failure_report_does_not_claim_archive,
        test_chunked_diagnostic_report_metadata,
        test_split_diagnostic_logd_keeps_small_archive,
        test_split_diagnostic_logd_splits_large_archive_and_removes_original,
    ]

    for test in tests:
        test()
        print("PASS", test.__name__)

    print("ALL BUILD DIAGNOSTIC METADATA TESTS PASSED")


if __name__ == "__main__":
    run_all()
