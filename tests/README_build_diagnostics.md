# Build diagnostic metadata tests

Run:

    python3 tests/test_build_diagnostic_metadata.py

Compile check:

    python3 -m py_compile build.py tests/test_build_diagnostic_metadata.py

These tests cover:

- successful diagnostic report metadata
- logd generation failure metadata
- chunked diagnostic logd references
- small logd archive split behavior
- large logd archive split behavior

The tests avoid external toolchains and do not require encryptly, npm, cargo, or network access.
