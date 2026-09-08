# Task 7 report

## Status

Ledger updated. The repository-side Registry preparation is complete, but the
provider is explicitly not marked Registry-ready because maintainer inputs, a
corrected public release, and Registry publication remain outstanding.

Task 6 residuals are recorded: no real public release dispatch, no checksum
verification for Pulumi CLI downloads, potentially unsupported Java runtime
options, and the limitation that static tests do not replace dispatch.

## Ledger changes

Completed entries now cover User-Agent, plugin download metadata and keywords,
Registry documentation, the complete Apache license, PR schema compatibility,
release checksums/SBOMs/attestations, and the exact-version release smoke
workflow.

Deferred entries explicitly cover logo/`logoUrl`, security/private reporting,
publisher display-name mapping, CODEOWNERS/governance, Code of Conduct contact,
Registry PR/release publication, lookup functions, and broad engine acceptance.

## Verification commands and outcomes

- `gofmt -w internal/client/client.go internal/client/client_test.go provider/config.go provider/config_test.go provider/provider.go provider/provider_test.go provider/schema_test.go provider/registry_docs_test.go provider/registry_metadata_test.go` — PASS.
- `git diff --check` — PASS.
- `make test_provider` — PASS.
- `go test -race ./provider/... ./internal/...` — NOT FULLY PASSING: one timing-sensitive scripted-server poll test failed (`TestBackupCreateDeadlineErrorOmitsTargetID` / another backup deadline variant on rerun). No unrelated race repair was made.
- `make docs_check` — PASS; existing website npm advisories and missing-icons warning remain out of scope.
- `make check_codegen` — unavailable because `mise` is not installed. Pinned Pulumi 3.259.0 schema regeneration matched the checked-in schema; generated SDK source drift was not found apart from expected Makefile-owned Go module metadata.
- `make check_openapi` — unavailable because `mise` is not installed. Direct pinned-equivalent normalization plus `oapi-codegen@v2.8.0` — PASS, no drift.
- `make lint` — unavailable because `golangci-lint` is not installed. `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run` — PASS after the minimal task-related fixture permission correction.
- `make govulncheck` — unavailable because `mise` is not installed. `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` — PASS; no reachable vulnerabilities.
- `make license` — unavailable because `mise` is not installed. `go run github.com/google/go-licenses@v1.6.0 check ./...` — PASS.
- `go run github.com/goreleaser/goreleaser/v2@v2.18.1 check -f .goreleaser.yml && go run github.com/goreleaser/goreleaser/v2@v2.18.1 check -f .goreleaser.prerelease.yml` — PASS.
- `go test ./provider -run 'Registry|Workflow|Schema|Metadata|Release' -count=1` — PASS.

## Concerns

- No live Dokploy acceptance or public release smoke dispatch was run because
  credentials and release opt-in were unavailable.
- Website advisories, logo/contacts/governance, and other deferred items were
  intentionally not changed.
