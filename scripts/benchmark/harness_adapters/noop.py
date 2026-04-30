"""Calibration adapter that intentionally does nothing."""

from __future__ import annotations

from typing import Any

from .base import BaseAdapter, BenchmarkProfile, CommandSpec, InstallSpec, empty_telemetry


class Agent(BaseAdapter):
    name = "noop"

    def install(self) -> InstallSpec:
        return InstallSpec(["true"])

    def apply_profile(self, profile: BenchmarkProfile) -> CommandSpec:
        return CommandSpec(argv=[], env={}, stdin=None)

    def command(self, profile: BenchmarkProfile, prompt: str, workdir: str | None = None) -> CommandSpec:
        return CommandSpec(argv=["sh", "-c", "true"], cwd=workdir, stdin="")

    def parse_telemetry(self, stream: str) -> dict[str, Any]:
        telemetry = empty_telemetry()
        telemetry["reward"] = 0
        telemetry["wall_seconds"] = 0.0
        return telemetry
