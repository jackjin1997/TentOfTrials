 ```diff
--- a/tools/verify_backup.py
+++ b/tools/verify_backup.py
@@ -0,0 +1,264 @@
+#!/usr/bin/env python3
+"""
+Backup verification helper for TentOfTrials.
+
+Validates a restored database snapshot or exported metadata against
+expected table names and row counts. Reports missing tables and
+row-count mismatches with a non-zero exit code.
+"""
+
+import argparse
+import json
+import os
+import sqlite3
+import sys
+from dataclasses import dataclass
+from pathlib import Path
+from typing import Optional
+
+
+@dataclass
+class VerificationResult:
+    """Result of verifying a single table."""
+    table_name: str
+    expected_count: int
+    actual_count: int
+    exists: bool
+
+    @property
+    def is_valid(self) -> bool:
+        return self.exists and self.expected_count == self.actual_count
+
+
+@dataclass
+class VerificationReport:
+    """Complete verification report."""
+    results: list[VerificationResult]
+    missing_tables: list[str]
+    count_mismatches: list[VerificationResult]
+
+    @property
+    def is_valid(self) -> bool:
+        return len(self.missing_tables) == 0 and len(self.count_mismatches) == 0
+
+    def summary(self) -> str:
+        lines = [
+            "=" * 60,
+            "BACKUP VERIFICATION REPORT",
+            "=" * 60,
+            f"Total tables checked: {len(self.results)}",
+            f"Missing tables: {len(self.missing_tables)}",
+            f"Row-count mismatches: {len(self.count_mismatches)}",
+            f"Overall status: {'PASS' if self.is_valid else 'FAIL'}",
+            "-" * 60,
+        ]
+
+        if self.missing_tables:
+            lines.append("MISSING TABLES:")
+            for table in self.missing_tables:
+                lines.append(f"  - {table}")
+
+        if self.count_mismatches:
+            lines.append("ROW-COUNT MISMATCHES:")
+            for result in self.count_mismatches:
+                lines.append(
+                    f"  - {result.table_name}: "
+                    f"expected {result.expected_count}, got {result.actual_count}"
+                )
+
+        lines.append("=" * 60)
+        return "\n".join(lines)
+
+
+def load_expected_counts(path: Path) -> dict[str, int]:
+    """Load expected table counts from a JSON file."""
+    with open(path, "r", encoding="utf-8") as f:
+        data = json.load(f)
+    return data
+
+
+def verify_sqlite(db_path: Path, expected: dict[str, int]) -> VerificationReport:
+    """Verify a SQLite database against expected table counts."""
+    results: list[VerificationResult] = []
+    missing_tables: list[str] = []
+    count_mismatches: list[VerificationResult] = []
+
+    conn = sqlite3.connect(str(db_path))
+    cursor = conn.cursor()
+
+    # Get list of actual tables
+    cursor.execute("SELECT name FROM sqlite_master WHERE type='table'")
+    actual_tables = {row[0] for row in cursor.fetchall()}
+
+    for table_name, expected_count in expected.items():
+        if table_name not in actual_tables:
+            result = VerificationResult(
+                table_name=table_name,
+                expected_count=expected_count,
+                actual_count=0,
+                exists=False,
+            )
+            results.append(result)
+            missing_tables.append(table_name)
+        else:
+            cursor.execute(f"SELECT COUNT(*) FROM {table_name}")
+            actual_count = cursor.fetchone()[0]
+            result = VerificationResult(
+                table_name=table_name,
+                expected_count=expected_count,
+                actual_count=actual_count,
+                exists=True,
+            )
+            results.append(result)
+            if actual_count != expected_count:
+                count_mismatches.append(result)
+
+    conn.close()
+
+    return VerificationReport(
+        results=results,
+        missing_tables=missing_tables,
+        count_mismatches=count_mismatches,
+    )
+
+
+def verify_json_snapshot(snapshot_path: Path, expected: dict[str, int]) -> VerificationReport:
+    """Verify a JSON snapshot file against expected table counts."""
+    with open(snapshot_path, "r", encoding="utf-8") as f:
+        snapshot = json.load(f)
+
+    results: list[VerificationResult] = []
+    missing_tables: list[str] = []
+    count_mismatches: list[VerificationResult] = []
+
+    for table_name, expected_count in expected.items():
+        if table_name not in snapshot:
+            result = VerificationResult(
+                table_name=table_name,
+                expected_count=expected_count,
+                actual_count=0,
+                exists=False,
+            )
+            results.append(result)
+            missing_tables.append(table_name)
+        else:
+            actual_count = snapshot[table_name]
+            result = VerificationResult(
+                table_name=table_name,
+                expected_count=expected_count,
+                actual_count=actual_count,
+                exists=True,
+            )
+            results.append(result)
+            if actual_count != expected_count:
+                count_mismatches.append(result)
+
+    return VerificationReport(
+        results=results,
+        missing_tables=missing_tables,
+        count_mismatches=count_mismatches,
+    )
+
+
+def generate_sample_input(output_path: Path) -> None:
+    """Generate a sample expected-counts JSON file for operators to use as a template."""
+    sample = {
+        "users": 15000,
+        "trades": 450000,
+        "orders": 89000,
+        "positions": 12000,
+        "market_data": 5000000,
+        "audit_log": 1000000,
+    }
+    with open(output_path, "w", encoding="utf-8") as f:
+        json.dump(sample, f, indent=2)
+    print(f"Sample input written to {output_path}")
+
+
+def main() -> int:
+    parser = argparse.ArgumentParser(
+        description="Verify a restored database backup against expected table counts."
+    )
+    parser.add_argument(
+        "--expected",
+        type=Path,
+        required=True,
+        help="Path to JSON file with expected table names and row counts.",
+    )
+    parser.add_argument(
+        "--db",
+        type=Path,
+        help="Path to SQLite database to verify.",
+    )
+    parser.add_argument(
+        "--snapshot",
+        type=Path,
+        help