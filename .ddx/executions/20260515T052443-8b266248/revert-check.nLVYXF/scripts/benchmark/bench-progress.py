#!/usr/bin/env python3
"""bench-progress: read-only status of in-flight benchmark lanes.

Reports for each profile under canonical fiz-tools-v<N>/cells:
  - cell counts (total, valid graded, invalid, by reward)
  - cells written since a chosen since-time (default: 1h ago) for throughput
  - per-running-container JSONL activity (so you don't get fooled by an empty
    /logs/agent/fiz.txt — fiz writes structured trace to .fizeau/sessions/)
  - estimated remaining cells vs --reps target

This script is read-only. It never restarts, kills, or modifies state.
"""
from __future__ import annotations

import argparse
import datetime as dt
import glob
import json
import os
import re
import shlex
import subprocess
import sys
from collections import Counter
from pathlib import Path

HARBOR_TASK_NAME = re.compile(r"^[0-9a-f]{32}__[a-z0-9]+-main-1$")


def repo_root() -> Path:
    here = Path(__file__).resolve()
    for parent in [here.parent] + list(here.parents):
        if (parent / "go.mod").exists() or (parent / ".git").exists():
            return parent
    return Path.cwd()


def fiz_tools_version(repo: Path) -> int:
    src = repo / "internal" / "fiztools" / "version.go"
    if not src.exists():
        return 1
    m = re.search(r"const Version = (\d+)", src.read_text())
    return int(m.group(1)) if m else 1


def canonical_root(repo: Path) -> Path:
    env = os.environ.get("FIZ_BENCHMARK_ROOT")
    if env:
        p = Path(env)
        return p if p.is_absolute() else repo / env
    return repo / "benchmark-results" / f"fiz-tools-v{fiz_tools_version(repo)}"


def docker_ps_task_containers() -> list[tuple[str, dt.datetime]]:
    """Return [(name, created_at), ...] for Harbor TB task containers currently up."""
    out = subprocess.run(
        ["docker", "ps", "--format", "{{.Names}}\t{{.CreatedAt}}"],
        capture_output=True, text=True, check=False,
    ).stdout
    rows: list[tuple[str, dt.datetime]] = []
    for line in out.strip().splitlines():
        parts = line.split("\t", 1)
        if len(parts) != 2:
            continue
        name, ts = parts[0].strip(), parts[1].strip()
        if not HARBOR_TASK_NAME.match(name):
            continue
        try:
            created = dt.datetime.strptime(ts, "%Y-%m-%d %H:%M:%S %z %Z")
        except ValueError:
            try:
                created = dt.datetime.strptime(ts.split(" UTC")[0], "%Y-%m-%d %H:%M:%S %z")
            except Exception:
                created = dt.datetime.now(dt.timezone.utc)
        rows.append((name, created))
    return rows


def container_jsonl_stats(name: str) -> dict:
    """Inspect the live agent's session JSONL inside a running container.

    Returns {'lines', 'bytes', 'event_counts', 'last_type', 'last_ts'} or {} on failure.
    """
    script = (
        'JSONL=$(ls /app/.fizeau/sessions/*.jsonl 2>/dev/null | head -1); '
        'test -n "$JSONL" || JSONL=$(ls /installed-agent/home/.fizeau/sessions/*.jsonl 2>/dev/null | head -1); '
        'if [ -z "$JSONL" ]; then echo NO_JSONL; exit 0; fi; '
        'echo "JSONL=$JSONL"; '
        'wc -lc "$JSONL"; '
        'tail -3 "$JSONL"'
    )
    cp = subprocess.run(
        ["docker", "exec", name, "sh", "-c", script],
        capture_output=True, text=True, check=False, timeout=10,
    )
    text = cp.stdout
    if "NO_JSONL" in text or not text.strip():
        return {}
    lines, size, last_lines = 0, 0, []
    for line in text.splitlines():
        m = re.match(r"\s*(\d+)\s+(\d+)\s+", line)
        if m and lines == 0:
            lines = int(m.group(1))
            size = int(m.group(2))
            continue
        # Collect JSONL lines for last-event peek
        if line.startswith("{") and line.endswith("}"):
            last_lines.append(line)
    counts: Counter = Counter()
    last_type, last_ts = "", ""
    if last_lines:
        try:
            ev = json.loads(last_lines[-1])
            last_type = ev.get("type", "")
            last_ts = (ev.get("ts") or "")[:19]
        except json.JSONDecodeError:
            pass
        for ln in last_lines:
            try:
                counts[json.loads(ln).get("type", "?")] += 1
            except json.JSONDecodeError:
                pass
    return {
        "lines": lines,
        "bytes": size,
        "event_counts": dict(counts),
        "last_type": last_type,
        "last_ts": last_ts,
    }


def _started_at_utc(report: dict) -> dt.datetime | None:
    """Parse report.started_at into an aware UTC datetime, or None on failure."""
    raw = (report.get("started_at") or "").strip()
    if not raw:
        return None
    try:
        return dt.datetime.fromisoformat(raw.replace("Z", "+00:00")).astimezone(dt.timezone.utc)
    except ValueError:
        return None


def lane_cell_stats(canonical: Path, profile: str, since: dt.datetime | None) -> dict:
    """Tally cells under <canonical>/cells/<dataset>/*/<profile>/rep-*/report.json.

    Freshness is keyed on report.started_at (when the cell was actually run),
    NOT file mtime. This matters because bench's --retry-invalid pass touches
    every invalid cell's report.json (rewriting it with the latest classifier
    decision) — those are merely re-evaluated, not re-run, and counting them
    as "fresh" floods the anomaly detector with stale errors.

    Also collects per-cell turn / wall_seconds / error signals over the fresh
    window so the anomaly detector can flag silent quality degradations
    (e.g. "all recent fails are 2-turn fast-fails" → likely config bug,
    not real model failure).
    """
    pattern = str(canonical / "cells" / "terminal-bench-2-1" / "*" / profile / "rep-*" / "report.json")
    paths = glob.glob(pattern)
    total = pas = fail = inv = other = fresh = 0
    fresh_pass = fresh_fail = fresh_inv = 0
    fresh_fail_turns: list[int] = []
    fresh_fail_walls: list[float] = []
    fresh_error_phrases: Counter = Counter()
    for p in paths:
        try:
            r = json.loads(Path(p).read_text())
        except (OSError, json.JSONDecodeError):
            continue
        total += 1
        is_pass = r.get("reward") == 1
        is_fail = r.get("reward") == 0
        is_inv = bool(r.get("invalid_class"))
        if is_pass:
            pas += 1
        elif is_fail:
            fail += 1
        elif is_inv:
            inv += 1
        else:
            other += 1
        # Freshness keyed on started_at, not mtime — see docstring.
        started = _started_at_utc(r)
        if since and started and started > since:
            fresh += 1
            if is_pass:
                fresh_pass += 1
            elif is_fail:
                fresh_fail += 1
                fresh_fail_turns.append(int(r.get("turns") or 0))
                fresh_fail_walls.append(float(r.get("wall_seconds") or 0))
            elif is_inv:
                fresh_inv += 1
            # Bucket recurring error phrases — a single phrase appearing in
            # many cells is the smoking gun of a config bug.
            err = (r.get("error") or "").strip()
            if err:
                # First short tag-like phrase: provider/wire-error markers we know about
                for marker in ("not supported by provider type",
                               "reasoning_wire=none",
                               "agent runtime bundle",
                               "address pools",
                               "binary not found",
                               "asyncio.run()",
                               "Connection refused",
                               "context length"):
                    if marker.lower() in err.lower():
                        fresh_error_phrases[marker] += 1
                        break
    return {
        "total": total,
        "pass": pas,
        "fail": fail,
        "invalid": inv,
        "other": other,
        "fresh": fresh,
        "fresh_pass": fresh_pass,
        "fresh_fail": fresh_fail,
        "fresh_invalid": fresh_inv,
        "fresh_fail_turns": fresh_fail_turns,
        "fresh_fail_walls": fresh_fail_walls,
        "fresh_error_phrases": dict(fresh_error_phrases),
    }


def lane_anomaly(stats: dict) -> list[str]:
    """Return human-readable anomaly tags for a lane's recent activity.

    Conservative — fires fast (low N thresholds) and treats anything
    structurally suspicious as a harness/config issue, not a model
    quality issue. The whole point is to stop us from concluding "this
    model is bad" when really our test rig is broken.

    Heuristics, all tuned to fire as early as 3 cells:
      - ≥3 recent fails averaging ≤2 turns → agent isn't engaging
      - ≥3 recent fails averaging <30s wall → model not producing usable tokens
      - Any error phrase recurs in ≥3 fresh cells → config bug
      - ≥5 fresh cells with 0 passes (and none of the above already firing) → verify
      - ≥3 fresh structurally-suspicious cells (zero output_tokens) → harness/setup issue
    """
    anomalies: list[str] = []
    n_fail = stats["fresh_fail"]
    turns = stats["fresh_fail_turns"]
    walls = stats["fresh_fail_walls"]
    THRESH = 3  # fire fast — operator would rather investigate a false positive than miss a silent regression
    if n_fail >= THRESH and turns:
        avg_turns = sum(turns) / len(turns)
        if avg_turns <= 2.5:
            anomalies.append(
                f"HARNESS/CONFIG SUSPECT: avg_turns={avg_turns:.1f} across {n_fail} recent fails — "
                "agent not engaging; check provider config (sampling, reasoning, model wire format) before concluding model quality"
            )
    if n_fail >= THRESH and walls:
        avg_wall = sum(walls) / len(walls)
        if avg_wall < 30:
            anomalies.append(
                f"HARNESS/CONFIG SUSPECT: avg_wall={avg_wall:.0f}s across {n_fail} recent fails — "
                "model not producing usable tokens; this is almost never a model quality issue"
            )
    for phrase, n in stats["fresh_error_phrases"].items():
        if n >= THRESH:
            anomalies.append(
                f"HARNESS/CONFIG SUSPECT: recurring error '{phrase}' in {n} recent cells — fix config before retrying"
            )
    if stats["fresh"] >= 5 and stats["fresh_pass"] == 0 and not anomalies:
        anomalies.append(
            f"verify lane health: 0 passes in {stats['fresh']} recent cells (could be hard tasks or silent failure)"
        )
    return anomalies


def lane_pid_alive(profile: str) -> tuple[int | None, bool]:
    pid_path = Path(f"/tmp/k5-pid-{profile}")
    if not pid_path.exists():
        return None, False
    try:
        pid = int(pid_path.read_text().strip())
    except ValueError:
        return None, False
    try:
        os.kill(pid, 0)
        return pid, True
    except ProcessLookupError:
        return pid, False


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profiles", default=None,
                        help="Comma-separated profile ids (default: all profiles in canonical/profiles/)")
    parser.add_argument("--since-minutes", type=float, default=60,
                        help="Window for 'fresh' cell counts (default: 60)")
    parser.add_argument("--reps", type=int, default=5,
                        help="Reps target per task for completion percentage (default: 5)")
    parser.add_argument("--tasks", type=int, default=89,
                        help="Tasks per lane for completion percentage (default: 89 for tb21-all)")
    parser.add_argument("--containers", action="store_true",
                        help="Also peek inside each running task container's JSONL")
    parser.add_argument("--json", action="store_true",
                        help="Emit JSON instead of human-readable output")
    args = parser.parse_args()

    repo = repo_root()
    canonical = canonical_root(repo)
    if not canonical.exists():
        print(f"canonical root missing: {canonical}", file=sys.stderr)
        return 1

    if args.profiles:
        profiles = [p.strip() for p in args.profiles.split(",") if p.strip()]
    else:
        profiles = sorted(p.stem for p in (canonical / "profiles").glob("*.yaml"))

    since = dt.datetime.now(dt.timezone.utc) - dt.timedelta(minutes=args.since_minutes)
    target_per_lane = args.reps * args.tasks

    lanes = []
    for prof in profiles:
        pid, alive = lane_pid_alive(prof)
        cells = lane_cell_stats(canonical, prof, since)
        valid_graded = cells["pass"] + cells["fail"]
        rate_per_min = cells["fresh"] / args.since_minutes if args.since_minutes > 0 else 0
        remaining = max(0, target_per_lane - cells["total"])
        eta_min = remaining / rate_per_min if rate_per_min > 0 else None
        anomalies = lane_anomaly(cells)
        lanes.append({
            "profile": prof,
            "pid": pid, "alive": alive,
            "cells": cells, "valid_graded": valid_graded,
            "target": target_per_lane,
            "rate_per_min": rate_per_min,
            "remaining": remaining, "eta_min": eta_min,
            "anomalies": anomalies,
        })

    containers = docker_ps_task_containers() if args.containers else []

    if args.json:
        json.dump({
            "canonical_root": str(canonical),
            "fiz_tools_version": fiz_tools_version(repo),
            "since_minutes": args.since_minutes,
            "lanes": lanes,
            "containers": [
                {"name": n, "created": c.isoformat(), "jsonl": container_jsonl_stats(n)}
                for n, c in containers
            ] if args.containers else [{"name": n, "created": c.isoformat()} for n, c in containers],
        }, sys.stdout, indent=2, default=str)
        print()
        return 0

    print(f"canonical: {canonical}  (fiz_tools v{fiz_tools_version(repo)})")
    print(f"window:    last {args.since_minutes:.0f}m   target: {target_per_lane} cells/lane ({args.reps} reps × {args.tasks} tasks)")
    print()
    print(f"{'profile':<32} {'pid':>7} {'live':>5} {'cells':>6} {'pass':>5} {'fail':>5} {'inv':>5} {'fresh':>6} {'rate/h':>7} {'remain':>7} {'eta':>10}")
    for L in lanes:
        c = L["cells"]
        eta = f"{L['eta_min']/60:.1f}h" if L["eta_min"] else "—"
        print(f"  {L['profile']:<30} {L['pid'] or '—':>7} {('yes' if L['alive'] else 'NO'):>5} "
              f"{c['total']:>6} {c['pass']:>5} {c['fail']:>5} {c['invalid']:>5} "
              f"{c['fresh']:>6} {L['rate_per_min']*60:>7.1f} {L['remaining']:>7} {eta:>10}")
        for a in L.get("anomalies", []):
            print(f"    !! {a}")

    if args.containers and containers:
        print(f"\nrunning task containers ({len(containers)}):")
        for name, created in containers:
            age = (dt.datetime.now(dt.timezone.utc) - created.astimezone(dt.timezone.utc)).total_seconds()
            stats = container_jsonl_stats(name)
            if not stats:
                print(f"  {name[:60]:<60} age={age:>5.0f}s  (no JSONL yet — setup phase)")
                continue
            top = ", ".join(f"{t}={n}" for t, n in sorted(stats["event_counts"].items(), key=lambda x: -x[1])[:3])
            print(f"  {name[:60]:<60} age={age:>5.0f}s  {stats['lines']:>4}L/{stats['bytes']:>6}b  last={stats['last_type']:<14}  top: {top}")
    elif args.containers:
        print("\nno running Harbor task containers")

    return 0


if __name__ == "__main__":
    sys.exit(main())
