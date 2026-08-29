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
        self.assertIn('    - "**"\n', normalized)

    def test_unrelated_paths_ignore_text_is_not_modified(self):
        with self.assertRaises(SystemExit):
            NORMALIZE.normalize_build_trigger("    paths-ignore:\n    - CHANGELOG.md\n")

    def test_normalized_trigger_is_accepted(self):
        normalized = NORMALIZE.BUILD_PUSH_NORMALIZED
        self.assertEqual(NORMALIZE.normalize_build_trigger(normalized), normalized)


class GeneratedMiseSafetyTests(unittest.TestCase):
    def test_modified_generated_mise_is_not_deleted(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = root / ".config"
            config.mkdir()
            path = config / "mise.toml"
            path.write_text("# generated\n[tools]\ngo = 'modified'\n")
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            old_root = NORMALIZE.ROOT
            old_hashes = NORMALIZE.KNOWN_MISE_SHA256
            NORMALIZE.ROOT = root
            NORMALIZE.KNOWN_MISE_SHA256 = {"mise.toml": "different", "mise.test.toml": "unused"}
            try:
                with self.assertRaises(SystemExit):
                    NORMALIZE.remove_validated_generated_mise()
                self.assertTrue(path.exists())
                self.assertNotEqual(digest, NORMALIZE.KNOWN_MISE_SHA256["mise.toml"])
            finally:
                NORMALIZE.ROOT = old_root
                NORMALIZE.KNOWN_MISE_SHA256 = old_hashes


if __name__ == "__main__":
    unittest.main()
