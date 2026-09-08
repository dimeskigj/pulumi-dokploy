# Pulumi Registry Readiness Assessment

Date: 2026-09-08

## Summary

Pulumi Dokploy has completed the repository-side Registry preparation work. It
has provider-specific request identification, plugin download metadata and
keywords, Registry documentation, the complete Apache License 2.0 text,
pull-request schema compatibility checking, release checksums/SBOMs/attestations,
and a manual exact-version release smoke workflow.

It is **not yet Registry-ready for publication**. A corrected release must be
published and the required maintainer inputs and Registry publication steps
must still be completed and verified.

## Completed repository work

- Provider-specific `User-Agent` with development fallback and retry coverage.
- `pluginDownloadURL` and Registry discovery keywords in the provider schema and
  regenerated SDK metadata.
- `docs/_index.md` and `docs/installation-configuration.md` with installation,
  configuration, examples, and community-maintenance status.
- Complete canonical Apache License 2.0 text.
- Pull-request-only schema compatibility checking.
- Release SHA256 checksums, archive SBOMs, and GitHub OIDC build-provenance
  attestations for stable and prerelease provider artifacts.
- Manual exact-version provider and five-language release smoke workflow,
  including fresh Pulumi homes and provider archive checksum verification.

## Explicitly deferred or still required

These items are intentionally not represented as complete:

- Logo and `logoUrl`.
- Security/private reporting contact.
- Maintainer/publisher display-name mapping.
- CODEOWNERS/governance ownership.
- Code of Conduct enforcement contact.
- Registry PR and release publication.
- Lookup functions and broad engine acceptance.

The Registry PR and release smoke workflow require a real corrected public
release. The workflow has not been dispatched against one. Do not describe the
provider as Registry-ready until the required maintainer inputs are supplied,
the corrected release exists, and publication has been verified.

## Task 6 residuals and release-smoke limitations

- A real public release dispatch was not run; static workflow and semantic tests
  do not replace dispatch evidence.
- Pulumi CLI downloads used by the workflow are not checksum-verified.
- Java package and runtime behavior remains unverified because no real public
  release dispatch was run.
- The workflow's static tests validate its contract but cannot prove package
  availability, plugin acquisition, or language-runtime behavior.

## Production correctness and acceptance scope

Backup creation now uses bounded post-create identity discovery and safe error
messages, with regression coverage. Direct provider and live API tests remain
substantial, but broad Pulumi engine lifecycle acceptance is still deferred.
The existing engine smoke coverage does not establish acceptance for all
workloads, databases, domains, credentials, mounts, and backups, including
import, refresh, replacement, secrets, and interrupted operations.

## Verification evidence

Passed:

- `gofmt` on all Task 7-listed Go files and `git diff --check`.
- `make test_provider`.
- `make docs_check` (website checks/build passed; existing three high-severity
  npm advisories and the existing missing `src/icons` warning remain out of
  scope).
- Pinned Pulumi 3.259.0 schema regeneration and OpenAPI v2.8.0 drift check.
- Pinned golangci-lint v2.9.0 equivalent after correcting a task-related G306
  fixture finding.
- Pinned `govulncheck` v1.1.4 equivalent: no reachable vulnerabilities.
- Pinned `go-licenses` v1.6.0 equivalent.
- Pinned GoReleaser v2.18.1 validation for both release configurations.

Not available or not fully passing locally:

- `mise` is unavailable, so the Makefile wrappers `make test_race`,
  `make check_codegen`, `make check_openapi`, `make govulncheck`, and
  `make license` could not run as written. Pinned equivalents were run where
  available; the OpenAPI and security/license checks passed.
- The pinned `go test -race ./provider/... ./internal/...` suite had one
  timing-sensitive failure in `TestBackupCreateDeadlineErrorOmitsTargetID`
  because its scripted server retained one poll request. The test passed in
  repeated non-race runs and was not changed because this Task 7 ledger work
  does not broaden into unrelated race-test repair.
- No live Dokploy acceptance or public release smoke dispatch was run; explicit
  credentials and release opt-in were not available.

Website npm advisories, logo/contacts/governance gaps, and other deferred items
are not fixed by this task.
