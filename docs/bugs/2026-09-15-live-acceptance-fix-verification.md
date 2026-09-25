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

## Follow-up verification (2026-09-25)

The results above are historical. Revision `154ed6e` includes deterministic
backup polling tests, safe live diagnostics and cleanup gates, Git readback
normalization, stable Pulumi preview IDs, the gRPC security update, and a
PostgreSQL-deploy-only 90-second HTTP deadline. It does not retry deploy POSTs.

| Check | Result |
| --- | --- |
| `env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test -count=1 -timeout=240s ./...` | PASS |
| `env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test -race -count=1 -timeout=300s ./provider/... ./internal/... ./tests/...` | PASS |
| `env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY mise exec -- go test ./examples -tags=all -count=1` | PASS |
| `make check_openapi && make check_codegen` | PASS; SDK tree clean; no generated .NET PNG |
| `mise exec -- golangci-lint run` | PASS; 0 issues |
| `env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY make govulncheck` | PASS; no reachable vulnerabilities |

`make test_examples` and `make docs_check` also passed after the .NET icon
removal and before the final PostgreSQL client change. The Java example built
with pinned JDK/Maven and TLS certificate validation enabled.

Live tests were run serially with fresh absent stop-marker paths. Both focused
Domain paths passed. Tier 1, Tier 3, Tier 4, and the Pulumi Automation smoke
passed; only documented optional cases without dedicated prerequisites skipped.
Tier 2 passed in full on multiple runs, including Domain, Git source, and
PostgreSQL mount dispatch. A later Tier 2 repeat still failed intermittently:
the PostgreSQL mount update and readback succeeded, but the synchronous deploy
POST returned this classification after the former 30-second client deadline:

```text
operation=mount-dispatch;phase=mount-update;status=5xx;code=unknown;step=redeploy;stepStatus=failed;redeployStep=deploy
```

No fresh stop marker was created in these follow-up runs. Two older incident
markers were preserved. Their historical resource ownership cannot be
independently verified because their IDs were not retained; the operator
explicitly authorized fresh-marker testing on this disposable server despite
that uncertainty.

The HTTP 5xx is a server or intermediary response, not evidence that retrying
the state-changing POST is safe. Resolving that last intermittent failure
requires a sanitized server/proxy status classification and deploy-handler or
capacity evidence. Go-module caching is enabled for CI SDK/test matrix jobs,
but a hosted workflow run is still needed to confirm whether it eliminates
the intermittent `proxy.golang.org` download failure.

## Interrupted follow-up (2026-09-25)

A later serial Tier 2 run began at 07:15:19 UTC and was externally aborted
while the Redis dispatch subtest was running. It produced no test result, so
the run is neither a pass nor a failure attribution. No provider test process
or fresh stop marker remained afterward, and an authenticated
`settings.health` check returned 2xx. Because the process was interrupted,
test-owned resource cleanup from that run cannot be confirmed.

The supplied Docker logs include service-not-found errors but no timestamps or
handler correlation for the earlier failing PostgreSQL deploy POST. They do
not establish whether that 5xx was caused by a missing service, unrelated
cleanup activity, or another server/proxy error. Do not retry or change the
state-changing POST on that evidence alone.
