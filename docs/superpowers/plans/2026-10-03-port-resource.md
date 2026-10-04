# Port Resource Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an independently managed, configuration-only Dokploy application Port resource, including typed SDKs, examples, and user documentation.

**Architecture:** Register `dokploy:index:Port` as an inferred Go resource backed by the four dedicated API operations. Preserve server-issued identity before create read-back, validate complete canonical observations, and never invoke application deployment. Generate the client, schema, SDKs, examples, and reference docs from their existing sources.

**Tech Stack:** Go, pulumi-go-provider/infer, oapi-codegen, Pulumi CLI, Go/Node.js/Python/.NET/Java SDK generation, Astro/Starlight, Node's test runner.

**Spec:** `docs/superpowers/specs/2026-10-03-port-resource-design.md` (approved).

## Global Constraints

- Token: `dokploy:index:Port`; required `applicationId`, `publishedPort`, `targetPort`; output-only `portId`.
- Both ports are integers in 1..65535. No automatic/random allocation.
- `protocol`: `tcp` (default), `udp`; `publishMode`: `ingress` (default), `host`. Default omitted inputs only; reject explicit empty/null enums.
- Only `applicationId` replaces the resource. Other inputs update in place; retain normal framework replacement ordering.
- Configuration-only create/update/delete: no deployment, redeployment, application mutation, registry testing, port discovery, availability checks, or firewall changes.
- Only explicit HTTP 404 or structured `NOT_FOUND` means absent; ambiguous HTTP 400 remains an error.
- Preserve acknowledged create identity in partial state; never retry mutations or automatically clean up uncertain creation outcomes.
- Keep IDs, private URLs, credentials, and arbitrary response prose out of diagnostics. Preserve cancellation/deadline classification.
- Keep the pinned upstream OpenAPI/hash unchanged. Regenerate generated files; inspect unrelated changes before destructive generation targets.
- All five SDKs, canonical/generated examples, README, Port guide, import/troubleshooting guidance, sidebar, and generated reference are required deliverables.
- Use `.mise.toml` versions, including Go `1.26.6` and Pulumi `3.259.0`. Do not introduce dependencies for this feature.
- No live acceptance tests, release/publication operations, or automatically run live fixtures. Offline tests do not establish deployed compatibility.
- After implementation, ask whether to retain/delete the task-specific spec and this plan; delete only with explicit approval.

## Review Focus

- An unrelated computed input must not hide a known invalid port or enum; Task 2 pins independent Check failures.
- Explicit null/empty enums must not become default TCP/ingress through inference coercion; Task 2 pins omission versus null/empty behavior.
- A valid create ID must survive an invalid unrelated response field or failing read-back; Task 3 pins minimal identity decoding and partial state.
- A successful update/delete must not fail merely because its unused response body cannot decode as Port; Task 3 pins acknowledgment-first mutation handling.
- Newly required references/examples must remain linked and generated, and explain deployment after deletion as well as creation; Task 4 pins publication and lifecycle wording.

---

## File map and boundaries

| Files | Responsibility |
| --- | --- |
| `openapi/operations.txt`, `openapi/corrections.json`, `openapi/cmd/normalize/main_test.go` | Select/correct the upstream Port API and test normalization |
| `openapi/dokploy.json`, `internal/client/generated/generated.gen.go` | Generated contract/client |
| `internal/client/port_contract_test.go` | Offline generated-client request/response contract |
| `provider/port.go`, `provider/port_test.go` | Public resource types, enum values, annotations, validation, Check/Diff/dependencies |
| `provider/port_lifecycle.go`, `provider/port_lifecycle_test.go` | Port-specific requests, canonical read-back, safe errors, CRUD/import/refresh |
| `provider/provider.go`, `provider/schema_test.go`, `provider/preview_dependencies_test.go` | Registration, metadata, exact schema and provider preview assertions |
| `provider/cmd/pulumi-resource-dokploy/schema.json`, `sdk/{go,nodejs,python,dotnet,java}/` | Generated public schema/SDKs |
| `examples/yaml/Pulumi.yaml`, `examples/yaml/README.md`, `examples/yaml_test.go` | Canonical Port example and assertions |
| `examples/{go,nodejs,python,dotnet,java}/` | Generated language examples |
| `README.md`, `website/src/content/docs/guides/{ports,applications,imports,troubleshooting}.mdx`, `website/src/content/docs/concepts/lifecycle-and-state.mdx` | Prose usage, import, lifecycle, and compatibility guidance |
| `website/scripts/reference-model.mjs`, `website/astro.config.mjs`, `website/tests/{reference-model,content}.test.mjs`, `website/tests/site-output.built-test.mjs` | Expected resource inventory, navigation, source/built documentation regression coverage |
| `website/src/content/docs/reference/{port,types}.mdx`, `website/src/content/docs/examples/complete.mdx` | Generated Port reference/types and complete example page |

No changes to existing `Application` ownership or resource lifecycle. Preserve any separately implemented read-only lookup work if it lands before execution; do not restore earlier hard-coded function/resource inventories.

## Execution safety prerequisite

- [ ] Establish implementation isolation using using-git-worktrees. Read `AGENTS.md`, `CONTRIBUTING.md`, `tests/README.md`, the spec, and this plan; inspect status. Do not read `.env` or print environment dumps.
- [ ] Because `.mise.toml` loads `.env` and enables acceptance, create an uncommitted `mise.opencode-safe.local.toml` in the implementation worktree only if the path is absent:

```toml
[env]
DOKPLOY_ACCEPTANCE = "0"
DOKPLOY_ENDPOINT = ""
DOKPLOY_API_KEY = ""
```

- [ ] Assert safe values in both outer and nested mise processes before any test:

```bash
MISE_ENV=opencode-safe mise exec -- python3 -c 'import os, subprocess; check="import os; assert os.environ.get(\"DOKPLOY_ACCEPTANCE\")==\"0\"; assert not os.environ.get(\"DOKPLOY_ENDPOINT\"); assert not os.environ.get(\"DOKPLOY_API_KEY\")"; exec(check); subprocess.run(["mise", "exec", "--", "python3", "-c", check], check=True)'
```

Expected: exit 0, no output. Stop if it fails; an outer environment override alone does not prove nested Makefile commands are safe.

- [ ] Prefix every execution command below with `MISE_ENV=opencode-safe mise exec --` (abbreviated **SAFE** below). Install missing pinned tooling under this environment only as needed. Keep the overlay until final checks finish; never stage it. Stage task files explicitly, not `git add .`.

### Task 1: Select and generate the typed Port API

**Files:** Modify `openapi/operations.txt`, `openapi/corrections.json`, `openapi/cmd/normalize/main_test.go`; create `internal/client/port_contract_test.go`; regenerate `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`.

**Interfaces:** Consumes `normalizeRealContract`, `responseSchema`, `componentSchema`, and the authenticated client. Produces `generated.Port` (`PortId string`; `ApplicationId *string`, `PublishedPort *int`, `TargetPort *int`, `Protocol *string`, `PublishMode *string`) and generated Port create/update/delete raw methods, WithResponse methods, request bodies, and `PortOneParams{PortId string}`. Upstream request numeric fields remain their generated number type; do not narrow requests to integers in OpenAPI.

- [ ] Write `TestPortOpenAPIContract` in the normalization tests. Assert GET-only `/port.one`, POST-only mutations, all four response references `#/components/schemas/Port`, required response ID, optional string/integer settings, unchanged selected operations plus exactly four new ones, required create fields, and no `applicationId` in update. Core assertions:

```go
d := normalizeRealContract(t)
require.Equal(t, "#/components/schemas/Port", responseSchema(t, d, "/port.one", "get", "200").Ref)
s := componentSchema(t, d, "Port")
require.Equal(t, []any{"portId"}, s["required"])
update := operationRequestSchema(t, d, "port.update")
require.NotContains(t, update["properties"], "applicationId")
```

- [ ] Run **SAFE** `go test ./openapi/cmd/normalize -run TestPortOpenAPIContract -count=1`. Expected: assertion failure because Port is not selected/corrected.
- [ ] Add `port.create`, `port.one`, `port.update`, `port.delete` to the allowlist; add a flat response model with only `portId` required and `additionalProperties:true`; correct all four responses to it. Run **SAFE** `make generate_openapi` after checking status.
- [ ] Write `TestPortGeneratedClientContract` against `httptest.Server`: create/update bodies carry all settings; update carries no application ID; GET encodes an opaque ID safely; Port response integers decode; omitted/null optional fields remain nil; fractional response numbers fail decoding; the nested application relation is ignored. Use the existing client test conventions and generated request signatures.
- [ ] Run **SAFE** `go test ./openapi/cmd/normalize ./internal/client/... -count=1`. Expected: all tests pass; existing API/authentication tests remain unchanged.
- [ ] Run `git diff --check`, inspect the generated diff, stage only the six task files, and commit as `feat: generate dedicated Port API client`. Then run **SAFE** `make check_openapi`; expected: exit 0 and no generated drift.

### Task 2: Define the resource contract, enums, validation, and diff

**Files:** Create `provider/port.go`, `provider/port_test.go`. Do not register an incomplete inferred resource yet.

**Interfaces:** Produces `PortProtocol string` with constants `PortProtocolTCP="tcp"`, `PortProtocolUDP="udp"`; `PortPublishMode string` with `PortPublishModeIngress="ingress"`, `PortPublishModeHost="host"`. Each implements the inference enum `Values() []infer.EnumValue[T]` API with descriptions and value names `Tcp`, `Udp`, `Ingress`, `Host`; annotate their tokens as `dokploy:index:PortProtocol` and `dokploy:index:PortPublishMode`. Produces `PortArgs{ApplicationID string, PublishedPort int, TargetPort int, Protocol PortProtocol, PublishMode PortPublishMode}`, `PortState{PortArgs; PortID string}`, `Port{client clientFactory}`, and `validatePortArgs(PortArgs) error`. Implement Annotate, `Check(context.Context, infer.CheckRequest) (infer.CheckResponse[PortArgs], error)`, `Diff(context.Context, infer.DiffRequest[PortArgs, PortState]) (infer.DiffResponse, error)`, and `WireDependencies(infer.FieldSelector, *PortArgs, *PortState)`.

- [ ] Write `TestPortCheckDefaults`, `TestPortCheckRejectsInvalidInputs`, `TestPortCheckDefersOnlyComputedFields`, `TestPortDiff`, and `TestValidatePortArgs`. For Check, use raw `property.Map` values so fractional numbers/nulls are tested before inference can coerce them. Base input assertions:

```go
checked, err := (Port{}).Check(t.Context(), infer.CheckRequest{NewInputs: property.NewMap(map[string]property.Value{
    "applicationId": property.New("a1"), "publishedPort": property.New(8081), "targetPort": property.New(80),
})})
require.NoError(t, err)
require.Empty(t, checked.Failures)
require.Equal(t, PortProtocolTCP, checked.Inputs.Protocol)
require.Equal(t, PortPublishModeIngress, checked.Inputs.PublishMode)
```

- [ ] Include table cases for missing/null/empty/whitespace application IDs; port values 1, 65535, 0, -1, 65536, 80.5, missing and null; omitted versus null/empty/invalid enums; both supported enum alternatives. When application ID is computed, `targetPort=0` still fails. When a port is computed, a known unsupported protocol still fails. Confirm an opaque nonempty application ID is not trimmed in output.
- [ ] Run **SAFE** `go test -short ./provider -run 'TestPort(Check|Diff)|TestValidatePortArgs' -count=1`. Expected: compile failure from absent resource contract.
- [ ] Implement the types and methods in `provider/port.go`. Mark `applicationId` replace-on-change, the enums optional with annotated defaults, and `portId` output-only. Validate raw numeric integrality/presence and enum null/empty presence before relying on `infer.DefaultCheck`; retain independent known-field failures. `validatePortArgs` accepts concrete complete inputs only, with explicit enums. Diff uses the spec's five fields and does not force delete-before-replace. Describe every input/output and each enum type/value.
- [ ] Wire `portId` to creation identity dependencies without making mutable setting changes turn an existing update ID into a computed output; Task 3's provider tests are the final authority on framework behavior.
- [ ] Run the same targeted command and **SAFE** `make test_provider`. Expected: pass; no registration/schema change yet.
- [ ] Run `git diff --check`, review the two files, and commit as `feat: define Port validation and update semantics`.

### Task 3: Implement safe lifecycle, register Port, and generate SDKs

**Files:** Create `provider/port_lifecycle.go`, `provider/port_lifecycle_test.go`; modify `provider/provider.go`, `provider/schema_test.go`, `provider/preview_dependencies_test.go`; regenerate schema and all five SDKs. Modify `provider/port.go` dependency wiring only if preview tests require it.

**Interfaces:** Consumes Task 1's generated methods and Task 2's resource contract. Produces `readPort(ctx context.Context, api *client.Client, id string) (PortState, error)`, `sanitizePortError(err error) error`, and `createPort(ctx context.Context, api *client.Client, args PortArgs) (string, error)`. Produces Port's inferred Create/Read/Update/Delete methods with the usual `PortArgs`/`PortState` request/response generics and resource registration. Use `initFailed` for acknowledged-create partial errors; classify `client.IsNotFound` on original errors before sanitization.

Exact lifecycle signatures (receiver `Port`):

```go
Create(context.Context, infer.CreateRequest[PortArgs]) (infer.CreateResponse[PortState], error)
Read(context.Context, infer.ReadRequest[PortArgs, PortState]) (infer.ReadResponse[PortArgs, PortState], error)
Update(context.Context, infer.UpdateRequest[PortArgs, PortState]) (infer.UpdateResponse[PortState], error)
Delete(context.Context, infer.DeleteRequest[PortState]) (infer.DeleteResponse, error)
```

- [ ] Write `TestPortCreateAndReadBack`, `TestPortImportAndRefresh`, `TestPortUpdateAndReadBack`, and `TestPortDelete` with strict scripted endpoint sequences. Base row: `{"portId":"p1","applicationId":"a1","publishedPort":8081,"targetPort":80,"protocol":"tcp","publishMode":"ingress"}`. Create expects POST then GET by `p1`; update expects POST then GET with a stable ID; Read/import expects GET only; delete expects POST only. Assert canonical observed settings override attempted mutable values, and import works with empty state.
- [ ] Write `TestPortReadRejectsInvalidResponses` with missing/null settings, empty owner, port 0/65536/80.5, unsupported protocol/mode, mismatched IDs, and malformed JSON. Add `TestPortMutationReadBackRejectsWrongOwner`; no wrong identity/owner may become state. Add `TestPortUpdateRejectsRetargeting` and concrete-invalid-input tests with a client factory that panics if constructed.
- [ ] Write `TestPortCreateRetainsAcknowledgedID`: a create body containing `{"portId":"p1","publishedPort":"invalid"}` followed by failed canonical GET returns `ID="p1"`, `Output.PortID="p1"`, attempted inputs, and `ResourceInitFailed`. Also test successful GET after that same create body. Missing/null/empty ID, invalid JSON, transport failure, or oversized body must not retry, search, delete, or return an invented identity.

```go
got, err := r.Create(t.Context(), infer.CreateRequest[PortArgs]{Inputs: args})
require.Error(t, err)
require.Equal(t, "p1", got.ID)
require.Equal(t, "p1", got.Output.PortID)
require.Equal(t, args, got.Output.PortArgs)
var partial infer.ResourceInitFailedError
require.ErrorAs(t, err, &partial)
```
- [ ] Write `TestPortUpdateFailureState` for mutation rejection retaining prior output versus acknowledged mutation/read-back failure retaining ID and attempted output. Write `TestPortMutationAcknowledgmentIgnoresUnusedBody` with malformed/empty update/delete response bodies; update still performs GET, delete succeeds without decoding the row. Confirm an acknowledged identity survives a response Close error with a custom transport/body fixture.
- [ ] Write `TestPortAbsenceAndErrors`: HTTP 404 and structured `NOT_FOUND` clear Read identity and make Delete succeed; HTTP 400 `BAD_REQUEST` “Port not found,” 401, and 403 return errors. Use one-attempt GET retry policy for fixtures that intentionally return retryable status codes. Write `TestPortDiagnosticsDoNotLeak` with fake IDs, URLs, and server prose; errors omit them but retain a safe operation/status. Assert `errors.Is` for canceled/deadline contexts.
- [ ] Run **SAFE** `go test -short ./provider -run TestPort -count=1`. Expected: compile failure or assertions because lifecycle methods do not exist.
- [ ] Implement lifecycle helpers/methods in `provider/port_lifecycle.go`. Raw create reads a bounded body (1 MiB maximum), minimally decodes only the identity, closes the body, and does not let unrelated field types erase that identity. Raw update/delete accept successful HTTP acknowledgment without decoding unused row bodies. Canonical GET validates complete settings and matching identity. Create/update additionally require the observed application to match inputs. No preliminary delete GET, mutation retries, deployment calls, or discovery.
- [ ] Implement safe errors by retaining context cancellation/deadline and API HTTP status only, wrapped with the fixed operation name; otherwise report a fixed request/response failure. Do not forward arbitrary API codes/prose or transport URLs. Validate resource IDs without echoing them. Preserve prior state on mutation rejection and attempted state after acknowledged read-back failure.
- [ ] Run the lifecycle tests; expected: pass with exactly the scripted requests. Add `TestSchemaPortContract` and `TestPortPreviewIdentity` before registration: assert `dokploy:index:Port`, integer ports, required input set, output-only ID, exact enum values/defaults, owner replacement only, descriptions, no secret flags, create computed ID/update known ID, and no preview client access. Run the tests; expected: schema/provider failures because Port is not registered.
- [ ] Register `infer.Resource(&Port{client: configuredClient})`, update metadata to include application ports, and extend—not replace—the existing resource/schema assertions. Fix dependency wiring if the integration preview test fails. Run **SAFE** `make codegen` after checking all SDK directories for unrelated changes.
- [ ] Run **SAFE** `go test -short ./provider -run 'TestPort|TestSchemaPortContract|TestSchemaHasExactlyTheMVPResources' -count=1`, **SAFE** `make test_provider`, and **SAFE** `make build_sdks`. Expected: pass. Review schemas/SDKs for the exact contract and no unrelated API removals.
- [ ] Run `git diff --check`, stage task sources and generated schema/SDKs, and commit as `feat: manage standalone Port resources`. Run **SAFE** `make check_codegen`; expected: no drift. Website inventory is updated in Task 4 before docs checks.

### Task 4: Publish docs/examples and verify the complete change

**Files:** Modify `README.md`, `examples/yaml/Pulumi.yaml`, `examples/yaml/README.md`, `examples/yaml_test.go`; create `website/src/content/docs/guides/ports.mdx`; modify applications/imports/troubleshooting guides and lifecycle concept page; modify `website/scripts/reference-model.mjs`, `website/astro.config.mjs`, `website/tests/reference-model.test.mjs`, `website/tests/content.test.mjs`, `website/tests/site-output.built-test.mjs`; regenerate language examples and website reference/example pages.

**Interfaces:** Consumes the committed schema and generated SDKs from Task 3. Produces documented routes `/guides/ports/`, `/reference/port/`, canonical YAML `applicationPort`, and equivalent generated examples. Existing Schedule and other references/examples remain present.

- [ ] Add `TestCanonicalYAMLPort` in `examples/yaml_test.go`; assert one `dokploy:index:Port` with `applicationId=${application.applicationId}`, `publishedPort=8081`, `targetPort=80`, `protocol=tcp`, `publishMode=ingress`, and no deployment property. Update existing expected resource count from 24 to 25 and add Port count 1, adjusting for any independently landed example work instead of deleting it. Run **SAFE** `go test ./examples -tags=yaml -run TestCanonicalYAMLPort -count=1`; expected: missing Port assertion failure.
- [ ] Add the canonical resource and deployment comment; update `examples/yaml/README.md` to explain configuration-only create/update/delete and explicit application deployment. Run **SAFE** `make gen_examples` after checking every example directory for unrelated work; do not manually edit converted output. Run **SAFE** `go test ./examples -tags=yaml -run 'TestCanonicalYAMLPort|TestCanonicalYAMLUsesGeneratedSchema' -count=1`; expected: pass.
- [ ] Add Node tests named `Port reference and guide are published`, `Port guide explains configuration-only lifecycle and import limitations`, and `Port canonical and generated examples are present` in `website/tests/content.test.mjs`. Assert both new sidebar links, generated Port reference properties/defaults, canonical/YAML and complete-page `applicationPort`, ID-only import syntax, 1..65535 bounds, ingress/host wording, no automatic deploy/redeploy, deployment after deletion, Compose exclusion, firewall/availability caveat, and ambiguous 400 versus explicit absence. Add a real-schema Port assertion to `reference-model.test.mjs` (current count 19 becomes 20; preserve newer inventory changes if present).
- [ ] Run **SAFE** `node --test website/tests/content.test.mjs website/tests/reference-model.test.mjs`. Expected: new tests fail because the guide/reference/sidebar/inventory are absent. The existing strict inventory also rejects the newly added schema until updated.
- [ ] Add Port to `EXPECTED_RESOURCES` in `website/scripts/reference-model.mjs`. Add `Ports` under Guides and `Port` under Resources in `website/astro.config.mjs`. Write `ports.mdx`, linking the generated reference; link it from applications, update import/troubleshooting pages, and clarify configuration-only semantics in the lifecycle concept page and README. Preserve existing automatic Mount redeployment wording and Schedule support. Remove stale resource-count prose instead of introducing another fragile count. Generated enum type descriptions must state their allowed values, since the current type renderer does not render enum-value tables; no general renderer rewrite is needed.
- [ ] Include the literal documentation command `pulumi import dokploy:index:Port mapping <port-id>` in a fenced code block. Explain that a 400 “Port not found” may mean lost permission, so refresh does not discard state; external application deletion can therefore need operator investigation on the pinned version. Explain the application relation separately from Pulumi parent and avoid implying host ports are available or firewalls are configured.
- [ ] Run **SAFE** `make docs_generate`; inspect generated `reference/port.mdx`, `reference/types.mdx`, and `examples/complete.mdx`. Run the Node source tests again; expected: pass. Add built-site assertions for `reference/port/index.html` and `guides/ports/index.html` to `website/tests/site-output.built-test.mjs`.
- [ ] Run **SAFE** `make build_sdks`, **SAFE** `make test_examples`, and **SAFE** `npm --prefix website run check` followed by **SAFE** `npm --prefix website run build` and **SAFE** `npm --prefix website run test:built`. Expected: exit 0. Review all generated language samples; they must not claim Port deployment happens automatically.
- [ ] Run `git diff --check`, review source/generated docs/examples, explicitly stage the Task 4 files, and commit as `docs: document Port configuration and regenerate examples`.
- [ ] Re-run the outer/nested safety assertion. Run each of the following with the full **SAFE** prefix: `make lint`, `make test_provider`, `make test_race`, `make provider`, `go test ./openapi/cmd/normalize -count=1`, `make check_openapi`, `make check_codegen`, `make build_sdks`, `make test_examples`, `make docs_check`. Expected: exit 0, no generation drift, and live acceptance skipped. Do not treat missing tooling or unrun checks as passed; record blockers precisely. Run `git diff --check`, inspect final status and the complete branch diff against the implementation base.
- [ ] Perform the selected workflow's final review before integration. Report actual validation results and the upstream refresh limitation; request approval before any push/merge. Ask whether to retain the Port spec and plan after implementation, preserving unrelated read-only lookup documents.

## Plan self-review

- Spec coverage: Task 1 owns upstream correction; Task 2 owns input/public contract; Task 3 owns lifecycle, errors, previews, registration, and SDK generation; Task 4 owns every documentation/example requirement and final checks.
- All five Review Focus items have named regression tests in their owning task.
- Types and property names are shared through Interfaces blocks; generated request number conversions are confined to the request boundary. Port helpers do not depend on a new cross-resource abstraction.
- Safety override is checked for nested mise before any test. No execution or live verification is performed while authoring this plan.
- Implementation begins only after plan review and execution-method selection.
