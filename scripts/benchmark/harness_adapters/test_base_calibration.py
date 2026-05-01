from __future__ import annotations

import unittest

from scripts.benchmark.harness_adapters.base import BenchmarkProfile
from scripts.benchmark.harness_adapters._test.fake_provider import PROFILE
from scripts.benchmark.harness_adapters.fiz import Agent as Fiz
from scripts.benchmark.harness_adapters.dumb_script import Agent as DumbScript
from scripts.benchmark.harness_adapters.noop import Agent as Noop


class BaseCalibrationAdapterTest(unittest.TestCase):
    def setUp(self) -> None:
        self.profile = BenchmarkProfile.from_mapping(PROFILE)

    def test_noop_calibration_reports_zero_reward(self) -> None:
        agent = Noop()
        self.assertEqual(agent.install().argv, ["true"])
        self.assertEqual(agent.command(self.profile, "ignored").argv, ["sh", "-c", "true"])
        telemetry = agent.parse_telemetry("")
        self.assertEqual(telemetry["reward"], 0)
        self.assertEqual(telemetry["process_outcome"], "completed")

    def test_dumb_script_calibration_only_rewards_hello_world(self) -> None:
        agent = DumbScript()
        self.assertIn("hello.txt", " ".join(agent.command(self.profile, "ignored").argv))
        self.assertEqual(agent.parse_telemetry("task=hello-world")["reward"], 1)
        self.assertEqual(agent.parse_telemetry("task=git-leak-recovery")["reward"], 0)

    def test_fiz_command_maps_profile_to_env(self) -> None:
        agent = Fiz()
        spec = agent.command(self.profile, "solve", "/tmp/task")
        self.assertEqual(spec.argv[:4], ["/installed-agent/fiz", "--json", "--preset", "benchmark"])
        self.assertEqual(spec.env["DDX_BENCH_PROVIDER_TYPE"], "openai-compat")
        self.assertEqual(spec.env["DDX_BENCH_PROVIDER_MODEL"], "qwen/qwen3.6-plus")
        self.assertEqual(spec.env["DDX_BENCH_PROVIDER_API_KEY_ENV"], "FAKE_PROVIDER_API_KEY")
        self.assertEqual(spec.stdin, "")

    def test_fiz_telemetry_reads_session_end_usage(self) -> None:
        stream = '{"type":"session.end","data":{"usage":{"input":10,"output":7}}}'
        telemetry = Fiz().parse_telemetry(stream)
        self.assertEqual(telemetry["input_tokens"], 10)
        self.assertEqual(telemetry["output_tokens"], 7)


if __name__ == "__main__":
    unittest.main()
