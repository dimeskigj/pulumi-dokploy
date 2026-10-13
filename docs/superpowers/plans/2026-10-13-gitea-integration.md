# GiteaIntegration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a managed Pulumi `dokploy:index:GiteaIntegration` resource for Dokploy Gitea integration configuration and lifecycle.

**Architecture:** Follow the inferred-resource and generated OpenAPI client patterns used by `GitLabIntegration`, but use the Gitea API's positively acknowledged `giteaId` rather than GitLab's list-based identity recovery. Keep raw API responses behind a whitelist observation layer, preserve secret/dependency metadata, register additively, and generate all SDKs and reference docs from canonical source.

**Tech Stack:** Go, pulumi-go-provider infer, oapi-codegen generated client, Pulumi schema/SDK codegen, Makefile offline checks.

**Spec:** `docs/superpowers/specs/2026-10-13-gitea-integration-design.md`

## Global Constraints

- Keep the Dokploy OpenAPI pin at `cebd3808565ea9bed0791961bc25c60513d94c5a`; use the pinned API evidence in `.superpowers/sdd/2026-10-04-gitlab-integration/followup-api-research.md` and `followup-api-verification.md`; do not re-research token or repository-discovery scopes.
- Implement only `gitlab`-analogous Gitea resource operations: active organization/user identity as needed, `gitea.create`, `gitea.one`, `gitea.update`, provider list/identity verification as required, and generic `gitProvider.remove`; no OAuth callback/token/test/repository APIs.
- Public inputs are exactly `name`, `giteaUrl`, `clientId`, `clientSecret`, `redirectUri`, `giteaInternalUrl`, and `organizationName`; the first five are required. Do not add readiness, tokens, expiry, username, scopes, sharing, parent ID, or public organization selector.
- `clientSecret` and `giteaInternalUrl` remain secret in input/output schemas, state, SDKs, and dependency wiring; all three IDs are ordinary outputs.
- Gitea URL and callback validation must respect the spec: known values validated independently, computed values not rejected as empty, absolute HTTP(S), no URL rewriting; base URLs permit paths but reject userinfo/query/fragment/bare delimiters, callback rejects userinfo.
- Create uses the acknowledged positive `giteaId`, reads back by that ID, and never performs name/list reconciliation or speculative cleanup/retry. Partial state after failed readback may assert only acknowledged/verified fields; do not assert unread parent or organization identity.
- Updates are in-place and send exact managed settings; clear `organizationName` with empty string and clear absent `giteaInternalUrl` with explicit null. Secret omission during refresh is not rotation evidence; updates require a concrete nonempty desired secret.
- Preserve existing Application/Compose source variants and their external-ID consumers; do not add a Gitea workload source type.
- Use the approved SAFE launcher `/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run`, actual temp directory under `/tmp/opencode`, stripped inherited Dokploy credentials, live acceptance off including nested mise, and portable pinned generator converter 1.38.7 with cache/preflight available. No live acceptance, external calls, release, publication, push, or merge.
- This is a new subsystem plan. Recalculate actual baseline counts at integration; GitHub resource work may land earlier or concurrently, so do not reuse old 23/145 counts. Commit only intentional outputs before running schema/SDK drift checks.
- Retain this plan and the approved spec unless the user explicitly approves cleanup.

## Review Focus

1. Computed or partially known inputs must not hide errors in another known field; test per-field validation in Task 2.
2. URL edge cases (userinfo, paths, query/fragment, bare delimiters, null/empty optional URL) must have the specified accept/reject behavior; test in Task 2.
3. A successful create response followed by failed/malformed readback must preserve only acknowledged identity/settings and must not invent parent/org IDs or retry; test in Task 3.
4. Import/read with omitted secret must preserve prior secret, while incomplete imported OAuth configuration cannot be updated; test in Task 3.
5. Provider/organization/parent identity mismatch and sensitive malformed/error responses must fail safely without raw diagnostic leakage; test in Task 3.

---

### Task 1: Pin and generate the minimal Gitea API contract

**Files:**
- Modify: `openapi/operations.txt`
- Preserve: `openapi/source.json` pin; do not modify it.
- Modify (generated): `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`
- Test: `internal/client/gitea_contract_test.go`

**Interfaces:**
- Consumes: pinned API behavior documented in the approved spec and follow-up research/verification reports.
- Produces: generated `GiteaCreate`, `GiteaOne`, `GiteaUpdate`, `GitProviderGetAll`, `GitProviderRemove`, `OrganizationActive`, and `UserGet` client operations as needed, with response models containing only required Gitea relation and nested parent identity/configuration fields. Create response must model positive `giteaId`, `clientId`, and `giteaUrl`; correct broad placeholder responses from reviewed source.

- [ ] **Step 1: Add contract tests first** in `internal/client/gitea_contract_test.go` asserting operation selection, pinned create response shape, detail identity/config fields and nested parent identity, plus generic remove operation availability; reject unrelated OAuth/token/repository/test operations.
- [ ] **Step 2: Run contract test to verify it fails** with missing generated Gitea contract/operations.
- [ ] **Step 3: Select only the required operations and correct response projections** in canonical OpenAPI source, then run the repository OpenAPI generation target. Do not change the upstream pin or hand-edit generated outputs.
- [ ] **Step 4: Run full OpenAPI normalization tests**, including the new contract test and existing OpenAPI tests; separately run `make check_openapi` after the intentional changes are committed, not before.
- [ ] **Step 5: Commit** only canonical OpenAPI source and generated OpenAPI/client files for this task.

### Task 2: Define and validate the public resource contract

**Files:**
- Create: `provider/gitea_integration.go`
- Test: `provider/gitea_integration_test.go`

**Interfaces:**
- Produces: `GiteaIntegrationArgs` with fields `Name string`, `GiteaURL string`, `ClientID string`, `ClientSecret string`, `RedirectURI string`, `GiteaInternalURL *string`, and `OrganizationName string` using matching Pulumi tags; mark `ClientSecret` and `GiteaInternalURL` secret.
- Produces: `GiteaIntegrationState` embedding args plus `GiteaID`, `GitProviderID`, `OrganizationID` string outputs; `GiteaIntegration{client clientFactory}`; `validateGiteaIntegrationArgs(GiteaIntegrationArgs) error`.

- [ ] **Step 1: Write failing Check/default/validation tests** covering required values, `organizationName` default/empty canonicalization, computed-known independent failures (including a known invalid field alongside another computed input), valid paths, and invalid URL cases from Review Focus 1–2.
- [ ] **Step 2: Run targeted tests and confirm failure** on missing `GiteaIntegration` and validation behavior.
- [ ] **Step 3: Implement args, state, annotations, `Check`, `validateGiteaIntegrationArgs`, in-place `Diff`, and dependency wiring** in `provider/gitea_integration.go`; use `validWebURL` where appropriate and a Gitea-base helper that rejects userinfo/query/fragment/bare delimiters without rewriting the input.
- [ ] **Step 4: Add tests for exact public field inventory and secret metadata**, defaults, URL canonical preservation, and in-place property diffs; run `go test ./provider -run 'TestGiteaIntegration(Check|URL|Diff|Schema)'` with the SAFE launcher.
- [ ] **Step 5: Commit** the resource contract and its tests.

### Task 3: Implement observation, acknowledged create, read/import, update, delete, and previews

**Files:**
- Create: `provider/gitea_integration_observation.go`
- Create: `provider/gitea_integration_lifecycle.go`
- Test: `provider/gitea_integration_observation_test.go`
- Test: `provider/gitea_integration_lifecycle_test.go`
- Test: `provider/gitea_integration_preview_test.go`

**Interfaces:**
- Consumes: Task 1 generated client operations and Task 2 `GiteaIntegrationArgs`, `GiteaIntegrationState`, and `validateGiteaIntegrationArgs`.
- Produces: `giteaObservation{State GiteaIntegrationState, OwnerUserID string, ConfigurationComplete bool}`; `readGiteaIntegration(ctx context.Context, api *client.Client, id string, prior GiteaIntegrationState, organizationID string) (giteaObservation, error)`; `sanitizeGiteaIntegrationError(error) error`; `GiteaIntegration` methods `Create`, `Read`, `Update`, `Delete`. Use concrete infer generic signatures matching existing inferred resource methods in neighboring lifecycle code.

- [ ] **Step 1: Write observation and lifecycle tests first** for verified detail/import identity, type/org/parent mismatch, genuine not-found only, secret-preserving omitted response, malformed identity, and safe diagnostics (Review Focus 4–5).
- [ ] **Step 2: Run focused tests to verify failure** before implementing observation or lifecycle methods.
- [ ] **Step 3: Implement whitelist observation and sanitized errors** in `provider/gitea_integration_observation.go`; only retain managed configuration, three IDs, and internal owner identity needed for verified operations. Null internal URL maps to omission and null/missing/empty organization name to empty string.
- [ ] **Step 4: Write create tests** asserting one create request with exact managed payload, positive acknowledged `giteaId`, detail readback verification, no list/name reconciliation, no retry, partial state only from acknowledged/observed values on readback failure (Review Focus 3), and safe failure on failed create without acknowledgment.
- [ ] **Step 5: Implement `Create`** in `provider/gitea_integration_lifecycle.go`; dry-run makes no API calls and leaves IDs computed. Obtain active org/user only if required by verified create contract. On positive create acknowledgment, read by returned relation ID; never claim unread parent/org identity.
- [ ] **Step 6: Write update/delete tests** asserting pre-read identity checks, exact update payload and verified readback, explicit empty `organizationName` and null `giteaInternalUrl`, secret omission not treated as rotation, no callback/token/scope fields, delete with verified parent ID only, idempotent already-absent behavior, and generic parent-cascade semantics.
- [ ] **Step 7: Implement `Read`, `Update`, and `Delete`** using verified IDs; only structured not-found removes state, updates require complete concrete managed credentials, mutation errors are not retried, and delete errors succeed only after verified absence.
- [ ] **Step 8: Add pure preview tests/helper** proving create preview is API-free and leaves IDs computed, and update preview retains IDs/echoes desired managed inputs with no API calls. Do not use provider-level computed preview tests until registration exists; do not call Update in tests until this task has implemented it.
- [ ] **Step 9: Run focused lifecycle/observation/preview tests and race-safe relevant provider tests** via SAFE launcher, then commit these source/tests.

### Task 4: Register resource and generate provider schema plus all SDKs

**Files:**
- Modify: `provider/provider.go`
- Modify (generated): provider schema and Go, Node.js, Python, .NET, and Java SDK outputs
- Test: `provider/gitea_integration_schema_test.go`

**Interfaces:**
- Consumes: `GiteaIntegration` resource and state from Tasks 2–3.
- Produces: additive `dokploy:index:GiteaIntegration` schema registration with exactly seven inputs, outputs `giteaId`, `gitProviderId`, `organizationId`, secret metadata for client secret/internal URL, and no replacement behavior.

- [ ] **Step 1: Add failing schema/inventory tests** for exact input/output sets, required inputs, both secret markings, all ordinary ID outputs, and absence of readiness/scopes/tokens/sharing/parent selector.
- [ ] **Step 2: Run the targeted schema tests and confirm failure** because the resource is not yet registered.
- [ ] **Step 3: Register `GiteaIntegration` additively** beside existing integration resources and add its canonical schema annotations; preserve all pre-existing resources and invokes.
- [ ] **Step 4: Regenerate provider schema and five SDKs** using approved portable pinned generator tooling; do not hand-edit generated files. Record actual baseline resource counts before comparing output.
- [ ] **Step 5: Run provider schema tests, all five SDK builds, `make check_codegen`, and codegen drift checks** with SAFE launcher; run `make check_openapi` now that intended outputs have been committed.
- [ ] **Step 6: Commit** registration, schema, all five SDKs, and generated contract tests.

### Task 5: Add canonical docs/examples and complete offline verification

**Files:**
- Create or regenerate: `website/src/content/docs/reference/giteaIntegration.mdx` (use actual generator canonical naming)
- Modify: resource documentation navigation/inventory if required
- Modify: canonical examples across supported language examples and corresponding Pulumi manifests/config descriptions
- Test: documentation/example checks; no workload source model changes

**Interfaces:**
- Consumes: registered generated resource from Task 4.
- Produces: consistent generated reference docs and examples for manual OAuth setup, authorization boundary, ID-only import, secret handling, optional clears, lifecycle, and non-goals.

- [ ] **Step 1: Add or adapt example tests/docs assertions** ensuring the resource is described as configuration management only; no claims that Pulumi creates/authorizes OAuth or validates connectivity, and no new Gitea workload source type.
- [ ] **Step 2: Update canonical examples for all five languages** to show manual two-stage OAuth setup and use the managed integration's `giteaId` only as the integration ID; do not imply that ID dependency authorizes deployment. Keep existing workload-source variants unchanged.
- [ ] **Step 3: Generate reference docs from schema** and update navigation/inventory only if generator requires it; document secret config handling, ID-only import, optional clears, generic parent cascade, and unmanaged workload-reference limitation.
- [ ] **Step 4: Run applicable example builds/tests, docs check, `make lint`, `make test_provider`, `make test_race`, `make provider`, all five SDK builds, and `git diff --check`** through SAFE launcher with live acceptance disabled. Do not run live tests, external calls, release/publication, push, or merge.
- [ ] **Step 5: Review actual status/diff and generated drift**, confirm only intended files changed, then commit docs/examples/generated references.

## Integration sequencing and final verification

- Keep independent product-source work in isolated task worktrees only when files/interfaces do not overlap. Serialize shared OpenAPI, registration, SDK generation, and docs-generation integration; do not overwrite concurrent GitHub or pre-existing work.
- Do not run schema/SDK drift checks against intermediate generated output. Commit intended source/output first, then run checks; capture current actual baseline counts at integration time.
- After all task commits, inspect `git status --short`, relevant diff, and `git diff --check`. Report each offline command's result and any unavailable check; no live acceptance or external mutation is authorized.
