 ```diff
--- a/build.py
+++ b/build.py
@@ -1,4 +1,5 @@
 #!/usr/bin/env python3
+
 """Build script for TentOfTrials with diagnostic metadata generation."""
 
 import argparse
@@ -14,7 +15,7 @@
 from dataclasses import dataclass
 from pathlib import Path
 from typing import Optional
-
+import hashlib
 
 ROOT = Path(__file__).resolve().parent
 DIAGNOSTIC_DIR = ROOT / "diagnostic"
@@ -23,7 +24,7 @@
 
 def current_commit_id() -> str:
     """Return the first 4 bytes (8 hex chars) of HEAD for stable per-commit diagnostics."""
-    try:
+    try:
         result = subprocess.run(
             ["git", "rev-parse", "--verify", "HEAD"],
             cwd=str(ROOT),
@@ -32,7 +33,7 @@
             timeout=5,
         )
         commit = result.stdout.strip()
-        if result.returncode == 0 and len(commit) >= 8:
+        if result.returncode == 0 and len(commit) >= 8:
             return commit[:8]
     except Exception:
         pass
@@ -41,7 +42,7 @@
 
 def diagnostic_paths_for_commit() -> tuple[Path, Path, str]:
     """Return stable diagnostic artifact paths under diagnostic/ for the current commit."""
-    DIAGNOSTIC_DIR.mkdir(parents=True, exist_ok=True)
+    DIAGNOSTIC_DIR.mkdir(parents=True, exist_ok=True)
     commit_id = current_commit_id()
     logd_path = DIAGNOSTIC_DIR / f"build-{commit_id}.logd"
     metadata_path = DIAGNOSTIC_DIR / f"build-{commit_id}.json"
@@ -50,7 +51,7 @@
 
 def split_diagnostic_logd(logd_path: Path, chunk_size: int = DIAGNOSTIC_CHUNK_SIZE) -> list[Path]:
     """Split an oversized .logd into numbered .logd chunks and remove the original."""
-    if logd_path.stat().st_size <= chunk_size:
+    if logd_path.stat().st_size <= chunk_size:
         return [logd_path]
 
     chunks: list[Path] = []
@@ -68,6 +69,7 @@
     logd_path.unlink()
     return chunks
 
+
 @dataclass
 class Module:
     name: str
@@ -78,6 +80,7 @@
     build_dir: Optional[Path] = None
     env: Optional[dict[str, str]] = None
 
+
 MODULES = [
     Module(
         name="backend",
@@ -123,7 +126,7 @@
         name="v2-market-stream",
         language="Ruby",
         dir=ROOT / "v2" / "services",
-        build_cmd=["ruby", "-c", "market_stream.rb"],
+        build_cmd=["ruby", "-c", "market_stream.rb"],
         clean_cmd=["echo", "Ruby has no build artifacts to clean"],
     ),
     Module(
@@ -131,7 +134,7 @@
         language="Lua",
         dir=ROOT / "scans",
         build_cmd=["luac", "-p", "init.lua"],
-        clean_cmd=["rm", "-f", "lu ceased"],
+        clean_cmd=["rm", "-f", "luac.out"],
     ),
     Module(
         name="openapi",
@@ -139,7 +142,7 @@
         dir=ROOT / "openapi",
         build_cmd=["cabal", "build"],
         clean_cmd=["cabal", "clean"],
-        build_dir=ROOT / "openapi" / "dist-newstyle",
+        build_dir=ROOT / "openapi" / "dist-newstyle",
     ),
     Module(
         name="openapi-tools",
@@ -149,6 +152,7 @@
         clean_cmd=["rm", "-f", "*.out"],
     ),
 ]
+
 
 def run_module_build(module: Module, args: argparse.Namespace) -> dict:
     """Run a single module build and return its result dict for metadata."""
@@ -157,7 +161,7 @@
     start = time.time()
     try:
         env = os.environ.copy()
-        if module.env:
+        if module.env:
             env.update(module.env)
         result = subprocess.run(
             module.build_cmd,
@@ -169,7 +173,7 @@
         )
         elapsed = time.time() - start
         success = result.returncode == 0
-        return {
+        return {
             "name": module.name,
             "language": module.language,
             "success": success,
@@ -178,7 +182,7 @@
             "stdout": result.stdout if result.stdout else "",
             "stderr": result.stderr if result.stderr else "",
         }
-    except Exception as e:
+    except Exception as e:
         elapsed = time.time() - start
         return {
             "name": module.name,
@@ -189,6 +193,7 @@
             "error": str(e),
         }
 
+
 def generate_password() -> str:
     """Generate a random password for logd encryption."""
     chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
@@ -196,6 +201,7 @@
     return "".join(secrets.choice(chars) for _ in range(32))
 
 
+
 def encrypt_logd(data: bytes, password: str) -> bytes:
     """Encrypt logd data with a simple XOR-based scheme for demonstration."""
     key = hashlib.sha256(password.encode()).digest()
@@ -204,6 +210,7 @@
         encrypted.append(b ^ key[i % len(key)])
     return bytes(encrypted)
 
+
 def write_diagnostic_metadata(
     metadata_path: Path,
     commit_id: str,
@@ -212,7 +219,7 @@
     logd_chunks: list[Path],
     logd_error: Optional[str] = None,
 ) -> None:
-    """Write diagnostic metadata JSON with module results and logd references."""
+    """Write diagnostic metadata JSON with module results and logd references."""
     metadata = {
         "commit_id": commit_id,
         "timestamp": datetime.datetime.now(datetime.timezone.utc).isoformat(),
@@ -222,7 +229,7 @@
         "module_results": module_results,
     }
     if logd_error:
-        metadata["diagnostic_logd_error"] = logd_error
+        metadata["diagnostic_logd_error"] = logd_error
     else:
         metadata["diagnostic_logd"] = str(logd_path)
         if len(logd_chunks) > 1:
@@ -230,6 +237,7 @@
     with open(metadata_path, "w") as f:
         json.dump(metadata, f, indent=2)
 
+
 def write_diagnostic_logd(
     logd_path: Path,
     module_results: list[dict],
@@ -237,7 +245,7 @@
     """Write encrypted diagnostic logd and return list of chunk paths."""
     lines: list[str] = []
     lines.append