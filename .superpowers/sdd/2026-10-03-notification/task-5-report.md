# Task 5 implementation report

## Scope

- Added Notification Create/Read/Update/Delete in `provider/notification_lifecycle.go`, with a notification-only safe error boundary. Create snapshots IDs and checks an unpredictable marker, sends one disabled create, performs at most three post-create lists, verifies an exact-ID read, and only then finalizes and checks observed desired settings. A list-verified ID and observed state survive failed verification/finalization via `ResourceInitFailedError`; uncertainty never triggers a create replay or deletion. Read supports import and known-identity checks; update/delete verify active organization and exact relation before mutation; delete confirms exact-ID absence.
- Added `provider/notification_lifecycle_test.go` and `provider/notification_error_test.go`: all-channel lifecycle, dry-runs, discovery failures, verified partial state, import, in-place update, delete idempotence, Gotify/Ntfy threshold guard, and safe diagnostics.
- Extended `provider/notification_channel_test.go` to assert full per-channel endpoint payload and dispatch. This found Pushover creation sending null retry/expire instead of omission; fixed the create adapter in `provider/notification_channel_messaging.go` while retaining explicit null on update.

## TDD evidence

- RED: `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotificationSafeErrorCategories -count=1` failed because the sanitizer was absent; added fixed classification/status handling and cancellation identity.
- RED: `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(CreateAllChannels|CreateDryRunNoCalls|SafeErrorCategories)' -count=1` failed because Create was absent; GREEN after implementing guarded Create.
- RED: expanded full endpoint assertions via `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(ChannelEndpoints|SafeErrors|CreateAllChannels|ReadUpdateDelete)' -count=1` exposed Pushover create's unintended JSON nulls; GREEN after correcting create omission.
- RED: `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotificationReadIncompletePriorIdentity -count=1` showed a known incomplete identity reached the client; GREEN after rejecting it before client creation.
- `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotification -count=1` passed after the adapter and lifecycle changes.

## Verification

- `MISE_ENV=opencode-safe mise exec -- make test_provider`: passed (provider and internal client offline suite).
- `MISE_ENV=opencode-safe mise exec -- make test_race`: passed.
- `MISE_ENV=opencode-safe mise exec -- make provider`: passed.
- `MISE_ENV=opencode-safe mise exec -- make lint`: failed only at pre-existing unrelated `provider/mount.go:299` goconst warning; Notification lint findings were corrected. No unrelated mount edit.
- `git diff --check`: clean. No live acceptance or notification delivery was run.
- Resume check: `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotification -count=1` passed; staged diff and `git diff --cached --check` were reviewed. The untracked safe mise overlay remained unstaged.

## Boundaries / risks

Production registration, schema/SDK generation, and user documentation are separate assigned tasks. Offline scripted-server tests do not prove behavior of any deployed Dokploy server. The safe mise local overlay was pre-existing, untracked and left untouched. Task-specific SDD documents have been preserved pending explicit cleanup approval.
