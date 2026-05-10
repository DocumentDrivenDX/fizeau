#!/usr/bin/env python3
"""Build asciicast v2 for a non-LLM CLI demo by replaying real captured stdout
with realistic typing/pause delays. No fabrication: every output character is
exactly what the underlying command printed when captured.

Used for reels that wrap pure CLI subcommands (`fiz usage`, `fiz update
--check-only`, `fiz version`, `head | jq` on a session log) — i.e. demos
where there is no LLM loop and therefore no canonical session JSONL to
feed through demos/regen.py.

Usage:
  build-subcommand-cast.py --out OUT --width N --height N --title T \
                           --steps STEPS_JSON

STEPS_JSON: list of {prompt, output_file, post_pause}.
Each step renders as `$ <prompt>\\n<output>\\n` with realistic timing.
"""
import argparse
import json
import sys
from pathlib import Path

PINNED_TS = 1767225600
TYPE_DELAY = 0.04         # per char while typing the command
COMMAND_PAUSE = 0.5       # after Enter, before any output
LINE_GAP = 0.08           # between output lines
DEFAULT_POST_PAUSE = 1.5  # after each command finishes


def crlf(s: str) -> str:
    return s.replace("\r\n", "\n").replace("\n", "\r\n")


def emit(events, t, text):
    events.append([round(t, 6), "o", text])


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", required=True, type=Path)
    ap.add_argument("--width", type=int, default=80)
    ap.add_argument("--height", type=int, default=24)
    ap.add_argument("--title", required=True)
    ap.add_argument("--steps", required=True, type=Path,
                    help="JSON array of {prompt, output_file, post_pause}")
    args = ap.parse_args()

    steps = json.loads(args.steps.read_text())

    header = {
        "version": 2,
        "width": args.width,
        "height": args.height,
        "timestamp": PINNED_TS,
        "env": {"SHELL": "/bin/zsh", "TERM": "screen-256color"},
        "title": args.title,
        "idle_time_limit": 2.0,
    }

    events = []
    t = 0.0
    for step in steps:
        prompt = step["prompt"]
        emit(events, t, "$ ")
        t += 0.15
        for ch in prompt:
            emit(events, t, ch)
            t += TYPE_DELAY
        emit(events, t, "\r\n")
        t += COMMAND_PAUSE

        out_path = Path(step["output_file"])
        out_text = out_path.read_text()
        lines = out_text.split("\n")
        if lines and lines[-1] == "":
            lines = lines[:-1]
        for line in lines:
            emit(events, t, crlf(line + "\n"))
            t += LINE_GAP

        t += step.get("post_pause", DEFAULT_POST_PAUSE)

    out = [json.dumps(header, separators=(", ", ": "))]
    for ev in events:
        out.append(json.dumps(ev))
    args.out.write_text("\n".join(out) + "\n")
    print(f"wrote {args.out} ({len(events)} events, ~{t:.1f}s virtual)",
          file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
