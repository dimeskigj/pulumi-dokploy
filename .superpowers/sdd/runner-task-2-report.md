# Runner Task 2 Report: Workflow CLI And Provider Preflight

## RED/GREEN evidence

- RED: Added workflow contract assertions for the required step order, the
  exact Pulumi CLI command, and the local provider PATH command. Ran
  `go test ./provider -run TestOwnedWorkflow -count=1`; it failed because
  `Verify Pulumi CLI` was absent.
- GREEN: Added both workflow preflight steps. The focused contract test passed.

## Changes

- Added `Verify Pulumi CLI` immediately after `Setup Tools`, running
  `mise exec -- pulumi version`.
- Added `Expose local provider` immediately after `Build Provider`, appending
  `${{ github.workspace }}/bin` to `$GITHUB_PATH` without listing contents.
- Extended `TestOwnedWorkflow` to enforce the requested order and exact runs.

## Verification

```text
go test ./provider -run TestOwnedWorkflow -count=1
ok   github.com/dimeskigj/pulumi-dokploy/provider  0.035s

go test ./...
ok   github.com/dimeskigj/pulumi-dokploy/internal/client
ok   github.com/dimeskigj/pulumi-dokploy/openapi/cmd/normalize
ok   github.com/dimeskigj/pulumi-dokploy/provider  0.693s
ok   github.com/dimeskigj/pulumi-dokploy/tests

git diff --check
(no output; exit 0)
```

No live tests were run and no `.env` files were read.

## Commit

`ci: preflight Pulumi acceptance runtime` (final commit hash reported with the
task status).

## Concerns

None.
