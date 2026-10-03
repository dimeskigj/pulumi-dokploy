# Dokploy Notification resource

## Intent and approved scope

Manage Dokploy notification destinations and event subscriptions through Pulumi,
not notifications emitted by Pulumi or configuration of the provider itself.
Provide one typed `dokploy:index:Notification` resource covering all twelve
channels in the pinned Dokploy API. Success means safe create/update/read/delete
and import behavior, usable generated SDKs, protected credentials, offline
regression coverage, and updated user documentation.

The user approved:

- All twelve channels, with exactly one typed channel configuration per resource.
- Shared event settings defaulting to false; changing channel replaces the resource.
- Creation with an unpredictable temporary name and all events disabled, followed
  by guarded discovery and an ID-based update to the requested settings.
- No automatic connection tests, uncertain-create retries, or failure cleanup.
- Identity verification before mutations and preservation of verified partial state.
- OpenAPI corrections against the existing pin, regenerated SDKs, and offline tests.
- Documentation updates as explicit implementation deliverables.

This document authorizes no implementation or live testing by itself. Written-spec
approval precedes implementation planning; execution requires a separate decision.

## Architecture and alternatives

Register one inferred resource in `provider/provider.go`, using `configuredClient`
and the existing authenticated generated client. Follow existing annotation,
Check/Diff, preview, dependency wiring, and partial-state conventions. Keep the
notification implementation separate from unrelated resource lifecycles.

Use focused units:

- Public argument/state types, descriptions, defaults, validation, and diff.
- Explicit typed channel request/response adapters, grouped into small files.
- Shared lifecycle orchestration, ownership verification, and safe diagnostics.

Do not put all twelve adapters into one large file or use reflection/untyped maps
to dispatch configuration. Shared helpers must leave endpoint selection and
ownership checks visible. Only custom HTTP headers are a user-configurable map.

Alternatives considered:

1. Twelve channel-specific resources: excellent channel typing, but repeated event
   contracts and a larger public surface. The user chose one typed resource.
2. A channel selector plus arbitrary settings map: fewer types, but weak SDK
   contracts and runtime-only validation. Rejected.
3. Creation identity from an upstream ID-returning/caller-supplied-ID API: simpler,
   but unavailable at the pin. Requested-name discovery avoids a rename but has
   weaker concurrent ownership guarantees. The user chose a random temporary name.

## Public resource contract

Inputs:

- `name`: required, nonempty string.
- `events`: optional `NotificationEvents` object, containing the eight optional
  boolean fields below. An omitted object or field means false.
- Exactly one optional typed block named `slack`, `telegram`, `discord`, `email`,
  `resend`, `gotify`, `ntfy`, `mattermost`, `custom`, `lark`, `teams`, or `pushover`.

There is no input channel discriminator: the selected block determines the type.
There is no `enabled`, `organizationId`, `notificationId`, or channel-ID input.
Organization ownership follows the authenticated session's active organization.

Outputs include observed inputs and four required strings: `notificationId`,
`notificationType`, `channelId`, and `organizationId`. `channelId` is the selected
channel relation's stable ID, not a user routing field such as Slack `channel` or
Telegram `chatId`. Only the selected channel configuration appears in state.

Import token: `dokploy:index:Notification`; import identity: Dokploy notification ID.
Read determines the channel for a fresh import. A known existing resource whose
channel identity unexpectedly changes fails closed rather than adopting the new
relation. IDs are opaque; validate nonempty values without rewriting or logging them.

### Event settings

`appDeploy`, `appBuildError`, `databaseBackup`, `volumeBackup`, `dokployBackup`,
`dokployRestart`, `dockerCleanup`, and `serverThreshold` all default to false.
Updates send explicit managed boolean values, including false, rather than relying
on server defaults or omission. Do not invent success/failure-specific event filters
or per-workload scopes absent from the pinned contract.

Gotify and Ntfy create/update omit `serverThreshold` and their services never write
it. Reject desired true for those two channels. Refresh may report an externally
set true value, but a mutation attempting to manage it must fail safely rather than
claim it was cleared. Specifically, refuse Update when a Gotify/Ntfy pre-read shows
true and desired false; the pinned endpoint cannot perform that clear. Delete remains
available after identity verification. A disabled creation candidate must have all
eight observed events false, including the server default for this unsupported field.

### Channel settings

These are Pulumi property names; required fields have no implicit credential values.
Each block has a dedicated `Notification<Channel>Config` type, using `Email`,
`Ntfy`, and `Custom` for those block names.

| Block | Required settings | Optional settings and defaults |
| --- | --- | --- |
| `slack` | `webhookUrl` | `channel` = empty string |
| `telegram` | `botToken`, `chatId` | `messageThreadId` = empty string |
| `discord` | `webhookUrl` | `decoration` = false |
| `email` | `smtpServer`, `smtpPort`, `username`, `password`, `fromAddress`, `toAddresses` | None |
| `resend` | `apiKey`, `fromAddress`, `toAddresses` | None |
| `gotify` | `serverUrl`, `appToken` | `priority` = 5, `decoration` = false |
| `ntfy` | `serverUrl`, `topic` | `accessToken` = empty string, `priority` = 3 |
| `mattermost` | `webhookUrl` | `channel` and `username` = empty string |
| `custom` | `endpoint` | `headers` = empty string-to-string map |
| `lark` | `webhookUrl` | None |
| `teams` | `webhookUrl` | None |
| `pushover` | `userKey`, `apiToken` | `priority` = 0; nullable/optional `retry`, `expire` |

Canonical empty strings/maps resolve omission/default drift. Normalize nullable
optional routing strings and headers returned by the server to these canonical
empty values. Keep Pushover retry/expire absent when null, not zero.

Validate nonempty required strings without echoing values; HTTP(S) destination URLs
must be absolute with a host, but may contain credential-bearing paths or queries.
Do not require Slack channel or Telegram message thread values to be nonempty.
Require nonempty recipient arrays containing nonempty strings; do not introduce a
custom email-address parser. SMTP port is an integer from 1 to 65535. Priorities
are integers: Gotify >= 1, Ntfy 1 through 5, Pushover -2 through 2. Pushover retry
must be >= 30, expire 1 through 10800, and both are required at priority 2.

Clearing supported optional routing strings uses explicit empty strings; clearing
custom headers uses an empty map; clearing Ntfy accessToken uses an empty string;
clearing Pushover retry/expire uses explicit JSON null. Mattermost optional strings
cannot be cleared with JSON null at the pinned request schema. Do not silently
omit desired clears or send null to non-nullable fields.

### Validation, diff, and preview

Check defaults and validates known values only. Inspect nested computed values
before cross-field checks; never overwrite an unknown block/event/credential with
a default or reject an unresolved valid preview. Reject two definitely selected
blocks, no selected block when all are known absent, and invalid known settings.

Name, event, credential, header, and same-channel configuration changes update in
place. Changing which block is selected is a replacement. Do not mark whole channel
blocks unconditionally replace-on-change: that would replace on credential rotation.
Use detailed nested diffs and compare headers independently of map iteration order.

Create/update dry-runs perform no API calls. New identity outputs remain computed;
same-channel update previews preserve existing IDs. Secret dependencies and nested
schema annotations protect sensitive state without making ordinary IDs secret.

## Pinned upstream evidence and OpenAPI

The existing source is pinned to Dokploy commit
`cebd3808565ea9bed0791961bc25c60513d94c5a`. Evidence reviewed:

- `apps/dokploy/server/api/routers/notification.ts`: all create routers await the
  service but return no identity; one/all are scoped to active organization;
  connection-test endpoints send real external messages.
- `packages/server/src/services/notification.ts`: create services generate IDs
  internally and do not return their transaction results to the router; updates
  require notification ID and the selected channel ID; one/all include channel
  relations; Gotify/Ntfy do not write serverThreshold.
- `packages/server/src/db/schema/notification.ts`: exact field names, nullable
  relation/settings fields, event defaults, and Pushover emergency refinement.

Source links use `https://github.com/Dokploy/dokploy/blob/` followed by the commit
and paths above. Do not update the pin or fabricate caller-supplied-ID support.

Select all twelve `notification.create<Channel>` and `notification.update<Channel>`
operations plus `notification.one`, `notification.all`, and `notification.remove`
in `openapi/operations.txt`. Reuse existing `organization.active`. Do not select
testConnection, receiveNotification, or getEmailProviders operations.

Add source corrections defining a Notification response, NotificationList array,
and typed channel relation schemas with nullable inactive relations. Include the
identity/organization fields, all events, selected foreign-key IDs, and the channel
settings above. Preserve absent versus explicit null where needed. Response
validation belongs at the notification boundary; future unrelated response fields
are ignored and never projected into Pulumi state.

Create/update are bodyless contracts at this pin; do not correct them to invented
Notification response objects. Remove's service returns a deleted record, but its
response is not needed for lifecycle identity and may be ignored. Use raw generated
mutation calls and close response bodies, so parsing an irrelevant body cannot lose
acknowledged success. Non-2xx remains an error through the existing transport.

Regenerate `openapi/dokploy.json`, `internal/client/generated/generated.gen.go`,
the provider schema, and all five SDKs from their sources. Do not hand-edit output
or add a dependency solely for notification support.

## Lifecycle and ownership safety

### Create

1. Validate resolved settings. Read and validate active organization; snapshot a
   complete notification list with nonempty, unique IDs before any mutation.
2. Generate a cryptographically unpredictable temporary name using existing UUID
   support, for example `pulumi-notification-<uuid>`. Confirm that marker was absent
   from the snapshot. Create exactly once with that name, the requested channel
   settings, and all events false.
3. After acknowledged success or an uncertain transport failure, obtain a bounded
   post-create list. Never repeat the create POST. Allow at most three list attempts
   under the request context, in addition to existing bounded GET transport retries;
   only an empty candidate set permits another list attempt. Ambiguity/malformed
   responses fail immediately. A marker-matching record that fails ownership or
   configuration checks is an error, not an empty candidate set eligible for retry.
   Do not sleep beyond context cancellation/deadline.
4. A candidate must have a new nonempty ID absent from the snapshot, exact marker,
   expected organization/type, matching complete canonical channel settings,
   consistent foreign-key/nested channel IDs, and all observed events false.
   Require exactly one candidate. Do not select the first/latest record or match
   only user name. Omitted settings/credentials cannot establish this match.
5. A candidate meeting every list check establishes discoverable identity and
   safe partial state. Read that exact notification ID and verify the same identity,
   marker, configuration, and disabled events before any further write. A failed
   read-back preserves the list-verified ID/state, never adopts the response's ID.
6. If create had an error, return verified partial state with an initialization
   failure; do not silently finalize an uncertain create. If create succeeded and
   verification succeeds, update that ID to the desired name/events using its
   verified channel ID, then read back and return the canonical observed state.

Before verified discovery, a failure returns no fabricated ID and a safe diagnostic
that creation may have occurred and operator investigation is required. Do not print
the random marker, supplied settings, or candidate IDs. Documentation must explain
how to inspect recent disabled records in the Dokploy UI without deleting by name
alone. Creation requires notification read/list/create/update permissions.

After identity is verified, retain it through rename/update/read-back failures.
Partial state uses the last trustworthy observed configuration, not an assertion
that the final name/events were applied. The server may have committed the final
update despite a failure, so tell operators to refresh before repair. Never
automatically clean up, replay create, or finalize after an uncertain response.

The random marker mitigates concurrent same-name discovery; it is not a server
idempotency key or an absolute ownership proof against a malicious actor who can
observe and copy it. Cross-client idempotency remains an upstream limitation.

### Read and import

Read active organization and the exact notification ID. Validate notification ID,
organization, supported channel type, matching nonempty foreign-key/nested relation
IDs, name, event booleans, and required non-secret channel fields. For an existing
state, verify its type and known channel/organization IDs; for an import, derive them
from the response. Do not fill absent required metadata with unrelated defaults.

Require all event booleans to be present and non-null. Optional routing/settings
fields follow the canonical rules above. Required secret fields omitted from a
refresh retain prior secret values; a fresh import cannot recover omitted secrets
and represents these as empty secret values requiring user resupply before update.
Explicit null/empty required secrets become empty secret values in observed state,
never stale prior credentials. Read may report this drift; Check and runtime mutation
validation require resupply before Update and never send an empty required credential.
Pinned responses normally contain credentials, but do not promise that every server
version exposes them.

Confirmed not-found clears the resource ID. Malformed JSON, missing identity,
unsupported type, mismatched identity, or malformed active relations are errors,
never interpreted as deletion. Unselected relations must be null/absent; unexpected
active relations fail closed. No connection tests or mutation calls during Read.

### Update and delete

Pre-read the exact resource in its active organization, verify known identity, and
use the selected relation ID from that verified record. Never trust a supplied or
stale channel ID for mutation. If identity changed externally, require operator
reconciliation rather than changing a newly substituted relation.

Update sends complete desired same-channel settings and explicit event values,
including clears, then reads back. Treat missing/mismatched desired values after
acknowledgment as failure, not success. Return the existing verified identity and
last trustworthy state on failure; preserve context/error classification safely.

Delete uses notification.remove only after verification. Pre-read not-found is
success. The pinned remove router can wrap not-found as BAD_REQUEST: do not swallow
that category. If remove fails, only a fresh exact-ID read confirming absence makes
it idempotent success; otherwise return the safe remove error. Verify absence after
acknowledged remove. Do not delete channel records independently or refactor their
upstream cascading behavior.

## Secrets and diagnostics

Mark webhookUrl, Telegram botToken, SMTP password, Resend apiKey, Gotify appToken,
Ntfy accessToken, Pushover userKey/apiToken, and custom endpoint/headers secret.
Also mark Gotify/Ntfy serverUrl secret conservatively because it may embed URL
credentials. Apply nested input/output schema secrecy and engine-level dependency
propagation; test that ordinary unwrapped credential inputs become secret outputs.

Errors use fixed operation context, HTTP status, and allowlisted classifications,
not arbitrary server prose, bodies, decoder snippets, URLs, IDs, or secret values.
Preserve cancellation/deadline identity where possible. Avoid substring redaction
as the only protection: server-returned/transformed credentials may not match prior
input. Validation diagnostics identify property paths without printing their values.
Do not log raw list/one responses containing other notifications' credentials.

## Documentation and examples: required deliverables

Documentation is part of feature completion, not a follow-up:

- Describe the resource, all arguments/outputs, event fields, and all twelve channel
  types/fields in schema annotations; regenerate the Notification reference page
  and shared type references.
- Update `website/scripts/reference-model.mjs`'s expected resource set and add
  narrowly scoped string-map formatting for custom headers, with renderer/model
  tests. Preserve existing resource/type references and deterministic generation.
- Add `website/src/content/docs/guides/notifications.mdx` and a navigation entry in
  `website/astro.config.mjs`. Cover all channels/settings, false event defaults,
  Gotify/Ntfy threshold limits, Pushover emergency options, secret configuration,
  import, required permissions, temporary creation visibility, orphan investigation,
  partial-state recovery, channel replacement, and the absence of automatic tests.
- Update README resource inventory and notification/import/security guidance without
  overwriting unrelated resource descriptions. Update CHANGELOG under the existing
  unreleased convention if present; do not invent a release version.
- Add a focused SDK example using secret config and events, with offline compiled
  or mocked coverage. Do not add notifications to a live acceptance program or
  regenerate unrelated canonical workload examples just to demonstrate this feature.
- Use placeholders only. No real IDs, private URLs, credentials, or copy-pastable
  instructions for unsafe production acceptance testing.

## Tests and implementation verification

Use TDD with scripted local HTTP servers and inferred-provider integration tests.
No real Dokploy endpoint, external channel service, or message transmission is
required or authorized. Test:

1. Exact resource/type schema, nested required fields/defaults/descriptions/secrets,
   channel outputs, and unchanged existing resource/function contracts.
2. Every channel's create/update adapter, relation mapping, full boolean payloads,
   empty settings, explicit clears, priority bounds, recipients, and emergency rules.
3. Exactly-one-block validation and nested unknown/secret values through the actual
   inference framework, not only directly called typed handlers.
4. No API traffic during create/update previews; identity outputs computed on create
   and preserved for same-channel previews; nested secret state in engine responses.
5. Before/after discovery, unrelated/pre-existing records, concurrent requested names,
   multiple marker candidates, duplicate/missing IDs, wrong organization/type/config,
   malformed lists, bounded empty-list retries, cancellation, and uncertain creates
   with zero or one verified candidate. Assert no repeated POST and no auto-delete.
6. List-verified partial identity after one read/rename/final-read failure; failures
   without trustworthy identity do not invent a resource ID.
7. Refresh/import for all channels, null/omission semantics, credential preservation,
   externally changed events/settings, mismatched relations, and unsupported types.
8. Fresh pre-mutation relation checks, credential rotations without replacement,
   channel changes with replacement, deleted resources, BAD_REQUEST remove wrapping,
   and post-delete absence verification. Assert no testConnection calls anywhere.
9. Safe errors containing malicious server messages, credential-bearing URLs,
   decoder excerpts, previously unknown secret values, and cancellation/deadlines.
10. OpenAPI normalization/generated decoding, SDK example compilation/mocks, reference
    map rendering, guide navigation, and built-site content.

Install/use versions in `.mise.toml`. Its default environment loads `.env` and
enables acceptance, so implementation must establish a task-local safe mise overlay
disabling acceptance and blanking endpoint/API key, verify nested mise subprocesses,
and use that safe profile for every test/build/codegen command. Do not read `.env`,
print environment values, or run live tests incidentally.

Implementation checks: `make lint`, `make test_provider`, `make test_race`,
`make provider`, OpenAPI normalizer tests, `make check_openapi`, `make check_codegen`,
`make build_sdks`, `make test_examples`, and `make docs_check`. Generate/commit the
expected output before drift checks, which compare against the Git index/worktree.
Inspect uncommitted generated files before generation; do not overwrite user work.
Run `git diff --check`, inspect the scoped diff/status, and report unavailable or
unrun checks honestly. Dependency/license checks apply if dependencies change.

Do not implement the separately planned read-only lookup functions as part of this
feature. Preserve those existing planning documents and adjust to actual branch
contracts at execution time rather than assuming the lookup plan was implemented.

When implementation finishes, ask whether to retain or delete this task's spec and
plan. Do not remove either without explicit approval, and preserve unrelated plans.
