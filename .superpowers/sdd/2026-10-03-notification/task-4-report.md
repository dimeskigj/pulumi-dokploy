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

## Fix round 1 (review of `80dea15`)

- Confirmed negative SMTP ports previously passed projection; `TestNotificationIdentityValidation/negative_SMTP_port` was RED under `MISE_ENV=opencode-safe mise exec -- go test ./provider ./internal/client -run 'TestNotification(ObservedStateAllChannels|IdentityValidation|DiscoveryAllChannels|CustomHeadersPresenceRoundTrip)' -count=1` (provider failed, decoder passed). Changed only the email projection lower bound to require 1–65535.
- `TestNotificationObservedStateAllChannels` now compares the complete state for every channel against distinct expected values (all selected fields, credentials, priority/port numeric values, nonempty headers/routing, four true and four false events, and no unrelated state). `TestNotificationNullableRoutingAndHeaders` covers explicit null canonicalization and matching candidates.
- `TestNotificationIdentityValidation` now decodes a complete inline foreign/nested ID mismatch, a null event, an unsupported relation, and a negative SMTP port. `TestNotificationDiscoveryAllChannels` verifies all twelve complete canonical candidates and rejects a deliberately omitted setting per channel. `TestNotificationCustomHeadersPresenceRoundTrip` confirms generated omitted/null/empty/populated map presence survives decoding and re-encoding. These tests close all four Important findings and the minor follow-up.
- GREEN: `MISE_ENV=opencode-safe mise exec -- go test ./provider ./internal/client -run 'TestNotification(ObservedStateAllChannels|IdentityValidation|DiscoveryAllChannels|NullableRoutingAndHeaders|CustomHeadersPresenceRoundTrip)' -count=1`: both pass.
- `MISE_ENV=opencode-safe mise exec -- make test_provider`: pass; `MISE_ENV=opencode-safe mise exec -- make test_race`: pass; `MISE_ENV=opencode-safe mise exec -- make provider`: pass. `MISE_ENV=opencode-safe mise exec -- make lint`: failed on only the pre-existing, unrelated `provider/mount.go:299` goconst warning. `git diff --check`: pass.
- Self-review: only the two test files, email projection, and this report changed; unrelated untracked safe-mise overlay preserved unstaged. No schema/source-model changes or live tests. Remaining risk is the same Task 5 lifecycle sanitization/not-found boundary and lint warning in unrelated mount code.
