import unittest
from pathlib import Path


class SDKContractRunnerTests(unittest.TestCase):
    def test_make_examples_runs_all_language_contracts(self):
        makefile = Path(__file__).resolve().parents[1] / "Makefile"
        target = makefile.read_text().split("test_examples:", 1)[1].split("\ntest:", 1)[0]
        self.assertIn("test_sdk_contracts", target)

    def test_contracts_are_outside_generated_example_directories(self):
        root = Path(__file__).resolve().parents[1] / "examples" / "sdk-contracts"
        self.assertEqual({p.name for p in root.iterdir() if p.is_dir()}, {"go", "nodejs", "python", "dotnet", "java"})

    def test_example_generator_does_not_replace_contract_fixtures(self):
        generator = Path(__file__).resolve().parents[1] / "scripts" / "gen-examples.sh"
        self.assertNotIn("sdk-contracts", generator.read_text())
