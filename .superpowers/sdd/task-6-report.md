# Task 6: Exact-version consumer smoke workflow

## Status

Implemented and committed as `ci: verify released provider packages`.

## Changes

- Added manual-only `.github/workflows/release-smoke.yml` with required SemVer
  input and separate provider, Node.js, Python, .NET, Java, and Go jobs.
- Each job uses explicit read-only contents permission, isolated temporary
  caches, exact public package coordinates, and no secrets or publication
  commands.
- Provider and SDK jobs perform provider plugin acquisition, schema loading, and
  minimal preview checks; the provider archive is checksum-verified.
- Extended exhaustive workflow policy and semantic regression tests.
- Documented dispatch timing and ecosystem-specific failure interpretation in
  `CONTRIBUTING.md`.

## Verification

- `go test ./provider -run 'Workflow|Smoke' -count=1` — PASS
- `go test -short ./provider/... ./internal/... -count=1` — PASS
- `git diff --check` — PASS

The workflow was statically validated by the repository's YAML-parsing
semantic tests. A real public-release dispatch was not run in this change.

## Reviewer follow-up

- Moved SemVer validation into an independent `validate-version` job. Consumer
  jobs use only its validated output through environment variables.
- Fixed checksum verification to run from the directory containing both the
  downloaded archive and `checksums.txt`.
- Removed explicit plugin installation from all SDK consumers. Each now runs a
  language-native provider program and relies on SDK metadata during preview,
  then asserts the matching plugin in its fresh `PULUMI_HOME`.
- `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/release-smoke.yml` — PASS

## Java/cache follow-up

- Replaced the Java Gradle consumer with a Maven `pom.xml`, executable
  `smoke.Main`, `exec-maven-plugin`, package validation, and Pulumi Java runtime
  options.
- Applied npm, pip, NuGet, Maven, Go, and Go module cache paths before each
  dependency-resolution step and again for preview.
- Added static checks for Maven runtime metadata, exact dependency coordinates,
  and cache placement; Java example metadata is checked without network access.
