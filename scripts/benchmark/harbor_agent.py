#!/usr/bin/env python3
"""
harbor_agent.py — Harbor 0.3.x BaseInstalledAgent adapter for fiz.

This adapter stages a prebuilt fiz binary into the task environment,
writes a minimal config rooted under /installed-agent/home, runs fiz in
the task workspace, and converts downloaded session logs into a trajectory file
that our benchmark scoring path can consume.
"""

from __future__ import annotations

import json
import os
import shlex
import uuid
from pathlib import Path
from typing import Any

from harbor.agents.installed.base import BaseInstalledAgent, with_prompt_template
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

_INSTALL_ROOT = "/installed-agent"
_BINARY_TARGET = f"{_INSTALL_ROOT}/fiz"
_AGENTS_MD_TARGET = f"{_INSTALL_ROOT}/AGENTS.md"
_HOME_DIR = f"{_INSTALL_ROOT}/home"
_SESSION_LOG_DIR = "/logs/agent/sessions"
_OUTPUT_LOG = "/logs/agent/fiz.txt"


def _bench_env(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


class DDXAgent(BaseInstalledAgent):
    SUPPORTS_ATIF: bool = False

    def __init__(self, *args: Any, **kwargs: Any):
        kwargs.setdefault("version", "fiz-benchmark")
        super().__init__(*args, **kwargs)

    @staticmethod
    def name() -> str:
        return "fiz"

    async def install(self, environment: BaseEnvironment) -> None:
        binary_src = Path(os.environ.get("HARBOR_AGENT_ARTIFACT", ""))
        if not binary_src.exists():
            binary_src = Path(__file__).parent / "fiz-linux-amd64"
        if not binary_src.exists():
            raise FileNotFoundError(
                f"fiz binary not found. Expected {binary_src} or set "
                "HARBOR_AGENT_ARTIFACT to the host binary path."
            )

        await self.exec_as_root(
            environment,
            command=(
                f"mkdir -p {_INSTALL_ROOT} {_HOME_DIR} /logs/agent "
                f"&& chmod 755 {_INSTALL_ROOT}"
            ),
        )

        # Some TB-2 task images (e.g. minimal LaTeX containers) ship without
        # root CA certificates, which makes fiz's TLS handshake to OpenRouter
        # fail with `x509: certificate signed by unknown authority` and burn a
        # task with 0 tokens consumed. Probe for a CA bundle and install one if
        # missing, across Debian/Ubuntu, Alpine, and RHEL-family images. All
        # steps are best-effort so offline images do not block the run.
        await self.exec_as_root(
            environment,
            command=(
                "set +e; "
                "if [ -s /etc/ssl/certs/ca-certificates.crt ] "
                "|| [ -s /etc/pki/tls/certs/ca-bundle.crt ] "
                "|| [ -s /etc/ssl/cert.pem ]; then exit 0; fi; "
                "if command -v apt-get >/dev/null 2>&1; then "
                "  DEBIAN_FRONTEND=noninteractive apt-get update -qq "
                "  && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates "
                "  && update-ca-certificates; "
                "elif command -v apk >/dev/null 2>&1; then "
                "  apk add --no-cache ca-certificates && update-ca-certificates; "
                "elif command -v dnf >/dev/null 2>&1; then "
                "  dnf install -y ca-certificates && update-ca-trust; "
                "elif command -v yum >/dev/null 2>&1; then "
                "  yum install -y ca-certificates && update-ca-trust; "
                "fi; "
                "exit 0"
            ),
        )

        await environment.upload_file(binary_src, _BINARY_TARGET)
        await self.exec_as_root(
            environment, command=f"chmod 755 {_BINARY_TARGET}"
        )

        agents_md_src = Path(__file__).parent / "AGENTS.md"
        if agents_md_src.exists():
            await environment.upload_file(agents_md_src, _AGENTS_MD_TARGET)

    def _run_env(self, instruction: str) -> dict[str, str]:
        env: dict[str, str] = {
            "HARBOR_INSTRUCTION": instruction,
            "HOME": _HOME_DIR,
        }
        # Forward all FIZEAU_* vars from the process env into the container.
        # FIZEAU_BASE_URL, FIZEAU_MODEL, FIZEAU_PROVIDER, FIZEAU_API_KEY_ENV,
        # FIZEAU_HEADERS_JSON, FIZEAU_REASONING etc. are set by the runner scripts.
        for key, val in os.environ.items():
            if key.startswith("FIZEAU_"):
                env[key] = val
        # Resolve FIZEAU_API_KEY from FIZEAU_API_KEY_ENV if not already set.
        api_key_env = env.get("FIZEAU_API_KEY_ENV", "")
        if api_key_env and "FIZEAU_API_KEY" not in env:
            api_key_val = os.environ.get(api_key_env, "")
            if api_key_val:
                env["FIZEAU_API_KEY"] = api_key_val
        return env

    @with_prompt_template
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        del context

        command = (
            "set -euo pipefail; "
            "cd /testbed 2>/dev/null || cd /workspace 2>/dev/null || true; "
            f'cp {_AGENTS_MD_TARGET} "$(pwd)/AGENTS.md" 2>/dev/null || true; '
            f"{_BINARY_TARGET} --json --preset default "
            '--work-dir "$(pwd)" '
            '-p "$HARBOR_INSTRUCTION" '
            f'2>&1 | stdbuf -oL tee {_OUTPUT_LOG}'
        )

        await self.exec_as_agent(
            environment,
            command=command,
            env=self._run_env(instruction),
        )

    def populate_context_post_run(self, context: AgentContext) -> None:
        trajectory, totals = self._build_trajectory()
        trajectory_path = self.logs_dir / "trajectory.json"
        trajectory_path.write_text(json.dumps(trajectory, indent=2), encoding="utf-8")

        context.n_input_tokens = totals["input"]
        context.n_output_tokens = totals["output"]
        context.cost_usd = totals["cost"]

    def _build_trajectory(self) -> tuple[dict[str, Any], dict[str, float]]:
        session_files = sorted(
            (self.logs_dir / "sessions").glob("*.jsonl"),
            key=lambda p: p.stat().st_mtime,
        )
        if not session_files:
            return self._empty_trajectory(), {"input": 0, "output": 0, "cost": 0.0}

        events: list[dict[str, Any]] = []
        for line in session_files[-1].read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if not line:
                continue
            events.append(json.loads(line))

        steps: list[dict[str, Any]] = []
        session_id = session_files[-1].stem
        model_name = ""
        total_input = 0
        total_output = 0
        total_cost = 0.0
        step_id = 1

        for event in events:
            etype = event.get("type", "")
            data = event.get("data") or {}
            if isinstance(data, str):
                try:
                    data = json.loads(data)
                except json.JSONDecodeError:
                    data = {}
            timestamp = event.get("timestamp") or event.get("ts")
            session_id = event.get("session_id", session_id)

            if etype == "session.start":
                model_name = data.get("model", model_name)
                prompt = data.get("prompt", "")
                if prompt:
                    steps.append(
                        {
                            "step_id": step_id,
                            "timestamp": timestamp,
                            "source": "user",
                            "message": prompt,
                        }
                    )
                    step_id += 1
                continue

            if etype == "llm.response":
                usage = data.get("usage") or {}
                cost = data.get("cost_usd") or 0.0
                if cost == -1:
                    cost = 0.0
                prompt_tokens = usage.get("input", 0) or 0
                completion_tokens = usage.get("output", 0) or 0
                total_input += prompt_tokens
                total_output += completion_tokens
                total_cost += cost
                model_name = data.get("model", model_name)

                tool_calls = []
                for tc in data.get("tool_calls") or []:
                    name = tc.get("name", "")
                    tool_calls.append(
                        {
                            "tool_call_id": tc.get("id", ""),
                            "function_name": name,
                            "arguments": tc.get("arguments", {}),
                            "name": name,
                            "result": "",
                            "error": "",
                        }
                    )

                step: dict[str, Any] = {
                    "step_id": step_id,
                    "timestamp": timestamp,
                    "source": "agent",
                    "message": data.get("content", "") or "(tool use)",
                    "model_name": model_name,
                    "tool_calls": tool_calls or None,
                    "metrics": {
                        "prompt_tokens": prompt_tokens,
                        "completion_tokens": completion_tokens,
                        "cost_usd": cost,
                    },
                }
                steps.append(step)
                step_id += 1
                continue

            if etype == "tool.call":
                tool_name = data.get("tool", "")
                output = data.get("output", "")
                err = data.get("error", "")
                for step in reversed(steps):
                    if step.get("source") != "agent":
                        continue
                    tool_calls = step.get("tool_calls") or []
                    for tc in tool_calls:
                        if tc.get("name") == tool_name and not tc.get("result"):
                            tc["result"] = output
                            tc["error"] = err
                            observation = step.setdefault("observation", {"results": []})
                            observation["results"].append(
                                {
                                    "source_call_id": tc.get("tool_call_id"),
                                    "content": err or output,
                                }
                            )
                            break
                    else:
                        continue
                    break

        trajectory = {
            "schema_version": "ATIF-v1.6-ddx",
            "session_id": session_id,
            "agent": {
                "name": "fiz",
                "version": self.version() or "unknown",
                "model_name": model_name,
            },
            "steps": steps,
            "final_metrics": {
                "total_prompt_tokens": total_input,
                "total_completion_tokens": total_output,
                "total_cost_usd": total_cost,
                "total_steps": len(steps),
            },
        }
        return trajectory, {
            "input": total_input,
            "output": total_output,
            "cost": total_cost,
        }

    def _empty_trajectory(self) -> dict[str, Any]:
        return {
            "schema_version": "ATIF-v1.6-ddx",
            "session_id": str(uuid.uuid4()),
            "agent": {
                "name": "fiz",
                "version": self.version() or "unknown",
                "model_name": self.model_name or "",
            },
            "steps": [],
            "final_metrics": {
                "total_prompt_tokens": 0,
                "total_completion_tokens": 0,
                "total_cost_usd": 0.0,
                "total_steps": 0,
            },
        }
