#!/usr/bin/env python3
"""Deterministic fixtures for beadbench timeout artifact handling.

Run with ``python3 scripts/beadbench/test_run_beadbench.py`` — the script
exits non-zero on any failed assertion and zero on success.
"""

from __future__ import annotations

import json
import pathlib
import subprocess
import sys
import tempfile

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

import run_beadbench as rb  # noqa: E402


def _git(cwd: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", "-C", str(cwd), *args], text=True, capture_output=True, check=True
    )


def _init_sandbox(root: pathlib.Path) -> str:
    root.mkdir(parents=True, exist_ok=True)
    _git(root, "init", "--quiet", "-b", "main")
    _git(root, "config", "user.email", "beadbench-test@example.invalid")
    _git(root, "config", "user.name", "beadbench-test")
    (root / "README").write_text("base\n")
    _git(root, "add", "README")
    _git(root, "commit", "--quiet", "-m", "base")
    base_rev = _git(root, "rev-parse", "HEAD").stdout.strip()
    return base_rev


def test_no_output_timeout(tmp: pathlib.Path) -> None:
    sandbox = tmp / "s1"
    base = _init_sandbox(sandbox)
    artifacts = tmp / "a1"
    artifacts.mkdir()

    exc = subprocess.TimeoutExpired(cmd=["ddx", "agent"], timeout=1.0, output=b"", stderr=b"")
    info = rb.record_timeout_evidence(exc, sandbox, base, artifacts)

    assert info["progress_class"] == "no_output", info
    assert info["partial_stdout_bytes"] == 0
    assert info["partial_stderr_bytes"] == 0
    assert (artifacts / "timeout.txt").exists()
    assert (artifacts / "stdout.txt").read_text() == ""
    assert (artifacts / "stderr.txt").read_text() == ""
    assert "partial_execute_result" not in info


def test_read_only_progress(tmp: pathlib.Path) -> None:
    sandbox = tmp / "s2"
    base = _init_sandbox(sandbox)
    artifacts = tmp / "a2"
    artifacts.mkdir()

    exc = subprocess.TimeoutExpired(
        cmd=["ddx", "agent"],
        timeout=2.0,
        output=b"thinking...\n",
        stderr=b"resolving model...\n",
    )
    info = rb.record_timeout_evidence(exc, sandbox, base, artifacts)

    assert info["progress_class"] == "read_only_progress", info
    assert info["partial_stdout_bytes"] > 0
    assert info["partial_stderr_bytes"] > 0
    assert (artifacts / "stdout.txt").read_text() == "thinking...\n"
    assert (artifacts / "stderr.txt").read_text() == "resolving model...\n"


def test_partial_json_recovered(tmp: pathlib.Path) -> None:
    sandbox = tmp / "s3"
    base = _init_sandbox(sandbox)
    artifacts = tmp / "a3"
    artifacts.mkdir()

    partial = (
        b'log line\n'
        b'{"status":"partial","preserve_rev":"deadbeef1234"}\n'
        b'still more lo'
    )
    exc = subprocess.TimeoutExpired(
        cmd=["ddx", "agent"], timeout=3.0, output=partial, stderr=b""
    )
    info = rb.record_timeout_evidence(exc, sandbox, base, artifacts)

    assert "partial_execute_result" in info
    assert info["partial_execute_result"]["preserve_rev"] == "deadbeef1234"
    assert info["preserve_rev"] == "deadbeef1234"
    recovered = json.loads((artifacts / "execute-result.json").read_text())
    assert recovered["status"] == "partial"


def test_write_progress_via_commit(tmp: pathlib.Path) -> None:
    sandbox = tmp / "s4"
    base = _init_sandbox(sandbox)
    (sandbox / "NEW").write_text("work\n")
    _git(sandbox, "add", "NEW")
    _git(sandbox, "commit", "--quiet", "-m", "agent work in progress")

    artifacts = tmp / "a4"
    artifacts.mkdir()
    exc = subprocess.TimeoutExpired(
        cmd=["ddx", "agent"], timeout=4.0, output=b"", stderr=b""
    )
    info = rb.record_timeout_evidence(exc, sandbox, base, artifacts)

    assert info["progress_class"] == "write_progress", info
    assert info["sandbox_state"]["commits_ahead_of_base"] == 1
    assert (artifacts / "timeout-sandbox-state.json").exists()


def test_write_progress_via_preserve_ref(tmp: pathlib.Path) -> None:
    sandbox = tmp / "s5"
    base = _init_sandbox(sandbox)
    _git(sandbox, "update-ref", "refs/execute-bead/preserve/agent-xyz", base)

    artifacts = tmp / "a5"
    artifacts.mkdir()
    exc = subprocess.TimeoutExpired(
        cmd=["ddx", "agent"], timeout=5.0, output=b"", stderr=b""
    )
    info = rb.record_timeout_evidence(exc, sandbox, base, artifacts)

    assert info["progress_class"] == "write_progress", info
    refs = [r["ref"] for r in info["sandbox_state"]["preserve_refs"]]
    assert "refs/execute-bead/preserve/agent-xyz" in refs


def test_missing_sandbox_is_tolerated(tmp: pathlib.Path) -> None:
    sandbox = tmp / "does-not-exist"
    artifacts = tmp / "a6"
    artifacts.mkdir()
    exc = subprocess.TimeoutExpired(
        cmd=["ddx", "agent"], timeout=6.0, output=b"line\n", stderr=b""
    )
    info = rb.record_timeout_evidence(exc, sandbox, "HEAD", artifacts)
    assert info["progress_class"] == "read_only_progress"
    assert "sandbox_state" not in info


def main() -> int:
    cases = [
        test_no_output_timeout,
        test_read_only_progress,
        test_partial_json_recovered,
        test_write_progress_via_commit,
        test_write_progress_via_preserve_ref,
        test_missing_sandbox_is_tolerated,
    ]
    failures: list[str] = []
    for case in cases:
        with tempfile.TemporaryDirectory(prefix="beadbench-test-") as raw:
            try:
                case(pathlib.Path(raw))
                print(f"ok  {case.__name__}")
            except AssertionError as exc:
                failures.append(f"{case.__name__}: {exc}")
                print(f"FAIL {case.__name__}: {exc}")
            except Exception as exc:
                failures.append(f"{case.__name__}: {exc!r}")
                print(f"ERROR {case.__name__}: {exc!r}")
    if failures:
        print(f"\n{len(failures)} failure(s)", file=sys.stderr)
        return 1
    print(f"\n{len(cases)} tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
