# Task 3 Report: Container Registry Resource

## Status

Complete. Added the `dokploy:index:Registry` resource and registered it with the provider.

## Implementation

- Added `RegistryArgs` and `RegistryState` with required registry credentials/URL and nullable optional `imagePrefix` and `serverId`.
- Added validation for non-empty `name`, `username`, `password`, and `url`.
- Added update diffs for all inputs, including nullable clearing; name-only updates skip registry credential testing.
- Added registry credential testing before create and before relevant updates.
- Added cloud registry create/update/read/delete operations using generated API contracts.
- Preserved the prior password when the API omits it and handled not-found reads/deletes.
- Added current/prior password error sanitization and dependency wiring, including secret password output wiring.
- Registered the resource and updated provider/schema tests.

## TDD and verification

Red phase:

```text
mise exec -- go test ./provider -run 'TestRegistry|TestSchema' -count=1
```

Failed as expected because `RegistryArgs`, `RegistryState`, and `Registry` were undefined before implementation.

Focused verification:

```text
mise exec -- go test ./provider -run 'TestRegistry|TestSchema|TestProvider' -count=1
```

Passed: `ok github.com/dimeskigj/pulumi-dokploy/provider`.

Full provider verification:

```text
mise exec -- go test ./provider -count=1
```

Passed: `ok github.com/dimeskigj/pulumi-dokploy/provider`.

Diff verification:

```text
git diff --check
```

Passed with no output.

## Files changed

- `provider/registry.go`
- `provider/registry_test.go`
- `provider/provider.go`
- `provider/provider_test.go`
- `provider/schema_test.go`

## Concerns

None identified within Task 3 scope.

## Fix Round 1 Evidence

Addressed reviewer findings:

- Delete errors now pass through registry password sanitization, while 404 remains idempotent.
- Required-field validation now tests `name`, `username`, `password`, and `url` independently with exact empty strings; no whitespace rule was added.
- Added coverage for delete redaction/404, create and update API failures with current/prior password redaction, incomplete create/read responses, import, read errors containing the prior password, and post-create readback failures.
- Create now performs a readback after the API returns an ID. Readback errors or an empty read ID return `ResourceInitFailedError` with the populated partial state and ID.

Fix-round verification commands and outcomes:

```text
mise exec -- go test ./provider -run 'TestRegistry' -count=1
PASS: ok github.com/dimeskigj/pulumi-dokploy/provider

git diff --check
PASS: no output

mise exec -- go test ./provider -run 'TestRegistry|TestSchema|TestProvider' -count=1
PASS: ok github.com/dimeskigj/pulumi-dokploy/provider

mise exec -- go test ./provider -count=1
PASS: ok github.com/dimeskigj/pulumi-dokploy/provider
```

Self-review found no unrelated file changes or remaining Task 3 concerns.
