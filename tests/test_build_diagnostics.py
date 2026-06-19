import tempfile
import unittest
from pathlib import Path

import build


class DiagnosticReportTests(unittest.TestCase):
    def test_success_report_includes_commit_module_summary_and_logd(self):
        results = [
            ("backend", True, 1.23456, "ok", "backend/target/debug/backend"),
            ("frontend", False, 0.25, "npm failed", None),
        ]

        report = build.build_diagnostic_report(
            results,
            "abc123ef",
            logd_relpaths=["diagnostic/build-abc123ef.logd"],
            password="secret",  # noqa: S106 - benign test fixture value
        )

        self.assertEqual(report["commit"], "abc123ef")
        self.assertEqual(report["diagnostic_logd"], "diagnostic/build-abc123ef.logd")
        self.assertIsNone(report["diagnostic_logd_error"])
        self.assertFalse(report["chunked"])
        self.assertEqual(report["total_modules"], 2)
        self.assertEqual(report["passed"], 1)
        self.assertEqual(report["failed"], 1)
        self.assertEqual(
            report["decrypt_command"],
            "encryptly unpack diagnostic/build-abc123ef.logd <outdir> --password secret",
        )
        self.assertEqual(
            report["modules"],
            [
                {
                    "name": "backend",
                    "status": "PASS",
                    "elapsed_seconds": 1.235,
                    "artifact": "backend/target/debug/backend",
                    "output": "ok",
                },
                {
                    "name": "frontend",
                    "status": "FAIL",
                    "elapsed_seconds": 0.25,
                    "artifact": None,
                    "output": "npm failed",
                },
            ],
        )

    def test_logd_failure_report_records_error_without_archive(self):
        report = build.build_diagnostic_report(
            [("frailbox", False, 0, "make missing", None)],
            "deadbeef",
            logd_error="encryptly binary not found",
        )

        self.assertEqual(report["commit"], "deadbeef")
        self.assertIsNone(report["diagnostic_logd"])
        self.assertEqual(report["diagnostic_logd_error"], "encryptly binary not found")
        self.assertIsNone(report["password"])
        self.assertIsNone(report["decrypt_command"])
        self.assertIn("was not created", report["pr_note"])

    def test_chunked_report_lists_parts_and_reassembles_decrypt_target(self):
        report = build.build_diagnostic_report(
            [("market", True, 3.0, "built", "market/market")],
            "feedface",
            logd_relpaths=[
                "diagnostic/build-feedface-part001.logd",
                "diagnostic/build-feedface-part002.logd",
            ],
            password="pw",  # noqa: S106 - benign test fixture value
            chunked=True,
        )

        self.assertEqual(
            report["diagnostic_logd"],
            [
                "diagnostic/build-feedface-part001.logd",
                "diagnostic/build-feedface-part002.logd",
            ],
        )
        self.assertTrue(report["chunked"])
        self.assertEqual(report["chunk_size_bytes"], build.DIAGNOSTIC_CHUNK_SIZE)
        self.assertEqual(
            report["decrypt_command"].replace("\\", "/"),
            "encryptly unpack diagnostic/build-feedface.logd <outdir> --password pw",
        )


class DiagnosticLogdSplitTests(unittest.TestCase):
    def test_split_diagnostic_logd_keeps_small_file(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            logd_path = Path(tmpdir) / "build-abc123ef.logd"
            logd_path.write_bytes(b"small")

            chunks = build.split_diagnostic_logd(logd_path, chunk_size=10)

            self.assertEqual(chunks, [logd_path])
            self.assertEqual(logd_path.read_bytes(), b"small")

    def test_split_diagnostic_logd_creates_numbered_parts(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            logd_path = Path(tmpdir) / "build-abc123ef.logd"
            logd_path.write_bytes(b"abcdefghij")

            chunks = build.split_diagnostic_logd(logd_path, chunk_size=4)

            self.assertFalse(logd_path.exists())
            self.assertEqual([chunk.name for chunk in chunks], [
                "build-abc123ef-part001.logd",
                "build-abc123ef-part002.logd",
                "build-abc123ef-part003.logd",
            ])
            self.assertEqual(b"".join(chunk.read_bytes() for chunk in chunks), b"abcdefghij")


if __name__ == "__main__":
    unittest.main()
