# Application builders, repository sources, and automatic deployment

## Approved intent

Pulumi users must be able to configure every application builder exposed by the
checked-in Dokploy API, use GitHub/Gitea/Bitbucket integrations as application
sources, and configure application-level `autoDeploy`. Existing applications
remain compatible. The user approved configuration-only live acceptance tests,
SDD execution with GPT-6 Luna subagents, and an isolated worktree.

## Scope and architecture

Extend the existing `Application`, `ApplicationSource`, and `ApplicationBuild`
flow. Keep the current Docker, generic Git, and GitLab source APIs. Add typed
`github`, `gitea`, and `bitbucket` variants; reference existing Dokploy
integrations, without managing integrations or changing Compose.

Use generated API clients. Enable the three already-saved upstream source-save
operations through the normalization allowlist. Do not edit generated clients,
schema, SDKs, or website reference pages by hand.

## Builders

Accept `dockerfile`, `nixpacks`, `heroku_buildpacks`, `paketo_buildpacks`, `static`,
and `railpack` for all five repository source variants. Docker image sources do
not send build configuration.

Extend `ApplicationBuild` with optional `herokuVersion`, `railpackVersion`,
`publishDirectory`, and `isStaticSpa`. Match Dokploy's builder form:

| Builder | Options |
| --- | --- |
| Dockerfile | Existing required nonempty Dockerfile, optional context/stage |
| Heroku Buildpacks | Optional `herokuVersion` |
| Paketo Buildpacks | No builder-specific options |
| Nixpacks | Optional `publishDirectory` and `isStaticSpa` |
| Static | Optional `isStaticSpa` |
| Railpack | Optional `railpackVersion` |

Reject incompatible options with property-path diagnostics; do not enumerate or
pin allowed version strings. Unset fields clear previously managed builder
settings with API nulls, including optional SPA/publish fields. Preserve explicit
false, and distinguish it from an unset nullable flag. Decode only the selected
builder's relevant settings so residual server fields do not create invalid
imported configurations. Avoid repeated diffs from server-default false flags:
implicit false can normalize to omitted; explicitly configured false is retained.
Keep existing Nixpacks fallback for responses without a build type.

## Repository sources

New Go types are `GitHubAppSource`, `GiteaAppSource`, and `BitbucketAppSource`,
with `ApplicationSource.GitHub`, `.Gitea`, and `.Bitbucket` fields. Public Pulumi
property names are lowercase `github`, `gitea`, and `bitbucket`.

Every new variant has required nonempty `integrationId`, `owner`, `repository`,
and `branch`; optional `buildPath`, `watchPaths`, and `enableSubmodules`; and
required `build`. GitHub also has optional `triggerType` (`push` or `tag`, default
`push`). Bitbucket has required nonempty `repositorySlug`. Keep exactly one
variant selected, reject missing/mismatched variants, and map fields to the
existing upstream save-provider request names without inventing credentials.

Read/refresh and ID-only import reconstruct each source from the actual flat
Dokploy response fields, including `sourceType`. Server root/default paths and
GitHub's implicit push trigger must not produce needless redeploy diffs. Missing
required identity fields produce errors instead of guessed values.

Source-type changes replace the application. Changes within a repository source,
including builders, update configuration and redeploy through the existing
provider lifecycle. Preserve setup failure cleanup, partial state, and secret
redaction. Use a nil-safe shared build-selection helper to avoid assuming every
non-generic-Git source is GitLab.

## Automatic deployment

Add optional `ApplicationArgs.AutoDeploy *bool` / Pulumi `autoDeploy`.
Omission on create leaves the server default alone; explicit false is sent as
false, not omitted. Set explicit values through `application.update` before
source attachment on create (the create endpoint has no field for this option).

Changing only this option is configuration-only and must not call deploy,
redeploy, source-save, build-save, or environment-save endpoints. It can share
an update request with metadata/registry changes. Removing the input means leave
the current server value unmanaged, not restore a hardcoded default or send null;
ignore unset desired values in diff and retain previously observed output.

Refresh/import reconstruct observed boolean values, preserving false. Missing or
null response values preserve prior known values when available. The provider
still performs its existing explicit deployment during create and runtime
updates even when automatic deployment is disabled.

## Live acceptance safety and coverage

Add configuration-only Tier 2 coverage. Never clone, fetch, build, deploy, or
redeploy these new fixtures. Create bare application records using the generated
client, disable `autoDeploy` before attaching any source, and keep it false.
Cover all six builders on generic Git, including option changes, builder
transitions/clearing, readback, import, and no-op diff. Cover new sources using
protected integration/repository environment prerequisites; skip only a source
whose prerequisites are absent. Do not hide server contract failures as skips.

Exercise autoDeploy false persistence and its no-op/update behavior live; test
true and webhook-trigger distinctions offline only. There is no live enablement
or real-build test in this scope.

Use reserved fresh fixture names, serial execution, existing heavy-operation and
health gates, independent bounded cleanup, acknowledged IDs only, verified
absence, secret/metadata redaction, and project-retention/stop markers on cleanup
or ownership uncertainty. Never retry an uncertain create, search by name for an
ID, or cascade-delete an uncertain fixture via the parent project. Add offline
tests proving these safety properties, not just tests that inspect source text.

Update the manually dispatched workflow with protected source prerequisites and
document them in `tests/README.md`. Do not dispatch or run live tests as part of
implementation verification. Offline Go commands must override
`DOKPLOY_ACCEPTANCE=0` after `mise exec` because `.mise.toml` defaults to live mode.

## Generated surfaces, documentation, and validation

Regenerate all five SDKs and the provider schema after source changes. Update the
Application guide, source concepts, relevant README copy, and generated reference
documentation. Examples explain unmanaged integrations, builder option validity,
autoDeploy omission semantics, and explicit-versus-automatic deployment behavior.

Run relevant Makefile checks: lint, provider tests, race tests, provider build,
OpenAPI reproducibility, codegen reproducibility, SDK builds, language example
tests, and website checks. Record unavailable checks honestly. No release,
publication, merge, push, or live workflow is authorized.

## Non-goals

Integration-management resources, Compose source changes, webhook provisioning,
full deployment acceptance tests, and changes to existing source replacement
semantics are excluded. Ask the user whether to retain or delete these task's SDD
documents at completion; do not delete them without permission.
