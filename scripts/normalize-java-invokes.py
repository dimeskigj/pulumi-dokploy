#!/usr/bin/env python3
"""Remove whitespace artifacts from Java invoke files emitted by gen-sdk.

Select files from schema function tokens, leaving managed-resource classes intact.
"""

import json
from pathlib import Path
import re
import sys


LEADING = re.compile(r"^[ \t]+")


def main() -> None:
    schema = json.loads(Path(sys.argv[1]).read_text())
    package = Path(sys.argv[2]) / "src/main/java/net/dimeski/pulumi/dokploy"
    files = {package / "DokployFunctions.java"}
    for token in schema["functions"]:
        name = token.rsplit(":", 1)[-1]
        if not name.startswith("get") or len(name) <= 3:
            raise SystemExit("unexpected Java invoke function token")
        generated_name = "G" + name[1:]
        files.update({package / "inputs" / f"{generated_name}Args.java",
                      package / "inputs" / f"{generated_name}PlainArgs.java",
                      package / "outputs" / f"{generated_name}Result.java"})
    for path in sorted(files):
        if not path.is_file():
            raise SystemExit(f"missing generated Java invoke: {path}")
    for path in sorted(files):
        content = path.read_text()
        lines = []
        for line in content.splitlines(keepends=True):
            stripped = line.rstrip(" \t\r\n")
            prefix = LEADING.match(stripped)
            if prefix:
                stripped = prefix.group().expandtabs(4) + stripped[prefix.end():]
            lines.append(stripped + ("\n" if line.endswith("\n") else ""))
        path.write_text("".join(lines))


if __name__ == "__main__":
    main()
