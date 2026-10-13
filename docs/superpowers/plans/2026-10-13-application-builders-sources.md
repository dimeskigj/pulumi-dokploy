# Application Configuration Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support all application builders, GitHub/Gitea/Bitbucket sources, optional autoDeploy, and safe configuration-only live acceptance coverage.

**Architecture:** Extend the existing application lifecycle and tagged source variants using generated Dokploy clients. Keep source/build selection explicit, preserve omitted-versus-false semantics, and exercise live configuration on bare undeployed fixtures.

**Tech Stack:** Go 1.26.9, pulumi-go-provider infer, oapi-codegen nullable clients, Pulumi CLI 3.259.0, generated Go/Node.js/Python/.NET/Java SDKs, Astro documentation.

**Spec:** `docs/superpowers/specs/2026-10-13-application-builders-sources-design.md`

## Global Constraints

- Keep the current Docker, generic Git, and GitLab source APIs; do not change Compose or manage integrations.
- Use generated API clients; regenerate schema/SDK/reference output rather than hand-editing it.
- Source-type changes replace; runtime source/build changes configure and redeploy; autoDeploy-only changes never deploy/redeploy.
- Live coverage is configuration-only: never clone, fetch, build, deploy, or redeploy these new fixtures, and keep autoDeploy false before attaching sources.
- Use serial heavy-operation/health gates, acknowledged IDs, bounded verified cleanup, and stop/project retention on ownership uncertainty.
- Keep credentials, resource IDs, private URLs, and source metadata out of logs/reports; use protected prerequisites and safe sentinel values.
- Do not run live acceptance tests or dispatch release/publication workflows. Override `DOKPLOY_ACCEPTANCE=0` after `mise exec` for offline checks.
- Use GPT-6 Luna subagents for implementation and review. Do not dispatch multiple implementation workers concurrently or allow workers to spawn subagents.
- Each task uses TDD, self-review, a focused commit, a report with command/output evidence, and an independent task review.
- Inspect existing work before regeneration; do not discard user changes. Run `git diff --check` and inspect status/diffs before completion.

## Review Focus

- Builder switches can leave residual fields: verify null clearing and valid imported selected-builder settings (Task 1).
- A new source can accidentally follow the old GitLab fallback: test every repository source with every builder and required-field errors (Task 2).
- Omitted booleans can cause false to vanish or perpetual diffs: test explicit false, implicit defaults, and removing management (Tasks 1–3).
- Unacknowledged create/cleanup failure can cascade-delete a fixture: offline exercise stop markers and parent-retention behavior (Task 4).
- Configuration-only tests can accidentally deploy via provider Update or autoDeploy: scripted servers reject deploy/redeploy/fetch and assert disabling before source setup (Task 4).

## File responsibilities

- `provider/application_source.go`: existing source API and dispatch; builder API/validation can move to focused `application_build.go` without changing public names.
- `provider/application_repository_sources.go`: new source structs, annotations, and focused request/decode helpers.
- `provider/application.go`: lifecycle, diff, read, and autoDeploy integration.
- `provider/application_build_test.go`, `application_repository_sources_test.go`, `application_auto_deploy_test.go`: focused offline regression suites.
- `openapi/operations.txt`, `corrections.json`, `cmd/normalize/main_test.go`: generation source and contract tests.
- `provider/live_application_configuration_test.go` and `_unit_test.go`: safe live configuration cases and offline harness tests.
- `provider/live_workloads_test.go`: Tier 2 registration and relevant shared assertions/redaction helpers.
- `.github/workflows/run-acceptance-tests.yml`, `tests/README.md`: protected live prerequisites and safety documentation.
- `sdk/*`, `provider/cmd/pulumi-resource-dokploy/schema.json`: generated public surfaces.
- `website/src/content/docs/guides/applications.mdx`, `concepts/sources.mdx`, `reference/*`, `README.md`: user-facing documentation.

### Task 1: Generated source endpoints and complete builder support

**Files:** Modify OpenAPI allowlist/corrections/normalization tests and regenerate `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`. Modify existing builder declarations/validation/configuration in `provider/application_source.go` and decoding in `application.go`; create focused `application_build.go` and `application_build_test.go` if useful. Update existing expected build request bodies for newly explicit nullable fields.

**Interfaces:** Consume `configureApplicationBuild(ctx context.Context, api *client.Client, id string, source ApplicationSource) error` and existing generated build request. Produce constants `BuildHerokuBuildpacks`, `BuildPaketoBuildpacks`, `BuildStatic`, `BuildRailpack`; `ApplicationBuild.HerokuVersion`, `.RailpackVersion`, `.PublishDirectory` as `*string`, `.IsStaticSpa` as `*bool`; a nil-safe `(ApplicationSource).applicationBuild() (ApplicationBuild, bool)` helper; generated save GitHub/Gitea/Bitbucket client methods. `decodeBuild(m map[string]interface{}, prior ApplicationBuild) ApplicationBuild` accepts prior settings for default/presence normalization; update existing callers. Only Docker may silently skip build configuration; a missing/unsupported repository variant returns an error, not a panic or silent skip.

- [ ] **Step 1: Write failing contract and builder tests.** `TestApplicationSourceSaveOperationContracts` asserts the three upstream POST routes remain in normalized output, preserve request names/nullable requirements, and return boolean. `TestApplicationBuildValidateAllTypes` accepts all six types, Dockerfile requires a nonempty path, versions are only valid on their matching builders, publishDirectory only on Nixpacks, and isStaticSpa only on Nixpacks/Static. Include explicit false and unsupported types.
- [ ] **Step 2: Verify red.** Run `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./openapi/cmd/normalize ./provider -run 'TestApplication(SourceSaveOperationContracts|Build)' -count=1`; expect a missing-contract/API or validation failure.
- [ ] **Step 3: Implement generation source.** Allow `application.saveGithubProvider`, `application.saveGiteaProvider`, `application.saveBitbucketProvider`; preserve upstream request schemas and normalize responses to boolean. Run `mise exec -- make generate_openapi` after checking status.
- [ ] **Step 4: Implement builder API/validation/serialization/read.** Preserve old public API/errors where practical. Send all eight build option keys as values or null so removal and transitions clear previous settings. Use applicable fields only on read; preserve explicit false and normalize implicit server-default false to omitted. Do not pin versions. Docker sources send no build request; unsupported/missing source variants must not panic.
- [ ] **Step 5: Pin round-trip and clearing behavior.** `TestApplicationBuildRequestShapes` checks all six requests, populated versions/Nixpacks publish/SPA/Static SPA and null removals. `TestApplicationBuildReadSelectedFields` checks ID-only read and prior-aware read, absent/null/default false, stale irrelevant server fields, and explicit false. Exercise Git and GitLab. Test build type/option changes yield runtime update diffs and preserve existing lifecycle ordering.
- [ ] **Step 6: Verify and commit.** Run focused tests, offline `go test ./openapi/cmd/normalize ./provider ./internal/client/...`, `git diff --check`; commit source/generated-client changes. Run `mise exec -- make check_openapi` after commit and report reproducibility. Schema/SDK regeneration is reserved for Task 5.

### Task 2: GitHub, Gitea, and Bitbucket application sources

**Files:** Modify `provider/application_source.go`, `application.go`, and the shared build-selection helper from Task 1. Create `application_repository_sources.go` and `application_repository_sources_test.go`; extend source/diff/lifecycle assertions as needed.

**Interfaces:** Consume Task 1 generated methods and `applicationBuild()`. Produce `SourceGitHub`, `SourceGitea`, `SourceBitbucket`; `ApplicationSource.GitHub *GitHubAppSource`, `.Gitea *GiteaAppSource`, `.Bitbucket *BitbucketAppSource`; `GitHubTriggerType` with `GitHubTriggerPush = "push"` and `GitHubTriggerTag = "tag"`. New structs expose required string integrationId/owner/repository/branch, optional `BuildPath *string`, `WatchPaths []string`, `EnableSubmodules bool`, and `Build ApplicationBuild`. Bitbucket adds required `RepositorySlug string`; GitHub adds optional `TriggerType *GitHubTriggerType` (nil requests push).

- [ ] **Step 1: Write failing validation/request tests.** `TestApplicationRepositorySourceValidation` asserts exactly one of six source variants, all required nonempty fields, mismatched variants, invalid trigger, and builder validation. `TestApplicationRepositorySourceRequests` checks exact integration-specific wire names, Bitbucket slug, optional null clearing, paths/watch/submodules, and GitHub push/tag.
- [ ] **Step 2: Verify red.** Run `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./provider -run TestApplicationRepositorySource -count=1`; expect missing symbols or unsupported variant failures.
- [ ] **Step 3: Implement typed variants and dispatch.** Add annotations and explicit generated source-save mapping. Extend nil-safe shared build selection; do not use a generic non-Git => GitLab fallback. Make existing source validation deterministic and preserve established property-path diagnostics.
- [ ] **Step 4: Implement read/import.** Map githubId/repository/owner/branch/buildPath/triggerType, gitea-prefixed fields, and bitbucket-prefixed fields including repositorySlug. Require identity fields; accept actual sourceType discriminator. Normalize documented default/root paths and implicit push without hiding non-default observed changes.
- [ ] **Step 5: Pin lifecycle coverage.** `TestApplicationRepositorySourceReadAndImport` compares every public field, empty/null optional fields, prior/default handling, and missing required fields. `TestApplicationRepositorySourceBuilderMatrix` exercises five repository sources × six builders. Script create and update ordering for all new variants; assert same-variant runtime updates redeploy and source-type changes replace. Exercise setup failures/partial state and echoed environment/build-secret redaction on new routes. Assert schema exposes the new types and options.
- [ ] **Step 6: Verify and commit.** Focused tests followed by `mise exec -- make test_provider`, `git diff --check`; commit only source/test changes. SDKs remain Task 5's responsibility.

### Task 3: Optional application autoDeploy lifecycle

**Files:** Modify `provider/application.go`; create `provider/application_auto_deploy_test.go`.

**Interfaces:** Produce `ApplicationArgs.AutoDeploy *bool` with `pulumi:"autoDeploy,optional"`; use existing generated `ApplicationUpdateJSONRequestBody.AutoDeploy`. Add annotations/dependencies, explicit-value create/update mapping, presence-aware read, and ordinary update diffs.

- [ ] **Step 1: Write failing lifecycle tests.** `TestApplicationAutoDeployCreate` scripts omitted/true/false; no update for omitted, explicit value through update before source setup. `TestApplicationAutoDeployUpdateConfigurationOnly` scripts true↔false and metadata combinations, and expects no source/build/environment/deploy/redeploy requests. `TestApplicationAutoDeployReadAndDiff` covers true, false, null, missing, ID-only import, computed input, and omission after previously managed true/false.
- [ ] **Step 2: Verify red.** Run `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./provider -run TestApplicationAutoDeploy -count=1`; expect missing field or absent request/readback failures.
- [ ] **Step 3: Implement optional semantics.** Nil desired value means leave current value alone: suppress that diff, do not send null, preserve observed output on update. Explicit changes share the metadata request where useful and do not enter runtimeChanged. Read raw boolean presence to preserve false and prior known values on absent/null response. Existing create/runtime deployment behavior remains unchanged.
- [ ] **Step 4: Pin failures and previews.** Test DryRun makes no calls, schema exposes optional boolean, an autoDeploy update/create-setup failure redacts prior/current secrets and preserves existing partial-state/cleanup behavior. Include a combined runtime change proving it still deploys when autoDeploy false.
- [ ] **Step 5: Verify and commit.** Run focused tests and `mise exec -- make test_provider`, inspect diff/status, commit source/tests.

### Task 4: Safe configuration-only live acceptance coverage

**Files:** Create `provider/live_application_configuration_test.go` and `live_application_configuration_unit_test.go`; modify `live_workloads_test.go` for Tier 2 integration and assertions/redaction. Modify `.github/workflows/run-acceptance-tests.yml`, `tests/README.md`, and workflow contract tests where needed.

**Interfaces:** Consume all source/build types and AutoDeploy from Tasks 1–3, existing heavy-operation leases, cleanup owners/stop markers, `liveRecordCreateDefinitelyRejected`, and `liveProject` release callback. Produce a configuration fixture guard that can retain the parent project until every acknowledged fixture is verified absent; a Tier 2 application-configuration runner and pure/environment prerequisite helpers. Existing source variants should use the same safe bare-fixture setup rather than leaving default autoDeploy enabled.

- [ ] **Step 1: Write failing offline harness tests.** `TestLiveApplicationConfigurationNeverDeploys` uses scripted requests to assert bare create → update autoDeploy false → source/build save → read/import → configuration changes → verified delete, rejecting deploy/redeploy/fetch. `TestLiveApplicationConfigurationOwnership` covers accepted ID, acknowledged partial-create error, definite rejection, no ID/transport error, cleanup failure, parent retention, stop behavior, independent cleanup context, and no retry/search. Prerequisite tests exercise absent, complete, invalid, and redacted inputs with safe sentinels.
- [ ] **Step 2: Verify red.** Run `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 go test ./provider -run TestLiveApplicationConfiguration -count=1`; expect missing helper or request/safety assertion failures. Do not run live tests.
- [ ] **Step 3: Implement fixture safety.** Register ownership/retention before assertions; disable autoDeploy before sources; clean acknowledged partial creates; unknown identity sets stop and retains parent, definite rejection does not. Serialize API setup/mutations and respect stop gates between cases. Use raw ID-only API absence checks for a bare or partially configured application so source decoding cannot obstruct cleanup. Register source metadata with existing secret redaction and use sanitized assertions.
- [ ] **Step 4: Implement coverage.** All six builders on generic Git, populated relevant options, same-builder option mutation, transition through builders and back to Nixpacks with cleared fields, refresh/import and no-op diff. New sources use protected prerequisites and configuration mutations without `Application.Update` for runtime changes (that method redeploys). Check configuration-only provider autoDeploy false/no-op behavior, never enable true live.
- [ ] **Step 5: Wire prerequisites/docs.** Add `DOKPLOY_{GITHUB,GITEA,BITBUCKET}_INTEGRATION_ID`, `_OWNER`, `_REPOSITORY`, `_BRANCH`; Bitbucket additionally `_REPOSITORY_SLUG`, all supplied via acceptance environment secrets. Reuse existing GitLab prerequisites. Optional source cases skip only if prerequisites absent; invalid populated prerequisites or API incompatibility fail. Document that configuration checks do not prove clone/build/deploy compatibility, and no workflow was run. Add offline workflow-binding assertions.
- [ ] **Step 6: Verify and commit.** Run focused offline tests, `mise exec -- make test_provider`, `mise exec -- make test_race`, `git diff --check`, inspect status/diff and commit. Do not change unrelated live tiers or trigger workflows.

### Task 5: Public SDKs, application documentation, and final verification

**Files:** Regenerate provider schema and all `sdk/*`; modify `README.md`, application guide, source concepts and relevant website tests; regenerate affected reference pages. Add a narrow new-feature compilation fixture if existing examples need additional SDK surface coverage; do not broadly rewrite canonical examples.

**Interfaces:** Consume Tasks 1–4 public source/build/autoDeploy APIs and acceptance prerequisite contracts; produce consistent generated public APIs and documented behavior across Go/Node.js/Python/.NET/Java.

- [ ] **Step 1: Write documentation/public surface regression assertions.** Website tests assert all six builders, github/gitea/bitbucket guidance, external integrations, autoDeploy semantics, and configuration-only acceptance limitation. Use source schema assertions from earlier tasks and TypeScript checked example snippets (existing site generator/tests conventions).
- [ ] **Step 2: Regenerate.** Inspect status before `mise exec -- make codegen`; run it with default development version and verify no unrelated generated churn. Inspect that new types/options appear in all five SDKs.
- [ ] **Step 3: Document.** Update guides/concepts/README with builder option table (Nixpacks publishDirectory, Nixpacks/Static SPA), GitHub push/tag, Bitbucket slug, external integration setup, optional autoDeploy leave-unmanaged semantics, and explicit deployment distinction. Explain live coverage/prerequisite skips without real identifiers. Run `mise exec -- make docs_generate` and include generated reference output.
- [ ] **Step 4: Verify complete branch.** Run `mise exec -- make lint`, `test_provider`, `test_race`, `provider`, `check_openapi`, `build_sdks`, `test_examples`, and website generation/tests/build. Avoid command chains in reports. Commit intended generated/docs changes before `make check_codegen` and `make docs_check`, since those targets use git diff to check reproduction; rerun any affected verification after a fix. Use `mise exec -- /usr/bin/env DOKPLOY_ACCEPTANCE=0 make test_examples` for belt-and-suspenders live safety. Run normalization/unit/schema-drift tests and record exit codes and unavailable tools honestly.
- [ ] **Step 5: Commit and report.** Inspect diff/status and `git diff --check`; commit only task changes. Report exact commands/results, generated-file reproducibility, live tests added but unrun, and any pre-existing failures. Controller then performs whole-branch review with GPT-6 Luna, final verification, and asks whether to retain/delete the SDD spec/plan/working documents; no merge or push.
