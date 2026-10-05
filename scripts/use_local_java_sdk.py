"""Align the converted Maven example with the locally generated Java SDK."""

import re
import sys
from pathlib import Path


DEPENDENCY = re.compile(
    r'(<dependency>\s*<groupId>)(?:com\.dimeskigj|net\.dimeski\.pulumi)'
    r'(</groupId>\s*<artifactId>dokploy</artifactId>\s*<version>)[^<]+'
    r'(</version>\s*</dependency>)'
)


def local_sdk_version(project: str, version: str) -> str:
    result, count = DEPENDENCY.subn(lambda m: m[1] + "net.dimeski.pulumi" + m[2] + version + m[3], project)
    if count != 1:
        raise ValueError("expected exactly one Dokploy dependency in converted Java example")
    return result


if __name__ == "__main__":
    path = Path(sys.argv[1])
    path.write_text(local_sdk_version(path.read_text(), sys.argv[2]))
