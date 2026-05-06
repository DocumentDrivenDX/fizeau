"""Harbor BaseInstalledAgent adapter for opencode benchmark runs."""

from __future__ import annotations

import json
import os
import shlex
import shutil
import socket
from pathlib import Path
from typing import Any

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

OPENCODE_VERSION = "1.3.17"
_OUTPUT_LOG = "/logs/agent/opencode.txt"
_BINARY_TARGET = "/installed-agent/opencode"
_CONFIG_DIR = "/installed-agent/opencode-config"
_DATA_DIR = "/installed-agent/opencode-data"


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


def _config_template() -> str:
    return (
        '{"provider":{"openai-compat":{"npm":"@ai-sdk/openai-compatible",'
        '"name":"openai-compat","options":{"baseURL":"${FIZEAU_BASE_URL}",'
        '"apiKey":"{env:FIZEAU_API_KEY}","temperature":0.6},'
        '"models":{"${FIZEAU_MODEL}":{"limit":{"context":128000,"output":32768}}}}}}\n'
    )


class OpencodeAgent(BaseInstalledAgent):
    SUPPORTS_ATIF: bool = False

    def __init__(self, *args: Any, **kwargs: Any):
        kwargs.setdefault("version", "opencode-benchmark")
        super().__init__(*args, **kwargs)

    @staticmethod
    def name() -> str:
        return "opencode"

    async def install(self, environment: BaseEnvironment) -> None:
        binary_artifact = os.environ.get("HARBOR_OPENCODE_ARTIFACT", "")
        binary_src = Path(binary_artifact) if binary_artifact else Path(
            "benchmark-results/bin/opencode-1.3.17-linux-x64/opencode"
        )
        if not binary_src.is_file():
            fallback = Path(shutil.which("opencode") or "")
            if fallback.is_file():
                binary_src = fallback
        if not binary_src.is_file():
            raise FileNotFoundError(
                "opencode binary not found. Set HARBOR_OPENCODE_ARTIFACT or install opencode on the host."
            )
        await self.exec_as_root(
            environment,
            command=(
                "set -e; "
                "mkdir -p /installed-agent /logs/agent "
                f"{shlex.quote(_CONFIG_DIR)} {shlex.quote(_DATA_DIR)}"
            ),
        )
        await environment.upload_file(binary_src, _BINARY_TARGET)
        await self.exec_as_root(
            environment,
            command=f"chmod 755 {_BINARY_TARGET}",
        )
        hosts_entries = _resolve_hosts_for_url(os.environ.get("FIZEAU_BASE_URL", ""))
        if hosts_entries:
            entries_cmd = "; ".join(
                f"echo '{ip} {host}' >> /etc/hosts" for host, ip in hosts_entries.items()
            )
            await self.exec_as_root(environment, command=entries_cmd)

    def _run_env(self, instruction: str) -> dict[str, str]:
        env = {
            "HARBOR_INSTRUCTION": instruction,
            "FIZEAU_API_KEY": _bench_env("FIZEAU_API_KEY") or _bench_env("OMLX_API_KEY", "local"),
            "OPENCODE_DISABLE_AUTOUPDATE": "1",
            "OPENCODE_CONFIG_DIR": _CONFIG_DIR,
            "OPENCODE_DATA_DIR": _DATA_DIR,
        }
        return env

    @with_prompt_template
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        del context

        model_arg = "openai-compat/$FIZEAU_MODEL"
        command = (
            "set -euo pipefail; "
            "cd /testbed 2>/dev/null || cd /workspace 2>/dev/null || true; "
            "run_dir=\"$PWD\"; [ -d /app ] && run_dir=/app; "
            f"mkdir -p {shlex.quote(_CONFIG_DIR)} {shlex.quote(_DATA_DIR)} /logs/agent; "
            f"cat > {shlex.quote(_CONFIG_DIR)}/opencode.json <<EOF\n"
            f"{_config_template()}"
            "EOF\n"
            f"cp {shlex.quote(_CONFIG_DIR)}/opencode.json /logs/agent/opencode.config.json; "
            f"export OPENCODE_CONFIG_CONTENT=\"$(cat {shlex.quote(_CONFIG_DIR)}/opencode.json)\"; "
            f"{_BINARY_TARGET} run --format json --pure --print-logs --log-level DEBUG "
            f"--port 0 --title terminal-bench --dir \"$run_dir\" "
            f'-m "{model_arg}" -- "$HARBOR_INSTRUCTION" '
            f"2>&1 | stdbuf -oL tee {_OUTPUT_LOG}; "
            f"status=${{PIPESTATUS[0]}}; "
            f"tar -C {shlex.quote(_DATA_DIR)} -czf /logs/agent/opencode-data.tgz . 2>/dev/null || true; "
            f"exit $status"
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
            if isinstance(usage, dict):
                input_tokens += usage.get("input_tokens", 0) or usage.get("prompt_tokens", 0) or 0
                output_tokens += usage.get("output_tokens", 0) or usage.get("completion_tokens", 0) or 0
        context.n_input_tokens = input_tokens
        context.n_output_tokens = output_tokens
