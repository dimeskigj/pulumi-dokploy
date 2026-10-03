# Configuration-only Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a configuration-only `dokploy:index:Server` resource with generated SDKs, lifecycle regression tests, and complete user documentation.

**Architecture:** Use the existing inferred-resource and configured-client patterns. Generate the four server CRUD endpoints, project responses into flat managed state, and preserve acknowledged identity on readback failures. Generate references from the schema and language examples from canonical YAML; keep lifecycle explanations in curated guides.

**Tech Stack:** Go 1.26.6, pulumi-go-provider 1.6.0, Pulumi 3.259.0, oapi-codegen 2.8.0, nullable JSON fields, testify/scripted HTTP tests, Node.js 24.18.0, Astro/Starlight.

**Spec:** `docs/superpowers/specs/2026-10-03-server-resource-design.md` (approved, commit `c14b986`). Read it together with `AGENTS.md`, `CONTRIBUTING.md`, and `tests/README.md`.

## Global Constraints

- Configuration-only CRUD and import by server ID; no host bootstrap, connection tests, deployment polling, commands, monitoring, security operations, or lookup functions.
- Defaults: port `22`, username `root`, server type `deploy`, and Docker cleanup `false`.
- Inputs: `name`, `description`, `ipAddress`, `port`, `username`, `sshKeyId`, `serverType`, `enableDockerCleanup`. Additional outputs: `serverId`, `organizationId`, `status`.
- All configuration fields update in place; omit description/SSH key to clear them to null.
- Explicit false is always sent. Enabling Docker cleanup authorizes Dokploy's recurring cleanup schedule.
- Preserve nonempty acknowledged create identity and partial state; do not discover by name or retry mutations.
- Only flat public configuration/metadata; no raw response objects, nested credentials, private addresses/URLs, IDs, or arbitrary upstream text in diagnostics.
- Existing managed-resource contracts and pre-existing read-only lookup documents remain unchanged.
- Use pinned tools and generation targets. Never hand-edit generated SDKs, API clients, references, or converted language examples.
- Offline verification only. Do not read/copy `.env`, use live credentials, run live acceptance, or publish packages.
- Preserve unrelated changes. Stage explicit task paths only, never `git add .`.

## Review Focus

1. Unknown SSH-key outputs and defaulted fields must survive framework Check/preview without API calls or spurious replacement (Task 2).
2. A create acknowledgment containing an ID and malformed unrelated/nested fields must not lose that ID when the authoritative read fails (Tasks 1–2).
3. Clearing nullable strings and preserving explicit false/empty descriptions must round-trip through generated request serialization and inferred state (Tasks 1–2).
4. `NOT_FOUND` from an inactive-server update or a failed delete must not falsely report disappearance/deletion (Task 2).
5. Regenerated documentation and examples must expose Server without implying registration bootstraps a host or attaching example workloads to an unready machine (Task 3).

---

## File map and interfaces

- API contract: modify `openapi/operations.txt`, `openapi/corrections.json`, and `openapi/cmd/normalize/main_test.go`; create `internal/client/server_contract_test.go`. Regenerate `openapi/dokploy.json` and `internal/client/generated/generated.gen.go`.
- Resource: create `provider/server.go`, `provider/server_test.go`, `provider/server_framework_test.go`, and `provider/server_docs_test.go`; modify `provider/provider.go`, `provider/provider_test.go`, `provider/schema_test.go`, `provider/registry_metadata_test.go`, and `provider/preview_dependencies_test.go` where existing coverage belongs.
- Generated package contract: regenerate `provider/cmd/pulumi-resource-dokploy/schema.json` and all five `sdk/` trees.
- Documentation: modify `README.md`, `website/scripts/reference-model.mjs`, `website/astro.config.mjs`, `website/tests/reference-model.test.mjs`, `website/tests/content.test.mjs`, `website/src/content/docs/guides/imports.mdx`, `website/src/content/docs/concepts/lifecycle-and-state.mdx`, and `website/src/content/docs/examples/index.mdx`; create `website/src/content/docs/guides/servers.mdx`. Regenerate `website/src/content/docs/reference/` and `website/src/content/docs/examples/complete.mdx`.
- Examples: modify `examples/yaml/Pulumi.yaml`, `examples/yaml/README.md`, and `examples/yaml_test.go`; regenerate the existing Node.js, Python, Go, .NET, and Java example directories.

`Server` contains `client clientFactory`. `ServerArgs` has `Name string`, `Description *string`, `IPAddress string`, `Port int`, `Username string`, `SSHKeyID *string`, `ServerType string`, and `EnableDockerCleanup bool`, with exact Pulumi property tags above. Defaulted/nullable fields have `optional` tags and annotated defaults. `ServerState` embeds `ServerArgs` and adds `ServerID string`, `OrganizationID *string`, and `Status *string`. Metadata pointers permit omission in partial fallback state; complete successful reads require them. No extra public types are necessary.

## Safety prerequisite: before executing any tests or generation

- [ ] Use `using-git-worktrees` at execution time to establish isolation and inspect status. Do not run its automatic baseline tests before this safety setup. Do not copy `.env` or other credentials into the worktree.
- [ ] In the implementation worktree only, use a targeted patch to temporarily replace `.mise.toml`'s `[env]` block with the following. Keep `[tools]` unchanged, record this as an agent-owned temporary modification, and never stage it:

```toml
[env]
DOKPLOY_ACCEPTANCE = "0"
DOKPLOY_ENDPOINT = ""
DOKPLOY_API_KEY = ""
```

- [ ] Verify the effective environment in both outer and nested mise commands without printing values:

```bash
mise exec -- python3 -c 'import os, subprocess; check="import os; assert os.environ.get(\"DOKPLOY_ACCEPTANCE\")==\"0\"; assert not os.environ.get(\"DOKPLOY_ENDPOINT\"); assert not os.environ.get(\"DOKPLOY_API_KEY\")"; exec(check); subprocess.run(["mise", "exec", "--", "python3", "-c", check], check=True)'
```

Expected: exit 0, no output. Stop before running tests if the assertions fail. Do not inspect/print environment dumps. Ensure any inherited parent configuration does not re-enable credential loading; choose an external worktree under `/tmp/opencode` if project-local worktree ancestry inherits the original environment directives.

- [ ] Use `mise exec --` for execution commands below. Install missing pinned tools only after the environment assertions pass. Keep the safe environment active through all generation, verification, and review fixes; restore only the agent-owned `[env]` patch at the final handoff, using a targeted patch rather than a destructive git reset.

## Task 1: Typed server CRUD API contract

**Files:** API contract/source/test/generated files from the file map.

**Interfaces:** Produces `generated.Server` for authoritative reads, `generated.ServerMutationResult` for create/update identity acknowledgments, `generated.ServerCreateRequest`, `generated.ServerUpdateRequest`, and generated request/response wrappers for the four endpoints. Existing `client.Client` embeds the generated wrappers unchanged.

- [ ] Write `TestRealContractIncludesServerCRUD` in `openapi/cmd/normalize/main_test.go`. Use existing helpers and pin these assertions:

```go
d := normalizeRealContract(t)
require.Contains(t, d.Paths, "/server.one")
require.NotNil(t, d.Paths["/server.one"].Get)
for _, operation := range []string{"server.create", "server.update", "server.remove"} {
    require.Contains(t, d.Paths, "/"+operation)
    require.NotNil(t, d.Paths["/"+operation].Post)
}
require.Equal(t, "#/components/schemas/Server", responseSchema(t, d, "/server.one", "get", "200").Ref)
require.Equal(t, "#/components/schemas/ServerMutationResult", responseSchema(t, d, "/server.create", "post", "200").Ref)
for _, operation := range []string{"server.setup", "server.validate", "server.security", "server.setupMonitoring", "server.updateBuildsConcurrency"} {
    require.NotContains(t, d.Paths, "/"+operation)
}
```

Also assert integer request ports, nullable required description/SSH key properties, deploy/build request enums, required `serverId` query/update/delete identity, and unchanged existing selected operations. Assert response Server's exact eleven flat properties (eight inputs plus three API identity/metadata fields, using `serverStatus` rather than `status`).

- [ ] Run `mise exec -- go test ./openapi/cmd/normalize -run TestRealContractIncludesServerCRUD -count=1`; expect missing-server contract failure, not an environmental failure.
- [ ] Add exactly four operations and corrections. Use `ServerMutationResult` with only required `serverId` and `additionalProperties:false` for create/update: extra response fields are deliberately ignored so malformed unrelated data cannot discard identity. Use `Server` with required `serverId`, optional typed flat fields, nullable description/SSH key, integer port, string status/type, and `additionalProperties:false`; Read validates required persisted fields. Ignore the remove response body via the existing empty response correction. Do not change vendored upstream/source snapshots.
- [ ] Define `ServerCreateRequest` and `ServerUpdateRequest` by preserving the upstream request contract except correcting port to integer. Preserve the update-only optional `command` in the generated API request type, but the resource never assigns it. Use required nullable description/SSH key; retain the upstream cleanup default true in API metadata, distinct from the provider's false default.
- [ ] Run `mise exec -- make generate_openapi`, inspect changes, then rerun the normalization test; expect PASS.
- [ ] Write `TestServerClientContract` in `internal/client/server_contract_test.go` using `httptest`: assert exact serialization of `description:null`, `sshKeyId:null`, port `22`, explicit cleanup false, and omitted `command`; assert read decodes false and empty description distinctly from null. Assert an acknowledgment with valid ID plus malformed unrelated fields yields the ID, and nested `sshKey.privateKey`/monitoring fields never become typed public fields.
- [ ] Run `mise exec -- go test ./internal/client -run TestServerClientContract -count=1`; expect PASS. These are generated-client contract tests after source normalization's red/green cycle.
- [ ] Run `mise exec -- go test ./openapi/cmd/normalize ./internal/client -count=1` and `git diff --check`; expect PASS/exit 0.
- [ ] Commit explicit source/test/generated API paths as `feat: generate configuration-only server CRUD client`.
- [ ] Run `mise exec -- make check_openapi` after the commit; expect exit 0/no generated drift.

## Task 2: Managed Server lifecycle, schema, and SDKs

**Files:** Resource and generated-package files from the file map, plus `README.md` for the new resource list/import baseline.

**Interfaces:** Consumes Task 1 wrappers: `ServerCreateWithResponse(context.Context, generated.ServerCreateJSONRequestBody)`, `ServerOneWithResponse(context.Context, *generated.ServerOneParams)`, `ServerUpdateWithResponse(context.Context, generated.ServerUpdateJSONRequestBody)`, and `ServerRemoveWithResponse(context.Context, generated.ServerRemoveJSONRequestBody)`. Produces `Server`, `ServerArgs`, `ServerState`, and all five SDK resource exports.

Implement these receiver signatures:

```go
func (r Server) Check(context.Context, infer.CheckRequest) (infer.CheckResponse[ServerArgs], error)
func (r Server) Diff(context.Context, infer.DiffRequest[ServerArgs, ServerState]) (infer.DiffResponse, error)
func (r Server) Create(context.Context, infer.CreateRequest[ServerArgs]) (infer.CreateResponse[ServerState], error)
func (r Server) Read(context.Context, infer.ReadRequest[ServerArgs, ServerState]) (infer.ReadResponse[ServerArgs, ServerState], error)
func (r Server) Update(context.Context, infer.UpdateRequest[ServerArgs, ServerState]) (infer.UpdateResponse[ServerState], error)
func (r Server) Delete(context.Context, infer.DeleteRequest[ServerState]) (infer.DeleteResponse, error)
func (r Server) WireDependencies(infer.FieldSelector, *ServerArgs, *ServerState)
```

Add pointer-receiver `Annotate(infer.Annotator)` methods for receiver/args/state. Internal projection/error helpers can stay in this file; do not alter global error behavior.

- [ ] Write `TestServerCheckDefaultsAndValidation`, `TestServerDiff`, `TestServerDryRun`, and `TestSchemaServerContract` first. Pin required inputs exactly to name/address, output-only identity/metadata, no other fields, and no replacement flags. Core assertions:

```go
require.Equal(t, 22, checked.Inputs.Port)
require.Equal(t, "root", checked.Inputs.Username)
require.Equal(t, "deploy", checked.Inputs.ServerType)
require.False(t, checked.Inputs.EnableDockerCleanup)
require.Equal(t, p.Update, diff.DetailedDiff["ipAddress"].Kind)
require.False(t, unchanged.HasChanges)
```

Table cases cover whitespace-only name/address/username/SSH key, ports 0/65536 and valid boundaries 1/65535, explicit empty type versus omission, invalid type, hostname/IPv4/IPv6 addresses, unchanged versus every single-field diff, absent versus empty description, and explicit cleanup false/true. Use computed property values for every input to ensure validation defers correctly. Dry-run clients must fail the test if constructed.

- [ ] Run `mise exec -- go test -short ./provider -run 'Test(Server(Check|Diff|DryRun)|SchemaServerContract)' -count=1`; expect missing Server symbols/contract failure.
- [ ] Implement the public structs, annotations/defaults, Check, Diff, and dry-run paths in `provider/server.go`. Register `infer.Resource(&Server{client: configuredClient})`; add Server to schema exact-resource assertions and provider metadata, add its explicit-dependencies compile-time assertion in `provider/provider_test.go`, and change registry resource count from 19 to 20. Make lifecycle method bodies minimal until the next red/green cycle. Add Server/previously omitted Schedule to README's list and correct its stale count to twenty; add reserved-placeholder Server import guidance.
- [ ] Rerun the focused tests; expect PASS.
- [ ] Write `TestServerLifecycleAndImport`, `TestServerReadContract`, `TestServerPartialFailures`, `TestServerMutationErrors`, and `TestServerDiagnostics`. Use the scripted server with only reserved fixture values. Assert exact requests and authoritative readback, nullable clearing, imported inputs, false/empty preservation, identity matching, drift, unknown read status/type, and absence of all nested/unmanaged output fields. For missing/null/malformed required persisted fields, assert a safe error rather than fabricated defaults.
- [ ] Include these concrete lifecycle/error assertions:

```go
require.Equal(t, "placeholder-server", created.ID)
require.Equal(t, "placeholder-server", created.Output.ServerID)
var partial infer.ResourceInitFailedError
require.ErrorAs(t, createReadbackErr, &partial)
require.ErrorAs(t, updateReadbackErr, &partial)
require.Equal(t, "placeholder-server", partialCreate.ID)
require.Equal(t, "after", partialUpdate.Output.Name)
require.Equal(t, prior, rejectedUpdate.Output)
require.Empty(t, absentRead.ID)
```

Test create acknowledgment containing valid identity with malformed unrelated properties, followed by failed Read; valid identity must survive. Test missing identity without discovery/retry, create post-read not-found/mismatched ID, update mismatched acknowledgment, update `NOT_FOUND` for inactive existing record, mutation 400/403/500 and transport/cancellation failures, workload-delete refusal, delete-not-found confirmed absent versus still present/failed verification. Assert mutation count is one and unexpected operational endpoints are never called. Feed API errors containing fixture address/IDs, private-key markers, and monitoring tokens; none may appear in diagnostic text.

- [ ] Run `mise exec -- go test -short ./provider -run 'TestServer(Lifecycle|ReadContract|PartialFailures|MutationErrors|Diagnostics)' -count=1`; expect lifecycle assertions to fail against minimal methods.
- [ ] Implement actual CRUD and strict flat read projection. Create/update use identity-only acknowledgments and readback; fallback create state is effective inputs plus ID with metadata absent, fallback acknowledged update uses new inputs plus existing ID/metadata. Read uses generated field presence, not zero-value defaults. Use `initFailed` for acknowledged-write/readback failures; pulumi-go-provider 1.6.0 Update supports `ResourceInitFailedError` and serializes `PartialState`. Reject mismatched identity without adopting it. Failed mutations return safe errors and prior update state. On remove not-found, confirm absence with Read before accepting deletion.
- [ ] Rerun lifecycle tests; expect PASS.
- [ ] Write `TestServerFrameworkPreviewAndPartialState` in `provider/server_framework_test.go` following existing `integration.NewServer` tests. Assert framework Create preview yields computed ID, Update preview retains known ID/metadata, computed `sshKeyId` stays computed, and partial create/update errors retain encoded ID/new acknowledged configuration with nonnil `PartialState`. Also assert encoded state has no nested secrets. Configure only the local test server, never real provider credentials.
- [ ] Run the framework test; if it exposes missing dependency/encoding handling, capture the failure and implement minimal `WireDependencies` fixes. Stable identity/metadata must not become unnecessarily secret/computed merely because an SSH-key input was secret/computed. Rerun to PASS.
- [ ] Inspect status before generation; run `mise exec -- make codegen` and inspect schema/all-five-SDK output for defaults, exports, output optionality, and unexpected changes. No hand edits.
- [ ] Run `mise exec -- make test_provider`, `mise exec -- make lint`, `mise exec -- make provider`, and `git diff --check`; expect PASS with live tests skipped. Website generation waits for Task 3's allowlist update.
- [ ] Commit explicit provider, README, schema, and SDK paths as `feat: manage Dokploy server configuration`.
- [ ] Run `mise exec -- make check_codegen`; expect exit 0 and no generated drift.

## Task 3: Documentation and safe generated examples

**Files:** Documentation/example source/test/generated files from the file map, plus `provider/server_docs_test.go`.

**Interfaces:** Consumes Task 2 schema/SDKs. Produces `/reference/server/`, `/guides/servers/`, sidebar routes, Server import guidance, and six-language canonical examples. The reference renderer remains generic; lifecycle prose belongs in the curated guide.

- [ ] Write documentation regressions in `website/tests/content.test.mjs` and `provider/server_docs_test.go`. Assert Server reference/guide/sidebar exist, resource lists contain all 20 actual schema resources, README/import guide include `pulumi import dokploy:index:Server`, guide states no bootstrap/SSH check/readiness guarantee, status is descriptive, cleanup defaults false and enabling schedules recurring cleanup, all config updates are in place, and delete neither destroys the VM nor uninstalls software. Explain workload-delete refusal, deployment history/log removal, partial identity retention, and inactive update failures. Core content assertions:

```js
assert.match(guide, /enableDockerCleanup/);
assert.match(guide, /defaults to [`'"]?false/i);
assert.match(guide, /bootstrap|install/i);
assert.match(guide, /server\.serverId/);
assert.match(imports, /pulumi import dokploy:index:Server/);
assert.match(config, /link: "\/reference\/server\/"/);
assert.match(config, /link: "\/guides\/servers\/"/);
```

Also add a synthetic-schema Server case to `website/tests/reference-model.test.mjs` asserting slug `server`, defaults 22/root/deploy/false, exact flat outputs, and no replacement flags. Update real-schema expectation to 20 and require Server.

- [ ] Run `mise exec -- node --test website/tests/content.test.mjs website/tests/reference-model.test.mjs` and `mise exec -- go test -short ./provider -run TestServerDocumentation -count=1`; expect missing docs/unsupported resource assertions.
- [ ] Add Server to `website/scripts/reference-model.mjs`'s expected-resource set and insert Server after Environment in the resource sidebar and expected-order tests. Add the Servers guide link. Write `guides/servers.mdx`, update imports/README/examples index, and correct lifecycle prose that currently claims every create/update polls deployment. Leave landing-page visual design and unrelated documentation unchanged.
- [ ] Write `TestCanonicalServerExample` in `examples/yaml_test.go`: assert `remoteServer` has token `dokploy:index:Server`, `ipAddress:"192.0.2.10"`, `sshKeyId:"${sshKey.sshKeyId}"`, deploy type, and cleanup false; output `remoteServerId` references `${remoteServer.serverId}`. Assert no existing workload's active `serverId` targets this unready example server. A commented placement example in the guide explains how to connect a separately prepared host.
- [ ] Run `mise exec -- go test ./examples -tags=yaml -run TestCanonicalServerExample -count=1`; expect missing-example assertion failure.
- [ ] Add that registration-only Server example to `examples/yaml/Pulumi.yaml` and update its README. Keep the existing secret SSH-key configuration; do not add private material, actual endpoints, or operational actions. Explain replacing the reserved address and preparing the remote host separately before workload placement. Do not change the existing workloads' placement.
- [ ] Inspect status, run `mise exec -- make gen_examples`, inspect all five converted example directories, and rerun the canonical test; expect PASS. This target replaces example directories, so preserve unrelated changes before running it.
- [ ] Run `mise exec -- make docs_generate`; inspect generated `reference/server.mdx` and complete examples. Do not hand-edit them. Rerun focused website/provider documentation tests; expect PASS.
- [ ] Run `mise exec -- npm --prefix website run check` and `mise exec -- npm --prefix website run build`; expect PASS. Run `git diff --check` and review README/guide/reference/examples for unsafe or misleading copy.
- [ ] Commit explicit documentation, example, and test paths as `docs: explain configuration-only servers and cleanup safety`.
- [ ] Run `mise exec -- make docs_check` and `mise exec -- make test_examples` after committing generated docs/examples; expect exit 0, no generated drift, and compilation in all five languages.

## Final verification and handoff

- [ ] Reconfirm safe outer/nested environment assertions before validation.
- [ ] Run `mise exec -- make lint`, `mise exec -- make test_provider`, `mise exec -- make test_race`, and `mise exec -- make provider`; expect PASS/live tests skipped.
- [ ] Run `mise exec -- make check_openapi`, `mise exec -- make check_codegen`, `mise exec -- make build_sdks`, `mise exec -- make test_examples`, and `mise exec -- make docs_check`; expect PASS/no generated drift. Commit focused fixes and regenerate if necessary, then rerun affected checks. Report unavailable/failed checks explicitly rather than claiming success.
- [ ] Request the execution method's required independent review of the implementation branch against both spec and plan. Fix confirmed findings using TDD and rerun affected checks; never run live acceptance to resolve a review concern.
- [ ] Restore only the agent-owned temporary `.mise.toml` environment patch after all execution/verification is finished. Run `git diff --check`, `git status --short`, and inspect the branch diff without invoking mise again; distinguish any pre-existing changes.
- [ ] Report resource behavior, documentation locations, actual verification results, and the unverified live compatibility boundary.
- [ ] Ask whether to retain/delete this task's spec and plan. Do not remove either document or unrelated read-only lookup documents without explicit approval.
