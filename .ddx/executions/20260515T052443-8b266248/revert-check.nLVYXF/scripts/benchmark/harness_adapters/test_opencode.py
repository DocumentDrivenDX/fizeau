from __future__ import annotations

import json
import os
import unittest

from scripts.benchmark.harness_adapters.base import BenchmarkProfile
from scripts.benchmark.harness_adapters.opencode import Agent, OPENCODE_VERSION
from scripts.benchmark.harness_adapters._test.fake_provider import API_KEY, PROFILE


class OpencodeAdapterTest(unittest.TestCase):
    def setUp(self) -> None:
        raw = dict(PROFILE)
        raw["provider"] = dict(PROFILE["provider"])
        raw["provider"]["base_url"] = "https://openrouter.ai/api/v1"
        self.profile = BenchmarkProfile.from_mapping(raw)
        self.agent = Agent()

    def test_install_pins_versioned_installer(self) -> None:
        spec = self.agent.install()
        self.assertEqual(spec.argv[0:2], ["sh", "-c"])
        self.assertIn("https://opencode.ai/install", spec.argv[2])
        self.assertIn(f"VERSION={OPENCODE_VERSION}", spec.argv[2])

    def test_command_is_non_interactive_and_maps_profile(self) -> None:
        os.environ["FAKE_PROVIDER_API_KEY"] = API_KEY
        spec = self.agent.command(self.profile, "solve task", "/tmp/task")

        self.assertEqual(spec.argv[:7], [
            "opencode",
            "run",
            "--format",
            "json",
            "--pure",
            "--port",
            "0",
        ])
        self.assertIn("--dir", spec.argv)
        self.assertIn("/tmp/task", spec.argv)
        self.assertIn("-m", spec.argv)
        self.assertIn("openrouter/qwen/qwen3.6-plus", spec.argv)
        self.assertIn("--variant", spec.argv)
        self.assertIn("medium", spec.argv)
        self.assertEqual(spec.argv[-2:], ["--", "solve task"])
        self.assertEqual(spec.stdin, "")
        self.assertEqual(spec.env["OPENCODE_DISABLE_AUTOUPDATE"], "1")
        self.assertTrue(spec.env["OPENCODE_CONFIG_DIR"].endswith("/opencode-config"))
        self.assertTrue(spec.env["OPENCODE_DATA_DIR"].endswith("/opencode-data"))

        config = json.loads(spec.env["OPENCODE_CONFIG_CONTENT"])
        self.assertEqual(config["provider"]["openrouter"]["npm"], "@ai-sdk/openai-compatible")
        options = config["provider"]["openrouter"]["options"]
        self.assertEqual(options["baseURL"], "https://openrouter.ai/api/v1")
        self.assertEqual(options["apiKey"], "{env:FAKE_PROVIDER_API_KEY}")
        self.assertEqual(options["temperature"], 0.0)
        self.assertEqual(options["maxTokens"], 4096)
        self.assertEqual(
            config["provider"]["openrouter"]["models"]["qwen/qwen3.6-plus"]["limit"]["context"],
            131072,
        )

    def test_parse_telemetry_maps_json_events_to_d4_schema(self) -> None:
        stream = "\n".join([
            '{"type":"tool_call_start","name":"bash"}',
            '{"type":"message","usage":{"input_tokens":21,"output_tokens":8}}',
        ])
        telemetry = self.agent.parse_telemetry(stream)

        self.assertEqual(telemetry["process_outcome"], "completed")
        self.assertEqual(telemetry["grading_outcome"], "ungraded")
        self.assertEqual(telemetry["tool_calls"], 1)
        self.assertEqual(telemetry["input_tokens"], 21)
        self.assertEqual(telemetry["output_tokens"], 8)
        self.assertIn("cached_input_tokens", telemetry)
        self.assertIn("retried_input_tokens", telemetry)

    def test_redact_secrets_scrubs_generated_config_values(self) -> None:
        redacted = self.agent.redact_secrets(
            f"{API_KEY} https://openrouter.ai/api/v1",
            {"FAKE_PROVIDER_API_KEY": API_KEY},
            self.profile,
        )

        self.assertNotIn(API_KEY, redacted)
        self.assertNotIn("https://openrouter.ai/api/v1", redacted)
        self.assertIn("[REDACTED]", redacted)


if __name__ == "__main__":
    unittest.main()
