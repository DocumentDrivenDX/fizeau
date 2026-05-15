"""Shared adapter primitives for the TerminalBench harness matrix.

The matrix runner imports one adapter module per harness. Each adapter exposes
an ``Agent`` object with install, command, apply_profile, parse_telemetry, and
redact_secrets methods. The helpers here keep that contract small and testable
without invoking paid APIs or live harness binaries.
"""

from __future__ import annotations

from dataclasses import dataclass, field
import json
from typing import Any


@dataclass(frozen=True)
class ProviderProfile:
    type: str
    model: str
    base_url: str
    api_key_env: str


@dataclass(frozen=True)
class SamplingProfile:
    temperature: float | None = None
    reasoning: str = ""


@dataclass(frozen=True)
class LimitsProfile:
    max_output_tokens: int | None = None
    context_tokens: int | None = None


@dataclass(frozen=True)
class BenchmarkProfile:
    id: str
    provider: ProviderProfile
    sampling: SamplingProfile = field(default_factory=SamplingProfile)
    limits: LimitsProfile = field(default_factory=LimitsProfile)

    @classmethod
    def from_mapping(cls, raw: dict[str, Any]) -> "BenchmarkProfile":
        provider = raw.get("provider") or {}
        sampling = raw.get("sampling") or {}
        limits = raw.get("limits") or {}
        return cls(
            id=str(raw.get("id") or ""),
            provider=ProviderProfile(
                type=str(provider.get("type") or ""),
                model=str(provider.get("model") or ""),
                base_url=str(provider.get("base_url") or ""),
                api_key_env=str(provider.get("api_key_env") or ""),
            ),
            sampling=SamplingProfile(
                temperature=sampling.get("temperature"),
                reasoning=str(sampling.get("reasoning") or ""),
            ),
            limits=LimitsProfile(
                max_output_tokens=limits.get("max_output_tokens"),
                context_tokens=limits.get("context_tokens"),
            ),
        )


@dataclass(frozen=True)
class InstallSpec:
    argv: list[str]


@dataclass(frozen=True)
class CommandSpec:
    argv: list[str]
    env: dict[str, str] = field(default_factory=dict)
    stdin: str | None = None
    cwd: str | None = None
    notes: list[str] = field(default_factory=list)


def redact_values(text: str, values: list[str]) -> str:
    out = text
    for value in values:
        if value:
            out = out.replace(value, "[REDACTED]")
    return out


def empty_telemetry() -> dict[str, Any]:
    return {
        "process_outcome": "completed",
        "grading_outcome": "ungraded",
        "reward": None,
        "turns": None,
        "tool_calls": None,
        "tool_call_errors": None,
        "input_tokens": None,
        "output_tokens": None,
        "cached_input_tokens": None,
        "retried_input_tokens": None,
        "wall_seconds": None,
    }


def jsonl_objects(stream: str) -> list[dict[str, Any]]:
    objects: list[dict[str, Any]] = []
    for line in stream.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            parsed = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(parsed, dict):
            objects.append(parsed)
    return objects


def usage_from_event(event: dict[str, Any]) -> dict[str, int | None]:
    usage = event.get("usage")
    if not isinstance(usage, dict):
        message = event.get("message")
        if isinstance(message, dict):
            usage = message.get("usage")
    if not isinstance(usage, dict):
        partial = event.get("partial")
        if isinstance(partial, dict):
            usage = partial.get("usage")
    if not isinstance(usage, dict):
        return {"input_tokens": None, "output_tokens": None}
    return {
        "input_tokens": usage.get("input_tokens") or usage.get("prompt_tokens"),
        "output_tokens": usage.get("output_tokens") or usage.get("completion_tokens"),
    }


class BaseAdapter:
    name = ""

    def install(self) -> InstallSpec:
        raise NotImplementedError

    def apply_profile(self, profile: BenchmarkProfile) -> CommandSpec:
        raise NotImplementedError

    def command(self, profile: BenchmarkProfile, prompt: str, workdir: str | None = None) -> CommandSpec:
        raise NotImplementedError

    def parse_telemetry(self, stream: str) -> dict[str, Any]:
        raise NotImplementedError

    def redact_secrets(self, text: str, env: dict[str, str], profile: BenchmarkProfile | None = None) -> str:
        values = list(env.values())
        if profile is not None:
            values.append(env.get(profile.provider.api_key_env, ""))
        return redact_values(text, values)
