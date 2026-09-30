# Read-only Lookup Functions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide twelve typed, ID-only, non-secret Dokploy lookup functions for infrastructure managed outside the current Pulumi stack.

**Architecture:** Register inferred functions beside existing resources. Each calls one authenticated GET operation and explicitly projects its response into a dedicated allowlisted result type. Extend OpenAPI corrections, preserve existing resource behavior, and regenerate SDKs and function reference pages from source.

**Tech Stack:** Go 1.26.6, pulumi-go-provider 1.6.0, Pulumi 3.259.0, oapi-codegen 2.8.0, existing testify/scripted HTTP tests, Node.js 24.18.0, Astro/Starlight documentation tooling.

**Spec:** `docs/superpowers/specs/2026-09-30-read-only-functions-design.md` (approved; commit `b646818`). Read the spec and repository guidance before execution.

## Global Constraints

- ID-only lookups, with no name discovery or scoped searches.
- Engine-specific database functions rather than a generic `getDatabase`.
- Non-secret metadata only.
- Dedicated typed function results, separate from managed-resource state.
- Existing managed-resource contracts remain unchanged.
- Do not hand-edit generated output or add dependencies solely for these lookups.
- IDs are opaque: validate that their trimmed form is nonempty, but send the original value unchanged.
- All twelve tokens, ID fields, outputs, types, optionality, and upstream mappings are exactly those in the spec's Function contracts section.
- Only ID and name are required outputs; preserve explicit empty strings, false, and zero in optional fields. Never apply resource defaults.
- No secret fields, nested objects, source configuration, Compose files, commands, or monitoring configuration in results.
- No response bodies, free-form server messages, input excerpts, private URLs, API keys, or IDs in diagnostic text.
- Use pinned tools; no live acceptance tests, publication, new managed resources, or unrelated refactoring.
- Preserve unrelated user changes; never regenerate over someone else's uncommitted SDK/OpenAPI/docs work.

## Review Focus

1. Opaque IDs containing spaces, `+`, `/`, or `%` must round-trip through query encoding unchanged (Task 2).
2. Optional false/zero/empty values must survive inferred-function encoding, not merely typed handler tests (Tasks 2–4).
3. Moving status fields from `AdditionalProperties` to generated typed fields must not break managed-resource reads or polling (Task 1).
4. Registry URLs with embedded credentials, query tokens, fragments, or scheme-less host/port syntax must not leak credentials (Task 2).
5. Acronym-heavy function names and newly optional schema fields must produce usable SDK exports and stable, linked documentation routes (Tasks 5–6).

---

## File map and interface conventions

Create `provider/lookup_common.go` and `provider/lookup_common_test.go` for lookup-only validation and safe error/URL handling. Do not change existing client redaction or resource error behavior.

Create one source/test pair for each handler:

| Source (under `provider/`) | Receiver | Args/result types | ID Go field / Pulumi property | Token suffix (`dokploy:index:`) |
| --- | --- | --- | --- | --- |
| `get_project.go` | `GetProject` | `GetProjectArgs` / `GetProjectResult` | `ProjectID` / `projectId` | `getProject` |
| `get_environment.go` | `GetEnvironment` | `GetEnvironmentArgs` / `GetEnvironmentResult` | `EnvironmentID` / `environmentId` | `getEnvironment` |
| `get_server.go` | `GetServer` | `GetServerArgs` / `GetServerResult` | `ServerID` / `serverId` | `getServer` |
| `get_registry.go` | `GetRegistry` | `GetRegistryArgs` / `GetRegistryResult` | `RegistryID` / `registryId` | `getRegistry` |
| `get_ssh_key.go` | `GetSSHKey` | `GetSSHKeyArgs` / `GetSSHKeyResult` | `SSHKeyID` / `sshKeyId` | `getSSHKey` |
| `get_application.go` | `GetApplication` | `GetApplicationArgs` / `GetApplicationResult` | `ApplicationID` / `applicationId` | `getApplication` |
| `get_compose.go` | `GetCompose` | `GetComposeArgs` / `GetComposeResult` | `ComposeID` / `composeId` | `getCompose` |
| `get_postgres.go` | `GetPostgres` | `GetPostgresArgs` / `GetPostgresResult` | `PostgresID` / `postgresId` | `getPostgres` |
| `get_mysql.go` | `GetMySQL` | `GetMySQLArgs` / `GetMySQLResult` | `MySQLID` / `mysqlId` | `getMySQL` |
| `get_mariadb.go` | `GetMariaDB` | `GetMariaDBArgs` / `GetMariaDBResult` | `MariaDBID` / `mariadbId` | `getMariaDB` |
| `get_mongodb.go` | `GetMongoDB` | `GetMongoDBArgs` / `GetMongoDBResult` | `MongoDBID` / `mongoId` | `getMongoDB` |
| `get_redis.go` | `GetRedis` | `GetRedisArgs` / `GetRedisResult` | `RedisID` / `redisId` | `getRedis` |

Each corresponding test file replaces `.go` with `_test.go`. Each receiver contains `client clientFactory` and implements:

`Invoke(context.Context, infer.FunctionRequest[GetXArgs]) (infer.FunctionResponse[GetXResult], error)`.

Each Args contains only its required ID string. Each Result contains its required ID and `Name string`, plus exactly the spec's optional fields as `*string`, `*bool`, or `*int` with `pulumi:"field,optional"`. Use existing Go acronym conventions (`AppName`, `EnvironmentID`, `URL`, `SSHKeyID`). Receivers, Args, and Results implement `Annotate(infer.Annotator)` with descriptions; receivers explicitly set the exact lowercase function tokens. Avoid embedding resource Args/State or returning API objects.

`provider/lookup_invoke_test.go` owns framework integration coverage. `provider/lookup_schema_test.go` owns the exact twelve-function contract. `examples/go/lookup_test.go` owns a mocked compiled SDK usage example without changing the existing example program.

Documentation source changes belong in the existing `website/scripts/` reference model/generator/renderer, `website/astro.config.mjs`, `website/src/content/docs/guides/lookups.mdx`, and relevant `website/tests/` files. Generated outputs live in the existing schema/SDK/reference directories.

## Execution safety prerequisite (complete before any test command)

- [ ] Establish isolation using using-git-worktrees, inspect status, and read `AGENTS.md`, `CONTRIBUTING.md`, `tests/README.md`, `.mise.toml`, and the spec. Never read `.env` or print credential variables. This plan may be authored on the current branch; isolation is for implementation.
- [ ] Create a task-local, uncommitted `mise.opencode-safe.local.toml` in the implementation worktree, only if that path does not already exist, containing:

```toml
[env]
DOKPLOY_ACCEPTANCE = "0"
DOKPLOY_ENDPOINT = ""
DOKPLOY_API_KEY = ""
```

- [ ] Verify both outer and nested mise subprocesses have acceptance disabled and credentials blank using assertions, not environment dumps:

```bash
MISE_ENV=opencode-safe mise exec -- python3 -c 'import os, subprocess; check="import os; assert os.environ.get(\"DOKPLOY_ACCEPTANCE\")==\"0\"; assert not os.environ.get(\"DOKPLOY_ENDPOINT\"); assert not os.environ.get(\"DOKPLOY_API_KEY\")"; exec(check); subprocess.run(["mise", "exec", "--", "python3", "-c", check], check=True)'
```

Expected: exit 0, no output. If it fails, stop before testing and resolve mise environment selection; an outer `env DOKPLOY_ACCEPTANCE=0` alone is not sufficient for nested Makefile mise invocations.

- [ ] Use `MISE_ENV=opencode-safe mise exec -- ...` for every execution command below; keep the overlay available until validation finishes. Install missing pinned tools with the same safe environment only when necessary. Do not stage the overlay. Explicitly stage only task files, never `git add .`.

## Task 1: Generate lookup response metadata without resource regressions

**Files:** Modify `openapi/operations.txt`, `openapi/corrections.json`, `openapi/cmd/normalize/main_test.go`; create `internal/client/lookup_contract_test.go`, `provider/lookup_status_compat_test.go`; modify `provider/compose.go`, `provider/postgres.go`, `provider/mysql.go`, `provider/mariadb.go`, `provider/mongodb.go`, `provider/redis.go`; regenerate `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`.

**Interfaces:** Produces `Client.ServerOneWithResponse(ctx, *generated.ServerOneParams)` and typed Server metadata; adds optional `Environment.IsDefault`, `Compose.ComposeStatus`, and database `DockerImage` / `ApplicationStatus`. Existing generated legacy `Image` fields and status helper signatures remain supported.

- [ ] Write `TestLookupOpenAPIContracts` using `normalizeRealContract`: assert `/server.one` is GET-only with required `serverId`, corrected Server response, and only that new selected operation; assert new metadata fields have exact types and previously selected operations/legacy `image` fields remain. Core assertions:

```go
doc := normalizeRealContract(t)
require.Contains(t, doc.Paths, "/server.one")
require.NotNil(t, doc.Paths["/server.one"].Get)
require.Nil(t, doc.Paths["/server.one"].Post)
require.Equal(t, "#/components/schemas/Server", responseSchema(t, doc, "/server.one", "get", "200").Ref)
```

- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./openapi/cmd/normalize -run TestLookupOpenAPIContracts -count=1`; expect missing-contract assertion failures.
- [ ] Add `server.one` and its Server response correction; add optional flat fields matching the spec (nullable where upstream allows null), environment `isDefault`, Compose `composeStatus`, and database `dockerImage` / `applicationStatus`. Server includes only ID, name, and the spec's additional Server fields. Do not edit pinned upstream source.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make generate_openapi`; inspect generated changes.
- [ ] Rerun the normalization test; expect PASS.
- [ ] Write `TestLookupMetadataDecodes` and `TestLookupTypedStatusCompatibility`: decode GET responses containing canonical metadata plus nested/unknown fields; assert canonical and legacy images remain distinct, null optionals decode absent, and all six resource status helpers return `"done"` from typed status. Include typed status precedence over a conflicting legacy map, unknown nonempty strings, missing status, and empty status.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider ./internal/client -run 'TestLookup(MetadataDecodes|TypedStatusCompatibility)' -count=1`; expect existing status helpers to fail.
- [ ] Update only the six existing status helpers to prefer typed status and retain their AdditionalProperties fallback and missing/empty/invalid status errors. Keep resource image behavior and lifecycle logic unchanged.
- [ ] Rerun focused tests and existing map-based status tests; expect PASS.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make test_provider` and `MISE_ENV=opencode-safe mise exec -- go test ./openapi/cmd/normalize -count=1`; expect local tests PASS/live tests SKIP.
- [ ] Inspect `git diff --check` and commit only this task as `feat: generate lookup API metadata with resource compatibility`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make check_openapi` after that commit; expect no unexpected generated diff. Stop if drift occurs.

## Task 2: Lookup safety boundary and control-plane functions

**Files:** Create common helpers/tests and the five project/environment/server/registry/SSH-key source/test pairs from the file map; create `provider/lookup_invoke_test.go`; modify `provider/provider.go`, `provider/provider_test.go`, `provider/schema_test.go`.

**Interfaces:** Consumes Task 1 metadata and existing `clientFactory`, `configuredClient`, `fixedClient`, `newScriptedServer`. Produces the five handlers and these lookup-only helpers in `lookup_common.go`:

- `validateLookupID(field, id string) error`
- `validateLookupIdentity(operation, requestedID string, returnedID, name *string) error`
- `lookupResponseError(operation string) error`
- `lookupError(operation string, err error) error`
- `safeRegistryURL(raw *string) *string`

- [ ] Write `TestLookupValidationAndErrors` and `TestLookupRegistryURLSafety`: assert blank IDs fail, exact nonblank IDs remain untouched, nil/empty/mismatched returned IDs and nil names fail, but a present empty name succeeds. Assert API 404/NOT_FOUND, 401/403, timeout/cancellation, malformed JSON and other failures have safe fixed diagnostic categories. Seed error messages with mock IDs, URLs, private key material, and tokens; assert none appear in error text, while `errors.Is` recognizes context cancellation/deadline sentinels.

```go
require.Error(t, validateLookupID("projectId", " \t\n"))
require.NoError(t, validateLookupID("projectId", " opaque+/percent% "))
require.ErrorIs(t, lookupError("project.one", context.Canceled), context.Canceled)
require.Nil(t, safeRegistryURL(stringPtr("https://user:mock-password@registry.example")))
require.Nil(t, safeRegistryURL(stringPtr("registry.example:5000?token=mock-token")))
require.Equal(t, "registry.example:5000", *safeRegistryURL(stringPtr("registry.example:5000")))
```

- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run 'TestLookup(ValidationAndErrors|RegistryURLSafety)' -count=1`; expect compilation failure for missing helpers.
- [ ] Implement the five helper signatures. Never wrap an unsafe original error; preserve safe context sentinels instead. Classify JSON syntax/type failures as response-contract failures. For URLs preserve nil/empty/safe strings, parse bare hosts with a `//` prefix, and omit parse failures, userinfo, or any `?`/`#` delimiter; return safe input verbatim.
- [ ] Rerun helper tests; expect PASS.
- [ ] Write `TestLookupControlPlaneProject`, `TestLookupControlPlaneEnvironment`, `TestLookupControlPlaneServer`, `TestLookupControlPlaneRegistry`, and `TestLookupControlPlaneSSHKey` in their respective test files using scripted GET expectations and complete/minimal fixtures. Assert exact spec outputs, flat allowlisting, default environments true/false/absent, optional nulls, no follow-up requests, omitted credential-bearing registry URLs, malformed identity/name/type, and HTTP failures. Put `TestLookupOpaqueIDRoundTrip` in `get_project_test.go` with synthetic ID `" opaque+/percent% "`; assert exact decoded query/response identity.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run 'TestLookup(ControlPlane|OpaqueIDRoundTrip)' -count=1`; expect missing-handler failure.
- [ ] Implement the five handlers from the file-map interfaces and spec mappings. Validate ID before calling `r.client`; use no resource reads, organization discovery, or registry credential tests.
- [ ] Rerun control-plane tests; expect PASS.
- [ ] Add `TestLookupRequestCancellation` in `get_project_test.go` using a synchronized local server and real canceled/deadline contexts. Assert prompt termination, safe diagnostics, and `errors.Is` categories; no fixed sleeps or live endpoint. Run the new test with `-count=1` and expect PASS or follow RED/implementation/recheck if the boundary is incomplete.
- [ ] Write `TestLookupControlPlaneInvoke` in `lookup_invoke_test.go`: use `integration.NewServer(... integration.WithProvider(Provider()))`, configure it with a local scripted server URL and fake API key through `p.ConfigureRequest.Args`, then invoke each exact registered token with `property.Map` arguments. Assert returned keys exactly equal allowlisted present fields, optional nils are omitted, explicit false/zero/empty values survive, and missing/wrong-type/empty/whitespace-only input produces failures/error without HTTP traffic. Invoke an unknown token and assert it does not route to a lookup.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run TestLookupControlPlaneInvoke -count=1`; expect unregistered-token errors.
- [ ] Register the five `infer.Function(&GetX{client: configuredClient})` entries and update both existing empty-functions assertions to this exact subset while preserving resource/component assertions. Annotate all fields/types with no input defaults or secret-bearing outputs.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run 'TestLookup|TestProvider|TestSchema' -count=1`; expect PASS.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make test_provider`, inspect `git diff --check`, and commit the scoped changes as `feat: add safe control-plane lookup functions`.

## Task 3: Application and Compose lookup functions

**Files:** Create `provider/get_application.go`, `provider/get_application_test.go`, `provider/get_compose.go`, `provider/get_compose_test.go`; modify `provider/provider.go`, `provider/lookup_invoke_test.go`, `provider/provider_test.go`, `provider/schema_test.go`.

**Interfaces:** Consumes Task 2 helpers and invocation harness. Produces `GetApplication` / `GetCompose` and their Args/Result/Invoke signatures from the file map. Registers `dokploy:index:getApplication` and `dokploy:index:getCompose`.

- [ ] Write `TestLookupWorkloadsApplication` and `TestLookupWorkloadsCompose` in their respective test files with scripted GETs: map application status/nullable registry IDs and Compose status/type into exact results. Seed env/build/source/Compose-file/command/nested secret fields and assert exclusion. Minimal ID/name succeeds, future status/type strings remain valid, missing status stays absent, and explicit empty status stays empty. Include null/minimal responses, mismatched IDs, invalid types, 404 and 403.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run TestLookupWorkloads -count=1`; expect missing-handler failure.
- [ ] Implement the two direct GET handlers and annotations using Task 2 helper interfaces. Do not decode or validate source objects.
- [ ] Rerun workload handler tests; expect PASS.
- [ ] Write `TestLookupWorkloadInvoke` using the integration harness to assert exact encoded fields, omitted optionals, preserved empty strings, and exclusion of sensitive properties. Core assertions on its `p.InvokeResponse`:

```go
_, hasEnvironment := response.Return.GetOk("environment")
_, hasSource := response.Return.GetOk("source")
require.False(t, hasEnvironment)
require.False(t, hasSource)
require.Equal(t, "", response.Return.Get("status").AsString())
```

- [ ] Run the workload invoke test with the safe prefix and `-count=1`; expect unregistered-token errors.
- [ ] Register the two functions and extend exact-current-token assertions from five to seven.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run 'TestLookup|TestSchema|TestProvider' -count=1`; expect PASS.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make test_provider`, inspect `git diff --check`, and commit only this task as `feat: add application and Compose lookup functions`.

## Task 4: Engine-specific database lookups

**Files:** Create all five database source/test pairs from the file map; create `provider/lookup_schema_test.go`; modify `provider/provider.go`, `provider/lookup_invoke_test.go`, `provider/provider_test.go`, `provider/schema_test.go`.

**Interfaces:** Consumes Tasks 1–2 typed metadata and safety helpers. Produces five database handlers and completes the exact twelve-function schema/registration contract, with `getMongoDB` accepting `mongoId` (not `mongodbId`).

- [ ] Write unique `TestLookupDatabasesPostgres`, `...MySQL`, `...MariaDB`, `...MongoDB`, and `...Redis` tests in their respective files: exact GET/query, engine-specific fields, optional status/image/port/placement, and exclusion of passwords/env/nested values. Assert canonical image wins even when empty; legacy image applies only for absent/null canonical; no image defaults. Include false replicaSets, zero externalPort, empty strings, minimal/null responses, future statuses, malformed data, and mismatched IDs.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run TestLookupDatabases -count=1`; expect missing-handler failure.
- [ ] Implement the five exact handlers/annotations. If needed, add `lookupDatabaseImage(canonical, legacy *string) *string` to `lookup_common.go`; select the nonnil canonical pointer before legacy. No reflection or resource defaults.
- [ ] Rerun database handler tests; expect PASS.
- [ ] Write `TestLookupDatabaseInvoke` using the integration harness for every engine: exact encoded fields/optionality and false/zero/empty values plus image precedence. In the MongoDB explicit-zero fixture, assert:

```go
require.False(t, response.Return.Get("replicaSets").AsBool())
require.Equal(t, float64(0), response.Return.Get("externalPort").AsNumber())
_, hasPassword := response.Return.GetOk("databasePassword")
require.False(t, hasPassword)
```

- [ ] Run the database invoke test with the safe prefix and `-count=1`; expect unregistered-token errors.
- [ ] Register all five functions and extend existing exact-current-token tests to twelve.
- [ ] Write `TestLookupSchemaContract`: assert the exact twelve tokens, one required input each, ID/name-only required outputs, exact output sets/types, all descriptions, no output defaults/excluded fields, unchanged eighteen-resource set, and no components or generic/list/name-selector functions. Extend all-family invoke cases to reject missing/wrong-type/empty/whitespace IDs without HTTP calls; retain error classification and cancellation tests from Task 2.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run 'TestLookup|TestSchema|TestProvider' -count=1`; expect PASS. If schema assertions fail, fix and rerun before committing.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make test_provider`, inspect `git diff --check`, and commit as `feat: add typed database lookups and schema contracts`.

## Task 5: Regenerate SDKs and prove consumer invocation

**Files:** Regenerate `provider/cmd/pulumi-resource-dokploy/schema.json` and `sdk/go`, `sdk/nodejs`, `sdk/python`, `sdk/dotnet`, `sdk/java`; create `examples/go/lookup_test.go`; create `provider/lookup_codegen_test.go`; modify provider metadata in `provider/provider.go` to describe read-only lookups if needed.

**Interfaces:** Consumes the final twelve-function schema. Produces generated language-native exports and a compiled mocked consumer using generated Go `GetProject` and `GetEnvironment` functions with `pulumi.WithMocks`. No live endpoint or provider plugin is required by the mock test.

- [ ] Write `TestLookupGeneratedSchema` in `lookup_codegen_test.go` to parse committed generated JSON into `schema.PackageSpec`; assert the twelve tokens and the same required/optional/safe field contract as source schema, plus publishing metadata. Core assertions:

```go
require.Len(t, committed.Functions, 12)
require.ElementsMatch(t, []string{"projectId"}, committed.Functions["dokploy:index:getProject"].Inputs.Required)
require.ElementsMatch(t, []string{"projectId", "name"}, committed.Functions["dokploy:index:getProject"].Outputs.Required)
require.NotContains(t, committed.Functions, "dokploy:index:getDatabase")
```

- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test -short ./provider -run TestLookupGeneratedSchema -count=1`; expect absent-function assertions to fail.
- [ ] Inspect status/diff of every generated target; stop for unrelated uncommitted output changes.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make codegen` and inspect generated schema/exports. Preserve repository generation/version conventions; never hand-edit generated names or packaging.
- [ ] Rerun the generated-schema test; expect PASS.
- [ ] Write `TestLookupSDKReferenceWithoutResourceRegistration` in `examples/go/lookup_test.go`: call generated `GetProject` and `GetEnvironment` with synthetic IDs under `pulumi.WithMocks`; assert returned metadata and generated optional types. Assert exactly two function calls and zero Dokploy resources. Permit Pulumi's automatic root `pulumi:pulumi:Stack` bookkeeping, but have `NewResource` fail for any other type; initial `Call` fixtures return empty maps to establish RED.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go -C examples/go test -run TestLookupSDKReferenceWithoutResourceRegistration -count=1`; expect metadata assertions to fail, with no server contact.
- [ ] Implement the mock `Call(pulumi.MockCallArgs) (resource.PropertyMap, error)` to handle exactly `dokploy:index:getProject` and `dokploy:index:getEnvironment` and return synthetic ID/name/default-environment/project/flag fields. Do not add a running-provider requirement.
- [ ] Rerun the mock consumer test; expect PASS, exactly two calls, and zero Dokploy registrations.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make build_sdks` and `MISE_ENV=opencode-safe mise exec -- make test_examples`; expect all five SDK builds and existing example checks PASS. If a check is unavailable, record the precise blocker without weakening it or publishing packages.
- [ ] Inspect generated diffs for missing exports, incorrect acronym casing, accidental secrets, unexpected dependency changes, and unrelated changes. Run `git diff --check`, then explicitly commit schema/SDK outputs and the two new tests as `feat: generate lookup SDKs and mocked consumer coverage`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make check_codegen`; expect no unexpected schema or SDK drift. Do not count a dirty-worktree diff failure before committing expected generated changes as a generation defect.

## Task 6: Generate function references and lookup guidance

**Files:** Modify `website/scripts/reference-model.mjs`, `website/scripts/render-reference.mjs`, `website/scripts/generate-reference.mjs`, `website/astro.config.mjs`, `website/tests/reference-model.test.mjs`, `website/tests/render-reference.test.mjs`, `website/tests/content.test.mjs`, `website/tests/site-output.built-test.mjs`; create `website/src/content/docs/guides/lookups.mdx`; regenerate `website/src/content/docs/reference/`; modify `README.md`, `CHANGELOG.md` with concise feature notes following their existing formats.

**Interfaces:** `parseSchema(schema, options)` adds `functions: Array<{token,name,slug,description,inputs,outputs}>` without changing resource/config/type models; missing `schema.functions` yields `[]` for existing fixtures. Export `renderFunction(functionModel): string` beside `renderResource`. Generator writes one flat `<slug>.mdx` file per function. Add function slug overrides for exact routes `get-project`, `get-environment`, `get-application`, `get-compose`, `get-postgres`, `get-mysql`, `get-mariadb`, `get-mongodb`, `get-redis`, `get-server`, `get-registry`, `get-ssh-key`, without changing existing resource slugs.

- [ ] Add model tests for schema function input/output properties and required arrays, exact twelve real-schema names, optionality, deterministic ordering, missing descriptions/dangling references, absent-functions compatibility, and acronym-heavy slugs. In a minimal `getProject` fixture with only ID input and ID/name outputs, assert:

```js
assert.equal(model.functions[0].name, "getProject");
assert.equal(model.functions[0].slug, "get-project");
assert.equal(model.functions[0].inputs[0].name, "projectId");
assert.equal(model.functions[0].inputs[0].required, true);
assert.equal(slugFromToken("dokploy:index:getSSHKey"), "get-ssh-key");
assert.equal(slugFromToken("dokploy:index:getMySQL"), "get-mysql");
```

- [ ] Run `MISE_ENV=opencode-safe mise exec -- node --test website/tests/reference-model.test.mjs`; expect new assertions to fail.
- [ ] Extend model normalization and function slug overrides using the stated interfaces.
- [ ] Rerun model tests; expect PASS.
- [ ] Add renderer tests for function title/notice/description, Inputs/Outputs tables, required metadata, and explicit read-only/non-secret wording, using `assert.match(mdx, /## Inputs/)`, `assert.match(mdx, /## Outputs/)`, and `assert.match(mdx, /read-only/i)` plus exact serialized property arrays.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- node --test website/tests/render-reference.test.mjs`; expect missing renderer failure.
- [ ] Implement `renderFunction`, preserving PropertyTable/frontmatter serialization conventions.
- [ ] Rerun renderer tests; expect PASS.
- [ ] Add content/navigation/built-site tests for all twelve linked routes, the guide/frontmatter/approved guidance, preserved resource sidebar order, and old routes.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- node --test website/tests/content.test.mjs`; expect missing-guide/navigation failures. Built-site assertions remain RED until rebuilding.
- [ ] Update the generation loop to render sorted function pages within existing atomic replacement; add the Functions sidebar and Lookups guide link.
- [ ] Write the guide with TypeScript `getProject`/`getEnvironment` placeholder-ID snippets and the approved behavior distinctions. Verify exported names against generated SDKs; use Task 5's mocked Go consumer as a second usage example. Do not change unrelated example programs.
- [ ] Add README/CHANGELOG feature notes in their existing formats.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make docs_generate`; inspect all generated pages for safe contents and unchanged old routes.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- npm --prefix website run check`; expect source/doc tests PASS.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- npm --prefix website run build`; expect build PASS.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- npm --prefix website run test:built`; expect route/link/accessibility tests PASS. Avoid unrelated design/component changes.
- [ ] Run `git diff --check`, explicitly commit the source tests/guide/navigation and generated reference changes as `docs: document read-only lookup functions`, then run `MISE_ENV=opencode-safe mise exec -- make docs_check`; expect no unexpected generation drift and all checks PASS.

## Final verification and execution handoff

- [ ] Re-run the outer/nested acceptance-disabled assertions before the final suite. Execute `MISE_ENV=opencode-safe mise exec -- make lint`, `make test_provider`, `make test_race`, `make provider`, `make check_openapi`, `make check_codegen`, `make build_sdks`, `make test_examples`, and `make docs_check` with the same full prefix on each invocation. Also run `MISE_ENV=opencode-safe mise exec -- go test ./openapi/cmd/normalize -count=1`. Expect exit 0; report unavailable checks explicitly.
- [ ] Inspect source/committed-schema function contracts, all twelve GET paths, and final diffs. Confirm no dependencies were intentionally added and no credentials or real IDs are present. Run `git diff --check`, `git status --short`, and the relevant branch diff. Fix and re-verify any regression rather than declaring an unrun check passed.
- [ ] Obtain the review required by the execution method the user selects, address findings, and re-run affected tests. No review agents are dispatched during planning; native execution gets one fresh whole-branch reviewer, while subagent-driven execution follows its task review gates and final branch review.
- [ ] Remove only the task-created safety overlay once no more execution commands will run; never remove a pre-existing file at that path. Keep it present if further validation is needed. Do not revert unrelated files.
- [ ] Report functions delivered, validation evidence/blockers, commit/branch status, and ask the user whether to retain or delete this task's spec, plan, and `/tmp/opencode/dokploy-read-only-functions-brainstorming.md`. Preserve unrelated documents; do not delete SDD documents without approval. Use finishing-a-development-branch only after implementation/review and passing verification.

## Plan self-review

The six tasks cover the approved function contracts, optionality, default environments, security/error boundary, OpenAPI sources and compatibility, framework invokes, all SDKs, generated/curated docs, and safe local validation. Review Focus conditions have explicit owning tests. The typed status compatibility adjustments in Task 1 are required by code-generation behavior, not an expansion of lifecycle scope. No implementation begins until the user reviews this plan and chooses an execution method.
