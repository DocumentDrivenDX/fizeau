#!/usr/bin/env python3
"""Bootstrap a smaller Terminal-Bench-2 subset from public leaderboard trials.

The selector intentionally treats task difficulty as an empirical property,
not a scalar we assume is monotonic across model families. It downloads only
`verifier/reward.txt` files from the Hugging Face Terminal-Bench-2 leaderboard,
aggregates per-task pass rates by coarse model tiers, reports monotonicity
violations, and writes a candidate subset manifest.
"""

from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
import json
import os
from pathlib import Path
import re
import statistics
import sys
import time
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import quote
from urllib.request import urlopen


DATASET = "harborframework/terminal-bench-2-leaderboard"
API_ROOT = f"https://huggingface.co/api/datasets/{DATASET}/tree/main"
RAW_ROOT = f"https://huggingface.co/datasets/{DATASET}/resolve/main"
SUBMISSIONS_ROOT = "submissions/terminal-bench/2.0"
DATASET_COMMIT = "53ff2b87d621bdb97b455671f2bd9728b7d86c11"


FRONTIER_PATTERNS = [
    r"opus",
    r"gpt-5\.3-codex",
    r"gpt-5\.4(?!-mini|-nano)",
    r"gemini-3(?:\.1)?-pro",
    r"mythos",
]
MEDIUM_FRONTIER_PATTERNS = [
    r"sonnet",
    r"gpt-5\.4-mini",
    r"gpt-5\.2",
]
NON_FRONTIER_PATTERNS = [
    r"kimi",
    r"minimax",
    r"glm",
    r"qwen",
    r"deepseek",
    r"termigen",
    r"gpt-oss",
]


DEFAULT_POWER_MAP = Path(__file__).with_name("terminalbench_model_power.json")
DEFAULT_CATALOG = Path("internal/modelcatalog/catalog/models.yaml")


@dataclass
class Trial:
    submission: str
    tier: str
    task_id: str
    reward: float
    power: int = 0
    model_field: str = ""


@dataclass
class TaskStats:
    task_id: str
    by_tier: dict[str, list[float]] = field(default_factory=dict)
    by_submission: dict[str, list[float]] = field(default_factory=dict)

    def tier_mean(self, tier: str) -> float | None:
        vals = self.by_tier.get(tier, [])
        return statistics.fmean(vals) if vals else None

    def all_mean(self) -> float:
        vals = [v for rewards in self.by_tier.values() for v in rewards]
        return statistics.fmean(vals) if vals else 0.0

    def n_trials(self) -> int:
        return sum(len(v) for v in self.by_tier.values())

    def monotonicity_violations(self, margin: float) -> list[str]:
        out: list[str] = []
        frontier = self.tier_mean("frontier")
        medium = self.tier_mean("medium_frontier")
        non = self.tier_mean("non_frontier")
        if frontier is not None and medium is not None and medium > frontier + margin:
            out.append("medium_frontier > frontier")
        if medium is not None and non is not None and non > medium + margin:
            out.append("non_frontier > medium_frontier")
        if frontier is not None and non is not None and non > frontier + margin:
            out.append("non_frontier > frontier")
        return out


def http_json_with_headers(url: str) -> tuple[Any, dict[str, str]]:
    with urlopen(url, timeout=60) as response:
        return json.load(response), dict(response.headers.items())


def http_json(url: str) -> Any:
    data, _ = http_json_with_headers(url)
    return data


def http_text(url: str) -> str:
    with urlopen(url, timeout=60) as response:
        return response.read().decode("utf-8", errors="replace")


def api_list(path: str, *, recursive: bool = False) -> list[dict[str, Any]]:
    query = "?recursive=true&expand=false&limit=1000" if recursive else "?recursive=false&expand=false&limit=1000"
    url = f"{API_ROOT}/{quote(path)}{query}"
    items: list[dict[str, Any]] = []
    while url:
        page, headers = http_json_with_headers(url)
        items.extend(page)
        url = next_link(headers.get("Link", ""))
    return items


def next_link(link_header: str) -> str:
    for part in link_header.split(","):
        section = part.strip()
        if 'rel="next"' not in section:
            continue
        if section.startswith("<") and ">" in section:
            return section[1:section.index(">")]
    return ""


def power_band(power: int) -> str:
    if power >= 9:
        return "frontier"
    if power >= 7:
        return "medium_frontier"
    if power >= 4:
        return "non_frontier"
    return "other"


def load_power_map(path: Path) -> dict[str, Any]:
    raw = json.loads(path.read_text())
    models = raw.get("models", {})
    return {normalize_model_key(key): value for key, value in models.items()}


def load_catalog_power(path: Path) -> list[dict[str, Any]]:
    try:
        import yaml
    except ImportError as exc:
        raise SystemExit("PyYAML is required to load catalog model power") from exc

    raw = yaml.safe_load(path.read_text())
    out: list[dict[str, Any]] = []
    for model_id, entry in (raw.get("models") or {}).items():
        aliases = [
            model_id,
            entry.get("display_name", ""),
            entry.get("openrouter_id", ""),
            entry.get("openrouter_ref_id", ""),
        ]
        aliases.extend((entry.get("surfaces") or {}).values())
        out.append({
            "id": model_id,
            "power": int(entry.get("power") or 0),
            "aliases": [alias for alias in aliases if isinstance(alias, str) and alias.strip()],
        })
    return out


def normalize_model_key(value: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", value.strip().lower()).strip("-")


def submission_name(path: str) -> str:
    return path.rsplit("/", 1)[-1]


def model_field_from_submission(path: str) -> str:
    name = submission_name(path)
    _, sep, model = name.partition("__")
    return model if sep else name


def power_for_submission(path: str, power_map: dict[str, Any], catalog_power: list[dict[str, Any]]) -> int:
    name = submission_name(path)
    override = power_map.get(normalize_model_key("submission:" + name))
    if override:
        return int(override.get("power", 0))

    model_field = model_field_from_submission(path)
    catalog = power_for_model_field(model_field, catalog_power)
    if catalog:
        return catalog

    keys = [normalize_model_key(model_field)]
    for key in keys:
        entry = power_map.get(key)
        if entry:
            return int(entry.get("power", 0))
    return 0


def power_for_model_field(model_field: str, catalog_power: list[dict[str, Any]]) -> int:
    for entry in catalog_power:
        for alias in entry["aliases"]:
            if same_model_variant(model_field, alias):
                return entry["power"]
    return 0


def same_model_variant(a: str, b: str) -> bool:
    ca = catalog_model_key(a)
    cb = catalog_model_key(b)
    if not ca or not cb:
        return False
    if ca == cb:
        return True
    if same_named_family_major(a, b):
        return True
    if len(ca) < 8 or len(cb) < 8:
        return False
    return ca in cb or cb in ca


def catalog_model_key(value: str) -> str:
    value = value.strip()
    if "/" in value and not value.startswith("submission:"):
        value = value.split("/", 1)[1]
    return re.sub(r"[^a-z0-9]+", "", value.lower())


def same_named_family_major(a: str, b: str) -> bool:
    family_a, major_a = named_family_major(a)
    family_b, major_b = named_family_major(b)
    return bool(family_a and family_a == family_b and major_a and major_a == major_b)


def named_family_major(value: str) -> tuple[str, int]:
    if "/" in value:
        value = value.split("/", 1)[1]
    tokens = [token for token in re.split(r"[^a-z0-9]+", value.lower()) if token]
    family = ""
    for token in tokens:
        if token in {"opus", "sonnet", "haiku"}:
            family = token
    if not family:
        return "", 0
    for token in tokens:
        if token.isdigit():
            return family, int(token)
    return family, 0


def classify_submission(
    path: str,
    power_map: dict[str, Any] | None = None,
    catalog_power: list[dict[str, Any]] | None = None,
) -> str:
    if power_map is not None:
        power = power_for_submission(path, power_map, catalog_power or [])
        if power:
            return power_band(power)

    name = submission_name(path).lower()
    if any(re.search(pattern, name) for pattern in NON_FRONTIER_PATTERNS):
        return "non_frontier"
    if any(re.search(pattern, name) for pattern in MEDIUM_FRONTIER_PATTERNS):
        return "medium_frontier"
    if any(re.search(pattern, name) for pattern in FRONTIER_PATTERNS):
        return "frontier"
    return "other"


def task_id_from_reward_path(path: str) -> str | None:
    parts = path.split("/")
    try:
        trial_dir = parts[-3]
    except IndexError:
        return None
    return trial_dir.split("__", 1)[0] or None


def discover_trials(
    cache_path: Path,
    *,
    refresh: bool,
    power_map: dict[str, Any],
    catalog_power: list[dict[str, Any]],
) -> list[Trial]:
    if cache_path.exists() and not refresh:
        raw = json.loads(cache_path.read_text())
        trials = [Trial(**item) for item in raw["trials"]]
        return [with_power(trial, power_map, catalog_power) for trial in trials]

    if os.environ.get("TERMBENCH_SELECTOR_SNAPSHOT") == "1":
        trials = discover_trials_from_snapshot(cache_path.parent / "terminalbench-leaderboard-rewards", power_map, catalog_power)
        if trials:
            cache_path.parent.mkdir(parents=True, exist_ok=True)
            cache_path.write_text(json.dumps({"trials": [trial.__dict__ for trial in trials]}, indent=2, sort_keys=True))
            return trials

    print("listing leaderboard reward files", file=sys.stderr)
    submissions = [
        item["path"]
        for item in api_list(SUBMISSIONS_ROOT)
        if item.get("type") == "directory"
    ]
    reward_paths: list[str] = []
    for index, submission in enumerate(submissions, 1):
        print(f"[{index:02d}/{len(submissions):02d}] {classify_submission(submission, power_map, catalog_power):15s} {submission}", file=sys.stderr)
        try:
            files = api_list(submission, recursive=True)
        except (HTTPError, URLError) as exc:
            print(f"  warning: list failed: {exc}", file=sys.stderr)
            continue
        reward_paths.extend(
            item["path"]
            for item in files
            if item.get("type") == "file" and item.get("path", "").endswith("/verifier/reward.txt")
        )
    print(f"fetching {len(reward_paths)} reward files", file=sys.stderr)
    trials: list[Trial] = []
    with ThreadPoolExecutor(max_workers=32) as pool:
        futures = {pool.submit(fetch_reward_trial, path, power_map, catalog_power): path for path in reward_paths}
        for i, future in enumerate(as_completed(futures), 1):
            trial = future.result()
            if trial is not None:
                trials.append(trial)
            if i % 1000 == 0:
                print(f"  fetched {i}/{len(reward_paths)}", file=sys.stderr)
    cache_path.parent.mkdir(parents=True, exist_ok=True)
    cache_path.write_text(json.dumps({"trials": [trial.__dict__ for trial in trials]}, indent=2, sort_keys=True))
    return trials


def with_power(trial: Trial, power_map: dict[str, Any], catalog_power: list[dict[str, Any]]) -> Trial:
    power = power_for_submission(trial.submission, power_map, catalog_power)
    if power:
        trial.power = power
        trial.tier = power_band(power)
        trial.model_field = model_field_from_submission(trial.submission)
    return trial


def fetch_reward_trial(reward_path: str, power_map: dict[str, Any], catalog_power: list[dict[str, Any]]) -> Trial | None:
    task_id = task_id_from_reward_path(reward_path)
    if not task_id:
        return None
    parts = reward_path.split("/")
    if len(parts) < 8:
        return None
    submission = "/".join(parts[:4])
    try:
        text = http_text(f"{RAW_ROOT}/{quote(reward_path)}").strip()
        reward = float(text)
    except (HTTPError, URLError, ValueError):
        return None
    power = power_for_submission(submission, power_map, catalog_power)
    return Trial(
        submission=submission,
        tier=power_band(power) if power else classify_submission(submission),
        task_id=task_id,
        reward=reward,
        power=power,
        model_field=model_field_from_submission(submission),
    )


def discover_trials_from_snapshot(local_dir: Path, power_map: dict[str, Any], catalog_power: list[dict[str, Any]]) -> list[Trial]:
    try:
        from huggingface_hub import snapshot_download
    except Exception:
        return []

    print("downloading reward files with huggingface_hub snapshot_download", file=sys.stderr)
    snapshot_root = Path(snapshot_download(
        repo_id=DATASET,
        repo_type="dataset",
        local_dir=str(local_dir),
        allow_patterns=f"{SUBMISSIONS_ROOT}/*/*/*/verifier/reward.txt",
    ))
    trials: list[Trial] = []
    reward_paths = sorted(snapshot_root.glob(f"{SUBMISSIONS_ROOT}/*/*/*/verifier/reward.txt"))
    for reward_path in reward_paths:
        rel = reward_path.relative_to(snapshot_root).as_posix()
        parts = rel.split("/")
        if len(parts) < 8:
            continue
        submission = "/".join(parts[:4])
        task_id = task_id_from_reward_path(rel)
        if not task_id:
            continue
        try:
            reward = float(reward_path.read_text().strip())
        except ValueError:
            continue
        power = power_for_submission(submission, power_map, catalog_power)
        trials.append(Trial(
            submission=submission,
            tier=power_band(power) if power else classify_submission(submission),
            task_id=task_id,
            reward=reward,
            power=power,
            model_field=model_field_from_submission(submission),
        ))
    return trials


def aggregate(trials: list[Trial]) -> dict[str, TaskStats]:
    stats: dict[str, TaskStats] = {}
    for trial in trials:
        task = stats.setdefault(trial.task_id, TaskStats(task_id=trial.task_id))
        task.by_tier.setdefault(trial.tier, []).append(trial.reward)
        task.by_submission.setdefault(trial.submission, []).append(trial.reward)
    return stats


def valid_task_ids(tasks_dir: Path) -> set[str]:
    return {path.name for path in tasks_dir.iterdir() if path.is_dir()}


def score_task(task: TaskStats, bucket: str) -> float:
    all_rate = task.all_mean()
    frontier = task.tier_mean("frontier") or 0.0
    medium = task.tier_mean("medium_frontier") or 0.0
    non = task.tier_mean("non_frontier") or 0.0
    if bucket == "global_easy":
        return all_rate
    if bucket == "global_hard":
        return -all_rate
    if bucket == "frontier_only":
        return frontier - max(medium, non)
    if bucket == "medium_frontier":
        return medium - non + min(frontier, medium) * 0.25
    if bucket == "non_frontier":
        return non + min(frontier, medium, non) * 0.25
    if bucket == "non_monotonic_probe":
        return len(task.monotonicity_violations(0.15)) + abs(non - frontier)
    return 0.0


def select_bucket(
    tasks: dict[str, TaskStats],
    bucket: str,
    count: int,
    selected: set[str],
) -> list[str]:
    candidates = [task for task in tasks.values() if task.task_id not in selected]
    candidates.sort(key=lambda task: (score_task(task, bucket), task.n_trials()), reverse=True)
    picked = [task.task_id for task in candidates[:count]]
    selected.update(picked)
    return picked


def build_manifest(tasks: dict[str, TaskStats], selected_by_bucket: dict[str, list[str]]) -> dict[str, Any]:
    entries = []
    for bucket, ids in selected_by_bucket.items():
        for task_id in ids:
            task = tasks[task_id]
            entries.append({
                "id": task_id,
                "bucket": bucket,
                "external_rates": {
                    "all": round(task.all_mean(), 3),
                    "frontier": round(task.tier_mean("frontier") or 0.0, 3),
                    "medium_frontier": round(task.tier_mean("medium_frontier") or 0.0, 3),
                    "non_frontier": round(task.tier_mean("non_frontier") or 0.0, 3),
                },
                "monotonicity_violations": task.monotonicity_violations(0.15),
            })
    return {
        "_comment": "Generated by scripts/benchmark/select_terminalbench_subset.py from public Hugging Face Terminal-Bench-2 leaderboard reward files.",
        "version": "bootstrap-1",
        "captured": time.strftime("%Y-%m-%d"),
        "dataset": "terminal-bench@2.0",
        "dataset_repo": "https://github.com/laude-institute/terminal-bench-2",
        "dataset_commit": DATASET_COMMIT,
        "selection_rule": "External leaderboard bootstrap: choose global-easy, global-hard, frontier-only, medium-frontier, non-frontier, and non-monotonic probe tasks by tiered reward rates.",
        "power_bands": {
            "frontier": "power >= 9",
            "medium_frontier": "7 <= power <= 8",
            "non_frontier": "4 <= power <= 6",
        },
        "source": {
            "type": "huggingface_dataset",
            "dataset": DATASET,
            "path": SUBMISSIONS_ROOT,
        },
        "tasks": entries,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tasks-dir", default="scripts/benchmark/external/terminal-bench-2")
    parser.add_argument("--cache", default="benchmark-results/cache/terminalbench-leaderboard-rewards.json")
    parser.add_argument("--out", default="scripts/beadbench/external/termbench-subset-external-bootstrap.json")
    parser.add_argument("--catalog", default=str(DEFAULT_CATALOG))
    parser.add_argument("--power-map", default=str(DEFAULT_POWER_MAP))
    parser.add_argument("--refresh", action="store_true")
    args = parser.parse_args()

    power_map = load_power_map(Path(args.power_map))
    catalog_power = load_catalog_power(Path(args.catalog))
    task_ids = valid_task_ids(Path(args.tasks_dir))
    trials = discover_trials(Path(args.cache), refresh=args.refresh, power_map=power_map, catalog_power=catalog_power)
    stats = {tid: stat for tid, stat in aggregate(trials).items() if tid in task_ids}
    if not stats:
        raise SystemExit("no leaderboard task stats matched local TB-2 task ids")

    selected: set[str] = set()
    selected_by_bucket = {
        "global_easy": select_bucket(stats, "global_easy", 2, selected),
        "global_hard": select_bucket(stats, "global_hard", 2, selected),
        "frontier_only": select_bucket(stats, "frontier_only", 3, selected),
        "medium_frontier": select_bucket(stats, "medium_frontier", 3, selected),
        "non_frontier": select_bucket(stats, "non_frontier", 3, selected),
        "non_monotonic_probe": select_bucket(stats, "non_monotonic_probe", 2, selected),
    }

    manifest = build_manifest(stats, selected_by_bucket)
    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(manifest, indent=2, sort_keys=False) + "\n")

    violations = [
        (task.task_id, task.monotonicity_violations(0.15))
        for task in stats.values()
        if task.monotonicity_violations(0.15)
    ]
    print(f"wrote {out_path}")
    print(f"trials: {len(trials)}")
    print(f"tasks matched: {len(stats)}")
    print(f"tasks with monotonicity violations (>0.15): {len(violations)}")
    for bucket, ids in selected_by_bucket.items():
        print(f"{bucket}: {', '.join(ids)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
