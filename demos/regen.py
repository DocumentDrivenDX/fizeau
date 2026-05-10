#!/usr/bin/env python3
"""Regenerate asciinema cast files from canonical session JSONLs.

Reads a fiz session JSONL and emits an asciicast v2 file mimicking what an
interactive `fiz -p '<prompt>'` invocation would print. No live LLM calls,
no asciinema-rec dependency — fully deterministic.

The renderer supports **time compression** for slow operations (model
downloads, model loads, large LLM responses). Slow segments are replayed
with a banner like

    ⏩  Fast-forward: model load (47.2s → 2.0s)

so the playback stays under ~30 seconds even when the underlying capture
took minutes.

Detection rules (all configurable via CLI flags):

  - Any `llm.response` / `llm.delta` event with `latency_ms > THRESHOLD`
    is replayed compressed. Default threshold: 8000 ms.
  - Any event of type `model.load`, `model.download`, or with
    `data.kind in {"download","model_load"}` is treated as slow.
  - The fast-forward target is `compressed_secs` (default 2.0) regardless
    of original length, with a one-line banner showing the real elapsed
    time so viewers aren't misled.

The output cast also has a header `title` suffixed with
"[time-compressed]" when any compression was applied, and the asciinema
header gets an `idle_time_limit` of 2.0 so even untouched gaps feel snappy.
"""
from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Iterable

# Pinned timestamp so casts are byte-identical across regenerations.
PINNED_TIMESTAMP = 1767225600   # 2026-01-01T00:00:00Z

# Initial paint delay before the prompt line appears.
PROMPT_DELAY_S = 0.005

# Real-time minimum think time (seconds) for short LLM turns so the cast
# feels alive instead of instantaneous.
MIN_RESPONSE_GAP_S = 1.0

# Fast-forward visual marker — wrapped in CSI dim so it stands out without
# wrecking 80-col layout.
FF_BANNER_FMT = "\x1b[2m⏩  Fast-forward: {label} ({real:.1f}s → {compressed:.1f}s)\x1b[0m"

DEFAULT_LATENCY_THRESHOLD_MS = 8000
DEFAULT_COMPRESSED_S = 2.0


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
def crlf(s: str) -> str:
    """Convert lone LFs to CRLFs as a real terminal would render."""
    return s.replace("\r\n", "\n").replace("\n", "\r\n")


@dataclass
class Segment:
    """One emitted asciicast frame plus the *virtual* time it consumes."""
    text: str
    duration_s: float       # how long until the *next* frame may emit
    real_duration_s: float  # original (uncompressed) duration, for tally
    compressed: bool = False
    label: str = ""


@dataclass
class CastPlan:
    width: int
    height: int
    title: str
    segments: list[Segment] = field(default_factory=list)
    # Compression tally for the header banner.
    real_total_s: float = 0.0
    virtual_total_s: float = 0.0
    compressed_count: int = 0


# ---------------------------------------------------------------------------
# Session loading
# ---------------------------------------------------------------------------
def load_events(path: Path) -> list[dict]:
    events: list[dict] = []
    with path.open() as fh:
        for raw in fh:
            raw = raw.strip()
            if not raw:
                continue
            try:
                events.append(json.loads(raw))
            except json.JSONDecodeError:
                continue
    return events


def canonical_triple(events: list[dict]) -> tuple[dict, dict, dict]:
    """Return (start, last_response, end) — back-compat with the original
    minimal-fixture format used by the existing 3 demos."""
    start = response = end = None
    for e in events:
        t = e.get("type")
        if t == "session.start" and start is None:
            start = e
        elif t == "llm.response":
            response = e
        elif t == "session.end":
            end = e
    if start is None or response is None or end is None:
        raise SystemExit(
            "session JSONL: missing session.start / llm.response / session.end"
        )
    return start, response, end


# ---------------------------------------------------------------------------
# Compression rules
# ---------------------------------------------------------------------------
def is_slow_event(event: dict, latency_threshold_ms: int) -> tuple[bool, str]:
    """Return (slow, label) for a single event."""
    t = event.get("type", "")
    data = event.get("data", {}) or {}
    if t in ("model.download", "model.load"):
        return True, t.replace(".", " ")
    kind = data.get("kind", "")
    if kind in ("download", "model_load", "model_download"):
        return True, kind.replace("_", " ")
    if t in ("llm.response", "llm.delta"):
        lat = data.get("latency_ms", 0) or 0
        if lat > latency_threshold_ms:
            label = f"LLM response ({lat/1000:.1f}s)"
            return True, label
    return False, ""


# ---------------------------------------------------------------------------
# Plan construction
# ---------------------------------------------------------------------------
def build_plan(
    events: list[dict],
    prompt_display: str,
    title: str,
    width: int,
    height: int,
    latency_threshold_ms: int,
    compressed_s: float,
    preface: list[tuple[str, float]] | None = None,
) -> CastPlan:
    start, response, end = canonical_triple(events)

    rdata = response.get("data", {}) or {}
    edata = end.get("data", {}) or {}

    content = (rdata.get("content") or edata.get("output") or "").strip()
    usage = edata.get("tokens") or rdata.get("usage") or {}
    tokens_in = usage.get("input", 0)
    tokens_out = usage.get("output", 0)
    status = edata.get("status", "success")

    plan = CastPlan(width=width, height=height, title=title)

    # Optional shell-history preface (e.g. `curl install.sh | bash`). Each
    # entry is (text, virtual-duration-seconds). Real duration is assumed
    # equal to virtual for these (they are typed/scripted, not slow ops).
    if preface:
        for text, dur in preface:
            plan.segments.append(
                Segment(
                    text=crlf(text),
                    duration_s=dur,
                    real_duration_s=dur,
                )
            )

    # Frame N: the actual `$ fiz -p '...'` line.
    plan.segments.append(
        Segment(
            text=crlf(f"$ fiz -p '{prompt_display}'\n\n"),
            duration_s=PROMPT_DELAY_S,
            real_duration_s=PROMPT_DELAY_S,
        )
    )

    # Look at every event for slow-op markers; emit ff banners for each.
    for ev in events:
        slow, label = is_slow_event(ev, latency_threshold_ms)
        if not slow:
            continue
        # Skip the *final* llm.response — that one carries the answer text
        # and is rendered below; we don't want a banner *and* the answer.
        if ev is response:
            continue
        real_s = ((ev.get("data") or {}).get("latency_ms", 0) or 0) / 1000.0
        if real_s <= 0:
            real_s = 1.0
        banner = FF_BANNER_FMT.format(label=label, real=real_s, compressed=compressed_s)
        plan.segments.append(
            Segment(
                text=crlf(banner + "\n"),
                duration_s=compressed_s,
                real_duration_s=real_s,
                compressed=True,
                label=label,
            )
        )
        plan.compressed_count += 1

    # Frame for the final assistant turn — possibly compressed.
    final_lat_ms = rdata.get("latency_ms", 3000)
    final_real_s = max(MIN_RESPONSE_GAP_S, final_lat_ms / 1000.0)
    if final_lat_ms > latency_threshold_ms:
        # Show a banner first, then the answer.
        banner = FF_BANNER_FMT.format(
            label=f"LLM response ({final_lat_ms/1000:.1f}s)",
            real=final_real_s,
            compressed=compressed_s,
        )
        plan.segments.append(
            Segment(
                text=crlf(banner + "\n"),
                duration_s=compressed_s,
                real_duration_s=final_real_s,
                compressed=True,
                label="LLM response",
            )
        )
        plan.compressed_count += 1
        # Answer renders quickly after the banner.
        gap_s = 0.3
    else:
        gap_s = max(MIN_RESPONSE_GAP_S, min(10.0, final_lat_ms / 1000.0))

    response_text = crlf(
        f"{content}\n[{status}] tokens: {tokens_in} in / {tokens_out} out\n"
    )
    plan.segments.append(
        Segment(
            text=response_text,
            duration_s=gap_s,
            real_duration_s=final_real_s if final_lat_ms <= latency_threshold_ms else 0.3,
        )
    )

    # Tally totals. duration_s on segment[i] is the gap *before* segment[i+1],
    # so the last segment's duration doesn't add to virtual time. We follow
    # the same convention for real_duration_s (banner approximates real cost).
    for i, seg in enumerate(plan.segments):
        plan.real_total_s += seg.real_duration_s if i < len(plan.segments) - 1 else 0
        plan.virtual_total_s += seg.duration_s if i < len(plan.segments) - 1 else 0

    if plan.compressed_count > 0:
        plan.title = f"{title} [time-compressed]"

    return plan


# ---------------------------------------------------------------------------
# Asciicast emission
# ---------------------------------------------------------------------------
def render_cast(plan: CastPlan) -> str:
    header: dict[str, Any] = {
        "version": 2,
        "width": plan.width,
        "height": plan.height,
        "timestamp": PINNED_TIMESTAMP,
        "env": {"SHELL": "/bin/zsh", "TERM": "screen-256color"},
        "title": plan.title,
        "idle_time_limit": 2.0,
    }
    lines = [json.dumps(header, separators=(", ", ": "))]

    # If we compressed anything, emit a leading banner explaining the
    # convention so viewers aren't misled.
    t = 0.0
    if plan.compressed_count > 0:
        intro = (
            f"\x1b[2m# time-compressed playback: "
            f"{plan.compressed_count} slow op(s) fast-forwarded "
            f"({plan.real_total_s:.0f}s real → {plan.virtual_total_s:.0f}s shown)"
            f"\x1b[0m\r\n"
        )
        lines.append(json.dumps([round(t, 6), "o", intro]))
        t += 0.4

    for seg in plan.segments:
        lines.append(json.dumps([round(t, 6), "o", seg.text]))
        t += seg.duration_s

    return "\n".join(lines) + "\n"


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------
def main() -> int:
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("--in", dest="inp", required=True, type=Path)
    ap.add_argument("--out", dest="out", required=True, type=Path)
    ap.add_argument("--prompt", required=True)
    ap.add_argument("--title", required=True)
    ap.add_argument("--width", type=int, default=80)
    ap.add_argument("--height", type=int, default=24)
    ap.add_argument(
        "--latency-threshold-ms",
        type=int,
        default=DEFAULT_LATENCY_THRESHOLD_MS,
        help=f"LLM responses slower than this get fast-forwarded "
             f"(default {DEFAULT_LATENCY_THRESHOLD_MS} ms)",
    )
    ap.add_argument(
        "--compressed-s",
        type=float,
        default=DEFAULT_COMPRESSED_S,
        help=f"Virtual playback duration of each fast-forwarded segment "
             f"(default {DEFAULT_COMPRESSED_S} s)",
    )
    ap.add_argument(
        "--no-compress",
        action="store_true",
        help="Disable time compression (legacy behavior).",
    )
    ap.add_argument(
        "--preface",
        type=Path,
        default=None,
        help="Optional JSON file with [[text, virtual_duration_s], ...] frames "
             "to render before the `$ fiz -p ...` prompt line. Used by the "
             "quickstart cast to show install + download steps.",
    )
    args = ap.parse_args()

    threshold = (
        10**12 if args.no_compress else args.latency_threshold_ms
    )

    preface = None
    if args.preface is not None:
        raw = json.loads(args.preface.read_text())
        preface = [(str(t), float(d)) for t, d in raw]

    events = load_events(args.inp)
    plan = build_plan(
        events,
        prompt_display=args.prompt,
        title=args.title,
        width=args.width,
        height=args.height,
        latency_threshold_ms=threshold,
        compressed_s=args.compressed_s,
        preface=preface,
    )
    cast = render_cast(plan)

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(cast)
    print(
        f"wrote {args.out} ({args.out.stat().st_size} bytes, "
        f"{plan.compressed_count} compressed seg(s))",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
