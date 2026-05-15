"""opencode harness adapter for the TerminalBench matrix."""

from __future__ import annotations

import json
import os
import tempfile
import time
from typing import Any

from .base import (
    BaseAdapter,
    BenchmarkProfile,
    CommandSpec,
    InstallSpec,
    empty_telemetry,
    jsonl_objects,
    redact_values,
    usage_from_event,
)


OPENCODE_VERSION = "1.3.17"


class Agent(BaseAdapter):
    name = "opencode"

    def install(self) -> InstallSpec:
        return InstallSpec([
            "sh",
            "-c",
            f"curl -fsSL https://opencode.ai/install | VERSION={OPENCODE_VERSION} bash",
        ])

    def apply_profile(self, profile: BenchmarkProfile) -> CommandSpec:
        provider_id = provider_id_for(profile)
        provider_model = f"{provider_id}/{profile.provider.model}"
        tmp = tempfile.mkdtemp(prefix="opencode-bench-")
        config = provider_config(provider_id, profile)
        env = {
            "OPENCODE_DISABLE_AUTOUPDATE": "1",
            "OPENCODE_CONFIG_DIR": f"{tmp}/opencode-config",
            "OPENCODE_DATA_DIR": f"{tmp}/opencode-data",
            profile.provider.api_key_env: os.environ.get(profile.provider.api_key_env, ""),
            "OPENCODE_CONFIG_CONTENT": json.dumps(config, sort_keys=True, separators=(",", ":")),
        }
        args = ["-m", provider_model]
        notes: list[str] = []
        if profile.sampling.reasoning:
            args.extend(["--variant", profile.sampling.reasoning])
        if profile.sampling.temperature is not None:
            notes.append("opencode sampling.temperature is represented in generated opencode.json")
        if profile.limits.max_output_tokens:
            notes.append("opencode limits.max_output_tokens is represented in generated opencode.json")
        return CommandSpec(argv=args, env=env, notes=notes)

    def command(self, profile: BenchmarkProfile, prompt: str, workdir: str | None = None) -> CommandSpec:
        applied = self.apply_profile(profile)
        argv = [
            "opencode",
            "run",
            "--format",
            "json",
            "--pure",
            "--port",
            "0",
        ]
        if workdir:
            argv.extend(["--dir", workdir])
        argv.extend(applied.argv)
        argv.append("--")
        argv.append(prompt)
        return CommandSpec(argv=argv, env=applied.env, stdin="", cwd=workdir, notes=applied.notes)

    def parse_telemetry(self, stream: str) -> dict[str, Any]:
        started = time.monotonic()
        telemetry = empty_telemetry()
        tool_calls = 0
        input_tokens = None
        output_tokens = None

        for event in jsonl_objects(stream):
            event_type = str(event.get("type") or "")
            if "tool" in event_type and ("call" in event_type or "start" in event_type):
                tool_calls += 1
            usage = usage_from_event(event)
            if usage["input_tokens"] is not None:
                input_tokens = usage["input_tokens"]
            if usage["output_tokens"] is not None:
                output_tokens = usage["output_tokens"]
            if event.get("error"):
                telemetry["process_outcome"] = "harness_crash"

        telemetry["tool_calls"] = tool_calls
        telemetry["input_tokens"] = input_tokens
        telemetry["output_tokens"] = output_tokens
        telemetry["wall_seconds"] = max(time.monotonic() - started, 0.0)
        return telemetry

    def redact_secrets(self, text: str, env: dict[str, str], profile: BenchmarkProfile | None = None) -> str:
        values = list(env.values())
        if profile is not None:
            values.extend([
                env.get(profile.provider.api_key_env, ""),
                profile.provider.base_url,
            ])
        return redact_values(text, values)


def provider_id_for(profile: BenchmarkProfile) -> str:
    if profile.provider.type == "openai-compat":
        if "openrouter" in profile.provider.base_url:
            return "openrouter"
        return "openai-compat"
    return profile.provider.type


def provider_config(provider_id: str, profile: BenchmarkProfile) -> dict[str, Any]:
    options: dict[str, Any] = {
        "baseURL": profile.provider.base_url,
        "apiKey": "{env:" + profile.provider.api_key_env + "}",
    }
    if profile.sampling.temperature is not None:
        options["temperature"] = profile.sampling.temperature
    if profile.limits.max_output_tokens:
        options["maxTokens"] = profile.limits.max_output_tokens
    return {
        "provider": {
            provider_id: {
                "npm": "@ai-sdk/openai-compatible",
                "name": provider_id,
                "options": options,
                "models": {
                    profile.provider.model: {
                        "limit": {
                            "context": profile.limits.context_tokens or 128000,
                            "output": profile.limits.max_output_tokens or 32768,
                        },
                    },
                },
            },
        },
    }


__all__ = ["Agent", "OPENCODE_VERSION", "provider_config"]
