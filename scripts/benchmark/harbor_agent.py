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
import socket
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


def _resolve_hosts_for_url(base_url: str) -> dict[str, str]:
    """Resolve the hostname in base_url on the host and return {hostname: ip}.

    Returns an empty dict if base_url is empty, uses an IP address directly,
    or if DNS resolution fails. Only non-loopback, non-link-local results are
    returned (local inference servers like vidar/bragi have routable IPs).
    """
    if not base_url:
        return {}
    try:
        from urllib.parse import urlparse
        parsed = urlparse(base_url)
        host = parsed.hostname or ""
        if not host or host in ("localhost", "127.0.0.1", "::1"):
            return {}
        # Skip if already an IP address
        try:
            socket.inet_pton(socket.AF_INET, host)
            return {}
        except OSError:
            pass
        try:
            socket.inet_pton(socket.AF_INET6, host)
            return {}
        except OSError:
            pass
        ip = socket.gethostbyname(host)
        if ip.startswith("127.") or ip.startswith("169.254."):
            return {}
        return {host: ip}
    except Exception:
        return {}


class FizeauAgent(BaseInstalledAgent):
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

        # Some TB-2 task images (e.g. ubuntu:24.04-based overfull-hbox /
        # regex-log) ship without root CA certificates, which makes fiz's TLS
        # handshake to OpenRouter fail with `x509: certificate signed by
        # unknown authority` and burn a task with 0 tokens consumed. Stage a
        # CA bundle into the container so fiz can reach https endpoints.
        #
        # Strategy: upload a host CA bundle directly to the standard Debian
        # path. This is deterministic and does not depend on the container
        # having working apt/dnf/network at install time (the runtime
        # apt-install fallback below handles the case where no host bundle
        # is available).
        await self.exec_as_root(
            environment,
            command="mkdir -p /etc/ssl/certs",
        )
        host_ca_bundle = self._find_host_ca_bundle()
        if host_ca_bundle is not None:
            await environment.upload_file(
                host_ca_bundle, "/etc/ssl/certs/ca-certificates.crt"
            )
            await self.exec_as_root(
                environment,
                command=(
                    "chmod 644 /etc/ssl/certs/ca-certificates.crt && "
                    "ln -sf /etc/ssl/certs/ca-certificates.crt /etc/ssl/cert.pem 2>/dev/null; "
                    "exit 0"
                ),
            )

        # Fallback: best-effort package-manager install for images that
        # already had network/apt working but were missing the bundle. This
        # also runs when the host had no bundle to upload.
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

        # Harbor task Docker Compose networks use isolated DNS that doesn't
        # inherit the host's Tailscale DNS. Resolve any FIZEAU_BASE_URL
        # hostname on the host side and inject it into /etc/hosts so that
        # fiz inside the container can reach local inference servers (vidar,
        # bragi, etc.) by their Tailscale hostnames.
        base_url = os.environ.get("FIZEAU_BASE_URL", "")
        hosts_entries = _resolve_hosts_for_url(base_url)
        if hosts_entries:
            entries_cmd = "; ".join(
                f"echo '{ip} {host}' >> /etc/hosts"
                for host, ip in hosts_entries.items()
            )
            await self.exec_as_root(environment, command=entries_cmd)

    @staticmethod
    def _find_host_ca_bundle() -> Path | None:
        # Standard CA bundle locations across Debian/Ubuntu, RHEL, Alpine,
        # macOS-with-homebrew, and Python's certifi (used as a last resort).
        candidates = [
            os.environ.get("SSL_CERT_FILE", ""),
            "/etc/ssl/certs/ca-certificates.crt",
            "/etc/pki/tls/certs/ca-bundle.crt",
            "/etc/ssl/cert.pem",
            "/usr/local/etc/openssl/cert.pem",
            "/opt/homebrew/etc/ca-certificates/cert.pem",
        ]
        for candidate in candidates:
            if candidate and Path(candidate).is_file() and Path(candidate).stat().st_size > 0:
                return Path(candidate)
        try:
            import certifi  # type: ignore[import-not-found]

            certifi_path = Path(certifi.where())
            if certifi_path.is_file() and certifi_path.stat().st_size > 0:
                return certifi_path
        except Exception:
            pass
        return None

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

        # fiz writes its session JSONL to <workdir>/.fizeau/sessions/ by
        # default (DefaultSessionLogDir). Harbor downloads /logs/agent into
        # the adapter's logs_dir, so we mirror the JSONL files into
        # /logs/agent/sessions/ after fiz exits — that's where
        # populate_context_post_run looks for them. Without this copy,
        # trajectory.json comes back with 0 steps even on passing runs.
        command = (
            "set -uo pipefail; "
            "cd /testbed 2>/dev/null || cd /workspace 2>/dev/null || true; "
            f'cp {_AGENTS_MD_TARGET} "$(pwd)/AGENTS.md" 2>/dev/null || true; '
            f"mkdir -p {_SESSION_LOG_DIR}; "
            f"{_BINARY_TARGET} --json --preset default "
            '--work-dir "$(pwd)" '
            '-p "$HARBOR_INSTRUCTION" '
            f'2>&1 | stdbuf -oL tee {_OUTPUT_LOG}; '
            'fiz_rc=${PIPESTATUS[0]}; '
            f'cp -f "$(pwd)/.fizeau/sessions/"*.jsonl {_SESSION_LOG_DIR}/ 2>/dev/null || true; '
            'exit "$fiz_rc"'
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
