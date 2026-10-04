#!/bin/sh
set -eu

make_command=$1
repository_root=$2
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

PULUMI_HOME=$cache "$make_command" --no-print-directory -C "$repository_root" gen_examples_in_cache PULUMI_HOME="$cache"
