# Live acceptance fix verification — 2026-09-15

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
| `mise exec -- golangci-lint run` | FAIL | 8 existing lint findings: 1 goconst, 3 gosec, 4 unused |
| `git diff --check` | PASS | no whitespace errors |

The direct `env -u` form was used for static tests so mise-injected live configuration could not enable acceptance tests.

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
