# Dokploy GiteaIntegration resource

## Intent and approved scope

Manage a Dokploy Gitea integration's configuration and lifecycle in Pulumi while
leaving OAuth application setup and authorization to the operator. This document
records the approved conversational design, not implementation authority. Written
spec review must precede an implementation plan.

Existing Application/Compose workload-source types and behavior remain unchanged.
This is not an OAuth app creator, authorization client, token manager, connection
tester, repository discovery feature, or sharing/permission manager. Do not add
readiness, token, expiry, username, or scopes inputs/outputs.

## Public contract

Register `dokploy:index:GiteaIntegration` following existing inferred-resource and
generated-client conventions.

| Input | Requirement and meaning |
| --- | --- |
| `name` | Required nonempty Dokploy display name. |
| `giteaUrl` | Required absolute HTTP(S) Gitea base URL. |
| `clientId` | Required nonempty OAuth client ID. |
| `clientSecret` | Required nonempty OAuth client secret; secret in input and output schema. |
| `redirectUri` | Required nonempty absolute HTTP(S) callback URL. |
| `giteaInternalUrl` | Optional nullable absolute HTTP(S) internal base URL; secret because it may contain credentials. |
| `organizationName` | Optional string; omission and empty canonicalize to empty string. |

Outputs are observed managed configuration plus `giteaId` (Pulumi/import identity),
`gitProviderId` (stable parent identity used for update/removal), and
`organizationId` (owner). No public organization selector, parent ID, OAuth token,
expiration, username, scopes, sharing, or readiness field. IDs remain ordinary
outputs. Import takes the Gitea relation ID, not parent ID.

Validate each known value independently; unresolved computed values are not empty
errors and must not hide another known invalid value. Base URLs may contain paths;
reject userinfo, query, fragment, and bare delimiters. Callback must be absolute
HTTP(S) and reject userinfo. Do not rewrite supplied URLs. All managed changes,
including secret rotation and URL changes, update in place. Secret metadata and
dependency annotations must preserve clientSecret and giteaInternalUrl secrecy.

## Pinned API boundary

The OpenAPI pin is Dokploy commit
`cebd3808565ea9bed0791961bc25c60513d94c5a` (`openapi/source.json`). Research and
verification are recorded in `.superpowers/sdd/2026-10-04-gitlab-integration/`
`followup-api-research.md` and `followup-api-verification.md`; do not re-research
or change the pin as part of implementation. The pinned Gitea create operation
returns a positive `{giteaId, clientId, giteaUrl}` result; detail read is
`gitea.one`, update is `gitea.update`, and generic `gitProvider.remove` deletes the
parent with database cascade. The removal performs no external OAuth revocation
or host-side effect. Create/update and parent configuration interactions must
follow the pinned router/service behavior, not assumptions from OpenAPI's broad
placeholder responses.

Select only required active-organization/user identity operations, Gitea create,
one, update, provider listing/identity verification as required by the approved
implementation, and generic parent removal. Do not select OAuth callback, token
operations, connection tests, or repository APIs. Correct generated response
models from the already-reviewed pinned source to include only necessary Gitea
relation fields and nested parent identity/configuration; discard unrelated fields.

The public resource maps `clientSecret` to API `clientSecret`; never send or expose
callback-issued access/refresh tokens or expiry. Do not treat scopes as managed
configuration: although the DB has a default and API accepts a string, callback
lifecycle is not established and the user chose no scopes input.

## Lifecycle

### Create and preview

Create validates concrete inputs, obtains active organization and authenticated
user only if needed by the verified create contract, and sends exact managed
settings once. The create response's positive `giteaId` acknowledgment establishes
resource identity; do not invent identity by name/list reconciliation. Return that
ID and read back by it. Verify returned relation ID, nonempty parent ID, nested
parent identity, provider type `gitea`, active organization and managed settings.
The response/create acknowledgment alone does not prove unreturned parent or
organization fields were observed; never claim those as observed until verified.

If readback fails after acknowledged create, report partial state with the
acknowledged `giteaId` and only configuration actually acknowledged/verified;
do not claim unread parent or organization identity. Never retry create, attempt
uncertain cleanup, or adopt/delete a candidate by name. A proven pre-mutation
permission rejection does not trigger speculative recovery.

Create preview makes no API calls and leaves identity outputs computed. Update
preview retains established IDs, echoes managed inputs, and performs no API calls.
No preview fabricates readiness or API observations.

### Read and import

Fetch by relation ID. Require returned `giteaId` to equal the request; nonempty
parent ID to match nested parent ID; provider type `gitea`; active organization;
and any recorded parent/organization identity to remain stable. Import establishes
identity only from a verified record. If necessary, authenticated `user.get` may
establish the creator identity required for API operations; it is never a public
input or substitute for the observed owner.

Reconstruct only whitelisted observed configuration and IDs. Null internal URL
canonicalizes to omission. `organizationName` omission/null/empty canonicalizes
to empty string. Preserve prior `clientSecret` when refresh omits it; omission is
not evidence of credential rotation. An imported incomplete OAuth configuration
remains readable, but required settings must be supplied before update. Do not
invent placeholders or expose credential-bearing extra response fields.

Only structured genuine not-found removes state. Authorization, transport,
decoding, malformed identity, or organization mismatch are errors. Safe errors
must not reveal raw server text, IDs, private URLs, or credentials.

### Update

Pre-read and verify current relation, parent, type, and organization before
mutation. Send exact managed settings and verified IDs only; do not send callback-
managed extra fields, tokens, scopes, sharing, ownership, or unrelated settings.
All updates occur in place. `organizationName` is always sent as a string; empty
explicitly clears it. `giteaInternalUrl` is nullable on create/update; absence in
desired state maps to explicit null, clearing the override. Do not omit either
optional clear from the update payload.

Preserve a prior secret only during refresh when API response omits it. Update
requires a concrete nonempty desired client secret; omission from an import is not
proof of rotation and must not be silently converted into an empty value. A user
must resupply required OAuth settings before update if they are not observable.

After acknowledged mutation read back and verify stable IDs and exact canonical
managed settings. On failure, perform safe identity-verified readback only when
context permits and preserve actual verified/last-observed state; do not claim
unconfirmed desired settings. Never automatically retry mutations.

### Delete

Verify the relation and parent/type/organization, then call generic removal with
only the verified parent ID. A pre-read proving relation absence is idempotent
success. Treat a removal error as success only if verified read proves absence;
otherwise report safe failure. This deletes the Dokploy parent and cascades its
Gitea row, not the external OAuth application, grants, host configuration, or
workloads. Warn that unmanaged workloads may still reference the integration.

## Secrets, errors, and cancellation

Mark clientSecret and giteaInternalUrl secret throughout schema, state, and SDKs.
Do not include IDs, URLs, markers, request/response bodies, tokens, or arbitrary
upstream prose in resource diagnostics. Use fixed safe operation/status/context
classification; classify structured not-found and preserve cancellation/deadline
before discarding unsafe server text. No automatic mutation retry or cleanup after
uncertain create/update/delete.

## Deliverables and verification requirements for a future plan

Generate schema and Go, Node.js, Python, .NET, and Java SDKs from canonical source;
update resource inventories additively and preserve existing integrations. Update
canonical resource docs/examples to explain manual OAuth app configuration and
Dokploy authorization, ID-only import, secret handling, optional clears, lifecycle,
and limits. Do not imply Pulumi configures/authorizes OAuth or validates
connectivity. Keep workload source types unchanged. Regenerate reference docs from
schema.

Offline regression coverage must include per-field known/computed validation,
URL validation/canonicalization, defaults and secret metadata; in-place diffs and
stable preview IDs/no preview calls; exact create/update/delete payloads and
ordering; acknowledged create plus verified readback and partial state on read
failure; read/import identity/type/org and recorded identity mismatches; incomplete
imported credentials and secret omission; empty organization clear and null internal
URL clear; callback-field/token exclusion; structured not-found versus other
errors; delete idempotence and generic cascade semantics; mutation non-retry,
cancellation, and safe diagnostics including sensitive malformed responses.
Schema/OpenAPI tests must assert exact public inputs/outputs, no readiness/scopes/
tokens/sharing, secret markings, no replacement behavior, selected operations and
pinned response corrections.

Run applicable offline checks from the repository Makefile using the approved SAFE
launcher/configuration: provider tests, lint/race/build, OpenAPI/codegen drift,
all five SDK builds, examples, and docs checks. The launcher must avoid loading
protected `.env`, force live acceptance off including nested commands, and use the
repository generator portably; use actual local temp directory `/tmp/opencode`.
No live acceptance, external calls, release, publication, push, or unrelated
worktree is authorized. Inspect status/diff and `git diff --check`; report any
unavailable check rather than claiming it passed.

## Self-review and retention

The contract matches the approved manual OAuth boundary: no automatic app setup,
authorization, token control, readiness, scopes, or workload-source changes.
Identity comes from create acknowledgment and verified detail, not name matching;
partial state does not assert unread parent/organization. Optional clears are
explicit and secret omission is not rotation proof. API verification supports the
Gitea returned ID and generic parent cascade; no unsupported external side effect
is claimed. There are no TBD placeholders or implementation-authority claims.

Retain this spec and any future plan unless the user explicitly approves cleanup.
Preserve unrelated worktree changes.
