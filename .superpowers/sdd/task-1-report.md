# Task 1 Report: Provider-Specific User-Agent

## Status

Implemented and committed on `feat/registry-release-readiness`.

## RED

Added client header tests for a versioned User-Agent and the development fallback.

Command:

```text
go test ./internal/client -run '^TestNew(SendsProviderUserAgent|UsesDevelopmentUserAgentFallback)$' -count=1
```

Result: expected failure during compilation because `WithUserAgentVersion` did not yet exist.

## GREEN

Implemented `WithUserAgentVersion` and `providerUserAgent` in `internal/client`, including trimming, one leading `v` removal, and the `dev` fallback. The request editor now sets `User-Agent: pulumi-dokploy/<version>`. Added retry-preservation coverage and passed `Version` from provider configuration.

Commands and results:

```text
go test ./internal/client -run 'UserAgent' -count=1
PASS

go test ./internal/client ./provider -run 'UserAgent|Config' -count=1
PASS

go test -short ./provider/... ./internal/... -count=1
PASS

go test ./internal/client ./provider -count=1
PASS

git diff --check
PASS
```

The supplied baseline warning about `provider/live_harness_unit_test.go` referencing undefined `classifyWorkloadCreateError` was not reproduced in this worktree; the focused provider and short package test commands both passed. That unrelated file was not modified.

## Commit

```text
e38fa0e feat: identify provider API requests
```

## Self-review

- Only the four Task 1 implementation/test files were changed in the feature commit.
- Endpoint and API-key validation paths remain unchanged.
- Retry attempts receive the same request editor and User-Agent.
- Provider configuration consumes linker-injected `Version` without introducing a provider dependency into `internal/client`.
- `git diff --check` passed.

## Concerns

No known Task 1 concerns. The reported unrelated baseline compile failure could not be observed with the required commands in this checkout.
