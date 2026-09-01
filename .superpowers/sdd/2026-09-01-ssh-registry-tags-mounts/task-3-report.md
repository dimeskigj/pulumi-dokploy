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
