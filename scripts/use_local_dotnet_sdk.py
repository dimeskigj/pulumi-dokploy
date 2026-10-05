"""Use the generated local .NET SDK when compiling converted examples."""

import re
import sys
from pathlib import Path


PACKAGE_REFERENCE = re.compile(
    r'<PackageReference Include="Dimeskigj\.Pulumi\.Dokploy" Version="[^"]+" />'
)
PROJECT_REFERENCE = (
    '<ProjectReference Include="../../sdk/dotnet/Dimeskigj.Pulumi.Dokploy.csproj" />'
)


def local_sdk_reference(project: str) -> str:
    result, count = PACKAGE_REFERENCE.subn(PROJECT_REFERENCE, project)
    if count != 1:
        raise ValueError("expected exactly one Dokploy package reference in converted .NET example")
    return result


if __name__ == "__main__":
    path = Path(sys.argv[1])
    path.write_text(local_sdk_reference(path.read_text()))
