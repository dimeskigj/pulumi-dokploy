# GitHubIntegration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Manage existing Dokploy GitHub integration configuration through a safe, import-only Pulumi resource.

**Architecture:** Add a separate inferred `GitHubIntegration` using the authenticated generated client and verified relation/parent observations. Generate API clients, schema, all five SDKs, examples and reference docs from source; preserve existing GitLab and workload source contracts.

**Tech Stack:** Go, pulumi-go-provider/infer, oapi-codegen, Pulumi schema/SDK generators, existing Go/Node.js/Python/.NET/Java examples and Astro/Starlight docs.

**Spec:** `docs/superpowers/specs/2026-10-13-github-integration-design.md` (approved).

## Global Constraints

- Resource token: `dokploy:index:GitHubIntegration`; Pulumi and import identity is `githubId` (relation ID), never GitHub App ID or `gitProviderId`.
- Exactly two required nonempty inputs: `name`, `githubAppName`; observed outputs also include stable `githubId`, `gitProviderId`, and `organizationId`.
- No create API exists: real Create always returns a fixed import/setup diagnostic without any API request or fabricated identity; previews make no calls.
- No credentials, tokens, upstream user record, sharing fields, or readiness/connection guarantee in the public schema or state. IDs are ordinary nonsecret outputs.
- Verify relation ID, nested parent ID, GitHub provider type, and active/previous organization before accepting observations or mutating/removing; never select a parent by list position or name.
- Update only `name` and `githubAppName` with verified relation/parent IDs; no mutation retries; preserve verified actual/last-observed state after partial failures.
- Only structured genuine not-found proves absence; sanitize arbitrary response/error prose and never leak IDs, URLs, credentials, bodies, or parser fragments.
- Preserve Application/Compose source types and all existing resource/invoke/API selections; do not reimplement, clean up, or rewrite GitLab history.
- Pin remains unchanged; use generated sources, never hand-edit generated OpenAPI/client/schema/SDK/reference outputs.
- Run offline checks only via `SAFE=/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run`; actual executions force live-off and approved TMPDIR. Never access `.env`, run live acceptance, release/publish, push, or merge.
- At execution, capture actual baseline counts/status because other rollout phases may have landed; update inventories additively and adjust full-operation-count assertions only for intended selected operations. Keep all six GitLab operation checks intact.
- Commit only explicit owned paths; preserve unrelated work. Keep this spec and plan until explicit cleanup approval.

## Review Focus

- A present parent must match the nested identity/type/org despite unrelated list entries: test wrong-parent/type/org and relation mismatch in Task 3.
- Missing app-name config must remain distinguishable from omitted modeled data: test explicit incomplete import versus malformed omission in Task 3.
- A computed input cannot mask another known invalid input: test both independently in Task 2.
- An acknowledged or split update may fail readback without claiming desired state: test partial update and sanitized diagnostics in Task 3.
- Removal or read errors must not become false absence: test ambiguous remove and only structured not-found in Task 3.

---

## File map and boundaries

| Files | Responsibility |
| --- | --- |
| `openapi/operations.txt`, `openapi/corrections.json`, `openapi/cmd/normalize/main_test.go` | Selected API operations, source-grounded response corrections and contract assertions |
| `internal/client/generated/generated.gen.go`, new `internal/client/github_contract_test.go` | Generated typed client and offline request/response contract |
| New `provider/github_integration.go`, `provider/github_integration_observation.go`, `provider/github_integration_lifecycle.go` and matching tests | Public contract, validated safe observation, import/read/update/delete and fixed Create rejection |
| `provider/provider.go`, `provider/provider_test.go`, `provider/schema_test.go`, preview/inventory tests | Registration, exact schema and additive inventories |
| `provider/cmd/pulumi-resource-dokploy/schema.json`, `sdk/{go,nodejs,python,dotnet,java}/` | Generated provider schema and five SDKs |
| `README.md`, `website/src/content/docs/guides/{github-integrations,imports}.mdx`, website inventory/reference sources and tests | Import/setup/destruction guidance and generated references |
| `examples/yaml/{Pulumi.yaml,README.md}`, `examples/{go,nodejs,python,dotnet,java}/`, `examples/yaml_test.go` | Optional import-option documentation example and generated language compile coverage; do not add it to canonical all-resources program by default |

### Task 1: Verify API contract and generate GitHub client

**Interfaces:** Preserve pinned upstream source/hash. Consume existing authenticated client and raw mutation conventions. Expected selected operations are existing `organization.active`, existing `gitProvider.remove`, and only `github.one`/`github.update`; verify exact response and parent-delete contract against pinned Dokploy router/service/schema sources before choosing corrected types.

- [ ] Add `TestGitHubIntegrationOpenAPIContract` in `openapi/cmd/normalize/main_test.go`: assert only necessary added selections, response refs/whitelisted identity and settings fields, update required `githubId`, `gitProviderId`, `name`, `githubAppName`, and remove required parent ID; retain unrelated operations and all six GitLab operation assertions. Capture actual current base count at execution and assert additive expected count, not a stale baseline.
- [ ] Inspect the pinned Dokploy source evidence for `github.one`, `github.update`, and `gitProvider.remove` (including delete parent semantics). If it contradicts the approved spec or cannot establish response shape, stop and escalate the concrete contradiction; do not invent corrections or duplicate external API research.
- [ ] Add only necessary selected operations and source-grounded response corrections in `openapi/operations.txt` and `openapi/corrections.json`. Run `SAFE go test ./openapi/cmd/normalize -run TestGitHubIntegrationOpenAPIContract -count=1`; expected: pass after source updates.
- [ ] Run `SAFE make generate_openapi`; add `TestGitHubGeneratedClientContract` to assert encoded relation ID, exact update body, whitelisted nullable/incomplete settings handling, parent deletion ID, and irrelevant success JSON not invalidating acknowledged mutations. Run `SAFE go test ./internal/client/... -run TestGitHubGeneratedClientContract -count=1`.
- [ ] Run `SAFE make check_openapi`, `git diff --check`, review generated diff/status; commit explicit API source/test/generated paths as `feat: generate GitHub integration API contract`.

### Task 2: Public inferred resource, validation, no-create and preview contract

**Files:** New `provider/github_integration.go`, `provider/github_integration_test.go`, preview test; defer registration/schema to Task 4.

**Interfaces:** `GitHubIntegrationArgs{ Name string \`pulumi:"name"\`; GitHubAppName string \`pulumi:"githubAppName"\` }`. `GitHubIntegrationState` embeds args and adds `GitHubID`, `GitProviderID`, `OrganizationID string` tagged `githubId`, `gitProviderId`, `organizationId`. Resource `GitHubIntegration{client clientFactory}`; inferred methods use the corresponding `infer.CheckRequest`, `DiffRequest`, `CreateRequest` generic signatures. Check accepts computed values but evaluates each known required input independently. Diff marks both fields in-place (`p.Update`), no replacement. Create preview returns unresolved identity outputs/no client; real Create always errors with the same fixed safe instruction and no API calls.

- [ ] Add `TestGitHubIntegrationCheckKnownAndComputed`, `TestGitHubIntegrationDiff`, `TestGitHubIntegrationCreateRejectedNoAPI`, and `TestGitHubIntegrationPreviewIdentity`. Assert empty known name and app name both fail even when the other is computed; both changed properties update in place; real Create returns the fixed setup/import diagnostic and test client is never invoked; create preview IDs are computed and update preview preserves all prior IDs, echoes inputs, and makes no client call.
- [ ] Run `SAFE go test -short ./provider -run 'TestGitHubIntegration(Check|Diff|CreateRejected|Preview)' -count=1`; expected: missing resource/types or failing assertions.
- [ ] Implement annotations, exact public args/state, Check, concrete validation helper, Diff, dependency wiring and preview behavior. Do not add credential/readiness fields or replacement flags.
- [ ] Run targeted tests and `SAFE make test_provider`; inspect/stage only Task 2 files and commit `feat: define import-only GitHubIntegration contract`.

### Task 3: Verified observation, import/read, update and delete

**Files:** New `provider/github_integration_observation.go`, `provider/github_integration_lifecycle.go` and tests.

**Interfaces:** `githubObservation{State GitHubIntegrationState; ConfigurationComplete bool}` (no token fields). `readGitHubIntegration(ctx context.Context, api *client.Client, relationID string, prior GitHubIntegrationState, organizationID string) (githubObservation, error)`. Implement inferred `Read`, `Update`, and `Delete` using args/state types from Task 2 and existing configured client patterns. Read error sanitizer preserves cancellation/deadline and structured not-found classification while discarding unsafe prose.

- [ ] Add offline tests named `TestGitHubIntegrationReadAndImport`, `TestGitHubIntegrationObservationValidation`, `TestGitHubIntegrationIncompleteConfiguration`, `TestGitHubIntegrationDiagnostics`, `TestGitHubIntegrationUpdateVerified`, `TestGitHubIntegrationUpdatePartialState`, `TestGitHubIntegrationDeleteVerified`, and `TestGitHubIntegrationDeleteAmbiguousFailure`. Pin relation/nested-parent/type/org validation; prior identity/org cannot change; explicit incomplete app name remains readable but omitted modeled data is malformed; no credentials leak; pre-read then exact two-setting update/readback; safe last-observed state after partial mutation; verified parent-only removal; pre-read absence skips remove; only genuine structured not-found clears state and ambiguous failure stays error.
- [ ] Run `SAFE go test -short ./provider -run 'TestGitHubIntegration(Read|Observation|Incomplete|Diagnostics|Update|Delete)' -count=1`; expected: missing helpers/methods or contract failures.
- [ ] Implement whitelisted response projection, active-org verification, sanitized diagnostics, in-place mutation and readback, and verified parent removal. Require nonempty `githubAppName` before update; do not invent missing configuration. Use raw mutation acknowledgment/body-close conventions; no retry and no delete by unverified parent.
- [ ] Run focused tests, `SAFE make test_provider`, and `SAFE make test_race`; inspect sequence and `git diff --check`; commit explicit lifecycle source/tests as `feat: manage imported GitHub integration state`.

### Task 4: Register, generate schema/SDKs, docs and import examples

**Files:** `provider/provider.go`, provider schema/inventory/preview tests; generated schema and five SDKs; README, GitHub/import guides, website inventory/reference sources/tests, YAML import example/readme and generated language examples/tests.

**Interfaces:** Register only `infer.Resource(&GitHubIntegration{client: configuredClient})` under token `dokploy:index:GitHubIntegration`. Schema exactly has two required inputs and three ordinary identity outputs, no replacement flags, credential/sharing/readiness fields. Import option uses relation ID placeholder `githubId`; source variants in Application/Compose are unchanged.

- [ ] Add `TestSchemaGitHubIntegrationContract`, provider registration/explicit-dependency assertion, and additive resource/invoke/schema inventory assertions. Ensure prior inventory remains and only expected full-operation counts increase; retain all six GitLab selection checks.
- [ ] Run those tests before registration; expected: missing resource assertion. Register resource and update provider description, then run targeted tests and `SAFE make test_provider`.
- [ ] Run `SAFE make codegen`, `SAFE make check_codegen`, `SAFE make build_sdks`; inspect generated schema/SDK diff. Generated files must come from source, and no unrelated schema/SDK removals are acceptable.
- [ ] Document manual Dokploy/GitHub App setup, `githubId` import, subsequent managed use, incomplete-setting repair, fixed unsupported Create, and that destroy removes the Dokploy record but not the GitHub App/authorization; warn about unmanaged workloads. Add an explicitly opt-in Pulumi import-options snippet with reserved placeholders, not a default resource in the canonical all-resources program. Keep examples free of secrets/real IDs.
- [ ] Regenerate language examples/reference docs from canonical sources. Add compiled SDK contract coverage for the new args/state/resource in all five languages and test docs inventories/routes. Run `SAFE make build_sdks`, `SAFE make test_examples`, `SAFE make docs_check`, plus relevant website tests.
- [ ] Run final offline `SAFE make lint`, `SAFE make test_provider`, `SAFE make test_race`, `SAFE make provider`, `SAFE make check_openapi`, `SAFE make check_codegen`, `SAFE make build_sdks`, `SAFE make test_examples`, `SAFE make docs_check`, then `git diff --check`. Record unavailable checks without implying pass. Inspect status and complete diff; stage only explicit task paths and commit `feat: publish GitHub integration resource and docs`.

## Self-review

- Coverage: API evidence and additive selection (Task 1); public contract/Create rejection/previews (Task 2); verified read/import/update/delete and safe partial failures (Task 3); registration/schema/SDK/docs/import examples (Task 4).
- Type consistency: all tasks use `GitHubIntegrationArgs`, `GitHubIntegrationState`, and `GitHubIntegration`; Pulumi relation token is consistently `githubId`.
- Review focus cases are explicitly assigned to owning task tests above. Existing Application/Compose source types and GitLab adoption are untouched.
- No API response correction is assumed in advance; a pinned-source/spec contradiction blocks Task 1 for clarification rather than inviting invention.
