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

## Controlled one-field Domain experiments

Added a serial, self-contained `TestLiveDomainContractExperiments`. Each target is created and owned by its selected subtest; each Domain attempt changes exactly one request field from the baseline, emits only `field`, structural status/code, and fixed category, and immediately verifies deletion if a response contains a resource. Target cleanup is verified by the harness owner. The experiment matrix covers DomainType omission, Port omission, certificate enum change, HTTPS change, StripPath change, and Compose ServiceName omission.

TDD RED:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDomainContractExperimentsChangeOneNamedField$' -count=1 -v
```

Result: expected build failure because `domainContractExperimentCases` was undefined.

TDD GREEN:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDomainContractExperimentsChangeOneNamedField$' -count=1 -v
```

Result: PASS; the matrix names and orders one-field changes deterministically.

Live command with a fresh absent marker:

```text
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-field-experiments.stop mise exec -- go test ./provider -run '^TestLiveDomainContractExperiments$' -parallel=1 -count=1 -v
```

Sanitized results for both disposable ready targets:

```text
field=baseline;status=4xx;code=BAD_REQUEST;category=rejected
field=domainType;status=4xx;code=BAD_REQUEST;category=rejected
field=port;status=4xx;code=BAD_REQUEST;category=rejected
field=certificateType;status=4xx;code=BAD_REQUEST;category=rejected
field=https;status=4xx;code=BAD_REQUEST;category=rejected
field=stripPath;status=4xx;code=BAD_REQUEST;category=rejected
field=serviceName;status=4xx;code=BAD_REQUEST;category=rejected
```

The Application and Compose runs completed all authorized serial experiments; every variant was rejected identically. No causal field contract was identified, so no provider payload or generated schema change was made. No raw values, payloads, IDs, hosts, response bodies, or errors were emitted. Partial/successful Domain cleanup was verified per attempt, target/project cleanup completed without a recorded failure, and `/tmp/opencode/domain-field-experiments.stop` remained absent.

Focused verification after the experiments:

```text
git diff --check
```

Result: both checks PASS. The evidence indicates an external deployed-server defect or an untested mandatory contract requirement; guessing a provider correction remains disallowed.

## Review fixes for 88fa54d

Added `TestDomainContractExperimentFailsClosedWithoutID` before implementing the response classification. A 2xx response without a resource ID now produces `cleanup-failure`, records the existing structural cleanup failure/stop marker, and cannot be reported as accepted or allow subsequent experiments. Cleanup errors now use `reportLiveCleanup`, which applies existing sanitized diagnostics and marker handling; raw cleanup errors are not passed to `requireNoError`.

Added `TestDomainContractExperimentsSerializeExactlyOneFieldDifference` before adding `serializedDomainFieldDiff`. It marshals baseline and every Application/Compose variant and asserts exactly the named field differs, including presence/absence changes.

RED commands:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run 'DomainContractExperimentsSerializeExactlyOneFieldDifference|DomainContractExperimentFailsClosedWithoutID' -count=1 -v
```

The first run failed at the expected missing helper; the initial syntax correction was local test authoring only. The final RED run for the missing response-classification helper failed as expected.

GREEN covering command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run 'DomainContractExperiment|DomainComparisonEvidence|SanitizeDomainValidationReason' -count=1 -v
```

Result: PASS, including serialized one-field checks, fail-closed 2xx-without-ID behavior, existing sanitization tests, and the live test’s opt-out skip. No live tests were run for this review fix, as instructed.

Final verification:

```text
git diff --check
```

Result: PASS. Existing cancellation edits and the untracked plan remain unstaged.

## Luna medium finding: reason interpolation hardening

Added `TestDomainComparisonEvidenceRejectsMaliciousReasons` before changing `formatDomainComparisonEvidence`. RED demonstrated that a malicious `provider.reason` was interpolated into evidence. The formatter now validates both reasons against the fixed `field=<allowlisted Domain field>;category=<allowlisted category>` shape (or a fixed category-only shape), normalizes only the validated tokens, and returns fixed `invalid-evidence` for any other input.

RED command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDomainComparisonEvidenceRejectsMaliciousReasons$' -count=1 -v
```

Result: expected failure showing the unsafe message would have been emitted.

GREEN focused command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run 'DomainComparisonEvidence|SanitizeDomainValidationReason' -count=1 -v
```

Result: PASS, including existing sanitized evidence tests and the malicious-reason regression. No live retry or production payload change was made.

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

## Resumed Task 1: focused live harness

### Root cause

The prior focused commands selected `TestLiveTier2Workloads/Domain/...` without executing the sibling `Application` and `Compose` setup subtests. The Domain subtests therefore inherited no target IDs and stopped at readiness. The fix adds `TestLiveDomainFocused`, whose selected subtest creates its own disposable target, verifies readiness, runs the Domain lifecycle, and owns verified cleanup independently of sibling subtests.

### Harness TDD evidence

Added `TestRunFocusedDomainTargetOwnsAndCleansTarget` before implementing `runFocusedDomainTarget` and `focusedDomainTarget`.

RED command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestRunFocusedDomainTargetOwnsAndCleansTarget$' -count=1 -v
```

Result: expected build failure because the focused harness helper and target type were undefined.

GREEN command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestRunFocusedDomainTargetOwnsAndCleansTarget$' -count=1 -v
```

Result: PASS; create, run, and cleanup ownership ordering was verified.

Added `TestDomainComparisonEvidenceReportsSafeKeyShapeMismatch` before extending the formatter to report safe provider/generated key sets when they differ. RED failed because the old formatter returned only `invalid-evidence`; GREEN passed after the formatter change.

### Focused live evidence

Fresh marker paths were absent before each run:

```text
/tmp/opencode/domain-focused-application.stop
/tmp/opencode/domain-focused-compose.stop
```

Commands:

```text
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-focused-application.stop mise exec -- go test ./provider -run '^TestLiveDomainFocused/application$' -parallel=1 -count=1 -v
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-focused-compose.stop mise exec -- go test ./provider -run '^TestLiveDomainFocused/compose$' -parallel=1 -count=1 -v
```

Both focused harnesses independently created a ready disposable target and reached Domain create. Both provider and generated paths returned the same sanitized structural result:

```text
target=application;provider=4xx/BAD_REQUEST;generated=4xx/BAD_REQUEST;keys=applicationId,certificateType,domainType,host,https,port,stripPath
target=compose;provider=4xx/BAD_REQUEST;generated=4xx/BAD_REQUEST;keys=certificateType,composeId,domainType,host,https,port,serviceName,stripPath
```

The provider and generated request key sets were equal for each target, so the evidence is `server-contract-rejection`, not a provider serialization mismatch. No raw response, error, endpoint, host, ID, or field value was emitted. Both commands failed at the expected Domain create assertion after evidence collection. Their target and project cleanup callbacks completed without a recorded cleanup failure, and both fresh stop markers remained absent.

### Contract decision

The evidence proves the provider and generated-client paths are structurally equal and both rejected by the deployed server. It does not prove which request field the server rejects. Per the approved evidence gate, no speculative Domain payload correction, alternate retry, OpenAPI change, or generated-client change was made. A production correction remains blocked until a safe deployed contract source identifies the rejected field or the server exposes an approved structural contract diagnostic.

### Additional changed files

- `provider/live_harness_test.go`: focused target ownership helper and safe key-shape evidence.
- `provider/live_harness_unit_test.go`: focused ownership and key-shape formatter regressions.
- `provider/live_workloads_test.go`: self-contained focused Application/Compose Domain live harness and sanitized evidence logging.

## Resumed Task 1: approved server-side reason inspection

### Access investigated

The repository exposes the typed `client.APIError` fields `StatusCode`, `Code`, `Message`, and `Operation`. A sanitizer was added that inspects only the typed message in memory, allowlists Domain field names and fixed categories (`missing-field`, `invalid-value`, `unsupported`, `validation`), and records only the resulting structural classification. Raw response text, logs, message text, request values, IDs, hosts, endpoints, and errors are never emitted.

TDD RED:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestSanitizeDomainValidationReasonAllowListsFieldAndCategory$' -count=1 -v
```

Result: expected build failure because `sanitizeDomainValidationReason` was undefined.

TDD GREEN:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run 'SanitizeDomainValidationReason|DomainComparisonEvidence' -count=1 -v
```

Result: PASS. The sanitizer regression confirms a field/category classification is emitted without the sentinel message value.

### Live reason result

Fresh marker paths were absent before the runs:

```text
/tmp/opencode/domain-reason-application.stop
/tmp/opencode/domain-reason-compose.stop
```

Commands:

```text
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-reason-application.stop mise exec -- go test ./provider -run '^TestLiveDomainFocused/application$' -parallel=1 -count=1 -v
DOKPLOY_ACCEPTANCE_STOP_FILE=/tmp/opencode/domain-reason-compose.stop mise exec -- go test ./provider -run '^TestLiveDomainFocused/compose$' -parallel=1 -count=1 -v
```

Both self-contained focused runs reached Domain create. The typed provider and generated errors exposed no allowlisted server validation field/category; the sanitized result was `providerReason=category=unknown;generatedReason=category=unknown` for both targets. Both remained `4xx/BAD_REQUEST` with equal structural request key sets. No raw error, response, body, or value was recorded. Cleanup completed without a recorded cleanup failure and both fresh markers remained absent.

### Finding / remaining blocker

No approved server-side validation reason is accessible through the typed API error or the repository’s documented schema/log access. The missing access is a server response/log channel that exposes a sanitized Domain validation field or fixed category. Individual-field payload experiments remain unauthorized, so no Domain payload or generated contract change was made.

Final local verification:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider ./tests -run 'Domain|LiveDiagnosticSourceContract' -count=1
```

Result: PASS.
