# Owned GitHub Workflows Design

## Goal

Replace the Pulumi `ci-mgmt` generated workflow surface with repository-owned GitHub Actions workflows that are smaller, reviewable, and usable without Pulumi organization infrastructure. Rebase the existing `feature-owned-ci-github-releases` work onto current `main`, preserve current provider behavior, and publish the repaired branch.

## Branch Strategy

Rebase `feature-owned-ci-github-releases` onto `main` and resolve conflicts in favor of the owned-workflow architecture while retaining provider, documentation, schema, and acceptance-test additions from `main`. Push only this feature branch with `--force-with-lease`; leave `owned-ci-github-releases` untouched.

## Workflow Surface

The repository will own these six workflow files:

- `build.yml`: pull-request, `main`, matching feature-branch, and manual validation.
- `lint.yml`: reusable lint job called by build and acceptance workflows.
- `pages.yml`: existing documentation deployment workflow.
- `prerelease.yml`: prerelease tag validation, build, test, GitHub prerelease, and SDK publication.
- `release.yml`: stable tag validation, build, test, GitHub release, SDK publication, and docs dispatch.
- `run-acceptance-tests.yml`: manually dispatched live Dokploy validation.

Delete `.ci-mgmt.yaml`, the local normalization scripts and tests, and generated ancillary workflows that are not part of this repository's required CI or release path. Remove generated-file warnings and the `ci-mgmt` Make target.

## Build And Release Behavior

All jobs use GitHub-hosted Ubuntu runners; Pulumi-only runner labels are removed. Actions remain pinned to immutable commit SHAs. Workflow and job permissions use the minimum access needed, with `contents: write` limited to release/publishing jobs.

The build workflow runs documentation checks, OpenAPI and code-generation drift checks, race tests, all SDK builds, examples, vulnerability scanning, licensing, provider tests, and lint. It builds and uploads only binaries the repository actually produces.

Stable and prerelease workflows trigger on mutually exclusive semantic-version tag patterns. GoReleaser creates GitHub release artifacts without Pulumi's S3 upload configuration. SDK jobs consume artifacts from the same workflow run and publish Node.js, Python, .NET, Java, and Go packages using repository secrets and established package tooling. Release dependencies must prevent publication when build or test jobs fail.

## Acceptance Secrets

Acceptance tests run only via `workflow_dispatch` and use the protected `dokploy-acceptance` environment. Only jobs that need Dokploy credentials reference them. The workflow requires and passes:

- `DOKPLOY_ENDPOINT`
- `DOKPLOY_API_KEY`
- `DOKPLOY_REGISTRY_URL`
- `DOKPLOY_REGISTRY_USERNAME`
- `DOKPLOY_REGISTRY_PASSWORD`
- optional `DOKPLOY_REGISTRY_IMAGE_PREFIX`

The prerequisites job runs the explicit live provider suite before packaging the provider. Language example jobs receive the same credentials only where live behavior needs them. Lint remains outside the protected environment and receives no acceptance secrets.

## Conflict Resolution

The workflow metadata tests will retain current `main` resource-count, resource-token, README, and acceptance assertions while replacing generated-workflow expectations with the six-file owned policy. Tests will verify trigger separation, exact required gates, secret isolation, permissions, pinned actions, artifact contracts, removal of Pulumi AWS/ESC dependencies, and absence of `ci-mgmt` inputs.

Documentation changes from the feature branch remain limited to installation and contribution guidance affected by GitHub Releases and removal of generated CI. Unrelated provider documentation from `main` is preserved.

## Error Handling

Jobs use normal dependency gating and fail-fast matrices where later publication would be unsafe. Credential preflight emits a failing status before live operations when required secrets are absent. Artifact creation and extraction paths must match exactly; no step may reference an unbuilt generator binary. Rebase and push operations stop on conflicts or a failed `--force-with-lease` rather than overwriting newer remote work.

## Verification

Before pushing:

- Parse all owned workflow YAML and run the workflow metadata tests.
- Run `go test ./...` and the provider race tests.
- Run lint, OpenAPI/codegen drift checks, SDK builds, example tests, and documentation checks.
- Validate GoReleaser configurations when the installed tool supports it.
- Inspect the final diff against `main`, check action pins and permissions, and confirm no `ci-mgmt`, Pulumi ESC, Pulumi-only runners, AWS release upload, or nonexistent binary references remain.
- Confirm the feature branch is rebased on current `main`, then push with `git push --force-with-lease origin feature-owned-ci-github-releases`.

Live Dokploy execution is not required locally because credentials are expected to be available only through the protected GitHub environment.
