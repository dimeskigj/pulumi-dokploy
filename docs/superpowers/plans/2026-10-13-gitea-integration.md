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
- Modify: `openapi/corrections.json`
- Test: `openapi/cmd/normalize/main_test.go`
- Preserve: `openapi/source.json` pin; do not modify it.
- Modify (generated): `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`
- Test: `internal/client/gitea_contract_test.go`

**Interfaces:**
- Consumes: pinned API behavior documented in the approved spec and follow-up research/verification reports.
- Produces: `GiteaCreateWithResponse`/`GiteaCreate` (raw), `GiteaOneWithResponse`, `GiteaUpdate` (raw), and their generated parameter/request types; reuse existing common organization/member/removal operations rather than adding duplicates. Correction models are `GiteaCreateAcknowledgment` (nonempty `giteaId`, returned `clientId` nullable, returned `giteaUrl`), `GiteaIntegrationRecord` (relation/parent IDs, managed URL/client/secret/callback/internal URL/group fields with pinned nullability/presence, nested parent), and `GiteaProviderIdentity` (parent ID/name/providerType/org/user). Whitelist only needed fields; no callback tokens/scopes/expiry models.

- [ ] **Step 1: Add `TestGiteaIntegrationOpenAPIContract` first** in `openapi/cmd/normalize/main_test.go`, asserting the three added Gitea selections, named corrected response models/nullability, and preservation of common/GitLab/GitHub operations; reject unrelated OAuth/token/repository/test operations. Capture actual baseline and update intended total assertions additively.
- [ ] **Step 2: Run `SAFE go test ./openapi/cmd/normalize -run '^TestGiteaIntegrationOpenAPIContract$' -count=1`** and capture missing-selection failure before source changes.
- [ ] **Step 3: Select only the required operations and correct response projections** in canonical OpenAPI source, then run the repository OpenAPI generation target. Do not change the upstream pin or hand-edit generated outputs.
- [ ] **Step 4: Add `TestGiteaGeneratedClientContract`** in `internal/client/gitea_contract_test.go`: assert encoded relation ID, create acknowledgment, nullable/present detail settings, secret/internal-URL/group payloads, and raw mutation acknowledgment handling. Run `SAFE go test ./openapi/cmd/normalize ./internal/client/... -count=1`; expected: pass.
- [ ] **Step 5: Commit** canonical OpenAPI source/tests and generated OpenAPI/client files, then run `SAFE make check_openapi` against committed output; expected: no drift.

### Task 2: Define and validate the public resource contract

**Files:**
- Create: `provider/gitea_integration.go`
- Test: `provider/gitea_integration_test.go`

**Interfaces:**
- Produces: `GiteaIntegrationArgs` with fields `Name string`, `GiteaURL string`, `ClientID string`, `ClientSecret string`, `RedirectURI string`, `GiteaInternalURL *string`, and `OrganizationName string` using matching Pulumi tags; mark `ClientSecret` and `GiteaInternalURL` secret.
- Produces: `GiteaIntegrationState` embedding args plus `GiteaID`, `GitProviderID`, `OrganizationID` string outputs; `GiteaIntegration{client clientFactory}`; `validateGiteaIntegrationArgs(GiteaIntegrationArgs) error`.
- Produces: Check/Diff/WireDependencies using the existing infer generic interfaces, plus `validateGiteaIntegrationBaseURL(raw string) bool` without unsafe parse diagnostics. Public schema/provider-level preview tests remain in Task 4, after lifecycle methods and registration exist.

- [ ] **Step 1: Write `TestGiteaIntegrationCheck`, `TestGiteaIntegrationURLValidation`, and `TestGiteaIntegrationDiff`** covering required values, `organizationName` default/empty canonicalization, computed-known independent failures, valid paths, and all specified invalid URL cases. Assert both secret struct tags and ordinary output-ID tags directly; do not require global registration yet.
- [ ] **Step 2: Run `SAFE go test ./provider -run 'TestGiteaIntegration(Check|URL|Diff)' -count=1`** and capture missing-type/behavior failure.
- [ ] **Step 3: Implement args, state, annotations, `Check`, `validateGiteaIntegrationArgs`, in-place `Diff`, and dependency wiring** in `provider/gitea_integration.go`; use `validWebURL` where appropriate and a Gitea-base helper that rejects userinfo/query/fragment/bare delimiters without rewriting the input.
- [ ] **Step 4: Run direct public-contract tests**, defaults, URL preservation and in-place diffs with `SAFE go test ./provider -run 'TestGiteaIntegration(Check|URL|Diff)' -count=1`, then `SAFE make test_provider`; full served-schema/secret propagation tests belong to Task 4.
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
- Produces: `giteaIntegrationPreviewState(inputs GiteaIntegrationArgs, relationID string, prior GiteaIntegrationState) GiteaIntegrationState`; Update uses it on DryRun. Direct preview tests inspect empty new IDs/preserved prior IDs, not engine computed-property encoding before registration.

- [ ] **Step 1: Write `TestGiteaIntegrationReadAndImport`, `TestGiteaIntegrationObservationValidation`, and `TestGiteaIntegrationDiagnostics`** for verified detail/import identity, type/org/parent mismatch, genuine not-found only, secret-preserving omitted response, incomplete imported settings, and credential-bearing malformed/error responses.
- [ ] **Step 2: Run `SAFE go test ./provider -run 'TestGiteaIntegration(Read|Observation|Diagnostics)' -count=1`** and capture failure before helper/method implementation.
- [ ] **Step 3: Implement whitelist observation and sanitized errors** in `provider/gitea_integration_observation.go`; only retain managed configuration, three IDs, and internal owner identity needed for verified operations. Null internal URL maps to omission and null/missing/empty organization name to empty string.
- [ ] **Step 4: Write `TestGiteaIntegrationCreateAcknowledged` and `TestGiteaIntegrationCreatePartialState`** asserting exact single create payload, positive acknowledged ID, full configuration/detail verification, no list/name reconciliation/retry, and partial state limited to acknowledged/observed values with unread parent/org empty. Run `SAFE go test ./provider -run TestGiteaIntegrationCreate -count=1` before Create implementation; expected failure.
- [ ] **Step 5: Implement `Create`** in `provider/gitea_integration_lifecycle.go`; dry-run makes no API calls and leaves IDs computed. Obtain active org/user only if required by verified create contract. On positive create acknowledgment, read by returned relation ID; never claim unread parent/org identity.
- [ ] **Step 6: Write `TestGiteaIntegrationUpdateVerified`, `TestGiteaIntegrationUpdatePartialState`, and `TestGiteaIntegrationDeleteVerified`** for pre-read identity, exact payload/readback, group/internal clears, no omitted-secret rotation proof or callback fields, verified parent removal, genuine absence and cancellation/no-retry behavior. Run `SAFE go test ./provider -run 'TestGiteaIntegration(Update|Delete)' -count=1` before mutation implementation; expected failure.
- [ ] **Step 7: Implement `Read`, `Update`, and `Delete`** using verified IDs; only structured not-found removes state, updates require complete concrete managed credentials, mutation errors are not retried, and delete errors succeed only after verified absence.
- [ ] **Step 8: Add `TestGiteaIntegrationPreviewState` and the specified pure helper** proving create preview makes no API calls and leaves new IDs empty, while Update preview retains IDs/echoes inputs without client calls. Engine-level computed-property assertions belong to Task 4.
- [ ] **Step 9: Run focused lifecycle/observation/preview tests and race-safe relevant provider tests** via SAFE launcher, then commit these source/tests.

### Task 4: Register resource and generate provider schema plus all SDKs

**Files:**
- Modify: `provider/provider.go`
- Modify: `provider/provider_test.go`, `provider/schema_test.go` and any existing resource inventories affected by the additive registration
- Modify (generated): provider schema and Go, Node.js, Python, .NET, and Java SDK outputs
- Test: `provider/gitea_integration_schema_test.go`

**Interfaces:**
- Consumes: `GiteaIntegration` resource and state from Tasks 2–3.
- Produces: additive `dokploy:index:GiteaIntegration` schema registration with exactly seven inputs, the same managed configuration in outputs plus `giteaId`, `gitProviderId`, `organizationId`, secret metadata for client secret/internal URL, and no replacement behavior.

- [ ] **Step 1: Add `TestSchemaGiteaIntegrationContract` and `TestGiteaIntegrationPreviewIdentity`** for exact input/output sets, five required inputs, both input/output secret markings, ordinary stable update IDs, computed create IDs and no preview API calls; exclude readiness/scopes/tokens/sharing/parent selector.
- [ ] **Step 2: Run the targeted schema tests and confirm failure** because the resource is not yet registered.
- [ ] **Step 3: Register `GiteaIntegration` additively** beside existing integration resources and add its canonical schema annotations; preserve all pre-existing resources and invokes.
- [ ] **Step 4: Regenerate provider schema and five SDKs** using approved portable pinned generator tooling; do not hand-edit generated files. Record actual baseline resource counts before comparing output.
- [ ] **Step 4a: Add compiled contract coverage in the existing five-language SDK/example test fixtures** for `GiteaIntegration` args/state/resource, required settings, optional internal URL/group, and ordinary ID outputs. Inspect source-generated secret metadata in every SDK; keep maintained Go example fixtures during regeneration.
- [ ] **Step 5: Run provider schema/preview tests and all five SDK builds** with SAFE launcher, inspect additive generated output, then commit registration, schema, all five SDKs and source contract tests.
- [ ] **Step 6: Run `SAFE make check_codegen` and `SAFE make check_openapi`** against committed outputs; expected: no unintended drift. Report and fix only concrete source/generated defects.

### Task 5: Add canonical docs/examples and complete offline verification

**Files:**
- Create: `website/src/content/docs/guides/gitea-integrations.mdx`
- Modify: `README.md`, `examples/yaml/{Pulumi.yaml,README.md}`, `examples/yaml_test.go`, `website/astro.config.mjs`, `website/scripts/reference-model.mjs`, relevant import/secret/lifecycle guides and website source/built tests
- Regenerate: `examples/{go,nodejs,python,dotnet,java}/`, `website/src/content/docs/reference/gitea-integration.mdx`, generated reference types and complete-example embed using existing generators
- Test: `TestCanonicalYAMLGiteaIntegration`, resource/reference/guide/secret/manual-auth source tests and built-route checks; no workload source changes

**Interfaces:**
- Consumes: registered generated resource from Task 4.
- Produces: consistent generated reference docs and examples for manual OAuth setup, authorization boundary, ID-only import, secret handling, optional clears, lifecycle, and non-goals.

- [ ] **Step 1: Add or adapt example tests/docs assertions** ensuring the resource is described as configuration management only; no claims that Pulumi creates/authorizes OAuth or validates connectivity, and no new Gitea workload source type.
- [ ] **Step 2: Update canonical YAML secret config/resource and its README**, then regenerate all five language examples with `SAFE make gen_examples` using the pinned isolated converter. Show manual OAuth setup/authorization and output `giteaId`; do not introduce an unsupported Gitea workload source or imply an ID dependency authorizes deployment. Never edit generated translations by hand.
- [ ] **Step 3: Generate reference docs from schema** and update navigation/inventory only if generator requires it; document secret config handling, ID-only import, optional clears, generic parent cascade, and unmanaged workload-reference limitation.
- [ ] **Step 4: Run applicable example builds/tests, docs check, `make lint`, `make test_provider`, `make test_race`, `make provider`, all five SDK builds, and `git diff --check`** through SAFE launcher with live acceptance disabled. Do not run live tests, external calls, release/publication, push, or merge.
- [ ] **Step 5: Review actual status/diff and generated drift**, confirm only intended files changed, then commit docs/examples/generated references.

## Integration sequencing and final verification

- Keep independent product-source work in isolated task worktrees only when files/interfaces do not overlap. Serialize shared OpenAPI, registration, SDK generation, and docs-generation integration; do not overwrite concurrent GitHub or pre-existing work.
- Do not run schema/SDK drift checks against intermediate generated output. Commit intended source/output first, then run checks; capture current actual baseline counts at integration time.
- After all task commits, inspect `git status --short`, relevant diff, and `git diff --check`. Report each offline command's result and any unavailable check; no live acceptance or external mutation is authorized.
- Final committed-output validation: `SAFE make lint`, `SAFE make test_provider`, `SAFE make test_race`, `SAFE make provider`, `SAFE go test ./openapi/cmd/normalize ./internal/client/... -count=1`, `SAFE make check_openapi`, `SAFE make check_codegen`, `SAFE make build_sdks`, `SAFE make test_examples`, `SAFE make docs_check`, and `git diff --check`. Expected: successful offline checks with no unintended generated drift; explicitly report gated skips or unavailable tooling.

## Plan self-review

- All approved public fields, independent computed validation, URL constraints, defaults and secret metadata map to Tasks 2 and 4.
- Direct acknowledged create, incomplete imports, optional clears, unobservable rotation, verified identities, safe partial failures, absence classification and cancellation map to Task 3.
- Generated API/client contracts, all five SDKs, canonical examples and manual OAuth/import/destruction guidance map to Tasks 1, 4 and 5.
- Interface names agree across tasks; provider-level previews run only after registration, and drift checks run against committed intentional outputs. Implementation remains gated on written-plan approval with Luna SDD already selected.
