# Workload API Compatibility Task 2

Status: complete

Commit: `cdff7dc test: add safe workload compatibility diagnostics`

Implemented the allowlisted workload-create classifier, fresh target reads before Domain and Application/Compose Mount creates, and structural create-failure evidence based only on typed status/code metadata and request-key names. Diagnostics exclude IDs, hosts, paths, content, raw SQL, and secrets.

Tests:

- `go test ./provider -run 'TestClassifyWorkloadCreateAttempt' -count=1` — PASS
- `go test ./provider -run 'TestClassifyWorkloadCreateAttempt|TestTask9|TestLiveHarness' -count=1` — PASS
- `go test ./...` — PASS
- `git diff --check` — PASS

TDD RED evidence: the classifier-focused test initially failed to compile with `undefined: classifyWorkloadCreateAttempt`.

Concerns: none. No live calls or `.env` files were read.

## Review finding follow-up

Commit: `bc40d06 fix: tighten workload diagnostic allowlists`

The classifier now rejects unknown operations through an error-returning signature, accepts only the explicit Domain/Mount JSON request-key allowlist, and accepts only the known safe typed API codes `BAD_REQUEST`, `NOT_FOUND`, and `VALIDATION_ERROR`. Unknown codes become `unknown`. Adversarial tests cover alphanumeric secret/ID sentinels, mixed valid/invalid keys, invalid operations, and sanitization.

Verification:

- `go test ./provider -run 'TestClassifyWorkloadCreateAttempt|TestTask9|TestLiveHarness' -count=1` — PASS
- `go test ./...` — PASS
- `git diff --check` — PASS

## Cherry-pick resolution follow-up

Resolved the conflict while preserving the strict allowlists, adversarial safety
coverage, expanded mount target keys, and unknown-operation regression test.
`classifyWorkloadCreateError` now propagates classifier errors, and Domain/Mount
call sites handle those errors while retaining the fresh target state.

Commit: `aeb6d6d fix: propagate workload diagnostic errors`

Verification:

- `git cherry-pick --continue` — PASS; created `aeb6d6d`
- `go test ./provider -run 'TestClassifyWorkloadCreate' -count=1` — PASS
- `go test ./... -count=1` — PASS
- `git diff --check` — PASS

Concerns: none. The report remains unstaged per the instruction to stage only
the three task provider files.

## Tier 2 harness safety follow-up

Status: complete

Successful Application and Compose creates remain owned by the parent serial
test until Domain, Mount, dispatch, and source checks finish. Partial create
IDs are still cleaned immediately; successful workloads are deleted in the
final explicit lifecycle block, with the parent cleanup as failure backstop.
Domain/Mount create failures, including MountDispatch/compose, use the strict
structural classifier instead of raw errors. Database target keys are included
in its allowlist.

TDD RED evidence: the new lifecycle/diagnostic tests initially failed to
compile because the required helpers did not yet exist.

Verification:

- `go test ./provider -run 'Test(ClassifyWorkloadCreateErrorNeverIncludesRawServerDetails|Tier2WorkloadLifecycleDeletesTargetsOnlyAfterDependents|SuccessfulCreateDoesNotRegisterSubtestCleanup)$' -count=1` — PASS
- `go test ./... -count=1` — PASS

Concerns: no live calls made; `.env` files were not read.

## Off-branch intent transplant from `3f7fcad`

Status: complete on current HEAD `5f226e6`.

Preserved the newer workload-target ownership changes from `f6787c5` and
`5f226e6`, while restoring the missing intent from `3f7fcad`: fresh Compose
reads immediately before dispatch Mount.Create, structural Domain/Mount
diagnostics, secret-safe comparisons, and non-panicking Diff-kind lookup.
Adversarial tests cover IDs, secrets, SQL, paths, content, and read/create
ordering without printing values.

TDD RED evidence:

- `go test ./provider -run 'TestLive(LifecycleDiagnosticContainsOnlyStructure|TargetReadMustPrecedeCreate|DiffKindMissingFieldIsReportedWithoutPanic)$' -count=1`
  initially failed to compile because the transplanted helpers were absent.

Verification:

- Focused normal workload/live tests — PASS
- Focused helper race tests — PASS
- `go test ./... -count=1` — PASS
- `go test -race ./provider/... -count=1` — PASS
- `git diff --check` — PASS

The previously observed full provider race failure in unrelated
`TestBackupCreate_Cancellation` was not addressed and no backup tests were
modified.

## Review follow-up: evidence interpretation and cleanup ownership

The raw SQL/parameter output preserved in
`.superpowers/sdd/workload-task-3-tier2-live.log` is pre-`f6787c5` reproducer
evidence that motivated the structural diagnostic fix. It is not post-fix
verification and has intentionally not been rewritten or deleted.

The final workload delete helpers now mark ownership released immediately
after a successful delete (before read-after-delete verification), preventing
deferred cleanup from retrying a successful mutation if verification fails.
Domain/Mount read, update, and delete failures use the same structural
operation/status/code classifier as create failures; raw SQL, params, IDs,
paths, content, and arbitrary sentinel values are excluded.

Verification:

- `go test ./provider -run 'Test(WorkloadLifecycleDiagnosticsExcludeUnstructuredFailureDetails|DeleteAndVerifyOnceMarksOwnershipBeforeVerification)$' -count=1` — PASS

Additional verification:

- Focused Tier 2 regression suite — PASS
- `go test ./... -count=1` — PASS
- `go test -race ./provider -run 'Test(WorkloadLifecycleDiagnosticsExcludeUnstructuredFailureDetails|DeleteAndVerifyOnceMarksOwnershipBeforeVerification)$' -count=1` — PASS
- `git diff --check` — PASS

## Tier 2 harness safety follow-up

Status: complete

The successful Application and Compose creates now remain owned by the parent
serial test until Domain, Mount, dispatch, and source checks finish. Partial
create IDs are still cleaned immediately; successful workloads are deleted in
the final explicit lifecycle block, with the existing parent cleanup as the
failure-path backstop. Domain/Mount create failures, including
MountDispatch/compose, now use the strict structural classifier instead of
printing raw errors. Database target keys were added to the classifier's
allowlist.

TDD RED evidence: the new lifecycle/diagnostic tests initially failed to
compile because the required helpers did not yet exist.

Verification:

- `go test ./provider -run 'Test(ClassifyWorkloadCreateErrorNeverIncludesRawServerDetails|Tier2WorkloadLifecycleDeletesTargetsOnlyAfterDependents|SuccessfulCreateDoesNotRegisterSubtestCleanup)$' -count=1` — PASS
- `go test ./... -count=1` — PASS

Concerns: no live calls made; `.env` files were not read.
