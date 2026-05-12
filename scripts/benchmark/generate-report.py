#!/usr/bin/env python3
"""Generate a self-contained HTML benchmark report at docs/benchmarks/.

Two-step design:
  1. This script computes structured data (aggregates, timings) and renders
     charts. Data lands in docs/benchmarks/data/*.json, charts in
     docs/benchmarks/charts/*.svg. No narrative strings live in this script.
  2. Narrative sections live as markdown in docs/benchmarks/sections/*.md.
     Humans (or LLMs with file tools) edit them. The script slots them into
     the HTML by section number.

Re-running this script regenerates data, charts, and HTML deterministically
from on-disk inputs. Editing a section markdown file and re-running updates
just that prose without recomputing the data.

Inputs (all paths absolute, all read-only to this script):
  benchmark-results/fiz-tools-v1/cells/**/report.json   per-trial summaries
  benchmark-results/fiz-tools-v1/cells/**/sessions/*.jsonl  per-turn timings
  scripts/benchmark/profiles/*.yaml                     lane definitions
  scripts/benchmark/task-subset-tb21-*.yaml             subset definitions
  scripts/benchmark/terminalbench_model_power.json      model-power scale
  benchmark-results/cache/terminalbench-leaderboard-rewards.json  external

Outputs:
  docs/benchmarks/data/aggregates.json     per (profile, subset) rollups
  docs/benchmarks/data/timing.json         per-profile per-bucket TTFT/decode
  docs/benchmarks/data/profiles.json       profile metadata pulled from YAML
  docs/benchmarks/data/leaderboard.json    external pass-rates per submission
  docs/benchmarks/charts/*.svg             matplotlib figures
  docs/benchmarks/terminal-bench-2.1-report.html   final report

Usage:
  generate-report.py                       full rebuild
  generate-report.py --emit-data-only      data + JSON only, no charts/HTML
                                           (useful for LLMs to inspect before
                                           writing narrative)
  generate-report.py --refresh-leaderboard re-fetch reward.txt files from HF
"""

from __future__ import annotations

import argparse
import datetime as dt
import glob
import html
import json
import os
import re
import sys
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from statistics import median, mean
from typing import Any

import yaml

REPO = Path(__file__).resolve().parents[2]
CELLS = REPO / "benchmark-results/fiz-tools-v1/cells/terminal-bench-2-1"
PROFILES_DIR = REPO / "scripts/benchmark/profiles"
SUBSET_GLOB = str(REPO / "scripts/benchmark/task-subset-tb21-*.yaml")
POWER_JSON = REPO / "scripts/benchmark/terminalbench_model_power.json"
MACHINES_YAML = REPO / "scripts/benchmark/machines.yaml"
LEADERBOARD_CACHE = REPO / "benchmark-results/cache/terminalbench-leaderboard-rewards.json"

OUT_ROOT = REPO / "docs/benchmarks"
DATA_DIR = OUT_ROOT / "data"
CHARTS_DIR = OUT_ROOT / "charts"
SECTIONS_DIR = OUT_ROOT / "sections"
OUT_HTML = OUT_ROOT / "terminal-bench-2.1-report.html"

# Hugo microsite integration. The script publishes a page bundle at this path;
# Hugo treats the directory as a single page with co-located assets.
HUGO_BUNDLE = REPO / "website/content/benchmarks/terminal-bench-2-1"
HUGO_LANDING = REPO / "website/content/benchmarks/_index.md"

# Visual palette keyed on provider/runtime, used by both chart code and the HTML.
PALETTE = {
    "openrouter": "#3477eb",
    "openai":     "#1aa25b",
    "anthropic":  "#c2602e",
    "vllm":       "#a445c2",
    "omlx":       "#e8b020",
    "rapid_mlx":  "#b85c4f",
    "external":   "#888888",
    "default":    "#666666",
}

SUBSET_ORDER = ["canary", "openai-cheap", "full", "all"]

# Buckets for context-length analysis. Edit here, both data and charts use it.
CONTEXT_BUCKETS: list[tuple[int, int, str, int]] = [
    # (lo, hi, label, midpoint_for_x_axis)
    (0,        10_000,    "0–10k",    5_000),
    (10_000,   30_000,    "10–30k",   20_000),
    (30_000,   60_000,    "30–60k",   45_000),
    (60_000,   120_000,   "60–120k",  90_000),
    (120_000,  10_000_000,"120k+",    150_000),
]

# Lanes excluded from "active" headlines (kept in raw data).
EXCLUDED_PROFILES = {"noop", "smoke"}


# ---------- low-level helpers ----------

def parse_ts(ts: str | None) -> float | None:
    """ISO-8601 with up to nanosecond precision → POSIX seconds."""
    if not ts:
        return None
    if ts.endswith("Z"):
        ts = ts[:-1] + "+00:00"
    if "." in ts:
        head, dot, rest = ts.rpartition(".")
        sep = None
        for s in ("+", "-"):
            if s in rest[1:]:
                sep = s
                break
        if sep:
            frac, tz = rest.split(sep, 1)
            rest = frac[:6] + sep + tz
        else:
            rest = rest[:6]
        ts = head + dot + rest
    try:
        return datetime.fromisoformat(ts).timestamp()
    except Exception:
        return None


# ---------- input loaders ----------

def load_reports() -> list[dict[str, Any]]:
    """Walk cells/<task>/<profile>/rep-*/report.json. Tag each with task/profile/rep."""
    out: list[dict[str, Any]] = []
    for p in glob.glob(f"{CELLS}/*/*/rep-*/report.json"):
        try:
            r = json.load(open(p))
        except Exception:
            continue
        rel = Path(p).relative_to(CELLS)
        parts = rel.parts
        if len(parts) < 4:
            continue
        r["_task"] = parts[0]
        r["_profile"] = parts[1]
        r["_rep"] = parts[2]
        out.append(r)
    return out


def load_per_turn_timings(profile: str) -> list[dict[str, Any]]:
    """Walk session JSONL files for one profile. Pair each llm.request with its
    first llm.delta and matching llm.response to compute TTFT and decode tok/s."""
    out: list[dict[str, Any]] = []
    for jsonl in glob.glob(f"{CELLS}/*/{profile}/rep-*/*/*/agent/sessions/*.jsonl"):
        try:
            events = [json.loads(l) for l in open(jsonl) if l.strip()]
        except Exception:
            continue
        rel = Path(jsonl).relative_to(CELLS)
        task = rel.parts[0]
        cur_req_ts = None
        first_delta_ts = None
        for e in events:
            t = e.get("type")
            ts = parse_ts(e.get("ts"))
            if t == "llm.request":
                cur_req_ts = ts
                first_delta_ts = None
            elif t == "llm.delta" and cur_req_ts is not None and first_delta_ts is None:
                first_delta_ts = ts
            elif t == "llm.response" and cur_req_ts is not None:
                d = e.get("data") or {}
                usage = d.get("usage") or {}
                in_tok = usage.get("input") or 0
                out_tok = usage.get("output") or 0
                latency_ms = d.get("latency_ms") or 0
                resp_ts = ts
                ttft = (first_delta_ts - cur_req_ts) if (first_delta_ts and cur_req_ts and first_delta_ts > cur_req_ts) else None
                decode_s = (resp_ts - first_delta_ts) if (first_delta_ts and resp_ts and resp_ts > first_delta_ts) else None
                out.append({
                    "task": task,
                    "in_tok": in_tok,
                    "out_tok": out_tok,
                    "latency_ms": latency_ms,
                    "ttft": ttft,
                    "decode_s": decode_s,
                    "decode_tps": (out_tok / decode_s) if (decode_s and out_tok) else None,
                })
                cur_req_ts = None
                first_delta_ts = None
    return out


def load_profiles() -> dict[str, dict[str, Any]]:
    out = {}
    for p in sorted(PROFILES_DIR.glob("*.yaml")):
        try:
            with open(p) as f:
                doc = yaml.safe_load(f)
            if not doc:
                continue
            with open(p) as f:
                first = f.readline().strip()
            doc["_header_comment"] = first.lstrip("# ").strip() if first.startswith("#") else ""
            doc["_path"] = str(p.relative_to(REPO))
            out[doc.get("id") or p.stem] = doc
        except Exception:
            continue
    return out


def load_subsets() -> dict[str, dict[str, Any]]:
    out = {}
    for p in sorted(glob.glob(SUBSET_GLOB)):
        try:
            with open(p) as f:
                doc = yaml.safe_load(f)
        except Exception:
            continue
        if not doc:
            continue
        name = Path(p).stem.replace("task-subset-tb21-", "")
        tasks = [t["id"] for t in doc.get("tasks") or [] if isinstance(t, dict) and t.get("id")]
        out[name] = {
            "name": name,
            "tasks": tasks,
            "selection_rule": doc.get("selection_rule", ""),
            "_path": str(Path(p).relative_to(REPO)),
        }
    return out


def filter_reports_to_declared_subset_tasks(reports: list[dict[str, Any]], subsets: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    """Drop reports whose task id is not in the declared TB-2.1 subset catalog.

    Older runner builds hardcoded the canonical cells dataset path, so aborted
    TB-2.0/bootstrap experiments can be present below the TB-2.1 storage root.
    The website summarizer is scoped to Terminal-Bench 2.1 and should only
    aggregate tasks that appear in the current TB-2.1 manifests.
    """
    known_tasks = {task for subset in subsets.values() for task in subset.get("tasks", [])}
    if not known_tasks:
        return reports
    return [r for r in reports if r.get("_task") in known_tasks]


def load_machines() -> dict[str, dict[str, Any]]:
    """Load the machine registry. Keys are server hostnames matching profile metadata.server."""
    if not MACHINES_YAML.is_file():
        return {}
    try:
        with open(MACHINES_YAML) as f:
            doc = yaml.safe_load(f) or {}
    except Exception:
        return {}
    return doc.get("machines") or {}


def machine_for_profile(profile_meta: dict[str, Any] | None, machines: dict[str, dict[str, Any]]) -> dict[str, Any] | None:
    """Resolve a profile's machine entry via metadata.server (cloud servers return None)."""
    if not profile_meta:
        return None
    md = profile_meta.get("metadata") or {}
    server = md.get("server") or ""
    if not server:
        return None
    return machines.get(server)


def load_model_power() -> dict[str, dict[str, Any]]:
    try:
        d = json.load(open(POWER_JSON))
        return d.get("models") or {}
    except Exception:
        return {}


def load_leaderboard(refresh: bool = False) -> list[dict[str, Any]]:
    """Returns trial records {submission, task_id, reward, tier} from the cached HF leaderboard."""
    if refresh:
        try:
            _refresh_leaderboard()
        except Exception as e:
            print(f"warning: leaderboard refresh failed ({e}); using cached", file=sys.stderr)
    try:
        d = json.load(open(LEADERBOARD_CACHE))
    except Exception:
        return []
    return d.get("trials") or []


def _refresh_leaderboard() -> None:
    """Re-fetch reward.txt files from the HF leaderboard repo, rebuild the cache."""
    from huggingface_hub import HfApi, hf_hub_download
    from concurrent.futures import ThreadPoolExecutor, as_completed
    DATASET = "harborframework/terminal-bench-2-leaderboard"
    api = HfApi()
    print("listing leaderboard files from HF…", file=sys.stderr)
    files = api.list_repo_files(DATASET, repo_type="dataset")
    rewards = [f for f in files if f.endswith("/reward.txt") and "submissions/terminal-bench/2.0/" in f]
    print(f"  found {len(rewards)} reward files", file=sys.stderr)
    cache_dir = LEADERBOARD_CACHE.parent / "_hf_cache"
    trials = []
    def _fetch(rel: str):
        try:
            local = hf_hub_download(DATASET, rel, repo_type="dataset", cache_dir=str(cache_dir))
            return rel, open(local).read().strip()
        except Exception:
            return rel, None
    with ThreadPoolExecutor(max_workers=16) as ex:
        for rel, val in [fut.result() for fut in as_completed([ex.submit(_fetch, r) for r in rewards])]:
            if val is None: continue
            try:
                reward = float(val)
            except ValueError:
                continue
            # Path format: submissions/terminal-bench/2.0/<sub>/<run-id>/<task>__<trial>/verifier/reward.txt
            parts = rel.split("/")
            try:
                idx = parts.index("2.0")
            except ValueError:
                continue
            if idx + 3 >= len(parts):
                continue
            submission = "/".join(parts[:idx + 2])         # ".../2.0/<sub>"
            task_trial = parts[idx + 3]                    # "<task>__<trial>"
            task_id = task_trial.split("__")[0]
            trials.append({"submission": submission, "task_id": task_id, "reward": reward, "tier": "external"})
    LEADERBOARD_CACHE.parent.mkdir(parents=True, exist_ok=True)
    json.dump({"trials": trials}, open(LEADERBOARD_CACHE, "w"))
    print(f"  cached {len(trials)} trial records → {LEADERBOARD_CACHE}", file=sys.stderr)


# ---------- aggregations ----------

def aggregate_per_profile(reports: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    by_profile: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for r in reports:
        by_profile[r["_profile"]].append(r)
    out = {}
    for profile, rs in by_profile.items():
        graded = [r for r in rs if r.get("grading_outcome") == "graded"]
        passed = [r for r in graded if (r.get("reward") or 0) > 0]
        by_task: dict[str, list] = defaultdict(list)
        for r in rs:
            by_task[r["_task"]].append(r)
        tasks_passed_any = sum(1 for ts in by_task.values() if any((x.get("reward") or 0) > 0 for x in ts))
        real = [r for r in rs if (r.get("turns") or 0) > 0
                and ((r.get("input_tokens") or 0) + (r.get("output_tokens") or 0)) > 0]
        invalids: dict[str, int] = defaultdict(int)
        for r in rs:
            if r.get("invalid_class"):
                invalids[r["invalid_class"]] += 1
        # n_truncated: real attempts cut off mid-tool-call (model was
        # actively producing output when the wall budget hit). Distinct
        # from invalid_* (systemic) and graded_fail (claimed-and-failed).
        # Stays in pass@k denominator but flags throughput/budget bottlenecks.
        n_truncated = sum(1 for r in rs if r.get("terminated_mid_work") is True
                          and not r.get("invalid_class"))
        n_finished_clean = sum(1 for r in rs if r.get("terminated_mid_work") is False
                               and not r.get("invalid_class"))
        out[profile] = {
            "n_attempts": len(rs),
            "n_graded": len(graded),
            "n_pass": len(passed),
            "tasks_touched": len(by_task),
            "tasks_passed_any": tasks_passed_any,
            "n_real": len(real),
            "n_truncated": n_truncated,
            "n_finished_clean": n_finished_clean,
            "invalids": dict(invalids),
            "median_turns": median([r["turns"] for r in real]) if real else None,
            "median_in_tok": median([r["input_tokens"] for r in real]) if real else None,
            "median_out_tok": median([r["output_tokens"] for r in real]) if real else None,
            "median_wall": median([r["wall_seconds"] for r in real if r.get("wall_seconds")]) if real else None,
            "avg_cost": (sum(r.get("cost_usd") or 0 for r in real) / len(real)) if real else 0.0,
        }
    return out


def aggregate_per_subset(reports: list[dict[str, Any]], subsets: dict[str, dict[str, Any]]) -> dict[str, dict[str, dict[str, Any]]]:
    out: dict[str, dict[str, dict[str, Any]]] = defaultdict(dict)
    by_pt: dict[str, dict[str, list]] = defaultdict(lambda: defaultdict(list))
    for r in reports:
        by_pt[r["_profile"]][r["_task"]].append(r)
    for subset, info in subsets.items():
        task_set = set(info["tasks"])
        for profile, by_task in by_pt.items():
            attempts_n = 0
            tasks_attempted = 0
            tasks_passed = 0
            for task, rs in by_task.items():
                if task not in task_set: continue
                tasks_attempted += 1
                attempts_n += len(rs)
                if any((x.get("reward") or 0) > 0 for x in rs):
                    tasks_passed += 1
            out[profile][subset] = {
                "n_attempts": attempts_n,
                "tasks_attempted": tasks_attempted,
                "tasks_in_subset": len(task_set),
                "tasks_passed": tasks_passed,
            }
    return out


def aggregate_external_per_subset(leaderboard: list[dict[str, Any]], subsets: dict[str, dict[str, Any]]) -> dict[str, dict[str, dict[str, Any]]]:
    by_st: dict[str, dict[str, list[float]]] = defaultdict(lambda: defaultdict(list))
    for t in leaderboard:
        short = t["submission"].split("/")[-1]
        by_st[short][t["task_id"]].append(t.get("reward") or 0.0)
    out: dict[str, dict[str, dict[str, Any]]] = defaultdict(dict)
    for subset, info in subsets.items():
        task_set = set(info["tasks"])
        for sub, by_task in by_st.items():
            attempted = sum(1 for t in by_task if t in task_set)
            passed = sum(1 for t, rs in by_task.items() if t in task_set and any(r > 0 for r in rs))
            out[sub][subset] = {
                "tasks_attempted": attempted,
                "tasks_in_subset": len(task_set),
                "tasks_passed": passed,
            }
    return out


def subset_pass_counts(per_subset: dict[str, dict[str, dict[str, Any]]], profile_id: str, subset: str = "all") -> tuple[int | None, int | None]:
    """Return pass@k numerator/denominator for one declared subset.

    Profile-wide task directories can include historical canaries, aborted
    experiments, or other benchmark versions that share an old cells root.
    Headline TB-2.1 numbers must therefore come from the subset rollup, not
    from aggregate_per_profile(tasks_touched).
    """
    d = per_subset.get(profile_id, {}).get(subset, {})
    attempted = d.get("tasks_attempted") or 0
    if attempted <= 0:
        return None, None
    return d.get("tasks_passed") or 0, attempted


def compute_per_profile_timing(profiles_to_scan: list[str]) -> dict[str, dict[str, Any]]:
    """For each profile, compute headline TTFT/decode p50 (median-of-task-medians)
    plus per-context-bucket TTFT/decode p50."""
    out: dict[str, dict[str, Any]] = {}
    for pid in profiles_to_scan:
        turns = load_per_turn_timings(pid)
        if not turns:
            out[pid] = {"n_turns": 0, "ttft_p50": None, "decode_tps_p50": None, "buckets": []}
            continue
        # headline
        by_task_ttft: dict[str, list[float]] = defaultdict(list)
        by_task_dtps: dict[str, list[float]] = defaultdict(list)
        for t in turns:
            if t["ttft"] is not None: by_task_ttft[t["task"]].append(t["ttft"])
            if t["decode_tps"] is not None: by_task_dtps[t["task"]].append(t["decode_tps"])
        ttft_med = median([median(v) for v in by_task_ttft.values()]) if by_task_ttft else None
        dtps_med = median([median(v) for v in by_task_dtps.values()]) if by_task_dtps else None
        # per bucket
        bucket_data = []
        for i, (lo, hi, label, mid) in enumerate(CONTEXT_BUCKETS):
            ttft_in = [t["ttft"] for t in turns if t["ttft"] is not None and lo <= (t["in_tok"] or 0) < hi]
            dec_in = [t["decode_tps"] for t in turns if t["decode_tps"] is not None and lo <= (t["in_tok"] or 0) < hi]
            bucket_data.append({
                "label": label, "midpoint": mid, "lo": lo, "hi": hi,
                "n_ttft": len(ttft_in), "ttft_p50": median(ttft_in) if len(ttft_in) >= 5 else None,
                "n_decode": len(dec_in), "decode_tps_p50": median(dec_in) if len(dec_in) >= 5 else None,
            })
        out[pid] = {
            "n_turns": len(turns),
            "ttft_p50": ttft_med,
            "decode_tps_p50": dtps_med,
            "buckets": bucket_data,
        }
    return out


# ---------- color picker ----------

def harness_label(profile_id: str, profile_meta: dict[str, Any] | None) -> str:
    """Identify which agent harness drives this lane's tool-calling loop.

    Looks at the profile YAML for explicit signal first, then falls back to id heuristics:
      - metadata.runtime in {claude-code, codex, pi, opencode} → that harness, native CLI
      - id starts with fiz-harness-<name>- → fiz-wrapped <name> (fiz orchestrates, <name> reasons)
      - everything else (fiz, openrouter, sindri, vidar, bragi, grendel, …) → fiz built-in loop
    """
    md = (profile_meta or {}).get("metadata") or {}
    runtime = (md.get("runtime") or "").lower()
    surface = (md.get("provider_surface") or "").lower()
    NATIVE_LABELS = {
        "claude-code": "Claude Code (native CLI)",
        "codex": "Codex (native CLI)",
        "pi": "Pi (native CLI)",
        "opencode": "OpenCode (native CLI)",
    }
    if runtime in NATIVE_LABELS:
        return NATIVE_LABELS[runtime]
    # native surface clue when runtime field is absent
    for k, v in NATIVE_LABELS.items():
        if surface.endswith(f"{k}-native") or surface == f"{k}-native":
            return v
    pid = profile_id.lower()
    if pid.startswith("fiz-harness-claude"):
        return "Claude Code (wrapped by fiz)"
    if pid.startswith("fiz-harness-codex"):
        return "Codex (wrapped by fiz)"
    if pid.startswith("fiz-harness-pi"):
        return "Pi (wrapped by fiz)"
    if pid.startswith("fiz-harness-opencode"):
        return "OpenCode (wrapped by fiz)"
    return "fiz (built-in agent loop)"


def color_for_profile(profile_id: str, profile_meta: dict[str, Any] | None) -> str:
    if not profile_meta:
        return PALETTE["default"]
    md = (profile_meta.get("metadata") or {})
    pr = ((profile_meta.get("provider") or {}).get("type") or "").lower()
    candidates = [md.get("server", ""), md.get("runtime", ""), md.get("provider_surface", ""), pr]
    for key in candidates:
        for k, v in PALETTE.items():
            if k in key.lower():
                return v
    if "openrouter" in profile_id: return PALETTE["openrouter"]
    if "openai" in profile_id: return PALETTE["openai"]
    if "claude" in profile_id: return PALETTE["anthropic"]
    return PALETTE["default"]


# ---------- chart rendering (matplotlib → SVG files) ----------

def _setup_mpl():
    import matplotlib
    matplotlib.use("svg")
    import matplotlib.pyplot as plt  # noqa
    plt.rcParams.update({
        "font.size": 10,
        "axes.titlesize": 12,
        "axes.labelsize": 10,
        "xtick.labelsize": 9,
        "ytick.labelsize": 9,
        "legend.fontsize": 9,
        "figure.dpi": 100,
        "axes.spines.top": False,
        "axes.spines.right": False,
    })
    return plt


def chart_pass_rate_by_subset(per_subset: dict[str, dict[str, dict[str, Any]]],
                              ext_per_subset: dict[str, dict[str, dict[str, Any]]],
                              profiles: dict[str, dict[str, Any]],
                              out_path: Path) -> None:
    plt = _setup_mpl()
    rows = []
    # Internal first (all profiles with any 'all' attempts), then external (≥30 tasks attempted in 'all')
    internal_pids = sorted(per_subset.keys())
    for pid in internal_pids:
        if pid in EXCLUDED_PROFILES: continue
        d = per_subset[pid].get("all", {})
        if not d.get("tasks_attempted"): continue
        rows.append({
            "label": pid,
            "rate": d["tasks_passed"] / d["tasks_attempted"],
            "color": color_for_profile(pid, profiles.get(pid)),
            "kind": "internal",
            "tag": f'{d["tasks_passed"]}/{d["tasks_attempted"]}',
        })
    ext_sorted = sorted(ext_per_subset.keys(),
                        key=lambda s: -ext_per_subset[s].get("all", {}).get("tasks_attempted", 0))
    for sub in ext_sorted:
        d = ext_per_subset[sub].get("all", {})
        if d.get("tasks_attempted", 0) < 30: continue
        rows.append({
            "label": sub,
            "rate": d["tasks_passed"] / d["tasks_attempted"],
            "color": PALETTE["external"],
            "kind": "external",
            "tag": f'{d["tasks_passed"]}/{d["tasks_attempted"]}',
        })
    if not rows: return
    rows.sort(key=lambda r: r["rate"], reverse=True)
    fig, ax = plt.subplots(figsize=(9, 0.3 * len(rows) + 0.7))
    y = list(range(len(rows)))
    ax.barh(y, [r["rate"] for r in rows], color=[r["color"] for r in rows],
            edgecolor="none", height=0.7)
    ax.set_yticks(y)
    ax.set_yticklabels([r["label"] for r in rows])
    ax.invert_yaxis()
    ax.set_xlim(0, max(0.05, max(r["rate"] for r in rows) * 1.18))
    ax.set_xlabel("pass@k on TB-2.1 'all' subset")
    for i, r in enumerate(rows):
        ax.text(r["rate"] + 0.005, i, f' {r["rate"]*100:.1f}% ({r["tag"]})',
                va="center", ha="left", fontsize=8, color="#333")
    fig.tight_layout()
    fig.savefig(out_path, format="svg", bbox_inches="tight")
    plt.close(fig)


def chart_model_power_scatter(per_profile: dict[str, dict[str, Any]],
                              per_subset: dict[str, dict[str, dict[str, Any]]],
                              ext_per_subset: dict[str, dict[str, dict[str, Any]]],
                              profiles: dict[str, dict[str, Any]],
                              model_power: dict[str, dict[str, Any]],
                              out_path: Path) -> None:
    plt = _setup_mpl()
    def power_for(pid_or_sub: str, meta: dict[str, Any] | None = None) -> int | None:
        candidates = []
        if meta:
            md = meta.get("metadata") or {}
            for k in (md.get("model_id"), md.get("model_family")):
                if k: candidates.append(k.replace(".", "-").replace("/", "-"))
        candidates.append(pid_or_sub)
        for c in candidates:
            for k, v in model_power.items():
                if k.lower() == c.lower(): return v.get("power")
                if c.lower() in k.lower() or k.lower() in c.lower(): return v.get("power")
        # heuristic fallback for our own profiles where catalog match misses
        s = (candidates[0] if candidates else pid_or_sub).lower()
        if "gpt-5.5" in s: return 10
        if "gpt-5.4" in s: return 9
        if "gpt-5.3" in s or "claude-opus" in s or "opus-4" in s: return 9
        if "sonnet-4" in s: return 8
        if "qwen3.6-27b" in s or "qwen3-6-27b" in s: return 7
        return None

    pts = []
    for pid, a in per_profile.items():
        if pid in EXCLUDED_PROFILES: continue
        passed, attempted = subset_pass_counts(per_subset, pid, "all")
        if not attempted or attempted < 5: continue
        meta = profiles.get(pid)
        pwr = power_for(pid, meta)
        if pwr is None: continue
        pts.append({
            "x": pwr + 0.05 * ((hash(pid) % 11) - 5),
            "y": passed / attempted,
            "label": pid.replace("fiz-", "").replace("-qwen3-6-27b", "/Q27b").replace("-claude-sonnet-4-6", "/Sonnet"),
            "color": color_for_profile(pid, meta),
            "size": 30 + (a["median_turns"] or 0) * 4,
            "kind": "internal",
        })
    for sub, by_sub in ext_per_subset.items():
        d = by_sub.get("all", {})
        if not d.get("tasks_attempted"): continue
        pwr = power_for(sub.split("__")[-1])
        if pwr is None: continue
        pts.append({
            "x": pwr + 0.05 * ((hash(sub) % 11) - 5),
            "y": d["tasks_passed"] / d["tasks_attempted"],
            "label": sub.split("__")[-1],
            "color": PALETTE["external"],
            "size": 50,
            "kind": "external",
        })
    if not pts: return
    fig, ax = plt.subplots(figsize=(9, 5.5))
    for p in pts:
        ax.scatter(p["x"], p["y"], s=p["size"], c=p["color"],
                   alpha=0.55, edgecolors=p["color"], linewidths=1)
    for p in pts:
        ax.annotate(p["label"], (p["x"], p["y"]), xytext=(0, 6),
                    textcoords="offset points", ha="center", fontsize=7.5, color="#222")
    ax.set_xlim(0.5, 10.5)
    ax.set_xticks(range(1, 11))
    ax.set_ylim(0, max(0.05, max(p["y"] for p in pts) * 1.15))
    ax.set_xlabel("Model power (1 weak — 10 frontier)")
    ax.set_ylabel("pass@k on TB-2.1 'all'")
    ax.yaxis.set_major_formatter(plt.FuncFormatter(lambda v, _: f"{v*100:.0f}%"))
    ax.grid(axis="y", linestyle=":", alpha=0.5)
    fig.tight_layout()
    fig.savefig(out_path, format="svg", bbox_inches="tight")
    plt.close(fig)


def chart_lines_over_context(timing: dict[str, dict[str, Any]],
                             profiles: dict[str, dict[str, Any]],
                             metric: str,
                             y_label: str,
                             out_path: Path) -> None:
    """metric = 'ttft_p50' | 'decode_tps_p50'"""
    plt = _setup_mpl()
    fig, ax = plt.subplots(figsize=(9, 4.5))
    plotted = False
    for pid in sorted(timing.keys()):
        if pid in EXCLUDED_PROFILES: continue
        buckets = timing[pid]["buckets"]
        xs = [b["midpoint"] for b in buckets if b[metric] is not None]
        ys = [b[metric] for b in buckets if b[metric] is not None]
        if len(xs) < 2: continue
        color = color_for_profile(pid, profiles.get(pid))
        ax.plot(xs, ys, marker="o", color=color, label=pid, linewidth=1.6)
        plotted = True
    if not plotted: return
    bucket_labels = [b[2] for b in CONTEXT_BUCKETS]
    bucket_xs = [b[3] for b in CONTEXT_BUCKETS]
    ax.set_xticks(bucket_xs)
    ax.set_xticklabels(bucket_labels)
    ax.set_xlabel("Input tokens (bucket midpoint)")
    ax.set_ylabel(y_label)
    ax.grid(linestyle=":", alpha=0.5)
    ax.legend(bbox_to_anchor=(1.02, 1), loc="upper left", borderaxespad=0., frameon=False, fontsize=8)
    fig.tight_layout()
    fig.savefig(out_path, format="svg", bbox_inches="tight")
    plt.close(fig)


# ---------- HTML composition ----------

def _md_to_html(text: str) -> str:
    import markdown
    return markdown.markdown(text, extensions=["fenced_code", "tables"])


def _read_section(name: str) -> str:
    p = SECTIONS_DIR / name
    if p.is_file():
        return _md_to_html(p.read_text(encoding="utf-8"))
    # placeholder for missing section
    return f'<p style="color:#a44">[missing section: {html.escape(name)} — create at {p.relative_to(REPO)}]</p>'


def _read_svg_inline(p: Path) -> str:
    if not p.is_file():
        return f'<p style="color:#a44">[missing chart: {html.escape(str(p.relative_to(REPO)))}]</p>'
    text = p.read_text(encoding="utf-8")
    # Strip the <?xml … and DOCTYPE if matplotlib emits them
    text = re.sub(r"<\?xml[^?]*\?>\s*", "", text)
    text = re.sub(r"<!DOCTYPE[^>]*>\s*", "", text)
    return text


REPORT_CSS = """
  .br-body { font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; color: #222; }
  .br-body h2 { font-size: 20px; margin-top: 36px; padding-bottom: 4px; border-bottom: 1px solid #ddd; }
  .br-body h3 { font-size: 16px; margin-top: 22px; }
  .br-body table { border-collapse: collapse; font-size: 12px; margin: 12px 0; }
  .br-body th, .br-body td { padding: 5px 9px; border-bottom: 1px solid #eee; text-align: right; }
  .br-body th:first-child, .br-body td:first-child { text-align: left; }
  .br-body th { background: #f7f7f7; font-weight: 600; }
  .br-body tr.external td { color: #555; background: #fafafa; }
  .br-body tr.section-divider td { background: #f0f0f0; font-weight: 600; text-align: left; }
  .br-body tr:hover td { background: #f6f9ff; }
  .br-body .meta { color: #666; font-size: 12px; }
  .br-body .pill { display: inline-block; font-size: 11px; padding: 1px 7px; border-radius: 9px; background: #eef; color: #335; margin-right: 4px; }
  .br-body .pill.warn { background: #fee; color: #844; }
  .br-body .profile-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 10px; margin-top: 10px; }
  .br-body .profile-card { border: 1px solid #e3e3e3; padding: 10px 14px; border-radius: 6px; }
  .br-body .profile-card h4 { margin: 0 0 4px 0; font-size: 14px; }
  .br-body .profile-card .desc { color: #555; font-size: 12px; margin-top: 6px; }
  .br-body .machine { margin-top: 8px; padding: 6px 8px; background: #f6f8fb; border-radius: 4px; font-size: 11.5px; }
  .br-body .machine b { font-size: 11px; color: #335; text-transform: uppercase; letter-spacing: 0.04em; }
  .br-body .machine dl { display: grid; grid-template-columns: 70px 1fr; gap: 1px 8px; margin: 4px 0 0 0; }
  .br-body .machine dt { color: #888; }
  .br-body .machine dd { margin: 0; color: #222; }
  .br-body .narrative { max-width: 880px; }
  .br-body .narrative ul { margin-left: 20px; }
  .br-body code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; background: #f3f3f3; padding: 1px 4px; border-radius: 3px; }
  .br-body .chart { margin: 12px 0; max-width: 1100px; }
  .br-body .chart svg, .br-body .chart img { max-width: 100%; height: auto; }
"""


def _chart_ref_inline(p: Path) -> str:
    """Return inline <svg> for standalone HTML output."""
    if not p.is_file():
        return f'<p style="color:#a44">[missing chart: {html.escape(str(p.relative_to(REPO)))}]</p>'
    text = p.read_text(encoding="utf-8")
    text = re.sub(r"<\?xml[^?]*\?>\s*", "", text)
    text = re.sub(r"<!DOCTYPE[^>]*>\s*", "", text)
    return text


def _chart_ref_url(name: str) -> str:
    """Return <img> tag for Hugo page bundle output."""
    return f'<img src="charts/{html.escape(name)}" alt="{html.escape(name)}">'


# ---- profile filters used by the multi-page IA ----

def _filter_models_lanes(per_profile, profiles):
    """Lanes that exercise different MODELS through the same harness (fiz built-in
    or native CLI). Excludes provider variants of the same model and Qwen lanes
    (those live on the providers page)."""
    out = []
    for pid, _ in per_profile.items():
        if pid in EXCLUDED_PROFILES: continue
        prof = profiles.get(pid) or {}
        meta = prof.get("metadata") or {}
        family = (meta.get("model_family") or "").lower()
        # Skip qwen lanes — those are the providers page.
        if "qwen" in family: continue
        # Skip wrapper lanes — those are the harnesses page.
        if pid.startswith("fiz-harness-"): continue
        out.append(pid)
    return sorted(out)


def _filter_harness_lanes(per_profile, profiles):
    """Lanes that exercise the same MODEL through different HARNESSES. Includes
    native CLI lanes (claude-native-*, codex-native-*) and fiz-harness-* wrappers,
    plus the corresponding fiz built-in lanes that share the model so direct
    side-by-side reads are possible."""
    out = []
    for pid in per_profile.keys():
        if pid in EXCLUDED_PROFILES: continue
        prof = profiles.get(pid) or {}
        meta = prof.get("metadata") or {}
        family = (meta.get("model_family") or "").lower()
        if "qwen" in family: continue
        # Include if it's a wrapper, a native CLI lane, or a fiz lane on a model
        # that ALSO appears in some wrapper/native lane.
        if pid.startswith("fiz-harness-") or "native" in pid:
            out.append(pid)
            continue
        # Direct fiz-on-X for any X that appears in the wrapper set.
        if any((profiles.get(p) or {}).get("metadata", {}).get("model_family") == family
               for p in per_profile if p.startswith("fiz-harness-") or "native" in p):
            out.append(pid)
    return sorted(out)


def _filter_provider_lanes(per_profile, profiles):
    """Qwen3.6-27B lanes only — same model, different provider/runtime."""
    out = []
    for pid in per_profile.keys():
        if pid in EXCLUDED_PROFILES: continue
        prof = profiles.get(pid) or {}
        meta = prof.get("metadata") or {}
        family = (meta.get("model_family") or "").lower()
        if "qwen" in family:
            out.append(pid)
    return sorted(out)


def _provider_descriptive_label(pid: str, profile_meta: dict | None, machine: dict | None) -> str:
    """Generate a descriptive (no-hostname) label for a provider lane.
    Examples:
      - "OpenRouter (cloud aggregator)"
      - "vLLM int4 / RTX 5090 Ti / Linux + WSL2"
      - "oMLX 8-bit / Apple M2 Ultra (192 GB unified)"
    """
    md = (profile_meta or {}).get("metadata") or {}
    pr = (profile_meta or {}).get("provider") or {}
    runtime = (md.get("runtime") or pr.get("type") or "").lower()
    if runtime == "openrouter":
        return "OpenRouter (cloud aggregator)"
    if runtime == "openai":
        return "OpenAI (native API)"
    if runtime == "anthropic":
        return "Anthropic (native API)"
    quant = md.get("quant_label") or ""
    parts = []
    if "vllm" in runtime: parts.append("vLLM")
    elif "omlx" in runtime: parts.append("oMLX")
    elif "rapid" in runtime: parts.append("RapidMLX")
    else: parts.append(runtime or "?")
    # Quant suffix
    if "int4" in quant or "autoround" in quant: parts[-1] += " int4"
    elif "8bit" in quant or "8-bit" in quant: parts[-1] += " 8-bit"
    elif "4bit" in quant or "4-bit" in quant: parts[-1] += " 4-bit"
    if machine:
        gpu = machine.get("gpu") or ""
        cpu = machine.get("cpu") or ""
        os_ = machine.get("os") or ""
        if gpu and "Apple" not in gpu: parts.append(gpu)
        elif cpu and "Apple" in cpu: parts.append(f"{cpu}" + (f" ({machine.get('memory')})" if machine.get("memory") else ""))
        if os_ and "Apple" not in (cpu or ""): parts.append(os_)
    return " / ".join(parts)


def render_body(*,
                profiles: dict[str, dict[str, Any]],
                machines: dict[str, dict[str, Any]],
                subsets: dict[str, dict[str, Any]],
                per_profile: dict[str, dict[str, Any]],
                per_subset: dict[str, dict[str, dict[str, Any]]],
                ext_per_subset: dict[str, dict[str, dict[str, Any]]],
                timing: dict[str, dict[str, Any]],
                snapshot_ts: str,
                n_reports: int,
                chart_emitter) -> str:
    """Build the (legacy single-page) report body — kept for the standalone
    HTML at docs/benchmarks/. The Hugo bundle uses the per-page renderers
    below: render_overview_body, render_models_body, render_harnesses_body,
    render_providers_body."""

    pid_active = sorted(p for p in per_profile if p not in EXCLUDED_PROFILES)
    ext_visible = sorted(
        [s for s in ext_per_subset if ext_per_subset[s].get("all", {}).get("tasks_attempted", 0) >= 30],
        key=lambda s: -ext_per_subset[s].get("all", {}).get("tasks_passed", 0),
    )

    parts: list[str] = []
    parts.append(f'<div class="meta">Snapshot: {html.escape(snapshot_ts)} · {n_reports:,} trial reports · {len(pid_active)} active lanes · external comparators from <code>harborframework/terminal-bench-2-leaderboard</code></div>')

    # (Former §1 "What is Fizeau" was lifted to the homepage; the report
    # starts at the benchmark setup.)

    # Section 1
    parts.append('<h2>1. Terminal-Bench 2.1 and how we run it</h2>')
    parts.append(f'<div class="narrative">{_read_section("02-terminal-bench.md")}</div>')
    parts.append('<table><thead><tr><th>Subset</th><th>Tasks</th><th>Selection rule</th></tr></thead><tbody>')
    for name in SUBSET_ORDER:
        info = subsets.get(name)
        if not info: continue
        parts.append(f'<tr><td>{html.escape(name)}</td><td>{len(info["tasks"])}</td><td>{html.escape(info.get("selection_rule",""))}</td></tr>')
    parts.append('</tbody></table>')

    # Section 3
    parts.append('<h2>2. Profile catalog</h2>')
    parts.append(f'<div class="narrative">{_read_section("03-profiles-intro.md")}</div>')
    parts.append('<div class="profile-grid">')
    for pid in pid_active:
        prof = profiles.get(pid) or {}
        agg = per_profile.get(pid, {})
        meta = prof.get("metadata") or {}
        provider = prof.get("provider") or {}
        sampling = prof.get("sampling") or {}
        pricing = prof.get("pricing") or {}
        rationale = prof.get("_header_comment", "")
        color = color_for_profile(pid, prof)
        harness = harness_label(pid, prof)
        parts.append(f'<div class="profile-card" style="border-left: 4px solid {color}">')
        parts.append(f'<h4>{html.escape(pid)}</h4>')
        parts.append(f'<div style="font-size:12px;margin:2px 0 6px 0;"><b>harness:</b> {html.escape(harness)}</div>')
        bits = []
        if meta.get("model_id"): bits.append(f'model <code>{html.escape(str(meta["model_id"]))}</code>')
        if meta.get("quant_label"): bits.append(f'<span class="pill">{html.escape(str(meta["quant_label"]))}</span>')
        if meta.get("server"): bits.append(f'host <code>{html.escape(str(meta["server"]))}</code>')
        if provider.get("type"): bits.append(f'provider <code>{html.escape(provider["type"])}</code>')
        parts.append('<div>' + ' · '.join(bits) + '</div>')
        if sampling:
            parts.append(f'<div class="meta">sampling: {html.escape(json.dumps(sampling, separators=(",", ":")))}</div>')
        if pricing.get("input_usd_per_mtok") is not None:
            parts.append(f'<div class="meta">pricing: ${pricing.get("input_usd_per_mtok",0):g} in / ${pricing.get("output_usd_per_mtok",0):g} out per Mtok</div>')
        if rationale:
            parts.append(f'<div class="desc">{html.escape(rationale)}</div>')
        # Hardware (only for self-hosted lanes whose server resolves in the machine registry)
        machine = machine_for_profile(prof, machines)
        if machine:
            hw_rows: list[tuple[str, str]] = []
            for k in ("chassis", "cpu", "gpu", "memory", "os", "network"):
                v = machine.get(k) or ""
                if v:
                    hw_rows.append((k, str(v)))
            if hw_rows or machine.get("notes"):
                parts.append('<div class="machine"><b>hardware</b>')
                parts.append('<dl>')
                for k, v in hw_rows:
                    parts.append(f'<dt>{html.escape(k)}</dt><dd>{html.escape(v)}</dd>')
                parts.append('</dl>')
                if machine.get("notes"):
                    parts.append(f'<div class="meta" style="margin-top:4px;">{html.escape(machine["notes"])}</div>')
                parts.append('</div>')
        if agg:
            pass1 = (agg["n_pass"]/agg["n_graded"]*100) if agg["n_graded"] else 0
            parts.append(f'<div class="meta" style="margin-top:6px;">attempts: <b>{agg["n_attempts"]}</b> · graded: {agg["n_graded"]} · pass@1: <b>{pass1:.1f}%</b></div>')
        parts.append('</div>')
    parts.append('</div>')

    # Section 4
    parts.append('<h2>3. Pass-rate summary by subset</h2>')
    parts.append(f'<div class="narrative">{_read_section("04-pass-rate-narrative.md")}</div>')
    parts.append('<table><thead><tr><th>Profile / Submission</th>')
    for s in SUBSET_ORDER:
        parts.append(f'<th>{s} ({len(subsets.get(s,{}).get("tasks",[]))} tasks)</th>')
    parts.append('<th>Provider</th></tr></thead><tbody>')
    for pid in pid_active:
        prof = profiles.get(pid) or {}
        provider_type = (prof.get("provider") or {}).get("type") or ""
        ags = per_subset.get(pid, {})
        cells = [f'<td>{html.escape(pid)}</td>']
        for s in SUBSET_ORDER:
            d = ags.get(s)
            if not d or d["tasks_attempted"] == 0:
                cells.append('<td><span class="pill warn">no data</span></td>')
            else:
                cells.append(f'<td>{d["tasks_passed"]/d["tasks_attempted"]*100:.1f}% <span class="meta">({d["tasks_passed"]}/{d["tasks_attempted"]})</span></td>')
        cells.append(f'<td><span class="meta">{html.escape(provider_type)}</span></td>')
        parts.append('<tr>' + ''.join(cells) + '</tr>')
    parts.append(f'<tr class="section-divider"><td colspan="{len(SUBSET_ORDER)+2}">External leaderboard (HF: harborframework/terminal-bench-2-leaderboard)</td></tr>')
    for sub in ext_visible:
        ags = ext_per_subset.get(sub, {})
        cells = [f'<td>{html.escape(sub)}</td>']
        for s in SUBSET_ORDER:
            d = ags.get(s, {})
            if not d.get("tasks_attempted"):
                cells.append('<td><span class="pill warn">no data</span></td>')
            else:
                cells.append(f'<td>{d["tasks_passed"]/d["tasks_attempted"]*100:.1f}% <span class="meta">({d["tasks_passed"]}/{d["tasks_attempted"]})</span></td>')
        cells.append('<td><span class="meta">external</span></td>')
        parts.append('<tr class="external">' + ''.join(cells) + '</tr>')
    parts.append('</tbody></table>')
    parts.append(f'<div class="chart">{chart_emitter("pass-rate.svg")}</div>')

    # Section 5
    parts.append('<h2>4. Detailed metrics by lane (TB-2.1 \'all\' subset)</h2>')
    parts.append(f'<div class="narrative">{_read_section("05-detailed-metrics-intro.md")}</div>')
    parts.append('<table><thead><tr><th>Profile</th><th>Harness</th><th>Attempts</th><th>Real runs</th>'
                 '<th>pass@1</th><th>pass@k</th>'
                 '<th>med turns</th><th>med in (tok)</th><th>med out (tok)</th>'
                 '<th>med wall (s)</th><th>avg cost ($)</th>'
                 '<th>p50 TTFT (s)</th><th>p50 decode (tok/s)</th></tr></thead><tbody>')
    def _fmt_int(x): return "—" if x is None else f"{int(x):,}"
    def _fmt_num(x, dp=1): return "—" if x is None else f"{x:.{dp}f}"
    def _fmt_pct(p, t): return "—" if not t else f"{p/t*100:.1f}%"
    for pid in pid_active:
        a = per_profile[pid]
        td = timing.get(pid, {})
        passed, attempted = subset_pass_counts(per_subset, pid, "all")
        parts.append('<tr>')
        parts.append(f'<td>{html.escape(pid)}</td>')
        parts.append(f'<td><span class="meta">{html.escape(harness_label(pid, profiles.get(pid)))}</span></td>')
        parts.append(f'<td>{a["n_attempts"]}</td><td>{a["n_real"]}</td>')
        parts.append(f'<td>{_fmt_pct(a["n_pass"], a["n_graded"])}</td>')
        parts.append(f'<td>{_fmt_pct(passed, attempted)}</td>')
        parts.append(f'<td>{_fmt_num(a["median_turns"], 0)}</td>')
        parts.append(f'<td>{_fmt_int(a["median_in_tok"])}</td>')
        parts.append(f'<td>{_fmt_int(a["median_out_tok"])}</td>')
        parts.append(f'<td>{_fmt_num(a["median_wall"], 0)}</td>')
        parts.append(f'<td>{a["avg_cost"]:.3f}</td>')
        parts.append(f'<td>{_fmt_num(td.get("ttft_p50"), 2)}</td>')
        parts.append(f'<td>{_fmt_num(td.get("decode_tps_p50"), 1)}</td>')
        parts.append('</tr>')
    parts.append('</tbody></table>')

    # Section 6
    parts.append('<h2>5. Model power vs pass rate</h2>')
    parts.append(f'<div class="narrative">{_read_section("06-model-power-observations.md")}</div>')
    parts.append(f'<div class="chart">{chart_emitter("model-power-scatter.svg")}</div>')

    # Section 7
    parts.append('<h2>6. Performance vs context length</h2>')
    parts.append(f'<div class="narrative">{_read_section("07-context-length-observations.md")}</div>')
    parts.append(f'<h3>TTFT (seconds, lower is better)</h3><div class="chart">{chart_emitter("ttft-by-context.svg")}</div>')
    parts.append(f'<h3>Decode tok/s (higher is better)</h3><div class="chart">{chart_emitter("decode-by-context.svg")}</div>')

    # Section 8
    parts.append('<h2>7. Observations and conclusions</h2>')
    parts.append(f'<div class="narrative">{_read_section("08-conclusions.md")}</div>')

    # Method notes (always last)
    parts.append('<h2>Method notes</h2>')
    parts.append(f'<div class="narrative">{_read_section("method-notes.md")}</div>')

    return "\n".join(parts)


# ============================================================================
# Per-page renderers for the multi-page Hugo bundle.
#
# Each function emits the BODY (no front matter) for one Hugo page. They share
# the same chart_emitter signature as render_body. Charts are referenced via
# absolute Hugo URLs (chart_emitter_abs) so sub-pages can reach the parent
# bundle's charts/ dir.
# ============================================================================

def _render_subset_table(subsets) -> str:
    s = ['<table><thead><tr><th>Subset</th><th>Tasks</th><th>Selection rule</th></tr></thead><tbody>']
    for name in SUBSET_ORDER:
        info = subsets.get(name)
        if not info: continue
        s.append(f'<tr><td>{html.escape(name)}</td><td>{len(info["tasks"])}</td><td>{html.escape(info.get("selection_rule",""))}</td></tr>')
    s.append('</tbody></table>')
    return "".join(s)


def _render_pass_table(pids: list[str], subsets, per_subset, profiles,
                       include_external: bool = False, ext_per_subset: dict | None = None,
                       ext_filter=None) -> str:
    """Pass@k table for the given profile ids. Optionally append external rows."""
    s = ['<table><thead><tr><th>Profile / Submission</th>']
    for sub in SUBSET_ORDER:
        s.append(f'<th>{sub} ({len(subsets.get(sub,{}).get("tasks",[]))} tasks)</th>')
    s.append('<th>Provider</th></tr></thead><tbody>')
    for pid in pids:
        prof = profiles.get(pid) or {}
        provider_type = (prof.get("provider") or {}).get("type") or ""
        ags = per_subset.get(pid, {})
        cells = [f'<td>{html.escape(pid)}</td>']
        for sub in SUBSET_ORDER:
            d = ags.get(sub)
            if not d or d["tasks_attempted"] == 0:
                cells.append('<td><span class="pill warn">no data</span></td>')
            else:
                cells.append(f'<td>{d["tasks_passed"]/d["tasks_attempted"]*100:.1f}% <span class="meta">({d["tasks_passed"]}/{d["tasks_attempted"]})</span></td>')
        cells.append(f'<td><span class="meta">{html.escape(provider_type)}</span></td>')
        s.append('<tr>' + ''.join(cells) + '</tr>')
    if include_external and ext_per_subset:
        ext_rows = sorted(
            [sub for sub in ext_per_subset if ext_per_subset[sub].get("all", {}).get("tasks_attempted", 0) >= 30],
            key=lambda x: -ext_per_subset[x].get("all", {}).get("tasks_passed", 0),
        )
        if ext_filter:
            ext_rows = [sub for sub in ext_rows if ext_filter(sub)]
        if ext_rows:
            s.append(f'<tr class="section-divider"><td colspan="{len(SUBSET_ORDER)+2}">External leaderboard (HF)</td></tr>')
            for sub in ext_rows:
                ags = ext_per_subset.get(sub, {})
                cells = [f'<td>{html.escape(sub)}</td>']
                for ssub in SUBSET_ORDER:
                    d = ags.get(ssub, {})
                    if not d.get("tasks_attempted"):
                        cells.append('<td><span class="pill warn">no data</span></td>')
                    else:
                        cells.append(f'<td>{d["tasks_passed"]/d["tasks_attempted"]*100:.1f}% <span class="meta">({d["tasks_passed"]}/{d["tasks_attempted"]})</span></td>')
                cells.append('<td><span class="meta">external</span></td>')
                s.append('<tr class="external">' + ''.join(cells) + '</tr>')
    s.append('</tbody></table>')
    return "".join(s)


def _render_detailed_table(pids: list[str], per_profile, per_subset, profiles, timing, label_for=None) -> str:
    def _fmt_int(x): return "—" if x is None else f"{int(x):,}"
    def _fmt_num(x, dp=1): return "—" if x is None else f"{x:.{dp}f}"
    def _fmt_pct(p, t): return "—" if not t else f"{p/t*100:.1f}%"
    s = ['<table><thead><tr><th>Profile</th><th>Harness</th><th>Attempts</th><th>Real</th>'
         '<th>pass@1</th><th>pass@k</th><th>med turns</th><th>med in</th><th>med out</th>'
         '<th>med wall (s)</th><th>cost ($)</th><th>p50 TTFT (s)</th><th>p50 decode (tok/s)</th></tr></thead><tbody>']
    for pid in pids:
        a = per_profile[pid]
        td = timing.get(pid, {})
        prof = profiles.get(pid)
        label = label_for(pid, prof) if label_for else pid
        passed, attempted = subset_pass_counts(per_subset, pid, "all")
        s.append('<tr>')
        s.append(f'<td>{html.escape(label)}</td>')
        s.append(f'<td><span class="meta">{html.escape(harness_label(pid, prof))}</span></td>')
        s.append(f'<td>{a["n_attempts"]}</td><td>{a["n_real"]}</td>')
        s.append(f'<td>{_fmt_pct(a["n_pass"], a["n_graded"])}</td>')
        s.append(f'<td>{_fmt_pct(passed, attempted)}</td>')
        s.append(f'<td>{_fmt_num(a["median_turns"], 0)}</td>')
        s.append(f'<td>{_fmt_int(a["median_in_tok"])}</td>')
        s.append(f'<td>{_fmt_int(a["median_out_tok"])}</td>')
        s.append(f'<td>{_fmt_num(a["median_wall"], 0)}</td>')
        s.append(f'<td>{a["avg_cost"]:.3f}</td>')
        s.append(f'<td>{_fmt_num(td.get("ttft_p50"), 2)}</td>')
        s.append(f'<td>{_fmt_num(td.get("decode_tps_p50"), 1)}</td>')
        s.append('</tr>')
    s.append('</tbody></table>')
    return "".join(s)


def render_overview_body(*, snapshot_ts, n_reports, profiles, subsets,
                         per_profile, per_subset, ext_per_subset, timing,
                         chart_emitter) -> str:
    pid_active = sorted(p for p in per_profile if p not in EXCLUDED_PROFILES)
    parts = [
        f'<div class="meta">Snapshot: {html.escape(snapshot_ts)} · {n_reports:,} trial reports · {len(pid_active)} active lanes</div>',
        '<h2>How we run it</h2>',
        f'<div class="narrative">{_read_section("02-terminal-bench.md")}</div>',
        _render_subset_table(subsets),
        '<h2>Three perspectives on the same data</h2>',
        '<div class="narrative">',
        '<p>The trial set runs each task many ways. Each of the three sub-pages slices the data along a different axis:</p>',
        '<ul>',
        '<li><a href="models/"><b>Models</b></a> — fiz with its built-in agent loop across multiple models, on the cheap subset where cost lets us run real reps.</li>',
        '<li><a href="harnesses/"><b>Harnesses</b></a> — same model, different agent loop. Includes external leaderboard rows for the same models in other harnesses.</li>',
        '<li><a href="providers/"><b>Providers</b></a> — Qwen3.6-27B held constant, varying the host (cloud aggregator vs local CUDA vs Apple silicon). The harness/runtime story.</li>',
        '</ul>',
        '</div>',
        '<h2>Headline observations</h2>',
        f'<div class="narrative">{_read_section("08-conclusions.md")}</div>',
        '<h2>Method notes</h2>',
        f'<div class="narrative">{_read_section("method-notes.md")}</div>',
    ]
    return "\n".join(parts)


def render_models_body(*, snapshot_ts, profiles, subsets, per_profile,
                       per_subset, ext_per_subset, timing, chart_emitter) -> str:
    pids = _filter_models_lanes(per_profile, profiles)
    parts = [
        f'<div class="meta">Snapshot: {html.escape(snapshot_ts)} · {len(pids)} model lanes shown</div>',
        '<div class="narrative">',
        '<p>Each row is fiz running its own built-in agent loop against a different model. Where possible we report on the <code>openai-cheap</code> subset (35 tasks) so the cost gate doesn\'t bias the model selection — frontier hosted models are typically too expensive to run with k=5 reps across all 89 TB-2.1 tasks.</p>',
        '</div>',
        '<h2>Pass-rate</h2>',
        _render_pass_table(pids, subsets, per_subset, profiles),
        '<h2>Detailed metrics</h2>',
        _render_detailed_table(pids, per_profile, per_subset, profiles, timing),
        '<h2>Cost to extend coverage</h2>',
        f'<div class="narrative">{_read_section("models-coverage-cost.md")}</div>',
        '<h2>Model power vs pass-rate</h2>',
        f'<div class="narrative">{_read_section("06-model-power-observations.md")}</div>',
        f'<div class="chart">{chart_emitter("model-power-scatter.svg")}</div>',
    ]
    return "\n".join(parts)


def render_harnesses_body(*, snapshot_ts, profiles, subsets, per_profile,
                          per_subset, ext_per_subset, timing, chart_emitter) -> str:
    pids = _filter_harness_lanes(per_profile, profiles)
    # External rows for harness models we care about (Sonnet, GPT-5 family, Codex)
    def harness_ext_filter(sub):
        s = sub.lower()
        return any(needle in s for needle in [
            "claude-opus-4.6", "claude-sonnet", "gpt-5", "codex"
        ])
    parts = [
        f'<div class="meta">Snapshot: {html.escape(snapshot_ts)} · {len(pids)} fiz harness lanes shown · external leaderboard for the same models below</div>',
        '<div class="narrative">',
        '<p>Each row holds the model constant (Sonnet 4.6, GPT-5.4-mini, etc.) and varies the agent loop. Native CLI lanes (<code>claude-native-*</code>, <code>codex-native-*</code>) run their own harness directly. <code>fiz-harness-*</code> lanes use fiz as a measurement wrapper around the same CLI. <code>fiz-openrouter-*</code> / <code>fiz-openai-*</code> lanes call the model\'s API directly through fiz\'s built-in loop. A delta between these is harness loss, isolated from model loss.</p>',
        '</div>',
        '<h2>Pass-rate (with external comparators)</h2>',
        _render_pass_table(pids, subsets, per_subset, profiles,
                           include_external=True, ext_per_subset=ext_per_subset,
                           ext_filter=harness_ext_filter),
        '<h2>Detailed metrics</h2>',
        _render_detailed_table(pids, per_profile, per_subset, profiles, timing),
        '<h2>Side-by-side coverage and gaps</h2>',
        f'<div class="narrative">{_read_section("harnesses-side-by-side.md")}</div>',
    ]
    return "\n".join(parts)


def render_providers_body(*, snapshot_ts, profiles, machines, subsets,
                          per_profile, per_subset, ext_per_subset, timing,
                          chart_emitter) -> str:
    pids = _filter_provider_lanes(per_profile, profiles)
    def label(pid, prof):
        return _provider_descriptive_label(pid, prof, machine_for_profile(prof, machines))
    parts = [
        f'<div class="meta">Snapshot: {html.escape(snapshot_ts)} · Qwen3.6-27B across {len(pids)} provider/runtime combinations</div>',
        '<div class="narrative">',
        '<p>The model weights are the same across every row here — Qwen3.6-27B in some quantization. The variable is everything else: where the bytes get computed, which serving engine runs them, what sampling defaults the server applies, whether prefix-cache is hit, and how much round-trip latency the network adds.</p>',
        '<p>Hostnames are abstracted to the substantive characteristics. The descriptive label captures engine + quantization + GPU/CPU + OS — enough to map to a known-good machine spec without leaking inventory.</p>',
        '</div>',
        '<h2>Pass-rate</h2>',
        _render_pass_table(pids, subsets, per_subset, profiles),
        '<h2>Detailed metrics</h2>',
        _render_detailed_table(pids, per_profile, per_subset, profiles, timing, label_for=label),
        '<h2>Performance vs context length</h2>',
        f'<div class="narrative">{_read_section("07-context-length-observations.md")}</div>',
        f'<h3>TTFT (seconds, lower is better)</h3><div class="chart">{chart_emitter("ttft-by-context.svg")}</div>',
        f'<h3>Decode tok/s (higher is better)</h3><div class="chart">{chart_emitter("decode-by-context.svg")}</div>',
        '<h2>Provider details</h2>',
        f'<div class="narrative">{_read_section("providers-details.md")}</div>',
    ]
    return "\n".join(parts)


def render_standalone_html(*, body: str, snapshot_ts: str) -> str:
    """Wrap a body fragment in a complete standalone HTML doc."""
    return f"""<!DOCTYPE html>
<html lang="en"><head>
<meta charset="utf-8">
<title>Terminal-Bench 2.1 — Fizeau benchmark report</title>
<style>{REPORT_CSS}
  body {{ max-width: 1200px; margin: 24px auto; padding: 0 24px; }}
  h1 {{ font-size: 26px; margin-bottom: 4px; }}
</style>
</head><body class="br-body">
<h1>Terminal-Bench 2.1 — Fizeau benchmark report</h1>
{body}
</body></html>
"""


def render_hugo_md(*, body: str, snapshot_ts: str) -> str:
    """Wrap a body fragment in a Hugo content page. Styling comes from the
    site's design overlay (website/assets/css/custom.css). We do not inline
    REPORT_CSS here — the .br-body rules in custom.css would lose the cascade
    against an inline <style>, and we want the design system to win."""
    front = (
        "---\n"
        "title: \"Terminal-Bench 2.1 — Qwen3.6-27B across providers\"\n"
        "linkTitle: \"Terminal-Bench 2.1\"\n"
        "weight: 1\n"
        "toc: true\n"
        "---\n"
    )
    return f"""{front}
<div class="br-body">
{body}
</div>
"""


# ---------- driver ----------

def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n", 1)[0])
    ap.add_argument("--refresh-leaderboard", action="store_true",
                    help="Re-fetch reward.txt files from HF dataset")
    ap.add_argument("--emit-data-only", action="store_true",
                    help="Compute and write JSON aggregates only; skip charts/HTML. "
                         "Useful for an LLM tool-use loop: read the JSON, write narrative, then re-run without --emit-data-only.")
    args = ap.parse_args()

    DATA_DIR.mkdir(parents=True, exist_ok=True)
    CHARTS_DIR.mkdir(parents=True, exist_ok=True)
    SECTIONS_DIR.mkdir(parents=True, exist_ok=True)

    print(f"[1/5] loading reports from {CELLS} …", file=sys.stderr)
    reports = load_reports()
    print(f"      {len(reports)} reports", file=sys.stderr)
    profiles = load_profiles()
    print(f"      {len(profiles)} profile YAMLs", file=sys.stderr)
    subsets = load_subsets()
    print(f"      {len(subsets)} subsets: {sorted(subsets.keys())}", file=sys.stderr)
    raw_report_count = len(reports)
    reports = filter_reports_to_declared_subset_tasks(reports, subsets)
    if len(reports) != raw_report_count:
        print(f"      filtered {raw_report_count - len(reports)} report(s) outside declared TB-2.1 subsets", file=sys.stderr)
    leaderboard = load_leaderboard(refresh=args.refresh_leaderboard)
    print(f"      {len(leaderboard)} external trial records", file=sys.stderr)
    model_power = load_model_power()
    print(f"      {len(model_power)} model-power entries", file=sys.stderr)
    machines = load_machines()
    print(f"      {len(machines)} machines in registry: {sorted(machines.keys())}", file=sys.stderr)

    print("[2/5] aggregating …", file=sys.stderr)
    per_profile = aggregate_per_profile(reports)
    # Decorate per-profile rollups with derived harness label so JSON readers (and an LLM
    # editing the narrative markdown) don't have to re-derive it from the raw YAMLs.
    for pid, agg in per_profile.items():
        agg["harness"] = harness_label(pid, profiles.get(pid))
    per_subset = aggregate_per_subset(reports, subsets)
    ext_per_subset = aggregate_external_per_subset(leaderboard, subsets)

    print("[3/5] computing per-turn timing …", file=sys.stderr)
    profiles_to_scan = [p for p in per_profile if p not in EXCLUDED_PROFILES]
    timing = compute_per_profile_timing(profiles_to_scan)

    snapshot_ts = dt.datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")

    print("[4/5] writing data JSON …", file=sys.stderr)
    (DATA_DIR / "snapshot.json").write_text(json.dumps({
        "generated_at": snapshot_ts,
        "n_reports": len(reports),
        "n_profiles_active": len(profiles_to_scan),
        "subset_sizes": {k: len(v["tasks"]) for k, v in subsets.items()},
        "model_power_keys": sorted(model_power.keys()),
        "external_submissions": sorted(set(t["submission"].split("/")[-1] for t in leaderboard)),
    }, indent=2), encoding="utf-8")
    (DATA_DIR / "aggregates.json").write_text(json.dumps({
        "per_profile": per_profile,
        "per_subset": per_subset,
        "external_per_subset": ext_per_subset,
    }, indent=2, default=str), encoding="utf-8")
    (DATA_DIR / "timing.json").write_text(json.dumps(timing, indent=2, default=str), encoding="utf-8")
    (DATA_DIR / "profiles.json").write_text(json.dumps(profiles, indent=2, default=str), encoding="utf-8")
    (DATA_DIR / "machines.json").write_text(json.dumps(machines, indent=2, default=str), encoding="utf-8")

    # Headline-numbers JSON for the website homepage hero. Hugo reads this
    # as a Site Data file (website/data/bench_latest.json) so the hero
    # readout updates whenever this script runs.
    HERO_PROFILE = "fiz-openrouter-qwen3-6-27b"  # benchmark "throughput reference"
    a = per_profile.get(HERO_PROFILE, {})
    t = timing.get(HERO_PROFILE, {})
    hero_pass, hero_attempted = subset_pass_counts(per_subset, HERO_PROFILE, "all")
    bench_latest = {
        "snapshot_ts": snapshot_ts,
        "subset": "all",
        "subset_size": len(subsets.get("all", {}).get("tasks", [])),
        "n_reports_total": len(reports),
        "hero_profile_id": HERO_PROFILE,
        "hero_label": "Qwen3.6-27B / OpenRouter",
        "hero": {
            "decode_tps_p50": t.get("decode_tps_p50"),
            "ttft_p50_s": t.get("ttft_p50"),
            "pass_at_k_pct": (hero_pass / hero_attempted * 100) if hero_attempted else None,
            "pass_at_k_num": hero_pass,
            "pass_at_k_den": hero_attempted,
            "median_wall_s": a.get("median_wall"),
            "median_turns": a.get("median_turns"),
            "median_in_tok": a.get("median_in_tok"),
            "median_out_tok": a.get("median_out_tok"),
            "n_attempts": a.get("n_attempts"),
            "n_real": a.get("n_real"),
        },
    }
    HUGO_DATA = REPO / "website/data"
    HUGO_DATA.mkdir(parents=True, exist_ok=True)
    (HUGO_DATA / "bench_latest.json").write_text(
        json.dumps(bench_latest, indent=2, default=str), encoding="utf-8"
    )

    # Comparative overview JSON for the /benchmarks/ landing page. Picks a
    # focused set of rows that tell the harness/model isolation story: the
    # three Qwen lanes (same model, different provider/runtime) plus the
    # top external comparators by pass@k (different model, same TB-2.1
    # task set).
    QWEN_LANES = [
        ("fiz-openrouter-qwen3-6-27b", ["fiz-openrouter-qwen3-6-27b"], "OpenRouter (cloud)"),
        ("sindri-vllm",                ["sindri-vllm", "sindri-club-3090"], "vLLM int4 (sindri / RTX 5090)"),
        ("sindri-llamacpp",            ["sindri-llamacpp", "sindri-club-3090-llamacpp"], "llama.cpp Q3_K_XL (sindri / RTX 3090)"),
        ("vidar-qwen3-6-27b",          ["vidar-qwen3-6-27b"], "oMLX 8-bit (vidar / Mac Studio)"),
    ]
    qwen_rows = []
    for public_pid, data_pids, label in QWEN_LANES:
        data_pid = next((pid for pid in data_pids if pid in per_profile), public_pid)
        a = per_profile.get(data_pid, {})
        t = timing.get(data_pid, {})
        passed, attempted = subset_pass_counts(per_subset, data_pid, "all")
        qwen_rows.append({
            "profile_id": public_pid,
            "source_profile_id": data_pid,
            "label": label,
            "pass_at_k_pct": (passed / attempted * 100) if attempted else None,
            "pass_at_k_num": passed,
            "pass_at_k_den": attempted,
            "median_wall_s": a.get("median_wall"),
            "decode_tps_p50": t.get("decode_tps_p50"),
            "ttft_p50_s": t.get("ttft_p50"),
            "avg_cost_usd": a.get("avg_cost"),
            "n_attempts": a.get("n_attempts"),
        })

    # External: pick best-performing submission per underlying frontier model on TB-2.1 'all'.
    FRONTIER_KEYS = [
        ("Claude Opus 4.6", ["Opus-4.6", "claude-opus-4-6"]),
        ("GPT-5.4",         ["GPT-5.4"]),
        ("GPT-5.3-Codex",   ["GPT-5.3-Codex"]),
        ("Gemini 3 Pro",    ["Gemini-3-Pro"]),
        ("Gemini 3.1 Pro",  ["Gemini-3.1-Pro"]),
    ]
    # Heuristic: an "ensemble" submission packs multiple model families into one
    # row name (e.g. "Junie_CLI__Gemini-3-Flash-...-GPT-5.3-Codex" or "OB-1_..."
    # naming Opus + Codex together). For per-model headline rows we want
    # single-model entries only, so skip submissions whose post-__ tail names
    # more than one model family or that use the multi-model OB-1 / Junie_CLI
    # harness prefix.
    import re as _re
    MODEL_FAMILY_RE = _re.compile(r"(Claude-Opus|Claude-Sonnet|GPT-5|Gemini-3|Kimi|GLM|Minimax|DeepSeek|qwen3)", _re.IGNORECASE)
    def _is_ensemble(sub: str) -> bool:
        if sub.startswith(("OB-1", "Junie_CLI")):
            return True
        tail = sub.split("__", 1)[1] if "__" in sub else sub
        return len(MODEL_FAMILY_RE.findall(tail)) > 1

    external_rows = []
    for label, needles in FRONTIER_KEYS:
        candidates = []
        for sub, by_sub in ext_per_subset.items():
            if _is_ensemble(sub):
                continue
            d = by_sub.get("all", {})
            if not d.get("tasks_attempted"):
                continue
            if any(n.lower() in sub.lower() for n in needles):
                pct = d["tasks_passed"] / d["tasks_attempted"] * 100
                candidates.append((sub, pct, d["tasks_passed"], d["tasks_attempted"]))
        if not candidates:
            continue
        candidates.sort(key=lambda x: -x[1])
        best_sub, pct, p, a_ = candidates[0]
        external_rows.append({
            "model_label": label,
            "submission": best_sub,
            "harness": best_sub.split("__")[0] if "__" in best_sub else "",
            "pass_at_k_pct": pct,
            "pass_at_k_num": p,
            "pass_at_k_den": a_,
        })

    bench_overview = {
        "snapshot_ts": snapshot_ts,
        "n_reports_total": len(reports),
        "n_profiles_active": len([p for p in per_profile if p not in EXCLUDED_PROFILES]),
        "n_external_comparators": len(ext_per_subset),
        "n_external_trials": sum(1 for _ in leaderboard),
        "subset_size_all": len(subsets.get("all", {}).get("tasks", [])),
        "qwen_across_providers": qwen_rows,
        "frontier_comparators": external_rows,
    }
    (HUGO_DATA / "bench_overview.json").write_text(
        json.dumps(bench_overview, indent=2, default=str), encoding="utf-8"
    )

    HUGO_BUNDLE.mkdir(parents=True, exist_ok=True)
    (HUGO_BUNDLE / "data").mkdir(exist_ok=True)
    for js in DATA_DIR.glob("*.json"):
        (HUGO_BUNDLE / "data" / js.name).write_bytes(js.read_bytes())

    if args.emit_data_only:
        print(f"[done, data-only] data/ written. Edit sections/*.md, then re-run without --emit-data-only.", file=sys.stderr)
        return

    print("[5/5] rendering charts (matplotlib) and HTML …", file=sys.stderr)
    chart_pass_rate_by_subset(per_subset, ext_per_subset, profiles, CHARTS_DIR / "pass-rate.svg")
    chart_model_power_scatter(per_profile, per_subset, ext_per_subset, profiles, model_power, CHARTS_DIR / "model-power-scatter.svg")
    chart_lines_over_context(timing, profiles, "ttft_p50", "median TTFT (s)", CHARTS_DIR / "ttft-by-context.svg")
    chart_lines_over_context(timing, profiles, "decode_tps_p50", "median decode tok/s", CHARTS_DIR / "decode-by-context.svg")

    body_inline = render_body(
        profiles=profiles, machines=machines, subsets=subsets,
        per_profile=per_profile, per_subset=per_subset, ext_per_subset=ext_per_subset,
        timing=timing, snapshot_ts=snapshot_ts, n_reports=len(reports),
        chart_emitter=lambda name: _chart_ref_inline(CHARTS_DIR / name),
    )
    html_doc = render_standalone_html(body=body_inline, snapshot_ts=snapshot_ts)
    OUT_HTML.write_text(html_doc, encoding="utf-8")
    print(f"      wrote {OUT_HTML.relative_to(REPO)} ({OUT_HTML.stat().st_size:,} bytes)", file=sys.stderr)

    # Hugo page bundle. The TB-2.1 page is split into 4 sub-pages so each
    # comparison axis (models / harnesses / providers) gets its own URL and
    # left-nav entry. Charts and data live at the parent bundle and are
    # referenced via absolute Hugo URLs from the sub-pages.
    (HUGO_BUNDLE / "charts").mkdir(exist_ok=True)
    for svg in CHARTS_DIR.glob("*.svg"):
        (HUGO_BUNDLE / "charts" / svg.name).write_bytes(svg.read_bytes())

    # Sub-pages reference charts via absolute Hugo URLs (charts live at the
    # parent bundle's charts/ dir).
    BENCH_BASE = "/benchmarks/terminal-bench-2-1"
    def _chart_ref_abs(name: str) -> str:
        return f'<img src="{BENCH_BASE}/charts/{html.escape(name)}" alt="{html.escape(name)}">'
    # The parent index page can use bundle-relative refs since charts are in
    # the same bundle.
    def _chart_ref_local(name: str) -> str:
        return f'<img src="charts/{html.escape(name)}" alt="{html.escape(name)}">'

    pages = [
        # (path under HUGO_BUNDLE, title, weight, body fn, kwargs)
        ("_index.md",            "Terminal-Bench 2.1", 1, render_overview_body,   dict(chart_emitter=_chart_ref_local)),
        ("models/_index.md",     "Models",             2, render_models_body,     dict(chart_emitter=_chart_ref_abs)),
        ("harnesses/_index.md",  "Harnesses",          3, render_harnesses_body,  dict(chart_emitter=_chart_ref_abs)),
        ("providers/_index.md",  "Providers",          4, render_providers_body,  dict(chart_emitter=_chart_ref_abs)),
    ]
    for path, title, weight, body_fn, extra_kwargs in pages:
        kwargs = dict(snapshot_ts=snapshot_ts, profiles=profiles, subsets=subsets,
                      per_profile=per_profile, per_subset=per_subset,
                      ext_per_subset=ext_per_subset, timing=timing)
        # render_overview_body needs n_reports; render_providers_body needs machines.
        if body_fn is render_overview_body:
            kwargs["n_reports"] = len(reports)
        if body_fn is render_providers_body:
            kwargs["machines"] = machines
        kwargs.update(extra_kwargs)
        body = body_fn(**kwargs)
        front = (
            "---\n"
            f"title: \"{title}\"\n"
            f"weight: {weight}\n"
            "toc: true\n"
            "---\n"
        )
        out_path = HUGO_BUNDLE / path
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(f"{front}\n<div class=\"br-body\">\n{body}\n</div>\n", encoding="utf-8")

    # Landing page for the benchmarks section. Only create if it doesn't exist
    # (it's intended to be hand-edited; we won't clobber edits on regen).
    if not HUGO_LANDING.is_file():
        HUGO_LANDING.parent.mkdir(parents=True, exist_ok=True)
        HUGO_LANDING.write_text(
            "---\ntitle: Benchmarks\nweight: 2\n---\n\n"
            "Independent and reproducible benchmark runs of Fizeau against public coding-agent benchmarks. "
            "Each report below is regenerated from raw trial data by `scripts/benchmark/generate-report.py`.\n\n"
            "{{< cards >}}\n"
            "  {{< card link=\"terminal-bench-2-1\" title=\"Terminal-Bench 2.1\" "
            "subtitle=\"Qwen3.6-27B across openrouter / vLLM / oMLX, plus claude/codex/gpt-5 lanes and external leaderboard comparators.\" >}}\n"
            "{{< /cards >}}\n",
            encoding="utf-8",
        )
        print(f"      wrote {HUGO_LANDING.relative_to(REPO)} (landing page; safe to hand-edit, will not be overwritten)", file=sys.stderr)
    print(f"      hugo bundle → {HUGO_BUNDLE.relative_to(REPO)}/", file=sys.stderr)
    print(f"      data/ → {DATA_DIR.relative_to(REPO)}", file=sys.stderr)
    print(f"      charts/ → {CHARTS_DIR.relative_to(REPO)}", file=sys.stderr)


if __name__ == "__main__":
    main()
