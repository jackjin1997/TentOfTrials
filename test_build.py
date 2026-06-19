import unittest
import json
import os
from unittest.mock import patch
from build import current_commit_id, diagnostic_paths_for_commit, split_diagnostic_logd

class TestBuildDiagnosticMetadata(unittest.TestCase):
    def test_current_commit_id(self):
        commit_id = current_commit_id()
        self.assertIsNotNone(commit_id)
        self.assertEqual(len(commit_id), 8)

    def test_diagnostic_paths_for_commit(self):
        logd_path, metadata_path, commit_id = diagnostic_paths_for_commit()
        self.assertIsNotNone(logd_path)
        self.assertIsNotNone(metadata_path)
        self.assertEqual(len(commit_id), 8)

    def test_split_diagnostic_logd(self):
        logd_path = diagnostic_paths_for_commit()[0]
        chunks = split_diagnostic_logd(logd_path)
        self.assertEqual(len(chunks), 1)
        self.assertEqual(chunks[0], logd_path)

    @patch('build.subprocess.run')
    def test_logd_generation_failure(self, mock_run):
        mock_run.side_effect = Exception('Mocked exception')
        logd_path, metadata_path, commit_id = diagnostic_paths_for_commit()
        with open(metadata_path, 'r') as f:
            metadata = json.load(f)
        self.assertIn('diagnostic_logd_error', metadata)

    def test_chunked_logd_references(self):
        logd_path = diagnostic_paths_for_commit()[0]
        with open(logd_path, 'wb') as f:
            f.write(os.urandom(50 * 1024 * 1024))  # Create a large logd file
        chunks = split_diagnostic_logd(logd_path)
        self.assertGreater(len(chunks), 1)
        for chunk in chunks:
            self.assertTrue(os.path.exists(chunk))

if __name__ == '__main__':
    unittest.main()