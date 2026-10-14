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
- Sanitize diagnostics and cancellation safely; never emit secrets, raw upstream prose/bodies, IDs, or private URLs. No live acceptance tests or credentials; preserve existing GitHub/GitLab behavior.
- Generate OpenAPI, generated client, schema, and all five SDKs from canonical sources. Never hand-edit generated output; commit intentional generated changes before drift checks.
- Offline validation must use verified SAFE launcher `safe-run` with live acceptance disabled, protected `.env` untouched. `/tmp/opencode` and the local launcher are execution-only; do not hardcode either into product or generators.
- Baseline at planning: 24 resources, 12 invokes, 147 selected API paths. At implementation start capture actual counts; retain the baseline unless this feature intentionally adds exactly one resource and only the required Bitbucket operations.

## Review Focus

1. A computed credential must not mask a known invalid supplied email or known mode error — Task 2: `TestBitbucketCheckValidatesKnownFieldsIndependently`.
2. Refresh-preserved prior credentials must not fake successful rotation/clear verification — Task 3: `TestBitbucketUpdateRequiresFreshCredentialEvidence`.
3. A preexisting marker collision or candidate with reused/mismatched IDs must never be adopted — Task 4: `TestBitbucketCreateRejectsPriorOrInconsistentCandidate`.
4. A bodyless create followed by uncertain error must return only verified partial identity and never rename — Task 4: `TestBitbucketCreateUncertainVerifiedIdentityReturnsPartialWithoutRename`.
5. Generic `BAD_REQUEST` and unsafe response text must not be treated as not-found or leak secrets — Task 3: `TestBitbucketDeleteNotFoundClassificationAndSafeDiagnostics`.

---

## File map and task interfaces

Canonical API source: `openapi/operations.txt`, `openapi/source.json`; generated API contract/client under `openapi/` and `internal/client/generated/`. Provider resource files use focused `provider/bitbucket_integration*.go` names; registration is in `provider/provider.go`, schema in `provider/provider.go` annotations/inference. Docs/examples and maintained SDK contract fixtures are outside generated SDK directories. Any generated SDK files are codegen outputs only.

Interfaces passed forward:

- Task 1 provides generated typed operations for `BitbucketCreate`, `BitbucketOne`, `BitbucketUpdate`, `GitProviderGetAllWithResponse`, `GitProviderRemove`, `UserGetWithResponse`, and active organization, with explicit nullable/presence shapes from the spec; bodyless mutation responses are consumed as raw HTTP responses.
- Task 2 defines `BitbucketIntegrationArgs`, `BitbucketIntegrationState`, `validateBitbucketIntegrationArgs(args BitbucketIntegrationArgs) error`, and `bitbucketIntegrationPreviewState(args BitbucketIntegrationArgs, id string, prior BitbucketIntegrationState) BitbucketIntegrationState`. Optional identity/workspace arguments use pointers to distinguish omitted from explicit empty.
- Task 3 defines `bitbucketObservation{State BitbucketIntegrationState, OwnerUserID string}` plus credential-presence evidence separate from projected state; `readBitbucketIntegration(ctx, api, relationID string, prior BitbucketIntegrationState, organizationID string) (bitbucketObservation, error)`, and safe `bitbucketMutation(response *http.Response, err error, operation string) error`.
- Task 4 consumes Tasks 1–3 and defines the resource `BitbucketIntegration` methods and `discoverBitbucketIntegration(...)`/snapshot helpers following GitLab's separate parent/relation ID domains. Registration enables provider-level preview tests here.

### Task 1: Minimal pinned API surface and accurate response presence

**Files:**
- Modify: `openapi/operations.txt`, adding only `bitbucket.create`, `bitbucket.one`, `bitbucket.update`, `gitProvider.getAll`, `gitProvider.remove`, `user.get`, and existing active organization operation.
- Modify canonical OpenAPI source as needed: `openapi/source.json` and operation source under `openapi/`.
- Generate: `openapi/` output and `internal/client/generated/` (never hand-edit).
- Test: API generation/client contract tests under `openapi/` and `internal/client/` (add focused tests if generated operation testing has no presence/bodyless fixture).

**Interfaces:** Consumes pinned evidence in `.superpowers/sdd/2026-10-04-gitlab-integration/followup-api-research.md` and `followup-api-verification.md`; produces Task 1 API interfaces above.

- [ ] **Step 1: Capture starting surface and write failing contract tests.** Record actual resource/invoke/selected-operation counts; add tests for response field specified/null/omitted distinctions, list parent identity plus nullable Bitbucket summary, nested detail parent, and bodyless mutation acknowledgment.
- [ ] **Step 2: Verify RED.** Run `go test ./internal/client/... ./openapi/...`; expected new tests fail on missing/mis-shaped operations or response presence. A generator drift failure before deliberate generation is expected, not a blocker.
- [ ] **Step 3: Update canonical operation selection and source models.** Model authenticated user; list parent (`gitProviderId`, name/type/org/user, sharing metadata) plus nullable Bitbucket summary (`bitbucketId`, username, configured/deprecated); detail relation/parent/username/email/workspace/both credentials/nested parent; create/update requests as non-null strings. Preserve credential response-only models internally. Keep mutation decoding independent of response JSON.
- [ ] **Step 4: Regenerate and verify API contract.** Run `make check_openapi`; expected generated client exposes typed operations and presence-sensitive response values, while bodyless successful mutations are accepted and bodies closed.
- [ ] **Step 5: Commit this reviewable slice.** `git add openapi/operations.txt openapi/source.json openapi internal/client/generated && git commit -m 'feat: model Bitbucket integration API'` (include only canonical and generated files actually changed).

### Task 2: Public inputs, validation, diff, preview helper, and secrets

**Files:**
- Create: `provider/bitbucket_integration.go` (args/state, annotations, validation, diff, secret/dependency annotations, preview helper).
- Test: `provider/bitbucket_integration_test.go`, `provider/bitbucket_integration_preview_test.go` (pure helper tests only until registration in Task 4).
- Reference: `provider/gitlab_integration.go`, `provider/github_integration_preview_test.go`, existing secret/optional pointer patterns.

**Interfaces:** Produces `BitbucketIntegrationArgs` fields `Name string`, `APIToken *string`, `AppPassword *string`, `BitbucketEmail *string`, `BitbucketUsername *string`, `BitbucketWorkspaceName *string`, pulumi tags matching camel-case names, and `BitbucketIntegrationState` embedding args plus `BitbucketID`, `GitProviderID`, `OrganizationID`.

- [ ] **Step 1: Write failing tests.** Cover each valid mode; neither/both/empty selected credential; invalid/missing conditional identity; invalid supplied inactive email; known bad email despite unknown credential; unknown preview retention; explicit-empty workspace vs omission; secret metadata/dependencies and ordinary identity outputs. Name tests `TestBitbucketCheck...`, `TestValidateBitbucketIntegrationArgs...`, `TestBitbucketPreviewState...`, `TestBitbucketSecretAnnotations`.
- [ ] **Step 2: Run RED.** `go test ./provider -run 'Test(BitbucketCheck|ValidateBitbucketIntegrationArgs|BitbucketPreviewState|BitbucketSecretAnnotations)' -count=1`; expected compile/test failures before implementation.
- [ ] **Step 3: Implement exact contract.** `Check` independently validates all known fields without treating unknowns as empty; real-apply validator requires exactly one nonempty concrete credential and valid conditional identity. Validate any supplied email. Diff only meaningful desired differences; omitted optional inactive values preserve observed values and do not perpetually diff. Helper preserves prior values for omitted optional args, preserves identity IDs and leaves computed outputs alone on create preview.
- [ ] **Step 4: Run GREEN.** Repeat focused command; verify secret args depend into their own outputs, while IDs remain nonsecret.
- [ ] **Step 5: Commit.** `git add provider/bitbucket_integration.go provider/bitbucket_integration_test.go provider/bitbucket_integration_preview_test.go && git commit -m 'feat: define Bitbucket integration contract'`.

### Task 3: Whitelisted observations, read/import, update, and delete

**Files:**
- Create: `provider/bitbucket_integration_observation.go`, `provider/bitbucket_integration_lifecycle.go`.
- Test: `provider/bitbucket_integration_observation_test.go`, `provider/bitbucket_integration_lifecycle_test.go` using scripted HTTP responses; keep fixtures sanitized.
- Reference: `provider/gitlab_integration_observation.go`, `provider/gitlab_integration_lifecycle.go`, GitHub lifecycle and tests.

**Interfaces:** Consumes Task 1 client and Task 2 args/state/validator. Produces observation/evidence and lifecycle method contracts stated in the interface map.

- [ ] **Step 1: Write failing tests.** Cover valid and incomplete/both-credential imports; relation-ID identity and parent consistency; credential null/omission preservation on refresh; no credential leakage in errors; update requests send no null, empty unused credential and supplied optional fields only; omitted workspace preserves while empty clears; inactive identity omission no diff; fresh active credential match and explicit inactive-clear evidence required; readback actual partial state after split update failure; verified parent-only delete/cascade; structured not-found only. Test names include `TestReadBitbucketIntegration...`, `TestBitbucketUpdate...`, `TestBitbucketDelete...`.
- [ ] **Step 2: Run RED.** `go test ./provider -run 'Test(ReadBitbucketIntegration|BitbucketUpdate|BitbucketDelete)' -count=1`; expected missing implementation/test failures.
- [ ] **Step 3: Implement observation.** Verify relation ID, parent ID/nested parent, type, org and stable prior identity; normalize null/empty credentials as absent; whitelist outputs. Preserve prior secret only on refresh omission/null while retaining field presence/clear evidence separately. Sanitize status/classification and preserve cancellation.
- [ ] **Step 4: Implement lifecycle methods.** Read by relation ID; only genuine structured not-found removes. Update pre-reads verified identities, sends full credentials with unused empty string and only specified optionals, closes raw body, never retries, reads back when context permits, compares newly active credential against fresh response and requires explicit null/empty evidence that unused credential cleared. Return verified actual/last-observed partial state on later error. Delete pre-verifies relation/parent/org/stable identity, removes only parent, then confirms absence; never interpret generic bad request as not-found.
- [ ] **Step 5: Run GREEN.** Repeat focused test command. Add `TestBitbucketUpdateRequiresFreshCredentialEvidence` and `TestBitbucketDeleteNotFoundClassificationAndSafeDiagnostics` assertions from Review Focus.
- [ ] **Step 6: Commit.** `git add provider/bitbucket_integration_observation.go provider/bitbucket_integration_lifecycle.go provider/bitbucket_integration_observation_test.go provider/bitbucket_integration_lifecycle_test.go && git commit -m 'feat: manage Bitbucket integration lifecycle'`.

### Task 4: Guarded create, registration, schema, and generated SDKs

**Files:**
- Create: `provider/bitbucket_integration_create.go`, `provider/bitbucket_integration_create_test.go`.
- Modify: `provider/provider.go` (register one resource); generated schema and SDK directories via canonical generation.
- Test: create tests plus `provider/provider_test.go` or the existing provider schema/registration test location; generated SDK contract tests in `examples/sdk-contracts/{go,nodejs,python,dotnet,java}`.
- Reference: `provider/gitlab_integration_create.go`, `provider/gitlab_integration_create_test.go`, `provider/provider.go`.

**Interfaces:** Consumes Task 1 API, Task 2 contract/helper, Task 3 observation and lifecycle. Produces registered `BitbucketIntegration` with `Create`, provider-level dry-run preview, and exactly one schema resource token.

- [ ] **Step 1: Write failing create/registration tests.** Cover no calls on preview; active org and member consistency; full snapshot validates unique nonempty parent/relation IDs; marker collision; new parent/relation only; wrong owner/org/type, prior IDs, summary/detail mismatch, ambiguity, bodyless acknowledgment, no-candidate-only maximum three discovery attempts, cancellation, no create retry, proven auth/permission rejection short circuit, uncertain create with verified identity returns partial actual state/original error without rename, unverified uncertainty no guessed ID, rename/readback success/failure partial state. Register `TestBitbucketCreate...` plus provider schema presence and all six fields/secret annotations/outputs; provider-level preview test only now.
- [ ] **Step 2: Run RED.** `go test ./provider -run 'Test(BitbucketCreate|Provider.*Bitbucket|Bitbucket.*Preview)' -count=1`; expected missing registration/behavior fails. API path/count drift before generation/commit is expected.
- [ ] **Step 3: Implement guarded create.** Adapt GitLab snapshot/discovery safety, separate parent/relation IDs, unpredictable `uuid.NewRandom` marker, authenticated `user.get` ID for required `authId`, exact detail/config/credential-mode verification. Do not include caller chosen IDs. Close body and do not depend on JSON. Search only empty candidate sets, max three attempts; no rename after create error, even if identity verified. For acknowledged create, rename then read back and require confirmed requested name/config. Preserve last verified state on failures.
- [ ] **Step 4: Register and generate.** Add exactly one `infer.Resource(&BitbucketIntegration{client: configuredClient})`; run repository canonical codegen (`make codegen`) only after confirming no uncommitted generated work, inspect every generated delta. Commit intended generated outputs before drift checks, with baseline resource/invoke/path-count comparison.
- [ ] **Step 5: Run GREEN and schema/SDK contract checks.** `go test ./provider -run 'Test(BitbucketCreate|Provider.*Bitbucket|Bitbucket.*Preview)' -count=1`; then `make check_codegen` and `make build_sdks`. Expected provider schema and all five SDKs expose secret args, correct relation ID identity and nonsecret identity outputs.
- [ ] **Step 6: Commit.** `git add provider/bitbucket_integration_create.go provider/bitbucket_integration_create_test.go provider/provider.go provider/provider_test.go sdk && git commit -m 'feat: add guarded Bitbucket integration resource'` (adjust test path and stage only actual intended generated outputs).

### Task 5: Documentation, examples, full offline verification, and self-review

**Files:**
- Modify: `README.md`, `docs/` resource and provider docs, `website/` canonical docs as appropriate, provider metadata, and one canonical example per maintained language under `examples/{yaml,nodejs,python,go,dotnet,java}` as repository patterns require.
- Modify: `examples/sdk-contracts/{go,nodejs,python,dotnet,java}` outside generated directories.
- Test: `provider/bitbucket_integration_docs_test.go`, example/SDK contract tests as appropriate.

**Interfaces:** Consume final registered schema and generated SDK surface. Do not alter workload-source contracts.

- [ ] **Step 1: Write failing docs/example contract tests.** Assert docs cover API-token and legacy app-password modes, conditional identities, secret Pulumi config reference, import ID, omission vs explicit-empty workspace, mode-switch verified clear, uncertain-create/partial state boundaries, delete scope, manual authorization limits, and no claim of current Bitbucket compatibility. Assert examples use reserved placeholders and canonical secret configuration, selecting API-token mode only; no plaintext credential literals. Add sanitized tests for diagnostics with secret-like upstream bodies.
- [ ] **Step 2: Run RED.** Run focused docs and example-contract tests; expected missing docs/example references fail.
- [ ] **Step 3: Write docs and examples.** Cover lifecycle/manual boundary, relation import identity, omission vs clear, no OAuth/readiness/connectivity guarantee, no legacy compatibility claim, and parent cascade deletion only. Update workload-source guidance only if it clarifies existing boundaries; no workload variant.
- [ ] **Step 4: Run GREEN and full safe verification.** Through the verified local SAFE launcher (not hardcoded): `make lint`, `make test_provider`, `make test_race`, `make provider`, `make check_openapi`, `make check_codegen`, `make build_sdks`, `make test_examples`, `make docs_check`. Keep live acceptance forcibly disabled for nested `mise exec`. Run `git diff --check`; inspect `git status --short`, resource/invoke/API counts and final relevant diff. Do not run live acceptance, release, or publication workflows. Report any unavailable checks explicitly.
- [ ] **Step 5: Self-review against spec.** Confirm all lifecycle/error/presence semantics and five review-focus tests, no unrelated source contracts or generated edits, generated outputs derive from canonical sources, and secret/private material absent. Retain the approved spec and this plan unless separately approved for cleanup.
- [ ] **Step 6: Commit docs and examples.** Stage only intended documentation, canonical examples, maintained contract fixtures, and tests; commit `docs: document Bitbucket integration`.

## Self-review performed while writing this plan

- **Spec coverage:** API contracts and field presence (Task 1); public six-field input/output schema, validation, computed preview and secret dependencies (Task 2); incomplete imports, secret retention/evidence, exact requests, optional omission/clear, update/read/delete/error handling (Task 3); preview, organization/member, snapshots, discovery, all create/rename uncertainty boundaries and registration (Task 4); all five SDKs, maintained fixtures, canonical secret example, documentation and safe checks (Task 5). No spec behavior is left unowned.
- **Step scan:** Each step is a test, implementation, generation, verification, or review action with exact paths/commands/names. RED and pre-commit drift are explicitly expected; no task requires implementing product behavior before the user approves this plan.
- **Type consistency:** Optional API inputs are pointers throughout; state embeds args and adds the relation ID and two ordinary identity outputs. Observation returns evidence independently from state. Create/update/read use relation ID while delete consumes verified parent ID. Generated operations and task interfaces align with pinned operation scope.
- **Review focus:** All five highest-risk input/failure cases have named owning tests, with their details covered in their task's test-first step.
- **Proportion:** The five tasks separate API modeling, public contract, lifecycle, guarded create/registration/codegen, and docs/final verification; this is sufficient for reviewer checkpoints without prescribing implementation bodies.
