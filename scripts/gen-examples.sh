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

if [ -n "$(ls -A "$cache")" ]; then
	echo "new Pulumi home is not empty" >&2
	exit 1
fi
owner_token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
if [ -z "$owner_token" ]; then
	echo "unable to create Pulumi home ownership token" >&2
	exit 1
fi
printf '%s' "$owner_token" > "$cache/.pulumi-dokploy-example-owner"

PULUMI_HOME=$cache PULUMI_HOME_OWNER_TOKEN=$owner_token "$make_command" --no-print-directory -C "$repository_root" gen_examples_in_cache PULUMI_HOME="$cache" PULUMI_HOME_OWNER_TOKEN="$owner_token"
