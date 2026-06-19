# Python Diagnostic Tests

Run the diagnostic metadata regression tests from the repository root:

```sh
python -m unittest discover -s tests -p "test_*.py"
```

These tests cover the JSON report shape produced by `build.py`, including successful
`.logd` metadata, `.logd` generation failures, and chunked `.logd` references.
