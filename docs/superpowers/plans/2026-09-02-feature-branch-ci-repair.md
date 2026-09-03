# Feature Branch CI Repair Implementation Plan

## User-directed amendment

The active branch is `feat/owned-ci-github-releases`, and the active build
filter is `feat/**`. The retained `feature/owned-ci-github-releases` and
`feature-owned-ci-github-releases` branches are aliases at the final commit.
This supersedes earlier `feature/` references in this document.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adopt the `feature/` branch namespace, make every remote SDK test job execute from `examples/`, remove nonfunctional external release dependencies, and document the repository secrets needed to publish.

**Architecture:** Extend the existing parsed-YAML workflow contracts before changing the owned workflows, so branch filters, shell command boundaries, release job sets, and secret references remain executable policy. Keep maintainer setup guidance in `CONTRIBUTING.md`, then migrate and push the branch only after all local checks pass.

**Tech Stack:** GitHub Actions YAML, Go 1.26.6, `gopkg.in/yaml.v3`, Testify, Git, GitHub Actions REST API

## Global Constraints

- Use the active branch name `feature/owned-ci-github-releases` and the build push filter `feature/**`.
- Keep remote `feature-owned-ci-github-releases` as an alias at the same final commit.
- Do not modify remote `owned-ci-github-releases`.
- Keep GitHub Releases and npm, PyPI, NuGet, Maven Central, and Go SDK publication.
- Remove Azure signing inputs because the current GoReleaser signing hook is a no-op.
- Remove `JAVA_SIGNING_KEY_ID` because `sdk/java/build.gradle` does not consume it.
- Remove the stable Pulumi registry docs-dispatch job; Pages remains the owned documentation publisher.
- Do not trigger stable or prerelease publication during verification.
- Require a successful remote build, including every SDK matrix test, on `feature/owned-ci-github-releases`.

---

### Task 1: Repair SDK Test Execution

**Files:**
- Modify: `provider/registry_metadata_test.go:243-325`
- Modify: `.github/workflows/build.yml:5-9,197-199`
- Modify: `.github/workflows/release.yml:221-225`
- Modify: `.github/workflows/prerelease.yml:221-225`

**Interfaces:**
- Consumes: `readWorkflow`, `workflowJobRunSteps`, and `hasExactRunCommand` in `provider/registry_metadata_test.go`
- Produces: Parsed workflow contracts requiring `feature/**` and a standalone examples test command in all three automated workflows

- [ ] **Step 1: Write failing branch and command-boundary assertions**

In `TestRegistryMetadataMatchesSchema`, change the build branch expectation and add this assertion after loading `release` and `prerelease`:

```go
require.Equal(t, []any{"main", "feature/**"}, push["branches"])

sdkTestCommand := "cd examples && $GO_TEST_EXEC ./sdk/${{ matrix.language }}/examples/... -v -count=1 -coverprofile=coverage.txt -coverpkg=github.com/dimeskigj/pulumi-dokploy/sdk/${{ matrix.language }}/..."
for name, workflow := range map[string]map[string]any{
	"build.yml":      build,
	"release.yml":    release,
	"prerelease.yml": prerelease,
} {
	require.True(t, hasExactRunCommand(workflowJobRunSteps(workflow, "test"), sdkTestCommand), "%s SDK test must run from examples", name)
}
```

- [ ] **Step 2: Run the focused test and verify both contracts fail**

Run: `mise exec -- go test ./provider -run TestRegistryMetadataMatchesSchema -count=1`

Expected: FAIL because build still contains `feature-**` and the folded build test script does not expose `cd examples && ...` as a separate line.

- [ ] **Step 3: Use the slash branch filter and literal test scripts**

In `.github/workflows/build.yml`, replace `feature-**` with `feature/**`. In the `test` job of build, release, and prerelease, use this literal script:

```yaml
    - name: Run tests
      run: |
        set -euo pipefail
        cd examples && $GO_TEST_EXEC ./sdk/${{ matrix.language }}/examples/... -v -count=1 -coverprofile=coverage.txt -coverpkg=github.com/dimeskigj/pulumi-dokploy/sdk/${{ matrix.language }}/...
```

- [ ] **Step 4: Run focused and full workflow contract tests**

Run: `mise exec -- go test ./provider -run 'TestRegistryMetadataMatchesSchema|TestWorkflowSemanticFixturesRejectMetadataAndWrongJobs' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the workflow repair**

```bash
git add provider/registry_metadata_test.go .github/workflows/build.yml .github/workflows/release.yml .github/workflows/prerelease.yml
git commit -m "fix: run SDK tests from examples directory"
```

### Task 2: Remove Nonfunctional Release Dependencies

**Files:**
- Modify: `provider/registry_metadata_test.go:95-101,230-325`
- Modify: `.github/workflows/release.yml:262-273,379-387,420-443`
- Modify: `.github/workflows/prerelease.yml:262-273,379-387`

**Interfaces:**
- Consumes: `workflowJobPolicy`, `readWorkflow`, and workflow text collected by `TestRegistryMetadataMatchesSchema`
- Produces: Release workflows with only functional, repository-owned jobs and consumed credentials

- [ ] **Step 1: Tighten release dependency contracts**

Remove `dispatch_docs_build` from the `release.yml` entry in `workflowJobPolicy`. After loading `releaseText` and `prereleaseText`, add:

```go
for _, workflowText := range []string{releaseText, prereleaseText} {
	for _, unused := range []string{"AZURE_SIGNING_", "SKIP_SIGNING", "JAVA_SIGNING_KEY_ID"} {
		require.NotContains(t, workflowText, unused)
	}
}
require.NotContains(t, releaseText, "dispatch_docs_build")
require.NotContains(t, releaseText, "pulumictl create docs-build")
```

- [ ] **Step 2: Run the focused test and verify obsolete dependencies fail**

Run: `mise exec -- go test ./provider -run TestRegistryMetadataMatchesSchema -count=1`

Expected: FAIL because the stable job policy no longer matches, both workflows contain Azure and Java key-ID variables, and stable release still dispatches Pulumi registry docs.

- [ ] **Step 3: Remove unused signing inputs and external docs dispatch**

In both release workflows, leave only these GoReleaser environment values:

```yaml
      env:
        GORELEASER_CURRENT_TAG: v${{ steps.version.outputs.version }}
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

In both Java publication steps, remove only this line:

```yaml
        SIGNING_KEY_ID: ${{ secrets.JAVA_SIGNING_KEY_ID }}
```

Delete the complete `dispatch_docs_build` job from `.github/workflows/release.yml`. Do not alter the Pages workflow or package publication jobs.

- [ ] **Step 4: Run the workflow contract tests**

Run: `mise exec -- go test ./provider -run 'TestRegistryMetadataMatchesSchema|TestReleaseWorkflowContractsRejectRepresentativeDrift|TestWorkflowStepsDoNotHaveEmptyEnvMappings' -count=1`

Expected: PASS.

- [ ] **Step 5: Search active workflow files for removed dependencies**

Search `.github/workflows/*.yml` for `AZURE_SIGNING_`, `SKIP_SIGNING`, `JAVA_SIGNING_KEY_ID`, `dispatch_docs_build`, and `pulumictl create docs-build`.

Expected: no matches.

- [ ] **Step 6: Commit release simplification**

```bash
git add provider/registry_metadata_test.go .github/workflows/release.yml .github/workflows/prerelease.yml
git commit -m "ci: remove unused release dependencies"
```

### Task 3: Document Publication Credentials

**Files:**
- Modify: `provider/registry_metadata_test.go:368-385`
- Modify: `CONTRIBUTING.md:1-10`

**Interfaces:**
- Consumes: Secret names used by `.github/workflows/release.yml` and `.github/workflows/prerelease.yml`
- Produces: Maintainer instructions for configuring every external package publication credential

- [ ] **Step 1: Add a failing documentation contract**

After reading the Makefile in `TestRegistryMetadataMatchesSchema`, read `CONTRIBUTING.md` and require the setup location, all publication secrets, and the automatic token note:

```go
contributing, err := os.ReadFile("../CONTRIBUTING.md")
require.NoError(t, err)
contributingText := string(contributing)
require.Contains(t, contributingText, "Settings > Secrets and variables > Actions")
for _, secret := range []string{
	"NUGET_PUBLISH_KEY", "NPM_TOKEN", "PYPI_API_TOKEN",
	"JAVA_SIGNING_KEY", "JAVA_SIGNING_PASSWORD",
	"OSSRH_USERNAME", "OSSRH_PASSWORD", "CODECOV_TOKEN",
} {
	require.Contains(t, contributingText, "`"+secret+"`")
}
require.Contains(t, contributingText, "`GITHUB_TOKEN` is created automatically")
```

- [ ] **Step 2: Run the focused test and verify documentation is missing**

Run: `mise exec -- go test ./provider -run TestRegistryMetadataMatchesSchema -count=1`

Expected: FAIL because `CONTRIBUTING.md` does not yet describe release secrets.

- [ ] **Step 3: Add the release credentials section**

Append this content to `CONTRIBUTING.md`:

```markdown
## Release credentials

Before creating a stable or prerelease tag, configure package publication
credentials under **Settings > Secrets and variables > Actions > Repository
secrets**:

| Secret | Setup |
| --- | --- |
| `NUGET_PUBLISH_KEY` | Create an API key at NuGet.org with push access to the `Pulumi.Dokploy` package. |
| `NPM_TOKEN` | Create an npm automation or granular access token with publish access to the `@dimeskigj/pulumi-dokploy` package. |
| `PYPI_API_TOKEN` | Create a PyPI API token scoped to the `pulumi-dokploy` project; use an account-scoped token for the first publication. |
| `JAVA_SIGNING_KEY` | Export the ASCII-armored private PGP key used to sign Maven artifacts, including the `BEGIN` and `END` lines. |
| `JAVA_SIGNING_PASSWORD` | Set the passphrase for `JAVA_SIGNING_KEY`. |
| `OSSRH_USERNAME` | Set the Sonatype Central publishing username or user-token name. |
| `OSSRH_PASSWORD` | Set the matching Sonatype Central password or user-token value. |

`CODECOV_TOKEN` is optional and enables authenticated coverage uploads.
`GITHUB_TOKEN` is created automatically for each workflow run; do not create a
repository secret for it.

The manually dispatched live acceptance workflow uses separate protected
Dokploy and registry secrets in the `dokploy-acceptance` environment.
```

- [ ] **Step 4: Run the focused test**

Run: `mise exec -- go test ./provider -run TestRegistryMetadataMatchesSchema -count=1`

Expected: PASS.

- [ ] **Step 5: Commit maintainer documentation**

```bash
git add provider/registry_metadata_test.go CONTRIBUTING.md
git commit -m "docs: describe release credentials"
```

### Task 4: Verify, Migrate, and Push the Branch

**Files:**
- Verify only: all tracked files
- Rename branch: `feature-owned-ci-github-releases` to `feature/owned-ci-github-releases`

**Interfaces:**
- Consumes: Completed commits from Tasks 1-3 and remote `origin`
- Produces: Two synchronized remote feature refs and a successful build run on the slash-named branch

- [ ] **Step 1: Run local verification from a clean candidate commit**

Run: `mise exec -- go test ./...`

Expected: PASS for every Go package.

Run: `git diff --check`

Expected: no output.

Run: `git status --short`

Expected: no output.

- [ ] **Step 2: Rename the active local branch**

Run: `git branch -m feature/owned-ci-github-releases`

Expected: `git branch --show-current` prints `feature/owned-ci-github-releases`.

- [ ] **Step 3: Push the slash-named branch and set its upstream**

Run: `git push -u origin feature/owned-ci-github-releases`

Expected: Git creates or updates `origin/feature/owned-ci-github-releases` and sets upstream tracking.

- [ ] **Step 4: Synchronize the retained hyphenated alias**

Run: `git push origin HEAD:feature-owned-ci-github-releases`

Expected: `origin/feature-owned-ci-github-releases` advances to the same commit. Do not push to or delete `origin/owned-ci-github-releases`.

- [ ] **Step 5: Locate the slash-branch build run**

Query:

```text
https://api.github.com/repos/dimeskigj/pulumi-dokploy/actions/workflows/build.yml/runs?branch=feature%2Fowned-ci-github-releases&per_page=10
```

Expected: the newest run whose `head_sha` equals `git rev-parse HEAD` appears with event `push`.

- [ ] **Step 6: Poll the run and inspect every job**

Poll the run URL returned by the API until `status` is `completed`, then query its `jobs_url`.

Expected: run conclusion `success`; `prerequisites`, `build_sdks`, `lint`, and all `test (<language>)` jobs conclude `success`. No stable or prerelease workflow is triggered.

- [ ] **Step 7: Handle any remote-only failure systematically**

If the run is not successful, invoke `superpowers:systematic-debugging`, collect the failing job and step from the Actions API, reproduce it locally where possible, add a regression test, commit the minimal fix, push both refs again, and repeat Steps 5-6. Do not report completion while any latest slash-branch build is failing or canceled.

- [ ] **Step 8: Confirm synchronized refs**

Run: `git ls-remote --heads origin feature/owned-ci-github-releases feature-owned-ci-github-releases owned-ci-github-releases`

Expected: the slash and retained hyphenated feature refs have the same SHA; `owned-ci-github-releases` remains at its pre-task SHA.
