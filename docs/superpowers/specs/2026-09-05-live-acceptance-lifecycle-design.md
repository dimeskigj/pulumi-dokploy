# Live Acceptance Lifecycle Design

## Goal

Review all features exposed by the Pulumi Dokploy provider and validate every
resource lifecycle against the configured live Dokploy instance while keeping
load safe for a low-powered server. Record each confirmed provider defect as a
sanitized, reproducible bug report.

## Existing Coverage

The provider exposes eighteen resources: Project, Environment, Application,
Compose, Postgres, MySQL, MariaDB, MongoDB, Redis, Domain, Destination, Backup,
VolumeBackup, SSHKey, Registry, Tag, ProjectTag, and Mount.

Two live-test layers already exist:

- `provider/live_test.go` directly exercises provider resource methods. It has
  broad but uneven resource coverage and several deployment-heavy tests.
- `tests/acceptance_test.go` and `tests/acceptance_program_test.go` run a Pulumi
  Automation API stack covering Project, Environment, Application, Compose,
  Postgres, Redis, and Domain.

Current gaps include incomplete update, import, delete-verification, and
replacement coverage across most resources. Eleven resources are absent from
the Pulumi Automation API acceptance program. Running the existing live suite
as one unrestricted command can pull and initialize several databases and
repeatedly redeploy workloads, which is inappropriate for the target server.

## Architecture

Use two complementary test layers:

1. Extend the direct-provider live suite to cover each resource's complete
   lifecycle with precise, resource-level diagnostics.
2. Keep one small Pulumi Automation API smoke stack to validate plugin loading,
   provider configuration, preview, dependency/output chaining, refresh,
   update, state adoption where practical, and destroy.

The live suite runs only with an explicit opt-in environment variable in
addition to `DOKPLOY_ENDPOINT` and `DOKPLOY_API_KEY`. Ordinary `go test ./...`
must skip all live operations. The local invocation may source `.env`, but test
code must not parse, print, copy, or persist that file or its values.

## Resource Tiers

All tiers and subtests run serially. No live test may call `t.Parallel()`.

### Tier 1: Control Plane

Cover Project, Environment, Destination, SSHKey, Tag, ProjectTag, and Registry.
Registry tests run only when dedicated registry URL, username, and password
variables are configured. These tests create no application or database
containers.

### Tier 2: Workloads And Routing

Cover Application, Compose, Domain, and Mount. Reuse a minimal shared workload
where doing so does not weaken lifecycle isolation. Exercise supported mount
types and targets while minimizing redeploys. Only one deployment or redeploy
may be active at a time.

### Tier 3: Databases

Cover Postgres, MySQL, MariaDB, MongoDB, and Redis in strict sequence. Create,
exercise, and delete one database before creating the next. Use the provider's
documented default or established lightweight image versions and bounded
timeouts.

### Tier 4: Backup Definitions

Cover Backup and VolumeBackup definitions for all supported target dispatch
paths. Keep schedules disabled for the complete test lifetime and do not
trigger backup execution. Use only destination metadata accepted by Dokploy
without requiring real object-storage writes. If the live Dokploy version
requires network validation, skip the affected case with an explicit
prerequisite message rather than contacting an arbitrary external endpoint.

### Pulumi Smoke

Use a small stack containing Project, Environment, Tag, and ProjectTag. The
stack validates provider plugin loading and configuration, unresolved output
handling during preview, create, refresh, mutable update, adoption/import where
Automation API supports it without unsafe state manipulation, and destroy.
Deployment-heavy resources remain in the direct-provider tiers to avoid
duplicating container load.

## Lifecycle Contract

For every resource, test all lifecycle operations the resource supports:

1. Create with uniquely named baseline inputs.
2. Read or refresh and compare all observable baseline state.
3. Update every safely mutable behavior category with representative changed
   values.
4. Read again and verify the update persisted.
5. Perform an import-style read or adoption without relying on prior inputs,
   accounting for documented write-only secrets.
6. Validate replacement-only fields through provider diff assertions.
7. Perform a real replacement when it can be done without disproportionate
   server load; otherwise keep replacement validation at the diff layer and
   state that limitation in the coverage report.
8. Delete the resource.
9. Read after deletion and verify that the provider reports absence.

ProjectTag has no update operation, so its complete lifecycle is create, read,
import-style read, replacement diff, delete, and post-delete absence. Other
immutable associations follow the same principle. Secret and write-only fields
are verified only where the API can return them safely; tests must preserve
prior secret state where the provider contract requires it.

## Feature Coverage

Application coverage includes Docker and generic Git source configuration,
source reads, mutable runtime/build fields, secrets, registries when configured,
and source replacement diffs. GitLab integration paths that require an
unmanaged pre-existing integration are prerequisite-gated.

Compose coverage includes raw and generic Git sources, source reads, mutable
environment/configuration fields, compose mode behavior, source replacement
diffs, and volume-deletion settings. GitLab integration paths are similarly
prerequisite-gated.

Domain coverage includes Application and Compose-service targets, mutable host,
path, port, enablement, and safe certificate modes. Tests must not request or
provision public certificates for reserved test hosts.

Database coverage includes deployment completion, read/refresh, representative
mutable runtime fields, import reconstruction, replacement diffs for ownership
fields, delete, and post-delete absence. MongoDB replica behavior is tested only
if it does not require multiple containers on the target Dokploy version.

Mount coverage includes bind, volume, and file types and every supported target:
Application, Compose, Postgres, MySQL, MariaDB, and Redis. MongoDB and LibSQL
remain documented exclusions. Tests group checks to reduce target redeploys and
must not modify host paths outside a dedicated acceptance-test path.

Backup coverage includes Postgres, MySQL, MariaDB, and MongoDB dispatch.
VolumeBackup coverage includes Application and Compose-service targets.

## Naming, Isolation, And Cleanup

Every created entity uses a `pulumi-acceptance-` prefix plus a run-specific
identifier. Tests never discover and mutate arbitrary existing resources.

Register cleanup immediately after obtaining a resource ID. Explicit lifecycle
deletes may run before registered cleanup; cleanup must tolerate an already
absent resource. Parent Project cleanup is the final safety net for children.
Each operation uses a bounded context. Cleanup uses its own bounded context and
must not wait forever after a failed test.

If cleanup fails, stop progression to later deployment-heavy tiers and report
the leaked resource type and ID. Do not continue creating load while the server
or provider is in an uncertain state.

## Load Control

- Run all live tests serially.
- Permit only one active image pull, deployment, redeploy, or database startup.
- Reuse already pulled images where practical.
- Destroy each heavy resource and wait for observed absence before proceeding.
- Keep backup schedules disabled and never execute backups.
- Do not request public TLS certificates.
- Skip optional integrations unless their dedicated prerequisites exist.
- Use per-operation and per-tier timeouts, with no unbounded polling.
- Stop a tier after cleanup failure, repeated timeout, or server-health failure.

## Secrets

The harness reads credentials only from process environment variables. Local
commands may export variables from `.env` without displaying them. The API key,
registry password, SSH private key, database passwords, environment values,
build arguments, build secrets, and file mount contents must never appear in
test names, command output, assertion messages, generated reports, Pulumi
diagnostics, or committed files.

Secret assertions compare values in memory and identify failures by resource
and field name only. Bug reports sanitize endpoint hostnames, tokens, IDs when
they contain sensitive context, request bodies, and response payloads.

## Failure Handling

Resource failures should expose the operation, resource type, safe resource ID,
HTTP status, and provider error chain. When an `infer.ResourceInitFailedError`
is present, report its reasons without including secret inputs. Distinguish:

- Provider defects: incorrect requests, response decoding, state handling,
  lifecycle behavior, diff behavior, or cleanup behavior.
- Dokploy/server failures: capacity exhaustion, unhealthy daemon, unsupported
  server version, or upstream API defects.
- Test-environment limitations: missing optional credentials, integrations,
  DNS, certificates, or storage.

Only confirmed provider defects become provider bug reports.

## Bug Reports

Write one Markdown file per distinct confirmed issue under `docs/bugs/`. Each
report contains:

- Title and severity.
- Affected provider resource and tested revision.
- Sanitized Dokploy version/environment context when available.
- Minimal reproducible acceptance test and command.
- Expected and actual behavior.
- Sanitized error evidence.
- Likely provider code location and technical analysis.
- Workaround, if one exists.
- Cleanup result and any leaked test resource IDs safe to disclose.

Also write a run summary that lists every resource/feature tested, pass/skip/fail
status, skip reason, duration, and cleanup status. Environment limitations and
server defects belong in this summary rather than provider bug reports.

## Luna Workflow

A Luna implementer writes the acceptance harness and tests from a scoped task
brief, adds deterministic non-live tests for harness behavior, runs safe local
verification, and self-reviews the changes. A separate Luna reviewer checks
specification compliance, lifecycle completeness, cleanup safety, secret
handling, load control, and test quality.

After review fixes, Luna agents execute live tiers one at a time. Each tier is
reviewed before proceeding to the next heavier tier. Confirmed failures are
reproduced with the smallest relevant test before a bug report is written.

## Acceptance Criteria

- All eighteen resources have explicit live lifecycle coverage or a documented,
  prerequisite-based skip for an external integration.
- Every supported lifecycle includes create, read/refresh, update where
  applicable, import-style adoption, delete, and post-delete absence.
- Replacement-only behavior is validated and real replacements are limited to
  cases safe for the server.
- Live tests require explicit opt-in and run serially.
- No test leaks secrets or modifies pre-existing resources.
- No more than one heavy operation runs at once.
- Disabled backup definitions do not execute external backups.
- Cleanup is immediate, bounded, idempotent, and verified.
- The Pulumi smoke stack completes preview, up, refresh, update, adoption where
  practical, and destroy.
- Each confirmed provider defect has a sanitized report under `docs/bugs/`.
- A run summary records coverage, results, skips, duration, and cleanup status.
