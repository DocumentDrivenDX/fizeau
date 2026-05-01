"""Harbor BaseInstalledAgent adapter for pi (aider-based coding agent)."""

from __future__ import annotations

import os
from pathlib import Path
from typing import Any

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

_OUTPUT_LOG = "/logs/agent/pi.txt"


def _bench_env(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


class PiAgent(BaseInstalledAgent):
    SUPPORTS_ATIF: bool = False

    def __init__(self, *args: Any, **kwargs: Any):
        kwargs.setdefault("version", "pi-benchmark")
        super().__init__(*args, **kwargs)

    @staticmethod
    def name() -> str:
        return "pi"

    async def install(self, environment: BaseEnvironment) -> None:
        await self.exec_as_root(
            environment,
            command="npm install -g @openinterpreter/pi 2>&1 || pip install open-interpreter 2>&1 || true",
        )

    def _run_env(self, instruction: str) -> dict[str, str]:
        env: dict[str, str] = {"HARBOR_INSTRUCTION": instruction}
        base_url = _bench_env("FIZEAU_BASE_URL", "")
        model = _bench_env("FIZEAU_MODEL", "")
        api_key = _bench_env("FIZEAU_API_KEY", "")
        if base_url:
            env["OPENAI_BASE_URL"] = base_url
        if model:
            env["OPENAI_MODEL"] = model
        if api_key:
            env["OPENAI_API_KEY"] = api_key
            env["OPENROUTER_API_KEY"] = api_key
        return env

    @with_prompt_template
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        del context

        pi_bin = "pi"
        command = (
            "set -euo pipefail; "
            "cd /testbed 2>/dev/null || cd /workspace 2>/dev/null || true; "
            f'{pi_bin} "$HARBOR_INSTRUCTION" '
            f'2>&1 | stdbuf -oL tee {_OUTPUT_LOG}'
        )

        await self.exec_as_agent(
            environment,
            command=command,
            env=self._run_env(instruction),
        )

    def populate_context_post_run(self, context: AgentContext) -> None:
        log_path = Path(_OUTPUT_LOG)
        if not log_path.exists():
            return
        input_tokens = 0
        output_tokens = 0
        import json
        for line in log_path.read_text(encoding="utf-8", errors="replace").splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
                usage = obj.get("usage") or {}
                input_tokens += usage.get("prompt_tokens", 0) or 0
                output_tokens += usage.get("completion_tokens", 0) or 0
            except (json.JSONDecodeError, AttributeError):
                pass
        context.n_input_tokens = input_tokens
        context.n_output_tokens = output_tokens
