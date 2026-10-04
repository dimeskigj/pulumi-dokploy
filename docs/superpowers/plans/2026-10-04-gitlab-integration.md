# GitLabIntegration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Manage Dokploy GitLab integration configuration and lifecycle through Pulumi, with manual OAuth authorization and usable Application/Compose references.

**Architecture:** Add one inferred resource backed by the existing authenticated generated client. Separate public validation, verified observations, CRUD, and guarded create discovery; never project OAuth tokens into state. Generate OpenAPI output, schema, all five SDKs, language examples, and website references from their sources.

**Tech Stack:** Go, pulumi-go-provider/infer, oapi-codegen, existing google/uuid and nullable packages, Pulumi CLI, Go/Node.js/Python/.NET/Java SDKs, Astro/Starlight, Node test runner.

**Spec:** `docs/superpowers/specs/2026-10-04-gitlab-integration-design.md` (approved).

## Global Constraints

- Token `dokploy:index:GitLabIntegration`; Pulumi/import identity is `gitlabId`, never `gitProviderId`.
- Required inputs: `name`, `applicationId`, `applicationSecret`, `redirectUri`. Optional `gitlabUrl` defaults to `https://gitlab.com`; `groupName` defaults to an empty string; absent `gitlabInternalUrl` means null/no override.
- Secret input/output properties: `applicationSecret`, `gitlabInternalUrl`. Identity outputs remain ordinary strings.
- Outputs additionally include `gitProviderId`, `organizationId`, `isConfigured`. isConfigured observes nonempty access/refresh tokens, not connectivity or authorization validity.
- Configuration changes update in place. No sharing, token management, automatic OAuth, connection testing, mutation retries, or failure cleanup.
- Creation uses one unpredictable temporary name and verified new parent/relation identities. No adoption by requested name, list position, or incomplete credentials.
- Preserve only verified identities and observed settings as partial state. Generic BAD_REQUEST is not proof of absence or failed creation.
- Diagnostics retain safe operation/status/context classification only; no arbitrary server prose, tokens, credentials, IDs, markers, private URLs, or response bodies.
- Preserve external `source.gitlab.integrationId` support and all unrelated resources/functions. Workload lifecycle is unchanged.
- Preserve OpenAPI pin `cebd3808565ea9bed0791961bc25c60513d94c5a` and hash. No new dependencies; regenerate rather than hand-edit output.
- Use `.mise.toml` tool versions, including Go `1.26.6`, Pulumi `3.259.0`, Node `24.18.0`, oapi-codegen `v2.8.0`, and existing Java 11/17 selectors.
- No live acceptance, release, publication, push, or merge commands. All tests must have live opt-in disabled and credentials absent, including nested Makefile commands.
- Preserve pre-existing README and website guide edits. Stage explicit task paths, never `git add .`.
- At implementation completion, ask whether to retain/delete this task's spec/plan. Remove only approved related documents.

## Review Focus

- Computed integration IDs must work in Application/Compose previews without hiding a known invalid branch/project/build setting; Task 5 tests both.
- A new incomplete OAuth integration must be discoverable even though `gitlabProviders` filters it out; Tasks 1 and 4 test getAll, not that filtered endpoint.
- Tokens or private basic-auth URLs in malformed responses/error messages must not leak on fresh import with no prior secrets; Task 3 tests safe diagnostics.
- Concurrent same-name creation or inconsistent summary/detail identity must never lead to adopting/deleting the wrong provider; Task 4 tests independent markers and disagreement.
- Credential rotation with a token-bearing response must not claim token validity or persist tokens, while null/omitted credentials cannot silently erase known secrets; Tasks 3 and 6 pin state and documentation.

---

## File map and boundaries

| Files | Responsibility |
| --- | --- |
| `openapi/operations.txt`, `openapi/corrections.json`, `openapi/cmd/normalize/main_test.go` | Operation selection and typed source corrections/tests |
| `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`, `internal/client/gitlab_contract_test.go` | Generated API contract and offline client checks |
| `provider/gitlab_integration.go`, `provider/gitlab_integration_test.go` | Public types, descriptions, defaults, Check/Diff/dependencies |
| `provider/gitlab_integration_observation.go`, `provider/gitlab_integration_observation_test.go` | Identity/settings projection, discovery-list validation, safe diagnostics |
| `provider/gitlab_integration_lifecycle.go`, `provider/gitlab_integration_lifecycle_test.go` | Read/import/update/delete using verified observations |
| `provider/gitlab_integration_create.go`, `provider/gitlab_integration_create_test.go`, `provider/gitlab_integration_helpers_test.go` | Guarded creation and dynamic marker fixtures |
| `provider/provider.go`, `provider/provider_test.go`, `provider/schema_test.go`, `provider/preview_dependencies_test.go` | Registration, metadata, schema, provider preview tests |
| `provider/cmd/pulumi-resource-dokploy/schema.json`, `sdk/{go,nodejs,python,dotnet,java}/` | Generated public surface |
| `provider/application.go`, `provider/application_source.go`, `provider/application_source_test.go`, `provider/compose.go`, `provider/compose_source.go`, `provider/compose_source_test.go` | Narrow computed integration-reference validation and regressions |
| `provider/gitlab_integration_reference.go` | Optional shared helper recognizing computed workload integration IDs, only if the regression requires it |
| `examples/yaml/Pulumi.yaml`, `examples/yaml/README.md`, `examples/yaml_test.go`, `examples/{go,nodejs,python,dotnet,java}/` | Canonical configuration example and generated language equivalents |
| `README.md`, `website/src/content/docs/guides/{gitlab-integrations,applications,compose,imports,troubleshooting}.mdx`, `website/src/content/docs/concepts/{sources,secrets,lifecycle-and-state}.mdx` | User-facing usage, authorization boundary, secret/import/recovery guidance |
| `website/scripts/reference-model.mjs`, `website/astro.config.mjs`, `website/tests/{reference-model,content}.test.mjs`, `website/tests/site-output.built-test.mjs` | Inventory, routes, source/built documentation regressions |
| `website/src/content/docs/reference/git-lab-integration.mdx`, `website/src/content/docs/reference/types.mdx`, `website/src/content/docs/examples/complete.mdx` | Generated references/examples; use the existing slug algorithm |

## Execution safety prerequisite

- [ ] Read the spec, this plan, AGENTS.md, CONTRIBUTING.md, and tests/README.md; inspect status and establish isolation with using-git-worktrees at execution time. Keep user changes in the original workspace; do not copy, stage, reset, or stash them automatically. Document overlapping guide changes for later integration.
- [ ] Create an execution-only directory under `/tmp/opencode/gitlab-integration-tools-<unique>` containing a standalone `mise.toml` copied from only the repository's `[tools]` values, plus fixed `[env]` values `DOKPLOY_ACCEPTANCE="0"`, `DOKPLOY_ENDPOINT=""`, `DOKPLOY_API_KEY=""`. Never copy `[env] _.file`; never read `.env`. Resolve the real mise executable before adding any wrapper to PATH. If mise requires trust, trust only that newly authored temporary configuration.
- [ ] Implement a local `safe-run <command> [args...]` launcher and a local `mise` wrapper in that temporary directory. Both use the real mise executable with `--cd <temporary-tools-directory> exec`, not the repository configuration. The wrapper accepts `exec [tool@version ...] -- <command>`, preserves explicit Java/Pulumi selectors, captures its caller's working directory, and runs a small Python trampoline that changes back to that directory only after tool/environment selection. Reject unsupported wrapper invocations rather than falling through to project mise. Put the wrapper first in the final child PATH so nested Makefile invocations cannot re-load the project `.mise.toml`.
- [ ] Launch tooling from a sanitized environment: retain HOME and ordinary system/tool paths, remove inherited `DOKPLOY_*`, `MISE_*` activation/configuration overrides and credential variables, and set the three fixed safe values above. The temporary config contains no secret-file loading. Keep the child command's original worktree working directory. Use this launcher for installations and every Go/Pulumi/Node/Make command below; **SAFE** abbreviates this verified launcher.
- [ ] Verify the launcher's actual environment without printing it: use a Python assertion that `DOKPLOY_ACCEPTANCE == "0"` and no nonempty `DOKPLOY_*` variable exists except that gate. Run the same assertion through a nested `mise exec -- python3`, through an explicit `mise exec pulumi@3.259.0 -- python3`, and with the working directory changed to `sdk/go/dokploy`. Assert subprocess working directories remain unchanged. Expected: exit 0 with no values printed. Stop if any check fails.
- [ ] Verify safe Go/Pulumi/Node tool versions against `.mise.toml` and install missing tools from the temporary configuration only. Do not stage the temporary helpers. The previously planned project-local mise overlay is not used: it can still load `.env` before overriding values.

### Task 1: Generate the GitLab and parent-provider API contract

**Files:** OpenAPI source/tests and generated outputs from the file map; new `internal/client/gitlab_contract_test.go`.

**Interfaces:** Consumes `normalizeRealContract`, `operationRequestSchema`, `componentSchema`, and `responseSchema`. Produces generated raw methods `GitlabCreate`, `GitlabUpdate`, `GitProviderRemove`, and WithResponse GET methods `GitlabOneWithResponse`, `GitProviderGetAllWithResponse`, `UserGetWithResponse`; existing OrganizationActive stays unchanged. Request types retain upstream fields, especially required `authId`, `gitlabId`, `gitProviderId`, and nullable internal URL.

Use correction model names `GitLabProviderIdentity`, `GitLabProviderSummary`, `GitProviderListEntry`, `GitProviderList`, `GitLabIntegrationRecord`, `GitProviderUserIdentity`, and `AuthenticatedGitProviderMember`. Identity models use required nonempty-at-runtime strings. GitLab record configuration and token fields use nullable strings preserving omission versus null. Its required response identity is `gitlabId`, `gitProviderId`, `gitlabUrl`, `gitProvider`; token presence is explicitly validated at the observation boundary, not assumed from Go zero values. Member fields are `userId`, `organizationId`, and `user.id`. Summary contains `gitlabId`, nullable `applicationId`, `gitlabUrl`, `isConfigured`; list entry contains parent identity and nullable GitLab summary. Ignore extra upstream fields with `additionalProperties:true`.

- [ ] Write `TestGitLabIntegrationOpenAPIContract`: assert GET one/getAll/user.get, POST create/update/remove, exactly these six added operations, corrected response references/fields, nullable settings/tokens, bodyless mutation acknowledgments, and no selected gitlabProviders/testConnection/callback.

```go
d := normalizeRealContract(t)
require.Contains(t, d.Paths, "/gitProvider.getAll")
require.NotContains(t, d.Paths, "/gitlab.gitlabProviders")
require.Equal(t, "#/components/schemas/GitLabIntegrationRecord", responseSchema(t, d, "/gitlab.one", "get", "200").Ref)
require.Contains(t, operationRequestSchema(t, d, "gitlab.create")["required"], "authId")
require.NotContains(t, operationRequestSchema(t, d, "gitlab.update")["properties"], "accessToken")
```

- [ ] Run **SAFE** `go test ./openapi/cmd/normalize -run TestGitLabIntegrationOpenAPIContract -count=1`. Expected: missing-operation/correction assertion failure.
- [ ] Add the six operations and response corrections, including empty response correction strings for all three mutations. Inspect status before **SAFE** `make generate_openapi`; generate the typed contract/client without changing upstream source/hash.
- [ ] Write `TestGitLabGeneratedClientContract`: use httptest fixtures to assert `secret` mapping, authId, both update IDs, explicit internal URL null, empty group string, properly encoded opaque GET ID, member nested ID, incomplete getAll integration, ignored PII/other-provider settings, and token omission/null distinction. First run its tests; any decoding gap must fail before changing corrections.
- [ ] Correct only the source model if necessary; rerun **SAFE** `go test ./openapi/cmd/normalize ./internal/client/... -count=1`. Expected: pass. Review generated request/response types for the Interfaces above.
- [ ] Run `git diff --check`, stage the six task source/test/generated paths explicitly, and commit `feat: generate GitLab integration API client`. Run **SAFE** `make check_openapi`; expected: no drift.

### Task 2: Define the public resource contract and checks

**Files:** Create `provider/gitlab_integration.go`, `provider/gitlab_integration_test.go`. No resource registration yet.

**Interfaces:** Produces `GitLabIntegrationArgs` with `Name`, `ApplicationID`, `ApplicationSecret`, `RedirectURI`, `GitlabURL`, `GroupName` strings and `GitlabInternalURL *string`; exact Pulumi tags are the spec's seven inputs, with optional URL/group/internal URL. Produces `GitLabIntegrationState{GitLabIntegrationArgs; GitlabID, GitProviderID, OrganizationID string; IsConfigured bool}`, and `GitLabIntegration{client clientFactory}`. Produces `validateGitLabIntegrationArgs(GitLabIntegrationArgs) error`, annotated types/resource, Check, Diff, and WireDependencies using the repository's existing infer signatures.

- [ ] Write `TestGitLabIntegrationCheckDefaults`, `TestGitLabIntegrationCheckKnownAndComputed`, `TestGitLabIntegrationURLValidation`, and `TestGitLabIntegrationDiff`. Assert required fields nonempty; omission defaults; explicit null/empty public URL fails; absent internal URL is nil but explicit empty fails; HTTP(S) host/path rules; public URL/callback reject userinfo; internal URL permits basic auth; no value appears in validation messages. A computed secret must not hide a known invalid URL/name.

```go
require.Equal(t, "https://gitlab.com", checked.Inputs.GitlabURL)
require.Equal(t, "", checked.Inputs.GroupName)
require.Nil(t, checked.Inputs.GitlabInternalURL)
for _, diff := range changes.DetailedDiff {
    require.Equal(t, p.Update, diff.Kind)
}
```

- [ ] Run **SAFE** `go test -short ./provider -run 'TestGitLabIntegration(Check|URL|Diff)' -count=1`. Expected: compile failure because resource types are absent.
- [ ] Implement public types/annotations, omission-only defaults, known-field Check validation, concrete mutation validation, and seven-field in-place Diff. Describe manual OAuth and readiness limitations on the resource/output. Use `provider:"secret"` for client secret and internal URL; do not add replace-on-change flags. Keep groupName empty-string canonicalization stable.
- [ ] Implement WireDependencies without transitively making identity outputs secret. Preserve existing ID outputs during update previews; Task 4's provider-level tests decide framework behavior. Describe every public input/output; no token/authId/sharing fields.
- [ ] Run targeted tests and **SAFE** `make test_provider`. Expected: pass. Review/stage the two files and commit `feat: define GitLabIntegration resource contract`.

### Task 3: Implement verified observations and read/update/delete

**Files:** Observation/lifecycle source/tests from the file map.

**Interfaces:** Consumes Tasks 1/2. Produces:

- `gitLabObservation` with `State GitLabIntegrationState`, `OwnerUserID string`, `ConfigurationComplete bool`; never return tokens in it.
- `readGitLabIntegration(ctx context.Context, api *client.Client, id string, prior GitLabIntegrationState, organizationID string) (gitLabObservation, error)`.
- `readGitLabProviderList(ctx context.Context, api *client.Client, organizationID string) ([]generated.GitProviderListEntry, error)` validating common/list/relation identities.
- `sanitizeGitLabIntegrationError(err error) error`, keeping context cancellation/deadline identity and API HTTP status only.
- Read/Update/Delete methods on `GitLabIntegration`, with `infer.ReadRequest[GitLabIntegrationArgs, GitLabIntegrationState]`, `infer.UpdateRequest[GitLabIntegrationArgs, GitLabIntegrationState]`, and `infer.DeleteRequest[GitLabIntegrationState]`, returning their corresponding infer response types.

- [ ] Write `TestGitLabIntegrationReadAndImport`, `TestGitLabIntegrationObservationValidation`, and `TestGitLabIntegrationDiagnostics`: correct relation/parent/type/active-org; established identity mismatch fails; member/list common fields are complete and unique; missing/null token fields handled distinctly (missing is malformed, null is unconfigured); group null becomes empty; internal null becomes nil; absent/null secret preserves prior; ID-only incomplete-config import succeeds without invented OAuth values. Assert marshaled inputs/state contain neither token keys nor sentinel token values.

```go
require.Equal(t, requestedID, observed.State.GitlabID)
require.Equal(t, prior.ApplicationSecret, observed.State.ApplicationSecret)
require.False(t, observed.State.IsConfigured) // explicit null tokens
encoded, err := json.Marshal(observed.State)
require.NoError(t, err)
require.NotContains(t, string(encoded), "access-token-sentinel")
require.NotContains(t, string(encoded), "refresh-token-sentinel")
```
- [ ] Run **SAFE** `go test -short ./provider -run 'TestGitLabIntegration(Read|Observation|Diagnostics)' -count=1`. Expected: missing helper/method failure.
- [ ] Implement observation/list helpers and safe diagnostics. Validate returned ID equality, nested parent equality, GitLab type, nonempty organization/user, and active/prior organization. Require explicit token presence and derive IsConfigured locally, then discard tokens. ConfigurationComplete requires observable matching client ID/secret/redirect plus specified nullable group/internal fields for create verification. Do not forward typed-decoder errors or response data.
- [ ] Implement Read: obtain active organization before reading, return sanitized errors, reconstruct observed configuration, and clear identity only on original structured NOT_FOUND/HTTP 404. Import missing OAuth config is readable but needs resupply before mutation.
- [ ] Write `TestGitLabIntegrationUpdateVerified`, `TestGitLabIntegrationUpdatePartialState`, and `TestGitLabIntegrationDeleteVerified`: update pre-read, exact full request, explicit optional clears, post-read identity/settings check; non-atomic parent-name change survives in observed failure state; secrets/tokens never appear in errors; delete pre-read then verified parent remove; absent pre-read skips remove; remove BAD_REQUEST plus confirmed absence succeeds, but forbidden/transient/ambiguous presence does not.
- [ ] Run **SAFE** `go test -short ./provider -run 'TestGitLabIntegration(Update|Delete)' -count=1`. Expected: absent CRUD methods/assertion failures.
- [ ] Implement Update/Delete with raw mutation acknowledgment/closed body and no retries. Pre-read preserves actual state on rejection; safe failure readback updates it only after identity validation. On acknowledged-but-unobservable update, return last verified state and error, not desired settings. Compare observed settings to desired, preserving prior observed secret when omitted. If a rotated secret cannot be observed, report an unconfirmed-update partial failure rather than replacing last verified state with desired secret; omission is not proof of rotation. Never infer token validity from IsConfigured. Delete treats only verified absence as idempotent success.
- [ ] Add `TestGitLabIntegrationMutationBodyIgnored`, `TestGitLabIntegrationCancellation`, and fresh-import diagnostics cases: malformed success acknowledgment/body-close error does not undo mutation acknowledgment; original canceled/deadline errors retain classification; unexpected token-bearing error prose and malformed responses leak nothing. Use one-attempt GET policies for deliberately transient fixtures. Run the full Task 3 command and **SAFE** `make test_provider`; expected: pass.
- [ ] Inspect exact endpoint sequences, run `git diff --check`, stage the four new source/test files, and commit `feat: verify GitLab integration reads and mutations`.

### Task 4: Implement guarded create, register, and generate SDKs

**Files:** Create source/tests/helpers, registration/schema/preview files, generated schema and SDKs from the file map.

**Interfaces:** Consumes Task 3 observations/list validation. Produces Create on `GitLabIntegration`:
`Create(context.Context, infer.CreateRequest[GitLabIntegrationArgs]) (infer.CreateResponse[GitLabIntegrationState], error)`.
Produces `discoverGitLabIntegration(ctx context.Context, api *client.Client, args GitLabIntegrationArgs, marker, organizationID, userID string, priorIDs map[string]struct{}) (gitLabObservation, error)`, at most three list attempts with context-aware 100 ms spacing only after zero candidates. Register the completed resource; no cross-resource abstraction or global mutable test hook.

- [ ] Write `TestGitLabIntegrationCreateVerified`: strict sequence active-org, member, snapshot, create marker, getAll, one, rename/update, one. The create fixture captures the actual unpredictable marker, verifies it differs from requested name and was absent before creation, and emits the captured marker in subsequent responses. Assert one create, both final IDs, requested final name, IsConfigured false, and no token/testConnection/deploy requests.
- [ ] Write `TestGitLabIntegrationCreateDiscoveryFailures`: duplicate/empty prior IDs, wrong org/user/type, pre-existing candidate ID, two markers, detail/summary mismatch, missing secret/settings, malformed list, ID disappearance, empty candidates bounded to three attempts, and cancellation during spacing. None returns an unverified ID or makes rename/delete calls.
- [ ] Write `TestGitLabIntegrationCreatePartialFailures`: generic 400 after commit and transport/server uncertainty recover a fully verified ID but return ResourceInitFailed without rename; authentication/permission rejection avoids discovery; acknowledged creation with rename failure/readback failure preserves last verified IDs/settings. Test malformed/empty creation bodies and Close errors as acknowledged, not identity loss. Test simultaneous same requested display name with distinct markers never cross-adopts.

```go
var partial infer.ResourceInitFailedError
require.ErrorAs(t, err, &partial)
require.Equal(t, "gitlab-fixture", result.ID)
require.Equal(t, "parent-fixture", result.Output.GitProviderID)
require.Equal(t, verifiedName, result.Output.Name)
```

- [ ] Run **SAFE** `go test -short ./provider -run TestGitLabIntegrationCreate -count=1`. Expected: absent Create/discovery assertion failure.
- [ ] Implement member/org consistency and snapshot validation, one UUID-marker POST, bounded discovery, full detail/config/owner verification, and guarded rename. Close success bodies without decoding. Classify generic BAD_REQUEST as uncertain; do not retry/cleanup. After owned identity is verified, later errors return initFailed with verified state. Final readback confirms requested name/configuration as well as stable identity. A failed rename gets a safe identity-checked readback when context permits, so state does not manufacture requested settings.
- [ ] Run create and existing lifecycle tests; expected: pass. Add `TestSchemaGitLabIntegrationContract` and provider-level `TestGitLabIntegrationPreviewIdentity`: exact four required inputs, seven inputs, four output-only fields, URL/group defaults, two secrets on inputs/outputs, no replacements/tokens/authId/sharing; create IDs/readiness computed, update IDs known/unsecret, no client construction in dry-runs. First run before registration; expected: missing-resource failure.
- [ ] Register the resource and metadata, extend resource inventories without removing independently landed work, and add its ExplicitDependencies assertion. Implement dry-run outputs/wiring to satisfy the provider tests; readiness remains computed after relevant config changes without an API request.
- [ ] Inspect SDK/schema status, then run **SAFE** `make codegen`, **SAFE** `go test -short ./provider -run 'TestGitLabIntegration|TestSchemaGitLabIntegrationContract|TestSchemaHasExactlyTheMVPResources' -count=1`, **SAFE** `make test_provider`, and **SAFE** `make build_sdks`. Expected: pass, exact generated public contract, no token state or unrelated schema removals.
- [ ] Run `git diff --check`, review/stage explicit source/test/schema files and the five generated SDK directories, commit `feat: manage GitLab integration lifecycle`, then run **SAFE** `make check_codegen`. Expected: no drift. Website inventory is updated in Task 6 before docs generation/checks.

### Task 5: Accept computed integration references in workload previews

**Files:** `provider/application.go`, `provider/application_source.go`, `provider/application_source_test.go`, `provider/compose.go`, `provider/compose_source.go`, `provider/compose_source_test.go`; create `provider/gitlab_integration_reference.go` only if the regression requires it.

**Interfaces:** Consumes the registered resource/schema/SDKs. Existing workload input contracts remain unchanged. If the new regression proves needed, introduce `validateWithGitLabReferenceComputed(bool) error` for both source types, with existing `validate()` calling it with false, and a narrow `gitLabIntegrationIDIsComputed(inputs property.Map) bool` helper in `provider/gitlab_integration_reference.go` shared by Application/Compose Check. Only unresolved nested integrationId bypasses the nonempty-ID check; preserve all other validation and concrete mutation checks.

- [ ] Write `TestApplicationComputedGitLabIntegrationReference` and `TestComposeComputedGitLabIntegrationReference`: Check with nested computed integrationId and otherwise valid source succeeds; known external string IDs still succeed; known empty ID fails; computed ID does not suppress known empty branch, zero project ID, conflicting variants, or invalid Application build. Run **SAFE** `go test -short ./provider -run 'Test(Application|Compose)ComputedGitLabIntegrationReference' -count=1`; expected: current nonempty-ID validation fails for computed values.

```go
require.NoError(t, err)
require.Empty(t, checked.Failures) // computed integration ID, valid known source
require.NotEmpty(t, invalidBranch.Failures) // same unknown ID, known empty branch
require.NotEmpty(t, emptyKnownID.Failures) // external ID is known empty
```
- [ ] Implement only the narrow computed-reference validation if tests fail, never sentinel values in returned inputs or a blanket skip when any source field is computed. No API/lifecycle/schema changes to workloads.
- [ ] Run source/preview tests and **SAFE** `make test_provider`; expected: pass. Run `git diff --check` and commit explicit helper/source/tests as `fix: accept computed GitLab integration references` if production changes were necessary, or `test: pin computed GitLab integration references` if existing behavior already passed.

### Task 6: Publish examples/docs and verify the branch

**Files:** Canonical/generated examples, curated docs, website inventory/navigation/tests/generated pages from the file map. Also `website/scripts/generate-examples.mjs` and `website/tests/generate-examples.test.mjs` if the generated-page authorization note is needed.

**Interfaces:** Consumes the completed resource/SDKs and Task 5's usable computed references. Produces `/guides/gitlab-integrations/`, `/reference/git-lab-integration/`, canonical `managedGitlabIntegration` resource and generated equivalents. No additional public resource fields or workload lifecycle changes.
- [ ] Write `TestCanonicalYAMLGitLabIntegration`: managed resource has correct token, client ID/config secret/callback mappings, valid public default, and no tokens. Secret config has `secret:true` and empty placeholder only. Managed output references `.gitlabId`; documented Application and Compose alternatives use it. Preserve existing external gitlabIntegration config/reference documentation. Inventory gains exactly one resource (24 to 25 at this base; account for independently landed additions). Run **SAFE** `go test ./examples -tags=yaml -run TestCanonicalYAMLGitLabIntegration -count=1`; expected: missing resource failure.
- [ ] Add `gitlabApplicationId`, secret `gitlabApplicationSecret`, and `gitlabRedirectUri` config; add `managedGitlabIntegration` and output `managedGitlabIntegrationId`. Use only invalid/noncredential placeholders and empty credential defaults required for conversion. Keep default active workloads Docker/generic Git: managed-GitLab workload references remain explicitly stage-two alternatives, not auto-deployed before authorization. Document create integration, authorize in UI, then add GitLab workloads. Inspect generated example directories, run **SAFE** `make gen_examples`, and rerun YAML tests; expected: pass. Never manually edit translated programs.
- [ ] Write Node tests `GitLabIntegration reference and guide are published`, `GitLab authorization remains a separate stage`, and `GitLab import and recovery guidance is safe`. Assert guide/reference sidebar routes, real schema inventory/secret/default properties, correct slug, Application/Compose `.gitlabId` references, manual OAuth, no token inputs, required user/gitProviders permissions, reauthorization warning, IsConfigured caveat, two-stage deployment, exact import token/ID kind, parent-ID deletion, and inspect-before-retry uncertain-create guidance. Update canonical-route/resource-order assertions without removing unrelated entries. Add built-site tests for both new routes. Run **SAFE** `node --test website/tests/content.test.mjs website/tests/reference-model.test.mjs`; expected: missing guide/inventory/reference failures.
- [ ] Add GitLabIntegration to EXPECTED_RESOURCES, sidebar guide/resource links, and write `gitlab-integrations.mdx`. Layer focused edits into README, source/secret/lifecycle concepts, and applications/compose/imports/troubleshooting guides. Include `pulumi import dokploy:index:GitLabIntegration integration <gitlab-id>`; identify `<gitlab-id>` as relation ID, not OAuth application or parent ID. Explain credential resupply on incomplete import, imported sharing unchanged, no token export, unmanaged-workload deletion risk, and no automatic OAuth/connection verification. Preserve overlapping user edits in the original workspace for later integration.
- [ ] Run **SAFE** `make docs_generate`, inspect generated reference/example pages, and rerun Node source tests. Expected: pass. Check generated complete-example prose links to the two-stage authorization explanation; if the generator's generic deploy wording is misleading, add that note in `website/scripts/generate-examples.mjs` and its existing tests rather than editing generated MDX.
- [ ] Run **SAFE** `make build_sdks`, **SAFE** `make test_examples`, **SAFE** `npm --prefix website run check`, **SAFE** `npm --prefix website run build`, and **SAFE** `npm --prefix website run test:built`. Expected: pass; generated examples compile without embedding tokens/secrets. Report missing tooling honestly.
- [ ] Review source/generated publication diff, run `git diff --check`, stage only explicit Task 6 paths, and commit `docs: publish GitLab integration workflow and examples`.
- [ ] Recheck outer/nested safety assertions, then run each full final command: **SAFE** `make lint`, `make test_provider`, `make test_race`, `make provider`, `go test ./openapi/cmd/normalize -count=1`, `make check_openapi`, `make check_codegen`, `make build_sdks`, `make test_examples`, `make docs_check`. Expected: exit 0, no generation drift, live opt-in off. Run final `git diff --check`, inspect status and complete implementation diff against the execution base; distinguish unrun/blocked checks from passing evidence.
- [ ] Perform the selected workflow's final review, report offline verification and manual-OAuth/deployed-compatibility limitations, and request approval before integration/push/merge. Ask whether to retain or delete only the GitLabIntegration spec/plan; preserve all unrelated working documents.

## Plan self-review

- Spec coverage: Task 1 covers source/generated API; Task 2 covers public contract; Task 3 covers observation, diagnostics, read/import/update/delete; Task 4 covers guarded creation, registration/preview/SDKs; Task 5 covers computed consumers; Task 6 covers examples/docs and final checks.
- Every Review Focus item has named regression tests in its owning task; creation discovery uses unfiltered getAll.
- State/helpers never export tokens; names/property mappings and task Interfaces agree. Public/internal URL secrets are distinct from ordinary identity references.
- Generated schema/SDK/website drift checks run after the matching task commit, not against intentional uncommitted additions.
- Tooling setup preserves selected tool versions, original working directories, and nested safe mise selection without reading repository `.env`. No implementation tooling/tests/live requests were run while authoring this plan.
- Implementation begins only after plan review and execution-method selection.
