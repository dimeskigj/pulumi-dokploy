"""Exercise the serial acceptance runner without contacting Dokploy."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


RUNNER = Path(__file__).with_name("run-live-acceptance.sh")


class AcceptanceRunnerTests(unittest.TestCase):
    def run_segments(self, selection="all", fail="", stop="", repeats=1, overrides=None):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fake_go = root / "go"
            fake_go.write_text(
                '#!/bin/bash\n'
                'for arg in "$@"; do\n'
                ' arg="${arg#^}"; arg="${arg%\\$}"\n'
                ' case "$arg" in TestLiveTier*|TestAccLifecycleSmoke) segment="$arg";; esac\n'
                'done\n'
                'printf "%s\\n" "$segment" >> "$CALLS"\n'
                'if [[ "$segment" == "$STOP" ]]; then printf "stop\\n" > "$DOKPLOY_ACCEPTANCE_STOP_FILE"; fi\n'
                'if [[ "$segment" == "$FAIL" ]]; then exit 1; fi\n'
            )
            fake_go.chmod(0o755)
            calls = root / "calls"
            marker = root / "stop"
            environment = {
                **os.environ,
                "PATH": str(root) + ":/usr/bin:/bin",
                "DOKPLOY_ACCEPTANCE": "1",
                "DOKPLOY_ENDPOINT": "https://acceptance.example.invalid",
                "DOKPLOY_API_KEY": "offline-key",
                "DOKPLOY_ACCEPTANCE_STOP_FILE": str(marker),
                "CALLS": str(calls), "FAIL": fail, "STOP": stop,
            }
            environment.update(overrides or {})
            results = [subprocess.run(
                ["/bin/bash", str(RUNNER), selection], env=environment,
                capture_output=True, text=True, check=False,
            ) for _ in range(repeats)]
            return results, calls.read_text().splitlines() if calls.exists() else [], marker.exists()

    def test_ordinary_failure_does_not_block_other_segments(self):
        expected = ["TestLiveTier1ControlPlane", "TestLiveTier2Workloads",
                    "TestLiveTier3Databases", "TestLiveTier4Backups", "TestAccLifecycleSmoke"]
        for failure in expected:
            with self.subTest(failure=failure):
                results, calls, stopped = self.run_segments(fail=failure)
                self.assertEqual(calls, expected)
                self.assertEqual(results[0].returncode, 1)
                self.assertFalse(stopped)
                self.assertNotIn("offline-key", results[0].stdout + results[0].stderr)

    def test_each_segment_can_run_without_earlier_segments(self):
        for selection, expected in [("tier1", "TestLiveTier1ControlPlane"),
                                    ("tier2", "TestLiveTier2Workloads"),
                                    ("tier3", "TestLiveTier3Databases"),
                                    ("tier4", "TestLiveTier4Backups"),
                                    ("smoke", "TestAccLifecycleSmoke")]:
            with self.subTest(selection=selection):
                results, calls, stopped = self.run_segments(selection)
                self.assertEqual(results[0].returncode, 0, results[0].stderr)
                self.assertEqual(calls, [expected])
                self.assertFalse(stopped)

    def test_repeated_runs_do_not_retain_ordinary_failure_state(self):
        results, calls, stopped = self.run_segments("tier3", fail="TestLiveTier3Databases", repeats=3)
        self.assertEqual([result.returncode for result in results], [1, 1, 1])
        self.assertEqual(calls, ["TestLiveTier3Databases"] * 3)
        self.assertFalse(stopped)

    def test_successful_runs_can_be_repeated(self):
        results, calls, stopped = self.run_segments("smoke", repeats=3)
        self.assertEqual([result.returncode for result in results], [0, 0, 0])
        self.assertEqual(calls, ["TestAccLifecycleSmoke"] * 3)
        self.assertFalse(stopped)

    def test_missing_opt_in_credentials_or_marker_never_runs_go(self):
        for variable in ["DOKPLOY_ACCEPTANCE", "DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY", "DOKPLOY_ACCEPTANCE_STOP_FILE"]:
            with self.subTest(variable=variable):
                results, calls, stopped = self.run_segments(overrides={variable: ""})
                self.assertNotEqual(results[0].returncode, 0)
                self.assertEqual(calls, [])
                self.assertFalse(stopped)

    def test_stop_marker_blocks_later_segments_and_reruns(self):
        results, calls, stopped = self.run_segments(stop="TestLiveTier2Workloads", repeats=2)
        self.assertEqual(calls, ["TestLiveTier1ControlPlane", "TestLiveTier2Workloads"])
        self.assertEqual([result.returncode for result in results], [1, 1])
        self.assertTrue(stopped)

    def test_invalid_selection_has_no_side_effects(self):
        results, calls, stopped = self.run_segments("unknown")
        self.assertNotEqual(results[0].returncode, 0)
        self.assertEqual(calls, [])
        self.assertFalse(stopped)


if __name__ == "__main__":
    unittest.main()
