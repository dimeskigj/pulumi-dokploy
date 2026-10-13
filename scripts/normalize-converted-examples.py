#!/usr/bin/env python3
import re
from pathlib import Path


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
