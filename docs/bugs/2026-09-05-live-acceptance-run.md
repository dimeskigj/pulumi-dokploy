# Live acceptance run — 2026-09-05

## Run configuration

- **Workflow run:** none (local execution)

- **Provider revision:** `0ac0b55` (test-only Compose diff expectation corrected
  during this rerun; provider production code unchanged)
- **Environment:** Linux; Go `go1.26.6`; Pulumi CLI unavailable (`pulumi: command not found`)
- **Feature flags:** `DOKPLOY_ACCEPTANCE=1`; replica opt-in unset
- **Credential status:** configured by preflight; values not recorded
- **Cleanup policy:** each live test's registered cleanup ran; no cleanup or
  server-health stop was observed in Tier 1 output.
- **Stop-file contract:** local command helpers and CI export
  `DOKPLOY_ACCEPTANCE_STOP_FILE`.

## Tier results

| Tier / test | Result | Duration | Skip reason | Cleanup |
| --- | --- | ---: | --- | --- |
| Tier 1 control plane | **FAIL** | 4.72s | Registry skipped: dedicated registry credentials not configured | Project, destination, and tag passed and cleaned; no cleanup failure reported |
| Tier 2 workloads | **FAIL** | 91.58s | Registry/GitLab source credentials skipped; replica opt-in remained unset | All created workload/dispatch resources cleaned; no cleanup failure reported |
| Tier 3 databases | **FAIL** | 19.05s | MongoDB replica sets skipped because opt-in remained unset | All created database resources cleaned; no cleanup failure reported |
| Tier 4 backups | **PASS** | 25.08s | No skips | All backup and volume-backup definitions cleaned; no cleanup failure reported |
| Pulumi lifecycle smoke | **NOT RUN** | — | Pulumi CLI was unavailable before the live test could execute | not run |

## Tier 1 subtests

- **PASS:** Project (1.22s)
- **FAIL:** Environment (0.94s); sanitized error: `environment.update: BAD_REQUEST: Input validation failed`
- **PASS:** Destination (0.80s)
- **HISTORICAL FAIL:** SSHKey (0.08s); sanitized error: `organization.active returned incomplete organization` (superseded by the final focused skip before create)
- **SKIP:** Registry; dedicated registry credentials are not configured
- **PASS:** Tag (0.88s)
- **FAIL:** ProjectTag (0.96s); sanitized error: `project.one returned association missing after create`

## Feature rows

| Tier | Resource / feature | Status | Duration | Skip reason | Cleanup |
| --- | --- | --- | ---: | --- | --- |
| 1 | Project | pass | 1.13s | — | verified absent |
| 1 | Environment | fail | 0.76s | — | project cleanup completed |
| 1 | Destination | pass | 0.84s | — | verified absent |
| 1 | SSHKey | skip | 0.09s | organization.active response shape incompatible | none created |
| 1 | Registry | skip | 0.00s | dedicated registry credentials not configured | none created |
| 1 | Tag | pass | 0.90s | — | verified absent |
| 1 | ProjectTag | fail | 1.01s | — | project/tag cleanup completed |
| 2 | Application | fail | 3.82s | — | application cleanup completed; no cleanup failure reported |
| 2 | Compose | pass | 3.92s | — | compose cleanup completed; no cleanup failure reported |
| 2 | Domain/application | fail | 0.10s | — | no domain persisted; workload cleanup completed |
| 2 | Domain/compose | fail | 0.17s | — | no domain persisted; workload cleanup completed |
| 2 | Mount/application/bind | fail | 0.09s | — | no mount persisted; workload cleanup completed |
| 2 | Mount/application/volume | fail | 0.09s | — | no mount persisted; workload cleanup completed |
| 2 | Mount/application/file | fail | 0.09s | — | no mount persisted; workload cleanup completed |
| 2 | Mount/compose/bind | fail | 0.09s | — | no mount persisted; workload cleanup completed |
| 2 | Mount/compose/volume | fail | 0.09s | — | no mount persisted; workload cleanup completed |
| 2 | Mount/compose/file | fail | 0.09s | — | no mount persisted; workload cleanup completed |
| 2 | Mount dispatch: PostgreSQL | pass | 8.79s | — | verified absent |
| 2 | Mount dispatch: MySQL | pass | 22.62s | — | verified absent |
| 2 | Mount dispatch: MariaDB | pass | 8.61s | — | verified absent |
| 2 | Mount dispatch: Redis | pass | 40.95s | — | verified absent |
| 2 | Source: Git | pass | 2.33s | — | verified absent |
| 2 | Source: registry | skip | 0.00s | dedicated source credentials not configured | none created |
| 2 | Source: GitLab | skip | 0.00s | dedicated source credentials not configured | none created |
| 2 | Compose source: Git | pass | 1.73s | — | verified absent |
| 2 | Compose source: GitLab | skip | 0.00s | dedicated GitLab source credentials not configured | none created |
| 3 | PostgreSQL | fail | 3.69s | — | database cleanup completed; no cleanup failure reported |
| 3 | MySQL | fail | 3.55s | — | database cleanup completed; no cleanup failure reported |
| 3 | MariaDB | fail | 4.03s | — | database cleanup completed; no cleanup failure reported |
| 3 | MongoDB replica sets | skip | 0.00s | replica opt-in unset by policy | none created |
| 3 | MongoDB | fail | 3.73s | — | database cleanup completed; no cleanup failure reported |
| 3 | Redis | fail | 3.57s | — | database cleanup completed; no cleanup failure reported |
| 4 | Backup/PostgreSQL | pass | 4.48s | — | verified absent |
| 4 | Backup/MySQL | pass | 4.07s | — | verified absent |
| 4 | Backup/MariaDB | pass | 3.99s | — | verified absent |
| 4 | Backup/MongoDB | pass | 4.03s | — | verified absent |
| 4 | VolumeBackup/Application | pass | 3.56s | — | verified absent |
| 4 | VolumeBackup/Compose | pass | 4.20s | — | verified absent |
| 4 | Pulumi lifecycle smoke | not run | — | Pulumi CLI was unavailable before the live test could execute | not run |

The initial-run failure and cleanup output are preserved locally in the ignored file
`.superpowers/sdd/task-9-tier1.log`; it contains no credentials.

Initial-run Tier 2 output is preserved locally in the ignored file
`.superpowers/sdd/task-9-tier2.log`. Confirmed passes included database mount
dispatch for PostgreSQL, MySQL, MariaDB, and Redis, plus Git and compose-Git
source metadata. Failures were Application, Compose metadata diff, both Domain
creates, application/compose Mount creates, and compose Mount dispatch.

Initial-run Tier 3 output is preserved locally in the ignored file
`.superpowers/sdd/task-9-tier3.log`. All five non-replica database lifecycles
failed during update with the sanitized JSON decoding error
`json: cannot unmarshal bool into Go value of type map[string]json.RawMessage`.

Initial-run Tier 4 output is preserved locally in the ignored file
`.superpowers/sdd/task-9-tier4.log`. The backup PostgreSQL cleanup failure was a
stop condition, so no later backup cases were allowed to run and Pulumi smoke
was not run.

In the initial run, the smallest failing subtest was reproduced once with
`go test ./provider -run '^TestLiveTier4Backups/Backup/Postgres$' -parallel=1 -count=1 -v`.
It reproduced `backup.remove: BAD_REQUEST: Backup not found`. Provider code
sends the documented `backupId`, and the server reports the resource missing
with HTTP 400 rather than the not-found status/code handled by the provider.
This was classified as a Dokploy/server behavior issue, not a confirmed
provider defect; no provider code was changed. The initial reproduction output
is in ignored
`.superpowers/sdd/task-9-tier4-repro.log`.

## Task 9 rerun — corrected live evidence

The stop marker `/tmp/opencode/pulumi-dokploy-acceptance-rerun.stop` was removed
before start and remained absent throughout. Tiers ran serially. Tier 1 and
Tier 2 failed without cleanup/server-health stop conditions; Tier 3 ran with
replica opt-in unset; Tier 4 completed successfully. Pulumi smoke was **NOT
RUN** because `pulumi` was not installed before the live test could execute.
The corrected `t.Skip` prerequisite path was tested deterministically after this
live execution by `TestPulumiCLIAvailabilityHelper`; that helper test was not a
live execution. Rerun logs are sanitized and ignored under `.superpowers/sdd/`.

| Tier / focused command | Outcome |
| --- | --- |
| `TestLiveTier1ControlPlane` | FAIL, 4.72s; Environment and ProjectTag reproduced prior failures; SSHKey skipped before create because the organization response shape was incompatible; Registry skipped |
| `TestLiveTier2Workloads` | FAIL, 91.58s; Application boolean response mismatch, Compose passed, Domain and Mount API SQL failures; source prerequisites skipped |
| `TestLiveTier3Databases` | FAIL, 19.05s; boolean response mismatch for all five non-replica databases; replica sets skipped |
| `TestLiveTier4Backups` | PASS, 25.08s; all six configured backup cases cleaned |
| `TestAccLifecycleSmoke` | NOT RUN: Pulumi CLI missing (environment limitation) |

The Application and PostgreSQL smallest subtests were each reproduced once
after the tier runs. Both repeated the boolean response mismatch and completed
cleanup, confirming the provider/API contract defect recorded in this run
summary. The Compose mismatch was a test
expectation error: `composeType` is intentionally `update&replace`; the live
test now asserts that documented contract. Domain and Mount were reevaluated
with successfully created workload IDs and still returned Dokploy SQL validation
errors, so they remain server/API findings rather than confirmed provider
defects. Environment remains unresolved because no safe direct request
comparison proved a `projectId` requirement. ProjectTag remains unresolved
because no bounded observation proved eventual association. SSH organization
response remains an environment/server limitation.

The earlier PostgreSQL backup cleanup failure (`backup not found`) did not
repeat in the corrected Tier 4 run. It is classified as a transient
Dokploy/server cleanup response, not a provider defect; all corrected backup
cases cleaned successfully.

## Task 9 fix round — deterministic evidence

This correction round made no live calls and did not read `.env`; the existing
live status rows above were not overwritten.

### RED

`go test ./provider -run 'TestTask9' -count=1` failed to compile because the
new workload dependency, Compose diff, and exactly-once cleanup assertions had
no implementation.

### GREEN

`go test ./provider -run 'TestTask9' -count=1` passed. The focused tests cover
the documented `DOKPLOY_ACCEPTANCE_STOP_FILE` contract, immediate workload ID
preservation and dependent-target readiness, one-property-at-a-time Compose
diff inputs, and exactly-once Backup/VolumeBackup fallback cleanup.

The no-opt-in tier command, short suite, full suite, and race suite passed;
live tiers skipped closed by missing opt-in/credentials. `golangci-lint` and
`mise` were unavailable in this environment, so lint could not be run.

## Task 9 final focused execution

- Test changes were committed as `72b6119` before live execution.
- The root `.env` was sourced without printing values. The exact stop-file
  variable was `DOKPLOY_ACCEPTANCE_STOP_FILE`; the marker remained absent and
  no cleanup/stop condition occurred.
- `TestLiveTier1ControlPlane/Environment` ran once and failed with the
  sanitized provider classification `environment.update: BAD_REQUEST: Input
  validation failed`. Its failure path issued exactly one direct generated
  `environment.update` request including the prior project ID; no request body,
  status payload, ID, or secret was recorded, so no new provider report is
  opened from this result.
- The earlier SSHKey run is historical FAIL evidence: `organization.active`
  returned an incomplete organization. The final focused
  `TestLiveTier1ControlPlane/SSHKey` run was SKIP before create because
  `organization.active` was not structurally a flat non-empty ID response:
  No key was created.
- `TestLiveTier1ControlPlane/ProjectTag` ran once and failed. The returned ID
  was polled for the bounded observation window before cleanup and was never
  visible; no delayed-association provider report is opened.
- The full `TestLiveTier2Workloads` ran once serially. Application failed on
  the already-confirmed boolean update response defect; Compose passed;
  Domain application/compose and Mount application/compose were independently
  reached as nested cases and failed with sanitized server SQL validation
  errors. Each bind, volume, and file mount case produced separate evidence.
  Source prerequisites were skipped. No Tier 3 or Tier 4 rerun was performed.
- The boolean update-response mismatch remains the only confirmed provider/API
  defect; no standalone bug report is committed.

## Task 5 fix verification

This section adds verification evidence for the contract fixes; the historical
rows and evidence above are unchanged. No endpoint, resource ID, request or
response payload, credential, SSH material, database value, or stop-file path
is recorded here.

- **Provider revision:** `64fc03f`
- **Static verification:** the short test suite and race suite passed. The
  repository OpenAPI normalization diff passed, and code generation followed
  by the tracked generated-file diff passed. `make check_openapi` itself was
  unavailable because `mise` is not installed in this environment.

| Focused resource | Controller command | Result | Duration | Cleanup / stop marker |
| --- | --- | ---: | ---: | --- |
| Application | `go test ./provider -run '^TestLiveTier2Workloads/Application$' -parallel=1 -count=1 -v` | **PASS** | 3.64s | lifecycle test passed; stop marker absent; cleanup not separately logged |
| Environment | `go test ./provider -run '^TestLiveTier1ControlPlane/Environment$' -parallel=1 -count=1 -v` | **PASS** | 1.19s | lifecycle test passed; stop marker absent; cleanup not separately logged |
| ProjectTag | `go test ./provider -run '^TestLiveTier1ControlPlane/ProjectTag$' -parallel=1 -count=1 -v` | **PASS** | 1.58s | lifecycle test passed; stop marker absent; cleanup not separately logged |
| SSHKey | `go test ./provider -run '^TestLiveTier1ControlPlane/SSHKey$' -parallel=1 -count=1 -v` | **PASS** | 1.98s | lifecycle test passed; stop marker absent; cleanup not separately logged |

The controller test logs show the focused test/package PASS results. The
configured stop marker remained absent after each command. Cleanup detail was
not emitted by the sanitized log and therefore is not independently evidenced
by the preserved artifact. The test implementation's cleanup-and-absence
checks are a separate lifecycle contract, not additional artifact evidence.

### Controller PostgreSQL correction

The earlier selector using `PostgreSQL` matched no subtest and is not evidence.
The corrected controller command was:

`go test ./provider -run '^TestLiveTier3Databases/Postgres$' -parallel=1 -count=1 -v`

It passed: the `Postgres` subtest completed in **11.28s** and the package test
output was PASS; the stop marker remained absent. Cleanup detail was not
emitted by the sanitized log and therefore is not independently evidenced by
the preserved artifact. The test implementation's cleanup-and-absence checks
are a separate lifecycle contract, not additional artifact evidence. No
endpoint, resource ID, request or response payload, credential, SSH material,
or database value is recorded here.

## Task 4 non-live runner verification (2026-09-08 local preflight)

Provider revision: `b127bbc`. No live smoke was run by this task, and no
credentials or `.env` files were read.

| Check | Result | Duration / limitation |
| --- | --- | --- |
| `mise install` | **NOT RUN** | `mise` is unavailable (`command not found`) |
| `mise exec -- pulumi version` | **NOT RUN** | `mise` is unavailable; pinned version `3.259.0` could not be checked |
| `make provider` | **PASS** | 217 ms; `bin/pulumi-resource-dokploy` built |
| `PATH="$PWD/bin:$PATH" mise exec -- pulumi plugin ls` | **NOT RUN** | `mise` is unavailable |
| `go test -short -count=1 ./provider/... ./internal/... ./tests/...` | **PASS** | 2.601 s |
| `git diff --check` | **PASS** | no whitespace errors |
| focused live smoke | **NOT RUN** | controller-only; protected credentials were not sourced |

The direct `pulumi plugin ls` fallback was also unavailable because the Pulumi
CLI was not installed outside `mise`. The provider build completed, but plugin
discovery and the pinned CLI version were not verified in this local preflight.

## Task 4 controller verification (2026-09-08)

The controller bootstrapped the exact pinned Pulumi CLI version in a temporary
repository-safe location, without changing user configuration. Before cleanup,
`pulumi version` reported `v3.259.0` and `pulumi plugin ls` completed
successfully with an empty plugin cache. An empty plugin cache is expected here:
the command does not list provider executables discovered directly on `PATH`.
The successful Automation API stack is the behavioral evidence that the local
`bin/pulumi-resource-dokploy` executable was discoverable. Credentials,
endpoint details, stack state, resource IDs, and configuration values are
intentionally omitted.

| Check | Result | Duration / cleanup |
| --- | --- | --- |
| `make provider` | **PASS** | provider built before smoke |
| `pulumi version` | **PASS** | reported `v3.259.0` before cleanup |
| `pulumi plugin ls` | **PASS** | empty cache; PATH-discovered executables are not listed |
| `TestAccLifecycleSmoke` | **PASS** | test 12.83 s; package 12.848 s; stop marker absent |
| temporary CLI files | **PASS** | removed and verified absent after the run |

This supersedes the Task 4 local placeholder's **NOT RUN** smoke result for
the 2026-09-08 controller verification. The historical 2026-09-05 live
non-run evidence above remains unchanged. By test control flow, the passing
smoke necessarily completed revision-one preview/up/refresh and revision-two
preview/up/refresh: each phase fails the test immediately on error. Cleanup is
registered before those phases, and destroy/remove errors call `t.Errorf`; the
overall PASS therefore implies both cleanup calls returned without a reported
error. This is an inference from the test control flow, not direct phase-log
evidence. No live state details are recorded here.
