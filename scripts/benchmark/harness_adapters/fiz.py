"""fiz adapter command builder for the TerminalBench matrix."""

from __future__ import annotations

import os
import shutil
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
    name = "fiz"

    def install(self) -> InstallSpec:
        return InstallSpec(["install", "-m", "0755", "${HARBOR_AGENT_ARTIFACT}", "/installed-agent/fiz"])

    def apply_profile(self, profile: BenchmarkProfile) -> CommandSpec:
        env: dict[str, str] = {
            "FIZEAU_BASE_URL": profile.provider.base_url,
            "FIZEAU_MODEL": profile.provider.model,
            "FIZEAU_API_KEY": os.environ.get(profile.provider.api_key_env, ""),
        }
        if "openrouter" in profile.provider.base_url:
            env["FIZEAU_PROVIDER"] = "openrouter"
        elif profile.provider.type not in ("openai-compat",):
            env["FIZEAU_PROVIDER"] = profile.provider.type
        # Forward sampling params so fiz reads them via FIZEAU_* env overrides.
        sampling = getattr(profile, "sampling", None) or {}
        if isinstance(sampling, dict):
            if (t := sampling.get("temperature")) is not None:
                env["FIZEAU_TEMPERATURE"] = str(t)
            if (v := sampling.get("top_p")) is not None:
                env["FIZEAU_TOP_P"] = str(v)
            if (v := sampling.get("top_k")) is not None:
                env["FIZEAU_TOP_K"] = str(int(v))
            if (v := sampling.get("min_p")) is not None:
                env["FIZEAU_MIN_P"] = str(v)
        return CommandSpec(argv=[], env=env, stdin=None)

    def command(self, profile: BenchmarkProfile, prompt: str, workdir: str | None = None) -> CommandSpec:
        applied = self.apply_profile(profile)
        binary = shutil.which("fiz") or "/installed-agent/fiz"
        argv = [
            binary,
            "--json",
            "--preset",
            "default",
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
