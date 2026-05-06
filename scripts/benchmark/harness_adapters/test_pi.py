from __future__ import annotations

import json
import os
import unittest

from scripts.benchmark.harness_adapters.base import BenchmarkProfile
from scripts.benchmark.harness_adapters.pi import Agent, PI_VERSION
from scripts.benchmark.harness_adapters._test.fake_provider import API_KEY, PROFILE


class PiAdapterTest(unittest.TestCase):
    def setUp(self) -> None:
        self.profile = BenchmarkProfile.from_mapping(PROFILE)
        self.agent = Agent()

    def test_install_pins_npm_package(self) -> None:
        spec = self.agent.install()
        self.assertEqual(
            spec.argv,
            ["npm", "install", "-g", f"@mariozechner/pi-coding-agent@{PI_VERSION}"],
        )

    def test_command_is_non_interactive_and_maps_openai_compat_profile(self) -> None:
        os.environ["FAKE_PROVIDER_API_KEY"] = API_KEY
        spec = self.agent.command(self.profile, "solve task", "/tmp/task")

        self.assertEqual(spec.argv[:9], [
            "pi",
            "--mode",
            "json",
            "--print",
            "--no-session",
            "--no-extensions",
            "--no-skills",
            "--no-prompt-templates",
            "--no-themes",
        ])
        self.assertIn("--provider", spec.argv)
        self.assertIn("openai-compat", spec.argv)
        self.assertIn("--model", spec.argv)
        self.assertIn("qwen/qwen3.6-plus", spec.argv)
        self.assertIn("--thinking", spec.argv)
        self.assertIn("medium", spec.argv)
        self.assertEqual(spec.argv[-1], "solve task")
        self.assertEqual(spec.stdin, "")
        self.assertEqual(spec.cwd, "/tmp/task")
        self.assertEqual(spec.env["FAKE_PROVIDER_API_KEY"], API_KEY)
        self.assertIn("PI_CODING_AGENT_DIR", spec.env)
        with open(os.path.join(spec.env["PI_CODING_AGENT_DIR"], "models.json"), encoding="utf-8") as f:
            config = json.load(f)
        model = config["providers"]["openai-compat"]["models"][0]
        self.assertEqual(config["providers"]["openai-compat"]["baseUrl"], "http://127.0.0.1:65530/v1")
        self.assertEqual(config["providers"]["openai-compat"]["apiKey"], "FAKE_PROVIDER_API_KEY")
        self.assertEqual(model["id"], "qwen/qwen3.6-plus")
        self.assertEqual(model["api"], "openai-completions")
        self.assertEqual(model["contextWindow"], 131072)
        self.assertTrue(any("temperature" in note for note in spec.notes))
        self.assertTrue(any("max_output_tokens" in note for note in spec.notes))

    def test_parse_telemetry_maps_pi_jsonl_to_d4_schema(self) -> None:
        stream = "\n".join([
            '{"type":"tool_call_start","name":"bash"}',
            '{"type":"text_end","message":{"usage":{"input_tokens":12,"output_tokens":5}}}',
            '{"type":"text_end","response":"done"}',
        ])
        telemetry = self.agent.parse_telemetry(stream)

        self.assertEqual(telemetry["process_outcome"], "completed")
        self.assertEqual(telemetry["grading_outcome"], "ungraded")
        self.assertIsNone(telemetry["reward"])
        self.assertEqual(telemetry["tool_calls"], 1)
        self.assertEqual(telemetry["input_tokens"], 12)
        self.assertEqual(telemetry["output_tokens"], 5)
        self.assertIn("cached_input_tokens", telemetry)
        self.assertIn("retried_input_tokens", telemetry)
        self.assertGreaterEqual(telemetry["wall_seconds"], 0.0)

    def test_redact_secrets_scrubs_profile_api_key_values(self) -> None:
        redacted = self.agent.redact_secrets(
            f"Authorization: Bearer {API_KEY}",
            {"FAKE_PROVIDER_API_KEY": API_KEY},
            self.profile,
        )

        self.assertNotIn(API_KEY, redacted)
        self.assertIn("[REDACTED]", redacted)


if __name__ == "__main__":
    unittest.main()
