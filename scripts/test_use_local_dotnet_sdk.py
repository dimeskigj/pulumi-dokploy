import unittest

from use_local_dotnet_sdk import local_sdk_reference


class LocalSDKReferenceTest(unittest.TestCase):
    def test_replaces_published_package_regardless_of_version(self):
        for version in ("0.3.1", "0.0.1-alpha.0+dev"):
            with self.subTest(version=version):
                project = (
                    '<PackageReference Include="Pulumi" Version="3.*" />\n'
                    f'<PackageReference Include="Dimeskigj.Pulumi.Dokploy" Version="{version}" />\n'
                )
                self.assertEqual(
                    local_sdk_reference(project),
                    '<PackageReference Include="Pulumi" Version="3.*" />\n'
                    '<ProjectReference Include="../../sdk/dotnet/Dimeskigj.Pulumi.Dokploy.csproj" />\n',
                )

    def test_rejects_unrecognized_project_instead_of_using_remote_sdk(self):
        with self.assertRaises(ValueError):
            local_sdk_reference('<PackageReference Include="SomethingElse" Version="1" />')


if __name__ == "__main__":
    unittest.main()
