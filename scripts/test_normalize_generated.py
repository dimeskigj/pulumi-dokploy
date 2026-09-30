"""Regression tests for canonical generated artifacts."""

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


class NormalizeGeneratedTests(unittest.TestCase):
    def load_normalizer(self):
        path = Path(__file__).with_name("normalize-generated.py")
        self.assertTrue(path.exists(), "generation normalizer must exist")
        spec = importlib.util.spec_from_file_location("normalize_generated", path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def test_canonical_schema_removes_only_version_and_is_idempotent(self):
        module = self.load_normalizer()
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "schema.json"
            schema = {"name": "dokploy", "version": "0.0.1-alpha.0+dev", "resources": {}}
            path.write_text(json.dumps(schema, indent=2) + "\n")
            module.normalize_schema(path)
            self.assertEqual(json.loads(path.read_text()), {"name": "dokploy", "resources": {}})
            first = path.read_bytes()
            module.normalize_schema(path)
            self.assertEqual(path.read_bytes(), first)

    def test_sdk_whitespace_normalization_preserves_version_and_is_idempotent(self):
        module = self.load_normalizer()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            java = root / "java" / "Schedule.java"
            java.parent.mkdir()
            java.write_text("/**\n * \n */\nclass Schedule {}  \n\n")
            python = root / "schedule.py"
            python.write_text("VERSION = '0.0.1-alpha.0+dev'\n\n")
            metadata = root / "package.json"
            metadata.write_text('{"version":"0.0.1-alpha.0+dev"}\n')
            module.normalize_sdks(root)
            self.assertEqual(java.read_text(), "/**\n *\n */\nclass Schedule {}\n")
            self.assertEqual(python.read_text(), "VERSION = '0.0.1-alpha.0+dev'\n")
            first = {path: path.read_bytes() for path in [java, python, metadata]}
            module.normalize_sdks(root)
            self.assertEqual({path: path.read_bytes() for path in first}, first)
            self.assertEqual(json.loads(metadata.read_text())["version"], "0.0.1-alpha.0+dev")

    def test_makefile_normalizes_schema_after_versioned_sdk_generation(self):
        source = Path(__file__).resolve().parents[1].joinpath("Makefile").read_text()
        codegen = source.split("codegen: provider\n", 1)[1].split("\nbuild_go:", 1)[0]
        self.assertIn("normalize-generated.py schema", codegen)
        self.assertLess(codegen.index("pulumi package gen-sdk"), codegen.index("normalize-generated.py schema"))
        self.assertIn("normalize-generated.py sdk sdk", codegen)
        generate = source.split("generate_schema: provider\n", 1)[1].split("\ngenerate_go", 1)[0]
        self.assertIn("normalize-generated.py schema", generate)


if __name__ == "__main__":
    unittest.main()
