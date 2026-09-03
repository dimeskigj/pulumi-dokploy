# Owned GitHub Workflows Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebase and repair `feature-owned-ci-github-releases` so the repository owns a portable, tested CI and GitHub Releases pipeline without `ci-mgmt` or Pulumi organization infrastructure.

**Architecture:** Keep six explicit GitHub Actions workflows with Go tests enforcing their trigger, permission, secret, artifact, and portability contracts. Build validation stays separate from stable/prerelease publishing, while manually dispatched live tests are the only workflow path that receives Dokploy credentials.

**Tech Stack:** GitHub Actions YAML, Go 1.25.13, `gopkg.in/yaml.v3`, Testify, GoReleaser, Pulumi SDK build tooling, mise

## Global Constraints

- Work only on `feature-owned-ci-github-releases`; leave `owned-ci-github-releases` untouched.
- Rebase onto current local `main` and push with `--force-with-lease`, never an unconditional force push.
- Keep exactly `build.yml`, `lint.yml`, `pages.yml`, `prerelease.yml`, `release.yml`, and `run-acceptance-tests.yml` under `.github/workflows/`.
- Use GitHub-hosted Ubuntu runners; no `pulumi-ubuntu-8core` labels.
- Keep third-party actions pinned to immutable 40-character commit SHAs.
- Keep `contents: write` limited to jobs that create releases or publish source/tag changes.
- Keep Dokploy secrets out of workflow metadata, build, lint, release, prerelease, pages, and language-only example jobs.
- Require `DOKPLOY_ENDPOINT`, `DOKPLOY_API_KEY`, `DOKPLOY_REGISTRY_URL`, `DOKPLOY_REGISTRY_USERNAME`, and `DOKPLOY_REGISTRY_PASSWORD`; treat `DOKPLOY_REGISTRY_IMAGE_PREFIX` as optional.
- Preserve all provider resources, schema, examples, and documentation currently on `main`.
- Do not restore `.ci-mgmt.yaml`, `scripts/normalize_ci.py`, `scripts/test_normalize_ci.py`, or the `ci-mgmt` Make target.

---

### Task 1: Rebase And Reconcile Current Main

**Files:**
- Modify through conflict resolution: `.github/workflows/run-acceptance-tests.yml`
- Modify through conflict resolution: `provider/registry_metadata_test.go`
- Modify through conflict resolution: `README.md`
- Modify through conflict resolution: `CONTRIBUTING.md`
- Modify through conflict resolution: `website/src/content/docs/getting-started/installation.mdx`
- Preserve deletion: `.ci-mgmt.yaml`
- Preserve deletion: `scripts/normalize_ci.py`
- Preserve deletion: `scripts/test_normalize_ci.py`
- Preserve owned versions: `.github/workflows/build.yml`
- Preserve owned versions: `.github/workflows/lint.yml`
- Preserve owned versions: `.github/workflows/prerelease.yml`
- Preserve owned versions: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: local `main` at the merged SSH/registry/tag/mount implementation and remote feature tip recorded before rewriting.
- Produces: a linear feature branch based on `main`, with the owned workflow surface and all current provider assertions present.

- [ ] **Step 1: Fetch and record the lease target**

Run:

```bash
git fetch origin main feature-owned-ci-github-releases
git rev-parse origin/feature-owned-ci-github-releases
git merge-base --is-ancestor main HEAD || true
```

Expected: the remote feature tip is printed; the ancestor check may fail before the rebase.

- [ ] **Step 2: Rebase the feature branch onto main**

Run:

```bash
git rebase main
```

Expected: Git either completes or stops at conflicts caused by changes made after commit `71ff549`.

- [ ] **Step 3: Resolve conflicts using the approved ownership rules**

For every stopped commit:

```bash
git status --short
```

Apply these exact rules, stage only resolved files, and run `git rebase --continue`:

- Keep the feature branch's six-workflow file set and deletions of generated ancillary workflows.
- Keep deletion of `.ci-mgmt.yaml` and both normalization scripts.
- In `provider/registry_metadata_test.go`, keep the owned `workflowJobPolicy` and owned-workflow helpers, plus `main`'s `require.Len(t, spec.Resources, 18)` and assertions for `SSHKey`, `Registry`, `Tag`, `ProjectTag`, and `Mount`.
- In README and website conflicts, keep GitHub Releases installation instructions plus all SSH key, registry, tag, project-tag, and mount documentation from `main`.
- In acceptance conflicts, keep the owned manual-dispatch structure; Task 3 adds the final registry credential contract.
- Abort rather than guess if a conflict affects provider behavior outside these files.

Run after each resolution batch; `-u` stages only tracked modifications and deletions reported by the preceding status check:

```bash
git add -u
git rebase --continue
```

Expected: rebase completes without skipped owned-workflow commits.

- [ ] **Step 4: Verify branch topology and baseline tests**

Run:

```bash
git merge-base --is-ancestor main HEAD
test "$(git rev-list --count main..HEAD)" -ge 3
mise exec -- go test ./provider -run 'TestRegistryMetadata|TestWorkflow' -count=1
git status --short
```

Expected: ancestor and commit-count checks pass, workflow tests pass or expose only the portability/acceptance gaps addressed in Tasks 3 and 4, and the worktree is clean.

### Task 2: Preserve Owned Build Validation

**Files:**
- Modify: `provider/registry_metadata_test.go`
- Modify: `.github/workflows/build.yml`
- Modify: `.github/workflows/lint.yml`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `readWorkflow`, `workflowJobRunSteps`, `hasExactRunCommand`, and `validateWorkflowSemantics` in `provider/registry_metadata_test.go`.
- Produces: exact build gate assertions and a build workflow that calls the repository's existing Make targets.

- [ ] **Step 1: Add failing owned-surface and build-gate assertions**

Ensure `workflowJobPolicy` classifies exactly these files and jobs:

```go
var workflowJobPolicy = map[string]map[string]bool{
	"build.yml":                {"prerequisites": true, "build_sdks": true, "test": true, "lint": true},
	"lint.yml":                 {"lint": true},
	"pages.yml":                {"build": false, "deploy": false},
	"prerelease.yml":           {"prerequisites": true, "build_sdks": true, "test": true, "publish": true, "publish_sdk": true, "publish_java_sdk": true, "publish_go_sdk": true},
	"release.yml":              {"prerequisites": true, "build_sdks": true, "test": true, "publish": true, "publish_sdk": true, "publish_java_sdk": true, "publish_go_sdk": true, "dispatch_docs_build": false},
	"run-acceptance-tests.yml": {"prerequisites": true, "build_sdks": true, "test": true, "lint": true},
}
```

Keep or add exact build-gate assertions:

```go
gateRuns := map[string]string{
	"lint": "make lint", "race": "make test_race",
	"openapi and codegen": "make check_openapi && make check_codegen",
	"SDKs": "make build_sdks", "examples": "make test_examples",
	"vulnerability": "make govulncheck", "license": "make license",
}
```

Also assert that `.ci-mgmt.yaml`, `scripts/normalize_ci.py`, and `scripts/test_normalize_ci.py` do not exist and that `Makefile` does not contain a `ci-mgmt:` target.

- [ ] **Step 2: Run the focused test and identify any reconciliation drift**

Run:

```bash
mise exec -- go test ./provider -run TestRegistryMetadata -count=1
```

Expected: PASS when the rebase retained the existing owned build behavior. Any failure identifies generated workflow drift or a dropped current resource assertion that Step 3 must repair.

- [ ] **Step 3: Repair build and lint workflows minimally**

Make `.github/workflows/build.yml` use these triggers:

```yaml
on:
  pull_request: {}
  push:
    branches: [main, feature-**]
  workflow_dispatch: {}
```

In `prerequisites`, retain exact run steps for:

```yaml
- name: Check documentation
  run: make docs_check
- name: Check OpenAPI and generated SDKs
  run: make check_openapi && make check_codegen
- name: Run race tests
  run: make test_race
- name: Build all SDKs
  run: make build_sdks
- name: Test all examples
  run: make test_examples
- name: Vulnerability scan
  run: make govulncheck
- name: License scan
  run: make license
```

Keep `lint` as a reusable workflow invoked by both build and acceptance workflows. Remove only generator-related Make targets; retain all current provider, SDK, example, OpenAPI, and docs targets.

- [ ] **Step 4: Run focused and baseline verification**

Run:

```bash
mise exec -- go test ./provider -run 'TestRegistryMetadata|TestWorkflow' -count=1
mise exec -- make lint
git diff --check
```

Expected: all commands pass.

- [ ] **Step 5: Commit the build reconciliation**

```bash
git add .github/workflows/build.yml .github/workflows/lint.yml Makefile provider/registry_metadata_test.go
git commit -m "ci: reconcile owned build workflows"
```

### Task 3: Isolate Live Acceptance Credentials

**Files:**
- Modify: `provider/registry_metadata_test.go`
- Modify: `.github/workflows/run-acceptance-tests.yml`

**Interfaces:**
- Consumes: `TestLive*` tests in `provider/live_test.go` and repository/environment secrets named in Global Constraints.
- Produces: a manually dispatched acceptance workflow whose `prerequisites` job is the sole Dokploy-secret consumer.

- [ ] **Step 1: Write failing acceptance contract assertions**

In `TestRegistryMetadata`, require the explicit live command and credential preflight:

```go
require.Contains(t, acceptanceText, "mise exec -- go test ./provider -run TestLive -v -count=1")
require.Contains(t, acceptanceText, `test -n "$DOKPLOY_ENDPOINT" && test -n "$DOKPLOY_API_KEY" && test -n "$DOKPLOY_REGISTRY_URL" && test -n "$DOKPLOY_REGISTRY_USERNAME" && test -n "$DOKPLOY_REGISTRY_PASSWORD"`)
for _, variable := range []string{
	"DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY", "DOKPLOY_REGISTRY_URL",
	"DOKPLOY_REGISTRY_USERNAME", "DOKPLOY_REGISTRY_PASSWORD", "DOKPLOY_REGISTRY_IMAGE_PREFIX",
} {
	require.Contains(t, acceptanceText, variable+": ${{ secrets."+variable+" }}")
}
```

Change the protected-job assertion to:

```go
require.ElementsMatch(t, []string{"prerequisites"}, protectedJobs)
```

Scan every workflow and require any `secrets.DOKPLOY_` reference to occur only within the acceptance `prerequisites` job.

- [ ] **Step 2: Run the focused test to verify the stale branch fails**

Run:

```bash
mise exec -- go test ./provider -run TestRegistryMetadata -count=1
```

Expected: FAIL because the old owned acceptance workflow lacks registry secrets and an explicit live test, and exposes endpoint/API credentials to the language matrix.

- [ ] **Step 3: Add registry preflight and explicit live tests**

In the acceptance `prerequisites` job, use:

```yaml
environment: dokploy-acceptance
```

Add this preflight step:

```yaml
- name: Require Dokploy acceptance credentials
  env:
    DOKPLOY_ENDPOINT: ${{ secrets.DOKPLOY_ENDPOINT }}
    DOKPLOY_API_KEY: ${{ secrets.DOKPLOY_API_KEY }}
    DOKPLOY_REGISTRY_URL: ${{ secrets.DOKPLOY_REGISTRY_URL }}
    DOKPLOY_REGISTRY_USERNAME: ${{ secrets.DOKPLOY_REGISTRY_USERNAME }}
    DOKPLOY_REGISTRY_PASSWORD: ${{ secrets.DOKPLOY_REGISTRY_PASSWORD }}
    DOKPLOY_REGISTRY_IMAGE_PREFIX: ${{ secrets.DOKPLOY_REGISTRY_IMAGE_PREFIX }}
  run: test -n "$DOKPLOY_ENDPOINT" && test -n "$DOKPLOY_API_KEY" && test -n "$DOKPLOY_REGISTRY_URL" && test -n "$DOKPLOY_REGISTRY_USERNAME" && test -n "$DOKPLOY_REGISTRY_PASSWORD"
```

Before packaging, add:

```yaml
- name: Test live provider resources
  run: mise exec -- go test ./provider -run TestLive -v -count=1
  env:
    DOKPLOY_ENDPOINT: ${{ secrets.DOKPLOY_ENDPOINT }}
    DOKPLOY_API_KEY: ${{ secrets.DOKPLOY_API_KEY }}
    DOKPLOY_REGISTRY_URL: ${{ secrets.DOKPLOY_REGISTRY_URL }}
    DOKPLOY_REGISTRY_USERNAME: ${{ secrets.DOKPLOY_REGISTRY_USERNAME }}
    DOKPLOY_REGISTRY_PASSWORD: ${{ secrets.DOKPLOY_REGISTRY_PASSWORD }}
    DOKPLOY_REGISTRY_IMAGE_PREFIX: ${{ secrets.DOKPLOY_REGISTRY_IMAGE_PREFIX }}
```

Remove `environment: dokploy-acceptance` and all `DOKPLOY_*` entries from the language `test` matrix; those tests compile and bind examples but do not contact Dokploy. Keep lint unprotected and do not pass caller secrets to it.

- [ ] **Step 4: Verify secret isolation and ordinary provider tests**

Run:

```bash
mise exec -- go test ./provider -run 'TestRegistryMetadata|TestWorkflowStepsDoNotHaveEmptyEnvMappings' -count=1
mise exec -- go test ./provider/... ./internal/... -short -count=1
```

Expected: PASS. Do not run `TestLive` locally without the protected environment credentials.

- [ ] **Step 5: Commit acceptance repairs**

```bash
git add .github/workflows/run-acceptance-tests.yml provider/registry_metadata_test.go
git commit -m "ci: secure owned acceptance workflow"
```

### Task 4: Make Release Workflows Portable And Consistent

**Files:**
- Modify: `provider/registry_metadata_test.go`
- Modify: `.github/workflows/prerelease.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `.goreleaser.yml`
- Modify: `.goreleaser.prerelease.yml`

**Interfaces:**
- Consumes: provider artifact named `pulumi-dokploy-provider.tar.gz` and language artifacts named `<language>-sdk.tar.gz`.
- Produces: mutually exclusive stable/prerelease GitHub release pipelines with portable runners and matching artifact paths.

- [ ] **Step 1: Add failing portability and artifact assertions**

After collecting `allWorkflowText`, add:

```go
for _, forbidden := range []string{
	"pulumi-ubuntu-8core", "pulumi-gen-",
	"pulumi/esc-action@", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
	"AWS_UPLOAD_ROLE_ARN", "blobs:",
} {
	require.NotContains(t, allWorkflowText, forbidden)
}
```

Keep stable and prerelease trigger assertions:

```go
require.Equal(t, []any{"v*.*.*", "!v*.*.*-*"}, release["on"].(map[string]any)["push"].(map[string]any)["tags"])
require.Equal(t, []any{"v*.*.*-*"}, prerelease["on"].(map[string]any)["push"].(map[string]any)["tags"])
```

For both workflows, assert `publish` has `contents: write`, `publish_sdk` needs `publish`, `publish_go_sdk` needs `publish_sdk`, and the Go publisher receives `${{ steps.version.outputs.version }}`.

- [ ] **Step 2: Run focused tests and verify known failures**

Run:

```bash
mise exec -- go test ./provider -run TestRegistryMetadata -count=1
```

Expected: FAIL on `pulumi-ubuntu-8core` and/or a `pulumi-gen-` archive reference.

- [ ] **Step 3: Repair runners, provider archives, and release dependencies**

In both release workflows:

- Replace every `runs-on: pulumi-ubuntu-8core` with `runs-on: ubuntu-latest`.
- Package only the built provider binary:

```yaml
- name: Tar provider binaries
  run: tar -zcf ${{ github.workspace }}/bin/provider.tar.gz -C ${{ github.workspace }}/bin/ pulumi-resource-${{ env.PROVIDER }}
```

- Keep provider and SDK artifact upload/download names identical.
- Keep stable release on `.goreleaser.yml` and prerelease on `.goreleaser.prerelease.yml`.
- Keep `publish_sdk` dependent on `publish`, and `publish_go_sdk` dependent on `publish_sdk`.
- Keep `contents: write` on `publish` and `publish_go_sdk`; keep read-only permissions elsewhere unless a publishing action demonstrably requires more.

In both GoReleaser configurations, retain:

```yaml
release:
  prerelease: auto
  disable: false
```

Do not restore an S3 `blobs` section.

- [ ] **Step 4: Verify workflow and GoReleaser contracts**

Run:

```bash
mise exec -- go test ./provider -run 'TestRegistryMetadata|TestWorkflowStepsDoNotHaveEmptyEnvMappings' -count=1
if command -v goreleaser >/dev/null 2>&1; then goreleaser check --config .goreleaser.yml && goreleaser check --config .goreleaser.prerelease.yml; else printf '%s\n' 'SKIP: goreleaser is not installed'; fi
git diff --check
```

Expected: Go tests and diff check pass; GoReleaser checks pass when the binary is installed, otherwise the command emits the explicit skip message.

- [ ] **Step 5: Commit release portability repairs**

```bash
git add .github/workflows/release.yml .github/workflows/prerelease.yml .goreleaser.yml .goreleaser.prerelease.yml provider/registry_metadata_test.go
git commit -m "ci: make owned release workflows portable"
```

### Task 5: Full Review, Verification, And Push

**Files:**
- Verify: all files changed by `main...HEAD`
- Verify: `docs/superpowers/specs/2026-09-02-owned-github-workflows-design.md`
- Verify: `docs/superpowers/plans/2026-09-02-owned-github-workflows.md`

**Interfaces:**
- Consumes: completed owned workflow branch and original remote feature tip used as the force-with-lease safety boundary.
- Produces: reviewed and pushed `origin/feature-owned-ci-github-releases`.

- [ ] **Step 1: Audit the final workflow surface and pins**

Run:

```bash
test "$(ls .github/workflows/*.yml | wc -l | tr -d ' ')" = 6
test ! -e .ci-mgmt.yaml
test ! -e scripts/normalize_ci.py
test ! -e scripts/test_normalize_ci.py
! rg 'pulumi-ubuntu-8core|pulumi-gen-|pulumi/esc-action|AWS_(ACCESS_KEY_ID|SECRET_ACCESS_KEY|UPLOAD_ROLE_ARN)|blobs:' .github/workflows .goreleaser.yml .goreleaser.prerelease.yml
rg 'uses:' .github/workflows
```

Expected: exactly six workflows, removed generator inputs remain absent, forbidden dependencies produce no matches, and every displayed third-party action reference ends in a 40-character SHA before any comment.

- [ ] **Step 2: Run the complete local verification suite**

Run each command independently and stop on the first failure:

```bash
mise exec -- go test ./...
mise exec -- make test_race
mise exec -- make lint
mise exec -- make check_openapi
mise exec -- make check_codegen
mise exec -- make build_sdks
mise exec -- make test_examples
mise exec -- make docs_check
mise exec -- make docs_build
git diff --check
```

Expected: every command exits zero. Pulumi may print an ephemeral account claim URL during generation; report it to the user after work is complete and do not claim it during active automation.

- [ ] **Step 3: Review the branch diff and commits**

Run:

```bash
git status --short
git diff --stat main...HEAD
git diff --check main...HEAD
git log --oneline --decorate main..HEAD
git merge-base --is-ancestor main HEAD
```

Expected: clean worktree, focused owned-workflow diff, no whitespace errors, reviewable commits, and `main` is an ancestor.

- [ ] **Step 4: Push only the feature branch safely**

Run:

```bash
git fetch origin feature-owned-ci-github-releases
git push --force-with-lease origin HEAD:feature-owned-ci-github-releases
```

Expected: push succeeds without updating `owned-ci-github-releases`. If the lease is rejected, stop and inspect the new remote commits; do not retry with unconditional force.

- [ ] **Step 5: Confirm remote synchronization**

Run:

```bash
test "$(git rev-parse HEAD)" = "$(git rev-parse origin/feature-owned-ci-github-releases)"
git status --short
```

Expected: local and remote feature tips match and the worktree is clean.
