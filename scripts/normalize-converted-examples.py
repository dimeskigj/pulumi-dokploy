#!/usr/bin/env python3
import re
from pathlib import Path


def normalize_go_optional_inputs(sdk_root: Path, example_path: Path) -> None:
    """Make converted Go literals honor optional generated StringPtrInput args."""
    optional_fields = {}
    for sdk_path in sdk_root.glob("*.go"):
        sdk_source = sdk_path.read_text()
        for match in re.finditer(r"(?ms)^type\s+(\w+Args)\s+struct\s*\{(.*?)^\}", sdk_source):
            if match.group(1) not in {"GiteaIntegrationArgs", "BitbucketIntegrationArgs"}:
                continue
            fields = re.findall(r"(?m)^\s*(\w+)\s+pulumi\.StringPtrInput\b", match.group(2))
            if fields:
                optional_fields[match.group(1)] = set(fields)

    source = example_path.read_text()
    for args_type, fields in optional_fields.items():
        literal = re.compile(
            rf"(?ms)(?P<prefix>&(?:dokploy\.)?{re.escape(args_type)}\{{\n)"
            rf"(?P<body>.*?)(?P<suffix>^\s*\}})"
        )

        def normalize_literal(match):
            lines = match.group("body").splitlines()
            changed = False
            for index, line in enumerate(lines):
                field = re.match(r"^(\s*)(\w+)(:\s*)(.*?)(,?\s*)$", line)
                if not field or field.group(2) not in fields:
                    continue
                indent, name, separator, value, trailing = field.groups()
                if re.fullmatch(r"pulumi\.String\(.*\)", value):
                    value = "pulumi.StringPtr" + value[len("pulumi.String"):]
                elif re.fullmatch(r"pulumi\.ToSecret\(.*\)\.\(pulumi\.StringOutput\)", value):
                    secret = re.fullmatch(r"pulumi\.ToSecret\((.*)\)\.\(pulumi\.StringOutput\)", value)
                    value = f"pulumi.ToSecret(pulumi.StringPtr({secret.group(1)})).(pulumi.StringPtrOutput)"
                elif re.fullmatch(r"[A-Za-z_]\w*", value):
                    value = f"pulumi.StringPtr({value})"
                elif "pulumi.StringPtr(" in value:
                    pass
                else:
                    raise ValueError(f"unsupported converted optional Go input: {args_type}.{name}: {value}")
                lines[index] = f"{indent}{name}{separator}{value}{trailing}"
                changed = True
            if changed:
                return match.group("prefix") + "\n".join(lines) + "\n" + match.group("suffix")
            return match.group(0)

        source, count = literal.subn(normalize_literal, source)
        if count > 1:
            raise ValueError(f"multiple converted Go literals found for {args_type}")
    example_path.write_text(source)


def main() -> None:
    java = Path("examples/java/src/main/java/generated_program/App.java")
    source = java.read_text().replace("com.dimeskigj.dokploy", "net.dimeski.pulumi.dokploy")
    source = source.replace(
        'config.requireObject("dokploy:endpoint", com.pulumi.core.TypeShape.map(String.class, Object.class))',
        'config.require("dokploy:endpoint")',
    ).replace(
        'config.requireObject("dokploy:apiKey", com.pulumi.core.TypeShape.map(String.class, Object.class))',
        'config.requireSecret("dokploy:apiKey")',
    )
    source = re.sub(
        r'config\.getSecret\("(\w+)"\)\.orElse\("([^"]*)"\)',
        r'config.getSecret("\1").applyValue(v -> v.orElse("\2"))',
        source,
    )
    java.write_text("\n".join(line.rstrip() for line in source.splitlines()).rstrip() + "\n")

    dotnet = Path("examples/dotnet/Program.cs")
    source = re.sub(
        r'config\.GetSecret\("(\w+)"\) \?\? "([^"]*)"',
        r'config.GetSecret("\1") ?? Output.CreateSecret("\2")',
        dotnet.read_text(),
    )
    dotnet.write_text("\n".join(line.rstrip() for line in source.splitlines()).rstrip() + "\n")

    normalize_go_optional_inputs(Path("sdk/go/dokploy"), Path("examples/go/main.go"))

    pom = Path("examples/java/pom.xml")
    source = pom.read_text().replace(
        "<maven.compiler.source>11</maven.compiler.source>",
        "<maven.compiler.source>17</maven.compiler.source>",
    ).replace(
        "<maven.compiler.target>11</maven.compiler.target>",
        "<maven.compiler.target>17</maven.compiler.target>",
    ).replace(
        "<maven.compiler.release>11</maven.compiler.release>",
        "<maven.compiler.release>17</maven.compiler.release>",
    )
    pom.write_text("\n".join(line.rstrip() for line in source.splitlines()).rstrip() + "\n")


if __name__ == "__main__":
    main()
