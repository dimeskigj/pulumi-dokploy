"""Executable portability regressions for the example generator preflight."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("gen-examples.sh").resolve()


class GeneratorPortabilityTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir="/tmp/opencode")
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "calls"
        self.output = self.repo / "examples" / "nodejs"
        self.output.mkdir(parents=True)
        (self.output / "preserve.txt").write_text("existing")
        (self.repo / "examples/go").mkdir()
        (self.repo / "examples/go/lookup_test.go").write_text("fixture")
        self.mise = self.command("mise", 'printf "%s\\n" "$*" >> "$CALL_LOG"\nif [ "$*" = "exec pulumi@3.259.0 -- pulumi plugin ls" ]; then echo "yaml converter 1.38.7"; fi\ncase "$*" in *" pulumi convert "*) mkdir -p examples/nodejs examples/python examples/go examples/dotnet examples/java;; esac\n')
        self.command("make", 'exit 72\n')
        self.command("go", 'exit 0\n')
        self.env = os.environ.copy()
        self.env.update(PATH=f"{self.bin}:{self.env['PATH']}", CALL_LOG=str(self.log), HOME=str(self.root / "home"), TMPDIR=str(self.root / "ordinary-tmp"))
        (self.root / "home").mkdir()
        (self.root / "ordinary-tmp").mkdir()

    def tearDown(self):
        self.temp.cleanup()

    def command(self, name, body):
        path = self.bin / name
        path.write_text("#!/bin/sh\n" + body)
        path.chmod(0o755)
        return path

    def run_generator(self, **env):
        actual = self.env.copy()
        actual.update(env)
        return subprocess.run([str(SCRIPT), "make", str(self.repo), "project", "pack", "1.2.3"], env=actual, text=True, capture_output=True)

    def test_invalid_explicit_converter_fails_before_existing_outputs_are_deleted(self):
        result = self.run_generator(PULUMI_YAML_CONVERTER_BINARY=str(self.root / "missing"))
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue((self.output / "preserve.txt").exists())
        self.assertIn("configured", result.stderr)

    def test_explicit_converter_takes_precedence_and_installs_pinned_version(self):
        converter = self.root / "custom converter"
        converter.write_text("stub")
        converter.chmod(0o755)
        result = self.run_generator(PULUMI_YAML_CONVERTER_BINARY=str(converter))
        self.assertEqual(result.returncode, 72, result.stderr)
        calls = self.log.read_text()
        self.assertIn(f"--file {converter} --exact --reinstall", calls)
        self.assertNotIn(str(self.root / "home" / ".pulumi"), calls)

    def test_cached_binary_from_alternate_home_is_used_without_global_writes(self):
        home = self.root / "alternate"
        converter = home / ".pulumi/plugins/converter-yaml-v1.38.7/pulumi-converter-yaml"
        converter.parent.mkdir(parents=True)
        converter.write_text("stub")
        converter.chmod(0o755)
        result = self.run_generator(HOME=str(home), PULUMI_HOME="")
        self.assertEqual(result.returncode, 72, result.stderr)
        self.assertIn(f"--file {converter} --exact --reinstall", self.log.read_text())
        self.assertEqual(sorted(str(p.relative_to(home)) for p in home.rglob("*") if p.is_file()), [".pulumi/plugins/converter-yaml-v1.38.7/pulumi-converter-yaml"])

    def test_absent_cache_uses_pinned_public_install_fallback(self):
        result = self.run_generator()
        self.assertEqual(result.returncode, 72, result.stderr)
        calls = self.log.read_text()
        self.assertIn("plugin install converter yaml 1.38.7 --exact", calls)
        self.assertNotIn("plugin ls", calls.split("plugin install", 1)[0])
        self.assertFalse((self.root / "home/.pulumi").exists())

    def test_ordinary_tmpdir_is_honored_via_fake_mktemp(self):
        # Fake mktemp observes the user-selected parent but allocates only inside
        # the approved harness workspace.
        observed = self.root / "mktemp-args"
        self.command("mktemp", f'printf "%s\\n" "$*" > "{observed}"\nmkdir "{self.root}/isolated"\nprintf "%s\\n" "{self.root}/isolated"\n')
        result = self.run_generator()
        self.assertEqual(result.returncode, 72, result.stderr)
        self.assertIn(str(self.root / "ordinary-tmp"), observed.read_text())

    def test_sdk_contract_fixtures_survive_real_example_replacement(self):
        repo = self.root / "generation-repo"
        (repo / "examples/yaml").mkdir(parents=True)
        (repo / "examples/yaml/README.md").write_text("canonical yaml README\n")
        contracts = repo / "examples/sdk-contracts"
        expected = {}
        for language in ("go", "nodejs", "python", "dotnet", "java"):
            marker = contracts / language / "nested" / "fixture.txt"
            marker.parent.mkdir(parents=True)
            marker.write_text(f"maintained-{language}-fixture\n")
            expected[marker.relative_to(repo)] = marker.read_bytes()
        for language, output in {
            "typescript": "nodejs", "python": "python", "go": "go", "csharp": "dotnet", "java": "java"
        }.items():
            canonical = repo / "examples" / output
            canonical.mkdir(parents=True)
            (canonical / ".generated-contract-test").write_text(f"stale-{language}\n")

        self.command("make", "exit 0\n")
        self.command("npm", "exit 0\n")
        self.command("go", "exit 0\n")
        self.command("python3", "exit 0\n")
        self.command("mise", '''
if [ "$*" = "exec pulumi@3.259.0 -- pulumi plugin ls" ]; then
  echo "yaml converter 1.38.7"
fi
if echo "$*" | grep -q " pulumi convert "; then
  language=
  output=
  previous=
  for argument in "$@"; do
    if [ "$previous" = language ]; then language=$argument; fi
    if [ "$previous" = output ]; then output=$argument; fi
    case "$argument" in --language) previous=language ;; --out) previous=output ;; *) previous= ;; esac
  done
  case "$language" in typescript) directory=nodejs ;; csharp) directory=dotnet ;; *) directory=$language ;; esac
  mkdir -p "examples/$directory"
  printf 'generated-%s\\n' "$language" > "examples/$directory/.generated-contract-test"
fi
''')

        result = subprocess.run(
            [str(SCRIPT), "make", str(repo), "project", "pack", "1.2.3"],
            env=self.env,
            text=True,
            capture_output=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        for relative_path, contents in expected.items():
            self.assertTrue((repo / relative_path).is_file(), relative_path)
            self.assertEqual((repo / relative_path).read_bytes(), contents, relative_path)
        expected_outputs = {
            "typescript": "nodejs",
            "python": "python",
            "go": "go",
            "csharp": "dotnet",
            "java": "java",
        }
        for language, directory in expected_outputs.items():
            generated = repo / "examples" / directory / ".generated-contract-test"
            self.assertEqual(generated.read_text(), f"generated-{language}\n")


if __name__ == "__main__":
    unittest.main()
