"""Harbor BaseInstalledAgent adapter for pi (aider-based coding agent)."""

from __future__ import annotations

import os
import json
import shlex
import socket
from pathlib import Path
from typing import Any

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

_OUTPUT_LOG = "/logs/agent/pi.txt"
PI_VERSION = "0.67.1"
_AGENT_DIR = "/installed-agent/pi"
_NODE_DIR = "/installed-agent/node"
_NODE_TARBALL_TARGET = "/installed-agent/node.tgz"
_PI_TARBALL_TARGET = "/installed-agent/pi.tgz"


def _bench_env(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


def _resolve_hosts_for_url(base_url: str) -> dict[str, str]:
    if not base_url:
        return {}
    try:
        from urllib.parse import urlparse

        host = urlparse(base_url).hostname or ""
        if not host or host in ("localhost", "127.0.0.1", "::1"):
            return {}
        try:
            socket.inet_pton(socket.AF_INET, host)
            return {}
        except OSError:
            pass
        ip = socket.gethostbyname(host)
        if ip.startswith("127.") or ip.startswith("169.254."):
            return {}
        return {host: ip}
    except Exception:
        return {}


def _write_models_json_command() -> str:
    return (
        f"mkdir -p {shlex.quote(_AGENT_DIR)}; "
        f"cat > {shlex.quote(_AGENT_DIR)}/models.json <<EOF\n"
        '{"providers":{"openai-compat":{"baseUrl":"${FIZEAU_BASE_URL}",'
        '"apiKey":"FIZEAU_API_KEY","models":[{"id":"${FIZEAU_MODEL}",'
        '"api":"openai-completions","reasoning":true,"contextWindow":128000,'
        '"maxTokens":32768,"compat":{"supportsUsageInStreaming":true,'
        '"maxTokensField":"max_tokens","thinkingFormat":"qwen"}}]}}}\n'
        "EOF\n"
    )


class PiAgent(BaseInstalledAgent):
    SUPPORTS_ATIF: bool = False

    def __init__(self, *args: Any, **kwargs: Any):
        kwargs.setdefault("version", "pi-benchmark")
        super().__init__(*args, **kwargs)

    @staticmethod
    def name() -> str:
        return "pi"

    async def install(self, environment: BaseEnvironment) -> None:
        node_src = Path(os.environ.get("HARBOR_NODE_TARBALL", "benchmark-results/bin/node-v20.19.2-linux-x64.tar.gz"))
        pi_src = Path(os.environ.get("HARBOR_PI_PACKAGE_TARBALL", "benchmark-results/bin/pi-coding-agent-0.67.1/package.tgz"))
        if not node_src.is_file():
            raise FileNotFoundError(f"Node runtime tarball not found: {node_src}")
        if not pi_src.is_file():
            raise FileNotFoundError(f"pi package tarball not found: {pi_src}")
        await self.exec_as_root(
            environment,
            command=(
                "set -e; "
                f"mkdir -p /installed-agent /logs/agent {shlex.quote(_NODE_DIR)} {shlex.quote(_AGENT_DIR)}"
            ),
        )
        await environment.upload_file(node_src, _NODE_TARBALL_TARGET)
        await environment.upload_file(pi_src, _PI_TARBALL_TARGET)
        await self.exec_as_root(
            environment,
            command=(
                "set -e; "
                f"tar -xzf {shlex.quote(_NODE_TARBALL_TARGET)} -C {shlex.quote(_NODE_DIR)} --strip-components=1; "
                f"tar -xzf {shlex.quote(_PI_TARBALL_TARGET)} -C {shlex.quote(_AGENT_DIR)} --strip-components=1; "
                f"chmod 755 {shlex.quote(_NODE_DIR)}/bin/node {shlex.quote(_AGENT_DIR)}/dist/cli.js"
            ),
        )
        hosts_entries = _resolve_hosts_for_url(os.environ.get("FIZEAU_BASE_URL", ""))
        if hosts_entries:
            entries_cmd = "; ".join(
                f"echo '{ip} {host}' >> /etc/hosts" for host, ip in hosts_entries.items()
            )
            await self.exec_as_root(environment, command=entries_cmd)

    def _run_env(self, instruction: str) -> dict[str, str]:
        env: dict[str, str] = {
            "HARBOR_INSTRUCTION": instruction,
            "PI_CODING_AGENT_DIR": _AGENT_DIR,
        }
        base_url = _bench_env("FIZEAU_BASE_URL", "")
        model = _bench_env("FIZEAU_MODEL", "")
        api_key = _bench_env("FIZEAU_API_KEY", "")
        if api_key:
            env["FIZEAU_API_KEY"] = api_key
        elif _bench_env("OMLX_API_KEY"):
            env["FIZEAU_API_KEY"] = _bench_env("OMLX_API_KEY")
        return env

    @with_prompt_template
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        del context

        pi_bin = f"{_NODE_DIR}/bin/node {_AGENT_DIR}/dist/cli.js"
        command = (
            "set -euo pipefail; "
            "cd /testbed 2>/dev/null || cd /workspace 2>/dev/null || true; "
            f"{_write_models_json_command()}"
            f"{pi_bin} --mode json --print --no-session --no-extensions "
            f"--no-skills --no-prompt-templates --no-themes "
            '--provider openai-compat --model "$FIZEAU_MODEL" '
            f"--thinking {_bench_env('FIZEAU_REASONING', 'low')} "
            '"$HARBOR_INSTRUCTION" '
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
