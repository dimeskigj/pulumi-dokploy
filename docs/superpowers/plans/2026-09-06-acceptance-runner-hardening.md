# Acceptance Runner Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make explicit live acceptance runs fail closed when Pulumi is unavailable, execute the local provider binary, and continue independent tiers unless the stop marker indicates cleanup or server-health danger.

**Architecture:** Pin Pulumi with the repository toolchain, verify it before any live request, and expose the built provider on `PATH`. Mark tier commands as collectable failures, gate later work only on the stop marker, then aggregate ordinary failures at the end so all safe coverage runs.

**Tech Stack:** mise, Pulumi CLI 3.259.0, Go Automation API tests, GitHub Actions YAML, Go workflow contract tests.

## Global Constraints

- Keep the workflow manual-only and protected by the `dokploy-acceptance` environment.
- Never print secret values during preflight.
- Stop later heavy work immediately when `DOKPLOY_ACCEPTANCE_STOP_FILE` exists.
- Missing Pulumi is a skip only when live acceptance was not explicitly enabled.
- Use the repository-built `bin/pulumi-resource-dokploy`, never an accidentally installed released plugin.

---

### Task 1: Pin And Require Pulumi

**Files:**
- Modify: `.mise.toml`
- Modify: `tests/acceptance_test.go:17-31,34-45`
- Modify: `CONTRIBUTING.md:1-8`

**Interfaces:**
- Consumes: `pulumiCLIAvailable(func(string) (string, error)) bool`.
- Produces: an explicit-live prerequisite failure when `pulumi` is unavailable.

- [ ] **Step 1: Add failing prerequisite tests**

Extract the gate into:

```go
func requirePulumiCLI(acceptanceEnabled bool, lookPath func(string) (string, error)) error
```

Table-test that disabled acceptance returns nil without a CLI, enabled acceptance returns `Pulumi CLI prerequisite is unavailable` when lookup fails, and enabled acceptance returns nil when lookup succeeds.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./tests -run 'TestPulumiCLIAvailabilityHelper|TestRequirePulumiCLI' -count=1`

Expected: compilation failure because `requirePulumiCLI` does not exist.

- [ ] **Step 3: Implement fail-closed behavior**

Call `requirePulumiCLI(true, exec.LookPath)` after opt-in and credential checks in `TestAccLifecycleSmoke`; use `t.Fatal(err)` rather than `t.Skip`. Keep the existing no-opt-in skip.

- [ ] **Step 4: Pin the CLI**

Add to `.mise.toml`:

```toml
pulumi = "3.259.0"
```

Update `CONTRIBUTING.md` to state that `mise install` installs the Pulumi CLI used by acceptance and code generation.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./tests -run 'TestPulumiCLIAvailabilityHelper|TestRequirePulumiCLI|TestLifecycleSmokeProgram' -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add .mise.toml tests/acceptance_test.go CONTRIBUTING.md
git commit -m "build: pin Pulumi for acceptance tests"
```

### Task 2: Workflow CLI And Provider Preflight

**Files:**
- Modify: `.github/workflows/run-acceptance-tests.yml:58-73`
- Modify: `provider/registry_metadata_test.go:434-560`

**Interfaces:**
- Produces: workflow steps `Verify Pulumi CLI` and `Expose local provider` before live tests.

- [ ] **Step 1: Extend workflow contract tests**

Require this order:

```text
Setup Tools
Verify Pulumi CLI
Build codegen binaries
Build Schema
Build Provider
Expose local provider
Test Provider Library
```

Assert `Verify Pulumi CLI` runs `mise exec -- pulumi version`, and `Expose local provider` appends `${{ github.workspace }}/bin` to `$GITHUB_PATH` without printing directory contents.

- [ ] **Step 2: Run workflow tests and verify RED**

Run: `go test ./provider -run TestOwnedWorkflow -count=1`

Expected: FAIL because both preflight steps are absent.

- [ ] **Step 3: Add workflow preflights**

Insert after tool setup:

```yaml
- name: Verify Pulumi CLI
  run: mise exec -- pulumi version
```

Insert immediately after `Build Provider`:

```yaml
- name: Expose local provider
  run: echo "${{ github.workspace }}/bin" >> "$GITHUB_PATH"
```

- [ ] **Step 4: Verify GREEN**

Run: `go test ./provider -run TestOwnedWorkflow -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/run-acceptance-tests.yml provider/registry_metadata_test.go
git commit -m "ci: preflight Pulumi acceptance runtime"
```

### Task 3: Stop-Marker-Aware Failure Aggregation

**Files:**
- Modify: `.github/workflows/run-acceptance-tests.yml:73-161`
- Modify: `provider/registry_metadata_test.go:434-560`

**Interfaces:**
- Consumes: step outcomes for `tier1`, `tier2`, `tier3`, `tier4`, and `smoke`, plus each stop-marker gate output.
- Produces: final step `Report acceptance failures` that fails the job after all safe independent checks run.

- [ ] **Step 1: Write failing workflow assertions**

Assert every tier and smoke command has `continue-on-error: true`. Assert each gate uses `if: ${{ always() }}` and checks only marker presence. Assert later tier/smoke conditions depend on the previous gate's `marker_absent` output without `success()`. Assert a final `Report acceptance failures` step uses `if: ${{ always() }}` and inspects every test step outcome.

- [ ] **Step 2: Run workflow tests and verify RED**

Run: `go test ./provider -run TestOwnedWorkflow -count=1`

Expected: FAIL because current `success()` conditions suppress later safe coverage after an ordinary provider failure.

- [ ] **Step 3: Make test failures collectable**

Set `continue-on-error: true` on `tier1`, `tier2`, `tier3`, `tier4`, and `smoke`. Use `if: ${{ always() }}` on marker gates. Gate each later test only on the immediately preceding `marker_absent == 'true'` output.

- [ ] **Step 4: Add final aggregation**

After smoke, add:

```yaml
- name: Report acceptance failures
  if: ${{ always() }}
  run: |
    failed=0
    for outcome in \
      "${{ steps.tier1.outcome }}" \
      "${{ steps.tier2.outcome }}" \
      "${{ steps.tier3.outcome }}" \
      "${{ steps.tier4.outcome }}" \
      "${{ steps.smoke.outcome }}"; do
      if [ "$outcome" = "failure" ] || [ "$outcome" = "cancelled" ]; then
        failed=1
      fi
    done
    exit "$failed"
```

A skipped later step caused by a stop marker remains skipped; the gate that observed the marker already fails the job.

- [ ] **Step 5: Verify GREEN**

Run: `go test ./provider -run 'TestOwnedWorkflow|TestRegistryMetadata' -count=1`

Expected: PASS and workflow order remains serial.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/run-acceptance-tests.yml provider/registry_metadata_test.go
git commit -m "ci: preserve safe acceptance coverage after failures"
```

### Task 4: End-To-End Runner Verification

**Files:**
- Modify: `docs/bugs/2026-09-05-live-acceptance-run.md`

**Interfaces:**
- Consumes: Tasks 1-3 and protected acceptance credentials.
- Produces: proof that CLI/plugin preflight and lifecycle smoke execute rather than skip.

- [ ] **Step 1: Verify the pinned local toolchain**

Run: `mise install`

Run: `mise exec -- pulumi version`

Expected: Pulumi reports version `v3.259.0`.

- [ ] **Step 2: Build and expose the provider**

Run: `make provider`

Run: `PATH="$PWD/bin:$PATH" mise exec -- pulumi plugin ls`

Expected: commands succeed with the repository `bin` directory available for plugin discovery.

- [ ] **Step 3: Run static checks**

Run: `go test -short -count=1 ./provider/... ./internal/... ./tests/...`

Run: `git diff --check`

Expected: PASS.

- [ ] **Step 4: Run the focused live smoke**

After sourcing protected credentials without printing values:

```bash
PATH="$PWD/bin:$PATH" DOKPLOY_ACCEPTANCE=1 mise exec -- go test ./tests -run '^TestAccLifecycleSmoke$' -parallel=1 -count=1 -v
```

Expected: the test runs preview/up/refresh/update/destroy; it must not report a missing-CLI skip.

- [ ] **Step 5: Update the run summary**

Record the Pulumi version, provider revision, command outcome, duration, and cleanup status. Keep endpoint, stack state, IDs, config values, and credentials out of the report.

- [ ] **Step 6: Commit evidence**

```bash
git add docs/bugs/2026-09-05-live-acceptance-run.md
git commit -m "docs: record Pulumi lifecycle smoke result"
```
