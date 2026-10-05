import unittest

from use_local_java_sdk import local_sdk_version


class LocalJavaSDKVersionTest(unittest.TestCase):
    def test_converted_package_uses_local_version(self):
        project = '<dependency><groupId>com.dimeskigj</groupId><artifactId>dokploy</artifactId><version>0.3.1</version></dependency>'
        self.assertEqual(
            local_sdk_version(project, "0.0.1-alpha.0+dev"),
            '<dependency><groupId>net.dimeski.pulumi</groupId><artifactId>dokploy</artifactId><version>0.0.1-alpha.0+dev</version></dependency>',
        )

    def test_missing_dependency_fails(self):
        with self.assertRaises(ValueError):
            local_sdk_version("<dependencies/>", "0.0.1-alpha.0+dev")
