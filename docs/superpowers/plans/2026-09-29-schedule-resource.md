# Schedule Resource Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a safe, refreshable Pulumi Schedule resource for all four Dokploy schedule types, including SDKs, tests, and docs.

**Architecture:** Extend the allowlisted OpenAPI contract with corrected Schedule responses, generate its typed client, then implement infer-based Check/Diff/CRUD against that client. Generate schema and all SDKs from provider source; generate website reference docs from schema. Treat the response contract and caller-generated ID acceptance as unverified until tested on a dedicated server.

**Tech Stack:** Go 1.26.6, pulumi-go-provider/infer, oapi-codegen v2.8.0, `github.com/google/uuid`, Pulumi CLI 3.259.0, mise, Astro/Starlight.

**Spec:** `docs/superpowers/specs/2026-09-29-schedule-resource-design.md`

## Global Constraints

- Resource token: `dokploy:index:Schedule`; schedule types: `application`, `compose`, `server`, `dokploy-server`; shell types: `bash`, `sh`.
- `enabled` defaults to `false`; `command` and `script` are secret; target identity changes replace, editable settings update.
- Do not call `schedule.runManually` in resource lifecycle; do not silently adopt a schedule identified by name.
- Create uses provider-generated `scheduleId` and read-back verification; no blind create retries or deletion of unverified resources.
- Never run live acceptance routinely or against production; use `tests/README.md` for any separately approved dedicated-server run.
- Inspect uncommitted generated outputs before `make generate_openapi`, `make check_openapi`, or `make codegen`; preserve all unrelated user changes and secrets.
- The current OpenAPI Schedule response shapes and acceptance of caller-supplied IDs are unverified; report this limitation after offline checks.

## Review Focus

1. API `200` with `null`, `{}`, or a wrong `scheduleId` must not become an imported/owned resource (Task 3 read-back cases).
2. A successful create followed by failed read-back must retain its generated ID for recovery without automatic deletion or a second create (Task 3 partial-state case).
3. Explicit `null` optional fields versus omitted fields on refresh must clear versus preserve prior state (Task 3 refresh cases).
4. Server error text reflecting `command`, `script`, or generated ID must not leak them (Task 3 error cases).
5. Preview with unknown type or target must defer cross-field validation, while a known conflicting target fails before API calls (Task 2 computed cases).

---

## File structure

- `openapi/operations.txt`, `openapi/corrections.json`: allowlist and typed response assumptions; `openapi/cmd/normalize/main_test.go`: correction contract tests. Generated: `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`.
- `provider/schedule.go`: Schedule args/state, annotations, validation, diff, request mapping, CRUD, secret-safe errors; `provider/schedule_test.go`: isolated mock lifecycle tests. `provider/provider.go`, `provider/schema_test.go`: registration and schema tests. For clarity, split mapping/lifecycle into `provider/schedule_lifecycle.go` if `schedule.go` becomes unwieldy; maintain the same exported interfaces.
- `provider/live_schedule_test.go`: optional Tier 2 fixtures and cleanup; `provider/live_workloads_test.go`: subtest integration if useful; `tests/README.md`: any additional prerequisite/safety guidance.
- `provider/cmd/pulumi-resource-dokploy/schema.json`, `sdk/{go,nodejs,python,dotnet,java}`: generated only. `examples/yaml/Pulumi.yaml`, `website/astro.config.mjs`, `website/src/content/docs/guides/schedules.mdx`, generated `website/src/content/docs/reference/schedule.mdx` and `website/src/content/docs/examples/complete.mdx`: example and docs.

### Task 1: Typed Schedule API contract

**Files:** Modify `openapi/operations.txt`, `openapi/corrections.json`; test `openapi/cmd/normalize/main_test.go`; generate `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`.

**Interfaces:** Produces `generated.ScheduleCreateJSONRequestBody`, `ScheduleUpdateJSONRequestBody`, `ScheduleDeleteJSONRequestBody`, `ScheduleOneParams`, `Schedule` (component model), and methods `ScheduleCreateWithResponse`, `ScheduleOneWithResponse`, `ScheduleUpdateWithResponse`, `ScheduleDeleteWithResponse` on `client.Client`. Confirm actual generated spelling before Task 2; if it differs, update this Interfaces block. `schedule.one` must decode a Schedule object with optional fields and mandatory `scheduleId`; mutation response corrections should accept either a bodyless success or an object only when supported by typed client behavior. No list or runManually operation is needed.

- [ ] **Step 1: Write failing contract test.** In `main_test.go`, `TestRealContractIncludesScheduleCRUD` calls `normalizeRealContract(t)`, asserts create/update/delete/one paths and HTTP methods, asserts `schedule.one` response `$ref` is `#/components/schemas/Schedule`, `Schedule.required` includes `scheduleId`, `additionalProperties == true`, and create/update request schemas include `scheduleId`, `name`, `cronExpression`, `command`, `enabled`. Assert `schedule.list` and `schedule.runManually` absent.
- [ ] **Step 2: Verify red.** Run `mise exec -- go test ./openapi/cmd/normalize -run TestRealContractIncludesScheduleCRUD -count=1`; expect missing path/component failure.
- [ ] **Step 3: Update contract sources.** Allowlist exactly `schedule.create`, `schedule.update`, `schedule.delete`, `schedule.one`. Define `Schedule` with fields named in the spec (including nullable optional fields) and `additionalProperties: true`; correct `schedule.one` response to `Schedule`. Set mutation responses to bodyless only if `WithResponse` handles empty 200; do not invent create IDs in a response schema. Record the assumed response shape in a nearby comment/test name if useful.
- [ ] **Step 4: Generate and verify.** Before regeneration check `git status --short -- openapi/dokploy.json internal/client/generated/generated.gen.go`. Run `make generate_openapi`, then `mise exec -- go test ./openapi/cmd/normalize -run TestRealContractIncludesScheduleCRUD -count=1`; expect PASS. Inspect expected generated diff; `make check_openapi` may report expected drift until generated outputs are committed.
- [ ] **Step 5: Commit.** `git add openapi/operations.txt openapi/corrections.json openapi/cmd/normalize/main_test.go openapi/dokploy.json internal/client/generated/generated.gen.go` then `git commit -m "feat: add typed Schedule API contract"`.

### Task 2: Schedule schema, validation, and diff

**Files:** Create `provider/schedule.go`, `provider/schedule_test.go`; modify `provider/provider.go`, `provider/schema_test.go` (inspect current contents first).

**Interfaces:** Consumes generated request/model types from Task 1. Produces `ScheduleArgs` (`Name`, `CronExpression`, `Command`, `ScheduleType` as strings; `Description`, `AppName`, `ServiceName`, `ShellType`, `Script`, `Timezone`, `OrganizationID`, `ApplicationID`, `ComposeID`, `ServerID` as `*string`; `Enabled bool`), `ScheduleState` embedding `ScheduleArgs` plus `ScheduleID string`, `Schedule{client clientFactory}`, and `Check(context.Context, infer.CheckRequest) (infer.CheckResponse[ScheduleArgs], error)`, `Diff(context.Context, infer.DiffRequest[ScheduleArgs, ScheduleState]) (infer.DiffResponse, error)`.

- [ ] **Step 1: Write failing tests.** `TestScheduleSchemaSurface` asserts token, input descriptions, `enabled` default `false`, `command`/`script` Secret, and target identity ReplaceOnChanges. Add `Schedule` to expected resource tokens and metadata description assertions without discarding unrelated edits. `TestScheduleCheckTypesAndTargets` table-tests all four type strings, invalid type, empty name/cron/command, incompatible target IDs, absent relevant target ID where target-specific requirements are documented, and invalid shell type; do not require a target field not mandated by upstream schema without evidence. `TestScheduleCheckDefersComputedTargets` uses `property.Computed` to assert no false failures. `TestScheduleDiffReplacementAndUpdates` asserts target changes `p.UpdateReplace` and `DeleteBeforeReplace`, editable changes `p.Update`, no changes when equal. Choose and test how `appName`/`serviceName` affect target identity, documenting the choice.
- [ ] **Step 2: Verify red.** Run `mise exec -- go test ./provider -run 'TestSchedule(SchemaSurface|CheckTypesAndTargets|CheckDefersComputedTargets|DiffReplacementAndUpdates)' -count=1`; expect missing types/token or failing assertions.
- [ ] **Step 3: Implement smallest schema/check/diff surface.** Add `ScheduleArgs`/`ScheduleState` with exact Pulumi tags; `provider:"secret"` on command/script, `provider:"replaceOnChanges"` on target identity; annotate every field and `enabled` default `false`. Register `Schedule` in `Provider()`, update metadata. `Check` uses `infer.DefaultCheck`, skips cross-field checks for computed relevant inputs, validates known values only; `Diff` uses existing `sameOptionalString`/`hasReplacement` helpers and returns delete-before-replace for target changes. Keep the code in `schedule.go` until lifecycle size warrants splitting.
- [ ] **Step 4: Verify green.** Run the same focused command; expect PASS.
- [ ] **Step 5: Commit.** `git add provider/schedule.go provider/schedule_test.go provider/provider.go provider/schema_test.go` then `git commit -m "feat: define Schedule resource schema and validation"`.

### Task 3: Schedule CRUD, recovery, and secrecy

**Files:** Modify `provider/schedule.go`, `provider/schedule_test.go`; optionally create `provider/schedule_lifecycle.go` if separating lifecycle from validation improves readability.

**Interfaces:** Consumes Task 2 `ScheduleArgs`, `ScheduleState`, `Schedule{client clientFactory}`. Produces infer Create/Read/Update/Delete methods matching other resources, plus unexported `scheduleCreateBody(id string, args ScheduleArgs) generated.ScheduleCreateJSONRequestBody`, `scheduleUpdateBody(id string, args ScheduleArgs) generated.ScheduleUpdateJSONRequestBody`, `scheduleArgsFrom(v *generated.Schedule, prior ScheduleArgs) (ScheduleArgs, error)` and `sanitizeScheduleError(err error, args ScheduleArgs, prior ...ScheduleArgs) error`. Confirm generated `Schedule` field types before defining conversion internals.

- [ ] **Step 1: Write failing tests.** `TestScheduleCreateAndImportAllTypes` table-tests four request bodies to `/api/schedule.create`, the generated UUID in `scheduleId`, `enabled:false`, then GET `/api/schedule.one?scheduleId=<generated>` with matching object and refreshed state; inject an ID generator or assert UUID structure and capture the ID using a focused test server that never prints body values. `TestScheduleCreateDryRunNoCalls` verifies no network. `TestScheduleCreateReadbackFailuresKeepPartialID` table-tests GET not-found, null/empty JSON, mismatched ID, and transport error: no adoption, no second POST or delete, `infer.ResourceInitFailedError` with partial ID/state where a create was acknowledged. `TestScheduleReadDriftNullAbsentAndNotFound` tests import with empty prior state, known drift, explicit null versus omitted optional values, malformed 200, and 404. `TestScheduleUpdateAndDelete` asserts full edit payload, same-ID read-back, preview/no network, delete-by-ID and missing-as-success. `TestScheduleErrorsAreRedacted` injects server errors containing command/script/generated ID into each phase and asserts no secrets or IDs in returned errors; use placeholders only, not real values.
- [ ] **Step 2: Verify red.** Run `mise exec -- go test ./provider -run 'TestSchedule(Create|Read|Update|Delete|Errors)' -count=1`; expect missing lifecycle methods or failing cases.
- [ ] **Step 3: Implement create/read.** Use `uuid.NewString()` (already in `go.mod`); map nullable inputs deliberately (omitted vs explicit null), always transmit `enabled` explicitly, call create once, confirm with one by exact ID, and return partial state with generated ID on acknowledged-create read-back failure. The generated client may decode a bodyless success differently: test actual typed behavior rather than requiring an invented create response. Reject any one response whose `scheduleId` differs from requested ID; return not-found only on `client.IsNotFound`. Do not include ID or raw payload in an error.
- [ ] **Step 4: Implement update/delete and safe errors.** Update with full specified editable body and re-read exact ID; delete by ID and treat 404 as absent. Sanitize `command`, `script`, and IDs from all returned API errors, including prior state and explicit APIError fields; avoid logging payloads. For transport errors that can include private endpoint URLs, use safe classification rather than forwarding raw URLs. Never auto-delete a created schedule after uncertain read-back.
- [ ] **Step 5: Verify green.** Run the focused command and `mise exec -- go test ./provider -short -count=1`; expect PASS without `DOKPLOY_ACCEPTANCE=1` tests running (the live tests require separate explicit invocation).
- [ ] **Step 6: Commit.** `git add provider/schedule.go provider/schedule_lifecycle.go provider/schedule_test.go` (include only files created) then `git commit -m "feat: manage Schedule lifecycle safely"`.

### Task 4: Optional live acceptance coverage (code only)

**Files:** Modify `provider/live_workloads_test.go`, `tests/README.md` if a new opt-in or prerequisite is needed; add `provider/live_schedule_test.go` if a separate fixture keeps workload test readable.

**Interfaces:** Consumes Task 3 `Schedule` CRUD and existing `liveClient`, `liveContext`, `liveCleanupVerified` helpers; no production-facing exported symbols. Live tests must remain gated by `DOKPLOY_ACCEPTANCE=1` and the dedicated-server procedure.

- [ ] **Step 1: Write a failing, offline-safe harness/unit test** (e.g. `TestScheduleLiveFixtureDisabledAndCleanupOrdered`) around a small fixture helper using injected create/read/delete functions: assert created schedules default disabled, cleanup ownership is registered immediately after create acknowledgment, deletion/absence happens before target cleanup, and an absent prerequisite results in a skip. Do not run any live test to establish red.
- [ ] **Step 2: Verify red.** Run `mise exec -- go test ./provider -short -run TestScheduleLiveFixtureDisabledAndCleanupOrdered -count=1`; expect missing helper/failing assertion.
- [ ] **Step 3: Implement optional acceptance fixture.** Add a Schedule subtest to Tier 2 on disposable application and Compose IDs, with explicit `enabled:false`; include server/dokploy-server cases only with dedicated test scope and an explicit safe opt-in, otherwise skip with documented reason. Register fallback cleanup when the provider returns an acknowledged-create ID (including partial-state errors), delete/verify absence before target cleanup, and never report unsanitized IDs, commands, response bodies, or private URLs. Document any new prerequisite in `tests/README.md`. If the fixture cannot safely guarantee ownership on create uncertainty, stop instead of testing that branch live.
- [ ] **Step 4: Verify green offline only.** Run the focused harness/unit command; expect PASS. Do not run `TestLiveTier2Workloads` locally or against production.
- [ ] **Step 5: Commit.** Stage only live-fixture code and `tests/README.md`, then `git commit -m "test: add gated Schedule acceptance fixture"`.

### Task 5: Generated SDKs, example, and docs

**Files:** Modify `examples/yaml/Pulumi.yaml`, `website/astro.config.mjs`; create `website/src/content/docs/guides/schedules.mdx`; generate `provider/cmd/pulumi-resource-dokploy/schema.json`, `sdk/go/dokploy/**`, `sdk/nodejs/**`, `sdk/python/**`, `sdk/dotnet/**`, `sdk/java/**`, `website/src/content/docs/reference/schedule.mdx`, `website/src/content/docs/examples/complete.mdx`; test `website/tests/*.test.mjs` relevant generator output.

**Interfaces:** Consumes registered Task 2/3 Schedule resource; produces generated SDK type `Schedule` (language-specific names per Pulumi), reference page `/reference/schedule/`, and a disabled YAML Schedule sample attached to the example application. No hand-editing generated SDK/schema/reference files.

- [ ] **Step 1: Write failing generation/docs assertions.** Add a website test asserting Schedule reference presence, sidebar navigation, and a YAML sample with `type: dokploy:index:Schedule`, `scheduleType: application`, `applicationId: ${application.applicationId}`, `enabled: false`, and `command` wrapped with `fn::secret`. Assert the guide warns that enabled schedules execute commands. Schema properties are already covered by Task 2's `TestScheduleSchemaSurface`.
- [ ] **Step 2: Verify red.** Run `npm --prefix website test`; expect the new Schedule reference/sample assertion to fail.
- [ ] **Step 3: Add source example/guide/navigation.** Edit YAML and guide, link `/guides/schedules/` and `/reference/schedule/` from Astro sidebar. Keep sample disabled and avoid actual credentials. Ensure docs explicitly call out the unverified live contract until validated.
- [ ] **Step 4: Regenerate, then verify.** Check `git status --short -- provider/cmd/pulumi-resource-dokploy/schema.json sdk website/src/content/docs/reference website/src/content/docs/examples/complete.mdx` before regeneration. Run `make codegen`, `make docs_generate`, `npm --prefix website test`. Inspect generated diff for unrelated churn and expected five SDKs; no manual edits to generated output. `make check_codegen` expects generated outputs committed before its `git diff --exit-code -- sdk` subcheck; rerun after committing if necessary.
- [ ] **Step 5: Commit.** Stage only source, generated schema/SDKs/docs, and tests; `git commit -m "docs: publish Schedule SDKs and examples"`.

### Task 6: Integration verification and handoff

**Files:** No new code unless a failing check requires a focused fix and test. Any fix should be its own reviewed change/commit.

**Interfaces:** Consumes all preceding tasks; produces a validation report distinguishing offline checks from live compatibility, preserving unrelated changes.

- [ ] **Step 1: Run provider and OpenAPI checks.** `make lint`, `make test_provider`, `make test_race`, `make provider`, `make check_openapi`; expect zero exit (first `make check_openapi` may reveal expected uncommitted generated drift: commit those before treating it as passed).
- [ ] **Step 2: Run SDK/examples/docs checks.** `make check_codegen`, `make build_sdks`, `make test_examples`, `make docs_check`; expect zero exit, with exact failures/omissions reported. Never invoke live test targets; `make test_provider` uses `-short`.
- [ ] **Step 3: Inspect working tree.** Run `git diff --check`, `git status --short`, `git diff --stat`, and relevant `git diff`/`git show` output; check that regenerated output matches source and unrelated user changes remain intact.
- [ ] **Step 4: Record limitation and cleanup choice.** State that Swagger was 403 and provider-generated ID acceptance, body shapes, and server-type semantics remain unverified without dedicated-server acceptance. Identify this spec and plan as task-specific SDD documents and ask whether to retain or delete them; do not delete without approval.
