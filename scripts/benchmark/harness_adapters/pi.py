"""Pi harness adapter for the TerminalBench matrix."""

from __future__ import annotations

import time
from typing import Any

from .base import (
    BaseAdapter,
    BenchmarkProfile,
    CommandSpec,
    InstallSpec,
    empty_telemetry,
    jsonl_objects,
    usage_from_event,
)


PI_VERSION = "0.67.1"


class Agent(BaseAdapter):
    name = "pi"

    def install(self) -> InstallSpec:
        return InstallSpec(["npm", "install", "-g", f"@mariozechner/pi-coding-agent@{PI_VERSION}"])

    def apply_profile(self, profile: BenchmarkProfile) -> CommandSpec:
        env: dict[str, str] = {}
        args: list[str] = []
        notes: list[str] = []

        provider = profile.provider.type
        if provider == "openai-compat":
            provider = "openai-compat"
            env["PI_OPENAI_COMPAT_BASE_URL"] = profile.provider.base_url
            env["PI_OPENAI_COMPAT_API_KEY"] = "${" + profile.provider.api_key_env + "}"
        elif provider == "openai":
            provider = "openai"
            env["OPENAI_API_KEY"] = "${" + profile.provider.api_key_env + "}"
        elif provider == "anthropic":
            provider = "anthropic"
            env["ANTHROPIC_API_KEY"] = "${" + profile.provider.api_key_env + "}"
        elif provider == "google":
            provider = "google"
            env["GEMINI_API_KEY"] = "${" + profile.provider.api_key_env + "}"

        args.extend(["--provider", provider, "--model", profile.provider.model])
        if profile.sampling.reasoning:
            args.extend(["--thinking", profile.sampling.reasoning])
        if profile.sampling.temperature is not None:
            notes.append("pi does not expose sampling.temperature as a CLI flag")
        if profile.limits.max_output_tokens:
            notes.append("pi does not expose limits.max_output_tokens as a CLI flag")

        return CommandSpec(argv=args, env=env, stdin=None, notes=notes)

    def command(self, profile: BenchmarkProfile, prompt: str, workdir: str | None = None) -> CommandSpec:
        applied = self.apply_profile(profile)
        argv = [
            "pi",
            "--mode",
            "json",
            "--print",
            "--no-session",
            "--no-extensions",
            "--no-skills",
            "--no-prompt-templates",
            "--no-themes",
            *applied.argv,
            prompt,
        ]
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


__all__ = ["Agent", "PI_VERSION"]
