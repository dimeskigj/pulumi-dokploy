#!/bin/sh
set -eu

make_command=$1
repository_root=$2
project=$3
pack=$4
version=$5
temporary_parent=${TMPDIR:-/tmp}

if ! cache=$(mktemp -d "$temporary_parent/pulumi-dokploy-examples.XXXXXX"); then
	echo "unable to create isolated Pulumi home under TMPDIR" >&2
	exit 1
fi

cleanup() {
	rm -rf "$cache"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

export PULUMI_HOME=$cache
unset PULUMI_HOME_OWNER_TOKEN
cd "$repository_root"
"$make_command" --no-print-directory install_plugin PULUMI_HOME="$cache" VERSION_GENERIC="$version"

rm -rf examples/nodejs examples/python examples/go examples/dotnet examples/java
for language in typescript python go csharp java; do
	case "$language" in
		typescript) output=nodejs ;;
		csharp) output=dotnet ;;
		*) output=$language ;;
	esac
	mise exec pulumi@3.259.0 -- pulumi convert --from yaml --language "$language" --cwd examples/yaml --out "../$output" --generate-only
done
mise exec -- go test ./examples -tags=yaml -run TestGeneratedServerExamplesInstantiateAndExportIdentity -count=1
go mod edit -require="$project/sdk/go/$pack@v0.0.0" -replace="$project/sdk/go/$pack=../../sdk/go/$pack" examples/go/go.mod
(cd examples/go && go mod tidy)
python3 -c 'from pathlib import Path; import sys; p=Path("examples/go/go.mod"); s=p.read_text(); s=s.replace(" => " + str(Path.cwd() / "sdk/go" / sys.argv[1]), " => ../../sdk/go/" + sys.argv[1]); p.write_text(s)' "$pack"
(cd examples/nodejs && npm pkg set dependencies.@dimeskigj/pulumi-dokploy=file:../../sdk/nodejs)
printf '%s\n' '-e ../../sdk/python' > examples/python/requirements.txt
python3 -c 'from pathlib import Path; p=Path("examples/dotnet/dokploy-mvp.csproj"); p.write_text(p.read_text().replace('\''<PackageReference Include="Dimeskigj.Pulumi.Dokploy" Version="0.0.1-alpha.0+dev" />'\'', '\''<ProjectReference Include="../../sdk/dotnet/Dimeskigj.Pulumi.Dokploy.csproj" />'\''))'
python3 -c 'from pathlib import Path; p=Path("examples/java/pom.xml"); p.write_text(p.read_text().replace("<groupId>com.dimeskigj</groupId>", "<groupId>net.dimeski.pulumi</groupId>"))'
python3 -c 'from pathlib import Path; p=Path("examples/java/pom.xml"); p.write_text(p.read_text().replace("<maven.compiler.source>11</maven.compiler.source>", "<maven.compiler.source>17</maven.compiler.source>").replace("<maven.compiler.target>11</maven.compiler.target>", "<maven.compiler.target>17</maven.compiler.target>").replace("<maven.compiler.release>11</maven.compiler.release>", "<maven.compiler.release>17</maven.compiler.release>"))'
python3 -c 'import re; from pathlib import Path; p=Path("examples/java/src/main/java/generated_program/App.java"); s=p.read_text().replace("com.dimeskigj.dokploy", "net.dimeski.pulumi.dokploy").replace("config.requireObject(\"dokploy:endpoint\", com.pulumi.core.TypeShape.map(String.class, Object.class))", "config.require(\"dokploy:endpoint\")").replace("config.requireObject(\"dokploy:apiKey\", com.pulumi.core.TypeShape.map(String.class, Object.class))", "config.requireSecret(\"dokploy:apiKey\")"); s=re.sub(r'\''config\.getSecret\("(\w+)"\)\.orElse\("([^"]*)"\)'\'', r'\''config.getSecret("\1").applyValue(v -> v.orElse("\2"))'\'', s); p.write_text(s)'
python3 -c 'import re; from pathlib import Path; p=Path("examples/dotnet/Program.cs"); s=re.sub(r'\''config\.GetSecret\("(\w+)"\) \?\? "([^"]*)"'\'', r'\''config.GetSecret("\1") ?? Output.CreateSecret("\2")'\'', p.read_text()); p.write_text(s)'
python3 -c 'from pathlib import Path; [Path(x).write_text(chr(10).join(line.rstrip() for line in Path(x).read_text().splitlines()).rstrip()+chr(10)) for x in ["examples/dotnet/Program.cs", "examples/java/pom.xml"]]'
python3 website/scripts/normalize-examples.py
for language in nodejs python go dotnet java; do
	cp examples/yaml/README.md "examples/$language/README.md"
done
