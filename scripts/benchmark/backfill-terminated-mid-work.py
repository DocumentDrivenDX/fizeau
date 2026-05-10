#!/usr/bin/env python3
"""Backfill the `terminated_mid_work` field on every report.json under
benchmark-results/fiz-tools-v1/cells/ by inspecting the trial's
session jsonl and reading the LAST llm.response event's finish_reason.

A trial was "terminated mid-work" if its final llm.response had
finish_reason in ('tool_calls','length') — meaning the model emitted a
tool call or hit max_tokens as its terminal output without ever getting
a chance to declare itself done. Distinguished from finish_reason='stop',
where the model voluntarily ended the conversation.

Idempotent. Mirrors logic that fiz.py records inline for new runs.

Usage:
    .venv-report/bin/python scripts/benchmark/backfill-terminated-mid-work.py [--dry-run]
"""

from __future__ import annotations

import argparse
import glob
import json
import sys
from collections import Counter
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
CELLS = REPO / "benchmark-results/fiz-tools-v1/cells"

# finish_reason values that mean the model was actively producing output
# when the conversation ended (i.e. it didn't choose to stop).
TRUNCATED_REASONS = {"tool_calls", "length", "function_call"}
CLEAN_REASONS = {"stop", "end_turn"}


def find_session_jsonl(report_path: Path) -> Path | None:
    """The session jsonl lives at <rep>/<job-name>/<trial-hash>/agent/sessions/svc-*.jsonl."""
    rep_dir = report_path.parent
    candidates = list(rep_dir.glob("*/*/agent/sessions/svc-*.jsonl"))
    if not candidates:
        return None
    # If multiple (rare), take newest by mtime — matches the trial we're judging
    return max(candidates, key=lambda p: p.stat().st_mtime)


def last_finish_reason(session_path: Path) -> str | None:
    """Stream through the jsonl and return finish_reason of the last llm.response."""
    last = None
    try:
        with session_path.open() as f:
            for line in f:
                try:
                    r = json.loads(line)
                except Exception:
                    continue
                if r.get("type") != "llm.response":
                    continue
                fr = (r.get("data") or {}).get("finish_reason")
                if fr:
                    last = fr
    except Exception:
        return None
    return last


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n", 1)[0])
    ap.add_argument("--dry-run", action="store_true", help="Don't write changes; just report deltas.")
    args = ap.parse_args()

    paths = glob.glob(f"{CELLS}/*/*/*/rep-*/report.json")
    transitions: Counter = Counter()
    no_session = 0
    no_finish = 0
    deltas: list[tuple[Path, dict, bool]] = []
    unchanged = 0

    for p_str in paths:
        p = Path(p_str)
        try:
            r = json.loads(p.read_text())
        except Exception as e:
            print(f"  WARN: skip unreadable {p}: {e}", file=sys.stderr)
            continue

        sess = find_session_jsonl(p)
        if sess is None:
            no_session += 1
            continue
        fr = last_finish_reason(sess)
        if fr is None:
            no_finish += 1
            continue

        if fr in TRUNCATED_REASONS:
            new_val = True
        elif fr in CLEAN_REASONS:
            new_val = False
        else:
            # Unknown finish_reason — leave as absent rather than guess
            transitions[(r.get("terminated_mid_work"), f"unknown:{fr}")] += 1
            continue

        old_val = r.get("terminated_mid_work")
        if old_val == new_val:
            unchanged += 1
            continue
        deltas.append((p, r, new_val))
        transitions[(old_val, new_val)] += 1

    print(f"scanned {len(paths)} reports", file=sys.stderr)
    print(f"unchanged: {unchanged}", file=sys.stderr)
    print(f"would change: {len(deltas)}", file=sys.stderr)
    print(f"no session jsonl found: {no_session}", file=sys.stderr)
    print(f"no llm.response events: {no_finish}", file=sys.stderr)
    print("transition counts:", file=sys.stderr)
    for (old, new), n in transitions.most_common():
        print(f"  {old!s:8s} -> {new!s:20s}  {n:>5}", file=sys.stderr)

    if args.dry_run:
        return 0

    for p, r, new_val in deltas:
        r["terminated_mid_work"] = new_val
        p.write_text(json.dumps(r, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {len(deltas)} updated report.json files", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
