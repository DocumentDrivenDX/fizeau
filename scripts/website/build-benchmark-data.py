#!/usr/bin/env python3
"""Build normalized benchmark data feeds for the microsite.

The generator walks per-trial benchmark report.json files, joins each trial to
profile YAML and machine metadata, and writes denormalized rows for analytical
querying. Parquet is the primary static artifact for DuckDB-WASM. JSON feeds are
kept for existing summary pages while the microsite migrates away from treating
the browser DOM as the data engine.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import math
import re
import shutil
from collections import defaultdict
from pathlib import Path
from statistics import median
from typing import Any

import yaml


REPO = Path(__file__).resolve().parents[2]
DEFAULT_CELLS_ROOT = REPO / "benchmark-results/fiz-tools-v1/cells"
DEFAULT_PROFILES_DIR = REPO / "scripts/benchmark/profiles"
DEFAULT_MACHINES_FILE = REPO / "scripts/benchmark/machines.yaml"
DEFAULT_SUBSET_GLOB = "scripts/benchmark/task-subset-tb21-*.yaml"
DEFAULT_OUT_DIR = REPO / "website/static/data"
DEFAULT_SCHEMA = REPO / "docs/benchmarks/schema/benchmark-cells.schema.json"
DEFAULT_SUITE = "terminal-bench-2-1"
DEFAULT_FORMATS = ("json", "parquet")
PARQUET_TYPE_BOOL = "bool"
PARQUET_TYPE_INT64 = "int64"
PARQUET_TYPE_FLOAT64 = "float64"
PARQUET_TYPE_STRING = "string"
PARQUET_COLUMN_TYPES = {
    "any_pass": PARQUET_TYPE_BOOL,
    "grader_passed": PARQUET_TYPE_BOOL,
    "had_llm_request": PARQUET_TYPE_BOOL,
    "kv_cache_disk": PARQUET_TYPE_BOOL,
    "passed": PARQUET_TYPE_BOOL,
    "profile_exists": PARQUET_TYPE_BOOL,
    "reasoning_tokens_approx": PARQUET_TYPE_BOOL,
    "runtime_mtp_enabled": PARQUET_TYPE_BOOL,
    "terminated_mid_work": PARQUET_TYPE_BOOL,
    "cached_input_tokens": PARQUET_TYPE_INT64,
    "context_tokens": PARQUET_TYPE_INT64,
    "exit_code": PARQUET_TYPE_INT64,
    "fiz_tools_version": PARQUET_TYPE_INT64,
    "input_tokens": PARQUET_TYPE_INT64,
    "max_output_tokens": PARQUET_TYPE_INT64,
    "n_attempts": PARQUET_TYPE_INT64,
    "n_fail": PARQUET_TYPE_INT64,
    "n_graded": PARQUET_TYPE_INT64,
    "n_invalid": PARQUET_TYPE_INT64,
    "n_pass": PARQUET_TYPE_INT64,
    "n_real": PARQUET_TYPE_INT64,
    "n_timeout": PARQUET_TYPE_INT64,
    "n_ungraded": PARQUET_TYPE_INT64,
    "output_tokens": PARQUET_TYPE_INT64,
    "reasoning_tokens": PARQUET_TYPE_INT64,
    "rep": PARQUET_TYPE_INT64,
    "retried_input_tokens": PARQUET_TYPE_INT64,
    "runtime_max_context": PARQUET_TYPE_INT64,
    "tokens_consumed": PARQUET_TYPE_INT64,
    "tool_call_errors": PARQUET_TYPE_INT64,
    "tool_calls": PARQUET_TYPE_INT64,
    "total_cached_input_tokens": PARQUET_TYPE_INT64,
    "total_input_tokens": PARQUET_TYPE_INT64,
    "total_output_tokens": PARQUET_TYPE_INT64,
    "total_reasoning_tokens": PARQUET_TYPE_INT64,
    "total_retried_input_tokens": PARQUET_TYPE_INT64,
    "total_tokens": PARQUET_TYPE_INT64,
    "turns": PARQUET_TYPE_INT64,
    "avg_cost_usd": PARQUET_TYPE_FLOAT64,
    "cost_per_1k_tokens": PARQUET_TYPE_FLOAT64,
    "cost_per_pass_usd": PARQUET_TYPE_FLOAT64,
    "cost_usd": PARQUET_TYPE_FLOAT64,
    "gpu_ram_gb": PARQUET_TYPE_FLOAT64,
    "hardware_memory_gb": PARQUET_TYPE_FLOAT64,
    "hardware_tdp_watts": PARQUET_TYPE_FLOAT64,
    "hardware_tdp_watts_spec": PARQUET_TYPE_FLOAT64,
    "hardware_vram_gb": PARQUET_TYPE_FLOAT64,
    "kv_cache_disk_gb": PARQUET_TYPE_FLOAT64,
    "max_wall_seconds": PARQUET_TYPE_FLOAT64,
    "median_cached_input_tokens": PARQUET_TYPE_FLOAT64,
    "median_cost_usd": PARQUET_TYPE_FLOAT64,
    "median_input_tokens": PARQUET_TYPE_FLOAT64,
    "median_output_tokens": PARQUET_TYPE_FLOAT64,
    "median_reasoning_tokens": PARQUET_TYPE_FLOAT64,
    "median_retried_input_tokens": PARQUET_TYPE_FLOAT64,
    "median_total_tokens": PARQUET_TYPE_FLOAT64,
    "median_turns": PARQUET_TYPE_FLOAT64,
    "median_wall_seconds": PARQUET_TYPE_FLOAT64,
    "min_wall_seconds": PARQUET_TYPE_FLOAT64,
    "pass_rate": PARQUET_TYPE_FLOAT64,
    "reward": PARQUET_TYPE_FLOAT64,
    "sampling_temperature": PARQUET_TYPE_FLOAT64,
    "sampling_top_k": PARQUET_TYPE_FLOAT64,
    "sampling_top_p": PARQUET_TYPE_FLOAT64,
    "total_cost_usd": PARQUET_TYPE_FLOAT64,
    "wall_seconds": PARQUET_TYPE_FLOAT64,
    "weight_bits": PARQUET_TYPE_FLOAT64,
}


PUBLIC_PROFILE_LABELS = {
    "sindri-club-3090": "sindri-vllm",
    "sindri-club-3090-llamacpp": "sindri-llamacpp",
    "bragi-club-3090": "local-vllm-rtx3090",
    "bragi-qwen3-6-27b": "local-lmstudio-qwen3-6-27b",
    "grendel-rapid-mlx": "local-rapidmlx-qwen3-6-27b",
    "vidar-ds4": "local-ds4-deepseek-v4-flash",
    "vidar-qwen3-6-27b": "local-omlx-qwen3-6-27b",
    "vidar-qwen3-6-27b-openai-compat": "local-omlx-qwen3-6-27b-openai-compat",
}

PROFILE_ALIASES = {
    "sindri-club-3090": "sindri-vllm",
    "sindri-club-3090-llamacpp": "sindri-llamacpp",
}

CLOUD_PROVIDERS = {"anthropic", "google", "openai", "openrouter"}
RESULT_STATE_PASSED = "passed"
RESULT_STATE_FAILED = "failed"
RESULT_STATE_TIMEOUT = "timeout"
RESULT_STATES = {RESULT_STATE_PASSED, RESULT_STATE_FAILED, RESULT_STATE_TIMEOUT}
TIMEOUT_OUTCOMES = {"timeout", "timed_out", "agent_timeout"}

AGGREGATE_DIMENSIONS = [
    "suite",
    "task",
    "task_category",
    "task_difficulty",
    "harness",
    "harness_label",
    "lane_label",
    "descriptor",
    "deployment_class",
    "provider_type",
    "provider_surface",
    "provider_model",
    "model",
    "model_id",
    "model_display_name",
    "model_family",
    "quant_display",
    "model_quant",
    "weight_bits",
    "kv_cache_quant",
    "k_quant",
    "v_quant",
    "kv_cache_disk",
    "engine",
    "backend",
    "runtime_draft_model",
    "runtime_draft_mode",
    "runtime_mtp_enabled",
    "machine",
    "machine_label",
    "hardware_profile",
    "hardware_chip",
    "hardware_chip_family",
    "gpu_vendor",
    "gpu_model",
    "gpu_ram_gb",
    "hardware_vram_gb",
    "hardware_memory_gb",
    "hardware_memory_type",
    "hardware_tdp_watts",
    "hardware_tdp_source",
    "sampling_reasoning",
    "sampling_temperature",
    "sampling_top_p",
    "sampling_top_k",
    "max_output_tokens",
    "context_tokens",
]


def relpath(path: Path) -> str:
    try:
        return str(path.resolve().relative_to(REPO))
    except ValueError:
        return str(path)


def load_yaml(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {}
    with path.open(encoding="utf-8") as f:
        return yaml.safe_load(f) or {}


def load_profiles(profiles_dir: Path) -> dict[str, dict[str, Any]]:
    profiles: dict[str, dict[str, Any]] = {}
    for path in sorted(profiles_dir.glob("*.yaml")):
        doc = load_yaml(path)
        if not doc:
            continue
        profile_id = str(doc.get("id") or path.stem)
        doc["_path"] = relpath(path)
        profiles[profile_id] = doc
    return profiles


def load_machine_registry(path: Path) -> tuple[dict[str, dict[str, Any]], dict[str, dict[str, Any]]]:
    doc = load_yaml(path)
    return doc.get("machines") or {}, doc.get("hardware_profiles") or {}


def load_subsets(pattern: str) -> dict[str, set[str]]:
    subsets: dict[str, set[str]] = {}
    for path in sorted(REPO.glob(pattern)):
        doc = load_yaml(path)
        tasks = {
            str(task["id"])
            for task in doc.get("tasks") or []
            if isinstance(task, dict) and task.get("id")
        }
        subsets[path.stem.replace("task-subset-tb21-", "")] = tasks
    return subsets


def iter_report_paths(cells_root: Path, suite: str) -> list[Path]:
    suite_root = cells_root / suite
    return sorted(suite_root.glob("*/*/rep-*/report.json"))


def public_profile_label(profile_id: str) -> str:
    return PUBLIC_PROFILE_LABELS.get(profile_id, profile_id)


def coalesce(*values: Any) -> Any:
    for value in values:
        if value is not None and value != "":
            return value
    return None


def public_text(value: Any) -> str | None:
    if value is None:
        return None
    text = str(value)
    text = re.sub(r"https?://[^\s|]+", "[redacted-url]", text)
    text = re.sub(r"/Users/[^\s|]+", "[redacted-path]", text)
    text = re.sub(r"\b(?:10|127|169\.254|172\.(?:1[6-9]|2\d|3[0-1])|192\.168)(?:\.\d{1,3}){2}\b(?::\d+)?", "[redacted-host]", text)
    return text


def public_value(value: Any) -> Any:
    if isinstance(value, dict):
        return {str(k): public_value(v) for k, v in value.items()}
    if isinstance(value, list):
        return [public_value(v) for v in value]
    if isinstance(value, str):
        return public_text(value)
    return value


def column_name(*parts: str) -> str:
    raw = "_".join(part for part in parts if part)
    raw = re.sub(r"[^a-zA-Z0-9]+", "_", raw).strip("_").lower()
    return raw


def json_text(value: Any) -> str | None:
    if value is None:
        return None
    return json.dumps(public_value(value), sort_keys=True, separators=(",", ":"))


def flatten(prefix: str, value: Any) -> dict[str, Any]:
    out: dict[str, Any] = {}
    if isinstance(value, dict):
        out[f"{prefix}_json"] = json_text(value)
        for key, child in value.items():
            child_key = column_name(prefix, str(key))
            if isinstance(child, dict):
                out.update(flatten(child_key, child))
            elif isinstance(child, list):
                out[child_key] = json_text(child)
                out[f"{child_key}_count"] = len(child)
            else:
                out[child_key] = public_value(child)
    elif isinstance(value, list):
        out[prefix] = json_text(value)
        out[f"{prefix}_count"] = len(value)
    else:
        out[prefix] = public_value(value)
    return out


def number(value: Any) -> float | int | None:
    if value is None or value == "":
        return None
    if isinstance(value, (int, float)):
        return value
    try:
        parsed = float(str(value))
    except ValueError:
        return None
    return int(parsed) if parsed.is_integer() else parsed


def integer(value: Any) -> int | None:
    parsed = number(value)
    return int(parsed) if parsed is not None else None


def bool_or_none(value: Any) -> bool | None:
    if value is None:
        return None
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        lowered = value.lower()
        if lowered in {"true", "yes", "1"}:
            return True
        if lowered in {"false", "no", "0"}:
            return False
    return None


def gb_from_mb(value: Any) -> float | None:
    parsed = number(value)
    if parsed is None:
        return None
    return round(float(parsed) / 1024.0, 2)


def extract_weight_bits(*labels: Any) -> float | None:
    for label in labels:
        if not label:
            continue
        text = str(label).lower()
        match = re.search(r"(?:iq|q|int|fp)(\d+(?:\.\d+)?)", text)
        if match:
            return float(match.group(1))
        match = re.search(r"(\d+(?:\.\d+)?)\s*[- ]?bit", text)
        if match:
            return float(match.group(1))
    return None


def split_kv_quant(value: Any) -> tuple[str | None, str | None]:
    if not value:
        return None, None
    text = str(value)
    lowered = text.lower()
    patterns = [
        (r"k(?:_cache)?[:=]([a-z0-9_.-]+)", r"v(?:_cache)?[:=]([a-z0-9_.-]+)"),
        (r"k([a-z0-9_.-]+)[/_-]v([a-z0-9_.-]+)", None),
    ]
    first = re.search(patterns[0][0], lowered)
    second = re.search(patterns[0][1], lowered)
    if first or second:
        return (first.group(1) if first else None), (second.group(1) if second else None)
    pair = re.search(patterns[1][0], lowered)
    if pair:
        return pair.group(1), pair.group(2)
    return text, text


def harness_label(profile_id: str, profile: dict[str, Any] | None, report_harness: str | None) -> str | None:
    metadata = (profile or {}).get("metadata") or {}
    runtime = str(metadata.get("runtime") or "").lower()
    if runtime == "claude-code" or profile_id.startswith("claude-native-"):
        return "Claude Code (native CLI)"
    if runtime == "codex" or profile_id.startswith("codex-native-"):
        return "Codex (native CLI)"
    if profile_id.startswith("fiz-harness-claude-"):
        return "Claude Code (wrapped by fiz)"
    if profile_id.startswith("fiz-harness-codex-"):
        return "Codex (wrapped by fiz)"
    if profile_id.startswith("fiz-harness-opencode-"):
        return "OpenCode (wrapped by fiz)"
    if profile_id.startswith("fiz-harness-pi-"):
        return "Pi (wrapped by fiz)"
    if report_harness:
        return "fiz (built-in agent loop)" if report_harness == "fiz" else report_harness
    return None


def model_display_name(model_family: str | None, model_id: str | None) -> str | None:
    model_text = str(model_id or "").lower()
    model_patterns = [
        ("qwen3.6-27b", "Qwen3.6 27B"),
        ("deepseek-v4-flash", "DeepSeek V4 Flash"),
        ("gpt-5.5", "GPT-5.5"),
        ("gpt-5.4-mini", "GPT-5.4 Mini"),
        ("claude-sonnet-4.6", "Claude Sonnet 4.6"),
        ("claude-4.6-sonnet", "Claude Sonnet 4.6"),
        ("claude-opus-4.6", "Claude Opus 4.6"),
    ]
    for needle, display in model_patterns:
        if needle in model_text:
            return display

    raw = model_family or model_id
    if not raw:
        return None
    known = {
        "qwen3-6-27b": "Qwen3.6 27B",
        "deepseek-v4-flash": "DeepSeek V4 Flash",
        "claude-sonnet-4-6": "Claude Sonnet 4.6",
        "claude-sonnet-4": "Claude Sonnet 4",
        "gpt-5-4-mini": "GPT-5.4 Mini",
        "gpt-5-mini": "GPT-5 Mini",
        "gpt-5-5": "GPT-5.5",
    }
    if str(raw) in known:
        return known[str(raw)]
    text = str(raw).split("/")[-1]
    text = re.sub(r"\.(gguf|safetensors)$", "", text, flags=re.IGNORECASE)
    return text.replace("_", " ").replace("-", " ")


def machine_for_profile(profile: dict[str, Any] | None, machines: dict[str, dict[str, Any]]) -> tuple[str | None, dict[str, Any] | None]:
    metadata = (profile or {}).get("metadata") or {}
    server = metadata.get("server")
    if not server:
        return None, None
    return str(server), machines.get(str(server))


def tdp(machine: dict[str, Any] | None, hardware_profile: dict[str, Any] | None) -> tuple[float | None, str | None]:
    if machine and machine.get("tdp_watts_configured") is not None:
        return number(machine.get("tdp_watts_configured")), "configured"
    if hardware_profile and hardware_profile.get("tdp_watts_spec") is not None:
        return number(hardware_profile.get("tdp_watts_spec")), "spec"
    return None, None


def compose_descriptor(row: dict[str, Any]) -> str:
    parts = [
        row.get("model_display_name") or row.get("model_id") or row.get("model"),
        row.get("quant_display"),
        row.get("engine"),
    ]
    hardware = row.get("gpu_model") or row.get("hardware_chip")
    vram = row.get("hardware_vram_gb")
    memory = row.get("hardware_memory_gb")
    if hardware and vram and vram > 0:
        hardware = f"{hardware} ({vram:g} GB VRAM)"
    elif hardware and memory:
        hardware = f"{hardware} ({memory:g} GB memory)"
    parts.append(hardware)
    if row.get("hardware_tdp_watts"):
        parts.append(f"{row['hardware_tdp_watts']:g} W")
    if row.get("kv_cache_disk_gb"):
        parts.append(f"KV on disk {row['kv_cache_disk_gb']:g} GB")
    return " · ".join(str(part) for part in parts if part)


def runtime_props_from(report: dict[str, Any]) -> dict[str, Any]:
    props = report.get("runtime_props")
    return props if isinstance(props, dict) else {}


def outcome_label(report: dict[str, Any]) -> str:
    if report.get("invalid_class"):
        return "invalid"
    reward = number(report.get("reward"))
    if reward is not None:
        return "pass" if reward > 0 else "fail"
    if report.get("grading_outcome") == "ungraded":
        return "ungraded"
    return str(report.get("final_status") or "unknown")


def is_timeout_result(row: dict[str, Any]) -> bool:
    process_outcome = str(row.get("process_outcome") or "").lower()
    final_status = str(row.get("final_status") or "").lower()
    return (
        row.get("terminated_mid_work") is True
        or process_outcome in TIMEOUT_OUTCOMES
        or final_status in TIMEOUT_OUTCOMES
    )


def result_state_for_row(row: dict[str, Any]) -> str | None:
    """Return the public analytical result state, or None when the row is not a
    result-bearing benchmark attempt.

    The microsite treats invalid setup/provider/auth rows as source artifacts,
    not benchmark results. A timeout is first-class even when the grader later
    produced a reward, because the run did not exit normally.
    """
    if row.get("invalid_class"):
        return None
    if is_timeout_result(row):
        return RESULT_STATE_TIMEOUT
    if (
        row.get("process_outcome") == "completed"
        and row.get("grading_outcome") == "graded"
        and row.get("reward") is not None
    ):
        return RESULT_STATE_PASSED if row.get("reward") > 0 else RESULT_STATE_FAILED
    return None


def exclusion_reason_for_row(row: dict[str, Any]) -> str:
    if row.get("invalid_class"):
        return f"invalid:{row['invalid_class']}"
    if row.get("process_outcome") and row.get("process_outcome") != "completed":
        return f"process:{row['process_outcome']}"
    if row.get("grading_outcome") != "graded":
        status = row.get("final_status") or "unknown"
        return f"ungraded:{status}"
    if row.get("reward") is None:
        return "graded:missing_reward"
    return "not_result_bearing"


def apply_result_state(row: dict[str, Any], result_state: str) -> None:
    row["result_state"] = result_state
    row["pass_fail"] = {
        RESULT_STATE_PASSED: "pass",
        RESULT_STATE_FAILED: "fail",
        RESULT_STATE_TIMEOUT: "timeout",
    }[result_state]
    row["passed"] = result_state == RESULT_STATE_PASSED


def build_cell_row(
    report_path: Path,
    cells_root: Path,
    profiles: dict[str, dict[str, Any]],
    machines: dict[str, dict[str, Any]],
    hardware_profiles: dict[str, dict[str, Any]],
    subsets: dict[str, set[str]],
) -> dict[str, Any]:
    with report_path.open(encoding="utf-8") as f:
        report = json.load(f)

    rel = report_path.relative_to(cells_root)
    suite, task, lane_id, rep_dir = rel.parts[:4]
    profile_id = str(report.get("profile_id") or lane_id)
    profile_resolved_id = profile_id
    profile = profiles.get(profile_id) or profiles.get(lane_id)
    if profile is None:
        profile_resolved_id = PROFILE_ALIASES.get(profile_id) or PROFILE_ALIASES.get(lane_id) or profile_id
        profile = profiles.get(profile_resolved_id)
    metadata = (profile or {}).get("metadata") or {}
    provider = (profile or {}).get("provider") or {}
    limits = (profile or {}).get("limits") or {}
    sampling = report.get("sampling_used") or (profile or {}).get("sampling") or {}
    runtime_props = runtime_props_from(report)
    machine_id, machine = machine_for_profile(profile, machines)
    hardware_profile_id = machine.get("hardware_profile") if machine else None
    hardware_profile = hardware_profiles.get(str(hardware_profile_id)) if hardware_profile_id else None
    hardware = (machine or {}).get("hardware") or {}

    provider_type = provider.get("type")
    engine = coalesce(metadata.get("engine"), metadata.get("runtime"), provider_type)
    model_id = coalesce(metadata.get("model_id"), provider.get("model"), runtime_props.get("base_model"))
    model = coalesce(provider.get("model"), metadata.get("model_id"), runtime_props.get("base_model"))
    quant_display = coalesce(metadata.get("quant_display"), metadata.get("quant_label"), runtime_props.get("model_quant"))
    runtime_kv_quant = runtime_props.get("kv_quant")
    kv_cache_quant = coalesce(metadata.get("kv_cache_quant"), runtime_kv_quant)
    k_quant, v_quant = split_kv_quant(kv_cache_quant)
    kv_disk_mb = coalesce(metadata.get("kv_disk_space_mb"), metadata.get("kv_cache_disk_mb"))
    kv_cache_disk_gb = round(float(kv_disk_mb) / 1024.0, 2) if number(kv_disk_mb) is not None else None
    kv_cache_disk = bool(kv_cache_disk_gb) or bool(metadata.get("kv_disk_dir"))
    tdp_watts, tdp_source = tdp(machine, hardware_profile)
    hardware_memory_gb = coalesce(
        hardware.get("memory_gb"),
        (hardware_profile or {}).get("memory_gb"),
    )
    hardware_vram_gb = coalesce(
        gb_from_mb(hardware.get("gpu_vram_mb")),
        (hardware_profile or {}).get("vram_gb"),
    )
    reward = number(report.get("reward"))

    row = {
        "row_id": hashlib.sha256(relpath(report_path).encode("utf-8")).hexdigest()[:24],
        "schema_version": 1,
        "source_report": relpath(report_path),
        "suite": suite,
        "task": task,
        "test": task,
        "report_task_id": report.get("task_id"),
        "task_category": report.get("category"),
        "task_difficulty": report.get("difficulty"),
        "task_subsets": sorted(name for name, tasks in subsets.items() if task in tasks),
        "harness": report.get("harness"),
        "harness_label": harness_label(profile_id, profile, report.get("harness")),
        "internal_lane_id": lane_id,
        "profile_id": profile_id,
        "profile_resolved_id": profile_resolved_id if profile_resolved_id != profile_id else None,
        "profile_exists": profile is not None,
        "profile_path": (profile or {}).get("_path"),
        "lane_label": public_profile_label(profile_id),
        "descriptor": "",
        "deployment_class": "managed-cloud" if provider_type in CLOUD_PROVIDERS else ("self-hosted" if provider_type else None),
        "provider_type": provider_type,
        "provider_surface": metadata.get("provider_surface"),
        "provider_model": provider.get("model"),
        "model": model,
        "model_id": model_id,
        "model_display_name": coalesce(metadata.get("model_display_name"), model_display_name(metadata.get("model_family"), model_id)),
        "model_family": metadata.get("model_family"),
        "profile_snapshot": public_text(report.get("profile_snapshot") or ((profile or {}).get("versioning") or {}).get("snapshot")),
        "quant_display": quant_display,
        "model_quant": coalesce(runtime_props.get("model_quant"), metadata.get("quant_label")),
        "weight_bits": extract_weight_bits(runtime_props.get("model_quant"), metadata.get("weight_bits"), quant_display),
        "kv_cache_quant": kv_cache_quant,
        "k_quant": k_quant,
        "v_quant": v_quant,
        "kv_cache_disk": kv_cache_disk,
        "kv_cache_disk_gb": kv_cache_disk_gb,
        "engine": engine,
        "engine_version": coalesce(runtime_props.get("build_info"), ((machine or {}).get("serving") or {}).get("engine_version")),
        "backend": metadata.get("backend"),
        "runtime_props_extractor": runtime_props.get("extractor"),
        "runtime_base_model": runtime_props.get("base_model"),
        "runtime_model_quant": runtime_props.get("model_quant"),
        "runtime_kv_quant": runtime_kv_quant,
        "runtime_draft_model": runtime_props.get("draft_model"),
        "runtime_draft_mode": runtime_props.get("draft_mode"),
        "runtime_max_context": integer(runtime_props.get("max_context")),
        "runtime_mtp_enabled": bool_or_none(runtime_props.get("mtp_enabled")),
        "machine": machine_id,
        "machine_label": (machine or {}).get("label"),
        "hardware_profile": hardware_profile_id,
        "hardware_chip": (hardware_profile or {}).get("chip"),
        "hardware_chip_family": (hardware_profile or {}).get("chip_family"),
        "gpu_vendor": hardware.get("gpu_vendor"),
        "gpu_model": coalesce(hardware.get("gpu_model"), (machine or {}).get("gpu"), (hardware_profile or {}).get("chip")),
        "gpu_ram_gb": hardware_vram_gb,
        "hardware_vram_gb": hardware_vram_gb,
        "hardware_memory_gb": number(hardware_memory_gb),
        "hardware_memory_type": (hardware_profile or {}).get("memory_type"),
        "hardware_tdp_watts": tdp_watts,
        "hardware_tdp_source": tdp_source,
        "hardware_tdp_watts_spec": number((hardware_profile or {}).get("tdp_watts_spec")),
        "cpu_model": coalesce(hardware.get("cpu_model"), (machine or {}).get("cpu")),
        "os": coalesce(((machine or {}).get("snapshot") or {}).get("os_release"), (machine or {}).get("os")),
        "rep": integer(report.get("rep")) or integer(rep_dir.replace("rep-", "")) or 0,
        "adapter_module": report.get("adapter_module"),
        "adapter_translation_notes": json_text(report.get("adapter_translation_notes")),
        "adapter_translation_notes_count": len(report.get("adapter_translation_notes") or []),
        "harbor_agent": report.get("harbor_agent"),
        "command": json_text(report.get("command")),
        "command_string": " ".join(str(part) for part in report.get("command") or []) if report.get("command") else None,
        "output_dir": public_text(report.get("output_dir")),
        "report_profile_path": public_text(report.get("profile_path")),
        "pricing_source": public_text(report.get("pricing_source")),
        "process_outcome": report.get("process_outcome"),
        "grading_outcome": report.get("grading_outcome"),
        "reward": reward,
        "grader_passed": (reward > 0) if reward is not None else None,
        "passed": (reward or 0) > 0,
        "pass_fail": outcome_label(report),
        "final_status": str(report.get("final_status") or ""),
        "invalid_class": report.get("invalid_class"),
        "terminated_mid_work": bool_or_none(report.get("terminated_mid_work")),
        "had_llm_request": bool_or_none(report.get("had_llm_request")),
        "exit_code": integer(report.get("exit_code")),
        "error": public_text(report.get("error")),
        "turns": integer(report.get("turns")),
        "tool_calls": integer(report.get("tool_calls")),
        "tool_call_errors": integer(report.get("tool_call_errors")),
        "input_tokens": integer(report.get("input_tokens")),
        "output_tokens": integer(report.get("output_tokens")),
        "cached_input_tokens": integer(report.get("cached_input_tokens")),
        "retried_input_tokens": integer(report.get("retried_input_tokens")),
        "reasoning_tokens": integer(report.get("reasoning_tokens")),
        "reasoning_tokens_approx": bool_or_none(report.get("reasoning_tokens_approx")),
        "total_tokens": None,
        "tokens_consumed": None,
        "wall_seconds": number(report.get("wall_seconds")),
        "cost_usd": number(report.get("cost_usd")),
        "sampling_reasoning": sampling.get("reasoning"),
        "sampling_temperature": number(sampling.get("temperature")),
        "sampling_top_p": number(sampling.get("top_p")),
        "sampling_top_k": number(sampling.get("top_k")),
        "max_output_tokens": integer(limits.get("max_output_tokens")),
        "context_tokens": integer(limits.get("context_tokens")),
        "started_at": report.get("started_at"),
        "finished_at": report.get("finished_at"),
        "fiz_tools_version": integer(report.get("fiz_tools_version")),
        "dataset": report.get("dataset"),
        "dataset_version": report.get("dataset_version"),
        "raw_report_json": json_text(report),
        "profile_yaml_json": json_text(profile),
        "machine_yaml_json": json_text(machine),
        "hardware_profile_yaml_json": json_text(hardware_profile),
    }

    row.update(flatten("report", report))
    if profile:
        row.update(flatten("profile", profile))
        for section in ["metadata", "provider", "pricing", "limits", "sampling", "versioning"]:
            if isinstance(profile.get(section), dict):
                row.update(flatten(f"profile_{section}", profile[section]))
    if machine:
        row.update(flatten("machine_registry", machine))
        for section in ["snapshot", "hardware", "network_detail", "serving", "sampling_defaults"]:
            if isinstance(machine.get(section), dict):
                row.update(flatten(f"machine_{section}", machine[section]))
    if hardware_profile:
        row.update(flatten("hardware_profile_spec", hardware_profile))
    if report.get("model_server_info"):
        model_server_info = report["model_server_info"]
        row.update(flatten("model_server_info", model_server_info))

    token_parts = [
        row["input_tokens"],
        row["output_tokens"],
        row["cached_input_tokens"],
        row["retried_input_tokens"],
    ]
    if any(v is not None for v in token_parts):
        row["total_tokens"] = sum(v or 0 for v in token_parts)
        row["tokens_consumed"] = row["total_tokens"]
    if row.get("total_tokens") and row.get("cost_usd") is not None:
        row["cost_per_1k_tokens"] = row["cost_usd"] / (row["total_tokens"] / 1000.0)
    else:
        row["cost_per_1k_tokens"] = None
    row["descriptor"] = compose_descriptor(row)
    return row


def summarize_excluded_row(diagnostics: dict[str, Any], row: dict[str, Any]) -> None:
    reason = exclusion_reason_for_row(row)
    diagnostics["n_excluded"] += 1
    diagnostics["excluded_result_counts"][reason] += 1
    if row.get("invalid_class"):
        diagnostics["excluded_invalid_classes"][str(row["invalid_class"])] += 1
    if row.get("final_status"):
        diagnostics["excluded_final_statuses"][str(row["final_status"])] += 1
    if row.get("process_outcome"):
        diagnostics["excluded_process_outcomes"][str(row["process_outcome"])] += 1
    if row.get("grading_outcome"):
        diagnostics["excluded_grading_outcomes"][str(row["grading_outcome"])] += 1


def finalize_diagnostics(diagnostics: dict[str, Any]) -> dict[str, Any]:
    out = dict(diagnostics)
    for key in [
        "result_state_counts",
        "excluded_result_counts",
        "excluded_invalid_classes",
        "excluded_final_statuses",
        "excluded_process_outcomes",
        "excluded_grading_outcomes",
    ]:
        out[key] = dict(sorted(out[key].items()))
    return out


def build_cell_dataset(
    cells_root: Path,
    suites: list[str],
    profiles_dir: Path,
    machines_file: Path,
    subset_glob: str,
) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    profiles = load_profiles(profiles_dir)
    machines, hardware_profiles = load_machine_registry(machines_file)
    subsets = load_subsets(subset_glob)
    rows: list[dict[str, Any]] = []
    diagnostics: dict[str, Any] = {
        "n_reports": 0,
        "n_rows": 0,
        "n_excluded": 0,
        "result_state_counts": defaultdict(int),
        "excluded_result_counts": defaultdict(int),
        "excluded_invalid_classes": defaultdict(int),
        "excluded_final_statuses": defaultdict(int),
        "excluded_process_outcomes": defaultdict(int),
        "excluded_grading_outcomes": defaultdict(int),
    }
    for suite in suites:
        for report_path in iter_report_paths(cells_root, suite):
            diagnostics["n_reports"] += 1
            row = build_cell_row(report_path, cells_root, profiles, machines, hardware_profiles, subsets)
            result_state = result_state_for_row(row)
            if result_state is None:
                summarize_excluded_row(diagnostics, row)
                continue
            apply_result_state(row, result_state)
            diagnostics["result_state_counts"][result_state] += 1
            rows.append(row)
    normalize_columns(rows)
    rows.sort(key=lambda r: (r["suite"], r["task"], r["internal_lane_id"], r["rep"], r["source_report"]))
    diagnostics["n_rows"] = len(rows)
    return rows, finalize_diagnostics(diagnostics)


def build_cells(
    cells_root: Path,
    suites: list[str],
    profiles_dir: Path,
    machines_file: Path,
    subset_glob: str,
) -> list[dict[str, Any]]:
    rows, _ = build_cell_dataset(cells_root, suites, profiles_dir, machines_file, subset_glob)
    return rows


def normalize_columns(rows: list[dict[str, Any]]) -> None:
    """Make every row carry the same keys so table builders can derive columns
    once and render absent source data as null instead of missing."""
    keys = sorted({key for row in rows for key in row})
    for row in rows:
        for key in keys:
            row.setdefault(key, None)


def median_or_none(values: list[float | int | None]) -> float | int | None:
    present = [value for value in values if value is not None]
    return median(present) if present else None


def build_task_combinations(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    grouped: dict[tuple[Any, ...], list[dict[str, Any]]] = defaultdict(list)
    for row in rows:
        key = tuple(row.get(dim) for dim in AGGREGATE_DIMENSIONS)
        grouped[key].append(row)

    out: list[dict[str, Any]] = []
    for key, cells in grouped.items():
        base = {dim: key[i] for i, dim in enumerate(AGGREGATE_DIMENSIONS)}
        n_attempts = len(cells)
        n_pass = sum(1 for row in cells if row.get("result_state") == RESULT_STATE_PASSED)
        n_fail = sum(1 for row in cells if row.get("result_state") == RESULT_STATE_FAILED)
        n_timeout = sum(1 for row in cells if row.get("result_state") == RESULT_STATE_TIMEOUT)
        n_graded = n_pass + n_fail
        n_ungraded = n_timeout
        invalid_classes: dict[str, int] = defaultdict(int)
        final_statuses: dict[str, int] = defaultdict(int)
        process_outcomes: dict[str, int] = defaultdict(int)
        grading_outcomes: dict[str, int] = defaultdict(int)
        result_states: dict[str, int] = defaultdict(int)
        for row in cells:
            if row.get("result_state"):
                result_states[str(row["result_state"])] += 1
            if row.get("invalid_class"):
                invalid_classes[str(row["invalid_class"])] += 1
            if row.get("final_status"):
                final_statuses[str(row["final_status"])] += 1
            if row.get("process_outcome"):
                process_outcomes[str(row["process_outcome"])] += 1
            if row.get("grading_outcome"):
                grading_outcomes[str(row["grading_outcome"])] += 1
        total_cost = sum(row.get("cost_usd") or 0 for row in cells)
        base.update({
            "aggregate_id": hashlib.sha256(json.dumps(base, sort_keys=True, default=str).encode("utf-8")).hexdigest()[:24],
            "schema_version": 1,
            "profile_ids": sorted({row["profile_id"] for row in cells if row.get("profile_id")}),
            "profile_resolved_ids": sorted({row.get("profile_resolved_id") or row["profile_id"] for row in cells if row.get("profile_id")}),
            "internal_lane_ids": sorted({row["internal_lane_id"] for row in cells if row.get("internal_lane_id")}),
            "n_attempts": n_attempts,
            "n_graded": n_graded,
            "n_pass": n_pass,
            "n_fail": n_fail,
            "n_timeout": n_timeout,
            "n_ungraded": n_ungraded,
            "any_pass": n_pass > 0,
            "pass_rate": (n_pass / n_graded) if n_graded else None,
            "n_real": sum(1 for row in cells if (row.get("turns") or 0) > 0 and ((row.get("input_tokens") or 0) + (row.get("output_tokens") or 0)) > 0),
            "n_invalid": sum(invalid_classes.values()),
            "invalid_classes": dict(sorted(invalid_classes.items())),
            "final_statuses": dict(sorted(final_statuses.items())),
            "process_outcomes": dict(sorted(process_outcomes.items())),
            "grading_outcomes": dict(sorted(grading_outcomes.items())),
            "result_states": dict(sorted(result_states.items())),
            "median_wall_seconds": median_or_none([row.get("wall_seconds") for row in cells]),
            "min_wall_seconds": min([row["wall_seconds"] for row in cells if row.get("wall_seconds") is not None] or [None]),
            "max_wall_seconds": max([row["wall_seconds"] for row in cells if row.get("wall_seconds") is not None] or [None]),
            "median_turns": median_or_none([row.get("turns") for row in cells]),
            "median_input_tokens": median_or_none([row.get("input_tokens") for row in cells]),
            "median_output_tokens": median_or_none([row.get("output_tokens") for row in cells]),
            "median_cached_input_tokens": median_or_none([row.get("cached_input_tokens") for row in cells]),
            "median_retried_input_tokens": median_or_none([row.get("retried_input_tokens") for row in cells]),
            "median_reasoning_tokens": median_or_none([row.get("reasoning_tokens") for row in cells]),
            "median_total_tokens": median_or_none([row.get("total_tokens") for row in cells]),
            "total_input_tokens": sum(row.get("input_tokens") or 0 for row in cells),
            "total_output_tokens": sum(row.get("output_tokens") or 0 for row in cells),
            "total_cached_input_tokens": sum(row.get("cached_input_tokens") or 0 for row in cells),
            "total_retried_input_tokens": sum(row.get("retried_input_tokens") or 0 for row in cells),
            "total_reasoning_tokens": sum(row.get("reasoning_tokens") or 0 for row in cells),
            "total_tokens": sum(row.get("total_tokens") or 0 for row in cells),
            "total_cost_usd": total_cost,
            "median_cost_usd": median_or_none([row.get("cost_usd") for row in cells]),
            "avg_cost_usd": total_cost / n_attempts if n_attempts else None,
            "cost_per_pass_usd": total_cost / n_pass if n_pass else None,
            "first_started_at": min([row["started_at"] for row in cells if row.get("started_at")] or [None]),
            "last_finished_at": max([row["finished_at"] for row in cells if row.get("finished_at")] or [None]),
        })
        out.append(base)
    out.sort(key=lambda r: (r["suite"], r["task"], r["lane_label"], r["aggregate_id"]))
    return out


def feed(source: dict[str, Any], rows: list[dict[str, Any]], generated_at: str) -> dict[str, Any]:
    return {
        "schema_version": 1,
        "generated_at": generated_at,
        "source": source,
        "rows": rows,
    }


def write_json(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n", encoding="utf-8")


def parquet_cell_value(value: Any) -> Any:
    if isinstance(value, (dict, list)):
        return json_text(value)
    if isinstance(value, float):
        return value if math.isfinite(value) else None
    if isinstance(value, (str, int, bool)) or value is None:
        return value
    return public_text(value)


def parquet_logical_type(values: list[Any]) -> str:
    present = [value for value in values if value is not None]
    if not present:
        return PARQUET_TYPE_STRING
    if all(isinstance(value, bool) for value in present):
        return PARQUET_TYPE_BOOL
    if all(isinstance(value, int) and not isinstance(value, bool) for value in present):
        return PARQUET_TYPE_INT64
    if all(isinstance(value, (int, float)) and not isinstance(value, bool) for value in present):
        return PARQUET_TYPE_FLOAT64
    return PARQUET_TYPE_STRING


def parquet_type(logical_type: str) -> Any:
    try:
        import pyarrow as pa
    except ImportError as exc:  # pragma: no cover - exercised by CLI path.
        raise RuntimeError(
            "Parquet output requires pyarrow. Install scripts/website/requirements.txt "
            "or run with --formats json."
        ) from exc

    return {
        PARQUET_TYPE_BOOL: pa.bool_(),
        PARQUET_TYPE_INT64: pa.int64(),
        PARQUET_TYPE_FLOAT64: pa.float64(),
        PARQUET_TYPE_STRING: pa.string(),
    }[logical_type]


def coerce_parquet_values(values: list[Any], logical_type: str) -> list[Any]:
    if logical_type != PARQUET_TYPE_STRING:
        return values
    coerced = []
    for value in values:
        if value is None:
            coerced.append(None)
        elif isinstance(value, str):
            coerced.append(value)
        else:
            coerced.append(json.dumps(value, sort_keys=True, separators=(",", ":")))
    return coerced


def write_parquet(path: Path, rows: list[dict[str, Any]], metadata: dict[str, Any], compression: str) -> None:
    try:
        import pyarrow as pa
        import pyarrow.parquet as pq
    except ImportError as exc:
        raise RuntimeError(
            "Parquet output requires pyarrow. Install with "
            "`python3 -m pip install -r scripts/website/requirements.txt` "
            "or run with `--formats json`."
        ) from exc

    path.parent.mkdir(parents=True, exist_ok=True)
    columns = sorted({key for row in rows for key in row})
    arrays = []
    fields = []
    for column in columns:
        values = [parquet_cell_value(row.get(column)) for row in rows]
        logical_type = PARQUET_COLUMN_TYPES.get(column) or parquet_logical_type(values)
        arrow_type = parquet_type(logical_type)
        arrays.append(pa.array(coerce_parquet_values(values, logical_type), type=arrow_type))
        fields.append(pa.field(column, arrow_type))

    schema = pa.schema(
        fields,
        metadata={
            b"fizeau": json.dumps(public_value(metadata), sort_keys=True, separators=(",", ":")).encode("utf-8"),
        },
    )
    table = pa.Table.from_arrays(arrays, schema=schema)
    pq.write_table(table, path, compression=compression)


def parse_formats(value: str) -> set[str]:
    formats = {part.strip().lower() for part in value.split(",") if part.strip()}
    allowed = {"json", "parquet"}
    unknown = formats - allowed
    if unknown:
        raise argparse.ArgumentTypeError(f"unknown format(s): {', '.join(sorted(unknown))}")
    if not formats:
        raise argparse.ArgumentTypeError("at least one output format is required")
    return formats


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cells-root", type=Path, default=DEFAULT_CELLS_ROOT)
    parser.add_argument("--profiles-dir", type=Path, default=DEFAULT_PROFILES_DIR)
    parser.add_argument("--machines-file", type=Path, default=DEFAULT_MACHINES_FILE)
    parser.add_argument("--subset-glob", default=DEFAULT_SUBSET_GLOB)
    parser.add_argument("--suite", action="append", dest="suites", default=None, help="Suite id below cells root. Repeatable.")
    parser.add_argument("--out-dir", type=Path, default=DEFAULT_OUT_DIR)
    parser.add_argument("--schema", type=Path, default=DEFAULT_SCHEMA)
    parser.add_argument(
        "--formats",
        type=parse_formats,
        default=set(DEFAULT_FORMATS),
        help="Comma-separated outputs to write: json,parquet. Default: json,parquet.",
    )
    parser.add_argument("--parquet-compression", default="zstd", help="Parquet compression codec. Default: zstd.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    suites = args.suites or [DEFAULT_SUITE]
    generated_at = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    rows, diagnostics = build_cell_dataset(args.cells_root, suites, args.profiles_dir, args.machines_file, args.subset_glob)
    source = {
        "cells_root": relpath(args.cells_root),
        "profiles_dir": relpath(args.profiles_dir),
        "machines_file": relpath(args.machines_file),
        "suites": suites,
        "n_reports": diagnostics["n_reports"],
        "n_rows": diagnostics["n_rows"],
        "n_excluded": diagnostics["n_excluded"],
        "result_state_counts": diagnostics["result_state_counts"],
        "excluded_result_counts": diagnostics["excluded_result_counts"],
        "excluded_invalid_classes": diagnostics["excluded_invalid_classes"],
        "excluded_final_statuses": diagnostics["excluded_final_statuses"],
        "excluded_process_outcomes": diagnostics["excluded_process_outcomes"],
        "excluded_grading_outcomes": diagnostics["excluded_grading_outcomes"],
    }
    aggregates = build_task_combinations(rows)
    artifacts: list[dict[str, Any]] = []
    if "json" in args.formats:
        write_json(args.out_dir / "cells.json", feed(source, rows, generated_at))
        artifacts.append({"path": "cells.json", "kind": "cell_rows", "format": "json", "rows": len(rows)})
        write_json(
            args.out_dir / "task-combinations.json",
            {
                "schema_version": 1,
                "generated_at": generated_at,
                "source": source,
                "group_by": AGGREGATE_DIMENSIONS,
                "rows": aggregates,
            },
        )
        artifacts.append({"path": "task-combinations.json", "kind": "task_combinations", "format": "json", "rows": len(aggregates)})
    if args.schema.is_file():
        args.out_dir.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(args.schema, args.out_dir / "cells.schema.json")
        artifacts.append({"path": "cells.schema.json", "kind": "schema", "format": "json"})
    if "parquet" in args.formats:
        write_parquet(
            args.out_dir / "cells.parquet",
            rows,
            {"schema_version": 1, "generated_at": generated_at, "source": source, "kind": "cell_rows"},
            args.parquet_compression,
        )
        artifacts.append({"path": "cells.parquet", "kind": "cell_rows", "format": "parquet", "rows": len(rows)})
        write_parquet(
            args.out_dir / "task-combinations.parquet",
            aggregates,
            {
                "schema_version": 1,
                "generated_at": generated_at,
                "source": source,
                "kind": "task_combinations",
                "group_by": AGGREGATE_DIMENSIONS,
            },
            args.parquet_compression,
        )
        artifacts.append({"path": "task-combinations.parquet", "kind": "task_combinations", "format": "parquet", "rows": len(aggregates)})
    manifest = {
        "schema_version": 1,
        "generated_at": generated_at,
        "source": source,
        "artifacts": artifacts,
    }
    write_json(args.out_dir / "benchmark-data-manifest.json", manifest)
    for artifact in artifacts:
        if artifact.get("rows") is not None:
            print(f"wrote {artifact['rows']} {artifact['kind']} rows to {relpath(args.out_dir / artifact['path'])}")
        else:
            print(f"wrote {artifact['kind']} to {relpath(args.out_dir / artifact['path'])}")
    print(f"wrote benchmark data manifest to {relpath(args.out_dir / 'benchmark-data-manifest.json')}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
