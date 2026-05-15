"""No-API adapter that emits deterministic token usage for cost guard tests."""

from __future__ import annotations

from typing import Any

from .base import BaseAdapter, BenchmarkProfile, CommandSpec, InstallSpec, empty_telemetry


class Agent(BaseAdapter):
    name = "cost_probe"

    def install(self) -> InstallSpec:
        return InstallSpec(["true"])

    def apply_profile(self, profile: BenchmarkProfile) -> CommandSpec:
        return CommandSpec(argv=[], env={}, stdin=None)

    def command(self, profile: BenchmarkProfile, prompt: str, workdir: str | None = None) -> CommandSpec:
        return CommandSpec(argv=["sh", "-c", "true"], cwd=workdir, stdin="")

    def parse_telemetry(self, stream: str) -> dict[str, Any]:
        telemetry = empty_telemetry()
        telemetry["grading_outcome"] = "graded"
        telemetry["reward"] = 1
        telemetry["input_tokens"] = 1_000_000
        telemetry["output_tokens"] = 1_000_000
        telemetry["cached_input_tokens"] = 100_000
        telemetry["retried_input_tokens"] = 50_000
        telemetry["wall_seconds"] = 0.0
        return telemetry
