# Live Acceptance Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build and run a safe, serial live acceptance suite that validates the complete supported lifecycle of all eighteen Dokploy resources and records sanitized bug reports for confirmed provider defects.

**Architecture:** Keep direct-provider lifecycle tests in focused tier files and use one small Pulumi Automation API smoke stack for engine/plugin behavior. A shared harness enforces explicit opt-in, unique names, bounded cleanup, serial heavy operations, post-delete absence checks, result recording, and secret-safe diagnostics.

**Tech Stack:** Go 1.26.6, `testing`, `testify/require`, `pulumi-go-provider/infer`, Pulumi Automation API, existing generated Dokploy client, Markdown reports.

## Global Constraints

- Follow `docs/superpowers/specs/2026-09-05-live-acceptance-lifecycle-design.md`.
- Cover Project, Environment, Application, Compose, Postgres, MySQL, MariaDB, MongoDB, Redis, Domain, Destination, Backup, VolumeBackup, SSHKey, Registry, Tag, ProjectTag, and Mount.
- Live execution requires `DOKPLOY_ACCEPTANCE=1`, `DOKPLOY_ENDPOINT`, and `DOKPLOY_API_KEY`.
- Never read `.env` in Go code; the shell may source it without printing values.
- Never log credentials, secret values, request bodies containing secrets, or the contents of `.env`.
- Every created resource name starts with `pulumi-acceptance-` and contains a run-specific suffix.
- Do not mutate resources that were not created by the current test run.
- No live test uses `t.Parallel()`; only one heavy create, deploy, redeploy, image pull, or database startup runs at a time.
- Register bounded, idempotent cleanup immediately after receiving a resource ID.
- Stop heavier-tier execution after cleanup failure or server-health failure.
- Backup and VolumeBackup schedules remain disabled and are never executed.
- Registry, GitLab, and other external-integration cases skip unless dedicated prerequisites exist.
- A complete supported lifecycle is create, read/refresh, representative mutable update, second read, import-style read, replacement diff, delete, and post-delete absence.
- Real replacement is required only when it is safe for the low-powered server; otherwise validate replacement with `Diff` and document the limitation.

## File Structure

- Create `provider/live_harness_test.go`: opt-in, naming, timeout, cleanup, absence, tier-stop, and result-reporting helpers.
- Create `provider/live_harness_unit_test.go`: deterministic tests for harness behavior without a live server.
- Create `provider/live_control_plane_test.go`: Project, Environment, Destination, SSHKey, Registry, Tag, and ProjectTag lifecycles.
- Create `provider/live_workloads_test.go`: Application, Compose, Domain, and Mount lifecycles.
- Create `provider/live_databases_test.go`: Postgres, MySQL, MariaDB, MongoDB, and Redis lifecycles.
- Create `provider/live_backups_test.go`: Backup and VolumeBackup lifecycles with disabled schedules.
- Modify `provider/live_test.go`: retain reusable key/registry/project helpers, remove duplicate lifecycle tests after migration, and remove live parallelism.
- Modify `tests/acceptance_test.go`: require explicit live opt-in and invoke the smaller smoke program.
- Modify `tests/acceptance_program_test.go`: Project/Environment/Tag/ProjectTag preview, update, refresh, import-style adoption, and destroy.
- Modify `.github/workflows/run-acceptance-tests.yml`: pass explicit opt-in and run tiers serially.
- Create `docs/bugs/README.md`: bug-report and live-run-summary format.
- Create `docs/bugs/2026-09-05-live-acceptance-run.md`: sanitized results from this execution.
- Create one additional file per confirmed provider defect under `docs/bugs/`;
  derive each filename as `2026-09-05-` plus the lowercase affected resource,
  failing operation, and `.md` (for example,
  `docs/bugs/2026-09-05-project-update.md`).

---

### Task 1: Safe Live Harness

**Files:**
- Create: `provider/live_harness_test.go`
- Create: `provider/live_harness_unit_test.go`
- Modify: `provider/live_test.go:56-77,211-245,564-577`

**Interfaces:**
- Produces: `requireLiveAcceptance(t *testing.T)`, `liveRunName(kind string) string`, `cleanupContext() (context.Context, context.CancelFunc)`, `recordLiveResult(...)`, and tier-stop state used by all later tasks.
- Preserves: `liveClient`, `liveProject`, `liveSSHKeyPair`, and `liveRegistryArgs` for tier files.

- [ ] **Step 1: Write harness unit tests**

Add table-driven tests proving that live acceptance skips unless the opt-in and both credentials are present, generated names use the `pulumi-acceptance-` prefix followed by the resource kind and UUID, cleanup contexts have finite deadlines, result text redacts configured secret sentinel values, and a cleanup failure prevents a later heavy tier from starting.

```go
func TestLiveGateRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("DOKPLOY_ACCEPTANCE", "")
	t.Setenv("DOKPLOY_ENDPOINT", "https://example.invalid")
	t.Setenv("DOKPLOY_API_KEY", "secret-sentinel")
	require.False(t, liveAcceptanceEnabled())
	t.Setenv("DOKPLOY_ACCEPTANCE", "1")
	require.True(t, liveAcceptanceEnabled())
}

func TestSanitizeLiveDiagnostic(t *testing.T) {
	t.Setenv("DOKPLOY_API_KEY", "secret-sentinel")
	require.NotContains(t, sanitizeLiveDiagnostic("failed: secret-sentinel"), "secret-sentinel")
}
```

- [ ] **Step 2: Run tests and observe failure**

Run: `go test ./provider -run 'TestLiveGateRequiresExplicitOptIn|TestSanitizeLiveDiagnostic' -count=1`

Expected: compilation fails because the harness functions do not exist.

- [ ] **Step 3: Implement the harness**

Implement pure `liveAcceptanceEnabled` and `sanitizeLiveDiagnostic` helpers, explicit skip behavior, UUID-based names, five-minute cleanup contexts, synchronized result collection, and a package-level heavy-tier stop flag set only by cleanup/server-health failures. Replace the two `t.Parallel()` calls in `provider/live_test.go` with serial execution. Make `liveClient` call `requireLiveAcceptance` before reading credentials.

```go
func liveAcceptanceEnabled() bool {
	return os.Getenv("DOKPLOY_ACCEPTANCE") == "1" &&
		os.Getenv("DOKPLOY_ENDPOINT") != "" && os.Getenv("DOKPLOY_API_KEY") != ""
}

func liveRunName(kind string) string {
	return "pulumi-acceptance-" + kind + "-" + uuid.NewString()
}
```

- [ ] **Step 4: Verify harness and ordinary-suite safety**

Run: `go test ./provider -run 'TestLiveGateRequiresExplicitOptIn|TestSanitizeLiveDiagnostic' -count=1`

Expected: PASS.

Run: `env -u DOKPLOY_ACCEPTANCE go test ./provider -run TestLive -count=1`

Expected: all server-touching live tests SKIP and no HTTP request is attempted.

- [ ] **Step 5: Commit**

```bash
git add provider/live_harness_test.go provider/live_harness_unit_test.go provider/live_test.go
git commit -m "test: add safe live acceptance harness"
```

### Task 2: Control-Plane Lifecycles

**Files:**
- Create: `provider/live_control_plane_test.go`
- Modify: `provider/live_test.go:121-138,364-414,579-675`

**Interfaces:**
- Consumes: Task 1 harness and existing resource `Create`, `Read`, `Update`, `Diff`, and `Delete` methods.
- Produces: ordered `TestLiveTier1ControlPlane` and reusable project/environment/destination fixtures for later tiers.

- [ ] **Step 1: Write the lifecycle subtests**

Create ordered subtests for Project, Environment, Destination, SSHKey, Registry, Tag, and ProjectTag. Each subtest performs baseline create/read, a safe update when supported, import-style `Read` with ID and no prior inputs, replacement-field `Diff`, explicit delete, and read-after-delete. ProjectTag omits Update because the provider intentionally has no update operation. Registry skips unless all dedicated registry variables are present.

Use assertions shaped like:

```go
read, err := resource.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
requireNoError(t, err)
require.Equal(t, created.ID, read.State.ProjectID)
updated, err := resource.Update(ctx, infer.UpdateRequest[ProjectArgs, ProjectState]{
	ID: created.ID, Inputs: ProjectArgs{Name: originalName, Description: stringPtr("updated")}, State: read.State,
})
requireNoError(t, err)
imported, err := resource.Read(ctx, infer.ReadRequest[ProjectArgs, ProjectState]{ID: created.ID})
requireNoError(t, err)
require.Equal(t, created.ID, imported.State.ProjectID)
```

- [ ] **Step 2: Verify compile and skip path**

Run: `env -u DOKPLOY_ACCEPTANCE go test ./provider -run TestLiveTier1ControlPlane -count=1 -v`

Expected: PASS with the top-level live test skipped.

- [ ] **Step 3: Migrate duplicate tests and enforce idempotent cleanup**

Remove superseded standalone lifecycle tests from `provider/live_test.go`, retaining shared helpers. Each explicit delete must be followed by a read asserting `ID == ""`; deferred cleanup must ignore the provider's not-found result and report other errors through the sanitized recorder.

- [ ] **Step 4: Run focused non-live provider tests**

Run: `go test ./provider -short -run 'Test(Project|Environment|Destination|SSHKey|Registry|Tag|ProjectTag)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add provider/live_control_plane_test.go provider/live_test.go
git commit -m "test: cover control-plane live lifecycles"
```

### Task 3: Workload, Domain, And Mount Lifecycles

**Files:**
- Create: `provider/live_workloads_test.go`
- Modify: `provider/live_test.go:140-245,677-739`

**Interfaces:**
- Consumes: Task 1 harness and Task 2 project/environment fixtures.
- Produces: ordered `TestLiveTier2Workloads` covering Application, Compose, Domain, and Mount.

- [ ] **Step 1: Write workload lifecycle tests**

Create one Docker Application and one raw Compose sequentially, waiting for deployment completion. For each resource perform Read, one representative mutable update, second Read, import-style Read, source/environment replacement Diff, Delete, and post-delete Read. Cover generic Git source configuration without triggering a second full deployment when the server can validate source metadata directly. Gate GitLab and registry source cases on dedicated prerequisite variables.

- [ ] **Step 2: Write safe Domain and Mount lifecycle tests**

Use `.example.invalid` hosts and certificate type `none`. Cover both Domain target variants and mutable host/path/port/enabled fields. Cover bind, volume, and file mounts and each supported target dispatch, but group assertions to avoid repeated redeploys. Generate the bind path by joining `/tmp` with `liveRunName("mount")`. Validate target/type replacement with Diff rather than a deployment-heavy real replacement.

```go
diff, err := mount.Diff(ctx, infer.DiffRequest[MountArgs, MountState]{
	ID: created.ID, Inputs: replacementInputs, State: read.State,
})
requireNoError(t, err)
require.Contains(t, diff.DetailedDiff, "type")
```

- [ ] **Step 3: Verify compile and skip path**

Run: `env -u DOKPLOY_ACCEPTANCE go test ./provider -run TestLiveTier2Workloads -count=1 -v`

Expected: PASS with the live test skipped.

- [ ] **Step 4: Run focused provider regression tests**

Run: `go test ./provider -short -run 'Test(Application|Compose|Domain|Mount)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add provider/live_workloads_test.go provider/live_test.go
git commit -m "test: cover workload live lifecycles"
```

### Task 4: Serial Database Lifecycles

**Files:**
- Create: `provider/live_databases_test.go`
- Modify: `provider/live_test.go:247-362`

**Interfaces:**
- Consumes: Task 1 harness and project/environment fixture.
- Produces: ordered `TestLiveTier3Databases` with one active database at a time.

- [ ] **Step 1: Write a typed lifecycle case per database**

Implement explicit subtests for Postgres, MySQL, MariaDB, MongoDB, and Redis rather than a reflection-heavy generic helper. Each test creates with the established image, waits for status `done`, reads state, changes one safe mutable runtime field, reads again, performs import-style Read, checks environment/server replacement Diff, deletes, and waits for absence before returning.

- [ ] **Step 2: Add serialization and cleanup assertions**

Record active heavy operations in the harness and fail before create if another is active. Release the slot only after successful delete/absence. MongoDB replica behavior skips with an explicit low-capacity reason if it would create multiple containers.

- [ ] **Step 3: Verify compile and skip path**

Run: `env -u DOKPLOY_ACCEPTANCE go test ./provider -run TestLiveTier3Databases -count=1 -v`

Expected: PASS with the live test skipped.

- [ ] **Step 4: Run focused database regression tests**

Run: `go test ./provider -short -run 'Test(Postgres|MySQL|MariaDB|MongoDB|Redis)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add provider/live_databases_test.go provider/live_test.go
git commit -m "test: cover serial database live lifecycles"
```

### Task 5: Disabled Backup Definition Lifecycles

**Files:**
- Create: `provider/live_backups_test.go`
- Modify: `provider/live_test.go:416-562`

**Interfaces:**
- Consumes: Task 1 harness and Task 4 database fixtures.
- Produces: ordered `TestLiveTier4Backups` for every supported Backup and VolumeBackup target.

- [ ] **Step 1: Write disabled Backup lifecycle cases**

Cover Postgres, MySQL, MariaDB, and MongoDB target dispatch. Set `Enabled: false` at create and update. Verify empty-create-response ID recovery, read, schedule/prefix/retention update while disabled, import-style read, target replacement Diff, delete, and absence. Skip with a prerequisite result if the Dokploy version performs destination network validation.

- [ ] **Step 2: Write disabled VolumeBackup lifecycle cases**

Cover Application and Compose-service targets with `Enabled: false`. Verify read, cron/prefix/retention update while disabled, import-style read, target replacement Diff, delete, and absence. Do not call any execute/run endpoint.

- [ ] **Step 3: Verify compile and skip path**

Run: `env -u DOKPLOY_ACCEPTANCE go test ./provider -run TestLiveTier4Backups -count=1 -v`

Expected: PASS with the live test skipped.

- [ ] **Step 4: Run focused backup regression tests**

Run: `go test ./provider -short -run 'Test(Backup|VolumeBackup)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add provider/live_backups_test.go provider/live_test.go
git commit -m "test: cover disabled backup live lifecycles"
```

### Task 6: Pulumi Engine Smoke Lifecycle

**Files:**
- Modify: `tests/acceptance_test.go:16-26`
- Modify: `tests/acceptance_program_test.go:14-112`

**Interfaces:**
- Consumes: generated Go SDK and provider plugin.
- Produces: a low-load `TestAccLifecycleSmoke` that validates Pulumi engine behavior without deploying containers.

- [ ] **Step 1: Write mock assertions for both program revisions**

Replace the deployment-heavy MVP program with Project, Environment, Tag, and ProjectTag resources. Add mock assertions that revision two changes Project description, Environment name, and Tag color while preserving dependency IDs and association inputs.

- [ ] **Step 2: Run mock test and observe failure**

Run: `go test ./tests -run TestLifecycleSmokeProgram -count=1`

Expected: FAIL until the smoke program emits the required resources and revision changes.

- [ ] **Step 3: Implement the Automation API lifecycle**

Require `DOKPLOY_ACCEPTANCE=1`, configure endpoint/API key with the key secret, Preview revision one, Up, Refresh, switch to revision two, Preview and Up, Refresh, verify stack outputs, and Destroy in bounded cleanup. Exercise import-style provider reads in Tier 1; do not manipulate Pulumi state directly merely to claim import coverage.

- [ ] **Step 4: Verify mock and skip paths**

Run: `go test ./tests -run 'TestLifecycleSmokeProgram|TestAccLifecycleSmoke' -count=1 -v`

Expected: mock test PASS and live smoke SKIP without explicit opt-in.

- [ ] **Step 5: Commit**

```bash
git add tests/acceptance_test.go tests/acceptance_program_test.go
git commit -m "test: add lightweight Pulumi lifecycle smoke"
```

### Task 7: Workflow And Reporting Contract

**Files:**
- Modify: `.github/workflows/run-acceptance-tests.yml`
- Create: `docs/bugs/README.md`
- Create: `docs/bugs/2026-09-05-live-acceptance-run.md`

**Interfaces:**
- Consumes: named tier tests from Tasks 2-6 and harness results from Task 1.
- Produces: serial live commands and a sanitized report template.

- [ ] **Step 1: Extend workflow contract tests first**

Update the existing workflow metadata test that covers `run-acceptance-tests.yml` to require `DOKPLOY_ACCEPTANCE: "1"`, named serial tier commands, and no `-parallel` value above `1`.

- [ ] **Step 2: Run workflow test and observe failure**

Run: `go test ./provider -run TestOwnedWorkflow -count=1`

Expected: FAIL because the workflow does not yet pass the opt-in or tier commands.

- [ ] **Step 3: Update the workflow**

Run Tier 1, Tier 2, Tier 3, Tier 4, then Pulumi smoke as separate steps with `-parallel=1 -count=1 -v`. Preserve protected environment secrets and do not add `.env` loading to CI.

- [ ] **Step 4: Add reporting documentation**

Document severity, resource/revision, sanitized environment, reproduction, expected/actual, safe evidence, likely code location, workaround, and cleanup status. Seed the run summary with all eighteen resources and `not run` status; live execution updates each row with pass/skip/fail, duration, and cleanup status.

- [ ] **Step 5: Verify and commit**

Run: `go test ./provider -run TestOwnedWorkflow -count=1`

Expected: PASS.

```bash
git add .github/workflows/run-acceptance-tests.yml docs/bugs/README.md docs/bugs/2026-09-05-live-acceptance-run.md
git commit -m "test: wire serial live acceptance tiers"
```

### Task 8: Static Verification And Review Fixes

**Files:**
- Review: `provider/live_harness_test.go`
- Review: `provider/live_harness_unit_test.go`
- Review: `provider/live_control_plane_test.go`
- Review: `provider/live_workloads_test.go`
- Review: `provider/live_databases_test.go`
- Review: `provider/live_backups_test.go`
- Review: `tests/acceptance_test.go`
- Review: `tests/acceptance_program_test.go`
- Review: `.github/workflows/run-acceptance-tests.yml`
- Review: `docs/bugs/README.md`
- Review: `docs/bugs/2026-09-05-live-acceptance-run.md`

**Interfaces:**
- Consumes: Tasks 1-7.
- Produces: reviewed test suite ready for live execution.

- [ ] **Step 1: Run formatter and focused tests**

Run: `gofmt -w provider/live*_test.go tests/acceptance*_test.go`

Run: `go test -short -count=1 ./provider/... ./internal/... ./tests/...`

Expected: PASS; all live tests skip.

- [ ] **Step 2: Run race and lint checks**

Run: `go test -race ./provider/... ./internal/...`

Run: `golangci-lint run`

Expected: PASS.

- [ ] **Step 3: Dispatch Luna reviewer**

Require review of all changed files against the design, emphasizing lifecycle completeness, post-delete semantics, idempotent bounded cleanup, no secret exposure, no parallel heavy work, disabled backups, and explicit skips only for genuine external prerequisites.

- [ ] **Step 4: Apply review fixes and rerun checks**

For each accepted finding, add or adjust the smallest failing deterministic test first, implement the fix, then rerun the exact focused test plus the full commands from Steps 1-2.

- [ ] **Step 5: Commit review fixes**

```bash
git add provider tests .github/workflows/run-acceptance-tests.yml docs/bugs
git commit -m "test: harden live acceptance lifecycle coverage"
```

### Task 9: Cautious Live Execution And Bug Reports

**Files:**
- Modify: `docs/bugs/2026-09-05-live-acceptance-run.md`
- Create conditionally: one file per reproduced provider defect, named from the
  lowercase resource and operation, such as
  `docs/bugs/2026-09-05-project-update.md`.
- Modify: provider/test files only when a test itself is proven incorrect or unsafe.

**Interfaces:**
- Consumes: reviewed suite and local `.env` exported by the shell.
- Produces: live evidence, complete run summary, and sanitized bug reports.

- [ ] **Step 1: Confirm credential names without outputting values**

Run a shell preflight that sources `.env`, checks `DOKPLOY_ENDPOINT` and `DOKPLOY_API_KEY` are non-empty, sets `DOKPLOY_ACCEPTANCE=1`, and prints only `credentials configured`. Do not use `set -x`, `env`, or commands that print variable values.

- [ ] **Step 2: Run Tier 1 and review cleanup**

Run: `go test ./provider -run '^TestLiveTier1ControlPlane$' -parallel=1 -count=1 -v`

Expected: every configured control-plane lifecycle passes or yields a sanitized reproducible failure; all created resources are absent afterward.

- [ ] **Step 3: Run each heavier tier only after the prior tier is clean**

Run in order, never concurrently:

```bash
go test ./provider -run '^TestLiveTier2Workloads$' -parallel=1 -count=1 -v
```

Stop immediately on cleanup failure, server-health failure, or repeated timeout. Update the run summary after each command before proceeding.

- [ ] **Step 4: Reproduce and classify failures**

For each failure, rerun only the smallest failing subtest once after confirming cleanup and server health. Classify it as provider defect, Dokploy/server defect, or environment limitation. Do not modify provider implementation unless the user separately requests fixes.

- [ ] **Step 5: Write sanitized bug reports**

Create one report per reproduced provider defect using `docs/bugs/README.md`. Include the exact focused test command and safe error chain, but no endpoint, API key, secret field value, private key, credential, or unsanitized request/response payload.

- [ ] **Step 6: Verify reports and repository state**

Run: `git diff --check`

Run: `go test -short -count=1 ./provider/... ./internal/... ./tests/...`

Expected: PASS, with live tests skipped when opt-in is absent.

- [ ] **Step 7: Commit live findings**

```bash
git add docs/bugs provider tests
git commit -m "docs: report live acceptance findings"
```
