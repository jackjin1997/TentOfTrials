 ```diff
--- a/build.py
+++ b/build.py
@@ -1,4 +1,5 @@
 #!/usr/bin/env python3
+# -*- coding: utf-8 -*-
 
 import argparse
 import datetime
@@ -10,6 +11,7 @@
 import subprocess
 import sys
 import time
+import tempfile
 from dataclasses import dataclass
 from pathlib import Path
 from typing import Optional
@@ -18,6 +20,7 @@
 DIAGNOSTIC_DIR = ROOT / "diagnostic"
 DIAGNOSTIC_CHUNK_SIZE = 40 * 1024 * 1024
 
+_GIT_PATH = "git"
 
 def current_commit_id() -> str:
     """Return the first 4 bytes (8 hex chars) of HEAD for stable per-commit diagnostics."""
@@ -25,7 +28,7 @@
         result = subprocess.run(
             ["git", "rev-parse", "--verify", "HEAD"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -35,6 +38,17 @@
         pass
     return "00000000"
 
+def _set_git_path(path: str) -> None:
+    """Override the git binary path (used by tests)."""
+    global _GIT_PATH
+    _GIT_PATH = path
+
+
+def _git_cmd() -> list[str]:
+    """Return the git command list."""
+    return [_GIT_PATH]
+
+
 
 def diagnostic_paths_for_commit() -> tuple[Path, Path, str]:
     """Return stable diagnostic artifact paths under diagnostic/ for the current commit."""
@@ -45,6 +59,7 @@
     return logd_path, metadata_path, commit_id
 
 
+
 def split_diagnostic_logd(logd_path: Path, chunk_size: int = DIAGNOSTIC_CHUNK_SIZE) -> list[Path]:
     """Split an oversized .logd into numbered .logd chunks and remove the original."""
     if logd_path.stat().st_size <= chunk_size:
@@ -65,6 +80,7 @@
     return chunks
 
 
+
 @dataclass
 class Module:
     name: str
@@ -75,6 +91,7 @@
     build_dir: Optional[Path] = None
     env: Optional[dict[str, str]] = None
 
+
 MODULES = [
     Module(
         name="backend",
@@ -134,6 +151,7 @@
         build_dir=ROOT / "compliance" / "build",
     ),
     Module(
+        name="scans",
         name="scans",
         language="Lua",
         dir=ROOT / "scans",
@@ -141,6 +159,7 @@
         clean_cmd=["echo", "Lua has no build artifacts to clean"],
     ),
     Module(
+        name="openapi",
         name="openapi",
         language="Haskell",
         dir=ROOT / "openapi",
@@ -148,6 +167,7 @@
         clean_cmd=["echo", "Haskell has no build artifacts to clean"],
     ),
     Module(
+        name="openapi-tools",
         name="openapi-tools",
         language="Lua",
         dir=ROOT / "tools" / "openapi",
@@ -155,6 +175,7 @@
         clean_cmd=["echo", "Lua has no build artifacts to clean"],
     ),
 ]
+]
 
 
 def run_build(module: Module, args: argparse.Namespace) -> dict:
@@ -162,7 +183,7 @@
     start = time.time()
     env = os.environ.copy()
     if module.env:
-        env |= module.env
+        env.update(module.env)
 
     try:
         result = subprocess.run(
@@ -170,7 +191,7 @@
             cwd=str(module.dir),
             env=env,
             capture_output=True,
-            text=True,
+            text=True,  # type: ignore[call-arg]
             timeout=300,
         )
         elapsed = time.time() - start
@@ -196,7 +217,7 @@
             cwd=str(module.dir),
             env=env,
             capture_output=True,
-            text=True,
+            text=True,  # type: ignore[call-arg]
             timeout=300,
         )
         return {"success": result.returncode == 0, "returncode": result.returncode}
@@ -213,7 +234,7 @@
         result = subprocess.run(
             ["git", "rev-parse", "--show-toplevel"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -227,7 +248,7 @@
         result = subprocess.run(
             ["git", "status", "--short"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -241,7 +262,7 @@
         result = subprocess.run(
             ["git", "log", "-1", "--format=%H %ci"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -254,7 +275,7 @@
         result = subprocess.run(
             ["git", "config", "user.name"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -267,7 +288,7 @@
         result = subprocess.run(
             ["git", "config", "user.email"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -283,7 +304,7 @@
         result = subprocess.run(
             ["git", "remote", "-v"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -296,7 +317,7 @@
         result = subprocess.run(
             ["git", "branch", "-a"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -309,7 +330,7 @@
         result = subprocess.run(
             ["git", "tag", "--list"],
             cwd=str(ROOT),
-            capture_output=True,
+            capture_output=True,  # type: ignore[call-arg]
             text=True,
             timeout=5,
         )
@@ -322,7 +343,7 @@
         result = subprocess.run(
             ["git", "describe", "--tags", "--always"],
             cwd=str(ROOT),
