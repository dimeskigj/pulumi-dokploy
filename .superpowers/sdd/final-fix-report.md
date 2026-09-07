# Final Review Fix Report

## Status

PASS — both final review findings are fixed and verified.

## Fixes

- Backup snapshots now retain every non-empty `backupId`, even when another
  observation field is missing or malformed. Such observations are marked
  invalid and can never match a create request. A regression test proves an
  existing malformed record is not adopted when it later becomes complete.
- Target backup discovery failures now return the fixed operation-level error
  `backup.create could not read target backups`. Target IDs and API payload
  messages are not included. Cancellation and deadline errors remain wrapped
  so `errors.Is` continues to work.

## TDD Evidence

### RED

```text
go test ./provider -run '^TestBackupCreate(DoesNotAdoptMalformedPreExistingBackup|DiscoveryErrorOmitsTargetAndAPIMessage)$' -count=1
```

Failed as expected: the malformed pre-existing record was incorrectly adopted,
and discovery returned the previous incomplete/API error instead of the fixed
safe message.

### GREEN

```text
go test ./provider -run '^TestBackup(CreateDoesNotAdoptMalformedPreExistingBackup|DiscoveryErrorOmitsTargetAndAPIMessage|PostCreateDiscoveryErrorIsSafe|ObservationMatchesCreate)$' -count=1
```

Passed.

## Verification

- `go test ./provider -run '^TestBackup' -count=1` — PASS
- `go test -short ./provider/... ./internal/... -count=1` — PASS
- `go test -race ./provider/... ./internal/... -count=1` — PASS
- `git diff --check` — PASS

## Commit

`4ae4185 fix: harden backup discovery review findings`

## Concerns

No known concerns. Live Dokploy acceptance was not run; these fixes were
verified with the scripted provider tests and offline suites.
