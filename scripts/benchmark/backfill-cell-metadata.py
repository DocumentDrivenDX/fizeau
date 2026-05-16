#!/usr/bin/env python3
"""Backfill embedded profile metadata into existing benchmark cells.

Walks ``benchmark-results/<canonical>/cells/`` for every ``report.json`` written
before PR 1c (self-describing cells, ADR-016) and:

1. Resolves the cell's ``profile_id`` to a current
   ``scripts/benchmark/profiles/<id>.yaml`` (applying ``PROFILE_ALIASES`` from
   ``scripts/website/build-benchmark-data.py``).
2. For four profiles deleted from ``profiles/`` (``sindri-club-3090``,
   ``vidar-qwen3-6-27b-openai-compat``, ``sindri-club-3090-llamacpp``,
   ``gpt-5-3-mini``), the YAML is recovered from git history at the commit
   preceding its deletion.
3. Embeds the full resolved profile YAML under ``cell.profile``, stamps
   ``framework`` and ``dataset`` from the cell's path, and mints ``cell_id``
   as ``<started_at compact>-<4 hex>``.
4. Renames the cell directory to the canonical ADR-016 layout
   ``<canonical>/cells/<framework>-<dataset>/<task>/<cell-id>/``.

Cells that already carry a non-empty ``cell.profile`` block are skipped
(idempotent re-run).

Also deletes:

* ``benchmark-results/fiz-tools-v1/cells/.lane_aborted/`` (legacy abort markers).
* ``benchmark-results/fiz-tools-v1/{tb21-all,or-passing,local-qwen,timing-baseline}/``
  (per-recipe operational subdirs whose ``sweep_lane_meta.json`` files are
  superseded by self-describing cells).

Cells whose profile cannot be recovered are deleted and logged to
``benchmark-results/backfill-2026-05-16.log``.

Usage::

    python scripts/benchmark/backfill-cell-metadata.py --dry-run
    python scripts/benchmark/backfill-cell-metadata.py
"""
from __future__ import annotations

import argparse
import json
import os
import secrets
import shutil
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError:
    sys.stderr.write(
        "PyYAML is required: pip install pyyaml (or run inside the project venv).\n"
    )
    sys.exit(2)


REPO_ROOT = Path(__file__).resolve().parents[2]
PROFILES_DIR = REPO_ROOT / "scripts" / "benchmark" / "profiles"
DEFAULT_BENCHMARK_RESULTS = REPO_ROOT / "benchmark-results"
DEFAULT_LOG_PATH = DEFAULT_BENCHMARK_RESULTS / "backfill-2026-05-16.log"

# Mirrors scripts/website/build-benchmark-data.py:PROFILE_ALIASES at HEAD.
# Renamed profiles whose cells still reference the old id.
PROFILE_ALIASES: dict[str, str] = {
    "sindri-club-3090": "sindri-vllm",
    "sindri-club-3090-llamacpp": "sindri-llamacpp",
}

# Profiles deleted from scripts/benchmark/profiles/. We recover the YAML from
# the commit immediately preceding the deletion (git show <del>~1:<path>).
ORPHAN_PROFILE_IDS = (
    "sindri-club-3090",
    "vidar-qwen3-6-27b-openai-compat",
    "sindri-club-3090-llamacpp",
    "gpt-5-3-mini",
)

# Operational subdirs to wipe (recipe-named sweep_lane_meta.json roots).
RECIPE_SUBDIRS = ("tb21-all", "or-passing", "local-qwen", "timing-baseline")

# Default framework / dataset for cells whose path does not carry one
# (iteration-v1 layout B). All observed cells under iteration-v1 are
# TerminalBench 2.1 tasks; hello cells are dumb_script smoke.
DEFAULT_FRAMEWORK = "terminal-bench"
DEFAULT_DATASET = "terminal-bench-2-1"
SMOKE_FRAMEWORK = "smoke"
SMOKE_DATASET = "smoke"


def framework_dataset_segment(framework: str, dataset: str) -> str:
    """Mirror ``cmd/bench/matrix.go:matrixFrameworkDatasetSegment``.

    If the dataset segment already starts with ``<framework>-`` (or equals
    the framework), skip prepending to avoid the double prefix
    (e.g. ``terminal-bench`` + ``terminal-bench-2-1`` →
    ``terminal-bench-2-1``).
    """
    framework = (framework or "").strip()
    dataset = (dataset or "").strip()
    if not framework:
        return dataset
    if dataset == framework or dataset.startswith(framework + "-"):
        return dataset
    return f"{framework}-{dataset}"


@dataclass
class Action:
    kind: str  # "embed-move" | "skip-backfilled" | "delete-irrecoverable"
    src: Path
    dst: Path | None
    profile_id: str
    resolved_id: str
    note: str = ""
    framework: str = ""
    dataset: str = ""
    cell_id: str = ""
    layout: str = ""


def load_yaml(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as fh:
        return yaml.safe_load(fh) or {}


def load_orphan_profiles(workdir: Path) -> dict[str, dict[str, Any]]:
    """Recover the four deleted profile YAMLs from git history."""
    workdir.mkdir(parents=True, exist_ok=True)
    recovered: dict[str, dict[str, Any]] = {}
    for profile_id in ORPHAN_PROFILE_IDS:
        rel = f"scripts/benchmark/profiles/{profile_id}.yaml"
        # commit that deleted the file
        commit = subprocess.check_output(
            [
                "git",
                "-C",
                str(REPO_ROOT),
                "log",
                "--diff-filter=D",
                "--name-only",
                "--pretty=format:%H",
                "--",
                rel,
            ],
            text=True,
        ).strip().splitlines()
        commit = next((c for c in commit if c), "")
        if not commit:
            sys.stderr.write(
                f"warn: could not find deleting commit for orphan profile {profile_id}\n"
            )
            continue
        yaml_text = subprocess.check_output(
            [
                "git",
                "-C",
                str(REPO_ROOT),
                "show",
                f"{commit}~1:{rel}",
            ],
            text=True,
        )
        out_path = workdir / f"{profile_id}.yaml"
        out_path.write_text(yaml_text)
        doc = yaml.safe_load(yaml_text) or {}
        recovered[profile_id] = doc
    return recovered


def load_current_profiles() -> dict[str, dict[str, Any]]:
    profiles: dict[str, dict[str, Any]] = {}
    for path in sorted(PROFILES_DIR.glob("*.yaml")):
        doc = load_yaml(path)
        if not doc:
            continue
        pid = str(doc.get("id") or path.stem)
        profiles[pid] = doc
    return profiles


def resolve_profile(
    raw_id: str,
    current: dict[str, dict[str, Any]],
    orphans: dict[str, dict[str, Any]],
) -> tuple[dict[str, Any] | None, str]:
    """Return (profile_yaml_dict, resolved_id) or (None, raw_id) if unresolved.

    Resolution order: PROFILE_ALIASES → current profiles → recovered orphans.
    """
    aliased = PROFILE_ALIASES.get(raw_id, raw_id)
    if aliased in current:
        return current[aliased], aliased
    if raw_id in current:
        return current[raw_id], raw_id
    # The alias target may itself be missing; fall back to orphan recovery
    # under the *original* id (recovers the historical YAML the cell was run
    # against).
    if raw_id in orphans:
        return orphans[raw_id], raw_id
    if aliased in orphans:
        return orphans[aliased], aliased
    return None, raw_id


def parse_started_at(report: dict[str, Any]) -> datetime:
    raw = report.get("started_at") or report.get("finished_at")
    if not raw:
        return datetime.now(timezone.utc)
    text = str(raw)
    # Normalise trailing Z and trim sub-microsecond precision Python's
    # fromisoformat cannot read.
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    if "." in text:
        head, _, tail = text.partition(".")
        # tail may carry fractional seconds + timezone (e.g. ``123456789+00:00``).
        for sep in ("+", "-"):
            idx = tail.find(sep)
            if idx >= 0:
                frac, tz = tail[:idx], tail[idx:]
                tail = frac[:6] + tz
                break
        else:
            tail = tail[:6]
        text = f"{head}.{tail}"
    try:
        dt = datetime.fromisoformat(text)
    except ValueError:
        return datetime.now(timezone.utc)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc)


def mint_cell_id(report: dict[str, Any]) -> str:
    dt = parse_started_at(report)
    stamp = dt.strftime("%Y%m%dT%H%M%SZ")
    suffix = secrets.token_hex(2)  # 4 hex chars
    return f"{stamp}-{suffix}"


def split_cells_path(report_path: Path) -> tuple[Path, list[str]] | None:
    """Split a report.json path into (canonical_root_including_cells, parts_after).

    Returns None if the path does not contain a ``cells`` directory segment.
    """
    parts = report_path.parts
    for i, part in enumerate(parts):
        if part == "cells":
            canonical_root = Path(*parts[: i + 1])
            tail = list(parts[i + 1 :])
            return canonical_root, tail
    return None


def derive_framework_dataset(
    layout: str,
    tail: list[str],
    report: dict[str, Any],
) -> tuple[str, str]:
    """Pick framework/dataset for the cell.

    Layout A (fiz-tools-v1): ``<framework-dataset>/<task>/<profile_id>/rep-NNN/report.json``.
    Layout B (iteration-v1/hello): ``<harness>/<profile_id>/rep-NNN/<task>/report.json``.
    """
    if layout == "A":
        seg = tail[0]
        if seg.startswith("terminal-bench"):
            return "terminal-bench", seg
        # Future framework prefixes can be added here. Fall back to default.
        return seg.split("-", 1)[0] or DEFAULT_FRAMEWORK, seg
    # Layout B — no framework segment in path; infer from harness/task.
    harness = str(report.get("harness") or tail[0])
    if harness == "dumb_script":
        return SMOKE_FRAMEWORK, SMOKE_DATASET
    return DEFAULT_FRAMEWORK, DEFAULT_DATASET


def classify_layout(tail: list[str]) -> str | None:
    """Return 'A', 'B', or 'C' for a recognized cell layout, otherwise None.

    A: ``<framework-dataset>/<task>/<profile_id>/rep-NNN/report.json`` (legacy fiz-tools-v1)
    B: ``<harness>/<profile_id>/rep-NNN/<task>/report.json`` (legacy iteration-v1/hello)
    C: ``<framework-dataset>/<task>/<cell-id>/report.json`` (canonical ADR-016)
    """
    if not tail or tail[-1] != "report.json":
        return None
    if len(tail) == 4:
        # Canonical: only path layout already in target shape; idempotent skip.
        return "C"
    if len(tail) != 5:
        return None
    if tail[3].startswith("rep-"):
        return "A"
    if tail[2].startswith("rep-"):
        return "B"
    return None


def cell_source_dir(report_path: Path, layout: str) -> Path:
    """Return the directory whose contents constitute the cell.

    For Layout A the cell dir is ``rep-NNN`` (the parent of report.json).
    For Layout B the cell dir is ``<task>`` (also the parent of report.json).
    """
    return report_path.parent


def cell_path_dirs_to_prune(report_path: Path, layout: str) -> list[Path]:
    """Return ancestor directories that may be empty after the cell moves."""
    src_dir = cell_source_dir(report_path, layout)
    if layout == "A":
        # rep-NNN / <profile_id> / <task> may all become empty.
        return [src_dir.parent, src_dir.parent.parent]
    # Layout B: <task> sits under <rep-NNN>/<profile_id>/<harness>.
    return [src_dir.parent, src_dir.parent.parent, src_dir.parent.parent.parent]


def is_backfilled(report: dict[str, Any]) -> bool:
    profile = report.get("profile")
    return isinstance(profile, dict) and bool(profile.get("id"))


def plan_action(
    report_path: Path,
    current_profiles: dict[str, dict[str, Any]],
    orphan_profiles: dict[str, dict[str, Any]],
) -> Action | None:
    split = split_cells_path(report_path)
    if split is None:
        return None
    cells_root, tail = split
    layout = classify_layout(tail)
    if layout is None:
        sys.stderr.write(
            f"warn: skipping report with unrecognised layout: {report_path}\n"
        )
        return None
    try:
        with report_path.open(encoding="utf-8") as fh:
            report = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        sys.stderr.write(f"warn: cannot read {report_path}: {exc}\n")
        return None
    if layout == "C" or is_backfilled(report):
        return Action(
            kind="skip-backfilled",
            src=cell_source_dir(report_path, layout),
            dst=None,
            profile_id=str(report.get("profile", {}).get("id", "")),
            resolved_id=str(report.get("profile", {}).get("id", "")),
        )
    raw_profile_id = str(report.get("profile_id") or "")
    resolved, resolved_id = resolve_profile(
        raw_profile_id, current_profiles, orphan_profiles
    )
    src_dir = cell_source_dir(report_path, layout)
    if resolved is None:
        return Action(
            kind="delete-irrecoverable",
            src=src_dir,
            dst=None,
            profile_id=raw_profile_id,
            resolved_id=resolved_id,
            note="profile YAML not found in profiles/ or recovered orphans",
        )
    framework, dataset = derive_framework_dataset(layout, tail, report)
    task_id = str(report.get("task_id") or "")
    if not task_id:
        # fall back to path-derived task
        if layout == "A":
            task_id = tail[1]
        else:
            task_id = tail[3]
    cell_id = mint_cell_id(report)
    dst_dir = (
        cells_root
        / framework_dataset_segment(framework, dataset)
        / task_id.replace("/", "_")
        / cell_id
    )
    return Action(
        kind="embed-move",
        src=src_dir,
        dst=dst_dir,
        profile_id=raw_profile_id,
        resolved_id=resolved_id,
        note=f"framework={framework} dataset={dataset} cell_id={cell_id}",
        framework=framework,
        dataset=dataset,
        cell_id=cell_id,
        layout=layout,
    )


def apply_embed_move(
    action: Action,
    report_path: Path,
    profile_yaml: dict[str, Any],
    framework: str,
    dataset: str,
    cell_id: str,
) -> None:
    # 1. Update report.json in place with embedded profile + new fields.
    with report_path.open(encoding="utf-8") as fh:
        report = json.load(fh)
    embedded = dict(profile_yaml)
    embedded.pop("_path", None)
    report["profile"] = embedded
    report["framework"] = framework
    report["dataset"] = dataset
    report["cell_id"] = cell_id
    # Persist atomically.
    tmp_path = report_path.with_suffix(".json.tmp")
    with tmp_path.open("w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2, sort_keys=False)
        fh.write("\n")
    os.replace(tmp_path, report_path)
    # 2. Move the cell directory.
    assert action.dst is not None
    action.dst.parent.mkdir(parents=True, exist_ok=True)
    if action.dst.exists():
        # Extremely unlikely collision (same started_at second + same 4-hex
        # suffix). Mint a fresh suffix and retry once.
        retry_id = cell_id.rsplit("-", 1)[0] + "-" + secrets.token_hex(2)
        action = Action(
            kind=action.kind,
            src=action.src,
            dst=action.dst.parent / retry_id,
            profile_id=action.profile_id,
            resolved_id=action.resolved_id,
            note=action.note + f" (retry suffix {retry_id})",
        )
    shutil.move(str(action.src), str(action.dst))


def prune_empty_dirs(candidates: list[Path]) -> None:
    for path in candidates:
        try:
            path.rmdir()
        except OSError:
            # Not empty or already gone — fine.
            pass


def run(args: argparse.Namespace) -> int:
    benchmark_results = args.benchmark_results.resolve()
    if not benchmark_results.is_dir():
        sys.stderr.write(f"error: not a directory: {benchmark_results}\n")
        return 2

    workdir = Path(args.orphan_workdir).resolve()
    orphan_profiles = load_orphan_profiles(workdir)
    current_profiles = load_current_profiles()

    report_paths = sorted(benchmark_results.glob("**/cells/**/report.json"))
    actions: list[tuple[Action, Path]] = []
    for report_path in report_paths:
        action = plan_action(report_path, current_profiles, orphan_profiles)
        if action is None:
            continue
        actions.append((action, report_path))

    counts = {"embed-move": 0, "skip-backfilled": 0, "delete-irrecoverable": 0}
    for action, _ in actions:
        counts[action.kind] = counts.get(action.kind, 0) + 1

    def label(action: Action) -> str:
        if action.kind == "embed-move":
            return f"EMBED+MOVE  {action.src} -> {action.dst}  [{action.resolved_id}]"
        if action.kind == "skip-backfilled":
            return f"SKIP        {action.src}  [{action.profile_id}]"
        return f"DELETE      {action.src}  [{action.profile_id}: {action.note}]"

    for action, _ in actions:
        sys.stdout.write(label(action) + "\n")

    sys.stdout.write(
        f"\nplanned: embed-move={counts['embed-move']} "
        f"skip-backfilled={counts['skip-backfilled']} "
        f"delete-irrecoverable={counts['delete-irrecoverable']}\n"
    )

    # Recipe-named subdirs and .lane_aborted (fiz-tools-v1 only — these names
    # are operational artefacts that pre-date the canonical layout).
    fiz_tools_root = benchmark_results / "fiz-tools-v1"
    lane_aborted = fiz_tools_root / "cells" / ".lane_aborted"
    recipe_dirs = [fiz_tools_root / name for name in RECIPE_SUBDIRS]
    cleanup_targets = []
    if lane_aborted.is_dir():
        cleanup_targets.append(lane_aborted)
    cleanup_targets.extend(d for d in recipe_dirs if d.is_dir())
    for path in cleanup_targets:
        sys.stdout.write(f"CLEANUP     {path}\n")

    if args.dry_run:
        sys.stdout.write("\n(dry-run: no changes applied)\n")
        return 0

    # Apply changes. Log everything.
    log_path = args.log.resolve()
    log_path.parent.mkdir(parents=True, exist_ok=True)
    with log_path.open("w", encoding="utf-8") as log:
        log.write(f"# backfill-cell-metadata.py run at {datetime.now(timezone.utc).isoformat()}\n")
        log.write(f"# benchmark_results={benchmark_results}\n")
        log.write(f"# orphan_workdir={workdir}\n\n")

        for action, report_path in actions:
            if action.kind == "skip-backfilled":
                log.write(f"skip\t{report_path}\n")
                continue
            if action.kind == "delete-irrecoverable":
                shutil.rmtree(action.src, ignore_errors=True)
                log.write(
                    f"delete\t{action.src}\tprofile_id={action.profile_id}\t{action.note}\n"
                )
                continue
            # embed-move
            assert action.dst is not None
            framework, dataset, cell_id = action.framework, action.dataset, action.cell_id
            resolved_yaml = (
                current_profiles.get(action.resolved_id)
                or orphan_profiles.get(action.resolved_id)
                or orphan_profiles.get(action.profile_id)
                or current_profiles.get(action.profile_id)
            )
            if resolved_yaml is None:
                log.write(
                    f"error\t{report_path}\tresolved profile vanished mid-run: {action.resolved_id}\n"
                )
                continue
            apply_embed_move(
                action,
                report_path,
                resolved_yaml,
                framework,
                dataset,
                cell_id,
            )
            log.write(
                f"move\t{action.src} -> {action.dst}\tprofile_id={action.profile_id}\tresolved={action.resolved_id}\n"
            )
            prune_empty_dirs(cell_path_dirs_to_prune(report_path, action.layout))

        for path in cleanup_targets:
            shutil.rmtree(path, ignore_errors=True)
            log.write(f"cleanup\t{path}\n")

    sys.stdout.write(f"\nlive run complete; manifest written to {log_path}\n")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="List planned actions without modifying disk.",
    )
    parser.add_argument(
        "--benchmark-results",
        type=Path,
        default=DEFAULT_BENCHMARK_RESULTS,
        help=f"Path to benchmark-results/ (default: {DEFAULT_BENCHMARK_RESULTS}).",
    )
    parser.add_argument(
        "--orphan-workdir",
        type=Path,
        default=Path("/tmp/orphan-profiles"),
        help="Where recovered orphan profile YAMLs are written.",
    )
    parser.add_argument(
        "--log",
        type=Path,
        default=None,
        help="Manifest log path (default: <benchmark-results>/backfill-2026-05-16.log).",
    )
    args = parser.parse_args()
    if args.log is None:
        args.log = args.benchmark_results / "backfill-2026-05-16.log"
    return run(args)


if __name__ == "__main__":
    sys.exit(main())
