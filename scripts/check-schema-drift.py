#!/usr/bin/env python3
"""Check generated schema drift, disregarding only its top-level version."""

import json
import subprocess
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check-schema-drift.py <generated-schema>", file=sys.stderr)
        return 2
    path = Path(sys.argv[1]).resolve()
    root = subprocess.check_output(["git", "-C", str(path.parent), "rev-parse", "--show-toplevel"], text=True).strip()
    relative = path.relative_to(root)
    committed = json.loads(subprocess.check_output(["git", "-C", root, "show", f":{relative.as_posix()}"]))
    generated = json.loads(path.read_text())
    generated.pop("version", None)
    if "version" in committed:
        print("Committed schema must not pin a version", file=sys.stderr)
        return 1
    if generated != committed:
        print("Generated schema differs from the committed schema beyond version", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
