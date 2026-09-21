# Lint Fix Report

## Status

Implemented the requested lint fixes only:

- Replaced the literal volume backup service type with `volumeBackupApplicationService`.
- Changed registry documentation fixture `WriteFile` permissions to `0o600`.
- Removed the unused direct application and Compose cleanup helpers.

## Verification

- `gofmt -w provider/volume_backup.go provider/registry_docs_test.go provider/live_workloads_test.go` — passed.
- `env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider` — passed (`ok`, 36.437s).
- `mise exec -- golangci-lint run` — passed (`0 issues`).
- `git diff --check` — passed (no output).

Live tests were not run. No stop marker was removed.
