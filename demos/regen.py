#!/usr/bin/env python3
"""Regenerate asciinema cast files from canonical session JSONLs.

Reads a fiz session JSONL (session.start + llm.response + session.end events)
and emits an asciicast v2 file mimicking what an interactive `fiz -p '<prompt>'`
invocation would print.

This avoids any live LLM calls and any dependence on the `asciinema rec`
binary, so it is fully deterministic and portable.

Output format follows the existing demos in website/static/demos/*.cast:
  line 1: header     {"version":2, "width":W, "height":H, ...}
  line 2: prompt     [t0,  "o", "$ fiz -p '<prompt>'\r\n\r\n"]
  line 3: response   [t1,  "o", "<assistant content>\r\n[success] tokens: I in / O out\r\n"]

Usage:
  regen.py --in demos/sessions/file-read.jsonl \\
           --out website/static/demos/file-read.cast \\
           --prompt "Read main.go and explain what this program does" \\
           --title "Fizeau: file-read"
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

# Pinned timestamp so casts are byte-identical across regenerations.
# 2026-01-01T00:00:00Z
PINNED_TIMESTAMP = 1767225600

# Initial paint delay before the prompt line appears.
PROMPT_DELAY_S = 0.005

# Minimum think time between prompt echo and final answer (seconds).
MIN_RESPONSE_GAP_S = 1.0


def crlf(s: str) -> str:
    """Convert lone LFs to CRLFs as terminal would render."""
    return s.replace("\r\n", "\n").replace("\n", "\r\n")


def load_session(path: Path) -> dict:
    """Extract the canonical (start, response, end) triple from a session JSONL."""
    start = response = end = None
    with path.open() as fh:
        for raw in fh:
            raw = raw.strip()
            if not raw:
                continue
            event = json.loads(raw)
            etype = event.get("type")
            if etype == "session.start" and start is None:
                start = event
            elif etype == "llm.response":
                # Last response wins (final assistant turn).
                response = event
            elif etype == "session.end":
                end = event
    if start is None or response is None or end is None:
        raise SystemExit(
            f"{path}: missing session.start / llm.response / session.end events"
        )
    return {"start": start, "response": response, "end": end}


def render_cast(
    session: dict,
    prompt_display: str,
    title: str,
    width: int,
    height: int,
) -> str:
    response = session["response"]["data"]
    end = session["end"]["data"]

    content = (response.get("content") or end.get("output") or "").strip()
    usage = response.get("usage") or end.get("tokens") or {}
    tokens_in = usage.get("input", 0)
    tokens_out = usage.get("output", 0)
    status = end.get("status", "success")

    # Replay-pace gap derived from the recorded latency, clamped so the cast
    # feels alive but stays well under the 30s budget.
    latency_ms = response.get("latency_ms", 3000)
    gap_s = max(MIN_RESPONSE_GAP_S, min(10.0, latency_ms / 1000.0))

    header = {
        "version": 2,
        "width": width,
        "height": height,
        "timestamp": PINNED_TIMESTAMP,
        "env": {"SHELL": "/bin/zsh", "TERM": "screen-256color"},
        "title": title,
    }

    prompt_text = crlf(f"$ fiz -p '{prompt_display}'\n\n")
    response_text = crlf(
        f"{content}\n[{status}] tokens: {tokens_in} in / {tokens_out} out\n"
    )

    t0 = round(PROMPT_DELAY_S, 6)
    t1 = round(PROMPT_DELAY_S + gap_s, 6)

    lines = [
        json.dumps(header, separators=(", ", ": ")),
        json.dumps([t0, "o", prompt_text]),
        json.dumps([t1, "o", response_text]),
    ]
    return "\n".join(lines) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--in", dest="inp", required=True, type=Path, help="Session JSONL")
    ap.add_argument("--out", dest="out", required=True, type=Path, help="Cast output path")
    ap.add_argument("--prompt", required=True, help="Prompt text shown in the cast header line")
    ap.add_argument("--title", required=True, help="Asciicast title")
    ap.add_argument("--width", type=int, default=80, help="Terminal columns (default 80)")
    ap.add_argument("--height", type=int, default=24, help="Terminal rows (default 24)")
    args = ap.parse_args()

    session = load_session(args.inp)
    cast = render_cast(session, args.prompt, args.title, args.width, args.height)

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(cast)
    print(f"wrote {args.out} ({args.out.stat().st_size} bytes)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
