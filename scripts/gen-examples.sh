#!/bin/sh
set -eu

make_command=$1
repository_root=$2
project=$3
pack=$4
version=$5
temporary_parent=${TMPDIR:-/tmp}
yaml_converter_version=1.38.7
yaml_converter_override=${PULUMI_YAML_CONVERTER_BINARY:-}
source_pulumi_home=${PULUMI_HOME:-${HOME:-}/.pulumi}

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
# Resolve and install the pinned converter into the isolated home before any
# tracked translation directories are removed. Never discover/download latest.
if [ -n "$yaml_converter_override" ]; then
	if [ ! -x "$yaml_converter_override" ]; then
		echo "configured YAML converter override is not executable" >&2
		exit 1
	fi
	mise exec pulumi@3.259.0 -- pulumi plugin install converter yaml "$yaml_converter_version" --file "$yaml_converter_override" --exact --reinstall
else
	yaml_converter_binary=
	for candidate in \
		"$source_pulumi_home/plugins/converter-yaml-v$yaml_converter_version/pulumi-converter-yaml" \
		"$source_pulumi_home/plugins/converter-yaml-v$yaml_converter_version/pulumi-converter-yaml.exe"; do
		if [ -x "$candidate" ]; then yaml_converter_binary=$candidate; break; fi
	done
	if [ -n "$yaml_converter_binary" ]; then
		mise exec pulumi@3.259.0 -- pulumi plugin install converter yaml "$yaml_converter_version" --file "$yaml_converter_binary" --exact --reinstall
	else
		mise exec pulumi@3.259.0 -- pulumi plugin install converter yaml "$yaml_converter_version" --exact
	fi
fi
mise exec pulumi@3.259.0 -- pulumi plugin ls | grep -E "yaml[[:space:]]+converter[[:space:]]+$yaml_converter_version([[:space:]]|$)" >/dev/null || {
	echo "isolated Pulumi home does not contain YAML converter $yaml_converter_version" >&2
	exit 1
}
"$make_command" --no-print-directory install_plugin PULUMI_HOME="$cache" VERSION_GENERIC="$version"

# These Go regression fixtures live alongside generated program source and
# must survive replacement of the language output directory.
mkdir -p "$cache/go-tests"
for test_file in lookup_test.go notification_test.go; do
	if [ -f "examples/go/$test_file" ]; then
		cp "examples/go/$test_file" "$cache/go-tests/$test_file"
	fi
done
rm -rf examples/nodejs examples/python examples/go examples/dotnet examples/java
for language in typescript python go csharp java; do
	case "$language" in
		typescript) output=nodejs ;;
		csharp) output=dotnet ;;
		*) output=$language ;;
	esac
	mise exec pulumi@3.259.0 -- pulumi convert --from yaml --language "$language" --cwd examples/yaml --out "../$output" --generate-only
done
for test_file in "$cache"/go-tests/*.go; do
	[ -f "$test_file" ] || continue
	cp "$test_file" examples/go/
done
mise exec -- go test ./examples -tags=yaml -run TestGeneratedServerExamplesInstantiateAndExportIdentity -count=1
go mod edit -require="$project/sdk/go/$pack@v0.0.0" -replace="$project/sdk/go/$pack=../../sdk/go/$pack" examples/go/go.mod
(cd examples/go && go mod tidy)
python3 -c 'from pathlib import Path; import sys; p=Path("examples/go/go.mod"); s=p.read_text(); s=s.replace(" => " + str(Path.cwd() / "sdk/go" / sys.argv[1]), " => ../../sdk/go/" + sys.argv[1]); p.write_text(s)' "$pack"
(cd examples/nodejs && npm pkg set dependencies.@dimeskigj/pulumi-dokploy=file:../../sdk/nodejs)
printf '%s\n' '-e ../../sdk/python' > examples/python/requirements.txt
python3 scripts/use_local_dotnet_sdk.py examples/dotnet/dokploy-mvp.csproj
python3 scripts/use_local_java_sdk.py examples/java/pom.xml "$version"
python3 website/scripts/normalize-examples.py
python3 scripts/normalize-converted-examples.py
for language in nodejs python go dotnet java; do
	cp examples/yaml/README.md "examples/$language/README.md"
done
