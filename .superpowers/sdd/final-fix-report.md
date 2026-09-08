# Final broad-review fix report

Implemented on `fix/live-acceptance-findings` without live calls or `.env` use.

## Evidence

- Domain, Mount, and MountDispatch successful creates now register bounded
  fallback ownership immediately. Explicit deletion releases ownership before
  absence verification, preventing duplicate cleanup after verification errors.
- SSH key create performs one post-error list discovery. A single novel
  same-name key returns partial state and `initFailed`; no candidate returns the
  sanitized original create error; ambiguous discovery returns a sanitized
  ambiguity initialization error without an ID. Create is never retried.
- Workload call-path diagnostic tests use a scripted fake client and verify
  raw IDs, paths, SQL, and content do not appear in structural diagnostics.
- Tier 2 documentation records the final sanitized evidence and preserves the
  cleanup limitation and historical findings.

## Verification

- `go test ./provider -run 'TestSSHKeyCreate(Recovers|ReturnsOriginal|ReturnsAmbiguous)' -count=1` — PASS.
- `go test ./provider -run 'TestWorkloadCallPathsEmitOnlyStructuralDiagnostics' -count=1` — PASS.
- `go test ./provider -count=1` — PASS.
- `go test ./... -count=1` — PASS.
- `go test -race ./provider/... ./internal/... -count=1` — PASS.
- `make docs_check` — PASS (Astro check: 0 errors/warnings/hints; 44 docs tests and 2 built-site tests passed).
- `git diff --check` — PASS.

## Earlier concurrent main review evidence

- Backup snapshots retain every non-empty `backupId`, mark malformed observations
  invalid, and never adopt them on a later complete observation.
- Target-backup discovery failures use the fixed operation-level message
  `backup.create could not read target backups`; cancellation and deadline
  wrapping remains intact for `errors.Is`.
- The focused backup regression suite, short provider/internal suite, provider/
  internal race suite, and whitespace check passed. Live Dokploy acceptance was
  not run for those backup fixes.
