"""ddx-agent adapter command builder for the TerminalBench matrix."""

from __future__ import annotations

from typing import Any

from .base import (
    BaseAdapter,
    BenchmarkProfile,
    CommandSpec,
    InstallSpec,
    empty_telemetry,
    jsonl_objects,
    redact_values,
)


class Agent(BaseAdapter):
    name = "ddx-agent"

    def install(self) -> InstallSpec:
        return InstallSpec(["install", "-m", "0755", "${HARBOR_AGENT_ARTIFACT}", "/installed-agent/ddx-agent"])

    def apply_profile(self, profile: BenchmarkProfile) -> CommandSpec:
        env = {
            "DDX_BENCH_PROVIDER_TYPE": profile.provider.type,
            "DDX_BENCH_PROVIDER_MODEL": profile.provider.model,
            "DDX_BENCH_PROVIDER_BASE_URL": profile.provider.base_url,
            "DDX_BENCH_PROVIDER_API_KEY_ENV": profile.provider.api_key_env,
        }
        if profile.sampling.reasoning:
            env["DDX_BENCH_PROVIDER_REASONING"] = profile.sampling.reasoning
        return CommandSpec(argv=[], env=env, stdin=None)

    def command(self, profile: BenchmarkProfile, prompt: str, workdir: str | None = None) -> CommandSpec:
        applied = self.apply_profile(profile)
        argv = [
            "/installed-agent/ddx-agent",
            "--json",
            "--preset",
            "benchmark",
            "--work-dir",
            workdir or ".",
            "-p",
            prompt,
        ]
        return CommandSpec(argv=argv, env=applied.env, stdin="", cwd=workdir)

    def parse_telemetry(self, stream: str) -> dict[str, Any]:
        telemetry = empty_telemetry()
        for event in jsonl_objects(stream):
            if event.get("type") not in {"session.end", "final"}:
                continue
            data = event.get("data")
            if isinstance(data, dict):
                usage = data.get("usage") or {}
                telemetry["input_tokens"] = usage.get("input") or usage.get("input_tokens")
                telemetry["output_tokens"] = usage.get("output") or usage.get("output_tokens")
                if data.get("status") == "failed":
                    telemetry["process_outcome"] = "harness_crash"
        telemetry["wall_seconds"] = 0.0
        return telemetry

    def redact_secrets(self, text: str, env: dict[str, str], profile: BenchmarkProfile | None = None) -> str:
        values = list(env.values())
        if profile is not None:
            values.extend([
                env.get(profile.provider.api_key_env, ""),
                profile.provider.base_url,
            ])
        return redact_values(text, values)
