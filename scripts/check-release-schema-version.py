#!/usr/bin/env python3
"""Reject a release whose tagged schema advertises a different version."""

import json
import subprocess
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check-release-schema-version.py <release-version>", file=sys.stderr)
        return 2
    root = Path(__file__).resolve().parent.parent
    schema = json.loads(subprocess.check_output(
        ["git", "show", ":provider/cmd/pulumi-resource-dokploy/schema.json"], cwd=root
    ))
    actual = schema.get("version")
    if actual is not None and actual != sys.argv[1]:
        print(f"Tagged schema version {actual!r} does not match release {sys.argv[1]!r}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
