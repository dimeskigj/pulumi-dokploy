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
            java = root / "java" / "Notification.java"
            java.parent.mkdir()
            java.write_text("/**\n * \n */\nclass Schedule {}  \n    \t  this.value = defaults.value;\n\n")
            generated_java = root / "java" / "ApplicationBuild.java"
            generated_java.write_text("class ApplicationBuild {\n    \t      this.value = defaults.value;\n}\n")
            generated_source = root / "java" / "GitHubAppSource.java"
            generated_source.write_text("class GitHubAppSource {\n    \t      this.value = defaults.value;\n}\n")
            python = root / "schedule.py"
            python.write_text("VERSION = '0.0.1-alpha.0+dev'\n\n")
            metadata = root / "package.json"
            metadata.write_text('{"version":"0.0.1-alpha.0+dev"}\n')
            module.normalize_sdks(root)
            self.assertEqual(java.read_text(), "/**\n *\n */\nclass Schedule {}\n\t  this.value = defaults.value;\n")
            self.assertEqual(generated_java.read_text(), "class ApplicationBuild {\n\t      this.value = defaults.value;\n}\n")
            self.assertEqual(generated_source.read_text(), "class GitHubAppSource {\n\t      this.value = defaults.value;\n}\n")
            self.assertEqual(python.read_text(), "VERSION = '0.0.1-alpha.0+dev'\n")
            first = {path: path.read_bytes() for path in [java, generated_java, generated_source, python, metadata]}
            module.normalize_sdks(root)
            self.assertEqual({path: path.read_bytes() for path in first}, first)
            self.assertEqual(json.loads(metadata.read_text())["version"], "0.0.1-alpha.0+dev")

    def test_makefile_normalizes_schema_after_versioned_sdk_generation(self):
        source = Path(__file__).resolve().parents[1].joinpath("Makefile").read_text()
        codegen = source.split("codegen: provider\n", 1)[1].split("\nbuild_go:", 1)[0]
        self.assertIn("normalize-generated.py schema", codegen)
        self.assertLess(codegen.index("pulumi package gen-sdk"), codegen.index("normalize-generated.py schema"))
        self.assertIn("normalize-generated.py integration-inputs", codegen)
        self.assertGreater(codegen.index("normalize-generated.py integration-inputs"),
                           codegen.index("pulumi package gen-sdk"))
        self.assertIn("normalize-generated.py sdk sdk", codegen)
        self.assertLess(codegen.index("normalize-generated.py integration-inputs"),
                        codegen.index("normalize-generated.py sdk sdk"))
        generate = source.split("generate_schema: provider\n", 1)[1].split("\ngenerate_go", 1)[0]
        self.assertIn("normalize-generated.py schema", generate)

    def test_secret_integration_inputs_are_normalized_from_schema(self):
        module = self.load_normalizer()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            java = root / "java" / "src" / "main" / "java" / "net" / "dimeski" / "pulumi" / "dokploy"
            java.mkdir(parents=True)
            (java / "GiteaIntegrationArgs.java").write_text(
                "import com.pulumi.core.Output;\n"
                "import com.pulumi.core.annotations.Import;\n"
                "public class GiteaIntegrationArgs {\n"
                "    private Output<String> clientSecret;\n"
                "    private Output<String> giteaInternalUrl;\n"
                "    private Output<String> clientId;\n"
                "    public GiteaIntegrationArgs build() {\n"
                "        return $;\n"
                "    }\n"
                "}\n"
            )
            (java / "BitbucketIntegrationArgs.java").write_text(
                "import com.pulumi.core.Output;\n"
                "import com.pulumi.core.annotations.Import;\n"
                "public class BitbucketIntegrationArgs {\n"
                "    private Output<String> apiToken;\n"
                "    private Output<String> appPassword;\n"
                "    private Output<String> name;\n"
                "    public BitbucketIntegrationArgs build() {\n"
                "        return $;\n"
                "    }\n"
                "}\n"
            )
            node = root / "nodejs"
            node.mkdir()
            (node / "giteaIntegration.ts").write_text(
                'resourceInputs["clientSecret"] = args?.clientSecret ? pulumi.secret(args.clientSecret) : undefined;\n'
                'resourceInputs["giteaInternalUrl"] = args?.giteaInternalUrl ? pulumi.secret(args.giteaInternalUrl) : undefined;\n'
            )
            (node / "bitbucketIntegration.ts").write_text(
                'resourceInputs["apiToken"] = args?.apiToken ? pulumi.secret(args.apiToken) : undefined;\n'
                'resourceInputs["appPassword"] = args?.appPassword ? pulumi.secret(args.appPassword) : undefined;\n'
            )
            schema = {
                "resources": {
                    "dokploy:index:GiteaIntegration": {
                        "inputProperties": {
                            "clientSecret": {"type": "string", "secret": True},
                            "giteaInternalUrl": {"type": "string", "secret": True},
                            "clientId": {"type": "string"},
                        }
                    },
                    "dokploy:index:BitbucketIntegration": {
                        "inputProperties": {
                            "apiToken": {"type": "string", "secret": True},
                            "appPassword": {"type": "string", "secret": True},
                            "name": {"type": "string"},
                        }
                    },
                }
            }
            schema_path = root / "schema.json"
            schema_path.write_text(json.dumps(schema))

            module.normalize_secret_integration_inputs(schema_path, root)

            gitea = (java / "GiteaIntegrationArgs.java").read_text()
            bitbucket = (java / "BitbucketIntegrationArgs.java").read_text()
            self.assertIn('Codegen.stringProp("clientSecret").secret().arg($.clientSecret).getNullable()', gitea)
            self.assertIn('Codegen.stringProp("giteaInternalUrl").secret().arg($.giteaInternalUrl).getNullable()', gitea)
            self.assertIn('Codegen.stringProp("apiToken").secret().arg($.apiToken).getNullable()', bitbucket)
            self.assertIn('Codegen.stringProp("appPassword").secret().arg($.appPassword).getNullable()', bitbucket)
            self.assertNotIn('Codegen.stringProp("clientId").secret()', gitea)
            self.assertNotIn('Codegen.stringProp("name").secret()', bitbucket)
            self.assertLess(gitea.index("import com.pulumi.core.annotations.Import;"),
                            gitea.index("import com.pulumi.core.internal.Codegen;"))
            self.assertIn('args?.giteaInternalUrl !== undefined ? pulumi.secret(args.giteaInternalUrl)',
                          (node / "giteaIntegration.ts").read_text())
            generated_targets = list(java.glob("*.java")) + list(node.glob("*.ts"))
            first = {path: path.read_bytes() for path in generated_targets}
            module.normalize_secret_integration_inputs(schema_path, root)
            self.assertEqual({path: path.read_bytes() for path in generated_targets}, first)

    def test_secret_normalization_fails_closed_for_missing_generated_target(self):
        module = self.load_normalizer()
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            schema_path = root / "schema.json"
            schema_path.write_text(json.dumps({"resources": {"dokploy:index:GiteaIntegration": {
                "inputProperties": {"clientSecret": {"type": "string", "secret": True}}
            }}}))
            with self.assertRaisesRegex(ValueError, "GiteaIntegrationArgs.java"):
                module.normalize_secret_integration_inputs(schema_path, root)


if __name__ == "__main__":
    unittest.main()
