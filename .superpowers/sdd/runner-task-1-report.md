# Acceptance Runner Task 1 Report

## Status

Implemented and committed as `build: pin Pulumi for acceptance tests`.

## Changes

- Added the fail-closed `requirePulumiCLI` acceptance prerequisite gate.
- Added table-driven tests for disabled, unavailable, and available Pulumi CLI cases.
- Pinned Pulumi `3.259.0` in `.mise.toml`.
- Documented that `mise install` installs the Pulumi CLI used by acceptance tests and code generation.

## Verification

- `go test ./tests -run 'TestPulumiCLIAvailabilityHelper|TestRequirePulumiCLI' -count=1` — RED first with the expected missing-function compilation failure.
- `go test ./tests -run 'TestPulumiCLIAvailabilityHelper|TestRequirePulumiCLI|TestLifecycleSmokeProgram' -count=1` — PASS.
- `go test ./...` — PASS.
- `git diff --check` — PASS.

## Concerns

- No live acceptance calls were made and no `.env` or credentials were used.
