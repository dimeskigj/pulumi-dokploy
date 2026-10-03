"""Regression coverage for whitespace normalization of generated Java invokes."""

import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("normalize-java-invokes.py")


class NormalizeJavaInvokesTest(unittest.TestCase):
    def test_only_schema_function_files_are_normalized_and_repeatable(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            schema = root / "schema.json"
            schema.write_text(json.dumps({"functions": {
                "dokploy:index:getProject": {}, "dokploy:index:getMySQL": {},
            }}))
            java = root / "java"
            package = java / "src/main/java/net/dimeski/pulumi/dokploy"
            files = [package / "DokployFunctions.java"]
            for name in ("GetProject", "GetMySQL"):
                files += [package / "inputs" / f"{name}Args.java",
                          package / "inputs" / f"{name}PlainArgs.java",
                          package / "outputs" / f"{name}Result.java"]
            original = "    /**\n     * \n    \t  line\n    */\n"
            for path in files:
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(original)
            unrelated = package / "Project.java"
            unrelated.write_text(original)

            def run():
                return subprocess.run([sys.executable, str(SCRIPT), str(schema), str(java)],
                                      text=True, capture_output=True)

            first = run()
            self.assertEqual(first.returncode, 0, first.stderr)
            normalized = "    /**\n     *\n          line\n    */\n"
            for path in files:
                self.assertEqual(path.read_text(), normalized, str(path))
            self.assertEqual(unrelated.read_text(), original)
            second = run()
            self.assertEqual(second.returncode, 0, second.stderr)
            for path in files:
                self.assertEqual(path.read_text(), normalized, str(path))

    def test_missing_generated_function_file_fails_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            schema = root / "schema.json"
            schema.write_text(json.dumps({"functions": {"dokploy:index:getProject": {}}}))
            result = subprocess.run([sys.executable, str(SCRIPT), str(schema), str(root / "java")],
                                    text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("missing generated Java invoke", result.stderr)


if __name__ == "__main__":
    unittest.main()
