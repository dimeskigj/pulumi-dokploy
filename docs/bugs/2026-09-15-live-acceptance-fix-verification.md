# Live acceptance fix verification — 2026-09-15

This isolated-branch report is the substitute for the unavailable untracked
parent report `docs/bugs/2026-09-14-live-acceptance-run.md`. That absent parent
report was not fabricated or updated.

## Revision and safety

- Revision: `ece9ab2` (`test: harden generated domain cleanup comparison`).
- Preflight confirmed `DOKPLOY_ACCEPTANCE`, `DOKPLOY_ENDPOINT`, and `DOKPLOY_API_KEY` were set without printing their values.
- Authenticated `settings.health` preflight returned HTTP 200.
- A new stop-marker path was selected and was absent before the run. Existing marker state was not modified.

## Static verification

| Command | Result | Duration / note |
| --- | --- | --- |
| `env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider ./tests -run 'LiveTargetReadiness\|ValidateLiveTarget\|DispatchFixture\|DomainComparison\|DomainCreateRequestKeys\|LiveDiagnosticSourceContract' -count=1 -v` | PASS | 0.055s provider; 0.036s tests |
| `env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test -short -count=1 ./provider/... ./internal/... ./tests/...` | PASS | 46.901s provider; 0.032s internal/client; 0.044s tests |
| `env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test -race ./provider/... ./internal/...` | FAIL | `TestBackupCreate_Cancellation`: one scripted request remained; focused rerun reproduced intermittently (1/5 pass) |
| `mise exec -- golangci-lint run` | FAIL | Initial run observed 8 unresolved findings: 1 goconst, 3 gosec, 4 unused |
| `git diff --check` | PASS | no whitespace errors |

The direct `env -u` form was an intentional safety deviation: mise loads live
credentials, so static tests used `env -u DOKPLOY_ACCEPTANCE -u
DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ...` to prevent live execution.

## Focused live verification

The PostgreSQL dispatch command was run first, serially:

```text
DOKPLOY_ACCEPTANCE_STOP_FILE=<new-marker-path> mise exec -- go test ./provider -run '^TestLiveTier2Workloads/MountDispatch/postgres$' -parallel=1 -count=1 -v
```

Result: **FAIL** after 47.47s (test body 46.85s), with sanitized diagnostic `operation=mount;status=transport;code=unknown`. Cleanup/health handling created the new stop marker. The marker was confirmed present afterward.

Per the safety gate, Domain application and compose diagnostics were **not run**. Therefore no Domain ownership classification was produced; `provider-serialization-mismatch`, `server-contract-rejection`, and `environment-or-health-failure` remain unclassified for this run.

## Final state and concerns

- New marker: present.
- Prior marker: not configured/present in the verification shell; no prior marker was removed or altered.
- No production files were changed.
- The static race suite and lint suite remain concerns. The race failure was reproduced in the focused cancellation test, while the short non-race suite passed.

## Review follow-up (no live or race rerun)

- Removed the branch-caused unused helpers `runLiveDomainCreateAttempt` and
  `mountServiceType`. No deterministic coverage depended on either helper.
- The two removed unused-helper findings are classified as branch-caused and
  fixed in this round.
- The six remaining unresolved lint findings are classified as pre-existing at
  base `0a98621`: one goconst, three gosec, and two unused helpers.
- Race attribution remains unconfirmed because the race suite was not rerun
  after the fixes. The unchanged `provider/backup_test.go` cancellation test
  produced the earlier failure, but an unchanged file alone is not conclusive.
- Focused deterministic tests were rerun after the deletions: PASS.
- `mise exec -- golangci-lint run` was rerun after the deletions: FAIL with
  exactly six unresolved findings, matching the pre-existing classification.
- `git diff --check`: PASS.
- The live stop marker was preserved; no live tests or marker operations were
  performed during this follow-up.
