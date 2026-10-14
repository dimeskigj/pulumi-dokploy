import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


RUNNER = Path(__file__).with_name("test_sdk_contracts.py")
SPEC = importlib.util.spec_from_file_location("sdk_contract_runner", RUNNER)
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)


class SDKContractRunnerTests(unittest.TestCase):
    def make_temp(self, **kwargs):
        td = tempfile.TemporaryDirectory(**kwargs)
        self.addCleanup(td.cleanup)
        return Path(td.name)

    def test_runs_all_contracts_without_safe_launcher_and_sanitizes_runtime_env(self):
        tmp = self.make_temp()
        env = {"PATH": "/usr/bin", "TMPDIR": str(tmp), "DOKPLOY_TOKEN": "secret", "OTHER": "ok"}
        with patch.dict(os.environ, env, clear=True), patch.object(runner.subprocess, "run") as execute:
            runner.main()
        calls = execute.call_args_list
        commands = [call.args[0] for call in calls]
        self.assertEqual(sum("go" in command and "test" in command for command in commands), 1)
        self.assertEqual(sum("tsc" in command for command in commands), 1)
        self.assertEqual(sum("py_compile" in command for command in commands), 1)
        self.assertEqual(sum("import contract" in command for command in commands), 1)
        self.assertEqual(sum("dotnet" in command for command in commands), 1)
        self.assertEqual(sum("mvn" in command for command in commands), 1)
        for call in calls:
            child_env = call.kwargs.get("env")
            if child_env:
                self.assertEqual(child_env.get("DOKPLOY_ACCEPTANCE"), "0")
                self.assertNotIn("DOKPLOY_TOKEN", child_env)
        self.assertTrue(all(not str(c[0][0]).startswith("/tmp/opencode") for c in calls))

    def test_python_environment_uses_caller_tmpdir_and_is_isolated_per_run(self):
        tmp = self.make_temp()
        with patch.dict(os.environ, {"TMPDIR": str(tmp)}, clear=True), patch.object(runner.tempfile, "tempdir", None), patch.object(runner.subprocess, "run") as execute:
            runner.main()
            runner.main()
        venvs = [Path(c.args[0][-1]) for c in execute.call_args_list if "venv" in c.args[0]]
        self.assertEqual(len(venvs), 2)
        self.assertNotEqual(venvs[0], venvs[1])
        self.assertTrue(all(path.parent.parent == tmp for path in venvs))
        self.assertTrue(all(not path.exists() for path in venvs))

    def test_failure_propagates(self):
        import subprocess
        with patch.object(runner.subprocess, "run", side_effect=subprocess.CalledProcessError(7, ["go"])):
            with self.assertRaises(subprocess.CalledProcessError):
                runner.main()

    def test_make_orders_contracts_after_java_sdk_local_publish(self):
        makefile = Path(__file__).resolve().parents[1] / "Makefile"
        target = makefile.read_text().split("test_examples:", 1)[1].split("\n.PHONY: test_sdk_contracts", 1)[0]
        self.assertLess(target.index("gradle publishToMavenLocal"), target.index("test_sdk_contracts"))

    def test_make_has_standalone_local_java_sdk_setup_and_all_fixture_directories(self):
        makefile = Path(__file__).resolve().parents[1] / "Makefile"
        target = makefile.read_text().split("test_sdk_contracts:", 1)[1].split("\n", 1)[0]
        self.assertIn("install_java_sdk", target)
        root = Path(__file__).resolve().parents[1] / "examples" / "sdk-contracts"
        self.assertEqual({p.name for p in root.iterdir() if p.is_dir()}, {"go", "nodejs", "python", "dotnet", "java"})

    def test_contracts_stay_outside_generated_paths(self):
        for language in ("go", "nodejs", "python", "dotnet", "java"):
            self.assertTrue((Path(__file__).resolve().parents[1] / "examples" / "sdk-contracts" / language).is_dir())
