 ```diff
--- a/tools/verify_backup.py
+++ b/tools/verify_backup.py
@@ -0,0 +1,293 @@
+#!/usr/bin/env python3
+"""
+Backup verification helper for TentOfTrials.
+
+Validates a restored database snapshot or exported metadata against expected
+table names and row counts. Reports missing tables and row-count mismatches
+with a non-zero exit code.
+
+Usage:
+    # Validate against a live database
+    python3 tools/verify_backup.py --expected expected.json --dsn "postgresql://user:pass@host/db"
+
+    # Dry-run / sample mode (no database credentials needed)
+    python3 tools/verify_backup.py --expected expected.json --dry-run
+
+    # Generate sample expected.json
+    python3 tools/verify_backup.py --generate-sample > sample_expected.json
+"""
+
+from __future__ import annotations
+
+import argparse
+import json
+import sys
+from dataclasses import dataclass, field
+from pathlib import Path
+from typing import Any, Optional
+
+
+@dataclass
+class VerificationResult:
+    """Result of verifying a single table."""
+    table_name: str
+    expected_count: int
+    actual_count: int = -1
+    found: bool = False
+
+    @property
+    def count_matches(self) -> bool:
+        return self.found and self.actual_count == self.expected_count
+
+    def __str__(self) -> str:
+        status = "✓" if self.count_matches else "✗"
+        if not self.found:
+            return f"{status} {self.table_name}: MISSING (expected {self.expected_count} rows)"
+        return f"{status} {self.table_name}: {self.actual_count} rows (expected {self.expected_count})"
+
+
+@dataclass
+class VerificationReport:
+    """Overall verification report."""
+    results: list[VerificationResult] = field(default_factory=list)
+    errors: list[str] = field(default_factory=list)
+
+    @property
+    def missing_tables(self) -> list[VerificationResult]:
+        return [r for r in self.results if not r.found]
+
+    @property
+    def mismatched_counts(self) -> list[VerificationResult]:
+        return [r for r in self.results if r.found and not r.count_matches]
+
+    @property
+    def is_valid(self) -> bool:
+        return not self.missing_tables and not self.mismatched_counts and not self.errors
+
+    def summary(self) -> str:
+        lines = [
+            f"Tables checked: {len(self.results)}",
+            f"Missing tables: {len(self.missing_tables)}",
+            f"Count mismatches: {len(self.mismatched_counts)}",
+            f"Errors: {len(self.errors)}",
+        ]
+        if self.is_valid:
+            lines.append("Result: PASS")
+        else:
+            lines.append("Result: FAIL")
+        return "\n".join(lines)
+
+
+def load_expected(path: str) -> dict[str, int]:
+    """Load expected table counts from a JSON file."""
+    with open(path, "r", encoding="utf-8") as f:
+        data = json.load(f)
+
+    # Support both flat {"table": count} and nested {"tables": {"table": count}} formats
+    if "tables" in data:
+        return {str(k): int(v) for k, v in data["tables"].items()}
+    return {str(k): int(v) for k, v in data.items()}
+
+
+def query_database_counts(dsn: str) -> dict[str, int]:
+    """Query a PostgreSQL database for table row counts."""
+    import psycopg2
+
+    counts: dict[str, int] = {}
+    with psycopg2.connect(dsn) as conn:
+        with conn.cursor() as cur:
+            # Get all user tables in public schema
+            cur.execute("""
+                SELECT tablename
+                FROM pg_tables
+                WHERE schemaname = 'public'
+                ORDER BY tablename
+            """)
+            tables = [row[0] for row in cur.fetchall()]
+
+            for table in tables:
+                cur.execute(f"SELECT COUNT(*) FROM {table}")
+                count = cur.fetchone()[0]
+                counts[table] = count
+
+    return counts
+
+
+def verify_backup(expected: dict[str, int], actual: dict[str, int]) -> VerificationReport:
+    """Verify actual database counts against expected counts."""
+    report = VerificationReport()
+
+    for table_name, expected_count in expected.items():
+        result = VerificationResult(
+            table_name=table_name,
+            expected_count=expected_count,
+            actual_count=actual.get(table_name, -1),
+            found=table_name in actual,
+        )
+        report.results.append(result)
+
+    # Check for unexpected extra tables
+    for table_name in actual:
+        if table_name not in expected:
+            report.errors.append(f"Unexpected table found: {table_name} ({actual[table_name]} rows)")
+
+    return report
+
+
+def run_dry_run(expected: dict[str, int]) -> VerificationReport:
+    """Simulate a verification with synthetic data for testing the tool."""
+    # Simulate: first table matches, second is missing, third has wrong count
+    actual: dict[str, int] = {}
+    for i, (table, count) in enumerate(expected.items()):
+        if i == 1:
+            continue  # Skip second table to simulate missing
+        if i == 2:
+            actual[table] = count + 100  # Wrong count
+        else:
+            actual[table] = count
+
+    return verify_backup(expected, actual)
+
+
+def generate_sample() -> dict[str, Any]:
+    """Generate a sample expected counts file."""
+    return {
+        "description": "Expected table row counts for backup verification",
+        "environment": "staging",
+        "tables": {
+            "users": 15420,
+            "accounts": 8750,
+            "transactions": 987654,
+            "orders": 345000,
+            "audit_log": 1200000,
+        }
+    }
+
+
+def main() -> int:
+    parser = argparse.ArgumentParser(
+        description="Verify backup restore integrity by checking table row counts.",
+        formatter_class=argparse.RawDescriptionHelpFormatter,
+        epilog="""
+Examples:
+  %(prog)s --expected counts.json --dsn "postgresql://user:pass@localhost/db"
+  %(prog)s --expected counts.json --dry-run
+  %(prog)s --