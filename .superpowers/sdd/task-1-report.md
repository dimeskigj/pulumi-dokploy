# Task 1 report

## Base and commit

- Base HEAD: `0a98621b6c81da6613fc37baebb7c68e81c9ae88`
- Commit: `d4e57b0241ab89786c06ec8f91535a4b89088105`
- Commit message: `test: fix mount dispatch readiness`

## RED

Command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestLiveTargetReadinessUsesConcreteResourceReader$' -count=1 -v
```

Result: expected build failure. The new readiness type and six concrete readiness constructors were undefined. The fixture test also initially required the `context` import.

## GREEN and verification

Command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^(TestLiveTargetReadinessUsesConcreteResourceReader|TestPostgresTargetReadinessReportsNotReadyAndMissing)$' -count=1 -v
```

Result: PASS. The concrete-reader matrix and Postgres ready/not-ready/missing cases passed.

Command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDispatchFixtureCarriesConcreteReadiness$' -count=1 -v
```

Result: PASS.

Command:

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^(TestLiveTargetReadiness|TestPostgresTargetReadiness|TestDispatchFixture|TestPostgresMountCleanupOwnership)' -count=1 -v
```

Result: PASS; all selected tests passed.

Command:

```text
git diff --check
```

Result: PASS.

The unfiltered provider test command was also attempted with the required environment clearing, but exceeded the 120-second command timeout without producing output. No live tests were run.

## Files changed

- `provider/live_workloads_test.go`: added concrete resource readiness callbacks, threaded readiness through mount lifecycle and dispatch fixtures, and removed generic inferred workload dispatch.
- `provider/live_harness_unit_test.go`: added the concrete-reader matrix and readiness-state tests.
- `provider/mount_lifecycle_matrix_test.go`: added fixture readiness wiring coverage.

## Concern

The brief’s application and compose response examples are insufficient for their existing concrete `Read` implementations, which reconstruct required source data. The deterministic matrix responses therefore include the minimal non-sensitive source metadata needed for those existing readers; production behavior was not changed.

## Review fix

Finding addressed: PostgreSQL `MountDispatch` now passes `fixture.readiness`, matching the stored readiness callback used by the other database dispatch branches.

The fixture wiring test now constructs a PostgreSQL dispatch fixture through the same helper used by the PostgreSQL creation branch and verifies its concrete readiness callback against a scripted local server. No live calls are made.

Command (TDD RED):

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDispatchFixtureCarriesConcreteReadiness$' -count=1 -v
```

Result: expected build failure before implementation because `newPostgresDispatchFixture` was undefined.

Command (GREEN):

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^TestDispatchFixtureCarriesConcreteReadiness$' -count=1 -v
```

Result: PASS.

Command (focused regression verification):

```text
env -u DOKPLOY_ACCEPTANCE -u DOKPLOY_ENDPOINT -u DOKPLOY_API_KEY go test ./provider -run '^(TestLiveTargetReadiness|TestPostgresTargetReadiness|TestDispatchFixture|TestPostgresMountCleanupOwnership)' -count=1 -v
```

Result: PASS; all selected tests passed.

Self-review: `git diff --check` passed; no production provider behavior, live tests, credentials, endpoints, resource IDs, or response bodies were emitted in this report.
