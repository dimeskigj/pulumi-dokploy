# BitbucketIntegration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a safe Pulumi `dokploy:index:BitbucketIntegration` resource with verified bodyless-create reconciliation and supported update/read/delete lifecycle.

**Architecture:** Extend only the pinned API operations required for this resource, generate all derived contracts, then implement resource contract/validation, observation and mutations, guarded creation and registration. Keep response-only credentials internal, project a whitelist into secret Pulumi arguments/state, and follow the existing GitLab reconciliation plus GitHub lifecycle conventions.

**Tech Stack:** Go, Pulumi Go Provider `infer`, OpenAPI/oapi-codegen generated client, Pulumi schema/SDK codegen for Go, Node.js, Python, .NET, and Java.

**Spec:** `docs/superpowers/specs/2026-10-13-bitbucket-integration-design.md`

## Global Constraints

- Preserve upstream pin `cebd3808565ea9bed0791961bc25c60513d94c5a`; add only operations listed by the spec; reuse existing common operations.
- Public inputs are exactly `name`, secret `apiToken`, secret `appPassword`, `bitbucketEmail`, `bitbucketUsername`, and `bitbucketWorkspaceName`; exactly one nonempty credential mode and its conditional identity are required on apply.
- Request strings are never null. Send empty string to clear the unused credential; never clear email by empty/null; omitted inactive identities/workspace preserve upstream values, while explicitly empty workspace clears.
- Read/import may expose incomplete or dual-credential legacy configuration; apply cannot. Preserve prior secret only for refresh omission/null and never let that mask mutation clear/rotation verification.
- Resource identity/import ID is relation `bitbucketId`; export only `gitProviderId` and `organizationId` beyond declared args. Do not add OAuth/session/readiness, `authId`, owner/sharing, connection test, or workload variants.
- Create uses a cryptographically unpredictable temporary marker, full before snapshot, and at most three cancellation-aware discovery attempts; retry discovery only with no candidate. Never retry mutation, adopt by name alone, or delete uncertain state.
- Adopt only a new parent and relation with matching marker/type/org/authenticated user and detail-verified full requested settings. Rename only acknowledged verified creates; preserve verified actual/last-observed partial state on failures.
- Delete only a parent from verified relation/parent identity; use structured not-found only; no upstream credential revocation or inferred cleanup.
- Sanitize diagnostics and cancellation safely; never emit secrets, raw upstream prose/bodies, IDs, or private URLs. Preserve both `errors.Is` cancellation/deadline and `errors.As(infer.ResourceInitFailedError)` after partial mutations so the infer boundary retains verified state. No live acceptance tests or credentials; preserve existing GitHub/GitLab behavior.
- Generate OpenAPI, generated client, schema, and all five SDKs from canonical sources. Never hand-edit generated output; commit intentional generated changes before drift checks.
- For actual local commands, set `SAFE=/tmp/opencode/gitlab-integration-tools-fwQ0lcJ2/bin/safe-run` and prefix Go/Make/compiler tooling with it; acceptance must remain disabled including nested mise, protected `.env` untouched, and actual local TMPDIR under `/tmp/opencode`. The launcher/temp path are execution-only; never hardcode either into product, generators or CI runners. Repository test automation inherits portable caller configuration.
- Baseline at planning: 24 resources, 12 invokes, 147 selected API paths. At implementation start capture actual counts; add exactly one resource and only the three missing Bitbucket operations, preserving existing invokes and all GitHub/GitLab operation assertions. Gitea may land first, so never overwrite its newer inventory with these planning counts.

## Review Focus

1. A computed credential must not mask a known invalid supplied email or known mode error — Task 2: `TestBitbucketCheckValidatesKnownFieldsIndependently`.
2. Refresh-preserved prior credentials must not fake successful rotation/clear verification — Task 3: `TestBitbucketUpdateRequiresFreshCredentialEvidence`.
3. A preexisting marker collision or candidate with reused/mismatched IDs must never be adopted — Task 4: `TestBitbucketCreateRejectsPriorOrInconsistentCandidate`.
4. A bodyless create followed by uncertain error must return only verified partial identity and never rename — Task 4: `TestBitbucketCreateUncertainVerifiedIdentityReturnsPartialWithoutRename`.
5. Generic `BAD_REQUEST` and unsafe response text must not be treated as not-found or leak secrets — Task 3: `TestBitbucketDeleteNotFoundClassificationAndSafeDiagnostics`.

---

## File map and task interfaces

Canonical API selection/corrections: `openapi/operations.txt`, `openapi/corrections.json`; preserve `openapi/source.json` and the upstream snapshot. Generated API outputs are `openapi/dokploy.json` and `internal/client/generated/generated.gen.go`. Provider files use focused `provider/bitbucket_integration*.go` names; annotations belong to the resource and registration to `provider/provider.go`. Generate `provider/cmd/pulumi-resource-dokploy/schema.json` and five SDKs from that source. Docs/examples derive from canonical YAML and website sources; maintained SDK fixtures live in `examples/sdk-contracts/`, outside replaced example directories.

Interfaces passed forward:

- Task 1 provides raw `BitbucketCreate`, `BitbucketUpdate`, typed `BitbucketOneWithResponse`, `BitbucketOneParams`, `BitbucketCreateJSONRequestBody`, `BitbucketUpdateJSONRequestBody`; reuse existing `GitProviderGetAllWithResponse`, raw `GitProviderRemove`, `UserGetWithResponse`, and `OrganizationActiveWithResponse`. Correction models are `BitbucketIntegrationRecord`, `BitbucketProviderIdentity`, and `BitbucketProviderSummary`; extend existing `GitProviderListEntry` with a presence-sensitive nullable `bitbucket` summary without replacing its GitLab field or common additional properties. The pinned summary uses `bitbucketId`, `bitbucketUsername`, `isConfigured`, `isDeprecated`, not invented `id`/`username` fields. Bodyless mutations consume/close raw HTTP responses.
- Task 2 defines `BitbucketIntegrationArgs`, `BitbucketIntegrationState`, `validateBitbucketIntegrationArgs(args BitbucketIntegrationArgs) error`, and `bitbucketIntegrationPreviewState(args BitbucketIntegrationArgs, id string, prior BitbucketIntegrationState) BitbucketIntegrationState`. Optional identity/workspace arguments use pointers to distinguish omitted from explicit empty.
- Task 3 defines `bitbucketCredentialEvidence{Specified bool; Cleared bool; Value string}`: omitted is unspecified, explicit null/empty is specified and cleared, and a nonempty observed value is specified and not cleared. `bitbucketObservation{State BitbucketIntegrationState; OwnerUserID string; APITokenEvidence bitbucketCredentialEvidence; AppPasswordEvidence bitbucketCredentialEvidence}` never enters the public schema.
- Task 3 produces `readBitbucketIntegration(ctx context.Context, api *client.Client, relationID string, prior BitbucketIntegrationState, organizationID string) (bitbucketObservation, error)`, `sanitizeBitbucketIntegrationError(err error) error`, and `bitbucketMutation(response *http.Response, err error, operation string) error`. It also produces `bitbucketIntegrationSettingsMatch(observed bitbucketObservation, desired BitbucketIntegrationArgs, expectedName string) bool` and `bitbucketIntegrationMutationState(observed bitbucketObservation, desired BitbucketIntegrationArgs) BitbucketIntegrationState`; the latter removes an explicitly cleared inactive credential without copying unconfirmed desired settings.
- Task 4 produces `bitbucketCreateSnapshot{parentIDs map[string]struct{}; relationIDs map[string]struct{}}`, `readBitbucketProviderList(ctx context.Context, api *client.Client) ([]generated.GitProviderListEntry, error)`, `snapshotBitbucketProviders(entries []generated.GitProviderListEntry) (bitbucketCreateSnapshot, error)`, and `discoverBitbucketIntegration(ctx context.Context, api *client.Client, args BitbucketIntegrationArgs, marker string, organizationID string, userID string, prior bitbucketCreateSnapshot) (bitbucketObservation, error)`. Parent/relation ID domains remain separate even if textual IDs coincide. Task 4 adds Create and registration; Task 3 owns Read/Update/Delete.

### Task 1: Minimal pinned API surface and accurate response presence

**Files:**
- Modify: `openapi/operations.txt` (only three missing Bitbucket selections), `openapi/corrections.json` (necessary response/request corrections); reuse already-selected common operations.
- Preserve: `openapi/source.json` and `openapi/upstream.json`; no pin/snapshot change.
- Generate: `openapi/dokploy.json`, `internal/client/generated/generated.gen.go` (never hand-edit).
- Test: `openapi/cmd/normalize/main_test.go`, new `internal/client/bitbucket_contract_test.go`.

**Interfaces:** Consumes pinned evidence in `.superpowers/sdd/2026-10-04-gitlab-integration/followup-api-research.md` and `followup-api-verification.md`; produces Task 1 API interfaces above.

- [ ] **Step 1: Capture starting surface and add `TestBitbucketIntegrationOpenAPIContract`** in the normalize tests. Assert only three missing selections, preserved existing operations, named response models, exact pinned summary keys, presence/nullability, parent identity and non-null request strings. Update total-count assertions additively from the actual execution baseline.
- [ ] **Step 2: Verify RED.** Run `SAFE go test ./openapi/cmd/normalize -run '^TestBitbucketIntegrationOpenAPIContract$' -count=1`; expected missing selection/model failure before source changes. Keep the test and proceed; intentional RED is not a blocker.
- [ ] **Step 3: Update selection/corrections and run `SAFE make generate_openapi`.** Reuse existing authenticated-member/common models. Correct nullable Bitbucket summary/detail configuration and nested parent identity from pinned evidence, including raw credential presence; no callback/session field becomes managed configuration. Preserve existing list fields/additional properties and never invent list `type` in place of source `providerType`.
- [ ] **Step 4: Add `TestBitbucketGeneratedClientContract`** for encoded relation query, summary/detail nullable-presence decoding, exact create/update request keys with no nulls, and malformed/irrelevant success JSON not invalidating raw mutation acknowledgment. Run `SAFE go test ./openapi/cmd/normalize ./internal/client/... -count=1`; expected PASS.
- [ ] **Step 5: Commit explicit owned API source/tests/generated paths** as `feat: model Bitbucket integration API`, then run `SAFE make check_openapi` against committed outputs; expected no drift. Include `internal/client/bitbucket_contract_test.go`, not unrelated client/source files.

### Task 2: Public inputs, validation, diff, preview helper, and secrets

**Files:**
- Create: `provider/bitbucket_integration.go` (args/state, annotations, validation, diff, secret/dependency annotations, preview helper).
- Test: `provider/bitbucket_integration_test.go`, `provider/bitbucket_integration_preview_test.go` (pure helper tests only until registration in Task 4).
- Reference: `provider/gitlab_integration.go`, `provider/github_integration_preview_test.go`, existing secret/optional pointer patterns.

**Interfaces:** Produces `BitbucketIntegrationArgs` fields `Name string`, `APIToken *string`, `AppPassword *string`, `BitbucketEmail *string`, `BitbucketUsername *string`, `BitbucketWorkspaceName *string`, with Pulumi tags `name`, `apiToken`, `appPassword`, `bitbucketEmail`, `bitbucketUsername`, `bitbucketWorkspaceName`; both credentials have secret tags. `BitbucketIntegrationState` embeds args and adds string `BitbucketID`, `GitProviderID`, `OrganizationID`, tagged `bitbucketId`, `gitProviderId`, `organizationId`. Define `BitbucketIntegration{client clientFactory}`, the validator/preview helper from the interface map, and Check/Diff/WireDependencies using existing infer generic signatures.

- [ ] **Step 1: Write `TestBitbucketCheckValidatesKnownFieldsIndependently`, `TestValidateBitbucketIntegrationArgs`, `TestBitbucketPreviewState`, `TestBitbucketSecretAnnotations`, and `TestBitbucketDiffOptionalPreservation`.** Cover each valid mode; neither/both/empty selected credential; invalid/missing conditional identity; invalid supplied inactive email; known bad email despite unknown credential; unknown preview retention; explicit-empty workspace vs omission; all changes in place, omission of inactive identity/workspace not a diff, and actual credential mode switches still a diff. Assert direct tags/dependency interfaces here; served-schema/encoded-preview assertions wait for Task 4.
- [ ] **Step 2: Run RED.** `SAFE go test ./provider -run 'Test(BitbucketCheck|ValidateBitbucketIntegrationArgs|BitbucketPreviewState|BitbucketSecretAnnotations|BitbucketDiff)' -count=1`; expected compile/test failures before implementation.
- [ ] **Step 3: Implement exact contract.** Check validates each known field, accepting computed preview inputs; apply requires exactly one nonempty concrete credential and its valid identity. Validate every supplied email. Ignore omitted inactive identities/workspace in Diff, not a requested mode switch or credential change; no replacement flags. Preview helper preserves omitted inactive identity/workspace and established IDs without inferring a different credential mode. Explicit per-input computedness/secret dependencies must not make stable ordinary IDs depend on changed settings; mark IDs NeverSecret.
- [ ] **Step 4: Run GREEN.** Repeat focused command; verify secret args depend into their own outputs, while IDs remain nonsecret.
- [ ] **Step 5: Commit.** `git add provider/bitbucket_integration.go provider/bitbucket_integration_test.go provider/bitbucket_integration_preview_test.go && git commit -m 'feat: define Bitbucket integration contract'`.

### Task 3: Whitelisted observations, read/import, update, and delete

**Files:**
- Create: `provider/bitbucket_integration_observation.go`, `provider/bitbucket_integration_lifecycle.go`.
- Test: `provider/bitbucket_integration_observation_test.go`, `provider/bitbucket_integration_lifecycle_test.go` using scripted HTTP responses; keep fixtures sanitized.
- Reference: `provider/gitlab_integration_observation.go`, `provider/gitlab_integration_lifecycle.go`, GitHub lifecycle and tests.

**Interfaces:** Consumes Task 1 client and Task 2 args/state/validator. Produces observation/evidence and lifecycle method contracts stated in the interface map.

- [ ] **Step 1: Write `TestReadBitbucketIntegration`, `TestBitbucketUpdateRequiresFreshCredentialEvidence`, `TestBitbucketUpdatePartialState`, and `TestBitbucketDeleteNotFoundClassificationAndSafeDiagnostics`.** Cover valid/incomplete/both-credential imports, relation/parent/type/active/prior-org mismatches, credential null/omission preservation on refresh, explicit-empty normalization, sensitive malformed/error bodies without prior state, exact non-null requests and inactive clear, supplied-only optionals, omitted versus empty workspace, fresh active match, missing-versus-null inactive clear, split/failed/malformed readback and post-update identity drift, verified parent-only delete, pre-read absence/no remove and ambiguous removal errors.
- [ ] **Step 2: Run RED.** `SAFE go test ./provider -run 'Test(ReadBitbucketIntegration|BitbucketUpdate|BitbucketDelete)' -count=1`; expected missing implementation/test failures.
- [ ] **Step 3: Implement observation and evidence helpers.** Verify relation/nested parent/type/active and previous org/IDs; whitelist public configuration. For refresh, omitted/null credentials preserve prior known secrets, but an explicit empty value is absent. Record each raw credential's specified/cleared/value evidence separately. Post-mutation projection makes a deliberately inactive credential absent when explicitly null/empty, even if refresh projection preserved it; never fill active rotation evidence from prior state. Settings comparison checks freshly observed active value, explicit inactive clear, expected name, active identity and only supplied optional values.
- [ ] **Step 4: Implement lifecycle methods.** Read by relation ID; only genuine structured not-found removes. Update pre-reads verified identities, sends full credentials with unused empty string and only specified optionals, closes raw body, never retries, reads back when context permits, compares newly active credential against fresh response and requires explicit null/empty evidence that unused credential cleared. Return verified actual/last-observed partial state on later error. Delete pre-verifies relation/parent/org/stable identity, removes only parent, then confirms absence; never interpret generic bad request as not-found.
- [ ] **Step 5: Add `TestBitbucketUpdateCancellation` and `TestBitbucketDeleteCancellation`, then run GREEN.** Deterministic cancellation/deadline cases assert no further request/retry, retained verified state, `errors.Is` context classification and post-mutation init-failed marker. Use safe joined error classification as in GitHub, not a plain context error that loses partial output. Run `SAFE go test ./provider -run 'Test(ReadBitbucketIntegration|BitbucketUpdate|BitbucketDelete)' -count=1`, `SAFE make test_provider`, `SAFE make test_race`; expected PASS. Actual infer-boundary partial-state tests wait until Create exists in Task 4.
- [ ] **Step 6: Commit.** `git add provider/bitbucket_integration_observation.go provider/bitbucket_integration_lifecycle.go provider/bitbucket_integration_observation_test.go provider/bitbucket_integration_lifecycle_test.go && git commit -m 'feat: manage Bitbucket integration lifecycle'`.

### Task 4: Guarded create, registration, schema, and generated SDKs

**Files:**
- Create: `provider/bitbucket_integration_create.go`, `provider/bitbucket_integration_create_test.go`.
- Modify: `provider/provider.go`, `provider/provider_test.go`, `provider/schema_test.go`; source resource annotations/dependencies only where required.
- Generate: `provider/cmd/pulumi-resource-dokploy/schema.json`, `sdk/{go,nodejs,python,dotnet,java}/` via canonical generation.
- Test: create tests, new `provider/bitbucket_integration_provider_test.go`; maintained compiled fixtures `examples/sdk-contracts/{go,nodejs,python,dotnet,java}` (these are handwritten fixture sources, not generated SDK outputs).
- Reference: `provider/gitlab_integration_create.go`, `provider/gitlab_integration_create_test.go`, `provider/provider.go`.

**Interfaces:** Consumes Task 1 API, Task 2 contract/helper, Task 3 observation and lifecycle. Produces registered `BitbucketIntegration` with `Create`, provider-level dry-run preview, and exactly one schema resource token.

- [ ] **Step 1: Write `TestBitbucketCreateRejectsPriorOrInconsistentCandidate`, `TestBitbucketCreateUncertainVerifiedIdentityReturnsPartialWithoutRename`, `TestBitbucketCreateDiscoveryBounds`, and `TestBitbucketCreateRenamePartialState`.** Cover active org/member consistency, full unique snapshot, marker collisions, separate new parent/relation ID domains, wrong owner/org/type, prior IDs, summary/detail mismatch, ambiguity, bodyless acknowledgment, exactly three empty-only attempts, cancellation/deadline, one create only, proven 401/403 rejection/no discovery, uncertain verified identity/original error/no rename, unverified uncertainty/no ID, and rename/readback actual partial state.
- [ ] **Step 1a: Add `TestSchemaBitbucketIntegrationContract`, `TestBitbucketIntegrationPreviewIdentity`, and `TestBitbucketIntegrationInferPartialState`.** Assert exact six inputs with only `name` statically required, two input/output secrets, configuration plus three ordinary ID outputs, no readiness/session/replacement fields, unchanged invokes and additive registration. Exercise the registered provider boundary: computed/nonsecret new IDs, known/nonsecret stable update IDs despite changed/computed inputs, secret propagation, no preview client calls, and actual inferred partial properties plus cancellation/deadline classification after acknowledged mutation.
- [ ] **Step 2: Run RED.** `SAFE go test ./provider -run 'Test(BitbucketCreate|SchemaBitbucket|BitbucketIntegration)' -count=1`; expected missing Create/registration/behavior failure. Keep the failing test and implement; do not mistake intentional RED for a blocker.
- [ ] **Step 3: Implement guarded create.** Adapt GitLab snapshot/discovery safety, separate parent/relation IDs, unpredictable `uuid.NewRandom` marker, authenticated `user.get` ID for required `authId`, exact detail/config/credential-mode verification. Do not include caller chosen IDs. Close body and do not depend on JSON. Search only empty candidate sets, max three attempts; no rename after create error, even if identity verified. For acknowledged create, rename then read back and require confirmed requested name/config. Preserve last verified state on failures.
- [ ] **Step 4: Register, generate and extend maintained SDK contracts.** Add exactly one `infer.Resource(&BitbucketIntegration{client: configuredClient})`; inspect uncommitted status before `SAFE make codegen`. Extend all five existing compile-contract fixtures for both credential modes, optional workspace and ordinary IDs without executing constructors. Preserve GitHub contracts and every prior SDK/resource/invoke.
- [ ] **Step 5: Run GREEN and builds.** `SAFE go test ./provider -run 'Test(BitbucketCreate|SchemaBitbucket|BitbucketIntegration)' -count=1`, `SAFE make test_provider`, `SAFE make test_race`, `SAFE make build_sdks`, `SAFE make test_sdk_contracts`; expected PASS with all five contracts mandatory and fresh local Java SDK setup. Inspect generated diff/counts.
- [ ] **Step 6: Commit explicit create/registration/schema/test/SDK/fixture paths** as `feat: add guarded Bitbucket integration resource`, then run `SAFE make check_codegen` and `SAFE make check_openapi` on committed output. Restore any build-induced generated normalization through canonical generation, committing only intentional canonical changes; expected no drift.

### Task 5: Documentation, examples, full offline verification, and self-review

**Files:**
- Create: `website/src/content/docs/guides/bitbucket-integrations.mdx`.
- Modify: `README.md`, `examples/yaml/{Pulumi.yaml,README.md}`, `examples/yaml_test.go`, `website/astro.config.mjs`, `website/scripts/reference-model.mjs`, `website/scripts/generate-examples.mjs` and relevant existing import/secret/workload guidance.
- Generate: `examples/{go,nodejs,python,dotnet,java}/`, `website/src/content/docs/reference/bitbucket-integration.mdx` and complete-example documentation via canonical generators; preserve all maintained Go helpers and `examples/sdk-contracts/`.
- Test: new `website/tests/bitbucket-integrations.test.mjs`, existing built-site route/inventory tests, `TestCanonicalYAMLBitbucketIntegration`; five-language contract extensions are owned by Task 4.

**Interfaces:** Consume final registered schema and generated SDK surface. Do not alter workload-source contracts.

- [ ] **Step 1: Add `TestCanonicalYAMLBitbucketIntegration` and website source/built assertions.** Check both documented credential modes/conditional identities, Pulumi secret config, relation import ID, omitted-versus-empty workspace, verified mode-switch clear, uncertain-create/partial state, Dokploy-only deletion, unmanaged-workload warning, no OAuth/readiness/connectivity or current legacy-compatibility claim. Canonical YAML selects only API-token mode using reserved placeholders and secret config, never both credentials or plaintext secrets.
- [ ] **Step 2: Run RED.** `SAFE go test ./examples -tags=yaml -run '^TestCanonicalYAMLBitbucketIntegration$' -count=1` and `SAFE node --test website/tests/bitbucket-integrations.test.mjs`; expected missing documentation/example failure.
- [ ] **Step 3: Write docs and examples.** Cover lifecycle/manual boundary, relation import identity, omission vs clear, no OAuth/readiness/connectivity guarantee, no legacy compatibility claim, and parent cascade deletion only. Update workload-source guidance only if it clarifies existing boundaries; no workload variant.
- [ ] **Step 4: Generate and verify the source/docs slice.** Run `SAFE make gen_examples`, the website canonical generation command, and focused source/example tests. Inspect all generated deltas, fixture preservation and `git diff --check`; self-review against the approved lifecycle/credential boundaries. Commit intentional docs, canonical sources, generated translations/references and tests as `docs: document Bitbucket integration` **before** generated drift checks.
- [ ] **Step 5: Run final committed-output verification.** Through SAFE: `make lint`, `make test_provider`, `make test_race`, `make provider`, `go test ./openapi/cmd/normalize ./internal/client/... -count=1`, `make check_openapi`, `make build_sdks`, `make test_examples`, `make check_codegen`, `make docs_check`, full script unit discovery and relevant website tests, then `git diff --check` and final status. SDK/example builds precede canonical drift verification so Java build normalization cannot leave false drift. Keep nested acceptance off and `.env` untouched; do not run live/release/publication. Record gated skips/unavailable checks without implying pass.
- [ ] **Step 6: Inspect final counts/status/diff and report.** Include exact command order/results/logs and any concrete follow-up fixes/covering reruns. Additive inventories must preserve any earlier Gitea/GitHub/GitLab changes. Retain the approved spec, plan and working documents until explicit cleanup approval; no automatic deletion.

## Self-review performed while writing this plan

- **Spec coverage:** API contracts and field presence (Task 1); public six-field input/output schema, validation, computed preview and secret dependencies (Task 2); incomplete imports, secret retention/evidence, exact requests, optional omission/clear, update/read/delete/error handling (Task 3); preview, organization/member, snapshots, discovery, all create/rename uncertainty boundaries and registration (Task 4); all five SDKs, maintained fixtures, canonical secret example, documentation and safe checks (Task 5). No spec behavior is left unowned.
- **Step scan:** Each step is a test, implementation, generation, verification, or review action with exact paths/commands/names. RED and pre-commit drift are explicitly expected; no task requires implementing product behavior before the user approves this plan.
- **Type consistency:** Optional public args use pointers; generated request strings remain non-nullable. State embeds args with three string IDs. Concrete credential evidence, mutation projection/settings verification, snapshot/list/discovery interfaces agree across tasks. Create/update/read use relation ID while delete consumes verified parent ID. No ellipsis-only signatures or pin edits are required.
- **Review focus:** All five highest-risk input/failure cases have named owning tests, with their details covered in their task's test-first step.
- **Proportion:** The five tasks separate API modeling, public contract, lifecycle, guarded create/registration/codegen, and docs/final verification; this is sufficient for reviewer checkpoints without prescribing implementation bodies.
