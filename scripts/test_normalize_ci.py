import importlib.util
import hashlib
import tempfile
import unittest
from pathlib import Path


SPEC = importlib.util.spec_from_file_location("normalize_ci", Path(__file__).with_name("normalize_ci.py"))
NORMALIZE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(NORMALIZE)


class ExactTransformationTests(unittest.TestCase):
    def assert_fails(self, text):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture"
            path.write_text(text)
            with self.assertRaises(SystemExit):
                NORMALIZE.exact_once(path, "OLD", "NEW")

    def test_old_state_is_replaced(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture"
            path.write_text("OLD")
            NORMALIZE.exact_once(path, "OLD", "NEW")
            self.assertEqual(path.read_text(), "NEW")

    def test_normalized_state_is_accepted(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "fixture"
            path.write_text("NEW")
            NORMALIZE.exact_once(path, "OLD", "NEW")
            self.assertEqual(path.read_text(), "NEW")

    def test_mixed_state_fails(self):
        self.assert_fails("OLD NEW")

    def test_duplicate_old_state_fails(self):
        self.assert_fails("OLD OLD")

    def test_duplicate_new_state_fails(self):
        self.assert_fails("NEW NEW")

    def test_drifted_state_fails(self):
        self.assert_fails("DRIFT")

    def test_unrelated_text_is_not_a_valid_transformation(self):
        self.assert_fails("OLD\nunrelated OLD-looking content")


class TriggerFixtureTests(unittest.TestCase):
    def test_paths_ignore_block_is_removed_only_from_push_trigger(self):
        fixture = NORMALIZE.BUILD_PUSH_WITH_PATHS_IGNORE
        normalized = NORMALIZE.normalize_build_trigger(fixture)
        self.assertNotIn("paths-ignore:", normalized)
        self.assertIn("    tags-ignore:\n    - v*\n    - sdk/*\n", normalized)
        self.assertNotIn('    - "**"\n', normalized)
        self.assertEqual(normalized.count("  push:\n"), 1)
        self.assertEqual(normalized.count("paths-ignore:"), 0)

    def test_unrelated_paths_ignore_text_is_not_modified(self):
        with self.assertRaises(SystemExit):
            NORMALIZE.normalize_build_trigger("    paths-ignore:\n    - CHANGELOG.md\n")

    def test_normalized_trigger_is_accepted(self):
        normalized = NORMALIZE.BUILD_PUSH_NORMALIZED
        self.assertEqual(NORMALIZE.normalize_build_trigger(normalized), normalized)

    def test_broad_tag_ignore_is_not_an_accepted_normalized_state(self):
        with self.assertRaises(SystemExit):
            NORMALIZE.normalize_build_trigger(
                NORMALIZE.BUILD_PUSH_NORMALIZED.replace(
                    "    - sdk/*\n", '    - sdk/*\n    - "**"\n'
                )
            )


class GateCommandTests(unittest.TestCase):
    def test_multiline_fail_fast_script_has_exact_executable_commands(self):
        script = """run: |
  set -e
  # make lint is documented here, but is not executable
  make lint
  mise exec -- make lint
"""
        self.assertEqual(
            NORMALIZE.executable_run_lines(script),
            ["make lint", "mise exec -- make lint"],
        )

    def test_mentions_and_dead_conditionals_do_not_satisfy_exact_gate(self):
        for script in (
            "run: echo make lint",
            "run: printf '%s' 'make lint'",
            "run: command=\"make lint\"",
            "run: # make lint",
            "run: if false; then make lint; fi",
            "run: make lint --unrelated",
        ):
            with self.subTest(script=script):
                with self.assertRaises(SystemExit):
                    NORMALIZE.require_exact_run_values(script, ("make lint",))

    def test_gate_requirements_are_explicit_and_exact(self):
        fixture = """- name: lint
  run: |
    set -o pipefail
    make lint
- name: unrelated
  run: echo make lint
"""
        NORMALIZE.require_exact_run_values(fixture, ("make lint",))

    def test_multiline_dead_conditional_does_not_satisfy_exact_gate(self):
        with self.assertRaises(SystemExit):
            NORMALIZE.require_exact_run_values("run: |\n  if false; then\n    make lint\n  fi\n", ("make lint",))


class WorkflowPolicyTests(unittest.TestCase):
    def test_pages_workflow_is_explicitly_allowed_with_two_non_go_jobs(self):
        self.assertIn("pages.yml", NORMALIZE.GENERATED_WORKFLOW_NAMES)
        self.assertEqual(
            NORMALIZE.WORKFLOW_JOB_POLICY["pages.yml"],
            {"build": False, "deploy": False},
        )
        fixture = """jobs:
  build:
    steps:
    - run: npm ci --prefix website
  deploy:
    steps:
    - uses: actions/deploy-pages@sha
"""
        NORMALIZE.validate_workflow_jobs(
            "pages.yml", fixture, {"build": False, "deploy": False}
        )

    def test_docs_check_is_an_expected_build_gate(self):
        self.assertIn("make docs_check", NORMALIZE.EXPECTED_BUILD_GATES)

    def test_workflow_policy_requires_complete_filename_set(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            workflows = root / ".github" / "workflows"
            workflows.mkdir(parents=True)
            for name in NORMALIZE.GENERATED_WORKFLOW_NAMES[:-1]:
                (workflows / name).write_text("name: fixture\njobs:\n")
            with self.assertRaises(SystemExit):
                NORMALIZE.validate_workflow_policy(root)

    def test_workflow_policy_rejects_yaml_workflows(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            workflows = root / ".github" / "workflows"
            workflows.mkdir(parents=True)
            for name in NORMALIZE.GENERATED_WORKFLOW_NAMES:
                (workflows / name).write_text("name: fixture\njobs:\n")
            (workflows / "unexpected.yaml").write_text("name: fixture\njobs:\n")
            with self.assertRaises(SystemExit):
                NORMALIZE.validate_workflow_policy(root)

    def test_non_go_job_cannot_use_go_setup_or_make(self):
        fixture = """jobs:
  non_go:
    steps:
    - uses: ./.github/actions/setup-tools
    - run: echo safe
"""
        with self.assertRaises(SystemExit):
            NORMALIZE.validate_workflow_jobs("fixture.yml", fixture, {"non_go": False})

    def test_non_go_job_cannot_run_go_command(self):
        fixture = """jobs:
  non_go:
    steps:
    - run: go test ./...
"""
        with self.assertRaises(SystemExit):
            NORMALIZE.validate_workflow_jobs("fixture.yml", fixture, {"non_go": False})

    def test_unknown_job_and_stale_pin_fail_policy(self):
        fixture = """env:
  GOVERSION: "1.21.x"
jobs:
  unknown:
    steps: []
"""
        with self.assertRaises(SystemExit):
            NORMALIZE.validate_workflow_jobs("fixture.yml", fixture, {})

    def test_go_job_requires_exact_current_version(self):
        fixture = """env:
  GOVERSION: "1.25.13"
jobs:
  go_job:
    steps:
    - run: make test
"""
        NORMALIZE.validate_workflow_jobs("fixture.yml", fixture, {"go_job": True})

    def test_each_go_job_requires_its_own_version_pin(self):
        fixture = """jobs:
  go_job:
    env:
      GOVERSION: "1.25.13"
    steps:
    - run: make test
  missing_pin:
    steps:
    - run: go test ./...
"""
        with self.assertRaises(SystemExit):
            NORMALIZE.validate_workflow_jobs("fixture.yml", fixture, {"go_job": True, "missing_pin": True})

    def test_docs_gate_insertion_requires_one_prerequisites_marker(self):
        fixture = """jobs:
  prerequisites:
    steps:
    - name: Check OpenAPI and generated SDKs
      run: make check_openapi && make check_codegen
    - name: Check OpenAPI and generated SDKs
      run: make check_openapi && make check_codegen
"""
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "build.yml"
            path.write_text(fixture)
            with self.assertRaises(SystemExit):
                NORMALIZE.insert_in_job(
                    path,
                    "prerequisites",
                    "    - name: Check OpenAPI and generated SDKs\n",
                    "    - name: Check documentation\n      run: make docs_check\n",
                )


class GeneratedMiseSafetyTests(unittest.TestCase):
    MISE_TOML = """# WARNING: This file is autogenerated - changes will be overwritten when regenerated by https://github.com/pulumi/ci-mgmt
# You can create your own root-level mise.toml file to override/augment this. See https://mise.jdx.dev/configuration.html

[env]
_.vfox-pulumi = { module_path = "provider" } # Sets GO_VERSION_MISE and PULUMI_VERSION_MISE
PULUMI_HOME = "{{config_root}}/.pulumi"

[tools]

# Runtimes
go = "{{ env.GO_VERSION_MISE }}"
node = '24.18.0'
python = '3.11.15'
"vfox:version-fox/vfox-dotnet" = "8.0.20" # vfox backend doesn't work on Windows, gives "error converting Lua table to PreInstall (no version returned from vfox plugin)" https://github.com/jdx/mise/discussions/5876 https://github.com/jdx/mise/discussions/5550
# Corretto version used as Java SE/OpenJDK version no longer offered
java = 'corretto-11'

# Executable tools
"github:pulumi/pulumi" = "{{ env.PULUMI_VERSION_MISE }}"
"github:pulumi/pulumictl" = '0.0.50'
"github:pulumi/schema-tools" = "0.8.1"
"go:github.com/pulumi/upgrade-provider" = "main"
"aqua:gradle/gradle-distributions" = '7.6.6'
golangci-lint = "2.12.2" # See note about about overrides if you need to customize this.
"npm:yarn" = "1.22.22"

[settings]
experimental = true # Required for Go binaries (e.g. pulumictl).
lockfile = false
http_retries = 3
pin = true # `mise use` should pin versions instead of defaulting to latest.
fetch_remote_versions_cache = "24h" # Mise queries versions even if they're pinned to confirm they exist. Reduce GitHub API calls by doing that less often.

[plugins]
vfox-pulumi = "https://github.com/pulumi/vfox-pulumi"
"""
    MISE_TEST_TOML = """# WARNING: This file is autogenerated - changes will be overwritten when regenerated by https://github.com/pulumi/ci-mgmt

[tools]
"aqua:gotestyourself/gotestsum" = "1.12.0"
"""

    def write_mise(self, root, names=("mise.toml", "mise.test.toml")):
        config = root / ".config"
        config.mkdir()
        for name in names:
            (config / name).write_text(
                self.MISE_TOML if name == "mise.toml" else self.MISE_TEST_TOML
            )

    def configure_normalizer(self, root):
        old_root = NORMALIZE.ROOT
        old_hashes = NORMALIZE.KNOWN_MISE_SHA256
        NORMALIZE.ROOT = root
        hashes = {
            "mise.toml": hashlib.sha256(self.MISE_TOML.encode()).hexdigest(),
            "mise.test.toml": hashlib.sha256(self.MISE_TEST_TOML.encode()).hexdigest(),
        }
        self.assertEqual(hashes["mise.toml"], "6e4d48dfc6ba5bc647f8de223f35044793ae73aea8ff52f5e989a1dd597f1b08")
        self.assertEqual(hashes["mise.test.toml"], "25d93429d74aefa3c4c59cafc6c33dbbd00b1a876714855eaaadc7a6c8ded0c9")
        NORMALIZE.KNOWN_MISE_SHA256 = hashes
        return old_root, old_hashes

    def test_valid_first_and_modified_second_delete_none(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_mise(root)
            second = root / ".config" / "mise.test.toml"
            second.write_text(second.read_text() + "modified\n")
            old_root, old_hashes = self.configure_normalizer(root)
            try:
                with self.assertRaises(SystemExit):
                    NORMALIZE.remove_validated_generated_mise()
                self.assertTrue((root / ".config" / "mise.toml").exists())
                self.assertTrue(second.exists())
            finally:
                NORMALIZE.ROOT = old_root
                NORMALIZE.KNOWN_MISE_SHA256 = old_hashes

    def test_missing_candidate_deletes_none(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_mise(root, ("mise.toml",))
            old_root, old_hashes = self.configure_normalizer(root)
            try:
                with self.assertRaises(SystemExit):
                    NORMALIZE.remove_validated_generated_mise()
                self.assertTrue((root / ".config" / "mise.toml").exists())
            finally:
                NORMALIZE.ROOT = old_root
                NORMALIZE.KNOWN_MISE_SHA256 = old_hashes

    def test_already_removed_mise_pair_is_idempotent(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".config").mkdir()
            old_root, old_hashes = self.configure_normalizer(root)
            try:
                NORMALIZE.remove_validated_generated_mise()
            finally:
                NORMALIZE.ROOT = old_root
                NORMALIZE.KNOWN_MISE_SHA256 = old_hashes

    def test_valid_pair_deletes_both(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.write_mise(root)
            old_root, old_hashes = self.configure_normalizer(root)
            try:
                NORMALIZE.remove_validated_generated_mise()
                self.assertFalse((root / ".config" / "mise.toml").exists())
                self.assertFalse((root / ".config" / "mise.test.toml").exists())
            finally:
                NORMALIZE.ROOT = old_root
                NORMALIZE.KNOWN_MISE_SHA256 = old_hashes


if __name__ == "__main__":
    unittest.main()
