# Task 4 report — Notification observation and discovery

## Implementation

- Added typed observation/projection for all twelve selected relations, strict foreign/nested ID and event presence checks, active-organization extraction, exact-ID read, unique list ID validation, and fail-closed marker candidate verification (complete settings and all events disabled).
- Generated nullable model presence distinguishes omitted required credentials (retain prior or empty on import) from explicit null/empty (clear); optional routing/map nulls canonicalize. Added generated decoder round-trip assertions and gofmt correction, closing both Task 1 follow-ups.
- No generated code, lifecycle, registration, or unrelated user work changed. Projections live in a dedicated `notification_projection.go` rather than the three adapter files; all twelve requested helper names/signatures are present.

## Files

`provider/notification_observation.go`, `provider/notification_projection.go`, `provider/notification_observation_test.go`, `internal/client/notification_contract_test.go`.

## RED / GREEN

- RED: `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(ObservedStateAllChannels|IdentityValidation|DiscoveryCandidates|MarkerConflictIsNotEmpty)' -count=1`: compile failed on missing observation/identity/candidate helpers as expected.
- GREEN: `MISE_ENV=opencode-safe mise exec -- go test ./provider ./internal/client -run 'TestNotification(ObservedStateAllChannels|IdentityValidation|DiscoveryCandidates|MarkerConflictIsNotEmpty|SecretPresence|CandidateSafety|ExactReadAndOrganization|NullablePresenceRoundTrip)' -count=1`: both packages pass.
- Exact-read fixture initially failed for missing JSON content type; corrected local fixture and reran successfully.

## Verification and self-review

- `MISE_ENV=opencode-safe mise exec -- make test_provider`: pass.
- `MISE_ENV=opencode-safe mise exec -- make test_race`: pass.
- `MISE_ENV=opencode-safe mise exec -- make provider`: pass.
- `MISE_ENV=opencode-safe mise exec -- make lint`: latest run passes (0 issues). Earlier runs intermittently reported a pre-existing `provider/mount.go:299` goconst warning outside assigned scope. Notification test-file formatting blocker is closed.
- `git diff --check`: pass.
- Reviewed scoped diff; unrelated untracked `mise.opencode-safe.local.toml` preserved unstaged. No live tests, API writes, codegen or generated edits.
- Residual risk: Task 5 must sanitize errors at lifecycle boundaries and handle HTTP not-found semantics. Offline fixtures do not prove behavior of deployed Dokploy.
