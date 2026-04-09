#!/usr/bin/env python3
"""
harbor_agent.py — Harbor BaseInstalledAgent adapter for ddx-agent.

Implements the three-hook lifecycle expected by Harbor/Terminal-Bench:
  install()                   — copy binary, write provider config
  run(task_instruction)       — invoke ddx-agent, capture output
  populate_context_post_run() — convert session JSONL to ATIF v1.4

See docs/helix/02-design/solution-designs/SD-008-terminal-bench-integration.md
"""

import json
import os
import shutil
import subprocess
import sys
import uuid
from pathlib import Path
from typing import Any

# Harbor framework import — available inside the task container.
try:
    from harbor import BaseInstalledAgent
except ImportError:
    # Allow import for local testing without Harbor installed.
    class BaseInstalledAgent:  # type: ignore[no-redef]
        pass


# Paths inside the task container.
_INSTALL_DIR = Path("/usr/local/bin")
_CONFIG_DIR = Path.home() / ".config" / "agent"
_LOG_DIR = Path("/logs/agent")
_TRAJECTORY_PATH = _LOG_DIR / "trajectory.json"

# ddx-agent invocation flags (SD-008 §1, SD-009 §2).
_AGENT_FLAGS = ["--json", "--preset", "benchmark"]


class DDXAgent(BaseInstalledAgent):
    """
    Harbor installed-agent adapter for ddx-agent.

    Lifecycle (called by Harbor per trial):
      1. install()                   — once per container startup
      2. run(task)                   — once per task trial
      3. populate_context_post_run() — after run() returns
    """

    name = "ddx-agent"
    version = "1.0"

    def install(self) -> None:
        """
        Copy the pre-built linux/amd64 binary and write the provider config.

        Binary resolution order:
          1. $HARBOR_AGENT_ARTIFACT env var (Harbor passes artifact paths here)
          2. Same directory as this script (scripts/benchmark/ddx-agent-linux-amd64)
        """
        binary_src = Path(
            os.environ.get("HARBOR_AGENT_ARTIFACT", "ddx-agent-linux-amd64")
        )
        if not binary_src.exists():
            binary_src = Path(__file__).parent / "ddx-agent-linux-amd64"
        if not binary_src.exists():
            raise FileNotFoundError(
                f"ddx-agent binary not found. Expected at {binary_src} or "
                "set HARBOR_AGENT_ARTIFACT to the binary path."
            )

        dest = _INSTALL_DIR / "ddx-agent"
        _INSTALL_DIR.mkdir(parents=True, exist_ok=True)
        shutil.copy2(binary_src, dest)
        dest.chmod(0o755)
        print(f"[install] ddx-agent installed to {dest}", file=sys.stderr)

        # Write provider config with env-var expansion for API key.
        # ddx-agent config loader already supports ${ENV_VAR} expansion.
        _CONFIG_DIR.mkdir(parents=True, exist_ok=True)
        config_path = _CONFIG_DIR / "config.yaml"
        config_path.write_text(
            "providers:\n"
            "  benchmark:\n"
            "    type: anthropic\n"
            "    api_key: \"${ANTHROPIC_API_KEY}\"\n"
            "    model: claude-haiku-4-5-20251001\n"
            "default_provider: benchmark\n"
        )
        print(f"[install] config written to {config_path}", file=sys.stderr)

    def get_env(self) -> dict[str, str]:
        """Pass API credentials from the outer environment into the container."""
        env: dict[str, str] = {}
        for key in ("ANTHROPIC_API_KEY", "OPENROUTER_API_KEY"):
            val = os.environ.get(key, "")
            if val:
                env[key] = val
        return env

    def run(self, task_instruction: str, work_dir: str = ".") -> int:
        """
        Invoke ddx-agent with the task instruction.

        Returns:
          0  — agent attempted the task (Harbor reads reward from verifier)
          >0 — trial failure (Harbor marks task as failed)
        """
        _LOG_DIR.mkdir(parents=True, exist_ok=True)
        cmd = [
            str(_INSTALL_DIR / "ddx-agent"),
            *_AGENT_FLAGS,
            "-p", task_instruction,
            "--work-dir", work_dir,
        ]
        print(f"[run] {' '.join(cmd)}", file=sys.stderr)
        result = subprocess.run(cmd)
        print(f"[run] exit_code={result.returncode}", file=sys.stderr)
        return result.returncode

    def populate_context_post_run(self, work_dir: str = ".") -> None:
        """
        Convert the most recent ddx-agent JSONL session log to ATIF v1.4
        and write to /logs/agent/trajectory.json for Harbor's collector.
        """
        log_dir = Path(work_dir) / ".agent" / "session-logs"
        session_files = sorted(
            log_dir.glob("*.jsonl"),
            key=lambda p: p.stat().st_mtime,
        )
        if not session_files:
            print(
                "[post_run] WARNING: no session log found; writing empty trajectory",
                file=sys.stderr,
            )
            self._write_trajectory(self._empty_trajectory())
            return

        log_path = session_files[-1]
        print(f"[post_run] converting {log_path}", file=sys.stderr)
        trajectory = self._convert_session_log(log_path)
        self._write_trajectory(trajectory)
        print(
            f"[post_run] trajectory written to {_TRAJECTORY_PATH}"
            f" ({len(trajectory['steps'])} steps)",
            file=sys.stderr,
        )

    # ------------------------------------------------------------------ helpers

    def _convert_session_log(self, log_path: Path) -> dict[str, Any]:
        """Parse a ddx-agent JSONL session log and return an ATIF v1.4 dict."""
        events: list[dict[str, Any]] = []
        with log_path.open() as f:
            for line in f:
                line = line.strip()
                if line:
                    events.append(json.loads(line))

        # log_path stem is the session UUID written by ddx-agent's session logger.
        session_id = log_path.stem
        model_name = ""
        steps: list[dict[str, Any]] = []
        total_input = 0
        total_output = 0
        total_cost = 0.0
        step_id = 0

        for event in events:
            etype = event.get("type", "")
            data = event.get("data") or {}
            if isinstance(data, str):
                try:
                    data = json.loads(data)
                except json.JSONDecodeError:
                    data = {}
            ts = event.get("ts", "")
            session_id = event.get("session_id", session_id)

            if etype == "session.start":
                model_name = data.get("model", "")
                prompt = data.get("prompt", "")
                step_id += 1
                steps.append({
                    "step_id": step_id,
                    "timestamp": ts,
                    "source": "user",
                    "message": prompt,
                    "tool_calls": [],
                    "metrics": {"input_tokens": 0, "output_tokens": 0, "cost": 0},
                })

            elif etype == "llm.response":
                usage = data.get("usage") or {}
                cost = data.get("cost_usd") or 0.0
                if cost == -1:  # -1 means unknown model — treat as 0
                    cost = 0.0
                in_tok = usage.get("input", 0)
                out_tok = usage.get("output", 0)
                total_input += in_tok
                total_output += out_tok
                total_cost += cost
                model_name = data.get("model", model_name)

                tc_list = [
                    {
                        "id": tc.get("id", ""),
                        "name": tc.get("name", ""),
                        "arguments": tc.get("arguments", {}),
                        "result": None,
                    }
                    for tc in (data.get("tool_calls") or [])
                ]
                step_id += 1
                steps.append({
                    "step_id": step_id,
                    "timestamp": ts,
                    "source": "agent",
                    "message": data.get("content", ""),
                    "tool_calls": tc_list,
                    "metrics": {
                        "input_tokens": in_tok,
                        "output_tokens": out_tok,
                        "cost": cost,
                    },
                })

            elif etype == "tool.call":
                # Attach result to the matching pending tool call in the last agent step.
                tool_name = data.get("tool", "")
                output = data.get("output", "")
                for step in reversed(steps):
                    if step["source"] != "agent":
                        continue
                    for tc in step["tool_calls"]:
                        if tc["name"] == tool_name and tc["result"] is None:
                            tc["result"] = output
                            break
                    break

        return {
            "schema_version": "1.4",
            "session_id": session_id,
            "agent": {
                "name": "ddx-agent",
                "version": self.version,
                "model_name": model_name,
            },
            "steps": steps,
            "final_metrics": {
                "total_input_tokens": total_input,
                "total_output_tokens": total_output,
                "total_cost": total_cost,
            },
        }

    def _empty_trajectory(self) -> dict[str, Any]:
        return {
            "schema_version": "1.4",
            "session_id": str(uuid.uuid4()),
            "agent": {"name": "ddx-agent", "version": self.version, "model_name": ""},
            "steps": [],
            "final_metrics": {
                "total_input_tokens": 0,
                "total_output_tokens": 0,
                "total_cost": 0,
            },
        }

    def _write_trajectory(self, trajectory: dict[str, Any]) -> None:
        _LOG_DIR.mkdir(parents=True, exist_ok=True)
        _TRAJECTORY_PATH.write_text(json.dumps(trajectory, indent=2))


# Allow running directly for local testing outside Harbor.
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: harbor_agent.py '<task instruction>' [work_dir]")
        sys.exit(1)
    task = sys.argv[1]
    work_dir = sys.argv[2] if len(sys.argv) > 2 else "."
    a = DDXAgent()
    a.install()
    rc = a.run(task, work_dir)
    a.populate_context_post_run(work_dir)
    sys.exit(rc)
