#!/usr/bin/env python3
"""Tests for the benchmark microsite datatable generator."""

from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

import yaml


GENERATOR_PATH = Path(__file__).with_name("build-benchmark-data.py")
SPEC = importlib.util.spec_from_file_location("build_benchmark_data", GENERATOR_PATH)
assert SPEC is not None
build_benchmark_data = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(build_benchmark_data)


class BenchmarkDataBuilderTests(unittest.TestCase):
    def test_cells_join_profile_machine_and_runtime_props(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            cells = root / "cells"
            profiles = root / "profiles"
            cells.mkdir()
            profiles.mkdir()

            machines = root / "machines.yaml"
            machines.write_text(
                yaml.safe_dump(
                    {
                        "hardware_profiles": {
                            "nvidia-rtx-3090-ti-desktop": {
                                "chip": "nvidia-rtx-3090-ti",
                                "chip_family": "nvidia-cuda",
                                "memory_gb": 96,
                                "memory_type": "system+vram",
                                "vram_gb": 24,
                                "tdp_watts_spec": 450,
                            }
                        },
                        "machines": {
                            "sindri": {
                                "label": "Sindri",
                                "hardware_profile": "nvidia-rtx-3090-ti-desktop",
                                "tdp_watts_configured": 225,
                                "hardware": {
                                    "cpu_model": "AMD Ryzen",
                                    "memory_gb": 96,
                                    "gpu_vendor": "NVIDIA",
                                    "gpu_model": "NVIDIA RTX 3090 Ti",
                                    "gpu_vram_mb": 24576,
                                },
                            }
                        },
                    }
                ),
                encoding="utf-8",
            )
            (profiles / "sindri-llamacpp.yaml").write_text(
                yaml.safe_dump(
                    {
                        "id": "sindri-llamacpp",
                        "metadata": {
                            "model_family": "qwen3-6-27b",
                            "model_id": "Qwen3.6-27B-UD-Q3_K_XL.gguf",
                            "quant_label": "gguf-q3-k-xl-unsloth",
                            "provider_surface": "sindri-llamacpp",
                            "runtime": "llama-server",
                            "server": "sindri",
                        },
                        "provider": {"type": "llama-server", "model": "Qwen3.6-27B-UD-Q3_K_XL.gguf"},
                        "limits": {"max_output_tokens": 65536, "context_tokens": 180000},
                        "sampling": {"temperature": 0.6, "top_p": 0.95, "top_k": 20, "reasoning": "low"},
                    }
                ),
                encoding="utf-8",
            )
            report_dir = cells / "terminal-bench-2-1" / "bench-0" / "sindri-llamacpp" / "rep-001"
            report_dir.mkdir(parents=True)
            (report_dir / "report.json").write_text(
                json.dumps(
                    {
                        "dataset": "terminal-bench/terminal-bench-2-1",
                        "dataset_version": "2.1",
                        "harness": "fiz",
                        "profile_id": "sindri-llamacpp",
                        "rep": 1,
                        "task_id": "bench-0",
                        "category": "software-engineering",
                        "difficulty": "hard",
                        "adapter_module": "scripts.benchmark.harness_adapters.fiz",
                        "harbor_agent": "scripts/benchmark/harness_adapters/fiz.py:Agent",
                        "command": ["harbor", "run"],
                        "adapter_translation_notes": ["sample note"],
                        "process_outcome": "completed",
                        "grading_outcome": "graded",
                        "reward": 1,
                        "final_status": "graded_pass",
                        "turns": 3,
                        "cached_input_tokens": 100,
                        "retried_input_tokens": 50,
                        "input_tokens": 1200,
                        "output_tokens": 300,
                        "reasoning_tokens": 42,
                        "wall_seconds": 12.5,
                        "cost_usd": 0.12,
                        "error": "see http://192.168.1.12:1234/private and /Users/erik/private",
                        "sampling_used": {"temperature": 0.6, "top_p": 0.95, "top_k": 20, "reasoning": "low"},
                        "model_server_info": {
                            "loaded_context_length": 131072,
                            "max_context_length": 180000,
                            "quantization": "Q3_K_XL",
                            "source": "http://sindri:8020/api/private",
                        },
                        "runtime_props": {
                            "extractor": "llamacpp-v1",
                            "base_model": "qwen3.6-27b-Q3_K_XL",
                            "model_quant": "Q3_K_XL",
                            "kv_quant": "q8_0",
                            "max_context": 131072,
                            "mtp_enabled": False,
                        },
                    }
                ),
                encoding="utf-8",
            )

            rows = build_benchmark_data.build_cells(cells, ["terminal-bench-2-1"], profiles, machines, "no-subsets-*.yaml")

        self.assertEqual(1, len(rows))
        row = rows[0]
        self.assertEqual("bench-0", row["task"])
        self.assertEqual("bench-0", row["test"])
        self.assertEqual("software-engineering", row["task_category"])
        self.assertEqual("hard", row["task_difficulty"])
        self.assertTrue(row["passed"])
        self.assertTrue(row["grader_passed"])
        self.assertEqual("pass", row["pass_fail"])
        self.assertEqual("passed", row["result_state"])
        self.assertEqual("Qwen3.6 27B", row["model_display_name"])
        self.assertEqual("Q3_K_XL", row["model_quant"])
        self.assertEqual(3.0, row["weight_bits"])
        self.assertEqual("q8_0", row["k_quant"])
        self.assertEqual("NVIDIA RTX 3090 Ti", row["gpu_model"])
        self.assertEqual(24, row["gpu_ram_gb"])
        self.assertEqual(96, row["hardware_memory_gb"])
        self.assertEqual(225, row["hardware_tdp_watts"])
        self.assertIn("NVIDIA RTX 3090 Ti", row["descriptor"])
        self.assertEqual("scripts.benchmark.harness_adapters.fiz", row["adapter_module"])
        self.assertEqual("scripts/benchmark/harness_adapters/fiz.py:Agent", row["harbor_agent"])
        self.assertEqual("harbor run", row["command_string"])
        self.assertEqual(1650, row["tokens_consumed"])
        self.assertEqual(1650, row["total_tokens"])
        self.assertGreater(row["cost_per_1k_tokens"], 0)
        self.assertEqual(1, row["adapter_translation_notes_count"])
        self.assertIn("sample note", row["adapter_translation_notes"])
        self.assertEqual(131072, row["model_server_info_loaded_context_length"])
        self.assertEqual("Q3_K_XL", row["model_server_info_quantization"])
        self.assertNotIn("http://", row["error"])
        self.assertNotIn("/Users/erik", row["raw_report_json"])
        self.assertIn("report_sampling_used_temperature", row)
        self.assertEqual(0.6, row["report_sampling_used_temperature"])

    def test_cells_filter_invalid_and_ungraded_but_keep_timeouts(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            cells = root / "cells"
            profiles = root / "profiles"
            cells.mkdir()
            profiles.mkdir()
            machines = root / "machines.yaml"
            machines.write_text("machines: {}\nhardware_profiles: {}\n", encoding="utf-8")

            reports = [
                (
                    "passing",
                    {
                        "harness": "fiz",
                        "profile_id": "lane",
                        "rep": 1,
                        "task_id": "passing",
                        "process_outcome": "completed",
                        "grading_outcome": "graded",
                        "reward": 1,
                        "final_status": "graded_pass",
                    },
                ),
                (
                    "timeout",
                    {
                        "harness": "fiz",
                        "profile_id": "lane",
                        "rep": 1,
                        "task_id": "timeout",
                        "process_outcome": "completed",
                        "grading_outcome": "ungraded",
                        "final_status": "ran",
                        "terminated_mid_work": True,
                    },
                ),
                (
                    "invalid-graded",
                    {
                        "harness": "fiz",
                        "profile_id": "lane",
                        "rep": 1,
                        "task_id": "invalid-graded",
                        "process_outcome": "completed",
                        "grading_outcome": "graded",
                        "reward": 0,
                        "final_status": "graded_fail",
                        "invalid_class": "invalid_setup",
                    },
                ),
                (
                    "ungraded",
                    {
                        "harness": "fiz",
                        "profile_id": "lane",
                        "rep": 1,
                        "task_id": "ungraded",
                        "process_outcome": "completed",
                        "grading_outcome": "ungraded",
                        "final_status": "ran",
                    },
                ),
            ]
            for task, report in reports:
                report_dir = cells / "terminal-bench-2-1" / task / "lane" / "rep-001"
                report_dir.mkdir(parents=True)
                (report_dir / "report.json").write_text(json.dumps(report), encoding="utf-8")

            rows, diagnostics = build_benchmark_data.build_cell_dataset(
                cells,
                ["terminal-bench-2-1"],
                profiles,
                machines,
                "no-subsets-*.yaml",
            )

        self.assertEqual(4, diagnostics["n_reports"])
        self.assertEqual(2, diagnostics["n_rows"])
        self.assertEqual(2, diagnostics["n_excluded"])
        self.assertEqual({"passed": 1, "timeout": 1}, diagnostics["result_state_counts"])
        self.assertEqual({"invalid:invalid_setup": 1, "ungraded:ran": 1}, diagnostics["excluded_result_counts"])
        self.assertEqual(["passing", "timeout"], [row["task"] for row in rows])
        timeout_row = next(row for row in rows if row["task"] == "timeout")
        self.assertEqual("timeout", timeout_row["result_state"])
        self.assertFalse(timeout_row["passed"])

    def test_task_combinations_preserve_dimensions_for_filtering(self) -> None:
        rows = [
            {
                dim: None for dim in build_benchmark_data.AGGREGATE_DIMENSIONS
            },
            {
                dim: None for dim in build_benchmark_data.AGGREGATE_DIMENSIONS
            },
        ]
        for index, row in enumerate(rows):
            row.update(
                {
                    "suite": "terminal-bench-2-1",
                    "task": "bench-0",
                    "internal_lane_id": "sindri-llamacpp",
                    "profile_id": "sindri-llamacpp",
                    "lane_label": "sindri-llamacpp",
                    "model_display_name": "Qwen3.6 27B",
                    "model_quant": "Q3_K_XL",
                    "engine": "llama-server",
                    "gpu_model": "NVIDIA RTX 3090 Ti",
                    "gpu_ram_gb": 24,
                    "hardware_memory_gb": 96,
                    "grading_outcome": "graded",
                    "result_state": "passed" if index == 0 else "failed",
                    "passed": index == 0,
                    "turns": 2 + index,
                    "input_tokens": 1000 + index,
                    "output_tokens": 200 + index,
                    "wall_seconds": 10 + index,
                    "cost_usd": 0,
                    "started_at": f"2026-05-13T00:00:0{index}Z",
                    "finished_at": f"2026-05-13T00:01:0{index}Z",
                }
            )

        aggregates = build_benchmark_data.build_task_combinations(rows)

        self.assertEqual(1, len(aggregates))
        agg = aggregates[0]
        self.assertEqual("bench-0", agg["task"])
        self.assertEqual("NVIDIA RTX 3090 Ti", agg["gpu_model"])
        self.assertEqual(24, agg["gpu_ram_gb"])
        self.assertTrue(agg["any_pass"])
        self.assertEqual(2, agg["n_attempts"])
        self.assertEqual(1, agg["n_pass"])
        self.assertEqual(0.5, agg["pass_rate"])
        self.assertEqual(1, agg["n_fail"])
        self.assertEqual(0, agg["n_timeout"])
        self.assertEqual(2001, agg["total_input_tokens"])
        self.assertEqual(401, agg["total_output_tokens"])

    def test_task_combinations_split_when_runtime_properties_change(self) -> None:
        rows = []
        for mtp_enabled in [False, True]:
            row = {dim: None for dim in build_benchmark_data.AGGREGATE_DIMENSIONS}
            row.update(
                {
                    "suite": "terminal-bench-2-1",
                    "task": "bench-0",
                    "task_category": "software-engineering",
                    "task_difficulty": "hard",
                    "profile_id": "sindri-vllm",
                    "internal_lane_id": "sindri-vllm",
                    "lane_label": "sindri-vllm",
                    "model_display_name": "Qwen3.6 27B",
                    "model_quant": "int4",
                    "engine": "vllm",
                    "runtime_mtp_enabled": mtp_enabled,
                    "gpu_model": "NVIDIA RTX 3090 Ti",
                    "gpu_ram_gb": 24,
                    "hardware_memory_gb": 96,
                    "grading_outcome": "graded",
                    "result_state": "passed",
                    "passed": True,
                    "turns": 1,
                    "input_tokens": 100,
                    "output_tokens": 10,
                    "total_tokens": 110,
                    "wall_seconds": 10,
                    "cost_usd": 0,
                    "started_at": "2026-05-13T00:00:00Z",
                    "finished_at": "2026-05-13T00:01:00Z",
                }
            )
            rows.append(row)

        aggregates = build_benchmark_data.build_task_combinations(rows)

        self.assertEqual(2, len(aggregates))
        self.assertEqual([False, True], sorted(agg["runtime_mtp_enabled"] for agg in aggregates))

    def test_parquet_values_preserve_queryable_scalar_types(self) -> None:
        self.assertEqual(True, build_benchmark_data.parquet_cell_value(True))
        self.assertEqual(42, build_benchmark_data.parquet_cell_value(42))
        self.assertEqual(3.5, build_benchmark_data.parquet_cell_value(3.5))
        self.assertEqual('["canary"]', build_benchmark_data.parquet_cell_value(["canary"]))
        self.assertEqual('{"a":1}', build_benchmark_data.parquet_cell_value({"a": 1}))

        self.assertEqual(
            build_benchmark_data.PARQUET_TYPE_BOOL,
            build_benchmark_data.parquet_logical_type([True, False, None]),
        )
        self.assertEqual(
            build_benchmark_data.PARQUET_TYPE_INT64,
            build_benchmark_data.parquet_logical_type([1, 2, None]),
        )
        self.assertEqual(
            build_benchmark_data.PARQUET_TYPE_FLOAT64,
            build_benchmark_data.parquet_logical_type([1, 2.5, None]),
        )
        self.assertEqual(
            build_benchmark_data.PARQUET_TYPE_STRING,
            build_benchmark_data.parquet_logical_type(["a", 2, None]),
        )


if __name__ == "__main__":
    unittest.main()
