"""Calibration adapter that hard-codes the TerminalBench hello-world task."""

from __future__ import annotations

from typing import Any

from .base import BaseAdapter, BenchmarkProfile, CommandSpec, InstallSpec, empty_telemetry


class Agent(BaseAdapter):
    name = "dumb_script"

    def install(self) -> InstallSpec:
        return InstallSpec(["true"])

    def apply_profile(self, profile: BenchmarkProfile) -> CommandSpec:
        return CommandSpec(argv=[], env={}, stdin=None)

    def command(self, profile: BenchmarkProfile, prompt: str, workdir: str | None = None) -> CommandSpec:
        script = "printf 'hello world\\n' > hello.txt"
        return CommandSpec(argv=["sh", "-c", script], cwd=workdir, stdin="")

    def parse_telemetry(self, stream: str) -> dict[str, Any]:
        telemetry = empty_telemetry()
        telemetry["reward"] = 1 if "hello-world" in stream else 0
        telemetry["wall_seconds"] = 0.0
        return telemetry
