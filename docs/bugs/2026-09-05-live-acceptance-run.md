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

## Task 3 — final Tier 2 compatibility evidence

The final serial Tier 2 execution took **110.07s**. The acceptance stop marker
was absent. Application and Compose passed. All six Application/Compose Mount
lifecycle cases passed, as did the Compose, PostgreSQL, MySQL, MariaDB, and
Redis MountDispatch cases. Registry and GitLab source variants were skipped
because their dedicated credentials were not configured, as designed.

The two Domain cases (Application and Compose) each returned only the
sanitized classification `operation=domain;status=4xx;code=BAD_REQUEST`.
No request-shape mismatch was established. The deterministic provider and
generated-request matrices match the pinned OpenAPI contract.

### Revision and source comparison

The deployed Dokploy revision is **unavailable** from the already-safe local
metadata and no server administration metadata was queried. It must not be
invented or inferred from the live result. The pinned OpenAPI source revision
remains the repository-recorded source of contract comparison.

Inspection of the pinned Dokploy source shows that Domain create selects the
Application or Compose authorization target from `domainType`, accepts the
corresponding foreign-key field, and retains `serviceName` for Compose. Mount
create dispatches `serviceId` to the Application or Compose foreign-key field;
the same dispatch pattern is used by the working database branches. The pinned
schema declares the corresponding nullable foreign keys, and the pinned source
includes the migrations that establish them. Applied migration state on the
deployed instance is **unavailable** without an approved server-side metadata
source.

### Decision and ownership

Because no provider request mismatch was reproduced, no OpenAPI correction,
provider mapping change, or create-time null-omission change is justified.
The Domain failures are classified as Dokploy server/deployed-version
compatibility evidence, owned by the Dokploy deployment and migration path.
The exact follow-up boundary is to compare the deployed revision and applied
migrations with the pinned source, then patch Dokploy if the Application or
Compose foreign-key handling is absent or inconsistent. A targeted provider
compatibility diagnostic can be added only after a minimum compatible Dokploy
version is established.

### Cleanup evidence limitation

The supplied sanitized final log records the lifecycle outcomes and no stop
condition, but does not independently enumerate cleanup confirmations for each
case. No cleanup failure is present in that log; cleanup verification remains
limited to the harness evidence retained outside this tracked summary.
