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
            if path.suffix == ".java" and (
                path.stem.startswith("Notification")
                or path.stem in {
                    "ApplicationBuild", "ApplicationSource", "BitbucketAppSource", "GitHubAppSource", "GiteaAppSource",
                }
            ):
                # Pulumi's Java emitter mixes leading spaces before a tab in generated builders.
                normalized = re.sub(r"(?m)^ +\t", "\t", normalized)
            if normalized != content:
                path.write_text(normalized)


def normalize_secret_integration_inputs(schema_path: Path, sdk_root: Path) -> None:
    """Restore secret/falsy input semantics omitted by current SDK emitters."""
    schema = json.loads(schema_path.read_text())
    resources = schema.get("resources")
    if not isinstance(resources, dict):
        raise ValueError("schema resources must be an object")

    for resource_name in ("GiteaIntegration", "BitbucketIntegration", "GitLabIntegration"):
        token = f"dokploy:index:{resource_name}"
        resource = resources.get(token)
        if not isinstance(resource, dict):
            raise ValueError(f"schema resource missing: {token}")
        properties = resource.get("inputProperties")
        if not isinstance(properties, dict):
            raise ValueError(f"schema inputProperties missing for {token}")
        secret_fields = [
            name for name, prop in properties.items()
            if isinstance(prop, dict) and prop.get("secret") is True
        ]
        if not secret_fields:
            raise ValueError(f"schema has no secret input properties for {token}")
        for name in secret_fields:
            if properties[name].get("type") != "string":
                raise ValueError(f"unsupported secret input type for {token}.{name}")

        java_path = sdk_root / "java" / "src" / "main" / "java" / "net" / "dimeski" / "pulumi" / "dokploy" / f"{resource_name}Args.java"
        if not java_path.is_file():
            raise ValueError(f"generated Java target missing: {java_path.name}")
        java = java_path.read_text()
        codegen_import = "import com.pulumi.core.internal.Codegen;\n"
        anchor = "import com.pulumi.core.annotations.Import;\n"
        if anchor not in java:
            raise ValueError(f"unexpected Java imports in {java_path.name}")
        java = java.replace(codegen_import, "", 1)
        java = java.replace(anchor, anchor + codegen_import, 1)

        build_match = re.search(r"public\s+" + re.escape(resource_name) + r"Args\s+build\(\)\s*\{([\s\S]*?)\n    \}", java)
        if not build_match:
            raise ValueError(f"generated build method missing in {java_path.name}")
        body = build_match.group(1)
        lines = []
        for name in secret_fields:
            if not re.search(r"private\s+(?:@Nullable\s+)?Output<String>\s+" + re.escape(name) + r"\s*;", java):
                raise ValueError(f"generated Java field missing: {resource_name}.{name}")
            assignment = f'$.{name} = Codegen.stringProp("{name}").secret().arg($.{name}).getNullable();'
            if assignment not in body:
                lines.append(assignment)
        if lines:
            return_match = re.search(r"(?m)^([ \t]*)return \$;", body)
            if not return_match:
                raise ValueError(f"generated build return shape changed in {java_path.name}")
            indent = return_match.group(1)
            replacement = "\n".join(indent + line for line in lines) + "\n" + indent + "return $;"
            body = body[:return_match.start()] + replacement + body[return_match.end():]
            java = java[:build_match.start(1)] + body + java[build_match.end(1):]
        java_path.write_text(java)

        node_path = sdk_root / "nodejs" / f"{resource_name[0].lower()}{resource_name[1:]}.ts"
        if not node_path.is_file():
            raise ValueError(f"generated Node.js target missing: {node_path.name}")
        node = node_path.read_text()
        for name in secret_fields:
            expected = f'resourceInputs["{name}"] = args?.{name} ? pulumi.secret(args.{name}) : undefined;'
            replacement = f'resourceInputs["{name}"] = args?.{name} !== undefined ? pulumi.secret(args.{name}) : undefined;'
            if replacement not in node:
                if expected not in node:
                    raise ValueError(f"generated Node.js secret input shape changed: {resource_name}.{name}")
                node = node.replace(expected, replacement, 1)
        node_path.write_text(node)


def main() -> None:
    if len(sys.argv) == 4 and sys.argv[1] == "integration-inputs":
        normalize_secret_integration_inputs(Path(sys.argv[2]), Path(sys.argv[3]))
        return
    if len(sys.argv) != 3 or sys.argv[1] not in {"schema", "sdk"}:
        raise SystemExit("usage: normalize-generated.py {schema|sdk} <path> | integration-inputs <schema> <sdk-root>")
    path = Path(sys.argv[2])
    if sys.argv[1] == "schema":
        normalize_schema(path)
    else:
        if not path.is_dir():
            raise SystemExit(f"SDK directory does not exist: {path}")
        normalize_sdks(path)


if __name__ == "__main__":
    main()
