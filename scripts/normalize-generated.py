#!/usr/bin/env python3
"""Normalize canonical schema metadata and generated SDK source whitespace."""

import json
from pathlib import Path
import re
import sys


def normalize_schema(path: Path) -> None:
    schema = json.loads(path.read_text())
    schema.pop("version", None)
    path.write_text(json.dumps(schema, indent=2, ensure_ascii=False) + "\n")


def normalize_sdks(root: Path) -> None:
    for path in sorted(root.rglob("*")):
        if path.is_file() and path.suffix in {".cs", ".go", ".java", ".py", ".ts"}:
            content = path.read_text()
            normalized = "\n".join(line.rstrip() for line in content.splitlines()).rstrip() + "\n"
            if path.suffix == ".java" and path.stem.startswith("Notification"):
                # Pulumi's Notification Java emitter mixes leading spaces before a tab.
                normalized = re.sub(r"(?m)^ +\t", "\t", normalized)
            if normalized != content:
                path.write_text(normalized)


def main() -> None:
    if len(sys.argv) != 3 or sys.argv[1] not in {"schema", "sdk"}:
        raise SystemExit("usage: normalize-generated.py {schema|sdk} <path>")
    path = Path(sys.argv[2])
    if sys.argv[1] == "schema":
        normalize_schema(path)
    else:
        if not path.is_dir():
            raise SystemExit(f"SDK directory does not exist: {path}")
        normalize_sdks(path)


if __name__ == "__main__":
    main()
