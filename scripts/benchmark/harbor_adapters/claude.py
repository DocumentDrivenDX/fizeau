"""Harbor BaseInstalledAgent adapter for Claude Code reference baselines."""

from __future__ import annotations

import json
import os
import shlex
from pathlib import Path
from typing import Any

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

_BINARY_TARGET = "/installed-agent/claude"
_OUTPUT_LOG = "/logs/agent/claude.txt"
_HOME_TARBALL_TARGET = "/installed-agent/claude-home.tgz"


def _bench_env(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


class ClaudeAgent(BaseInstalledAgent):
    SUPPORTS_ATIF: bool = False

    def __init__(self, *args: Any, **kwargs: Any):
        kwargs.setdefault("version", "claude-code-reference")
        super().__init__(*args, **kwargs)

    @staticmethod
    def name() -> str:
        return "claude"

    async def install(self, environment: BaseEnvironment) -> None:
        binary_src = Path(_bench_env("HARBOR_CLAUDE_ARTIFACT", "benchmark-results/bin/claude-linux-amd64/claude"))
        if not binary_src.is_file():
            raise FileNotFoundError(
                "Claude Code binary not found. Set HARBOR_CLAUDE_ARTIFACT or prepare "
                "benchmark-results/bin/claude-linux-amd64/claude."
            )
        await self.exec_as_root(
            environment,
            command="mkdir -p /installed-agent /logs/agent /home/agent/.claude",
        )
        await environment.upload_file(binary_src, _BINARY_TARGET)
        await self.exec_as_root(environment, command=f"chmod 755 {shlex.quote(_BINARY_TARGET)}")

        home_tgz = _bench_env("HARBOR_CLAUDE_HOME_TARBALL", "")
        if home_tgz:
            home_src = Path(home_tgz)
            if not home_src.is_file():
                raise FileNotFoundError(f"HARBOR_CLAUDE_HOME_TARBALL not found: {home_src}")
            await environment.upload_file(home_src, _HOME_TARBALL_TARGET)
            await self.exec_as_root(
                environment,
                command=(
                    f"tar -xzf {shlex.quote(_HOME_TARBALL_TARGET)} -C /home/agent && "
                    "chown -R agent:agent /home/agent/.claude 2>/dev/null || true"
                ),
            )

    def _run_env(self, instruction: str) -> dict[str, str]:
        env = {
            "HARBOR_INSTRUCTION": instruction,
            "HOME": "/home/agent",
        }
        api_key = _bench_env("FIZEAU_API_KEY") or _bench_env("ANTHROPIC_API_KEY")
        if api_key:
            env["ANTHROPIC_API_KEY"] = api_key
        return env

    @with_prompt_template
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        del context

        model = _bench_env("FIZEAU_MODEL", "sonnet")
        effort = _bench_env("FIZEAU_REASONING", "")
        effort_args = f"--effort {shlex.quote(effort)} " if effort else ""
        bare = "--bare " if _bench_env("HARBOR_CLAUDE_BARE", "0") == "1" else ""
        command = (
            "set -euo pipefail; "
            "cd /testbed 2>/dev/null || cd /workspace 2>/dev/null || true; "
            f"{shlex.quote(_BINARY_TARGET)} {bare}--print -p --verbose "
            "--output-format stream-json --permission-mode bypassPermissions "
            f"--dangerously-skip-permissions --model {shlex.quote(model)} {effort_args}"
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
            usage = obj.get("usage") if isinstance(obj, dict) else None
            if not isinstance(usage, dict):
                msg = obj.get("message") if isinstance(obj, dict) else None
                usage = msg.get("usage") if isinstance(msg, dict) else None
            if isinstance(usage, dict):
                input_tokens += usage.get("input_tokens", 0) or 0
                output_tokens += usage.get("output_tokens", 0) or 0
        context.n_input_tokens = input_tokens
        context.n_output_tokens = output_tokens
