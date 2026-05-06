"""Harbor BaseInstalledAgent adapter for Codex reference baselines."""

from __future__ import annotations

import json
import os
import shlex
from pathlib import Path
from typing import Any

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

_BINARY_TARGET = "/installed-agent/codex"
_OUTPUT_LOG = "/logs/agent/codex.txt"
_CODEX_HOME = "/home/agent/.codex"
_HOME_TARBALL_TARGET = "/installed-agent/codex-home.tgz"


def _bench_env(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


class CodexAgent(BaseInstalledAgent):
    SUPPORTS_ATIF: bool = False

    def __init__(self, *args: Any, **kwargs: Any):
        kwargs.setdefault("version", "codex-reference")
        super().__init__(*args, **kwargs)

    @staticmethod
    def name() -> str:
        return "codex"

    async def install(self, environment: BaseEnvironment) -> None:
        binary_src = Path(_bench_env("HARBOR_CODEX_ARTIFACT", "benchmark-results/bin/codex-linux-amd64/codex"))
        if not binary_src.is_file():
            raise FileNotFoundError(
                "Codex binary not found. Set HARBOR_CODEX_ARTIFACT or prepare "
                "benchmark-results/bin/codex-linux-amd64/codex."
            )
        await self.exec_as_root(
            environment,
            command=f"mkdir -p /installed-agent /logs/agent {shlex.quote(_CODEX_HOME)}",
        )
        await environment.upload_file(binary_src, _BINARY_TARGET)
        await self.exec_as_root(environment, command=f"chmod 755 {shlex.quote(_BINARY_TARGET)}")

        home_tgz = _bench_env("HARBOR_CODEX_HOME_TARBALL", "")
        if home_tgz:
            home_src = Path(home_tgz)
            if not home_src.is_file():
                raise FileNotFoundError(f"HARBOR_CODEX_HOME_TARBALL not found: {home_src}")
            await environment.upload_file(home_src, _HOME_TARBALL_TARGET)
            await self.exec_as_root(
                environment,
                command=(
                    f"tar -xzf {shlex.quote(_HOME_TARBALL_TARGET)} -C /home/agent && "
                    f"chown -R agent:agent {shlex.quote(_CODEX_HOME)} 2>/dev/null || true"
                ),
            )

    def _run_env(self, instruction: str) -> dict[str, str]:
        env = {
            "HARBOR_INSTRUCTION": instruction,
            "HOME": "/home/agent",
            "CODEX_HOME": _CODEX_HOME,
        }
        api_key = _bench_env("FIZEAU_API_KEY") or _bench_env("OPENAI_API_KEY")
        if api_key:
            env["OPENAI_API_KEY"] = api_key
        return env

    @with_prompt_template
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        del context

        model = _bench_env("FIZEAU_MODEL", "gpt-5.5-mini")
        reasoning = _bench_env("FIZEAU_REASONING", "")
        reasoning_args = f"-c reasoning.effort={shlex.quote(reasoning)} " if reasoning else ""
        command = (
            "set -euo pipefail; "
            "cd /testbed 2>/dev/null || cd /workspace 2>/dev/null || true; "
            "run_dir=\"$PWD\"; "
            f"{shlex.quote(_BINARY_TARGET)} exec --json --ephemeral --skip-git-repo-check "
            "--ignore-rules --dangerously-bypass-approvals-and-sandbox "
            f"-C \"$run_dir\" -m {shlex.quote(model)} {reasoning_args}"
            '"$HARBOR_INSTRUCTION" '
            f"2>&1 | stdbuf -oL tee {shlex.quote(_OUTPUT_LOG)}"
        )
        await self.exec_as_agent(environment, command=command, env=self._run_env(instruction))

    def populate_context_post_run(self, context: AgentContext) -> None:
        log_path = Path(_OUTPUT_LOG)
        if not log_path.exists():
            return
        input_tokens = 0
        output_tokens = 0
        for line in log_path.read_text(encoding="utf-8", errors="replace").splitlines():
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            payload = obj.get("payload") if isinstance(obj, dict) else None
            usage = None
            if isinstance(payload, dict) and payload.get("type") == "token_count":
                info = payload.get("info")
                if isinstance(info, dict):
                    usage = info.get("last_token_usage")
            if isinstance(usage, dict):
                input_tokens += usage.get("input_tokens", 0) or 0
                output_tokens += usage.get("output_tokens", 0) or 0
        context.n_input_tokens = input_tokens
        context.n_output_tokens = output_tokens
