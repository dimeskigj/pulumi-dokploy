# Task 2 Report

## Status

Complete.

## Summary

Pinned Dokploy's OpenAPI contract, selected the resolved 46-operation allowlist
(including `application.reload`), normalized the contract with deterministic
response/schema corrections, and generated the embedded Go client with
oapi-codegen v2.8.0. Added reproducible generation and stale-output checks.

## Files changed

- `openapi/upstream.json`, `source.json`, `operations.txt`, `corrections.json`,
  `oapi-codegen.yaml`, `dokploy.json`
- `openapi/cmd/normalize/main.go`, `main_test.go`
- `internal/client/generated/generated.gen.go`
- `Makefile`, `go.mod`, `go.sum`

## Red/green evidence

The required test was run before the normalizer implementation:

```text
mise exec -- go test ./openapi/cmd/normalize -count=1
undefined: normalizeFixture
undefined: responseSchema
undefined: normalize
undefined: contractWithout
undefined: corrections
FAIL
```

After implementation, the focused test passed:

```text
mise exec -- go test ./openapi/cmd/normalize -count=1
ok   github.com/gjorgjidimeski/pulumi-dokploy/openapi/cmd/normalize  0.391s
```

## Upstream provenance

- Repository: `https://github.com/Dokploy/dokploy`
- Commit: `cebd3808565ea9bed0791961bc25c60513d94c5a`
- URL: `https://raw.githubusercontent.com/Dokploy/dokploy/cebd3808565ea9bed0791961bc25c60513d94c5a/openapi.json`
- SHA-256: `931195dde4999f9b4dd9cf61cd93086eabeb8eab666bc1e63a00f47dded5bb1d`

## Generation and verification

`make generate_openapi` output:

```text
mise exec -- go run ./openapi/cmd/normalize -in openapi/upstream.json -out openapi/dokploy.json
mise exec -- go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config openapi/oapi-codegen.yaml openapi/dokploy.json
mise exec -- gofmt -w internal/client/generated/generated.gen.go openapi/cmd/normalize/main.go openapi/cmd/normalize/main_test.go
```

Exact generator command:

```text
mise exec -- go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config openapi/oapi-codegen.yaml openapi/dokploy.json
```

`make check_openapi` passed, including:

```text
git diff --exit-code -- openapi/dokploy.json internal/client/generated/generated.gen.go
```

Focused compile/test command passed:

```text
mise exec -- go test ./internal/client ./openapi/cmd/normalize -count=1
ok   github.com/gjorgjidimeski/pulumi-dokploy/internal/client
ok   github.com/gjorgjidimeski/pulumi-dokploy/openapi/cmd/normalize
```

Full test command passed:

```text
mise exec -- go test ./... -count=1
ok   github.com/gjorgjidimeski/pulumi-dokploy/internal/client
?    github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated [no test files]
ok   github.com/gjorgjidimeski/pulumi-dokploy/openapi/cmd/normalize
ok   github.com/gjorgjidimeski/pulumi-dokploy/provider
?    github.com/gjorgjidimeski/pulumi-dokploy/provider/cmd/pulumi-resource-dokploy [no test files]
```

The normalized contract contains 46 paths and the pinned upstream hash was
rechecked with `shasum -a 256 openapi/upstream.json`.

## Self-review

- Allowlist is sorted and contains only the resolved 46 operations.
- Normalization rejects absent and duplicate operations, preserves selected
  security metadata, recursively retains referenced schemas, and emits
  indented JSON with a trailing newline.
- Corrected response schemas include the required `CreateProjectResult` and
  `DeploymentStatus` definitions plus named resource schemas with permissive
  additional properties.
- Generated output compiles and stale generation produces no diff.
- `git diff --check` passed before commit; no unrelated Task 1 behavior changed.

## Commits

- `c4d500dfb0d4d4baaa325299235027a1b82efdae` — `build: generate focused Dokploy API client`

## Concerns

None.
