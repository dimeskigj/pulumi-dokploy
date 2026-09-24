#!/usr/bin/env python3
"""Remove gen-sdk's optional .NET package icon references, failing closed."""

from pathlib import Path
import re
import sys


PROJECT_ICON = re.compile(r"^\s*<PackageIcon>logo\.png</PackageIcon>\s*\n", re.MULTILINE)
LOGO_ITEM = re.compile(
    r"^\s*<ItemGroup>\s*\n"
    r"\s*<None Include=\"logo\.png\">\s*\n"
    r"\s*<Pack>True</Pack>\s*\n"
    r"\s*<PackagePath></PackagePath>\s*\n"
    r"\s*</None>\s*\n"
    r"\s*</ItemGroup>\s*\n",
    re.MULTILINE,
)
RESIDUAL = re.compile(r"(?:PackageIcon|logo\.png)")


def main() -> None:
    path = Path(sys.argv[1])
    content = path.read_text()
    content = PROJECT_ICON.sub("", content)
    content = LOGO_ITEM.sub("", content)
    if RESIDUAL.search(content):
        raise SystemExit(f"unexpected .NET logo reference remains in {path}")
    path.write_text(content)


if __name__ == "__main__":
    main()
