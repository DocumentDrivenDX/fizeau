#!/usr/bin/env python3
"""Regression tests for the public benchmark report contract."""

from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


GENERATOR_PATH = Path(__file__).with_name("generate-report.py")
SPEC = importlib.util.spec_from_file_location("generate_report", GENERATOR_PATH)
assert SPEC is not None
generate_report = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(generate_report)


def _subset_rollup(tasks_passed: int = 1) -> dict:
    return {
        name: {
            "n_attempts": 2,
            "tasks_attempted": 2,
            "tasks_in_subset": 2,
            "tasks_passed": tasks_passed,
        }
        for name in generate_report.SUBSET_ORDER
    }


def _profile_rollup() -> dict:
    return {
        "n_attempts": 2,
        "n_real": 2,
        "n_graded": 2,
        "n_pass": 1,
        "median_turns": 3,
        "median_in_tok": 12000,
        "median_out_tok": 900,
        "median_wall": 123,
        "avg_cost": 0.0,
    }


class PublicBenchmarkReportTests(unittest.TestCase):
    def test_public_profile_labels_cover_retired_sindri_ids(self) -> None:
        self.assertEqual(
            generate_report.public_profile_label("sindri-club-3090"),
            "sindri-vllm",
        )
        self.assertEqual(
            generate_report.public_profile_label("sindri-club-3090-llamacpp"),
            "sindri-llamacpp",
        )
        self.assertEqual(
            generate_report.public_profile_label("sindri-vllm"),
            "sindri-vllm",
        )
        self.assertEqual(
            generate_report.public_profile_label("sindri-llamacpp"),
            "sindri-llamacpp",
        )

    def test_provider_page_uses_public_labels_without_runner_internals(self) -> None:
        raw_vllm = "sindri-club-3090"
        raw_llamacpp = "sindri-club-3090-llamacpp"
        inventory_key = "sindri-machine-inventory-key"
        profiles = {
            raw_vllm: {
                "id": raw_vllm,
                "metadata": {
                    "model_family": "qwen3.6-27b",
                    "runtime": "vllm",
                    "quant_label": "int4",
                    "server": inventory_key,
                    "source_profile_id": raw_vllm,
                },
                "provider": {
                    "type": "openai_compat",
                    "base_url": "http://10.1.2.3:8000/v1",
                },
            },
            raw_llamacpp: {
                "id": raw_llamacpp,
                "metadata": {
                    "model_family": "qwen3.6-27b",
                    "runtime": "llamacpp",
                    "quant_label": "Q3_K_XL",
                    "server": inventory_key,
                    "source_profile_id": raw_llamacpp,
                },
                "provider": {
                    "type": "openai_compat",
                    "base_url": "http://10.1.2.4:8080/v1",
                },
            },
        }
        machines = {
            inventory_key: {
                "gpu": "NVIDIA RTX 3090",
                "cpu": "AMD Ryzen",
                "os": "Linux",
                "memory": "128 GB",
            }
        }
        subsets = {
            name: {"name": name, "tasks": ["task-one", "task-two"], "selection_rule": ""}
            for name in generate_report.SUBSET_ORDER
        }
        per_profile = {raw_vllm: _profile_rollup(), raw_llamacpp: _profile_rollup()}
        per_subset = {raw_vllm: _subset_rollup(), raw_llamacpp: _subset_rollup()}
        timing = {
            raw_vllm: {"ttft_p50": 1.2, "decode_tps_p50": 42.0, "buckets": []},
            raw_llamacpp: {"ttft_p50": 1.5, "decode_tps_p50": 18.0, "buckets": []},
        }

        original_read_section = generate_report._read_section
        generate_report._read_section = lambda _name: "<p>public narrative</p>"
        try:
            body = generate_report.render_providers_body(
                snapshot_ts="2026-05-12 00:00:00 UTC",
                profiles=profiles,
                machines=machines,
                subsets=subsets,
                per_profile=per_profile,
                per_subset=per_subset,
                ext_per_subset={},
                timing=timing,
                chart_emitter=lambda name: f'<img src="{name}">',
            )
        finally:
            generate_report._read_section = original_read_section

        self.assertIn("sindri-vllm", body)
        self.assertIn("sindri-llamacpp", body)
        for forbidden in [
            raw_vllm,
            raw_llamacpp,
            "http://10.1.2.3:8000/v1",
            "http://10.1.2.4:8080/v1",
            inventory_key,
            "source_profile_id",
        ]:
            self.assertNotIn(forbidden, body)

    def test_hugo_bundle_data_publisher_is_allowlisted(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / "data"
            public = root / "hugo-data"
            source.mkdir()
            public.mkdir()

            snapshot = {"generated_at": "2026-05-12 00:00:00 UTC", "n_reports": 2}
            (source / "snapshot.json").write_text(json.dumps(snapshot), encoding="utf-8")
            for name in [
                "aggregates.json",
                "timing.json",
                "profiles.json",
                "machines.json",
            ]:
                (source / name).write_text(
                    json.dumps(
                        {
                            "source_profile_id": "sindri-club-3090",
                            "base_url": "http://10.1.2.3:8000/v1",
                            "machine": "sindri-machine-inventory-key",
                        }
                    ),
                    encoding="utf-8",
                )
                (public / name).write_text("stale", encoding="utf-8")

            generate_report.publish_hugo_bundle_data(public, source)

            self.assertEqual(["snapshot.json"], sorted(p.name for p in public.glob("*.json")))
            self.assertEqual(snapshot, json.loads((public / "snapshot.json").read_text()))


if __name__ == "__main__":
    unittest.main()
