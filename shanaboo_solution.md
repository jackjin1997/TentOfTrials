 ```diff
--- a/build.py
+++ b/build.py
@@ -1,4 +1,5 @@
 #!/usr/bin/env python3
+# -*- coding: utf-8 -*-
 
 import argparse
 import datetime
@@ -7,6 +8,7 @@
 import os
 import platform
 import shutil
+import struct
 import subprocess
 import sys
 import time
@@ -20,6 +22,7 @@
 DIAGNOSTIC_DIR = ROOT / "diagnostic"
 DIAGNOSTIC_CHUNK_SIZE = 40 * 1024 * 1024
 
+DIAGNOSTIC_PASSWORD = "tent-of-trials-diag"
 
 def current_commit_id() -> str:
     """Return the first 4 bytes (8 hex chars) of HEAD for stable per-commit diagnostics."""
@@ -35,7 +38,7 @@
         if result.returncode == 0 and len(commit) >= 8:
             return commit[:8]
     except Exception:
-        pass
+        return "00000000"
     return "00000000"
 
 
@@ -46,6 +49,7 @@
     metadata_path = DIAGNOSTIC_DIR / f"build-{commit_id}.json"
     return logd_path, metadata_path, commit_id
 
+
 def split_diagnostic_logd(logd_path: Path, chunk_size: int = DIAGNOSTIC_CHUNK_SIZE) -> list[Path]:
     """Split an oversized .logd into numbered .logd chunks and remove the original."""
     if logd_path.stat().st_size <= chunk_size:
@@ -67,6 +71,7 @@
     logd_path.unlink()
     return chunks
 
+
 @dataclass
 class Module:
     name: str
@@ -77,6 +82,7 @@
     build_dir: Optional[Path] = None
     env: Optional[dict[str, str]] = None
 
+
 MODULES = [
     Module(
         name="backend",
@@ -130,7 +136,7 @@
         name="v2-market-stream",
         language="Ruby",
         dir=ROOT / "v2" / "services",
-        build_cmd=["ruby", "-c", "market_stream.rb"],
+        build_cmd=["ruby", "-c", "market_stream.rb"],
         clean_cmd=["echo", "Ruby has no build artifacts to clean"],
         build_dir=None,
     ),
@@ -138,7 +144,7 @@
         name="scans",
         language="Lua",
         dir=ROOT / "scans",
-        build_cmd=["luac", "-p", "init.lua"],
+        build_cmd=["luac", "-p", "init.lua"],
         clean_cmd=["rm", "-f", "luac.out"],
         build_dir=None,
     ),
@@ -146,7 +152,7 @@
         name="openapi",
         language="Haskell",
         dir=ROOT / "openapi",
-        build_cmd=["cabal", "build"],
+        build_cmd=["cabal", "build"],
         clean_cmd=["cabal", "clean"],
         build_dir=ROOT / "openapi" / "dist-newstyle",
     ),
@@ -154,7 +160,7 @@
         name="openapi-tools",
         language="Lua",
         dir=ROOT / "tools" / "openapi",
-        build_cmd=["luac", "-p", "spec.lua"],
+        build_cmd=["luac", "-p", "spec.lua"],
         clean_cmd=["rm", "-f", "luac.out"],
         build_dir=None,
     ),
@@ -162,7 +168,7 @@
         name="data-pipeline",
         language="Python",
         dir=ROOT / "data",
-        build_cmd=["python3", "-m", "py_compile", "pipeline.py"],
+        build_cmd=["python3", "-m", "py_compile", "pipeline.py"],
         clean_cmd=["rm", "-rf", "__pycache__"],
         build_dir=None,
     ),
@@ -178,7 +184,7 @@
         name="frontend",
         language="TypeScript",
         dir=ROOT / "frontend",
-        build_cmd=["npm", "run", "build"],
+        build_cmd=["npm", "run", "build"],
         clean_cmd=["rm", "-rf", "node_modules", "dist"],
         build_dir=ROOT / "frontend" / "dist",
         env={"NODE_ENV": "production"},
@@ -187,7 +193,7 @@
         name="market",
         language="Go",
         dir=ROOT / "market",
-        build_cmd=["go", "build", "-o", "market", "."],
+        build_cmd=["go", "build", "-o", "market", "."],
         clean_cmd=["rm", "-f", "market"],
         build_dir=ROOT / "market" / "market",
     ),
@@ -195,7 +201,7 @@
         name="frailbox",
         language="C",
         dir=ROOT / "frailbox",
-        build_cmd=["make"],
+        build_cmd=["make"],
         clean_cmd=["make", "distclean"],
         build_dir=ROOT / "frailbox" / "frailbox",
     ),
@@ -203,7 +209,7 @@
         name="engine",
         language="C++",
         dir=ROOT / "frailbox" / "engine",
-        build_cmd=["cmake", "--build", "build"],
+        build_cmd=["cmake", "--build", "build"],
         clean_cmd=["rm", "-rf", "build"],
         build_dir=ROOT / "frailbox" / "engine" / "build" / "trial-engine",
     ),
@@ -211,7 +217,7 @@
         name="compliance",
         language="Java",
         dir=ROOT / "compliance",
-        build_cmd=["javac", "-d", "build", "ComplianceAuditor.java"],
+        build_cmd=["javac", "-d", "build", "ComplianceAuditor.java"],
         clean_cmd=["rm", "-rf", "build"],
         build_dir=ROOT / "compliance" / "build",
     ),
@@ -219,7 +225,7 @@
         name="v2-market-stream",
         language="Ruby",
         dir=ROOT / "v2" / "services",
-        build_cmd=["ruby", "-c", "market_stream.rb"],
+        build_cmd=["ruby", "-c", "market_stream.rb"],
         clean_cmd=["echo", "Ruby has no build artifacts to clean"],
         build_dir=None,
     ),
@@ -227,7 +233,7 @@
         name="scans",
         language="Lua",
         dir=ROOT / "scans",
-        build_cmd=["luac", "-p", "init.lua"],
+        build_cmd=["luac", "-p", "init.lua"],
         clean_cmd=["rm", "-f", "luac.out"],
         build_dir=None,
     ),
@@ -235,7 +241,7 @@
         name="openapi",
         language="Haskell",
         dir=ROOT / "openapi",
-        build_cmd=["