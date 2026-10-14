import importlib.util
from pathlib import Path
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("normalize-converted-examples.py")
SPEC = importlib.util.spec_from_file_location("normalize_converted_examples", SCRIPT)
NORMALIZER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(NORMALIZER)


class NormalizeGoOptionalInputsTests(unittest.TestCase):
    def test_converted_optional_string_args_match_generated_sdk_pointer_types(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            sdk = root / "sdk"
            sdk.mkdir()
            (sdk / "optional.go").write_text(
                "type GiteaIntegrationArgs struct {\n"
                " Optional pulumi.StringPtrInput\n"
                " Secret pulumi.StringPtrInput\n"
                " Config pulumi.StringPtrInput\n"
                " Required pulumi.StringInput\n"
                "}\n"
            )
            example = root / "main.go"
            example.write_text(
                "args := &dokploy.GiteaIntegrationArgs{\n"
                ' Optional: pulumi.String("workspace"),\n'
                ' Secret: pulumi.ToSecret(token).(pulumi.StringOutput),\n'
                ' Config: configValue,\n'
                ' Required: pulumi.String("required"),\n'
                "}\n"
            )

            NORMALIZER.normalize_go_optional_inputs(sdk, example)
            normalized = example.read_text()
            self.assertIn('Optional: pulumi.StringPtr("workspace")', normalized)
            self.assertIn(
                "Secret: pulumi.ToSecret(pulumi.StringPtr(token)).(pulumi.StringPtrOutput)",
                normalized,
            )
            self.assertIn("Config: pulumi.StringPtr(configValue)", normalized)
            self.assertIn('Required: pulumi.String("required")', normalized)

            NORMALIZER.normalize_go_optional_inputs(sdk, example)
            self.assertEqual(example.read_text(), normalized)


if __name__ == "__main__":
    unittest.main()
