# Registry Release Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a technically publishable Pulumi Registry release baseline with attributable API traffic, automatic plugin acquisition, Registry documentation, compatibility gates, artifact integrity, and clean-consumer verification.

**Architecture:** Runtime and schema metadata remain owned by the inferred provider, while Registry docs stay handwritten and generated SDKs remain generator-owned. Workflow changes are protected by semantic contract tests; release integrity uses checksums, SBOMs, and GitHub OIDC attestations without persistent signing credentials.

**Tech Stack:** Go 1.26.6, Pulumi Go Provider v1.6.0, Pulumi CLI 3.259.0, GitHub Actions, GoReleaser, npm, PyPI, NuGet, Maven Central, Go modules, Node test runner, Testify.

## Global Constraints

- Repository coordinates remain `dimeskigj/pulumi-dokploy` and provider name remains `dokploy`.
- Do not change existing Pulumi resource tokens or package names.
- Do not add `logoUrl`; visual asset selection is deferred.
- Do not invent personal names, security email addresses, publisher display names, or governance owners.
- Do not submit a Pulumi Registry pull request or publish a release.
- Do not add lookup functions or broaden live Pulumi-engine acceptance in this plan.
- Generated schema and SDK files must be regenerated, never manually edited.
- Stable and prerelease workflows must apply equivalent integrity controls.
- Release integrity must use GitHub OIDC and must not add long-lived signing credentials.
- CI and smoke tests must not print credentials, Pulumi secrets, endpoints, or package tokens.
- Existing user changes and unrelated files must remain untouched.

---

## File Structure

- `internal/client/client.go` and `internal/client/client_test.go`: own User-Agent construction and transport behavior.
- `provider/config.go`, `provider/provider.go`, and provider tests: pass provider version and own schema metadata.
- `provider/cmd/pulumi-resource-dokploy/schema.json` and `sdk/`: generator-owned metadata outputs.
- `docs/_index.md` and `docs/installation-configuration.md`: handwritten Pulumi Registry content.
- `LICENSE`: canonical Apache 2.0 license.
- `.github/workflows/build.yml`: pull-request schema compatibility gate.
- `.github/workflows/release.yml`, `.github/workflows/prerelease.yml`, `.goreleaser*.yml`: artifact publication, checksums, SBOMs, and attestations.
- `.github/workflows/release-smoke.yml`: exact-version, clean-cache consumer verification.
- `provider/registry_metadata_test.go`: workflow and release contract tests.
- `docs/provider-registry-readiness.md`: completed/deferred readiness ledger.

### Task 1: Provider-Specific User-Agent

**Files:**
- Modify: `internal/client/client.go:14-95`
- Modify: `internal/client/client_test.go:17-78`
- Modify: `provider/config.go:27-45`
- Test: `provider/config_test.go`

**Interfaces:**
- Produces: `func WithUserAgentVersion(version string) Option` and `func providerUserAgent(version string) string` in `internal/client`.
- Consumes: linker-injected `provider.Version` through `client.New(..., client.WithUserAgentVersion(Version))`.

- [ ] **Step 1: Write failing client header tests**

Extend the existing header server test with:

```go
func TestNewSendsProviderUserAgent(t *testing.T) {
	server := newHeaderServer(t)
	c, err := New(server.server.URL, "key", WithUserAgentVersion("v1.2.3"))
	require.NoError(t, err)
	_, err = c.ProjectOneWithResponse(t.Context(), &generated.ProjectOneParams{ProjectId: "p1"})
	require.NoError(t, err)
	require.Equal(t, "pulumi-dokploy/1.2.3", server.LastRequest().Header.Get("User-Agent"))
}

func TestNewUsesDevelopmentUserAgentFallback(t *testing.T) {
	for _, version := range []string{"", "   "} {
		server := newHeaderServer(t)
		c, err := New(server.server.URL, "key", WithUserAgentVersion(version))
		require.NoError(t, err)
		_, err = c.ProjectOneWithResponse(t.Context(), &generated.ProjectOneParams{ProjectId: "p1"})
		require.NoError(t, err)
		require.Equal(t, "pulumi-dokploy/dev", server.LastRequest().Header.Get("User-Agent"))
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./internal/client -run '^TestNew(SendsProviderUserAgent|UsesDevelopmentUserAgentFallback)$' -count=1`

Expected: compilation fails because `WithUserAgentVersion` does not exist.

- [ ] **Step 3: Implement normalized User-Agent construction**

Add `userAgentVersion` to `clientOptions`, implement `WithUserAgentVersion`, strip one leading `v`, map empty values to `dev`, and set exactly `pulumi-dokploy/<version>` in the request editor. Keep endpoint and API-key validation unchanged.

- [ ] **Step 4: Test retry preservation**

Add a test transport/server that returns one transient GET response followed by success, records every `User-Agent`, and asserts both attempts equal `pulumi-dokploy/1.2.3`.

Run: `go test ./internal/client -run 'UserAgent' -count=1`

Expected: PASS.

- [ ] **Step 5: Wire the provider version**

Update provider configuration client construction to pass:

```go
client.WithUserAgentVersion(Version)
```

Add or update the configuration test to assert the configured client sends the linker version. Do not import `provider` from `internal/client`.

- [ ] **Step 6: Run focused and package tests**

Run: `go test ./internal/client ./provider -run 'UserAgent|Config' -count=1`

Expected: PASS.

Run: `go test -short ./provider/... ./internal/... -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/client/client.go internal/client/client_test.go provider/config.go provider/config_test.go
git commit -m "feat: identify provider API requests"
```

### Task 2: Plugin Download And Discovery Metadata

**Files:**
- Modify: `provider/provider.go:20-35`
- Modify: `provider/provider_test.go`
- Modify: `provider/schema_test.go`
- Regenerate: `provider/cmd/pulumi-resource-dokploy/schema.json`
- Regenerate: `sdk/go`, `sdk/nodejs`, `sdk/python`, `sdk/dotnet`, `sdk/java`

**Interfaces:**
- Produces schema `pluginDownloadURL`: `github://api.github.com/dimeskigj/pulumi-dokploy`.
- Produces schema keywords: `category/infrastructure`, `kind/native`, `dokploy`, `deployment`, `self-hosted`, `paas`.

- [ ] **Step 1: Write failing schema metadata assertions**

In provider schema tests assert exact values:

```go
require.Equal(t, "github://api.github.com/dimeskigj/pulumi-dokploy", spec.PluginDownloadURL)
require.ElementsMatch(t, []string{
	"category/infrastructure", "kind/native", "dokploy",
	"deployment", "self-hosted", "paas",
}, spec.Keywords)
require.Empty(t, spec.LogoURL)
```

Add generated metadata assertions for `pulumi-plugin.json` and language utilities so an empty plugin download URL cannot recur.

- [ ] **Step 2: Verify RED**

Run: `go test ./provider -run 'Schema|Metadata' -count=1`

Expected: FAIL because plugin URL and keywords are absent.

- [ ] **Step 3: Add provider metadata**

Set `schema.Metadata.PluginDownloadURL` and `Keywords` in `provider/provider.go`. Leave `LogoURL` empty.

- [ ] **Step 4: Regenerate schema and SDKs**

Run: `make VERSION_GENERIC=0.0.1-alpha.0+dev codegen`

Expected: schema and all five SDKs regenerate successfully with the new plugin source.

- [ ] **Step 5: Verify metadata and generated drift**

Run: `go test ./provider -run 'Schema|Metadata' -count=1`

Expected: PASS.

Run: `make check_codegen`

Expected: PASS with no additional diff after regeneration.

- [ ] **Step 6: Build generated SDKs**

Run: `make build_sdks`

Expected: all five SDK builds pass.

- [ ] **Step 7: Commit**

```bash
git add provider/provider.go provider/provider_test.go provider/schema_test.go provider/cmd/pulumi-resource-dokploy/schema.json sdk
git commit -m "feat: publish provider download metadata"
```

### Task 3: Registry Documentation And Apache License

**Files:**
- Create: `docs/_index.md`
- Create: `docs/installation-configuration.md`
- Modify: `LICENSE`
- Create: `provider/registry_docs_test.go`

**Interfaces:**
- Consumes current package coordinates and schema metadata from Task 2.
- Produces Pulumi Registry handwritten documentation with valid front matter.

- [ ] **Step 1: Write failing Registry docs tests**

Create a Go test that reads both files and checks:

```go
func TestRegistryDocumentation(t *testing.T) {
	index := readProjectFile(t, "../docs/_index.md")
	installation := readProjectFile(t, "../docs/installation-configuration.md")

	require.True(t, strings.HasPrefix(index, "---\n"))
	for _, marker := range []string{"title: Dokploy", "Pulumi", "community-maintained", "## Installation", "## Configuration", "## Example"} {
		require.Contains(t, index, marker)
	}
	for _, marker := range []string{"@dimeskigj/pulumi-dokploy", "pulumi_dokploy", "Dimeskigj.Pulumi.Dokploy", "net.dimeski.pulumi:dokploy", "github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy", "dokploy:endpoint", "dokploy:apiKey", "DOKPLOY_ENDPOINT", "DOKPLOY_API_KEY"} {
		require.Contains(t, installation, marker)
	}
	require.NotContains(t, index+installation, "official Dokploy")
	require.NotContains(t, index+installation, "official Pulumi")
}
```

Add a license test requiring `TERMS AND CONDITIONS FOR USE, REPRODUCTION, AND DISTRIBUTION`, sections 1 through 9, and the appendix marker.

- [ ] **Step 2: Verify RED**

Run: `go test ./provider -run '^TestRegistry(Documentation|License)$' -count=1`

Expected: FAIL because the docs are absent and the license is abbreviated.

- [ ] **Step 3: Add Registry overview**

Create `docs/_index.md` with YAML front matter beginning at byte zero, title `Dokploy`, meta description, community-maintained disclaimer, installation link, endpoint/API-key configuration, and a minimal TypeScript Project example.

- [ ] **Step 4: Add installation and configuration page**

Create `docs/installation-configuration.md` with exact package commands for Node.js, Python, Go, .NET, Java, and YAML. Explain automatic GitHub plugin acquisition, `dokploy:endpoint`, secret `dokploy:apiKey`, environment variable alternatives, and secret handling.

- [ ] **Step 5: Replace the license text**

Replace `LICENSE` with the unmodified canonical Apache License Version 2.0, January 2004 text from `https://www.apache.org/licenses/LICENSE-2.0.txt`.

- [ ] **Step 6: Verify docs and license**

Run: `go test ./provider -run '^TestRegistry(Documentation|License)$' -count=1`

Expected: PASS.

Run: `make docs_check`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add docs/_index.md docs/installation-configuration.md LICENSE provider/registry_docs_test.go
git commit -m "docs: add Pulumi Registry package pages"
```

### Task 4: Pull-Request Schema Compatibility Gate

**Files:**
- Modify: `.github/workflows/build.yml:46-64`
- Modify: `.github/workflows/release.yml:45-93`
- Modify: `.github/workflows/prerelease.yml:45-93`
- Modify: `provider/registry_metadata_test.go`

**Interfaces:**
- Produces a PR-only schema comparison step in `build.yml` after schema generation.
- Removes unreachable PR-only schema steps from tag-only release workflows.

- [ ] **Step 1: Add failing workflow contract tests**

Extend `registry_metadata_test.go` to require:

```go
require.Contains(t, buildText, "schema-tools compare")
require.Contains(t, buildText, "github.event_name == 'pull_request'")
require.Contains(t, buildText, "Looking good! No breaking changes found.")
require.NotContains(t, releaseText, "schema-tools compare")
require.NotContains(t, prereleaseText, "schema-tools compare")
```

Also validate that an incompatible result exits nonzero rather than only writing a report.

- [ ] **Step 2: Verify RED**

Run: `go test ./provider -run 'Workflow|Schema' -count=1`

Expected: FAIL because comparison is absent from build CI and present in tag workflows.

- [ ] **Step 3: Move the schema gate into PR CI**

After schema generation in `build.yml`, run `schema-tools compare` only for pull requests. Preserve report output, but end the step with nonzero status when the success marker is absent. Do not add write permissions merely to post comments.

- [ ] **Step 4: Remove unreachable tag-workflow blocks**

Remove schema-tools installation, comparison, PR comment, and labeling blocks from `release.yml` and `prerelease.yml`.

- [ ] **Step 5: Verify workflow semantics**

Run: `go test ./provider -run 'Workflow|Schema' -count=1`

Expected: PASS.

Run: `go test -short ./provider/... -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/build.yml .github/workflows/release.yml .github/workflows/prerelease.yml provider/registry_metadata_test.go
git commit -m "ci: enforce schema compatibility on pull requests"
```

### Task 5: Release Checksums, SBOMs, And Attestations

**Files:**
- Modify: `.goreleaser.yml`
- Modify: `.goreleaser.prerelease.yml`
- Modify: `Makefile:8-26`
- Modify: `.github/workflows/release.yml:241-270`
- Modify: `.github/workflows/prerelease.yml:241-270`
- Modify: `provider/registry_metadata_test.go`

**Interfaces:**
- Produces SHA256 checksum file, SBOM artifacts, and GitHub build-provenance attestations for stable and prerelease provider artifacts.
- Removes `build-provider-sign-windows` and `sign-goreleaser-exe-%` naming/hook contracts.

- [ ] **Step 1: Write failing GoReleaser contract tests**

Require both configs to contain:

```yaml
checksum:
  name_template: checksums.txt
  algorithm: sha256
sboms:
- artifacts: archive
```

Require Windows to remain in the platform matrix while rejecting `sign-windows` and `sign-goreleaser` strings.

- [ ] **Step 2: Write failing workflow attestation tests**

For stable and prerelease publish jobs require `id-token: write`, `attestations: write`, a commit-SHA-pinned `actions/attest-build-provenance` action, and subject paths covering released archives, checksums, and SBOMs from `dist/`. Require `contents: write` to remain present for GitHub Releases.

- [ ] **Step 3: Verify RED**

Run: `go test ./provider -run 'Release|Workflow' -count=1`

Expected: FAIL because SBOM/attestation contracts are absent and signing placeholders remain.

- [ ] **Step 4: Update GoReleaser configs**

Rename the Windows build ID to a neutral name such as `build-provider-windows`, remove the post hook, add explicit SHA256 checksum configuration, and add archive SBOM generation in both files. Keep archive naming unchanged.

- [ ] **Step 5: Remove the no-op Make target**

Remove `sign-goreleaser-exe-%` from `.PHONY` and delete its no-op target.

- [ ] **Step 6: Add OIDC attestations**

Give only each `publish` job `id-token: write` and `attestations: write`. After GoReleaser completes, invoke a commit-SHA-pinned `actions/attest-build-provenance` release action over `dist/*.tar.gz`, `dist/*.zip`, `dist/checksums.txt`, and generated SBOM files. Do not expose SDK publishing credentials to this step.

- [ ] **Step 7: Validate release configuration and tests**

Run: `goreleaser check -f .goreleaser.yml`

Run: `goreleaser check -f .goreleaser.prerelease.yml`

Expected: both pass. If the project uses `mise`, run the same commands through `mise exec --`.

Run: `go test ./provider -run 'Release|Workflow' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add .goreleaser.yml .goreleaser.prerelease.yml Makefile .github/workflows/release.yml .github/workflows/prerelease.yml provider/registry_metadata_test.go
git commit -m "ci: attest provider release artifacts"
```

### Task 6: Exact-Version Consumer Smoke Workflow

**Files:**
- Create: `.github/workflows/release-smoke.yml`
- Modify: `provider/registry_metadata_test.go`
- Modify: `CONTRIBUTING.md`

**Interfaces:**
- Produces a manual `workflow_dispatch` input named `version` requiring SemVer without a leading `v`.
- Consumes public GitHub Releases, npm, PyPI, NuGet, Maven Central, and the Go module proxy only.

- [ ] **Step 1: Add failing smoke-workflow contract tests**

Require a workflow with only `workflow_dispatch`, exact `version` input, read-only contents permission, no publishing permissions/secrets, and separate jobs for provider, Node.js, Python, .NET, Java, and Go.

Require every job to set isolated cache paths under `${{ runner.temp }}` and require exact package coordinates containing `${{ inputs.version }}`.

- [ ] **Step 2: Verify RED**

Run: `go test ./provider -run 'Workflow|Smoke' -count=1`

Expected: FAIL because `release-smoke.yml` does not exist.

- [ ] **Step 3: Add provider artifact smoke job**

Create the manual workflow. Validate `version` with a strict SemVer expression, download the current runner platform archive and `checksums.txt` from `v${{ inputs.version }}`, verify SHA256, extract into an empty temporary plugin cache, run the binary's version command, and load schema using the pinned Pulumi CLI.

- [ ] **Step 4: Add clean SDK consumer jobs**

Each language job must create source files under `${{ runner.temp }}`, install the exact public package version, set a fresh `PULUMI_HOME`, and compile or execute schema registration without local SDK paths:

- Node.js: `@dimeskigj/pulumi-dokploy@${{ inputs.version }}`.
- Python: `pulumi_dokploy==${{ inputs.version }}`.
- .NET: `Dimeskigj.Pulumi.Dokploy` version `${{ inputs.version }}`.
- Java: `net.dimeski.pulumi:dokploy:${{ inputs.version }}`.
- Go: `github.com/dimeskigj/pulumi-dokploy/sdk/go/dokploy@v${{ inputs.version }}`.

Each job verifies that Pulumi downloads the matching provider into its isolated cache rather than using a preinstalled plugin.

- [ ] **Step 5: Document manual usage**

Add a `CONTRIBUTING.md` section describing how to dispatch the workflow after all packages for a version are visible and how to interpret ecosystem-specific failures.

- [ ] **Step 6: Verify workflow contracts**

Run: `go test ./provider -run 'Workflow|Smoke' -count=1`

Expected: PASS.

Run: `go test -short ./provider/... ./internal/... -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add .github/workflows/release-smoke.yml provider/registry_metadata_test.go CONTRIBUTING.md
git commit -m "ci: verify released provider packages"
```

### Task 7: Update Readiness Ledger And Final Verification

**Files:**
- Modify: `docs/provider-registry-readiness.md`
- Verify all files changed by Tasks 1-6.

**Interfaces:**
- Consumes completed implementation evidence.
- Produces an accurate completed/deferred readiness ledger.

- [ ] **Step 1: Update the readiness assessment**

Mark User-Agent, plugin download metadata, keywords, Registry docs, full license, PR schema checking, SBOMs/attestations, and release smoke workflow complete. Keep these explicitly deferred:

- Logo and `logoUrl`.
- Security/private reporting contact.
- Maintainer/publisher display-name mapping.
- CODEOWNERS/governance ownership.
- Code of Conduct enforcement contact.
- Registry PR and release publication.
- Lookup functions and broad engine acceptance.

Do not call the provider Registry-ready until required maintainer inputs and a corrected release exist.

- [ ] **Step 2: Format and inspect**

Run: `gofmt -w internal/client/client.go internal/client/client_test.go provider/config.go provider/config_test.go provider/provider.go provider/provider_test.go provider/schema_test.go provider/registry_docs_test.go provider/registry_metadata_test.go`

Run: `git diff --check`

Expected: no errors.

- [ ] **Step 3: Run provider and race suites**

Run: `make test_provider`

Expected: PASS.

Run: `make test_race`

Expected: PASS.

- [ ] **Step 4: Run generated and documentation drift checks**

Run: `make check_codegen`

Run: `make check_openapi`

Run: `make docs_check`

Expected: all pass with no generated drift.

- [ ] **Step 5: Run lint, vulnerability, and license checks**

Run: `make lint`

Run: `make govulncheck`

Run: `make license`

Expected: all pass.

- [ ] **Step 6: Validate both release configs**

Run: `goreleaser check -f .goreleaser.yml && goreleaser check -f .goreleaser.prerelease.yml`

Expected: PASS.

- [ ] **Step 7: Inspect branch status and commit ledger update**

Run: `git status --short`

Expected: only `docs/provider-registry-readiness.md` is uncommitted, unless verification generated an explained artifact that must be removed or committed according to repository policy.

```bash
git add docs/provider-registry-readiness.md
git commit -m "docs: update Registry readiness status"
```
