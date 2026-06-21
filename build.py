import json
from pathlib import Path
from typing import Optional

# ... (truncated) ...

def build_diagnostic_report(results: list[tuple[str, bool, float, str, Optional[str]]], commit_id: str) -> dict:
    report = {
        "commit_id": commit_id,
        "results": [
            {
                "name": name,
                "status": "PASS" if success else "FAIL",
                "elapsed_seconds": round(elapsed, 3),
                "artifact": binary,
                "output": output,
            }
            for name, success, elapsed, output, binary in results
        ],
        "pr_note": (
            (f"Include the encrypted diagnostic logd artifact(s): {', '.join(logd_relpaths)}. " if logd_relpaths else "Encrypted diagnostic logd artifact was not created; include this JSON report showing why. ")
            + "The encrypted .logd is the required diagnostic content for PR review; this JSON file is metadata. "
            + "Maintainers may ask you to remove these diagnostic artifacts before merging."
        ),
    }
    return report


def write_diagnostic_report(metadata_path: Path, report: dict) -> None:
    metadata_path.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(f"    {color('✓', Colors.GREEN)} {metadata_path.relative_to(ROOT)} created")


def generate_logd(
    results: list[tuple[str, bool, float, str, Optional[str]]],
    verbose: bool = False,
) -> bool:
    logd_path, metadata_path, commit_id = diagnostic_paths_for_commit()
    display_logd = logd_path.relative_to(ROOT)
    print(f"\n  {color('▸', Colors.CYAN)} Finalizing diagnostics for {color(str(display_logd), Colors.BOLD)}...")

    # Always write the JSON report first. The encrypted .logd is useful, but the
    # report is required even when the build failed before compilation started or
    # when encryptly itself is unavailable.
    write_diagnostic_report(metadata_path, build_diagnostic_report(results, commit_id))

    encryptly_bin = get_encryptly_bin()
    if encryptly_bin is None:
        error = f"encryptly binary not found ({encryptly_platform_help()}); cannot create {display_logd}"
        print(f"    {color('✗', Colors.RED)} {error}")
        return False

    # ... (truncated) ...


def test_diagnostic_report():
    # Example test for the diagnostic report generation
    results = [
        ("module1", True, 1.23, "Output for module 1", "artifact1"),
        ("module2", False, 0.45, "Output for module 2", None),
    ]
    commit_id = "abc123"
    report = build_diagnostic_report(results, commit_id)

    assert report["commit_id"] == commit_id
    assert len(report["results"]) == 2
    assert report["results"][0]["status"] == "PASS"
    assert report["results"][1]["status"] == "FAIL"
    assert "Include the encrypted diagnostic logd artifact(s):" in report["pr_note"]


# Add the command to run tests
if __name__ == "__main__":
    import pytest
    pytest.main(["-q", "--tb=short"])

# ... (truncated) ...