import unittest

import build


RESULTS = [
    ("backend", True, 1.2345, "cargo build finished", "backend/target/debug/backend"),
    ("frontend", False, 0.5, "npm run build failed", None),
]


class BuildDiagnosticReportTests(unittest.TestCase):
    def test_successful_logd_metadata_includes_decrypt_details(self):
        report = build.build_diagnostic_report(
            RESULTS,
            "1a2b3c4d",
            logd_relpaths=["diagnostic/build-1a2b3c4d.logd"],
            password="secret-password",
        )

        self.assertEqual(report["commit"], "1a2b3c4d")
        self.assertEqual(report["diagnostic_logd"], "diagnostic/build-1a2b3c4d.logd")
        self.assertIsNone(report["diagnostic_logd_error"])
        self.assertFalse(report["chunked"])
        self.assertIsNone(report["chunk_size_bytes"])
        self.assertEqual(report["password"], "secret-password")
        self.assertEqual(
            report["decrypt_command"],
            "encryptly unpack diagnostic/build-1a2b3c4d.logd <outdir> --password secret-password",
        )
        self.assertEqual(report["total_modules"], 2)
        self.assertEqual(report["passed"], 1)
        self.assertEqual(report["failed"], 1)
        self.assertEqual(
            report["modules"],
            [
                {
                    "name": "backend",
                    "status": "PASS",
                    "elapsed_seconds": 1.234,
                    "artifact": "backend/target/debug/backend",
                    "output": "cargo build finished",
                },
                {
                    "name": "frontend",
                    "status": "FAIL",
                    "elapsed_seconds": 0.5,
                    "artifact": None,
                    "output": "npm run build failed",
                },
            ],
        )
        self.assertIn("diagnostic/build-1a2b3c4d.logd", report["pr_note"])

    def test_logd_creation_failure_metadata_preserves_error(self):
        report = build.build_diagnostic_report(
            RESULTS,
            "1a2b3c4d",
            logd_error="encryptly binary not found",
        )

        self.assertIsNone(report["diagnostic_logd"])
        self.assertEqual(report["diagnostic_logd_error"], "encryptly binary not found")
        self.assertIsNone(report["password"])
        self.assertIsNone(report["decrypt_command"])
        self.assertFalse(report["chunked"])
        self.assertIsNone(report["chunk_size_bytes"])
        self.assertIn("was not created", report["pr_note"])

    def test_chunked_logd_metadata_references_all_parts_and_reassembly_target(self):
        report = build.build_diagnostic_report(
            RESULTS,
            "1a2b3c4d",
            logd_relpaths=[
                "diagnostic/build-1a2b3c4d-part001.logd",
                "diagnostic/build-1a2b3c4d-part002.logd",
            ],
            password="secret-password",
            chunked=True,
        )

        self.assertEqual(
            report["diagnostic_logd"],
            [
                "diagnostic/build-1a2b3c4d-part001.logd",
                "diagnostic/build-1a2b3c4d-part002.logd",
            ],
        )
        self.assertTrue(report["chunked"])
        self.assertEqual(report["chunk_size_bytes"], build.DIAGNOSTIC_CHUNK_SIZE)
        self.assertEqual(
            report["decrypt_command"],
            "encryptly unpack diagnostic/build-1a2b3c4d.logd <outdir> --password secret-password",
        )
        self.assertIn("diagnostic/build-1a2b3c4d-part001.logd", report["pr_note"])
        self.assertIn("diagnostic/build-1a2b3c4d-part002.logd", report["pr_note"])


if __name__ == "__main__":
    unittest.main()
