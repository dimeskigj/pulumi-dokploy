# Notification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship one typed Notification resource for all twelve Dokploy channels, with guarded lifecycle ownership, protected credentials, generated SDKs, and updated documentation.

**Architecture:** Infer a single resource with exactly one channel configuration block and shared event settings. Explicit generated-client adapters handle channel-specific payloads; common observation and lifecycle helpers verify organization/notification/channel identity and preserve trustworthy partial state. Creation uses a random disabled temporary record because the pinned API returns no ID.

**Tech Stack:** Go 1.26.6, pulumi-go-provider 1.6.0, Pulumi 3.259.0, oapi-codegen 2.8.0, existing testify/local HTTP tests, Node.js 24.18.0, Astro/Starlight documentation tooling, and all five existing SDK generators.

**Spec:** `docs/superpowers/specs/2026-10-03-notification-design.md` (approved; commit `b42b465`). Executors read the spec and repository guidance before starting.

## Global Constraints

- Manage Dokploy destinations/events, not notifications emitted by Pulumi or provider configuration.
- Token `dokploy:index:Notification`; exactly one of `slack`, `telegram`, `discord`, `email`, `resend`, `gotify`, `ntfy`, `mattermost`, `custom`, `lark`, `teams`, `pushover`.
- Shared events: `appDeploy`, `appBuildError`, `databaseBackup`, `volumeBackup`, `dokployBackup`, `dokployRestart`, `dockerCleanup`, `serverThreshold`; all default false.
- No input discriminator, `enabled`, `organizationId`, notification ID, or channel ID. Required outputs: `notificationId`, `notificationType`, `channelId`, `organizationId`.
- Follow the spec's exact channel properties/defaults: optional routing strings empty; decoration false; Gotify priority 5, Ntfy 3, Pushover 0; custom headers empty map; Pushover retry/expire absent.
- Reject desired Gotify/Ntfy `serverThreshold:true`; refuse Update if observed true would need clearing through their unsupported endpoint. Delete remains available.
- Same-channel settings/credential changes update in place; switching selected channel replaces. Do not annotate entire channel blocks with unconditional replace-on-change.
- IDs are opaque and must not be rewritten or printed in diagnostics.
- No API calls in create/update previews; no testConnection, receiveNotification, or getEmailProviders calls anywhere.
- Create exactly once with an unpredictable temporary name and all events false; discover and verify before final update. No name-only adoption, uncertain-create replay, or automatic failure deletion.
- Preserve list-verified identity/state through read-back/finalization failures; never fabricate identity or claim unobserved settings were applied.
- Fresh identity verification precedes update/delete. Do not independently delete channel records or change upstream cascade behavior.
- Nested credentials, webhookUrl, custom endpoint/headers, and Gotify/Ntfy serverUrl are secret. Never forward response prose, bodies, decoder snippets, URLs, or IDs in errors.
- Use pin `cebd3808565ea9bed0791961bc25c60513d94c5a`; no pin update, new dependency, unrelated refactor, or implementation of the separately planned lookup functions.
- Change sources and regenerate; inspect uncommitted SDK/OpenAPI/docs output first and never overwrite user work.
- Documentation is required for completion, including generated references, navigation, guide, README/import/security guidance, examples, and CHANGELOG.
- No live tests, real messages, release/publication workflows, or credentials in source/logs. Use the safe environment prerequisite below.

## Review Focus

1. An unknown nested channel block alongside a known block must not be discarded/defaulted or falsely accepted as definitely absent (Tasks 2 and 6).
2. Header keys containing dots/brackets and different map iteration orders must round-trip and diff correctly without leaking values (Tasks 2, 3, and 7).
3. A create response with acknowledged 2xx headers and an unreadable/truncated body must not lose success or replay the POST (Task 5).
4. A unique random marker whose record has wrong settings/organization is a conflict, not an empty result eligible for retries or adoption (Tasks 4 and 5).
5. A changed session organization or substituted channel relation between operations must not cause a mutation against the newly observed object (Tasks 4 and 5).

---

## File map and shared interfaces

| File(s) | Responsibility |
| --- | --- |
| `openapi/operations.txt`, `openapi/corrections.json` | Selected notification endpoints and accurate response schemas |
| `openapi/cmd/normalize/main_test.go`, `internal/client/notification_contract_test.go` | Normalization and generated decoding contracts |
| `provider/notification.go` | Args/state/events, resource annotation, Check/Diff, dependency wiring |
| `provider/notification_config_webhook.go` | Slack, Discord, Mattermost, Custom, Lark, Teams public config types/descriptions |
| `provider/notification_config_messaging.go` | Telegram, Gotify, Ntfy, Pushover public config types/descriptions |
| `provider/notification_config_email.go` | Email and Resend public config types/descriptions |
| `provider/notification_validation.go` | Resolved normalization/validation, known-value Check support, nested diff helpers |
| `provider/notification_channel_webhook.go`, `provider/notification_channel_messaging.go`, `provider/notification_channel_email.go` | Explicit typed request builders and channel response projections |
| `provider/notification_channel.go` | Small explicit dispatch and raw response closing helpers |
| `provider/notification_observation.go` | Active organization, list validation, response identity/projection, candidate verification |
| `provider/notification_lifecycle.go` | Create/Read/Update/Delete and safe error boundary |
| `provider/notification_test_helpers_test.go` | Notification-only local server, fixtures, non-sensitive assertions |
| Matching `*_test.go` files; `provider/notification_schema_test.go`, `provider/notification_inference_test.go` | Unit, lifecycle, schema, and engine-level regression coverage |
| `examples/go/notification_test.go` | Compiled SDK usage with Pulumi mocks, not a live program |
| Existing website model/renderer/generator, navigation/content/tests | Reference map rendering, Notification references, user guide and discoverability |

Generated output: `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`, `provider/cmd/pulumi-resource-dokploy/schema.json`, all `sdk/` language directories, `website/src/content/docs/reference/notification.mdx`, and shared reference type pages. Do not hand-edit them.

Use these exact shared names/signatures so task boundaries agree:

```go
type Notification struct { client clientFactory }
type NotificationArgs struct {
    Name string
    Events *NotificationEvents
    // Twelve *Notification<Channel>Config fields, with optional Pulumi tags.
}
type NotificationState struct {
    NotificationArgs
    NotificationID, NotificationType, ChannelID, OrganizationID string
}
type notificationIdentity struct {
    NotificationID, NotificationType, ChannelID, OrganizationID string
}
func notificationKind(a NotificationArgs) (string, error)
func normalizeNotificationArgs(a NotificationArgs) NotificationArgs
func validateNotificationArgs(a NotificationArgs) []p.CheckFailure
func createNotificationChannel(ctx context.Context, api *client.Client, name string, a NotificationArgs) error
func updateNotificationChannel(ctx context.Context, api *client.Client, id, channelID string, a NotificationArgs) error
func notificationMutationResult(resp *http.Response, err error) error
func notificationStateFrom(v *generated.Notification, prior *NotificationState, expected notificationIdentity) (NotificationState, error)
func notificationListIDs(v []generated.Notification) (map[string]struct{}, error)
func notificationCandidate(v *generated.Notification, marker, orgID string, desired NotificationArgs, before map[string]struct{}) (NotificationState, bool, error)
func notificationActiveOrganizationID(ctx context.Context, api *client.Client) (string, error)
func readNotification(ctx context.Context, api *client.Client, expected notificationIdentity, prior *NotificationState) (NotificationState, error)
func sanitizeNotificationError(err error) error
```

The snippets specify interfaces, not final source: add Pulumi/provider tags, imports, annotations, and comments. `notificationCandidate` returns `(zero,false,nil)` only for an unrelated marker; a matching marker with invalid identity/configuration returns an error. Task 4 owns the observation interfaces, Task 3 owns request dispatch, Task 5 owns sanitization/lifecycle.

## Execution safety prerequisite

- [ ] Establish implementation isolation with using-git-worktrees. Inspect status and read `AGENTS.md`, `CONTRIBUTING.md`, `tests/README.md`, `.mise.toml`, the spec, and this plan. Do not read `.env` or print credential variables.
- [ ] Create `mise.opencode-safe.local.toml` in that worktree only if the path does not already exist, with this task-local uncommitted overlay:

```toml
[env]
DOKPLOY_ACCEPTANCE = "0"
DOKPLOY_ENDPOINT = ""
DOKPLOY_API_KEY = ""
```

- [ ] Verify outer and nested mise environment selection before any test/build/generation:

```bash
MISE_ENV=opencode-safe mise exec -- python3 -c 'import os, subprocess; check="import os; assert os.environ.get(\"DOKPLOY_ACCEPTANCE\")==\"0\"; assert not os.environ.get(\"DOKPLOY_ENDPOINT\"); assert not os.environ.get(\"DOKPLOY_API_KEY\")"; exec(check); subprocess.run(["mise", "exec", "--", "python3", "-c", check], check=True)'
```

Expected: exit 0 with no output. If not, stop and resolve the safe environment; an outer `env DOKPLOY_ACCEPTANCE=0` alone does not protect nested mise invocations.

- [ ] Every execution command below uses `MISE_ENV=opencode-safe mise exec --`. Install missing pinned tools only as needed with this profile. Keep the overlay until all validation ends; never stage it. Stage explicit task files, never `git add .`.

## Task 1: Generate accurate notification API contracts

**Files:** Modify `openapi/operations.txt`, `openapi/corrections.json`, `openapi/cmd/normalize/main_test.go`; create `internal/client/notification_contract_test.go`; regenerate OpenAPI/client output.

**Interfaces:** Produces generated `Notification`, `NotificationList`, and twelve relation types named `NotificationSlack`, `NotificationTelegram`, etc., plus raw create/update/remove calls and `NotificationOneWithResponse`, `NotificationAllWithResponse`. Generated field casing follows oapi-codegen conventions.

- [ ] Write `TestRealContractIncludesNotificationCRUD` using `normalizeRealContract`: assert exactly 27 new notification operations (12 create, 12 update, one/all/remove), correct methods, one/all response references, bodyless create/update corrections, and unchanged pin/existing selections. Example core assertions:

```go
doc := normalizeRealContract(t)
require.Contains(t, doc.Paths, "/notification.one")
require.NotNil(t, doc.Paths["/notification.one"].Get)
require.Equal(t, "#/components/schemas/Notification", responseSchema(t, doc, "/notification.one", "get", "200").Ref)
require.NotContains(t, doc.Paths, "/notification.testSlackConnection")
require.Equal(t, []any{"notificationId"}, operationRequestSchema(t, doc, "notification.remove")["required"])
```

- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./openapi/cmd/normalize -run TestRealContractIncludesNotificationCRUD -count=1`; expect missing-path/reference failures.
- [ ] Select the 27 operations and correct one/all schemas. Map create/update/remove responses to empty corrections; keep request schemas pinned, including required nested channel IDs on updates. Model inactive relations and nullable optional settings; every consumed field retains omission/null presence as required by the spec. Include all eight event booleans and organization/foreign-key identity.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make generate_openapi`; inspect generated types and null handling before downstream code.
- [ ] Write `TestNotificationGeneratedDecoding` with all twelve nested relation fixtures, null inactive relations, omitted versus null secrets/settings, custom string-map headers, false events, and Pushover nullable numeric fields. Assert generated one/list decoding can represent these without treating unknown extra response fields as output. Run `MISE_ENV=opencode-safe mise exec -- go test ./internal/client -run TestNotificationGeneratedDecoding -count=1`; correct source schemas and regenerate if decoding fails.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./openapi/cmd/normalize ./internal/client -count=1`; expected all tests pass.
- [ ] Stage only Task 1 source/tests/generated output, run `git diff --cached --check`, and commit `feat: generate Dokploy notification API contracts`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make check_openapi`; expect no generated drift. Report any failure, do not dismiss it as expected.

## Task 2: Define typed configuration, defaults, and diff

**Files:** Create `provider/notification.go`, the three `notification_config_*.go` files, `provider/notification_validation.go`, `provider/notification_validation_test.go`, `provider/notification_diff_test.go`, and `provider/notification_test_helpers_test.go`.

**Interfaces:** Produces the public resource/Args/State/Events/config types and `notificationKind`, `normalizeNotificationArgs`, `validateNotificationArgs`. `Notification.Check`/`Diff` use the existing infer request/response signatures. Test helpers include `notificationTestArgs(kind string) NotificationArgs` and `notificationTestResource(t *testing.T, handler http.HandlerFunc) Notification`, following `scheduleTestResource` with retry Attempts 1 and no sensitive payload assertions. Resource registration is deferred until Task 6; do not add unimplemented lifecycle stubs to `Provider()`.

- [ ] Write `TestNotificationResolvedDefaultsAndValidation` and `TestNotificationDiffContract`. Add one valid fixture per channel with literal placeholder settings, plus invalid cases for zero/two blocks, empty required settings, wrong URLs, recipient arrays, SMTP integer bounds, and priorities/emergency rules. Define a test-only `notificationTestArgs(kind string) NotificationArgs` that contains exactly the selected valid block. Core checks:

```go
a := normalizeNotificationArgs(notificationTestArgs("gotify"))
require.Equal(t, 5, *a.Gotify.Priority)
require.False(t, a.Events.ServerThreshold)
a.Events.ServerThreshold = true
require.NotEmpty(t, validateNotificationArgs(a))
```

Defaults: routing/thread strings empty, decoration false, priorities 5/3/0, headers empty. Validate Pushover priority 2 requires retry >=30 and expire 1..10800; validate Ntfy 1..5, Gotify >=1, SMTP port 1..65535. Tests assert property paths, not credential text.

- [ ] Write `TestNotificationHeaderDiffEscapesKeys` with header keys `X.Version`, `X[Mode]`, and `Authorization`; detailed paths are valid Pulumi property paths and values never appear in error text. Write `TestNotificationCheckNestedUnknowns` with computed whole block, nested credential/event, and known block plus another unknown block; assert no discarded unknowns/false failures while two known blocks fail.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(ResolvedDefaultsAndValidation|DiffContract|HeaderDiffEscapesKeys|CheckNestedUnknowns)' -count=1`; expect missing types/helpers.
- [ ] Implement public types with exact spec properties. Events use optional booleans default false; optional defaulted numbers/strings use pointers so defaulting does not overwrite explicit zero/empty. Retry/expire are optional `*int` with no numeric default. Implement annotations for every type/field and the exact secret field set in the spec. Normalize by copying selected configs/maps instead of mutating shared prior state.
- [ ] Implement resolved helpers and `Check` known-value handling. Decode through `infer.DefaultCheck`, inspect nested property computed markers before validating or applying defaults, and retain unknown metadata. Reject two definitely present blocks; defer zero-block decisions if an unknown block could satisfy selection. Runtime Create/Update validation cannot rely on Check having run.
- [ ] Implement detailed Diff for every event/selected-channel setting: changing selected block is replacement, name/same-channel settings update, no changes after default canonicalization, map-order independence. If no valid nested diff path can be emitted for a map key, diff the containing headers property instead. Complete engine-level unknown handling tests in Task 6.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(ResolvedDefaultsAndValidation|DiffContract|HeaderDiffEscapesKeys|CheckNestedUnknowns)' -count=1`; expected pass.
- [ ] Inspect scoped changes/check whitespace and commit `feat: define typed Notification configuration and diff`.

## Task 3: Implement explicit typed channel request adapters

**Files:** Create `provider/notification_channel.go`, `provider/notification_channel_test.go`, the three `notification_channel_*.go` files, and matching group test files.

**Interfaces:** Consumes Task 1 generated requests and Task 2 normalized types. Produces `createNotificationChannel`, `updateNotificationChannel`, `notificationMutationResult` with the shared signatures. Define per-channel body builders `notificationSlackCreateBody(name string, a NotificationArgs) generated.NotificationCreateSlackJSONRequestBody` and `notificationSlackUpdateBody(id, channelID string, a NotificationArgs) generated.NotificationUpdateSlackJSONRequestBody`; apply the same naming pattern for all twelve suffixes.

- [ ] Write table-driven `TestNotificationChannelBodiesWebhook`, `TestNotificationChannelBodiesMessaging`, and `TestNotificationChannelBodiesEmail` in their group test files, marshaling generated bodies to assert exact keys and values for creates and updates. Tests use the requested name argument, all false create events, all desired update events, proper notification/channel IDs, and no organizationId input. Assert clears are explicit: empty routing/accessToken strings, `{}` headers, null retry/expire. Gotify/Ntfy never send serverThreshold.

Example assertions after marshaling/decoding a Custom update body into `body map[string]any`:

```go
require.True(t, body["notificationId"] == "placeholder-notification")
require.True(t, body["customId"] == "placeholder-channel")
require.Equal(t, map[string]any{}, body["headers"])
require.Equal(t, false, body["appDeploy"])
require.NotContains(t, body, "organizationId")
```

- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotificationChannelBodies -count=1`; expect missing builders.
- [ ] Implement builders in the owning group files. Use typed generated structs and existing nullable package, not generic body maps. Map channel IDs to `slackId`, `telegramId`, `discordId`, `emailId`, `resendId`, `gotifyId`, `ntfyId`, `mattermostId`, `customId`, `larkId`, `teamsId`, `pushoverId` respectively. Convert int settings to generated numeric types only where needed by the pinned request schema.
- [ ] Write `TestNotificationChannelEndpoints` using a notification-only local HTTP fixture for all twelve channels. Expect one exact `notification.create<Channel>` POST or `notification.update<Channel>` POST per adapter, no test/discovery requests, and accepted bodyless/irrelevant-body success. Include custom headers with dotted/bracketed keys and token-like values; assertions report field names/booleans rather than raw payloads.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotificationChannelEndpoints -count=1`; expect missing dispatch.
- [ ] Implement explicit channel switches and raw mutation calls. `notificationMutationResult` closes any response body, returns transport/non-2xx errors, and does not turn a close/irrelevant-body parsing failure into loss of acknowledged success. Do not sanitize here; Task 5 sanitizes at the lifecycle boundary.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotificationChannel(Bodies|Endpoints)' -count=1`; expected pass for all twelve cases.
- [ ] Inspect/check changes and commit `feat: add typed Notification channel request adapters`.

## Task 4: Project observed state and verify discovery identity

**Files:** Create `provider/notification_observation.go`, `provider/notification_observation_test.go`; add channel response projection helpers to the three existing channel group files/tests.

**Interfaces:** Produces `notificationIdentity`, `notificationStateFrom`, `notificationListIDs`, `notificationCandidate`, `notificationActiveOrganizationID`, `readNotification` with shared signatures. Define per-channel projection helper `notificationSlackArgsFrom(v *generated.NotificationSlack, prior *NotificationSlackConfig) (*NotificationSlackConfig, error)` and analogous helpers for the other eleven relations. Observation errors may be raw internally; exported lifecycle methods sanitize them in Task 5.

- [ ] Write `TestNotificationObservedStateAllChannels` for import/refresh. Assert exact ID/org/type/channel relation mapping, only selected block, all eight observed event booleans, canonical nullable routing/map values, numeric types, and no extra response properties. Include omitted versus explicit null/empty required secrets: omission retains prior or becomes empty on import; explicit clear becomes empty without restoring prior. Omitted Ntfy accessToken follows its empty default.
- [ ] Write `TestNotificationIdentityValidation`: mismatched requested notification ID, wrong session organization, changed known type/channel/org ID, missing foreign key, disagreeing nested ID, multiple active relations, unsupported type, missing/null event boolean, and malformed required non-secret fields all fail. An empty expected type/channel on import derives identity, but nonempty expected identity is never replaced. Include opaque IDs with `+`, `/`, `%`, and spaces; query values round-trip unchanged.

Core identity assertion using an inline complete response fixture `v` whose selected nested relation ID differs from its foreign key:

```go
_, err := notificationStateFrom(v, nil, notificationIdentity{
    NotificationID: "placeholder-notification", OrganizationID: "placeholder-org",
})
require.Error(t, err)
```

- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(ObservedStateAllChannels|IdentityValidation)' -count=1`; expect missing observation helpers.
- [ ] Implement typed projections and exact-ID reads. Fetch active organization using existing `activeOrganizationID` extraction; validate nonempty identity. Preserve raw field presence where generated pointers collapse null/omission; fix source schemas/regenerate rather than guessing. `readNotification` calls one exact GET and never writes or discovers by name.
- [ ] Write `TestNotificationDiscoveryCandidates`: snapshot IDs reject empties/duplicates; unrelated marker returns false; marker-matching pre-existing ID, wrong organization/type, missing/mismatched channel settings/secret, non-false event, or relation mismatch returns error; one full canonical match returns its verified state. Add `TestNotificationMarkerConflictIsNotEmpty` with a marker whose credential differs; assert error, not retry-eligible false.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(DiscoveryCandidates|MarkerConflictIsNotEmpty)' -count=1`; expect missing candidate helper or assertions fail until implemented.
- [ ] Implement candidate validation against the snapshot and canonical desired channel settings. A matching marker requires complete settings presence, not projection defaults concealing omissions. Null optional routing/maps may canonicalize only when explicitly present. Never select by requested final name or first/latest candidate; orchestrator cardinality is Task 5.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(ObservedStateAllChannels|IdentityValidation|DiscoveryCandidates|MarkerConflictIsNotEmpty)' -count=1`; expected pass.
- [ ] Inspect/check changes and commit `feat: verify Notification observation and discovery identity`.

## Task 5: Implement guarded lifecycle and partial-state safety

**Files:** Create `provider/notification_lifecycle.go`, `provider/notification_lifecycle_test.go`, `provider/notification_error_test.go`; extend notification-only test helpers.

**Interfaces:** Consumes Tasks 2–4 helpers. Produces `Notification.Create`, `Read`, `Update`, `Delete` with existing inferred-resource signatures, and `sanitizeNotificationError(error) error`. Raw helpers stay internal. Use `initFailed` for verified create partial state, preserving stable ID and last trustworthy observed state.

- [ ] Write `TestNotificationSafeErrorCategories` directly against the sanitizer: arbitrary server Code/Message, URL-bearing transport errors, and malformed decoder errors lose their prose; fixed status/allowlisted category remains; context cancellation/deadline preserve `errors.Is`. Assertions never print fixture-sensitive text.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotificationSafeErrorCategories -count=1`; expect missing sanitizer.
- [ ] Implement `sanitizeNotificationError` using fixed categories/status and allowlisted codes, dropping free-form messages/bodies/URLs. Do not change unrelated resource sanitizers.

- [ ] Write `TestNotificationCreateAllChannels` with the full request sequence: active org, before list, create POST with captured UUID marker/all events false, after list, exact-ID verification, same-channel final update, final exact-ID read. Assert final requested name/events, expected settings and IDs, exactly one create, no testConnection/delete. Validate the captured marker prefix and UUID without logging it; fixtures use only placeholders.
- [ ] Write `TestNotificationCreateDryRunNoCalls` with a client factory that fails on invocation; assert no HTTP traffic and no concrete create ID.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotificationCreate(AllChannels|DryRunNoCalls)' -count=1`; expect missing Create.
- [ ] Implement the basic Create sequence and dry-run path using Tasks 2–4 helpers and the safe error boundary. Runtime validation precedes HTTP. Use uuid.NewString, all-disabled create settings, exact candidate verification, and final update/read-back; never use a requested-name-only match.
- [ ] Write `TestNotificationCreateFailureStates` with: zero candidates through three lists; second-list visibility; duplicate marker; wrong-configuration marker; list error; uncertain create with zero/one candidate; canceled/deadline context; malformed snapshot; verified candidate followed by wrong-ID/404/transport read; final rename rejection; final read failure/mismatch. Assert correct ID availability, `ResourceInitFailedError` once verified, no replay or cleanup, and last trustworthy state rather than desired settings. Include `TestNotificationCreateUnreadableAckBody`: 2xx headers plus truncated/unreadable body still proceeds to discovery once.

Core assertions for a list-verified candidate followed by a failed exact-ID read (handler records `createPosts`, `deletePosts`, and the marker):

```go
got, err := r.Create(t.Context(), infer.CreateRequest[NotificationArgs]{Inputs: a})
var partial infer.ResourceInitFailedError
require.ErrorAs(t, err, &partial)
require.True(t, got.ID == "placeholder-notification")
require.True(t, got.Output.Name == marker)
require.False(t, got.Output.Events.AppDeploy)
require.Equal(t, 1, createPosts)
require.Zero(t, deletePosts)
```

- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotificationCreate(FailureStates|UnreadableAckBody)' -count=1`; verify failures expose missing safety branches.
- [ ] Implement Create failure safety: distinguish definite API rejection from uncertain transport failure; abort on invalid org/snapshot or marker collision; at most three post-create lists retry only a valid empty candidate set. Ambiguity/conflicts/malformed responses fail immediately without identity. List-verified state survives exact-ID read failures. After any create error, return verified partial state without finalizing. Never replay create or auto-delete. Keep GET retries and discovery within context.
- [ ] Write `TestNotificationReadUpdateDelete`: refresh/import all channels; not-found clears; Update rotates credentials/events/headers in place after active-org/exact-ID verification and reads back; Gotify/Ntfy observed threshold true cannot be cleared; Delete checks ownership, calls remove, verifies absence. Generic BAD_REQUEST with object still present fails; BAD_REQUEST followed by exact-ID absence succeeds. A substituted channel ID or changed session organization causes no POST. Add acknowledged update with unexpected observed settings and acknowledged delete with still-present record as failures.
- [ ] Write `TestNotificationUpdateDryRunNoCalls` with a client factory that fails on invocation; assert preserved notification/channel/org IDs and no traffic.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'TestNotification(ReadUpdateDelete|UpdateDryRunNoCalls)' -count=1`; expect missing methods/safety branches.
- [ ] Implement Read/Update/Delete per spec. Read derives import identity and enforces known prior identity. Runtime validation precedes Update; mutations use the freshly verified relation ID. Preserve trustworthy state on update failure. Treat absence only through classified NOT_FOUND/404 exact-ID reads, never broad BAD_REQUEST swallowing. Never independently delete nested channel rows.
- [ ] Write `TestNotificationSafeErrors` injecting credential-bearing URLs, malformed JSON excerpts, unknown server-returned credentials/IDs, transformed credentials, HTTP/status errors, cancellation/deadline, and validation failures. Assert diagnostic text omits every fixture-sensitive value, carries fixed operation context/status, and preserves `errors.Is` for context errors. Do not use failure assertions that print supplied secret values.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotificationSafeErrors -count=1`; expected pass because all lifecycle boundaries use the tested sanitizer. If a branch leaks, retain the failing regression and repair only that boundary.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotification -count=1`; expected all notification tests pass.
- [ ] Inspect/check changes and commit `feat: manage Notification lifecycle with guarded discovery`.

## Task 6: Register resource, verify inference, and generate SDKs

**Files:** Modify `provider/provider.go`, `provider/schema_test.go`, `provider/registry_metadata_test.go`, `provider/notification.go`; create `provider/notification_schema_test.go`, `provider/notification_inference_test.go`; regenerate provider schema and all SDKs.

**Interfaces:** Publishes `dokploy:index:Notification`, `NotificationEvents`, twelve `Notification<Channel>Config` types, static SDK get/import support, and all four identity outputs. Existing resources/functions remain unchanged.

- [ ] Write `TestSchemaNotificationContract`: exact token, required name, twelve optional typed blocks and optional events, no discriminator/identity inputs/enabled, four required identity outputs, descriptions throughout, exact default/secret fields, and no unconditional replacement annotation on channel blocks. Check nested schema types rather than only top-level properties. Add only Notification to the exact resource inventory and update `TestRegistryMetadata`'s resource count from 19 to 20 on this base; reconcile with actual execution-branch resources rather than removing independently shipped features.

```go
spec := providerSchema(t)
r := spec.Resources["dokploy:index:Notification"]
require.Equal(t, []string{"name"}, r.RequiredInputs)
require.NotContains(t, r.InputProperties, "notificationType")
require.NotContains(t, r.InputProperties, "organizationId")
slack := spec.Types[trimTypeRef(r.InputProperties["slack"].Ref)]
require.True(t, slack.Properties["webhookUrl"].Secret)
require.False(t, r.InputProperties["slack"].ReplaceOnChanges)
```

- [ ] Write `TestNotificationInferencePreviewAndSecrets` using `integration.NewServer`: unknown whole channel, nested webhookUrl, events object/individual event, known block plus unknown block; assert no discarded computed inputs, no preview traffic, computed create IDs, preserved same-channel update IDs, and secret nested credential/header outputs even when inputs were supplied as ordinary values. Test same-channel credential change reports Update while changing selected block reports replacement through engine Diff.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'Test(SchemaNotificationContract|NotificationInferencePreviewAndSecrets)' -count=1`; expect absent resource token.
- [ ] Register `infer.Resource(&Notification{client: configuredClient})`, update provider description to include notifications, and implement WireDependencies. Create IDs/type/org/channel outputs compute when selected channel/input dependencies are unknown; same-channel updates retain identity. Wire corresponding nested sensitive input/output dependencies without globally tainting identity outputs. Keep ordinary unknown block validation from Task 2 intact. Preserve existing functions, including any separately implemented lookup functions.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run 'Test(SchemaNotificationContract|NotificationInferencePreviewAndSecrets)' -count=1`; expected pass after correcting inference/dependency wiring. Do not settle for direct typed handler tests if engine behavior differs.
- [ ] Inspect generated-file status, then run `MISE_ENV=opencode-safe mise exec -- make codegen`; verify Notification and nested config types exist in Go, Node.js, Python, .NET, and Java. No manual SDK edits.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make build_sdks`; expected successful compilation of all five SDKs.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make test_provider`; expected offline provider/internal tests pass, including updated metadata inventory. Website checks are completed in Task 7, not silently waived here.
- [ ] Inspect/check changes, explicitly stage provider/tests/schema/all affected SDK output, and commit `feat: publish typed Notification resource across SDKs`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make check_codegen`; expect no generated drift. Report tooling/network limitations as limitations, not passes.

## Task 7: Update user docs, references, and offline examples

**Files:** Modify `website/scripts/reference-model.mjs`, `website/tests/reference-model.test.mjs`, `website/tests/render-reference.test.mjs`, `website/tests/content.test.mjs`, `website/tests/site-output.built-test.mjs`, `website/astro.config.mjs`, `README.md`, `CHANGELOG.md`, `website/src/content/docs/guides/imports.mdx`, `website/src/content/docs/concepts/secrets.mdx`; create `website/src/content/docs/guides/notifications.mdx`, `provider/notification_docs_test.go`, `examples/go/notification_test.go`; regenerate reference pages. Inspect other website route inventories and update only necessary assertions.

**Interfaces:** Required guide/reference routes `/guides/notifications/`, `/reference/notification/`. `formatType({type:"object",additionalProperties:{type:"string"}})` returns `map<string, string>`; preserve existing reference formatting and refuse unsupported object schemas. Header values are documentation examples, not credentials.

- [ ] Write website tests named `formats notification string maps`, `Notification reference renders secret headers`, and `Notification guide covers channels and safe recovery`: assert exact map formatting/rendered secret metadata, Notification inventory, guide title/description/navigation, all twelve channels, false events, threshold restrictions, emergency options, import token, secret config, recovery/permissions, and no automatic test-message promise. Add canonical route and built-site guide/reference/type-anchor checks.

```js
assert.equal(formatType({ type: "object", additionalProperties: { type: "string" } }), "map<string, string>");
assert.match(guide, /dokploy:index:Notification/);
assert.match(guide, /pulumi config set --secret/);
assert.match(guide, /temporary/i);
assert.match(guide, /refresh/i);
```

- [ ] Write `TestNotificationDocumentationCoverage` in `provider/notification_docs_test.go`, following existing docs tests: require Notification token/import and guide paths, temporary recovery/default restrictions, and no hard-coded credentials or live test instructions.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- node --test website/tests/reference-model.test.mjs website/tests/render-reference.test.mjs website/tests/content.test.mjs`; expect unsupported-map/missing-guide/inventory failures.
- [ ] Implement narrow string-map formatting and add Notification to expected resource set. Update navigation placing Notification after Schedule in Resources and Notifications after Schedules in Guides, preserving existing order/base routing. Generate references from schema using `MISE_ENV=opencode-safe mise exec -- make docs_generate`; do not rewrite generated pages manually.
- [ ] Write the guide with every spec documentation deliverable. Show a focused TypeScript Slack example using `config.requireSecret("notificationWebhookUrl")`, explicit `events: { appDeploy: true, appBuildError: true }`, and no real endpoint or ID. Include a Custom example with secret endpoint/headers, and a Pushover priority-2 snippet using retry 30/expire 3600 and secret credentials. Document all channel fields/defaults in a concise table and link to generated types. Explain disabled temporary creation, verified partial state, refresh-before-repair, possible orphan investigation before retry, and that names alone do not authorize deletion.
- [ ] Update README to list the actual implemented resource count/tokens (20 on the current base, including previously omitted Schedule), import token, notification secret/event/lifecycle guidance, and guide link. Add Notification to curated import/secrets pages and CHANGELOG `Unreleased`; do not invent a release or remove existing resource guidance.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./provider -run TestNotificationDocumentationCoverage -count=1`; expect pass once docs exist.
- [ ] Add `TestNotificationSDKExample` in `examples/go/notification_test.go` using Pulumi runtime mocks and generated Go SDK. Instantiate `Notification` with typed Slack config and secret webhook Output, explicit events; mocks return placeholder notification/type/channel/org IDs. Assert one Notification registration and usable ID output without HTTP calls. Include a typed Custom header map with dotted/bracketed keys so SDK shape compilation is exercised. Keep the existing `main.go` unchanged.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test . -run TestNotificationSDKExample -count=1` from `examples/go`; expected pass with mocks, no live endpoint.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- node --test website/tests/reference-model.test.mjs website/tests/render-reference.test.mjs website/tests/content.test.mjs`; expected pass.
- [ ] Inspect/check changes, explicitly stage all Task 7 docs/model/tests/example/reference output, and commit `docs: document Notification channels and safe recovery`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make docs_check` and `MISE_ENV=opencode-safe mise exec -- make test_examples`; expected generation drift checks, website checks/build/built tests, and language examples pass. Regeneration cannot overwrite unrelated user changes.

## Final validation and handoff

- [ ] Recheck the safe overlay with the prerequisite assertion; confirm no acceptance execution or external messages occurred.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make lint`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make test_provider`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make test_race`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make provider`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- go test ./openapi/cmd/normalize -count=1`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make check_openapi`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make check_codegen`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make build_sdks`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make test_examples`.
- [ ] Run `MISE_ENV=opencode-safe mise exec -- make docs_check`.
- [ ] If dependencies unexpectedly changed, explain why and run `make govulncheck` and `make license` with the safe profile; otherwise no dependency change is intended.
- [ ] Run `git diff --check`, inspect status and the whole scoped implementation diff. Exclude the safe overlay and distinguish any pre-existing user work. Do not claim an unavailable/unrun check passed.
- [ ] Self-review against every spec section and the five Review Focus cases. Obtain the execution method's required independent review; fix findings with targeted regression tests and focused commits before claiming completion.
- [ ] Report delivered channels/resource/SDK/docs changes, exact validation outcomes, limitations of bodyless creation and unsupported threshold settings, and no live-test claims.
- [ ] Ask whether to retain or delete this Notification spec/plan and its task-local working notes; delete only approved related documents. Preserve the existing lookup planning documents and unrelated user changes.

## Plan self-review coverage

- Public contract/defaults/validation/diff: Tasks 2 and 6.
- Pinned API/source generation and exact channel payloads: Tasks 1 and 3.
- Organization/relation identity, import, nullable settings and secrets: Task 4.
- Disabled temporary creation, uncertainty, bounded discovery, verified partial state, mutation/read-back/delete/error safety: Task 5.
- Engine unknowns/secrets/previews and all five generated SDKs: Task 6.
- Required documentation, map rendering, navigation, import/security guidance, examples, and built-site checks: Task 7.
- All Review Focus cases have named tests in their owning tasks. No live testing, unrelated lookup implementation, or new dependency is planned.
