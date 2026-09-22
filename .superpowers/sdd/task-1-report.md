# Task 1 Report: Domain Creation Diagnostics

## Result

`DONE_WITH_CONCERNS`

The structural, sanitized comparison diagnostic was implemented and verified locally. The authorized live runs could not reach Domain creation because both target-readiness checks failed, so no deployed request contract evidence was obtained and no Domain contract repair was attempted.

## TDD evidence

### RED

Added `TestDomainComparisonEvidenceIsStructuralAndSanitized` first, then ran:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDomainComparisonEvidenceIsStructuralAndSanitized$' -count=1 -v
```

Result: expected build failure because `formatDomainComparisonEvidence` was undefined.

### GREEN

Implemented strict structural parsing and fixed invalid-evidence output. The formatter accepts only allowlisted paths, targets, status classes, API codes, and sorted request keys; it never interpolates field values or raw errors.

Focused verification:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run 'DomainComparison|DomainCreateRequestKeys|DomainCreateBody' -count=1 -v
```

Result: PASS.

Final focused verification:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider ./tests -run 'Domain|LiveDiagnosticSourceContract' -count=1
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDomainComparisonEvidenceIsStructuralAndSanitized$' -count=1 -v
```

Result: both commands PASS.

## Authorized live verification

Fresh marker paths were used and were absent before both runs:

```text
/tmp/opencode/domain-application-diagnostic.stop
/tmp/opencode/domain-compose-diagnostic.stop
```

Commands:

```text
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-application-diagnostic.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/application$' -parallel=1 -count=1 -v
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-compose-diagnostic.stop mise exec -- go test ./provider -run '^TestLiveTier2Workloads/Domain/compose$' -parallel=1 -count=1 -v
```

Both stopped at the sanitized target-readiness failure before Domain create, so neither produced comparison evidence. Neither fresh stop marker appeared afterward. No Domain create was reached and no owned Domain resource was created by these runs.

The fixed live commands were not claimed as passing because the same prerequisite target-readiness failure prevented reaching the create path.

## Changed files

- `provider/live_harness_test.go`: strict sanitized Domain comparison formatter.
- `provider/live_harness_unit_test.go`: structural/sanitization regression test.
- `provider/live_workloads_test.go`: record structural comparison evidence without a second generated create attempt.

No provider/backup_test.go or provider/testserver_test.go changes were made or staged. No OpenAPI, generated client, or provider Domain payload changes were made because live contract evidence was unavailable.

## Self-review

- No alternate Domain payload, blind retry, or broad compatibility fallback was added.
- Generated Domain comparison is executed once and the result is reused for evidence.
- Evidence output is restricted to structural labels and sorted allowlisted keys.
- Fresh stop markers remained absent.
- `git diff --check` passed.

## Concern / blocker

The live environment did not provide a ready application or Compose target to exercise Domain creation. A subsequent run with authorized ready targets is required before determining or changing the deployed Domain create contract.
