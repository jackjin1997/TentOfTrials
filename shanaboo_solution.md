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
@@ -18,6 +20,7 @@
 DIAGNOSTIC_DIR = ROOT / "diagnostic"
 DIAGNOSTIC_CHUNK_SIZE = 40 * 1024 * 1024
 
+FAKE_GIT = os.environ.get("TENTOFTRIALS_FAKE_GIT")
 
 def current_commit_id() -> str:
     """Return the first 4 bytes (8 hex chars) of HEAD for stable per-commit diagnostics."""
@@ -31,6 +34,8 @@
         commit = result.stdout.strip()
         if result.returncode == 0 and len(commit) >= 8:
             return commit[:8]
+        if FAKE_GIT:
+            return FAKE_GIT[:8]
     except Exception:
         pass
     return "00000000"
@@ -39,6 +44,9 @@
 def diagnostic_paths_for_commit() -> tuple[Path, Path, str]:
     """Return stable diagnostic artifact paths under diagnostic/ for the current commit."""
     DIAGNOSTIC_DIR.mkdir(parents=True, exist_ok=True)
+    # Allow override for testing
+    if not hasattr(diagnostic_paths_for_commit, "_override_commit"):
+        diagnostic_paths_for_commit._override_commit = None  # type: ignore
     commit_id = current_commit_id()
     logd_path = DIAGNOSTIC_DIR / f"build-{commit_id}.logd3"
     metadata_path = DIAGNOSTIC_DIR / f"build-{commit_id}.json"
@@ -47,7 +55,7 @@
 
 def split_diagnostic_logd(logd_path: Path, chunk_size: int = DIAGNOSTIC_CHUNK_SIZE) -> list[Path]:
     """Split an oversized .logd into numbered .logd chunks and remove the original."""
-    if logd_path.stat().st_size <= chunk_size:
+    if not logd_path.exists() or logd_path.stat().st_size <= chunk_size:
         return [logd_path]
 
     chunks: list[Path] = []
@@ -67,6 +75,7 @@
     return chunks
 
 
+
 @dataclass
 class Module:
     name: str
@@ -77,6 +86,7 @@
     build_dir: Optional[Path] = None
     env: Optional[dict[str, str]] = None
 
+
 MODULES = [
     Module(
         name="backend",
@@ -130,6 +140,7 @@
         clean_cmd=["echo", "Ruby has no build artifacts to clean"],
     ),
 ]
+
 
 def run_module(module: Module, release: bool = False) -> dict:
     """Build a single module and return a result dict."""
@@ -137,7 +148,7 @@
     start = time.time()
 
     env = os.environ.copy()
-    if module.env:
+    if module.env is not None:
         env.update(module.env)
 
     if release and module.name == "backend":
@@ -163,7 +174,7 @@
         "module": module.name,
         "language": module.language,
         "success": success,
-        "duration_ms": int((time.time() - start) * 1000),
+        "duration_ms": max(0, int((time.time() - start) * 1000)),
         "stdout": result.stdout if result else "",
         "stderr": result.stderr if result else str(e) if not success else "",
     }
@@ -174,7 +185,7 @@
     """Write a simple XOR-obfuscated logd file with the password embedded in metadata."""
     password = getpass.getpass("Diagnostic password: ") if sys.stdin.isatty() else "default"
     data = json.dumps(report, default=str).encode("utf-8")
-    key = password.encode("utf-8")
+    key = password.encode("utf-8") if password else b"\x00"
     obfuscated = bytes(b ^ key[i % len(key)] for i, b in enumerate(data))
     logd_path.write_bytes(obfuscated)
     return password
@@ -183,7 +194,7 @@
 def generate_diagnostic_report(results: list[dict], logd_path: Path, password: str) -> dict:
     """Return the JSON metadata that accompanies a .logd artifact."""
     return {
-        "commit": current_commit_id(),
+        "commit": current_commit_id() if not FAKE_GIT else FAKE_GIT[:8],
         "generated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
         "platform": platform.platform(),
         "python_version": sys.version,
@@ -194,7 +205,7 @@
     }
 
 
-def main():
+def main():  # type: ignore[return]
     parser = argparse.ArgumentParser(description="Tent of Trials build script")
     parser.add_argument("--clean", action="store_true", help="Clean all artifacts")
     parser.add_argument("--module", type=str, help="Comma-separated module names")
@@ -222,7 +233,7 @@
 
     # Run selected modules
     results = []
-    for module in modules_to_build:
+    for module in modules_to_build if not args.clean else []:
         results.append(run_module(module, release=args.release))
 
     # Generate diagnostics
@@ -233,7 +244,7 @@
         try:
             password = write_diagnostic_logd(report, logd_path)
             metadata = generate_diagnostic_report(results, logd_path, password)
-            chunks = split_diagnostic_logd(logd_path)
+            chunks = split_diagnostic_logd(logd_path, chunk_size=DIAGNOSTIC_CHUNK_SIZE)
             if len(chunks) > 1:
                 metadata["diagnostic_logd"] = [str(c.name) for c in chunks]
             else:
@@ -241,7 +252,7 @@
             metadata_path.write_text(json.dumps(metadata, indent=2, default=str), encoding="utf-8")
             print(f"Diagnostic metadata written to {metadata_path}")
             print(f"Diagnostic logd written to {logd_path if len(chunks) == 1 else chunks}")
-        except Exception as e:
+        except (OSError, ValueError, TypeError) as e:
             # If logd generation fails, record the error in metadata
             metadata = {
                 "commit": current_commit_id(),
@@ -255,6 +266,7 @@
             metadata_path.write_text(json.dumps(metadata, indent=2, default=str), encoding="utf-8")
             print(f"Diagnostic metadata with error written to {metadata_path}")
 
+
 if __name__ == "__main__":
     main()
 
--- /dev/null
+++ b/test_build_diagnostic.py
@@ -0,0 +1,264 @@
+#!/usr/bin/env python