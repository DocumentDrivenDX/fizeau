#!/usr/bin/env python3
"""Re-classify the invalid_class field on every report.json under
benchmark-results/fiz-tools-v1/cells/ using the current Go classifier
logic in cmd/bench/matrix_invalid.go.

Idempotent. Mirrors classifyMatrixInvalid 1:1; if the Go logic changes,
update both.

Usage:
    .venv-report/bin/python scripts/benchmark/reclassify-invalid.py [--dry-run]
"""

from __future__ import annotations

import argparse
import glob
import json
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
CELLS = REPO / "benchmark-results/fiz-tools-v1/cells"

# Mirror cmd/bench/matrix_invalid.go regex patterns.
QUOTA = re.compile(r"(?i)(api_error_status:\s*429|insufficient[_\s-]*quota|out_of_credits|credits?\s+exhausted|usage\s+exhausted|rate\s*limit|too many requests|quota\s+exhausted|quota\s+exceeded)")
AUTH = re.compile(r"(?i)(unauthori[sz]ed|authentication failed|invalid api key|missing credentials?|not signed in|login required|account .*not .*authenticated|oauth.*failed|credential.*missing|account .*required|access denied)")
SETUP_DEFINITIVE = re.compile(r"(?i)(binary not found|exec format error|cannot execute binary file|wrong architecture|architecture mismatch|task dir not found|submodule not initialized|wrapper startup|asyncio\.run\(\) cannot be called from a running event loop|preflight failure|reasoning=[^ ]* is not supported by provider type|reasoning_wire=none|qwen reasoning control is not supported|unsupported reasoning [^ ]* for harness)")
SETUP_BROAD = re.compile(r"(?i)(no such file or directory|failed to start|startup failed|docker.*(failed|error)|container.*(failed|error)|image.*(failed|error|not found)|harbor[\\/]+environments[\\/]+docker|_run_docker_compose_command|_start_environment_with_retry|docker compose command|operation not permitted|permission denied|sandbox.*failed|setup failed)")
PROVIDER = re.compile(r"(?i)(connection refused|connection reset|socket hang up|fetch failed|tls handshake|dns|eof|timed out|timeout|stream closed|broken pipe|remote closed|upstream|service unavailable|bad gateway|gateway timeout|failed to connect|provider transport|network error)")

KNOWN_CLASSES = {"invalid_quota", "invalid_auth", "invalid_setup", "invalid_provider"}


def has_meaningful_attempt(r: dict) -> bool:
    for k in ("turns", "tool_calls", "tool_call_errors", "input_tokens",
              "output_tokens", "cached_input_tokens", "retried_input_tokens"):
        v = r.get(k)
        if v is not None and v > 0:
            return True
    return False


def signal_blob(r: dict) -> str:
    parts = [r.get("error", ""), r.get("process_outcome", ""), r.get("final_status", "")]
    notes = r.get("adapter_translation_notes")
    if isinstance(notes, list):
        parts.append(" ".join(notes))
    cmd = r.get("command")
    if isinstance(cmd, list):
        parts.append(" ".join(cmd))
    return "\n".join(p.strip() for p in parts if p and p.strip()).lower()


def classify(r: dict) -> str:
    """Return the new invalid_class for this report, or '' if not invalid."""
    fs = r.get("final_status", "")
    if fs == "graded_pass":
        return ""
    if fs in KNOWN_CLASSES:
        return fs
    # Pre-existing invalid_class is NOT preserved here — we're reclassifying
    # from scratch using the new logic. That's the whole point of this pass.
    has_attempt = has_meaningful_attempt(r)
    blob = signal_blob(r)
    if has_attempt:
        if QUOTA.search(blob):
            return "invalid_quota"
        if AUTH.search(blob):
            return "invalid_auth"
        if SETUP_DEFINITIVE.search(blob):
            return "invalid_setup"
    else:
        if QUOTA.search(blob):
            return "invalid_quota"
        if AUTH.search(blob):
            return "invalid_auth"
        if SETUP_DEFINITIVE.search(blob) or SETUP_BROAD.search(blob):
            return "invalid_setup"
        if PROVIDER.search(blob):
            return "invalid_provider"
    if fs == "verifier_fail":
        return ""
    if fs in ("install_fail_permanent", "install_failed"):
        return "invalid_setup"
    if fs == "harness_crash":
        # Mirror Go: agent runtime crash before grading is systemic.
        return "invalid_setup"
    if fs == "ran" and not has_attempt and r.get("grading_outcome") in (None, "", "ungraded"):
        # Mirror Go: final_status="ran" + ungraded + no attempt = harbor
        # wrapper exited but trial never ran (docker pull fail, env setup
        # error, etc.). Exception lives in a side-file, not report.error.
        return "invalid_setup"
    if fs == "graded_fail":
        if not has_attempt:
            # Sub-classify: provider hang (request fired, no response) vs
            # setup failure (never reached the model). Mirrors Go classifier.
            if r.get("had_llm_request") is True and r.get("terminated_mid_work") is None:
                return "invalid_provider"
            return "invalid_setup"
        if (r.get("output_tokens") or 0) == 0 and (r.get("turns") or 0) <= 2:
            wall = r.get("wall_seconds")
            if wall is not None and wall < 30:
                return "invalid_setup"
        return ""
    return ""


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n", 1)[0])
    ap.add_argument("--dry-run", action="store_true", help="Don't write changes; just report deltas.")
    args = ap.parse_args()

    paths = glob.glob(f"{CELLS}/*/*/*/rep-*/report.json")
    deltas = []
    unchanged = 0
    for p in paths:
        try:
            r = json.load(open(p))
        except Exception as e:
            print(f"  WARN: skip unreadable {p}: {e}", file=sys.stderr)
            continue
        old = r.get("invalid_class", "") or ""
        new = classify(r)
        if old == new:
            unchanged += 1
            continue
        deltas.append((p, old, new, r))
    print(f"scanned {len(paths)} reports", file=sys.stderr)
    print(f"unchanged: {unchanged}", file=sys.stderr)
    print(f"would change: {len(deltas)}", file=sys.stderr)
    from collections import Counter
    transitions = Counter((old or "(empty)", new or "(empty)") for _, old, new, _ in deltas)
    print("transition counts:", file=sys.stderr)
    for (old, new), n in transitions.most_common():
        print(f"  {old:20s} → {new:20s}  {n:>4}", file=sys.stderr)

    if args.dry_run:
        return 0

    for p, _, new, r in deltas:
        if new:
            r["invalid_class"] = new
        else:
            r.pop("invalid_class", None)
        Path(p).write_text(json.dumps(r, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {len(deltas)} updated report.json files", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
